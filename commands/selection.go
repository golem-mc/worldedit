package commands

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/core"
	"github.com/golem-mc/worldedit/region"
	"github.com/golem-mc/worldedit/tools"
)

func (r *Router) registerSelection() {
	r.Add(CmdSpec{
		Name:        "/pos1",
		Aliases:     []string{"/1"},
		Permission:  "worldedit.selection.pos",
		Description: "Set position 1",
		Run: func(ctx *CmdContext) error {
			pos, err := resolvePosArg(ctx)
			if err != nil {
				return err
			}
			if ctx.Player != nil {
				tools.SelectPoint(ctx.WE, ctx.Player, ctx.Session, ctx.World, pos, true)
			} else {
				msg, _ := ctx.Session.Selector(ctx.World).SelectPrimary(pos, 0)
				ctx.WE.Msgs.Info(ctx.Sender, msg)
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/pos2",
		Aliases:     []string{"/2"},
		Permission:  "worldedit.selection.pos",
		Description: "Set position 2",
		Run: func(ctx *CmdContext) error {
			pos, err := resolvePosArg(ctx)
			if err != nil {
				return err
			}
			if ctx.Player != nil {
				tools.SelectPoint(ctx.WE, ctx.Player, ctx.Session, ctx.World, pos, false)
			} else {
				msg, _ := ctx.Session.Selector(ctx.World).SelectSecondary(pos, 0)
				ctx.WE.Msgs.Info(ctx.Sender, msg)
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/hpos1",
		Permission:  "worldedit.selection.hpos",
		Description: "Set position 1 to targeted block",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			blk, ok := ctx.Player.TargetBlock(120)
			if !ok {
				return fmt.Errorf("no block in sight")
			}
			tools.SelectPoint(ctx.WE, ctx.Player, ctx.Session, ctx.World, blk.Pos, true)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/hpos2",
		Permission:  "worldedit.selection.hpos",
		Description: "Set position 2 to targeted block",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			blk, ok := ctx.Player.TargetBlock(120)
			if !ok {
				return fmt.Errorf("no block in sight")
			}
			tools.SelectPoint(ctx.WE, ctx.Player, ctx.Session, ctx.World, blk.Pos, false)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/chunk",
		Permission:  "worldedit.selection.chunk",
		Description: "Set the selection to your current chunk",
		Run: func(ctx *CmdContext) error {
			var cx, cz int
			if len(ctx.Args) >= 1 {
				parts := strings.Split(ctx.Args[0], ",")
				if len(parts) == 2 {
					cx, _ = strconv.Atoi(parts[0])
					cz, _ = strconv.Atoi(parts[1])
				} else if len(ctx.Args) >= 2 {
					cx, _ = strconv.Atoi(ctx.Args[0])
					cz, _ = strconv.Atoi(ctx.Args[1])
				}
			} else if ctx.Player != nil {
				cp := ctx.Player.Pos().Block().Chunk()
				cx, cz = cp.X, cp.Z
			}
			minY := ctx.World.MinY()
			maxY := minY + ctx.World.Height() - 1
			sel := ctx.Session.SetSelectorType(ctx.World, region.SelectorCuboid)
			sel.SelectPrimary(plugin.BlockPos{X: cx << 4, Y: minY, Z: cz << 4}, 0)
			sel.SelectSecondary(plugin.BlockPos{X: (cx << 4) + 15, Y: maxY, Z: (cz << 4) + 15}, 0)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Chunk (%d, %d) selected.", cx, cz))
			ctx.WE.DispatchCUI(ctx.Player, ctx.Session)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/wand",
		Permission:  "worldedit.wand",
		Description: "Get the selection wand item",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			item, ok := ctx.WE.WandItem()
			if !ok {
				return fmt.Errorf("configured wand item %q is invalid", ctx.WE.Config().WorldEdit.WandItem)
			}
			ctx.Player.Inventory().Add(plugin.NewStack(item, 1))
			ctx.WE.Msgs.Info(ctx.Sender, "Left click: select pos #1; Right click: select pos #2")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "toggleeditwand",
		Aliases:     []string{"/toggleeditwand"},
		Permission:  "worldedit.wand.toggle",
		Description: "Toggle the selection wand",
		Run: func(ctx *CmdContext) error {
			next := !ctx.Session.WandDisabled()
			ctx.Session.SetWandDisabled(next)
			if next {
				ctx.WE.Msgs.Info(ctx.Sender, "Edit wand disabled.")
			} else {
				ctx.WE.Msgs.Info(ctx.Sender, "Edit wand enabled.")
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/sel",
		Aliases:     []string{";", "/;", "/desel", "/deselect"},
		Permission:  "worldedit.analysis.sel",
		Description: "Choose a region selector or clear your selection",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				ctx.Session.Selector(ctx.World).Clear()
				ctx.WE.Msgs.Info(ctx.Sender, "Selection cleared.")
				ctx.WE.DispatchCUI(ctx.Player, ctx.Session)
				return nil
			}
			st, ok := region.ParseSelectorType(ctx.Args[0])
			if !ok {
				return fmt.Errorf("unknown selector %q (cuboid, extend, poly, ellipsoid, sphere, cyl, convex, fuzzy)", ctx.Args[0])
			}
			ctx.Session.SetSelectorType(ctx.World, st)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Selection mode set to %s.", st))
			ctx.WE.DispatchCUI(ctx.Player, ctx.Session)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/expand",
		Permission:  "worldedit.selection.expand",
		Description: "Expand the selection area",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //expand <amount> [reverse-amount] [direction] | //expand vert")
			}
			var changes []plugin.BlockPos
			if strings.EqualFold(ctx.Args[0], "vert") {
				minY := ctx.World.MinY()
				maxY := minY + ctx.World.Height() - 1
				changes = append(changes,
					plugin.BlockPos{Y: minY - reg.Min().Y},
					plugin.BlockPos{Y: maxY - reg.Max().Y},
				)
			} else {
				amt, err := strconv.Atoi(ctx.Args[0])
				if err != nil {
					return fmt.Errorf("invalid amount %q", ctx.Args[0])
				}
				rev := 0
				dirStr := ""
				if len(ctx.Args) >= 2 {
					if rval, err := strconv.Atoi(ctx.Args[1]); err == nil {
						rev = rval
						if len(ctx.Args) >= 3 {
							dirStr = strings.ToLower(ctx.Args[2])
						}
					} else {
						dirStr = strings.ToLower(ctx.Args[1])
					}
				}
				yaw, pitch := playerYawPitch(ctx.Player)
				dir, err := core.DirectionFromString(dirStr, yaw, pitch)
				if err != nil {
					return err
				}
				changes = append(changes, plugin.BlockPos{X: dir.X * amt, Y: dir.Y * amt, Z: dir.Z * amt})
				if rev != 0 {
					changes = append(changes, plugin.BlockPos{X: -dir.X * rev, Y: -dir.Y * rev, Z: -dir.Z * rev})
				}
			}
			before := reg.Volume()
			if err := reg.Expand(changes...); err != nil {
				return fmt.Errorf("selection cannot expand: %w", err)
			}
			applyModifiedRegion(ctx, reg)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Region expanded %d blocks (now %d).", reg.Volume()-before, reg.Volume()))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/contract",
		Permission:  "worldedit.selection.contract",
		Description: "Contract the selection area",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //contract <amount> [reverse-amount] [direction]")
			}
			amt, err := strconv.Atoi(ctx.Args[0])
			if err != nil {
				return fmt.Errorf("invalid amount %q", ctx.Args[0])
			}
			rev := 0
			dirStr := ""
			if len(ctx.Args) >= 2 {
				if rval, err := strconv.Atoi(ctx.Args[1]); err == nil {
					rev = rval
					if len(ctx.Args) >= 3 {
						dirStr = strings.ToLower(ctx.Args[2])
					}
				} else {
					dirStr = strings.ToLower(ctx.Args[1])
				}
			}
			yaw, pitch := playerYawPitch(ctx.Player)
			dir, err := core.DirectionFromString(dirStr, yaw, pitch)
			if err != nil {
				return err
			}
			changes := []plugin.BlockPos{{X: dir.X * amt, Y: dir.Y * amt, Z: dir.Z * amt}}
			if rev != 0 {
				changes = append(changes, plugin.BlockPos{X: -dir.X * rev, Y: -dir.Y * rev, Z: -dir.Z * rev})
			}
			before := reg.Volume()
			if err := reg.Contract(changes...); err != nil {
				return fmt.Errorf("selection cannot contract: %w", err)
			}
			applyModifiedRegion(ctx, reg)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Region contracted %d blocks (now %d).", before-reg.Volume(), reg.Volume()))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/shift",
		Permission:  "worldedit.selection.shift",
		Description: "Shift the selection area",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //shift <amount> [direction]")
			}
			amt, err := strconv.Atoi(ctx.Args[0])
			if err != nil {
				return fmt.Errorf("invalid amount %q", ctx.Args[0])
			}
			dirStr := ""
			if len(ctx.Args) >= 2 {
				dirStr = strings.ToLower(ctx.Args[1])
			}
			yaw, pitch := playerYawPitch(ctx.Player)
			dir, err := core.DirectionFromString(dirStr, yaw, pitch)
			if err != nil {
				return err
			}
			if err := reg.Shift(plugin.BlockPos{X: dir.X * amt, Y: dir.Y * amt, Z: dir.Z * amt}); err != nil {
				return err
			}
			applyModifiedRegion(ctx, reg)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Region shifted by %d.", amt))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/outset",
		Permission:  "worldedit.selection.outset",
		Description: "Outset the selection area",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //outset [-h|-v] <amount>")
			}
			amt, err := strconv.Atoi(ctx.Args[0])
			if err != nil {
				return fmt.Errorf("invalid amount %q", ctx.Args[0])
			}
			changes := outsetVectors(amt, ctx.Flags['h'], ctx.Flags['v'])
			if err := reg.Expand(changes...); err != nil {
				return err
			}
			applyModifiedRegion(ctx, reg)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Region outset by %d (volume %d).", amt, reg.Volume()))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/inset",
		Permission:  "worldedit.selection.inset",
		Description: "Inset the selection area",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //inset [-h|-v] <amount>")
			}
			amt, err := strconv.Atoi(ctx.Args[0])
			if err != nil {
				return fmt.Errorf("invalid amount %q", ctx.Args[0])
			}
			changes := outsetVectors(amt, ctx.Flags['h'], ctx.Flags['v'])
			if err := reg.Contract(changes...); err != nil {
				return err
			}
			applyModifiedRegion(ctx, reg)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Region inset by %d (volume %d).", amt, reg.Volume()))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/trim",
		Permission:  "worldedit.selection.trim",
		Description: "Minimize the selection to encompass matching blocks",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			maskStr := "!air"
			if len(ctx.Args) > 0 {
				maskStr = strings.Join(ctx.Args, " ")
			}
			m, err := block.ParseMask(maskStr, ctx.ParseContext())
			if err != nil {
				return err
			}
			ext := ctx.ParseContext().Extent
			var lo, hi plugin.BlockPos
			found := false
			reg.ForEach(func(p plugin.BlockPos) bool {
				if m.Test(ext, p) {
					if !found {
						lo, hi = p, p
						found = true
					} else {
						lo.X, lo.Y, lo.Z = min(lo.X, p.X), min(lo.Y, p.Y), min(lo.Z, p.Z)
						hi.X, hi.Y, hi.Z = max(hi.X, p.X), max(hi.Y, p.Y), max(hi.Z, p.Z)
					}
				}
				return true
			})
			if !found {
				return fmt.Errorf("no matching blocks found in selection")
			}
			applyModifiedRegion(ctx, region.NewCuboid(lo, hi))
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Selection trimmed to %s -> %s.", region.FormatPos(lo), region.FormatPos(hi)))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/size",
		Permission:  "worldedit.selection.size",
		Description: "Get information about the selection or clipboard (-c)",
		Run: func(ctx *CmdContext) error {
			if ctx.Flags['c'] {
				h := ctx.Session.Clipboard()
				if h == nil || h.Primary() == nil {
					return fmt.Errorf("clipboard is empty")
				}
				cb := h.Primary()
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Clipboard: %dx%dx%d (%d blocks), origin %s",
					cb.Width, cb.Height, cb.Length, cb.Volume(), region.FormatPos(cb.Origin)))
				return nil
			}
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Type: %s | Bounds: %s -> %s | Dimensions: %dx%dx%d | Volume: %d",
				reg.TypeName(), region.FormatPos(reg.Min()), region.FormatPos(reg.Max()),
				reg.Width(), reg.Height(), reg.Length(), reg.Volume()))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/count",
		Permission:  "worldedit.analysis.count",
		Description: "Counts the number of blocks matching a mask",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //count <mask>")
			}
			m, err := block.ParseMask(strings.Join(ctx.Args, " "), ctx.ParseContext())
			if err != nil {
				return err
			}
			ext := ctx.ParseContext().Extent
			var cnt int64
			reg.ForEach(func(p plugin.BlockPos) bool {
				if m.Test(ext, p) {
					cnt++
				}
				return true
			})
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.selection.selection.count", cnt)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/distr",
		Permission:  "worldedit.analysis.distr",
		Description: "Get the distribution of blocks in the selection or clipboard (-c)",
		Run: func(ctx *CmdContext) error {
			separateStates := ctx.Flags['d']
			counts := make(map[string]int64)
			var total int64
			if ctx.Flags['c'] {
				h := ctx.Session.Clipboard()
				if h == nil || h.Primary() == nil {
					return fmt.Errorf("clipboard is empty")
				}
				for _, st := range h.Primary().Blocks {
					k := block.BlockName(st)
					if separateStates {
						k = block.FormatState(st)
					}
					counts[k]++
					total++
				}
			} else {
				reg, err := ctx.Session.Selection(ctx.World)
				if err != nil {
					return fmt.Errorf("make a region selection first")
				}
				reg.ForEach(func(p plugin.BlockPos) bool {
					st := ctx.World.BlockState(p)
					k := block.BlockName(st)
					if separateStates {
						k = block.FormatState(st)
					}
					counts[k]++
					total++
					return true
				})
			}
			type kv struct {
				k string
				v int64
			}
			list := make([]kv, 0, len(counts))
			for k, v := range counts {
				list = append(list, kv{k, v})
			}
			sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Total blocks: %d", total))
			for i, e := range list {
				if i >= 15 {
					break
				}
				pct := float64(e.v) * 100.0 / float64(max(1, total))
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d (%.2f%%) %s", e.v, pct, e.k))
			}
			return nil
		},
	})
}

