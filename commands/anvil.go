package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/storage"
)

func (r *Router) registerAnvil() {
	r.Add(CmdSpec{
		Name:        "anvil",
		Aliases:     []string{"/anvil"},
		Description: "FAWE Anvil region-level commands for inspecting and bulk-editing selections or offline region files",
		Usage:       "/anvil <count|distr|set|replace|clear|deletealloldregions|countall>",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: /anvil <count|distr|set|replace|clear|deletealloldregions|countall>")
			}
			sub := strings.ToLower(ctx.Args[0])
			args := ctx.Args[1:]

			switch sub {
			case "count":
				if !ctx.Sender.HasPermission("worldedit.anvil.count") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.anvil.count")
					return nil
				}
				if sp, ok := r.byName["/count"]; ok {
					return r.Execute(ctx.Sender, "/count", args, "//count "+strings.Join(args, " "), sp)
				}

			case "distr":
				if !ctx.Sender.HasPermission("worldedit.anvil.distr") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.anvil.distr")
					return nil
				}
				if sp, ok := r.byName["/distr"]; ok {
					return r.Execute(ctx.Sender, "/distr", args, "//distr "+strings.Join(args, " "), sp)
				}

			case "set":
				if !ctx.Sender.HasPermission("worldedit.anvil.set") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.anvil.set")
					return nil
				}
				if sp, ok := r.byName["/set"]; ok {
					return r.Execute(ctx.Sender, "/set", args, "//set "+strings.Join(args, " "), sp)
				}

			case "replace", "r", "replacepattern", "rp", "replaceall", "rea":
				if !ctx.Sender.HasPermission("worldedit.anvil.replace") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.anvil.replace")
					return nil
				}
				if sp, ok := r.byName["/replace"]; ok {
					return r.Execute(ctx.Sender, "/replace", args, "//replace "+strings.Join(args, " "), sp)
				}

			case "clear", "unset":
				if !ctx.Sender.HasPermission("worldedit.anvil.clear") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.anvil.clear")
					return nil
				}
				reg, err := ctx.Session.Selection(ctx.World)
				if err != nil {
					return fmt.Errorf("make a region selection first")
				}
				air, _ := block.ParseState("air", false)
				es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
				n := es.SetRegion(reg, block.SinglePattern(air))
				if err := es.Commit(); err != nil {
					return err
				}
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", n)

			case "countall":
				if !ctx.Sender.HasPermission("worldedit.anvil.countall") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.anvil.countall")
					return nil
				}
				if len(args) < 2 {
					return fmt.Errorf("usage: /anvil countall <world-region-dir> <block>")
				}
				st, err := block.ParseState(args[1], true)
				if err != nil {
					return err
				}
				total, files, err := countBlockInRegionDir(args[0], st)
				if err != nil {
					return err
				}
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Counted %d %s across %d region file(s).", total, block.BlockName(st), files))

			case "deletealloldregions", "deloldreg":
				if !ctx.Sender.HasPermission("worldedit.anvil.deletealloldregions") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.anvil.deletealloldregions")
					return nil
				}
				if len(args) < 2 {
					return fmt.Errorf("usage: /anvil deletealloldregions <region-dir> <age-days>")
				}
				days, err := strconv.Atoi(args[1])
				if err != nil || days < 1 {
					return fmt.Errorf("invalid days %q", args[1])
				}
				cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
				entries, err := os.ReadDir(args[0])
				if err != nil {
					return err
				}
				removed := 0
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".mca") {
						continue
					}
					if fi, err := e.Info(); err == nil && fi.ModTime().Before(cutoff) {
						if os.Remove(filepath.Join(args[0], e.Name())) == nil {
							removed++
						}
					}
				}
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Deleted %d region file(s) older than %d days.", removed, days))

			default:
				return fmt.Errorf("unknown anvil subcommand %q", sub)
			}
			return nil
		},
	})
}

func countBlockInRegionDir(dir string, target plugin.BlockState) (int64, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	var mcaFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".mca") {
			mcaFiles = append(mcaFiles, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(mcaFiles)
	var total int64
	wantBlock := target.Block()
	for _, f := range mcaFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for cz := 0; cz < 32; cz++ {
			for cx := 0; cx < 32; cx++ {
				ch, err := storage.ReadAnvilChunk(data, cx, cz)
				if err != nil {
					continue
				}
				for _, sec := range ch.Sections {
					for _, st := range sec {
						if st.Block() == wantBlock {
							total++
						}
					}
				}
			}
		}
	}
	return total, len(mcaFiles), nil
}
