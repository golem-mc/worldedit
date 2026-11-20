package commands

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/core"
	"github.com/golem-mc/worldedit/tools"
)

func (r *Router) registerToolsAndBrushes() { //nolint:funlen // a command table reads better whole
	// --- Super-Pickaxe ---
	r.Add(CmdSpec{
		Name:        "/",
		Aliases:     []string{",", "/,"},
		Permission:  "worldedit.superpickaxe",
		Description: "Toggle the super pickaxe",
		Run: func(ctx *CmdContext) error {
			enabled, mode, rad := ctx.Session.SuperPickaxe()
			next := !enabled
			ctx.Session.SetSuperPickaxe(next, mode, rad)
			if next {
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Super pickaxe enabled (%s, radius %d).", mode, rad))
			} else {
				ctx.WE.Msgs.Info(ctx.Sender, "Super pickaxe disabled.")
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "superpickaxe",
		Aliases:     []string{"sp", "pickaxe", "/sp", "/pickaxe", "/superpickaxe"},
		Permission:  "worldedit.superpickaxe",
		Description: "Configure super-pickaxe mode (single, area, recursive)",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: /sp <single|area <radius>|recursive <radius>>")
			}
			maxSz := ctx.WE.Config().LimitFor(ctx.Sender).MaxSuperPickaxeSize
			switch strings.ToLower(ctx.Args[0]) {
			case "single":
				ctx.Session.SetSuperPickaxe(true, core.SuperPickSingle, 1)
				ctx.WE.Msgs.Info(ctx.Sender, "Mode changed. Left click with a pickaxe.")
			case "area":
				rad := 3
				if len(ctx.Args) >= 2 {
					if n, err := strconv.Atoi(ctx.Args[1]); err == nil && n > 0 {
						rad = n
					}
				}
				if rad > maxSz {
					return fmt.Errorf("maximum super-pickaxe size is %d", maxSz)
				}
				ctx.Session.SetSuperPickaxe(true, core.SuperPickArea, rad)
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Area super pickaxe enabled (radius %d).", rad))
			case "recursive", "recur":
				rad := 3
				if len(ctx.Args) >= 2 {
					if n, err := strconv.Atoi(ctx.Args[1]); err == nil && n > 0 {
						rad = n
					}
				}
				if rad > maxSz {
					return fmt.Errorf("maximum super-pickaxe size is %d", maxSz)
				}
				ctx.Session.SetSuperPickaxe(true, core.SuperPickRecursive, rad)
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Recursive super pickaxe enabled (radius %d).", rad))
			default:
				return fmt.Errorf("unknown super-pickaxe mode %q", ctx.Args[0])
			}
			return nil
		},
	})

	// --- Tool Binding Helpers ---
	heldItem := func(ctx *CmdContext) (registry.Item, error) {
		if !ctx.RequirePlayer() {
			return 0, fmt.Errorf("player required")
		}
		held := ctx.Player.Inventory().Get(plugin.SlotMainHand)
		if held.Empty() {
			return 0, fmt.Errorf("hold an item in your main hand first")
		}
		return held.Item, nil
	}

	bindTool := func(ctx *CmdContext, t core.Tool, msg string) error {
		it, err := heldItem(ctx)
		if err != nil {
			return err
		}
		ctx.Session.SetTool(it, t)
		ctx.WE.Msgs.Info(ctx.Sender, msg)
		return nil
	}

	// Standalone tool commands
	r.Add(CmdSpec{Name: "none", Aliases: []string{"unbind", "/none"}, Permission: "worldedit.tool.none", Description: "Unbind a tool/brush from your held item", Run: func(ctx *CmdContext) error {
		it, err := heldItem(ctx)
		if err != nil {
			return err
		}
		ctx.Session.SetTool(it, nil)
		ctx.WE.Msgs.Info(ctx.Sender, "Tool unbound from your current item.")
		return nil
	}})

	r.Add(CmdSpec{Name: "selwand", Aliases: []string{"/selwand"}, Permission: "worldedit.setwand", Description: "Bind selection wand tool to held item", Run: func(ctx *CmdContext) error {
		return bindTool(ctx, tools.SelectionWandTool{}, "Selection wand bound to your current item.")
	}})
	r.Add(CmdSpec{Name: "navwand", Aliases: []string{"/navwand"}, Permission: "worldedit.setwand", Description: "Bind navigation wand tool to held item", Run: func(ctx *CmdContext) error {
		return bindTool(ctx, tools.NavigationWandTool{}, "Navigation wand bound to your current item.")
	}})
	r.Add(CmdSpec{Name: "info", Aliases: []string{"/info"}, Permission: "worldedit.tool.info", Description: "Block information tool", Run: func(ctx *CmdContext) error {
		return bindTool(ctx, tools.InfoTool{}, "Info tool bound to your current item.")
	}})
	r.Add(CmdSpec{Name: "inspect", Aliases: []string{"/inspect"}, Permission: "worldedit.tool.inspect", Description: "Inspect edit history tool", Run: func(ctx *CmdContext) error {
		return bindTool(ctx, tools.InspectTool{}, "Inspect tool bound to your current item.")
	}})
	r.Add(CmdSpec{Name: "tree", Aliases: []string{"/tree"}, Permission: "worldedit.tool.tree", Description: "Tree planter tool", Run: func(ctx *CmdContext) error {
		tname := "oak"
		if len(ctx.Args) > 0 {
			tname = ctx.Args[0]
		}
		tt, ok := tools.ParseTreeType(tname)
		if !ok {
			return fmt.Errorf("unknown tree type %q", tname)
		}
		return bindTool(ctx, &tools.TreeTool{TreeType: tt, TypeName: tname}, "Tree tool ("+tname+") bound to your current item.")
	}})
	r.Add(CmdSpec{Name: "repl", Aliases: []string{"/repl"}, Permission: "worldedit.tool.replacer", Description: "Block replacer tool", Run: func(ctx *CmdContext) error {
		if len(ctx.Args) < 1 {
			return fmt.Errorf("usage: /repl <pattern>")
		}
		pat, err := block.ParsePattern(strings.Join(ctx.Args, " "), ctx.ParseContext())
		if err != nil {
			return err
		}
		return bindTool(ctx, &tools.ReplacerTool{Pattern: pat}, "Block replacer bound to your current item.")
	}})
	r.Add(CmdSpec{Name: "cycler", Aliases: []string{"/cycler"}, Permission: "worldedit.tool.data-cycler", Description: "Block data cycler tool", Run: func(ctx *CmdContext) error {
		return bindTool(ctx, &tools.DataCyclerTool{}, "Block data cycler bound to your current item.")
	}})
	r.Add(CmdSpec{Name: "floodfill", Aliases: []string{"flood", "/flood", "/floodfill"}, Permission: "worldedit.tool.flood-fill", Description: "Flood fill tool", Run: func(ctx *CmdContext) error {
		if len(ctx.Args) < 2 {
			return fmt.Errorf("usage: /floodfill <pattern> <range>")
		}
		pat, err := block.ParsePattern(ctx.Args[0], ctx.ParseContext())
		if err != nil {
			return err
		}
		rng, err := strconv.Atoi(ctx.Args[1])
		if err != nil || rng < 1 {
			return fmt.Errorf("invalid range %q", ctx.Args[1])
		}
		return bindTool(ctx, &tools.FloodFillTool{Pattern: pat, Range: rng}, fmt.Sprintf("Flood-fill tool (range %d) bound.", rng))
	}})
	r.Add(CmdSpec{Name: "deltree", Aliases: []string{"/deltree"}, Permission: "worldedit.tool.deltree", Description: "Floating tree remover tool", Run: func(ctx *CmdContext) error {
		return bindTool(ctx, tools.DelTreeTool{}, "Floating tree remover tool bound.")
	}})
	r.Add(CmdSpec{Name: "farwand", Aliases: []string{"/farwand"}, Permission: "worldedit.tool.farwand", Description: "Selection wand at a distance tool", Run: func(ctx *CmdContext) error {
		return bindTool(ctx, &tools.FarWandTool{Range: 120}, "Far wand tool bound.")
	}})
	r.Add(CmdSpec{Name: "lrbuild", Aliases: []string{"/lrbuild"}, Permission: "worldedit.tool.lrbuild", Description: "Long-range building tool", Run: func(ctx *CmdContext) error {
		if len(ctx.Args) < 2 {
			return fmt.Errorf("usage: /lrbuild <primary-pattern> <secondary-pattern>")
		}
		pctx := ctx.ParseContext()
		p1, err := block.ParsePattern(ctx.Args[0], pctx)
		if err != nil {
			return err
		}
		p2, err := block.ParsePattern(ctx.Args[1], pctx)
		if err != nil {
			return err
		}
		return bindTool(ctx, &tools.LongRangeBuildTool{PrimaryPat: p1, SecondaryPat: p2, Range: 120},
			ctx.WE.Msgs.Raw("fawe.worldedit.tool.tool.lrbuild.info", p1.String(), p2.String()))
	}})
	r.Add(CmdSpec{Name: "stacker", Permission: "worldedit.tool.stack", Description: "Block stacker tool", Run: func(ctx *CmdContext) error {
		cnt := 1
		if len(ctx.Args) >= 1 {
			if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
				cnt = n
			}
		}
		return bindTool(ctx, &tools.StackerTool{Count: cnt}, fmt.Sprintf("Stacker tool (count %d) bound.", cnt))
	}})

	// --- Brush Option Utility Commands (/mask, /smask, /material, /size, /range, /transform, /target, /scroll, etc.) ---
	getHeldBrush := func(ctx *CmdContext) (*tools.BrushTool, error) {
		it, err := heldItem(ctx)
		if err != nil {
			return nil, err
		}
		t, ok := ctx.Session.Tool(it)
		if !ok {
			ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.worldedit.brush.brush.none")
			return nil, fmt.Errorf("no brush held")
		}
		bt, ok := t.(*tools.BrushTool)
		if !ok {
			ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.worldedit.brush.brush.none")
			return nil, fmt.Errorf("held tool is not a brush")
		}
		return bt, nil
	}

	r.Add(CmdSpec{Name: "mask", Aliases: []string{"/mask"}, Permission: "worldedit.brush.options.mask", Description: "Set the brush destination mask", Run: func(ctx *CmdContext) error {
		bt, err := getHeldBrush(ctx)
		if err != nil {
			return nil
		}
		if len(ctx.Args) == 0 {
			bt.Mask = nil
			ctx.WE.Msgs.Info(ctx.Sender, "Brush mask disabled.")
			return nil
		}
		m, err := block.ParseMask(strings.Join(ctx.Args, " "), ctx.ParseContext())
		if err != nil {
			return err
		}
		bt.Mask = m
		ctx.WE.Msgs.Info(ctx.Sender, "Brush mask set to: "+m.String())
		return nil
	}})

	r.Add(CmdSpec{Name: "smask", Aliases: []string{"/smask", "sourcemask", "/sourcemask"}, Permission: "worldedit.brush.options.smask", Description: "Set the brush source mask", Run: func(ctx *CmdContext) error {
		bt, err := getHeldBrush(ctx)
		if err != nil {
			return nil
		}
		if len(ctx.Args) == 0 {
			bt.SourceMask = nil
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.brush.brush.source.mask.disabled")
			return nil
		}
		m, err := block.ParseMask(strings.Join(ctx.Args, " "), ctx.ParseContext())
		if err != nil {
			return err
		}
		bt.SourceMask = m
		ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.brush.brush.source.mask")
		return nil
	}})

	r.Add(CmdSpec{Name: "material", Aliases: []string{"mat", "/material", "/mat"}, Permission: "worldedit.brush.options.material", Description: "Set the brush pattern/material", Run: func(ctx *CmdContext) error {
		bt, err := getHeldBrush(ctx)
		if err != nil {
			return nil
		}
		if len(ctx.Args) == 0 {
			return fmt.Errorf("usage: /material <pattern>")
		}
		raw := strings.Join(ctx.Args, " ")
		pat, err := block.ParsePattern(raw, ctx.ParseContext())
		if err != nil {
			return err
		}
		bt.Active().Pattern = pat
		bt.Active().PatternStr = raw
		ctx.WE.Msgs.Info(ctx.Sender, "Brush material set to: "+pat.String())
		return nil
	}})

	r.Add(CmdSpec{Name: "size", Permission: "worldedit.brush.options.size", Description: "Set the brush size", Run: func(ctx *CmdContext) error {
		bt, err := getHeldBrush(ctx)
		if err != nil {
			return nil
		}
		if len(ctx.Args) == 0 {
			return fmt.Errorf("usage: /size <radius>")
		}
		sz, err := strconv.Atoi(ctx.Args[0])
		if err != nil || sz < 1 {
			return fmt.Errorf("invalid brush size %q", ctx.Args[0])
		}
		bt.Active().Size = sz
		ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Brush size set to %d.", sz))
		return nil
	}})

	r.Add(CmdSpec{Name: "range", Aliases: []string{"/range"}, Permission: "worldedit.brush.options.range", Description: "Set the brush range", Run: func(ctx *CmdContext) error {
		bt, err := getHeldBrush(ctx)
		if err != nil {
			return nil
		}
		if len(ctx.Args) == 0 {
			return fmt.Errorf("usage: /range <distance>")
		}
		rng, err := strconv.Atoi(ctx.Args[0])
		if err != nil || rng < 1 {
			return fmt.Errorf("invalid range %q", ctx.Args[0])
		}
		bt.Range = rng
		ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Brush range set to %d.", rng))
		return nil
	}})

	r.Add(CmdSpec{Name: "tracemask", Aliases: []string{"tarmask", "tm", "targetmask"}, Permission: "worldedit.brush.options.tracemask", Description: "Set the brush trace mask", Run: func(ctx *CmdContext) error {
		bt, err := getHeldBrush(ctx)
		if err != nil {
			return nil
		}
		if len(ctx.Args) == 0 {
			bt.TraceMask = nil
			ctx.WE.Msgs.Info(ctx.Sender, "Brush trace mask cleared.")
			return nil
		}
		m, err := block.ParseMask(strings.Join(ctx.Args, " "), ctx.ParseContext())
		if err != nil {
			return err
		}
		bt.TraceMask = m
		ctx.WE.Msgs.Info(ctx.Sender, "Brush trace mask set.")
		return nil
	}})

	r.Add(CmdSpec{Name: "transform", Aliases: []string{"/transform"}, Permission: "worldedit.brush.options.transform", Description: "Set the brush transform", Run: func(ctx *CmdContext) error {
		bt, err := getHeldBrush(ctx)
		if err != nil {
			return nil
		}
		if len(ctx.Args) == 0 {
			bt.Transform = nil
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.brush.brush.transform.disabled")
			return nil
		}
		tr, err := block.ParseTransform(strings.Join(ctx.Args, " "), ctx.ParseContext())
		if err != nil {
			return err
		}
		bt.Transform = tr
		ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.brush.brush.transform")
		return nil
	}})

	r.Add(CmdSpec{Name: "target", Aliases: []string{"tar", "/target", "/tar"}, Permission: "worldedit.brush.target", Description: "Set brush target mode", Run: func(ctx *CmdContext) error {
		bt, err := getHeldBrush(ctx)
		if err != nil {
			return nil
		}
		modes := []string{"TARGET_BLOCK_RANGE", "FORWARD_POINT_PITCH", "TARGET_POINT_HEIGHT", "TARGET_FACE_RANGE"}
		mode := modes[0]
		if len(ctx.Args) > 0 {
			if idx, err := strconv.Atoi(ctx.Args[0]); err == nil && idx >= 0 && idx < len(modes) {
				mode = modes[idx]
			} else {
				mode = strings.ToUpper(ctx.Args[0])
			}
		}
		bt.TargetMode = mode
		ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.brush.brush.target.mode.set", mode)
		return nil
	}})

	r.Add(CmdSpec{Name: "targetoffset", Aliases: []string{"to"}, Permission: "worldedit.brush.targetoffset", Description: "Set brush vertical target offset", Run: func(ctx *CmdContext) error {
		bt, err := getHeldBrush(ctx)
		if err != nil {
			return nil
		}
		off := 0
		if len(ctx.Args) > 0 {
			off, _ = strconv.Atoi(ctx.Args[0])
		}
		bt.TargetOffset = off
		ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.brush.brush.target.offset.set", off)
		return nil
	}})

	r.Add(CmdSpec{Name: "scroll", Permission: "worldedit.brush.scroll", Description: "Set brush scroll wheel action", Run: func(ctx *CmdContext) error {
		bt, err := getHeldBrush(ctx)
		if err != nil {
			return nil
		}
		if len(ctx.Args) == 0 || strings.EqualFold(ctx.Args[0], "none") {
			bt.ScrollMode = ""
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.brush.brush.scroll.action.unset")
			return nil
		}
		bt.ScrollMode = strings.ToLower(ctx.Args[0])
		ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.brush.brush.scroll.action.set", bt.ScrollMode)
		return nil
	}})

	bindSideBrush := func(secondary bool) func(*CmdContext) error {
		return func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: /%s <brush-type> [args...]", ctx.Label)
			}
			it, err := heldItem(ctx)
			if err != nil {
				return err
			}
			act, err := buildBrushSettings(ctx, strings.ToLower(ctx.Args[0]), ctx.Args[1:])
			if err != nil {
				return err
			}
			var bt *tools.BrushTool
			if cur, ok := ctx.Session.Tool(it); ok {
				if existing, ok := cur.(*tools.BrushTool); ok {
					bt = existing
				}
			}
			if bt == nil {
				bt = tools.NewBrushTool(nil)
			}
			if secondary {
				bt.SecondaryAction = act
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Equipped left-click brush %s (size %d).", act.Kind, act.Size))
			} else {
				bt.PrimaryAction = act
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Equipped right-click brush %s (size %d).", act.Kind, act.Size))
			}
			ctx.Session.SetTool(it, bt)
			return nil
		}
	}

	r.Add(CmdSpec{Name: "primary", Aliases: []string{"/primary"}, Permission: "worldedit.brush.primary", Description: "Set the right-click brush action", Run: bindSideBrush(false)})
	r.Add(CmdSpec{Name: "secondary", Aliases: []string{"/secondary"}, Permission: "worldedit.brush.secondary", Description: "Set the left-click brush action", Run: bindSideBrush(true)})

	r.Add(CmdSpec{Name: "/listbrush", Permission: "worldedit.brush.list", Description: "List saved brushes", Run: func(ctx *CmdContext) error {
		entries, _ := os.ReadDir(ctx.WE.BrushesDir())
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				names = append(names, strings.TrimSuffix(e.Name(), ".json"))
			}
		}
		if len(names) == 0 {
			ctx.WE.Msgs.Info(ctx.Sender, "No saved brushes found.")
			return nil
		}
		ctx.WE.Msgs.Info(ctx.Sender, "Saved brushes: "+strings.Join(names, ", "))
		return nil
	}})

	// --- Main /brush (/br, /tool) command dispatching all FAWE & WorldEdit brushes and tools ---
	r.Add(CmdSpec{
		Name:        "brush",
		Aliases:     []string{"br", "/brush", "/br", "tool", "/tool"},
		Description: "Equip a WorldEdit/FAWE brush or tool on your held item",
		Usage:       "/br <sphere|cyl|smooth|erode|blendball|height|flatten|cliff|clipboard|line|spline|rock|scatter|splatter|layer|gravity|ex|butcher|copypaste|command|deform|save|load|list|none|...> [args]",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: /br <type> [args...]")
			}
			sub := strings.ToLower(ctx.Args[0])
			args := ctx.Args[1:]

			// Delegate tool/utility subcommands if called under /tool or /br
			if sp, ok := r.byName[sub]; ok && sub != "brush" && sub != "br" && sub != "tool" {
				return r.Execute(ctx.Sender, sub, args, sub+" "+strings.Join(args, " "), sp)
			}

			switch sub {
			case "savebrush", "save":
				bt, err := getHeldBrush(ctx)
				if err != nil {
					return nil
				}
				if len(args) == 0 {
					return fmt.Errorf("usage: /br save <name>")
				}
				path, err := tools.SaveBrushFile(ctx.WE.BrushesDir(), args[0], bt)
				if err != nil {
					return err
				}
				ctx.WE.Msgs.Info(ctx.Sender, "Saved brush to "+path)
				return nil

			case "loadbrush", "load":
				it, err := heldItem(ctx)
				if err != nil {
					return err
				}
				if len(args) == 0 {
					return fmt.Errorf("usage: /br load <name>")
				}
				bt, err := tools.LoadBrushFile(ctx.WE.BrushesDir(), args[0], ctx.ParseContext())
				if err != nil {
					return err
				}
				ctx.Session.SetTool(it, bt)
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.brush.brush.equipped", bt.Name())
				return nil

			case "listbrush", "/listbrush", "list":
				entries, _ := os.ReadDir(ctx.WE.BrushesDir())
				var names []string
				for _, e := range entries {
					if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
						names = append(names, strings.TrimSuffix(e.Name(), ".json"))
					}
				}
				if len(names) == 0 {
					ctx.WE.Msgs.Info(ctx.Sender, "No saved brushes found.")
					return nil
				}
				ctx.WE.Msgs.Info(ctx.Sender, "Saved brushes: "+strings.Join(names, ", "))
				return nil
			}

			it, err := heldItem(ctx)
			if err != nil {
				return err
			}
			act, err := buildBrushSettings(ctx, sub, args)
			if err != nil {
				return err
			}
			bt := tools.NewBrushTool(act)
			ctx.Session.SetTool(it, bt)
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.brush.brush.equipped", fmt.Sprintf("%s (size %d)", act.Kind, act.Size))
			return nil
		},
	})
}