func resolvePosArg(ctx *CmdContext) (plugin.BlockPos, error) {
	if len(ctx.Args) == 1 && strings.Contains(ctx.Args[0], ",") {
		parts := strings.Split(ctx.Args[0], ",")
		if len(parts) != 3 {
			return plugin.BlockPos{}, fmt.Errorf("expected x,y,z coordinates")
		}
		x, err1 := strconv.Atoi(parts[0])
		y, err2 := strconv.Atoi(parts[1])
		z, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return plugin.BlockPos{}, fmt.Errorf("invalid coordinates %q", ctx.Args[0])
		}
		return plugin.BlockPos{X: x, Y: y, Z: z}, nil
	}
	if len(ctx.Args) >= 3 {
		x, err1 := strconv.Atoi(ctx.Args[0])
		y, err2 := strconv.Atoi(ctx.Args[1])
		z, err3 := strconv.Atoi(ctx.Args[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return plugin.BlockPos{}, fmt.Errorf("invalid coordinates")
		}
		return plugin.BlockPos{X: x, Y: y, Z: z}, nil
	}
	if ctx.Player == nil {
		return plugin.BlockPos{}, fmt.Errorf("console must specify x,y,z coordinates")
	}
	return ctx.Player.Pos().Block(), nil
}

func playerYawPitch(p plugin.Player) (float32, float32) {
	if p == nil {
		return 0, 0
	}
	loc := p.Location()
	return loc.Yaw, loc.Pitch
}

func outsetVectors(amt int, horiz, vert bool) []plugin.BlockPos {
	if !horiz && !vert {
		horiz, vert = true, true
	}
	var out []plugin.BlockPos
	if horiz {
		out = append(out,
			plugin.BlockPos{X: amt}, plugin.BlockPos{X: -amt},
			plugin.BlockPos{Z: amt}, plugin.BlockPos{Z: -amt},
		)
	}
	if vert {
		out = append(out, plugin.BlockPos{Y: amt}, plugin.BlockPos{Y: -amt})
	}
	return out
}

func applyModifiedRegion(ctx *CmdContext, reg region.Region) {
	if cub, ok := reg.(*region.Cuboid); ok {
		sel := ctx.Session.SetSelectorType(ctx.World, region.SelectorCuboid)
		sel.SelectPrimary(cub.Min(), 0)
		sel.SelectSecondary(cub.Max(), 0)
	}
	ctx.WE.DispatchCUI(ctx.Player, ctx.Session)
}
