package commands

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/storage"
)

func (r *Router) registerSnapshots() {
	r.Add(CmdSpec{
		Name:        "snapshot",
		Aliases:     []string{"snap", "/snapshot", "/snap"},
		Description: "Snapshot commands for browsing and selecting world backups",
		Usage:       "/snap <list|use|sel|before|after>",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: /snap <list|use|sel|before|after>")
			}
			repo := storage.SnapshotRepo{Dir: ctx.WE.SnapshotsDir()}
			if repo.Dir == "" {
				return fmt.Errorf("snapshots.directory is not configured in worldedit-config.yml")
			}
			sub := strings.ToLower(ctx.Args[0])
			args := ctx.Args[1:]

			switch sub {
			case "list":
				if !ctx.Sender.HasPermission("worldedit.snapshots.list") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.snapshots.list")
					return nil
				}
				snaps, err := repo.List()
				if err != nil {
					return err
				}
				if len(snaps) == 0 {
					ctx.WE.Msgs.Info(ctx.Sender, "No snapshots found.")
					return nil
				}
				for i, s := range snaps {
					ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d. %s (%s)", i+1, s.Name, s.Time.In(ctx.Session.Timezone()).Format(time.RFC3339)))
				}

			case "use":
				if !ctx.Sender.HasPermission("worldedit.snapshots.restore") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.snapshots.restore")
					return nil
				}
				if len(args) == 0 {
					return fmt.Errorf("usage: /snap use <name|latest>")
				}
				snaps, err := repo.List()
				if err != nil || len(snaps) == 0 {
					return fmt.Errorf("no snapshots available")
				}
				if strings.EqualFold(args[0], "latest") {
					s := snaps[0]
					ctx.Session.SetSnapshot(&s)
					ctx.WE.Msgs.Info(ctx.Sender, "Now using latest snapshot: "+s.Name)
					return nil
				}
				for _, s := range snaps {
					if strings.EqualFold(s.Name, args[0]) || strings.EqualFold(strings.TrimSuffix(s.Name, ".zip"), args[0]) {
						cp := s
						ctx.Session.SetSnapshot(&cp)
						ctx.WE.Msgs.Info(ctx.Sender, "Now using snapshot: "+s.Name)
						return nil
					}
				}
				return fmt.Errorf("snapshot %q not found", args[0])

			case "sel":
				if !ctx.Sender.HasPermission("worldedit.snapshots.restore") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.snapshots.restore")
					return nil
				}
				if len(args) == 0 {
					return fmt.Errorf("usage: /snap sel <index>")
				}
				idx, err := strconv.Atoi(args[0])
				if err != nil || idx < 1 {
					return fmt.Errorf("invalid snapshot index %q", args[0])
				}
				snaps, err := repo.List()
				if err != nil || idx > len(snaps) {
					return fmt.Errorf("snapshot #%d not found", idx)
				}
				cp := snaps[idx-1]
				ctx.Session.SetSnapshot(&cp)
				ctx.WE.Msgs.Info(ctx.Sender, "Now using snapshot: "+cp.Name)

			case "before", "after":
				if !ctx.Sender.HasPermission("worldedit.snapshots.restore") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.snapshots.restore")
					return nil
				}
				if len(args) == 0 {
					return fmt.Errorf("usage: /snap %s <YYYY-MM-DD>", sub)
				}
				t, err := time.ParseInLocation("2006-01-02", args[0], ctx.Session.Timezone())
				if err != nil {
					t, err = time.Parse(time.RFC3339, args[0])
					if err != nil {
						return fmt.Errorf("invalid date %q (use YYYY-MM-DD)", args[0])
					}
				}
				var s storage.Snapshot
				var ok bool
				if sub == "before" {
					s, ok, err = repo.FindBefore(t)
				} else {
					s, ok, err = repo.FindAfter(t)
				}
				if err != nil || !ok {
					return fmt.Errorf("no snapshot found %s %s", sub, args[0])
				}
				ctx.Session.SetSnapshot(&s)
				ctx.WE.Msgs.Info(ctx.Sender, "Now using snapshot: "+s.Name)

			default:
				return fmt.Errorf("unknown snapshot subcommand %q", sub)
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "restore",
		Aliases:     []string{"/restore"},
		Permission:  "worldedit.snapshots.restore",
		Description: "Restore the selection from the selected snapshot",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			snap := ctx.Session.Snapshot()
			if len(ctx.Args) > 0 {
				repo := storage.SnapshotRepo{Dir: ctx.WE.SnapshotsDir()}
				snaps, _ := repo.List()
				for _, s := range snaps {
					if strings.EqualFold(s.Name, ctx.Args[0]) || strings.EqualFold(strings.TrimSuffix(s.Name, ".zip"), ctx.Args[0]) {
						cp := s
						snap = &cp
						break
					}
				}
			}
			if snap == nil {
				repo := storage.SnapshotRepo{Dir: ctx.WE.SnapshotsDir()}
				if snaps, _ := repo.List(); len(snaps) > 0 {
					cp := snaps[0]
					snap = &cp
				}
			}
			if snap == nil {
				return fmt.Errorf("no snapshot selected or available")
			}

			chunkCache := make(map[plugin.ChunkPos]*storage.AnvilChunk)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			reg.ForEach(func(p plugin.BlockPos) bool {
				cp := p.Chunk()
				ch, loaded := chunkCache[cp]
				if !loaded {
					c, err := storage.ReadSnapshotChunk(*snap, cp.X, cp.Z)
					if err == nil {
						ch = c
					}
					chunkCache[cp] = ch
				}
				if ch == nil {
					return true
				}
				es.SetBlock(p, ch.BlockState(p.X, p.Y, p.Z))
				return true
			})
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Restored %d block(s) from snapshot %s.", es.Changed(), snap.Name))
			return nil
		},
	})
}
