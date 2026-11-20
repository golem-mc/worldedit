package commands

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/region"
	"github.com/golem-mc/worldedit/storage"
	"github.com/google/uuid"
)

func (r *Router) registerHistory() { //nolint:funlen // a command table reads better whole
	r.Add(CmdSpec{
		Name:        "undo",
		Aliases:     []string{"/undo"},
		Permission:  "worldedit.history.undo",
		Description: "Undoes the last action(s) from history",
		Run: func(ctx *CmdContext) error {
			if ctx.Session.FastMode() {
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.history.command.undo.disabled")
				return nil
			}
			times := 1
			targetSess := ctx.Session
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					times = n
				}
			}
			if len(ctx.Args) >= 2 && ctx.Sender.HasPermission("worldedit.history.undo.other") {
				if pl, ok := ctx.WE.Server.PlayerNamed(ctx.Args[1]); ok {
					targetSess = ctx.WE.Session(pl)
				}
			}
			var allowed []region.Region
			if ctx.Player != nil && ctx.WE.Config().LimitFor(ctx.Sender).RestrictHistoryToRegions && !ctx.Session.BypassRegions() {
				allowed = ctx.WE.AllowedRegions(ctx.Player, ctx.World)
			}
			undone := 0
			var totalBlocks int64
			for i := 0; i < times; i++ {
				cs := targetSess.PopUndo()
				if cs == nil {
					break
				}
				n, err := ctx.WE.UndoChangeSet(ctx.World, cs, allowed)
				if err != nil {
					return err
				}
				totalBlocks += n
				undone++
			}
			if undone == 0 {
				ctx.WE.Msgs.Error(ctx.Sender, "Nothing left to undo.")
				return nil
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Undo successful (%d edit(s), %d blocks restored).", undone, totalBlocks))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "redo",
		Aliases:     []string{"/redo"},
		Permission:  "worldedit.history.redo",
		Description: "Redoes the last undone action(s)",
		Run: func(ctx *CmdContext) error {
			times := 1
			targetSess := ctx.Session
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					times = n
				}
			}
			if len(ctx.Args) >= 2 && ctx.Sender.HasPermission("worldedit.history.redo.other") {
				if pl, ok := ctx.WE.Server.PlayerNamed(ctx.Args[1]); ok {
					targetSess = ctx.WE.Session(pl)
				}
			}
			var allowed []region.Region
			if ctx.Player != nil && ctx.WE.Config().LimitFor(ctx.Sender).RestrictHistoryToRegions && !ctx.Session.BypassRegions() {
				allowed = ctx.WE.AllowedRegions(ctx.Player, ctx.World)
			}
			redone := 0
			var totalBlocks int64
			for i := 0; i < times; i++ {
				cs := targetSess.PopRedo()
				if cs == nil {
					break
				}
				n, err := ctx.WE.RedoChangeSet(ctx.World, cs, allowed)
				if err != nil {
					return err
				}
				totalBlocks += n
				redone++
			}
			if redone == 0 {
				ctx.WE.Msgs.Error(ctx.Sender, "Nothing left to redo.")
				return nil
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Redo successful (%d edit(s), %d blocks re-applied).", redone, totalBlocks))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "clearhistory",
		Aliases:     []string{"/clearhistory"},
		Permission:  "worldedit.history.clear",
		Description: "Clear your undo/redo history",
		Run: func(ctx *CmdContext) error {
			ctx.Session.ClearHistory()
			ctx.WE.Msgs.Info(ctx.Sender, "History cleared.")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "history",
		Aliases:     []string{"/history", "frb", "/frb"},
		Description: "Inspect, search, rollback, restore, and import FAWE edit history",
		Usage:       "//history <list|find|info|distr|rollback|restore|clear|import> [args...]",
		ValueFlags:  "urt",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //history <list|find|info|distr|rollback|restore|clear|import>")
			}
			sub := strings.ToLower(ctx.Args[0])
			args := ctx.Args[1:]
			rdb := ctx.WE.RollbackDB(ctx.World.Key().String())

			switch sub {
			case "clear":
				if !ctx.Sender.HasPermission("worldedit.history.clear") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.history.clear")
					return nil
				}
				ctx.Session.ClearHistory()
				ctx.WE.Msgs.Info(ctx.Sender, "History cleared.")

			case "list":
				if !ctx.Sender.HasPermission("worldedit.history.list") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.history.list")
					return nil
				}
				if rdb != nil {
					uid := ctx.Session.PlayerUUID
					if len(args) > 0 {
						if resolved, ok := resolvePlayerUUID(ctx, args[0]); ok {
							uid = resolved
						}
					}
					edits, err := rdb.QueryByPlayer(uid, 20)
					if err == nil && len(edits) > 0 {
						for _, ed := range edits {
							ago := time.Since(ed.Time).Truncate(time.Second)
							ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("#%d (%s ago) %d blocks %s -> %s: /%s",
								ed.ID, ago, ed.Size, region.FormatPos(ed.Min), region.FormatPos(ed.Max), ed.Command))
						}
						return nil
					}
				}
				hist, idx := ctx.Session.HistoryList()
				if len(hist) == 0 {
					ctx.WE.Msgs.Info(ctx.Sender, "No history recorded.")
					return nil
				}
				for i, cs := range hist {
					marker := " "
					if i == idx-1 {
						marker = "*"
					}
					ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%s#%d %d blocks: %s", marker, cs.ID, cs.Size(), cs.Command))
				}

			case "find", "inspect", "search", "near":
				if !ctx.Sender.HasPermission("worldedit.history.find") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.history.find")
					return nil
				}
				if rdb == nil {
					return fmt.Errorf("rollback database is disabled")
				}
				radius := 10
				var filterUUID *uuid.UUID
				minTime := time.Time{}
				if u := ctx.FlagVal['u']; u != "" {
					if uid, ok := resolvePlayerUUID(ctx, u); ok {
						filterUUID = &uid
					}
				}
				if rv := ctx.FlagVal['r']; rv != "" {
					if n, err := strconv.Atoi(rv); err == nil && n >= 0 {
						radius = n
					}
				}
				if tv := ctx.FlagVal['t']; tv != "" {
					if d, err := parseFAWEDuration(tv); err == nil {
						minTime = time.Now().Add(-d)
					}
				}
				center := plugin.BlockPos{}
				if ctx.Player != nil {
					center = ctx.Player.Pos().Block()
				}
				lo := plugin.BlockPos{X: center.X - radius, Y: center.Y - radius, Z: center.Z - radius}
				hi := plugin.BlockPos{X: center.X + radius, Y: center.Y + radius, Z: center.Z + radius}
				edits, err := rdb.QueryEdits(filterUUID, minTime, lo, hi, false, 20)
				if err != nil {
					return err
				}
				if len(edits) == 0 {
					ctx.WE.Msgs.Info(ctx.Sender, "No matching edits found.")
					return nil
				}
				for _, ed := range edits {
					ago := time.Since(ed.Time).Truncate(time.Second)
					ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("#%d %s (%s ago) %d blocks: /%s",
						ed.ID, playerLabel(ctx, ed.Player), ago, ed.Size, ed.Command))
				}

			case "info", "summary", "summarize":
				if !ctx.Sender.HasPermission("worldedit.history.info") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.history.info")
					return nil
				}
				if rdb == nil || len(args) < 1 {
					return fmt.Errorf("usage: //history info <player> <id> or //history info <id>")
				}
				uid, editID, err := parsePlayerAndID(ctx, args)
				if err != nil {
					return err
				}
				ed, ok, err := rdb.GetEdit(uid, editID)
				if err != nil || !ok {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.worldedit.history.distr.summary_null")
					return nil
				}
				bdPath := storage.HistoryFilePath(ctx.WE.HistoryDir(), ctx.World.Key().String(), uid, editID)
				var diskSz int64
				if cs, err := storage.LoadChangeSetFromDisk(bdPath); err == nil {
					diskSz = cs.DiskSize()
				}
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Edit #%d by %s (%s): %d blocks | Bounds: %s -> %s | Disk: %d bytes",
					ed.ID, playerLabel(ctx, uid), ed.Command, ed.Size, region.FormatPos(ed.Min), region.FormatPos(ed.Max), diskSz))

			case "distr", "distribution":
				if !ctx.Sender.HasPermission("worldedit.history.distr") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.history.distr")
					return nil
				}
				uid, editID, err := parsePlayerAndID(ctx, args)
				if err != nil {
					return err
				}
				bdPath := storage.HistoryFilePath(ctx.WE.HistoryDir(), ctx.World.Key().String(), uid, editID)
				cs, err := storage.LoadChangeSetFromDisk(bdPath)
				if err != nil {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.worldedit.history.distr.summary_null")
					return nil
				}
				dist, err := cs.Distribution()
				if err != nil {
					return err
				}
				type kv struct {
					name string
					cnt  int64
				}
				var list []kv
				for st, cnt := range dist {
					list = append(list, kv{block.BlockName(st), cnt})
				}
				sort.Slice(list, func(i, j int) bool { return list[i].cnt > list[j].cnt })
				for i, item := range list {
					if i >= 15 {
						break
					}
					ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d %s", item.cnt, item.name))
				}

			case "rollback", "restore", "rerun":
				isRestore := sub == "restore" || sub == "rerun"
				perm := "worldedit.history.rollback"
				if isRestore {
					perm = "worldedit.history.restore"
				}
				if !ctx.Sender.HasPermission(perm) {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", perm)
					return nil
				}
				if rdb == nil {
					return fmt.Errorf("rollback database is disabled")
				}
				radius := 50
				var filterUUID *uuid.UUID
				minTime := time.Time{}
				if u := ctx.FlagVal['u']; u != "" {
					if uid, ok := resolvePlayerUUID(ctx, u); ok {
						filterUUID = &uid
					}
				}
				if rv := ctx.FlagVal['r']; rv != "" {
					if n, err := strconv.Atoi(rv); err == nil && n >= 0 {
						radius = n
					}
				}
				if tv := ctx.FlagVal['t']; tv != "" {
					if d, err := parseFAWEDuration(tv); err == nil {
						minTime = time.Now().Add(-d)
					}
				}
				// Also support positional `<player> <id>` rollback
				if filterUUID == nil && len(args) >= 2 {
					if uid, editID, err := parsePlayerAndID(ctx, args); err == nil {
						bdPath := storage.HistoryFilePath(ctx.WE.HistoryDir(), ctx.World.Key().String(), uid, editID)
						cs, err := storage.LoadChangeSetFromDisk(bdPath)
						if err != nil {
							return fmt.Errorf("could not load history file for edit #%d: %w", editID, err)
						}
						var n int64
						if isRestore {
							n, err = ctx.WE.RedoChangeSet(ctx.World, cs, nil)
						} else {
							n, err = ctx.WE.UndoChangeSet(ctx.World, cs, nil)
						}
						if err != nil {
							return err
						}
						ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%s edit #%d (%d blocks).", sub, editID, n))
						return nil
					}
				}
				center := plugin.BlockPos{}
				if ctx.Player != nil {
					center = ctx.Player.Pos().Block()
				}
				lo := plugin.BlockPos{X: center.X - radius, Y: center.Y - radius, Z: center.Z - radius}
				hi := plugin.BlockPos{X: center.X + radius, Y: center.Y + radius, Z: center.Z + radius}
				edits, err := rdb.QueryEdits(filterUUID, minTime, lo, hi, isRestore, 50)
				if err != nil {
					return err
				}
				if len(edits) == 0 {
					ctx.WE.Msgs.Info(ctx.Sender, "No matching edits found to "+sub+".")
					return nil
				}
				var totalBlocks int64
				applied := 0
				for _, ed := range edits {
					bdPath := storage.HistoryFilePath(ctx.WE.HistoryDir(), ctx.World.Key().String(), ed.Player, ed.ID)
					cs, err := storage.LoadChangeSetFromDisk(bdPath)
					if err != nil {
						continue
					}
					var n int64
					if isRestore {
						n, err = ctx.WE.RedoChangeSet(ctx.World, cs, nil)
					} else {
						n, err = ctx.WE.UndoChangeSet(ctx.World, cs, nil)
					}
					if err == nil {
						totalBlocks += n
						applied++
					}
				}
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Applied %s to %d edit(s) (%d blocks).", sub, applied, totalBlocks))

			case "import":
				if !ctx.Sender.HasPermission("fawe.rollback.import") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "fawe.rollback.import")
					return nil
				}
				if rdb == nil {
					return fmt.Errorf("rollback database is disabled")
				}
				n, err := rdb.ImportFromDisk()
				if err != nil {
					return err
				}
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Imported %d history file(s) into rollback database.", n))

			default:
				return fmt.Errorf("unknown history subcommand %q", sub)
			}
			return nil
		},
	})
}