func buildBrushSettings(ctx *CmdContext, sub string, args []string) (*tools.BrushSettings, error) {
	pctx := ctx.ParseContext()
	parsePat := func(idx int, def string) (block.Pattern, string, error) {
		raw := def
		if idx < len(args) {
			raw = args[idx]
		}
		p, err := block.ParsePattern(raw, pctx)
		return p, raw, err
	}
	parseInt := func(idx, def int) int {
		if idx < len(args) {
			if n, err := strconv.Atoi(args[idx]); err == nil && n > 0 {
				return n
			}
		}
		return def
	}
	parseFloat := func(idx int, def float64) float64 {
		if idx < len(args) {
			if f, err := strconv.ParseFloat(args[idx], 64); err == nil {
				return f
			}
		}
		return def
	}

	switch sub {
	case "sphere", "s", "set":
		pat, raw, err := parsePat(0, "stone")
		if err != nil {
			return nil, err
		}
		return &tools.BrushSettings{Kind: tools.BrushSphere, Pattern: pat, PatternStr: raw, Size: parseInt(1, 5), Hollow: ctx.Flags['h']}, nil

	case "cylinder", "cyl", "c":
		pat, raw, err := parsePat(0, "stone")
		if err != nil {
			return nil, err
		}
		return &tools.BrushSettings{Kind: tools.BrushCylinder, Pattern: pat, PatternStr: raw, Size: parseInt(1, 5), Height: parseInt(2, 1), Hollow: ctx.Flags['h']}, nil

	case "circle":
		pat, raw, err := parsePat(0, "stone")
		if err != nil {
			return nil, err
		}
		return &tools.BrushSettings{Kind: tools.BrushCircle, Pattern: pat, PatternStr: raw, Size: parseInt(1, 5)}, nil

	case "clipboard":
		return &tools.BrushSettings{Kind: tools.BrushClipboard, Size: 5, ClipboardAir: ctx.Flags['a']}, nil

	case "smooth":
		return &tools.BrushSettings{Kind: tools.BrushSmooth, Size: parseInt(0, 5), Iterations: parseInt(1, 2)}, nil

	case "snowsmooth":
		return &tools.BrushSettings{Kind: tools.BrushSnowSmooth, Size: parseInt(0, 5), Iterations: parseInt(1, 1)}, nil

	case "extinguish", "ex":
		return &tools.BrushSettings{Kind: tools.BrushExtinguish, Size: parseInt(0, 5)}, nil

	case "gravity", "grav":
		return &tools.BrushSettings{Kind: tools.BrushGravity, Size: parseInt(0, 5)}, nil

	case "butcher", "kill":
		return &tools.BrushSettings{Kind: tools.BrushButcher, Size: parseInt(0, 5)}, nil

	case "biome":
		return &tools.BrushSettings{Kind: tools.BrushBiome, Size: parseInt(1, 5)}, nil

	case "blendball", "bb", "blend", "morph", "dilate":
		return &tools.BrushSettings{Kind: tools.BrushBlendBall, Size: parseInt(0, 5)}, nil

	case "erode":
		return &tools.BrushSettings{
			Kind:       tools.BrushErode,
			Size:       parseInt(0, 5),
			ErodeFaces: parseInt(1, 2),
			ErodeIter:  parseInt(2, 1),
			FillFaces:  parseInt(3, 5),
			FillIter:   parseInt(4, 1),
		}, nil

	case "pull":
		return &tools.BrushSettings{Kind: tools.BrushPull, Size: parseInt(0, 5), YScale: parseFloat(1, 0.5)}, nil

	case "recursive", "recurse", "r":
		pat, raw, err := parsePat(0, "stone")
		if err != nil {
			return nil, err
		}
		return &tools.BrushSettings{Kind: tools.BrushRecursive, Pattern: pat, PatternStr: raw, Size: parseInt(1, 5)}, nil

	case "line", "l":
		pat, raw, err := parsePat(0, "stone")
		if err != nil {
			return nil, err
		}
		return &tools.BrushSettings{Kind: tools.BrushLine, Pattern: pat, PatternStr: raw, Size: parseInt(1, 1)}, nil

	case "spline", "spl", "curve", "surfacespline", "sspline", "sspl":
		pat, raw, err := parsePat(0, "stone")
		if err != nil {
			return nil, err
		}
		return &tools.BrushSettings{Kind: tools.BrushSpline, Pattern: pat, PatternStr: raw, Size: parseInt(1, 1)}, nil

	case "catenary", "cat", "gravityline", "saggedline":
		pat, raw, err := parsePat(0, "stone")
		if err != nil {
			return nil, err
		}
		return &tools.BrushSettings{Kind: tools.BrushCatenary, Pattern: pat, PatternStr: raw, Size: 1, YScale: parseFloat(1, 1.2)}, nil

	case "sweep", "sw":
		return &tools.BrushSettings{Kind: tools.BrushSweep, Size: 5}, nil

	case "rock", "blob":
		pat, raw, err := parsePat(0, "stone")
		if err != nil {
			return nil, err
		}
		return &tools.BrushSettings{Kind: tools.BrushRock, Pattern: pat, PatternStr: raw, Size: parseInt(1, 5)}, nil

	case "shatter", "partition", "split":
		pat, raw, err := parsePat(0, "air")
		if err != nil {
			return nil, err
		}
		return &tools.BrushSettings{Kind: tools.BrushShatter, Pattern: pat, PatternStr: raw, Size: parseInt(1, 10), Iterations: parseInt(2, 6)}, nil

	case "layer":
		sz := parseInt(0, 5)
		var layers []block.Pattern
		if len(args) >= 2 {
			for part := range strings.SplitSeq(args[1], ",") {
				if p, err := block.ParsePattern(part, pctx); err == nil {
					layers = append(layers, p)
				}
			}
		}
		return &tools.BrushSettings{Kind: tools.BrushLayer, Size: sz, Layers: layers}, nil

	case "scatter", "splatter", "surface", "surf", "stencil", "image":
		pat, raw, err := parsePat(0, "stone")
		if err != nil {
			return nil, err
		}
		k := tools.BrushScatter
		if sub == "splatter" {
			k = tools.BrushSplatter
		}
		return &tools.BrushSettings{Kind: k, Pattern: pat, PatternStr: raw, Size: parseInt(1, 5), Density: 0.25}, nil

	case "populateschematic", "populateschem", "popschem", "pschem", "ps":
		return &tools.BrushSettings{Kind: tools.BrushPopulateSchematic, Size: parseInt(0, 15), Density: 0.08}, nil

	case "height", "heightmap":
		return &tools.BrushSettings{Kind: tools.BrushHeight, Size: parseInt(0, 5), YScale: parseFloat(1, 0.5)}, nil

	case "cliff", "flatcylinder":
		return &tools.BrushSettings{Kind: tools.BrushCliff, Size: parseInt(0, 5), YScale: parseFloat(1, 1.0)}, nil

	case "flatten", "flatmap", "flat":
		return &tools.BrushSettings{Kind: tools.BrushFlatten, Size: parseInt(0, 5)}, nil

	case "copypaste", "cp", "copypasta":
		return &tools.BrushSettings{Kind: tools.BrushCopyPasta, Size: parseInt(0, 5)}, nil

	case "command", "cmd", "scattercommand", "scattercmd", "scmd":
		sz := parseInt(0, 5)
		rawCmds := ""
		if len(args) >= 2 {
			rawCmds = strings.Join(args[1:], " ")
		}
		return &tools.BrushSettings{Kind: tools.BrushCommand, Size: sz, Commands: strings.Split(rawCmds, ";")}, nil

	case "deform":
		sz := parseInt(0, 5)
		ex := "y-=0.1"
		if len(args) >= 2 {
			ex = strings.Join(args[1:], " ")
		}
		return &tools.BrushSettings{Kind: tools.BrushDeform, Size: sz, Expression: ex}, nil

	case "forest":
		tt := plugin.TreeOak
		if len(args) >= 2 {
			if parsed, ok := tools.ParseTreeType(args[1]); ok {
				tt = parsed
			}
		}
		return &tools.BrushSettings{Kind: tools.BrushForest, Size: parseInt(0, 5), TreeType: tt, Density: 0.08}, nil

	case "raise":
		return &tools.BrushSettings{Kind: tools.BrushRaise, Size: parseInt(0, 5)}, nil

	case "lower":
		return &tools.BrushSettings{Kind: tools.BrushLower, Size: parseInt(0, 5)}, nil

	case "apply", "paint":
		if len(args) < 2 {
			return nil, fmt.Errorf("usage: /br %s <sphere|cyl> <radius> <set|forest> [pattern]", sub)
		}
		kind := tools.BrushSphere
		if strings.HasPrefix(strings.ToLower(args[0]), "cyl") {
			kind = tools.BrushCylinder
		}
		sz := parseInt(1, 5)
		patIdx := 3
		if len(args) >= 3 && strings.EqualFold(args[2], "forest") {
			return &tools.BrushSettings{Kind: tools.BrushForest, Size: sz, TreeType: plugin.TreeOak, Density: 0.1}, nil
		}
		pat, raw, err := parsePat(patIdx, "stone")
		if err != nil {
			return nil, err
		}
		return &tools.BrushSettings{Kind: kind, Pattern: pat, PatternStr: raw, Size: sz, Height: 1}, nil

	case "snow":
		return &tools.BrushSettings{Kind: tools.BrushSnow, Size: parseInt(0, 5)}, nil

	default:
		return nil, fmt.Errorf("unknown brush type %q", sub)
	}
}
