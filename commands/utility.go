package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/core"
	"github.com/golem-mc/worldedit/expr"
	"github.com/golem-mc/worldedit/tools"
)

func (r *Router) registerUtility() {
	fillCmd := func(recursive bool) func(*CmdContext) error {
		return func(ctx *CmdContext) error {
			if len(ctx.Args) < 2 {
				return fmt.Errorf("usage: //fill <pattern> <radius> [depth]")
			}
			pat, err := block.ParsePattern(ctx.Args[0], ctx.ParseContext())
			if err != nil {
				return err
			}
			radius, err := strconv.Atoi(ctx.Args[1])
			if err != nil || radius < 1 {
				return fmt.Errorf("invalid radius %q", ctx.Args[1])
			}
			depth := radius
			if len(ctx.Args) >= 3 {
				if d, err := strconv.Atoi(ctx.Args[2]); err == nil && d > 0 {
					depth = d
				}
			}
			origin := placementOrigin(ctx)
			minY := origin.Y - depth
			maxY := origin.Y
			pts := core.FloodFill(ctx.World, origin, func(p plugin.BlockPos, st plugin.BlockState) bool {
				if p.Y < minY || p.Y > maxY {
					return false
				}
				return block.IsAir(st)
			}, radius, 65536)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for _, p := range pts {
				es.SetPattern(p, pat)
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d block(s) have been created.", es.Changed()))
			return nil
		}
	}

	r.Add(CmdSpec{Name: "/fill", Permission: "worldedit.fill", Description: "Fill a hole", Run: fillCmd(false)})
	r.Add(CmdSpec{Name: "/fillr", Permission: "worldedit.fill.recursive", Description: "Fill a hole recursively", Run: fillCmd(true)})

	r.Add(CmdSpec{
		Name:        "/drain",
		Permission:  "worldedit.drain",
		Description: "Drain a pool of water or lava",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: //drain <radius>")
			}
			radius, err := strconv.Atoi(ctx.Args[0])
			if err != nil || radius < 1 {
				return fmt.Errorf("invalid radius %q", ctx.Args[0])
			}
			origin := placementOrigin(ctx)
			// Find a nearby liquid seed block if player isn't standing directly inside liquid
			seed := origin
			if !block.IsLiquid(ctx.World.BlockState(seed)) {
				found := false
				for dy := -2; dy <= 1 && !found; dy++ {
					for dx := -2; dx <= 2 && !found; dx++ {
						for dz := -2; dz <= 2 && !found; dz++ {
							p := plugin.BlockPos{X: origin.X + dx, Y: origin.Y + dy, Z: origin.Z + dz}
							if block.IsLiquid(ctx.World.BlockState(p)) {
								seed = p
								found = true
							}
						}
					}
				}
			}
			pts := core.FloodFill(ctx.World, seed, func(_ plugin.BlockPos, st plugin.BlockState) bool {
				return block.IsLiquid(st)
			}, radius, 65536)
			air, _ := block.ParseState("air", false)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for _, p := range pts {
				es.SetBlock(p, air)
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d block(s) have been drained.", es.Changed()))
			return nil
		},
	})

	fixLiquid := func(liquidName string) func(*CmdContext) error {
		return func(ctx *CmdContext) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: /fix%s <radius>", liquidName)
			}
			radius, err := strconv.Atoi(ctx.Args[0])
			if err != nil || radius < 1 {
				return fmt.Errorf("invalid radius %q", ctx.Args[0])
			}
			srcState, _ := block.ParseState("minecraft:"+liquidName+"[level=0]", false)
			origin := placementOrigin(ctx)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for dy := -radius; dy <= radius; dy++ {
				for dz := -radius; dz <= radius; dz++ {
					for dx := -radius; dx <= radius; dx++ {
						if dx*dx+dz*dz > radius*radius {
							continue
						}
						p := plugin.BlockPos{X: origin.X + dx, Y: origin.Y + dy, Z: origin.Z + dz}
						if block.BlockName(ctx.World.BlockState(p)) == "minecraft:"+liquidName {
							es.SetBlock(p, srcState)
						}
					}
				}
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d block(s) fixed.", es.Changed()))
			return nil
		}
	}

	r.Add(CmdSpec{Name: "fixwater", Aliases: []string{"/fixwater"}, Permission: "worldedit.fixwater", Description: "Fix water to be stationary", Run: fixLiquid("water")})
	r.Add(CmdSpec{Name: "fixlava", Aliases: []string{"/fixlava"}, Permission: "worldedit.fixlava", Description: "Fix lava to be stationary", Run: fixLiquid("lava")})

	removeVert := func(up bool) func(*CmdContext) error {
		return func(ctx *CmdContext) error {
			size := 1
			height := 256
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					size = n
				}
			}
			if len(ctx.Args) >= 2 {
				if h, err := strconv.Atoi(ctx.Args[1]); err == nil && h > 0 {
					height = h
				}
			}
			origin := placementOrigin(ctx)
			air, _ := block.ParseState("air", false)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for dx := -size; dx <= size; dx++ {
				for dz := -size; dz <= size; dz++ {
					for i := 0; i < height; i++ {
						y := origin.Y + i
						if !up {
							y = origin.Y - i
						}
						p := plugin.BlockPos{X: origin.X + dx, Y: y, Z: origin.Z + dz}
						if !block.IsAir(ctx.World.BlockState(p)) {
							es.SetBlock(p, air)
						}
					}
				}
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		}
	}

	r.Add(CmdSpec{Name: "removeabove", Aliases: []string{"/removeabove"}, Permission: "worldedit.removeabove", Description: "Remove blocks above your head", Run: removeVert(true)})
	r.Add(CmdSpec{Name: "removebelow", Aliases: []string{"/removebelow"}, Permission: "worldedit.removebelow", Description: "Remove blocks below your feet", Run: removeVert(false)})

	r.Add(CmdSpec{
		Name:        "removenear",
		Aliases:     []string{"/removenear"},
		Permission:  "worldedit.removenear",
		Description: "Remove matching blocks near you",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: /removenear <mask> [size]")
			}
			m, err := block.ParseMask(ctx.Args[0], ctx.ParseContext())
			if err != nil {
				return err
			}
			size := 50
			if len(ctx.Args) >= 2 {
				if n, err := strconv.Atoi(ctx.Args[1]); err == nil && n > 0 {
					size = n
				}
			}
			origin := placementOrigin(ctx)
			air, _ := block.ParseState("air", false)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for dy := -size; dy <= size; dy++ {
				for dz := -size; dz <= size; dz++ {
					for dx := -size; dx <= size; dx++ {
						p := plugin.BlockPos{X: origin.X + dx, Y: origin.Y + dy, Z: origin.Z + dz}
						if m.Test(es, p) {
							es.SetBlock(p, air)
						}
					}
				}
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "replacenear",
		Aliases:     []string{"/replacenear"},
		Permission:  "worldedit.replacenear",
		Description: "Replace nearby blocks matching a mask with a pattern",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 2 {
				return fmt.Errorf("usage: /replacenear <size> [from-mask] <to-pattern>")
			}
			size, err := strconv.Atoi(ctx.Args[0])
			if err != nil || size < 1 {
				return fmt.Errorf("invalid size %q", ctx.Args[0])
			}
			pctx := ctx.ParseContext()
			var m block.Mask
			var pat block.Pattern
			if len(ctx.Args) == 2 {
				m, _ = block.ParseMask("#existing", pctx)
				pat, err = block.ParsePattern(ctx.Args[1], pctx)
			} else {
				m, err = block.ParseMask(ctx.Args[1], pctx)
				if err == nil {
					pat, err = block.ParsePattern(strings.Join(ctx.Args[2:], " "), pctx)
				}
			}
			if err != nil {
				return err
			}
			origin := placementOrigin(ctx)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for dy := -size; dy <= size; dy++ {
				for dz := -size; dz <= size; dz++ {
					for dx := -size; dx <= size; dx++ {
						p := plugin.BlockPos{X: origin.X + dx, Y: origin.Y + dy, Z: origin.Z + dz}
						if m.Test(es, p) {
							es.SetPattern(p, pat)
						}
					}
				}
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "snow",
		Aliases:     []string{"/snow"},
		Permission:  "worldedit.snow",
		Description: "Simulates snow around you",
		Run: func(ctx *CmdContext) error {
			radius := 10
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					radius = n
				}
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			n := tools.SimulateSnow(es, placementOrigin(ctx), radius, ctx.Flags['s'])
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d surfaces covered with snow.", n))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "thaw",
		Aliases:     []string{"/thaw"},
		Permission:  "worldedit.thaw",
		Description: "Thaws snow and ice in the area",
		Run: func(ctx *CmdContext) error {
			radius := 10
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					radius = n
				}
			}
			origin := placementOrigin(ctx)
			air, _ := block.ParseState("air", false)
			water, _ := block.ParseState("water", false)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for dz := -radius; dz <= radius; dz++ {
				for dx := -radius; dx <= radius; dx++ {
					if dx*dx+dz*dz > radius*radius {
						continue
					}
					topY := ctx.World.HighestBlockY(origin.X+dx, origin.Z+dz, plugin.WorldSurface)
					p := plugin.BlockPos{X: origin.X + dx, Y: topY, Z: origin.Z + dz}
					switch block.BlockName(ctx.World.BlockState(p)) {
					case "minecraft:snow":
						es.SetBlock(p, air)
					case "minecraft:ice":
						es.SetBlock(p, water)
					}
				}
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d surfaces thawed.", es.Changed()))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "green",
		Aliases:     []string{"/green"},
		Permission:  "worldedit.green",
		Description: "Converts exposed dirt to grass blocks in the area",
		Run: func(ctx *CmdContext) error {
			radius := 10
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					radius = n
				}
			}
			origin := placementOrigin(ctx)
			grass, _ := block.ParseState("grass_block", false)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for dz := -radius; dz <= radius; dz++ {
				for dx := -radius; dx <= radius; dx++ {
					if dx*dx+dz*dz > radius*radius {
						continue
					}
					topY := ctx.World.HighestBlockY(origin.X+dx, origin.Z+dz, plugin.WorldSurface)
					p := plugin.BlockPos{X: origin.X + dx, Y: topY, Z: origin.Z + dz}
					n := block.BlockName(ctx.World.BlockState(p))
					if n == "minecraft:dirt" || (!ctx.Flags['f'] && n == "minecraft:coarse_dirt") {
						es.SetBlock(p, grass)
					}
				}
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d surfaces greened.", es.Changed()))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "extinguish",
		Aliases:     []string{"ex", "ext", "/ex", "/ext", "/extinguish"},
		Permission:  "worldedit.extinguish",
		Description: "Extinguish nearby fire",
		Run: func(ctx *CmdContext) error {
			radius := 40
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					radius = n
				}
			}
			origin := placementOrigin(ctx)
			air, _ := block.ParseState("air", false)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for dy := -radius; dy <= radius; dy++ {
				for dz := -radius; dz <= radius; dz++ {
					for dx := -radius; dx <= radius; dx++ {
						p := plugin.BlockPos{X: origin.X + dx, Y: origin.Y + dy, Z: origin.Z + dz}
						n := block.BlockName(ctx.World.BlockState(p))
						if n == "minecraft:fire" || n == "minecraft:soul_fire" {
							es.SetBlock(p, air)
						}
					}
				}
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d fire(s) extinguished.", es.Changed()))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "butcher",
		Aliases:     []string{"/butcher"},
		Permission:  "worldedit.butcher",
		Description: "Kill nearby mobs (-a animals, -p pets, -n npcs, -g golems, -b ambient, -t tagged, -f all)",
		Run: func(ctx *CmdContext) error {
			radius := -1
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil {
					radius = n
				}
			}
			killAll := ctx.Flags['f']
			origin := plugin.Vec3{}
			if ctx.Player != nil {
				origin = ctx.Player.Pos()
			}
			r2 := float64(radius * radius)
			killed := 0
			for ent := range ctx.World.Entities() {
				if _, isPlayer := ent.(plugin.Player); isPlayer {
					continue
				}
				if radius > 0 && ent.Pos().DistanceSq(origin) > r2 {
					continue
				}
				if _, isMob := ent.(plugin.Mob); !isMob {
					continue
				}
				if !killAll {
					if ent.CustomName() != nil && !ctx.Flags['t'] {
						continue
					}
					if _, isV := ent.(plugin.Villager); isV && !ctx.Flags['n'] {
						continue
					}
					if t, ok := ent.(plugin.Tameable); ok && t.Tamed() && !ctx.Flags['p'] {
						continue
					}
					if _, isB := ent.(plugin.Breedable); isB && !ctx.Flags['a'] {
						continue
					}
				}
				ent.Remove()
				killed++
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Killed %d mob(s).", killed))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "remove",
		Aliases:     []string{"rem", "rement", "/remove", "/rem", "/rement"},
		Permission:  "worldedit.remove",
		Description: "Remove all entities of a type within radius",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 2 {
				return fmt.Errorf("usage: /remove <items|arrows|boats|minecarts|tnt|xp|paintings|itemframes|all|type> <radius>")
			}
			kind := strings.ToLower(ctx.Args[0])
			radius, err := strconv.Atoi(ctx.Args[1])
			if err != nil {
				return fmt.Errorf("invalid radius %q", ctx.Args[1])
			}
			origin := plugin.Vec3{}
			if ctx.Player != nil {
				origin = ctx.Player.Pos()
			}
			r2 := float64(radius * radius)
			removed := 0
			for ent := range ctx.World.Entities() {
				if _, isPlayer := ent.(plugin.Player); isPlayer {
					continue
				}
				if radius > 0 && ent.Pos().DistanceSq(origin) > r2 {
					continue
				}
				ename := strings.TrimPrefix(ent.Type().Name(), "minecraft:")
				match := false
				switch kind {
				case "all":
					match = true
				case "items", "item":
					_, match = ent.(plugin.ItemEntity)
				case "arrows", "arrow":
					_, match = ent.(plugin.Arrow)
				case "xp", "experience":
					_, match = ent.(plugin.ExperienceOrb)
				case "tnt":
					_, match = ent.(plugin.TNT)
				case "boats", "boat":
					match = strings.Contains(ename, "boat")
				case "minecarts", "minecart":
					match = strings.Contains(ename, "minecart")
				case "paintings", "painting":
					match = ename == "painting"
				case "itemframes", "itemframe":
					_, match = ent.(plugin.ItemFrame)
				default:
					match = ename == strings.TrimPrefix(kind, "minecraft:")
				}
				if match {
					ent.Remove()
					removed++
				}
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Marked %d entity(s) for removal.", removed))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/calculate",
		Aliases:     []string{"/calc", "/eval", "/evaluate", "/solve"},
		Permission:  "worldedit.calc",
		Description: "Evaluate a mathematical expression",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //calc <expression>")
			}
			src := strings.Join(ctx.Args, " ")
			prog, err := expr.Compile(src)
			if err != nil {
				return err
			}
			env := expr.NewEnv(100 * time.Millisecond)
			env.Perlin = block.Perlin3D
			env.Simplex = block.Simplex3D
			env.Voronoi = block.Voronoi3D
			env.RidgedMulti = block.RidgedMulti3D
			val, err := prog.Eval(env)
			if err != nil {
				return err
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("= %g", val))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/help",
		Permission:  "worldedit.help",
		Description: "Displays help for WorldEdit commands",
		Run: func(ctx *CmdContext) error {
			r.printHelp(ctx, ctx.Args)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/confirm",
		Permission:  "fawe.confirm",
		Description: "Confirm a pending large WorldEdit command",
		Run: func(ctx *CmdContext) error {
			_, fn := ctx.Session.TakePendingConfirm()
			if fn == nil {
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.utility.nothing.confirmed")
				return nil
			}
			fn()
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/cancel",
		Aliases:     []string{"fcancel"},
		Permission:  "fawe.cancel",
		Description: "Cancel your current WorldEdit operation",
		Run: func(ctx *CmdContext) error {
			if ctx.Session.CancelActive() {
				ctx.WE.Msgs.Info(ctx.Sender, "Cancelled active edit.")
			} else {
				ctx.WE.Msgs.Info(ctx.Sender, "No active edit to cancel.")
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "masks",
		Permission:  "worldedit.masks",
		Description: "View help about WorldEdit/FAWE masks",
		Run: func(ctx *CmdContext) error {
			ctx.WE.Msgs.Info(ctx.Sender, "Masks: block, !negate, >above, <below, ~adjacent, %chance, =expr, $biome, ##tag, #existing, #air, #solid, #liquid, #surface, #wall, #region, #offset[x][y][z][m], #simplex[scale][min][max], /[min][max]")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "transforms",
		Permission:  "worldedit.transforms",
		Description: "View help about FAWE transforms",
		Run: func(ctx *CmdContext) error {
			ctx.WE.Msgs.Info(ctx.Sender, "Transforms: #offset[x][y][z], #rotate[deg], #scale[sx][sy][sz], #spread[x][y][z], #pattern[pat], #random[t1,t2], #linear[t1,t2], #linear3d[t1,t2]")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/macro",
		Permission:  "worldedit.macro",
		Description: "Run a sequence of semicolon-separated WorldEdit commands",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //macro <cmd1; cmd2; ...>")
			}
			raw := strings.Join(ctx.Args, " ")
			for _, part := range strings.Split(raw, ";") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				if strings.HasPrefix(part, "//") {
					r.tryIntercept(ctx.Sender, part[1:])
				} else if strings.HasPrefix(part, "/") {
					if !r.tryIntercept(ctx.Sender, part) {
						_, _ = ctx.WE.Server.Dispatch(ctx.Sender, part[1:])
					}
				} else {
					_, _ = ctx.WE.Server.Dispatch(ctx.Sender, part)
				}
			}
			return nil
		},
	})

	runCraftScript := func(ctx *CmdContext, name string, scriptArgs []string) error {
		clean := filepath.Base(name)
		p := filepath.Join(ctx.WE.ScriptsDir(), clean)
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("craftscript %q not found in %s", clean, ctx.WE.Config().WorldEdit.Scripting.Dir)
		}
		ctx.Session.SetLastScript(clean, scriptArgs)
		// Execute line-by-line command scripts (lines starting with # or // are comments/commands)
		ran := 0
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			for i, a := range scriptArgs {
				line = strings.ReplaceAll(line, fmt.Sprintf("$%d", i+1), a)
			}
			if strings.HasPrefix(line, "//") {
				r.tryIntercept(ctx.Sender, line[1:])
			} else if strings.HasPrefix(line, "/") {
				_, _ = ctx.WE.Server.Dispatch(ctx.Sender, line[1:])
			} else {
				_, _ = ctx.WE.Server.Dispatch(ctx.Sender, line)
			}
			ran++
		}
		ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Executed script %s (%d commands).", clean, ran))
		return nil
	}

	r.Add(CmdSpec{
		Name:        "cs",
		Aliases:     []string{"/cs"},
		Permission:  "worldedit.scripting.execute",
		Description: "Execute a command script from craftscripts/",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: /cs <filename> [args...]")
			}
			return runCraftScript(ctx, ctx.Args[0], ctx.Args[1:])
		},
	})

	r.Add(CmdSpec{
		Name:        ".s",
		Aliases:     []string{"/.s"},
		Permission:  "worldedit.scripting.execute",
		Description: "Re-execute the last CraftScript",
		Run: func(ctx *CmdContext) error {
			name, lastArgs := ctx.Session.LastScript()
			if name == "" {
				return fmt.Errorf("no previous script executed")
			}
			if len(ctx.Args) > 0 {
				lastArgs = ctx.Args
			}
			return runCraftScript(ctx, name, lastArgs)
		},
	})
}
