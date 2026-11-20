package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/golem-mc/worldedit/core"
	"github.com/golem-mc/worldedit/storage"
)

func (r *Router) registerSchematic() {
	r.Add(CmdSpec{
		Name:        "schematic",
		Aliases:     []string{"schem", "/schematic", "/schem"},
		Description: "Schematic commands for saving, loading, listing, moving, and deleting schematics",
		Usage:       "/schem <load|loadall|save|delete|move|list|formats|clear|unload|share> [args...]",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: /schem <load|loadall|save|delete|move|list|formats|clear|unload|share>")
			}
			sub := strings.ToLower(ctx.Args[0])
			args := ctx.Args[1:]

			schemBaseDir := ctx.WE.SchematicsDir()
			if ctx.WE.Config().FAWE.Paths.PerPlayerSchematic && ctx.Player != nil && !ctx.Flags['g'] {
				schemBaseDir = filepath.Join(schemBaseDir, ctx.Player.UUID().String())
				_ = os.MkdirAll(schemBaseDir, 0o755)
			}

			switch sub {
			case "formats", "listformats", "f":
				if !ctx.Sender.HasPermission("worldedit.schematic.formats") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.schematic.formats")
					return nil
				}
				var parts []string
				for _, f := range storage.AllFormats {
					parts = append(parts, fmt.Sprintf("%s (.%s, aliases: %s)", f.Format, f.Extension, strings.Join(f.Aliases, ",")))
				}
				ctx.WE.Msgs.Info(ctx.Sender, "Available formats: "+strings.Join(parts, " | "))

			case "list", "all", "ls":
				if !ctx.Sender.HasPermission("worldedit.schematic.list") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.schematic.list")
					return nil
				}
				var files []string
				_ = filepath.WalkDir(schemBaseDir, func(path string, d os.DirEntry, err error) error {
					if err != nil || d.IsDir() {
						return nil
					}
					rel, _ := filepath.Rel(schemBaseDir, path)
					files = append(files, filepath.ToSlash(rel))
					return nil
				})
				sort.Strings(files)
				if len(files) == 0 {
					ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.schematic.schematic.none")
					return nil
				}
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Schematics (%d): %s", len(files), strings.Join(files, ", ")))

			case "save":
				if !ctx.Sender.HasPermission("worldedit.schematic.save") && !ctx.Sender.HasPermission("worldedit.clipboard.save") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.schematic.save")
					return nil
				}
				holder := ctx.Session.Clipboard()
				if holder == nil || holder.Primary() == nil {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.empty-clipboard", "/schem save")
					return nil
				}
				if len(args) == 0 {
					return fmt.Errorf("usage: /schem save [format] <filename>")
				}
				format := storage.FormatFastV3
				filename := args[0]
				if len(args) >= 2 {
					if fi, ok := storage.FindFormat(args[0]); ok {
						format = fi.Format
						filename = args[1]
					} else if fi, ok := storage.FindFormat(args[1]); ok {
						format = fi.Format
						filename = args[0]
					}
				}
				ext := "schem"
				for _, fi := range storage.AllFormats {
					if fi.Format == format {
						ext = fi.Extension
						break
					}
				}
				clean := filepath.Clean(filename)
				if strings.Contains(clean, "..") {
					return fmt.Errorf("invalid schematic path %q", filename)
				}
				if !strings.HasSuffix(strings.ToLower(clean), "."+ext) {
					clean += "." + ext
				}
				full := filepath.Join(schemBaseDir, clean)
				_, existed := os.Stat(full)
				if existed == nil && !ctx.Flags['f'] {
					return fmt.Errorf("schematic %q already exists (pass -f to overwrite)", clean)
				}
				lim := ctx.WE.Config().LimitFor(ctx.Sender)
				if existed != nil && ctx.WE.Config().FAWE.Paths.PerPlayerSchematic && lim.SchemFileNumLimit > 0 && !lim.Unlimited {
					var existingCount int
					_ = filepath.WalkDir(schemBaseDir, func(_ string, d os.DirEntry, err error) error {
						if err == nil && !d.IsDir() {
							existingCount++
						}
						return nil
					})
					if existingCount >= lim.SchemFileNumLimit {
						return fmt.Errorf("schematic file limit (%d) reached", lim.SchemFileNumLimit)
					}
				}
				ev := &core.SchematicSaveEvent{Actor: ctx.Sender, Path: full, Format: format}
				ctx.WE.Server.Events().Fire(ev)
				if ev.Cancelled() {
					return fmt.Errorf("schematic save cancelled")
				}
				if err := storage.SaveSchematicFile(full, holder.Primary(), format); err != nil {
					return err
				}
				if existed == nil {
					ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.schematic.schematic.overwritten", clean)
				} else {
					ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.schematic.schematic.saved", clean)
				}

			case "load":
				if !ctx.Sender.HasPermission("worldedit.schematic.load") && !ctx.Sender.HasPermission("worldedit.clipboard.load") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.schematic.load")
					return nil
				}
				if len(args) == 0 {
					return fmt.Errorf("usage: /schem load [format] <filename>")
				}
				var format storage.ClipboardFormat
				filename := args[0]
				if len(args) >= 2 {
					if fi, ok := storage.FindFormat(args[0]); ok {
						format = fi.Format
						filename = args[1]
					}
				}
				full, err := findSchematicFile(schemBaseDir, filename)
				if err != nil {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.worldedit.schematic.schematic.load-failure", filename)
					return nil
				}
				ev := &core.SchematicLoadEvent{Actor: ctx.Sender, Path: full, Format: format}
				ctx.WE.Server.Events().Fire(ev)
				if ev.Cancelled() {
					return fmt.Errorf("schematic load cancelled")
				}
				cb, err := storage.ReadSchematicFile(full, format)
				if err != nil {
					return err
				}
				ctx.Session.SetClipboard(storage.NewClipboardHolder(cb))
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.schematic.schematic.loaded", filepath.Base(full))

			case "loadall":
				if !ctx.Sender.HasPermission("worldedit.schematic.load") && !ctx.Sender.HasPermission("worldedit.clipboard.load") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.schematic.load")
					return nil
				}
				if len(args) == 0 {
					return fmt.Errorf("usage: /schem loadall <name1,name2,...>")
				}
				holder := &storage.ClipboardHolder{}
				loaded := 0
				for part := range strings.SplitSeq(strings.Join(args, ","), ",") {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					full, err := findSchematicFile(schemBaseDir, part)
					if err != nil {
						continue
					}
					if cb, err := storage.ReadSchematicFile(full, ""); err == nil {
						holder.Add(cb)
						loaded++
					}
				}
				if loaded == 0 {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.worldedit.schematic.schematic.none")
					return nil
				}
				ctx.Session.SetClipboard(holder)
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Loaded %d schematic(s) into multi-clipboard.", loaded))

			case "delete", "d":
				if !ctx.Sender.HasPermission("worldedit.schematic.delete") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.schematic.delete")
					return nil
				}
				if len(args) == 0 {
					return fmt.Errorf("usage: /schem delete <filename>")
				}
				full, err := findSchematicFile(schemBaseDir, args[0])
				if err != nil {
					return fmt.Errorf("schematic %q not found", args[0])
				}
				if err := os.Remove(full); err != nil {
					return err
				}
				ctx.WE.Msgs.Info(ctx.Sender, filepath.Base(full)+" has been deleted.")

			case "move", "m":
				if !ctx.Sender.HasPermission("worldedit.schematic.move") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.schematic.move")
					return nil
				}
				if len(args) < 2 {
					return fmt.Errorf("usage: /schem move <src> <dst>")
				}
				src, err := findSchematicFile(schemBaseDir, args[0])
				if err != nil {
					return fmt.Errorf("schematic %q not found", args[0])
				}
				dstClean := filepath.Clean(args[1])
				if strings.Contains(dstClean, "..") {
					return fmt.Errorf("invalid destination %q", args[1])
				}
				dst := filepath.Join(schemBaseDir, dstClean)
				if filepath.Ext(dst) == "" {
					dst += filepath.Ext(src)
				}
				if _, err := os.Stat(dst); err == nil {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.worldedit.schematic.schematic.move.exists", dstClean)
					return nil
				}
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					return err
				}
				if err := os.Rename(src, dst); err != nil {
					return err
				}
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.schematic.schematic.move.success", filepath.Base(src), filepath.Base(dst))

			case "clear":
				ctx.Session.SetClipboard(nil)
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.clipboard.clipboard.cleared")

			case "unload":
				if len(args) == 0 {
					return fmt.Errorf("usage: /schem unload <filename>")
				}
				h := ctx.Session.Clipboard()
				if h == nil || !h.UnloadByName(args[0]) {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.worldedit.clipboard.clipboard.uri.not.found", args[0])
					return nil
				}
				ctx.WE.Msgs.Info(ctx.Sender, "Unloaded "+args[0])

			case "share":
				// Delegate to /download
				if sp, ok := r.byName["download"]; ok {
					return r.Execute(ctx.Sender, "download", args, "download", sp)
				}

			default:
				return fmt.Errorf("unknown schematic subcommand %q", sub)
			}
			return nil
		},
	})
}

func findSchematicFile(dir, name string) (string, error) {
	clean := filepath.Clean(name)
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid path")
	}
	candidates := []string{
		filepath.Join(dir, clean),
		filepath.Join(dir, clean+".schem"),
		filepath.Join(dir, clean+".schematic"),
		filepath.Join(dir, clean+".nbt"),
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}
