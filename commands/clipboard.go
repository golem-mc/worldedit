package commands

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/core"
	"github.com/golem-mc/worldedit/region"
	"github.com/golem-mc/worldedit/storage"
	"github.com/golem-mc/worldedit/tools"
)

func cbTransformIdentity() storage.ClipboardTransform { return storage.ClipboardTransform{} }

func (r *Router) registerClipboard() { //nolint:funlen // a command table reads better whole
	r.Add(CmdSpec{
		Name:        "/copy",
		Aliases:     []string{"/cp", "/lazycopy"},
		Permission:  "worldedit.clipboard.copy",
		Description: "Copy the selection to the clipboard",
		ValueFlags:  "m",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			origin := reg.Min()
			if ctx.Flags['c'] {
				ctr := reg.Center()
				origin = plugin.BlockPos{X: int(ctr.X), Y: reg.Min().Y, Z: int(ctr.Z)}
			} else if ctx.Player != nil {
				origin = ctx.WE.PlacementPos(ctx.Player, ctx.Session)
			}
			var includeMask block.Mask
			if mStr := ctx.FlagVal['m']; mStr != "" {
				includeMask, err = block.ParseMask(mStr, ctx.ParseContext())
				if err != nil {
					return err
				}
			}
			cb := tools.CopyRegionToClipboard(ctx.World, reg, origin, ctx.Flags['e'])
			if includeMask != nil {
				lo := reg.Min()
				ext := ctx.ParseContext().Extent
				air, _ := block.ParseState("air", false)
				reg.ForEach(func(p plugin.BlockPos) bool {
					if !includeMask.Test(ext, p) {
						cb.SetRel(p.X-lo.X, p.Y-lo.Y, p.Z-lo.Z, air)
					}
					return true
				})
			}
			if ctx.Flags['b'] {
				lo := reg.Min()
				reg.ForEach(func(p plugin.BlockPos) bool {
					cb.SetRelBiome(p.X-lo.X, p.Y-lo.Y, p.Z-lo.Z, ctx.World.Biome(p))
					return true
				})
			}
			if ctx.WE.Config().FAWE.Clipboard.UseDisk {
				path := fmt.Sprintf("%s/%s.bd", ctx.WE.ClipboardDir(), ctx.Session.PlayerUUID.String())
				_ = cb.SaveToDisk(path, ctx.WE.Config().FAWE.Clipboard.SaveClipboardNBT)
			}
			ctx.Session.SetClipboard(storage.NewClipboardHolder(cb))
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.copy.command.copy", cb.Volume())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/cut",
		Aliases:     []string{"/lazycut"},
		Permission:  "worldedit.clipboard.cut",
		Description: "Cut the selection to the clipboard",
		ValueFlags:  "m",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			origin := reg.Min()
			if ctx.Flags['c'] {
				ctr := reg.Center()
				origin = plugin.BlockPos{X: int(ctr.X), Y: reg.Min().Y, Z: int(ctr.Z)}
			} else if ctx.Player != nil {
				origin = ctx.WE.PlacementPos(ctx.Player, ctx.Session)
			}
			var cutMask block.Mask
			if mStr := ctx.FlagVal['m']; mStr != "" {
				cutMask, err = block.ParseMask(mStr, ctx.ParseContext())
				if err != nil {
					return err
				}
			}
			cb := tools.CopyRegionToClipboard(ctx.World, reg, origin, ctx.Flags['e'])
			if cutMask != nil {
				lo := reg.Min()
				ext := ctx.ParseContext().Extent
				air, _ := block.ParseState("air", false)
				reg.ForEach(func(p plugin.BlockPos) bool {
					if !cutMask.Test(ext, p) {
						cb.SetRel(p.X-lo.X, p.Y-lo.Y, p.Z-lo.Z, air)
					}
					return true
				})
			}
			holder := storage.NewClipboardHolder(cb)

			if strings.HasSuffix(ctx.Label, "lazycut") {
				holder.LazyCut = reg.Clone()
				ctx.Session.SetClipboard(holder)
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.cut.command.cut.lazy", reg.Volume())
				return nil
			}

			fillStr := "air"
			if len(ctx.Args) > 0 {
				fillStr = strings.Join(ctx.Args, " ")
			}
			pat, err := block.ParsePattern(fillStr, ctx.ParseContext())
			if err != nil {
				return err
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			if cutMask != nil {
				es.ReplaceRegion(reg, cutMask, pat)
			} else {
				es.SetRegion(reg, pat)
			}
			if ctx.Flags['e'] {
				lo, hi := reg.Min(), reg.Max()
				box := plugin.Box{
					Min: plugin.Vec3{X: float64(lo.X), Y: float64(lo.Y), Z: float64(lo.Z)},
					Max: plugin.Vec3{X: float64(hi.X + 1), Y: float64(hi.Y + 1), Z: float64(hi.Z + 1)},
				}
				for _, ent := range ctx.World.EntitiesIn(box) {
					if _, isP := ent.(plugin.Player); !isP {
						ent.Remove()
					}
				}
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.Session.SetClipboard(holder)
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.copy.command.copy", cb.Volume())
			return nil
		},
	})

	pasteHandler := func(ctx *CmdContext, useTransform bool) error {
		holder := ctx.Session.Clipboard()
		if holder == nil || holder.Pick() == nil {
			ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.empty-clipboard", ctx.Label)
			return nil
		}
		cb := holder.Pick()
		target := cb.Origin
		if !ctx.Flags['o'] && ctx.Player != nil {
			target = ctx.WE.PlacementPos(ctx.Player, ctx.Session)
		}
		ev := &core.ClipboardPasteEvent{Actor: ctx.Sender, World: ctx.World, Target: target}
		ctx.WE.Server.Events().Fire(ev)
		if ev.Cancelled() {
			return fmt.Errorf("paste cancelled")
		}

		es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
		if mStr := ctx.FlagVal['m']; mStr != "" {
			pm, err := block.ParseMask(mStr, ctx.ParseContext())
			if err != nil {
				return err
			}
			if es.DestMask != nil {
				es.DestMask = block.AndMask(es.DestMask, pm)
			} else {
				es.DestMask = pm
			}
		}
		if holder.LazyCut != nil {
			air, _ := block.ParseState("air", false)
			es.SetRegion(holder.LazyCut, block.SinglePattern(air))
			holder.LazyCut = nil
		}
		skipAir := ctx.Flags['a']
		pasteEntities := !ctx.Flags['e']
		transform := holder.Transform
		n := tools.PasteClipboard(es, cb, transform, target, skipAir, pasteEntities, useTransform)
		if err := es.Commit(); err != nil {
			return err
		}
		if ctx.Flags['s'] {
			applyModifiedRegion(ctx, tools.ClipboardPasteRegion(cb, transform, target, useTransform))
		}
		ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.paste.command.paste", fmt.Sprintf("%s (%d blocks)", region.FormatPos(target), n))
		return nil
	}

	r.Add(CmdSpec{
		Name:        "/paste",
		Aliases:     []string{"/p", "/pa"},
		Permission:  "worldedit.clipboard.paste",
		Description: "Paste the clipboard's contents",
		ValueFlags:  "m",
		Run:         func(ctx *CmdContext) error { return pasteHandler(ctx, true) },
	})

	r.Add(CmdSpec{
		Name:        "/place",
		Permission:  "worldedit.clipboard.place",
		Description: "Place the clipboard's contents without applying transformations",
		ValueFlags:  "m",
		Run:         func(ctx *CmdContext) error { return pasteHandler(ctx, false) },
	})

	r.Add(CmdSpec{
		Name:        "/rotate",
		Permission:  "worldedit.clipboard.rotate",
		Description: "Rotate the contents of the clipboard (y [x] [z] degrees)",
		Run: func(ctx *CmdContext) error {
			holder := ctx.Session.Clipboard()
			if holder == nil || holder.Primary() == nil {
				ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.empty-clipboard", "//rotate")
				return nil
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //rotate <y-deg> [x-deg] [z-deg]")
			}
			ry, err := strconv.Atoi(ctx.Args[0])
			if err != nil {
				return fmt.Errorf("invalid angle %q", ctx.Args[0])
			}
			rx, rz := 0, 0
			if len(ctx.Args) >= 2 {
				rx, _ = strconv.Atoi(ctx.Args[1])
			}
			if len(ctx.Args) >= 3 {
				rz, _ = strconv.Atoi(ctx.Args[2])
			}
			holder.Rotate(ry, rx, rz)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Clipboard rotated by (%d, %d, %d) degrees.", ry, rx, rz))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/flip",
		Aliases:     []string{"/mirror"},
		Permission:  "worldedit.clipboard.flip",
		Description: "Flip the contents of the clipboard across the origin",
		Run: func(ctx *CmdContext) error {
			holder := ctx.Session.Clipboard()
			if holder == nil || holder.Primary() == nil {
				ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.empty-clipboard", "//flip")
				return nil
			}
			dirStr := ""
			if len(ctx.Args) > 0 {
				dirStr = strings.ToLower(ctx.Args[0])
			}
			yaw, pitch := playerYawPitch(ctx.Player)
			dir, err := core.DirectionFromString(dirStr, yaw, pitch)
			if err != nil {
				return err
			}
			axis := "z"
			if dir.X != 0 {
				axis = "x"
			} else if dir.Y != 0 {
				axis = "y"
			}
			holder.Flip(axis)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Clipboard flipped along the %s axis.", strings.ToUpper(axis)))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "clearclipboard",
		Aliases:     []string{"/clearclipboard", "/cc", "/clearclip"},
		Permission:  "worldedit.clipboard.clear",
		Description: "Clear your clipboard",
		Run: func(ctx *CmdContext) error {
			ctx.Session.SetClipboard(nil)
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.clipboard.clipboard.cleared")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "download",
		Aliases:     []string{"/download"},
		Permission:  "worldedit.clipboard.download",
		Description: "Uploads your clipboard to the configured web interface and returns a download link",
		Run: func(ctx *CmdContext) error {
			holder := ctx.Session.Clipboard()
			if holder == nil || holder.Primary() == nil {
				ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.empty-clipboard", "/download")
				return nil
			}
			cb := holder.Primary()
			format := storage.FormatFastV3
			if len(ctx.Args) > 0 {
				if fi, ok := storage.FindFormat(ctx.Args[0]); ok {
					format = fi.Format
				}
			}
			var buf bytes.Buffer
			if err := storage.WriteSchematic(&buf, cb, format); err != nil {
				return err
			}
			webCfg := ctx.WE.Config().FAWE.Web
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.web.generating.link", "clipboard.schem")
			endpoint := strings.TrimRight(webCfg.ArkitektonikaBackendURL, "/") + "/upload"
			ctx.WE.Server.Scheduler().RunAsync(func() {
				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Post(endpoint, "application/octet-stream", bytes.NewReader(buf.Bytes()))
				if err != nil {
					ctx.WE.Server.Scheduler().RunTask(func() {
						ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.web.generating.link.failed")
					})
					return
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
				ctx.WE.Server.Scheduler().RunTask(func() {
					if resp.StatusCode >= 200 && resp.StatusCode < 300 {
						ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.web.download.link", strings.TrimSpace(string(body)))
					} else {
						ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.web.generating.link.failed")
					}
				})
			})
			return nil
		},
	})
}
