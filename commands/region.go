package commands

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/core"
	"github.com/golem-mc/worldedit/expr"
	"github.com/golem-mc/worldedit/region"
	"github.com/golem-mc/worldedit/tools"
)

func (r *Router) registerRegion() { //nolint:funlen // a command table reads better whole
	r.Add(CmdSpec{
		Name:        "/set",
		Suggest:     suggestPattern,
		Permission:  "worldedit.region.set",
		Description: "Sets all the blocks in the region",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //set <pattern>")
			}
			pat, err := block.ParsePattern(strings.Join(ctx.Args, " "), ctx.ParseContext())
			if err != nil {
				return err
			}
			return runEditWithConfirm(ctx, reg, func() error {
				es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
				n := es.SetRegion(reg, pat)
				if err := es.Commit(); err != nil {
					return err
				}
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", n)
				return nil
			})
		},
	})

	r.Add(CmdSpec{
		Name:        "/air",
		Aliases:     []string{"/0"},
		Permission:  "worldedit.region.set",
		Description: "Sets all the blocks in the region to air",
		Run: func(ctx *CmdContext) error {
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
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/replace",
		Aliases:     []string{"/re", "/rep"},
		Permission:  "worldedit.region.replace",
		Description: "Replace all blocks in the selection with another",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //replace [from-mask] <to-pattern>")
			}
			pctx := ctx.ParseContext()
			var m block.Mask
			var pat block.Pattern
			if len(ctx.Args) == 1 {
				m, _ = block.ParseMask("#existing", pctx)
				pat, err = block.ParsePattern(ctx.Args[0], pctx)
				if err != nil {
					return err
				}
			} else {
				m, err = block.ParseMask(ctx.Args[0], pctx)
				if err != nil {
					return err
				}
				pat, err = block.ParsePattern(strings.Join(ctx.Args[1:], " "), pctx)
				if err != nil {
					return err
				}
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			n := es.ReplaceRegion(reg, m, pat)
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", n)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/walls",
		Suggest:     suggestPattern,
		Permission:  "worldedit.region.walls",
		Description: "Build the four sides of the selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //walls <pattern>")
			}
			pat, err := block.ParsePattern(strings.Join(ctx.Args, " "), ctx.ParseContext())
			if err != nil {
				return err
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for _, pos := range region.Walls(reg) {
				es.SetPattern(pos, pat)
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/faces",
		Suggest:     suggestPattern,
		Aliases:     []string{"/outline"},
		Permission:  "worldedit.region.faces",
		Description: "Build the walls, ceiling, and floor of a selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //faces <pattern>")
			}
			pat, err := block.ParsePattern(strings.Join(ctx.Args, " "), ctx.ParseContext())
			if err != nil {
				return err
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for _, pos := range region.Faces(reg) {
				es.SetPattern(pos, pat)
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/overlay",
		Suggest:     suggestPattern,
		Aliases:     []string{"/lay"},
		Permission:  "worldedit.region.overlay",
		Description: "Set a block on top of blocks in the region",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //overlay <pattern>")
			}
			pat, err := block.ParsePattern(strings.Join(ctx.Args, " "), ctx.ParseContext())
			if err != nil {
				return err
			}
			isLay := strings.HasSuffix(ctx.Label, "lay")
			lo, hi := reg.Min(), reg.Max()
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for x := lo.X; x <= hi.X; x++ {
				for z := lo.Z; z <= hi.Z; z++ {
					for y := hi.Y; y >= lo.Y; y-- {
						p := plugin.BlockPos{X: x, Y: y, Z: z}
						if !reg.Contains(p) {
							continue
						}
						if !block.IsAir(ctx.World.BlockState(p)) {
							target := p
							if !isLay {
								target.Y++
							}
							es.SetPattern(target, pat)
							break
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
		Name:        "/center",
		Suggest:     suggestPattern,
		Aliases:     []string{"/middle"},
		Permission:  "worldedit.region.center",
		Description: "Set the center block(s) of the selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //center <pattern>")
			}
			pat, err := block.ParsePattern(strings.Join(ctx.Args, " "), ctx.ParseContext())
			if err != nil {
				return err
			}
			lo, hi := reg.Min(), reg.Max()
			xs := midCoords(lo.X, hi.X)
			ys := midCoords(lo.Y, hi.Y)
			zs := midCoords(lo.Z, hi.Z)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for _, x := range xs {
				for _, y := range ys {
					for _, z := range zs {
						es.SetPattern(plugin.BlockPos{X: x, Y: y, Z: z}, pat)
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
		Name:        "/naturalize",
		Permission:  "worldedit.region.naturalize",
		Description: "3 layers of dirt on top then rock below",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			grass, _ := block.ParseState("grass_block", false)
			dirt, _ := block.ParseState("dirt", false)
			stone, _ := block.ParseState("stone", false)
			lo, hi := reg.Min(), reg.Max()
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for x := lo.X; x <= hi.X; x++ {
				for z := lo.Z; z <= hi.Z; z++ {
					depth := 0
					for y := hi.Y; y >= lo.Y; y-- {
						p := plugin.BlockPos{X: x, Y: y, Z: z}
						if !reg.Contains(p) {
							continue
						}
						if block.IsAir(ctx.World.BlockState(p)) {
							depth = 0
							continue
						}
						switch {
						case depth == 0:
							es.SetBlock(p, grass)
						case depth <= 3:
							es.SetBlock(p, dirt)
						default:
							es.SetBlock(p, stone)
						}
						depth++
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
		Name:        "/smooth",
		Permission:  "worldedit.region.smooth",
		Description: "Smooth the elevation in the selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			iters := 1
			if len(ctx.Args) > 0 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					iters = n
				}
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			n := tools.SmoothRegion(es, reg, iters, false)
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", n)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/snowsmooth",
		Permission:  "worldedit.region.snowsmooth",
		Description: "Smooth the elevation in the selection with snow layers",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			iters := 1
			if len(ctx.Args) > 0 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					iters = n
				}
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			n := tools.SmoothRegion(es, reg, iters, true)
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", n)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/move",
		Aliases:     []string{"/mv"},
		Permission:  "worldedit.region.move",
		Description: "Move the contents of the selection",
		ValueFlags:  "m",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			dist := 1
			dirStr := ""
			fillStr := "air"
			if len(ctx.Args) >= 1 {
				dist, err = strconv.Atoi(ctx.Args[0])
				if err != nil {
					return fmt.Errorf("invalid distance %q", ctx.Args[0])
				}
			}
			if len(ctx.Args) >= 2 {
				dirStr = strings.ToLower(ctx.Args[1])
			}
			if len(ctx.Args) >= 3 {
				fillStr = ctx.Args[2]
			}
			yaw, pitch := playerYawPitch(ctx.Player)
			dir, err := core.DirectionFromString(dirStr, yaw, pitch)
			if err != nil {
				return err
			}
			pctx := ctx.ParseContext()
			fillPat, err := block.ParsePattern(fillStr, pctx)
			if err != nil {
				return err
			}
			var srcMask block.Mask
			if mStr := ctx.FlagVal['m']; mStr != "" {
				srcMask, err = block.ParseMask(mStr, pctx)
				if err != nil {
					return err
				}
			}
			skipAir := ctx.Flags['a']
			cb := tools.CopyRegionToClipboard(ctx.World, reg, reg.Min(), ctx.Flags['e'])
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			// Clear source
			reg.ForEach(func(p plugin.BlockPos) bool {
				if srcMask != nil && !srcMask.Test(es, p) {
					return true
				}
				es.SetPattern(p, fillPat)
				return true
			})
			// Paste at shifted target
			shift := plugin.BlockPos{X: dir.X * dist, Y: dir.Y * dist, Z: dir.Z * dist}
			target := plugin.BlockPos{X: reg.Min().X + shift.X, Y: reg.Min().Y + shift.Y, Z: reg.Min().Z + shift.Z}
			tools.PasteClipboard(es, cb, cbTransformIdentity(), target, skipAir, ctx.Flags['e'], false)
			if err := es.Commit(); err != nil {
				return err
			}
			if ctx.Flags['s'] {
				_ = reg.Shift(shift)
				applyModifiedRegion(ctx, reg)
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/stack",
		Permission:  "worldedit.region.stack",
		Description: "Repeat the contents of the selection",
		ValueFlags:  "m",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			count := 1
			dirStr := ""
			if len(ctx.Args) >= 1 {
				count, err = strconv.Atoi(ctx.Args[0])
				if err != nil || count < 1 {
					return fmt.Errorf("invalid stack count %q", ctx.Args[0])
				}
			}
			if len(ctx.Args) >= 2 {
				dirStr = strings.ToLower(ctx.Args[1])
			}
			yaw, pitch := playerYawPitch(ctx.Player)
			dir, err := core.DirectionFromString(dirStr, yaw, pitch)
			if err != nil {
				return err
			}
			cb := tools.CopyRegionToClipboard(ctx.World, reg, reg.Min(), ctx.Flags['e'])
			step := plugin.BlockPos{X: dir.X * reg.Width(), Y: dir.Y * reg.Height(), Z: dir.Z * reg.Length()}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			lo := reg.Min()
			for i := 1; i <= count; i++ {
				target := plugin.BlockPos{X: lo.X + step.X*i, Y: lo.Y + step.Y*i, Z: lo.Z + step.Z*i}
				tools.PasteClipboard(es, cb, cbTransformIdentity(), target, ctx.Flags['a'], ctx.Flags['e'], false)
			}
			if err := es.Commit(); err != nil {
				return err
			}
			if ctx.Flags['s'] {
				_ = reg.Shift(plugin.BlockPos{X: step.X * count, Y: step.Y * count, Z: step.Z * count})
				applyModifiedRegion(ctx, reg)
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/fall",
		Permission:  "worldedit.region.fall",
		Description: "Have the blocks in the selection fall",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			n := tools.ApplyGravity(es, reg)
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", n)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/hollow",
		Permission:  "worldedit.region.hollow",
		Description: "Hollows out the object contained in this selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			thickness := 1
			fillStr := "air"
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n >= 1 {
					thickness = n
				}
			}
			if len(ctx.Args) >= 2 {
				fillStr = ctx.Args[1]
			}
			pat, err := block.ParsePattern(fillStr, ctx.ParseContext())
			if err != nil {
				return err
			}
			// Find interior blocks whose distance to any air block > thickness
			var interior []plugin.BlockPos
			reg.ForEach(func(p plugin.BlockPos) bool {
				if block.IsAir(ctx.World.BlockState(p)) {
					return true
				}
				nearAir := false
				for dx := -thickness; dx <= thickness && !nearAir; dx++ {
					for dy := -thickness; dy <= thickness && !nearAir; dy++ {
						for dz := -thickness; dz <= thickness && !nearAir; dz++ {
							if absInt(dx)+absInt(dy)+absInt(dz) > thickness {
								continue
							}
							if block.IsAir(ctx.World.BlockState(plugin.BlockPos{X: p.X + dx, Y: p.Y + dy, Z: p.Z + dz})) {
								nearAir = true
							}
						}
					}
				}
				if !nearAir {
					interior = append(interior, p)
				}
				return true
			})
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for _, p := range interior {
				es.SetPattern(p, pat)
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/line",
		Permission:  "worldedit.region.line",
		Description: "Draws line segments between cuboid corners or convex vertices",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //line <pattern> [thickness]")
			}
			pat, err := block.ParsePattern(ctx.Args[0], ctx.ParseContext())
			if err != nil {
				return err
			}
			thickness := 0
			if len(ctx.Args) >= 2 {
				thickness, _ = strconv.Atoi(ctx.Args[1])
			}
			var pts []plugin.BlockPos
			switch v := reg.(type) {
			case *region.Cuboid:
				pts = tools.DrawLine(v.Pos1, v.Pos2, thickness)
			case *region.ConvexPolyhedron:
				for i := 0; i < len(v.Vertices)-1; i++ {
					pts = append(pts, tools.DrawLine(v.Vertices[i], v.Vertices[i+1], thickness)...)
				}
			default:
				return fmt.Errorf("//line requires a cuboid or convex selection")
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for _, p := range pts {
				es.SetPattern(p, pat)
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/curve",
		Permission:  "worldedit.region.curve",
		Description: "Draws a spline through selected convex vertices",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //curve <pattern> [thickness]")
			}
			pat, err := block.ParsePattern(ctx.Args[0], ctx.ParseContext())
			if err != nil {
				return err
			}
			thickness := 0
			if len(ctx.Args) >= 2 {
				thickness, _ = strconv.Atoi(ctx.Args[1])
			}
			var verts []plugin.BlockPos
			switch v := reg.(type) {
			case *region.ConvexPolyhedron:
				verts = v.Vertices
			case *region.Cuboid:
				verts = []plugin.BlockPos{v.Pos1, v.Pos2}
			default:
				return fmt.Errorf("//curve requires a convex polyhedral selection")
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for _, p := range tools.DrawCatmullRomSpline(verts, thickness) {
				es.SetPattern(p, pat)
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/deform",
		Permission:  "worldedit.region.deform",
		Description: "Deforms a selected region with an expression",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: //deform <expression>")
			}
			prog, err := expr.Compile(strings.Join(ctx.Args, " "))
			if err != nil {
				return err
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			n := tools.DeformRegion(es, reg, prog, 100*time.Millisecond)
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", n)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/forest",
		Permission:  "worldedit.region.forest",
		Description: "Make a forest within the region",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			tt := plugin.TreeOak
			density := 5.0
			if len(ctx.Args) >= 1 {
				if parsed, ok := tools.ParseTreeType(ctx.Args[0]); ok {
					tt = parsed
				}
			}
			if len(ctx.Args) >= 2 {
				if d, err := strconv.ParseFloat(ctx.Args[1], 64); err == nil && d > 0 {
					density = d
				}
			}
			lo, hi := reg.Min(), reg.Max()
			planted := 0
			for x := lo.X; x <= hi.X; x++ {
				for z := lo.Z; z <= hi.Z; z++ {
					if rand.Float64()*100.0 >= density {
						continue
					}
					for y := hi.Y; y >= lo.Y; y-- {
						p := plugin.BlockPos{X: x, Y: y, Z: z}
						if !reg.Contains(p) {
							continue
						}
						st := ctx.World.BlockState(p)
						if block.IsSolid(st) {
							if ctx.World.GenerateTree(tt, plugin.BlockPos{X: x, Y: y + 1, Z: z}) {
								planted++
							}
							break
						}
					}
				}
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d trees created.", planted))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/flora",
		Permission:  "worldedit.region.flora",
		Description: "Make flora within the region",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			density := 5.0
			if len(ctx.Args) >= 1 {
				if d, err := strconv.ParseFloat(ctx.Args[0], 64); err == nil && d > 0 {
					density = d
				}
			}
			grassFlora := []string{"minecraft:short_grass", "minecraft:dandelion", "minecraft:poppy"}
			sandFlora := []string{"minecraft:dead_bush", "minecraft:cactus"}
			lo, hi := reg.Min(), reg.Max()
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for x := lo.X; x <= hi.X; x++ {
				for z := lo.Z; z <= hi.Z; z++ {
					if rand.Float64()*100.0 >= density {
						continue
					}
					for y := hi.Y; y >= lo.Y; y-- {
						p := plugin.BlockPos{X: x, Y: y, Z: z}
						if !reg.Contains(p) {
							continue
						}
						name := block.BlockName(ctx.World.BlockState(p))
						above := plugin.BlockPos{X: x, Y: y + 1, Z: z}
						if !block.IsAir(ctx.World.BlockState(above)) {
							break
						}
						if name == "minecraft:grass_block" || name == "minecraft:dirt" {
							if st, err := block.ParseState(grassFlora[rand.IntN(len(grassFlora))], false); err == nil {
								es.SetBlock(above, st)
							}
							break
						}
						if name == "minecraft:sand" || name == "minecraft:red_sand" {
							if st, err := block.ParseState(sandFlora[rand.IntN(len(sandFlora))], false); err == nil {
								es.SetBlock(above, st)
							}
							break
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
		Name:        "/fixblocks",
		Aliases:     []string{"/updateblocks", "/fixconnect"},
		Permission:  "worldedit.region.fixblocks",
		Description: "Updates block connections/shapes in the selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			var count int64
			reg.ForEach(func(p plugin.BlockPos) bool {
				st := ctx.World.BlockState(p)
				if !block.IsAir(st) {
					ctx.World.SetBlockState(p, st, plugin.UpdateAll)
					count++
				}
				return true
			})
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", count)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/regen",
		Permission:  "worldedit.regen",
		Description: "Regenerates the contents of the selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			regenerateRegion(ctx.World, reg)
			ctx.WE.Msgs.Info(ctx.Sender, "Region regenerated.")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "wea",
		Aliases:     []string{"/wea", "worldeditanywhere", "/worldeditanywhere", "/weanywhere"},
		Permission:  "fawe.admin",
		Description: "Bypass region restrictions",
		Run: func(ctx *CmdContext) error {
			next := !ctx.Session.BypassRegions()
			ctx.Session.SetBypassRegions(next)
			if next {
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.info.worldedit.bypassed")
			} else {
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.info.worldedit.restricted")
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "wer",
		Aliases:     []string{"/wer", "worldeditregion", "/worldeditregion"},
		Permission:  "fawe.worldeditregion",
		Description: "Select your current allowed region",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			allowed := ctx.WE.AllowedRegions(ctx.Player, ctx.World)
			if len(allowed) == 0 {
				ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no.region")
				return nil
			}
			applyModifiedRegion(ctx, allowed[0])
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.info.set.region")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/nbtinfo",
		Aliases:     []string{"/nbt"},
		Permission:  "worldedit.nbtinfo",
		Description: "View NBT info for the targeted block",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			blk, ok := ctx.Player.TargetBlock(20)
			if !ok {
				return fmt.Errorf("no block in sight")
			}
			be := core.CaptureBlockEntityNBT(ctx.World, blk.Pos)
			if be == nil {
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%s at %s has no block entity NBT.", block.FormatState(blk.State()), region.FormatPos(blk.Pos)))
				return nil
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("NBT at %s: %v", region.FormatPos(blk.Pos), be))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/fixlighting",
		Permission:  "worldedit.light.fix",
		Description: "Refresh lighting in the selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			chunks := reg.Chunks()
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.info.updated.lighting.selection", len(chunks))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/removelighting",
		Aliases:     []string{"/removelight"},
		Permission:  "worldedit.light.remove",
		Description: "Remove lighting in the selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.info.updated.lighting.selection", len(reg.Chunks()))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/setblocklight",
		Aliases:     []string{"/setlight", "/setskylight"},
		Permission:  "worldedit.light.set",
		Description: "Set lighting in a selection (deprecated: use minecraft:light blocks)",
		Run: func(ctx *CmdContext) error {
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.info.light-blocks")
			return nil
		},
	})
}

func runEditWithConfirm(ctx *CmdContext, reg region.Region, fn func() error) error {
	lim := ctx.WE.Config().LimitFor(ctx.Sender)
	if lim.ConfirmLarge && len(reg.Chunks()) > 16384 {
		ctx.Session.SetPendingConfirm(ctx.RawLine, func() {
			if err := fn(); err != nil {
				ctx.WE.Msgs.Error(ctx.Sender, err.Error())
			}
		})
		ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Large edit (%d chunks). Run //confirm to proceed.", len(reg.Chunks())))
		return nil
	}
	return fn()
}

func midCoords(lo, hi int) []int {
	if (lo+hi)%2 == 0 {
		return []int{(lo + hi) / 2}
	}
	m := (lo + hi - 1) / 2
	return []int{m, m + 1}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

var _ registry.Block

// regenerateRegion regenerates the selection from the world's seed. golem
// regenerates whole chunks, so the blocks of a chunk that lie outside the
// region are put back afterwards. A chunk that is not loaded is loaded first
// and regenerated when it is ready.
func regenerateRegion(w plugin.World, reg region.Region) {
	regen := func(cp plugin.ChunkPos) bool {
		type saved struct {
			pos plugin.BlockPos
			st  plugin.BlockState
		}
		var keep []saved
		for y := w.MinY(); y < w.MinY()+w.Height(); y++ {
			for z := cp.Z << 4; z < cp.Z<<4+16; z++ {
				for x := cp.X << 4; x < cp.X<<4+16; x++ {
					p := plugin.BlockPos{X: x, Y: y, Z: z}
					if !reg.Contains(p) {
						keep = append(keep, saved{p, w.BlockState(p)})
					}
				}
			}
		}
		if !w.Regenerate(cp) {
			return false
		}
		for _, k := range keep {
			w.SetBlockState(k.pos, k.st, plugin.UpdateClients)
		}
		return true
	}
	for _, cp := range reg.Chunks() {
		if regen(cp) {
			continue
		}
		w.LoadChunk(cp, true, func(c plugin.Chunk) {
			if c != nil {
				regen(cp)
			}
		})
	}
}