func resolvePlayerUUID(ctx *CmdContext, name string) (uuid.UUID, bool) {
	if uid, err := uuid.Parse(name); err == nil {
		return uid, true
	}
	if p, ok := ctx.WE.Server.PlayerNamed(name); ok {
		return p.UUID(), true
	}
	if op, ok := ctx.WE.Server.OfflinePlayerNamed(name); ok {
		return op.UUID(), true
	}
	return uuid.Nil, false
}

func playerLabel(ctx *CmdContext, uid uuid.UUID) string {
	if uid == uuid.Nil {
		return "CONSOLE"
	}
	if p, ok := ctx.WE.Server.Player(uid); ok {
		return p.Name()
	}
	if op, ok := ctx.WE.Server.OfflinePlayer(uid); ok && op.Name() != "" {
		return op.Name()
	}
	return uid.String()[:8]
}

func parsePlayerAndID(ctx *CmdContext, args []string) (uuid.UUID, int, error) {
	if len(args) == 1 {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return uuid.Nil, 0, fmt.Errorf("invalid edit id %q", args[0])
		}
		return ctx.Session.PlayerUUID, id, nil
	}
	if len(args) >= 2 {
		uid, ok := resolvePlayerUUID(ctx, args[0])
		if !ok {
			return uuid.Nil, 0, fmt.Errorf("player %q not found", args[0])
		}
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return uuid.Nil, 0, fmt.Errorf("invalid edit id %q", args[1])
		}
		return uid, id, nil
	}
	return uuid.Nil, 0, fmt.Errorf("expected edit id")
}

func parseFAWEDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	var total time.Duration
	num := ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			num += string(c)
			continue
		}
		v, err := strconv.Atoi(num)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		num = ""
		switch c {
		case 's':
			total += time.Duration(v) * time.Second
		case 'm':
			total += time.Duration(v) * time.Minute
		case 'h':
			total += time.Duration(v) * time.Hour
		case 'd':
			total += time.Duration(v) * 24 * time.Hour
		case 'w':
			total += time.Duration(v) * 7 * 24 * time.Hour
		case 'y':
			total += time.Duration(v) * 365 * 24 * time.Hour
		default:
			return 0, fmt.Errorf("unknown time unit %q", c)
		}
	}
	if num != "" {
		v, _ := strconv.Atoi(num)
		total += time.Duration(v) * time.Second
	}
	return total, nil
}
