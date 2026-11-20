package commands

import (
	"fmt"
	"strconv"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/tools"
)

func (r *Router) registerNavigation() {
	r.Add(CmdSpec{
		Name:        "unstuck",
		Aliases:     []string{"!", "/unstuck"},
		Permission:  "worldedit.navigation.unstuck",
		Description: "Escape from being stuck inside a block",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			bp := ctx.Player.Pos().Block()
			w := ctx.World
			maxY := w.MinY() + w.Height() - 2
			for y := bp.Y; y <= maxY; y++ {
				if block.IsAir(w.BlockState(plugin.BlockPos{X: bp.X, Y: y, Z: bp.Z})) &&
					block.IsAir(w.BlockState(plugin.BlockPos{X: bp.X, Y: y + 1, Z: bp.Z})) {
					loc := ctx.Player.Location()
					loc.Pos = plugin.Vec3{X: float64(bp.X) + 0.5, Y: float64(y), Z: float64(bp.Z) + 0.5}
					ctx.Player.Teleport(loc, plugin.TeleportCommand)
					ctx.WE.Msgs.Info(ctx.Sender, "There!")
					return nil
				}
			}
			return fmt.Errorf("no free spot above you")
		},
	})

	r.Add(CmdSpec{
		Name:        "ascend",
		Aliases:     []string{"asc", "/asc", "/ascend"},
		Permission:  "worldedit.navigation.ascend",
		Description: "Go up one or more floors",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			levels := 1
			if len(ctx.Args) > 0 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					levels = n
				}
			}
			bp := ctx.Player.Pos().Block()
			w := ctx.World
			maxY := w.MinY() + w.Height() - 2
			y := bp.Y
			found := 0
			for lvl := 0; lvl < levels; lvl++ {
				// Skip air until hitting a ceiling, then find next air pair above it
				hitSolid := false
				moved := false
				for ny := y + 2; ny <= maxY; ny++ {
					solid := !block.IsAir(w.BlockState(plugin.BlockPos{X: bp.X, Y: ny, Z: bp.Z}))
					if solid {
						hitSolid = true
						continue
					}
					if hitSolid && block.IsAir(w.BlockState(plugin.BlockPos{X: bp.X, Y: ny + 1, Z: bp.Z})) {
						y = ny
						found++
						moved = true
						break
					}
				}
				if !moved {
					break
				}
			}
			if found == 0 {
				return fmt.Errorf("no free floor above you")
			}
			loc := ctx.Player.Location()
			loc.Pos = plugin.Vec3{X: float64(bp.X) + 0.5, Y: float64(y), Z: float64(bp.Z) + 0.5}
			ctx.Player.Teleport(loc, plugin.TeleportCommand)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Ascended %d level(s).", found))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "descend",
		Aliases:     []string{"desc", "/desc", "/descend"},
		Permission:  "worldedit.navigation.descend",
		Description: "Go down one or more floors",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			levels := 1
			if len(ctx.Args) > 0 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					levels = n
				}
			}
			bp := ctx.Player.Pos().Block()
			w := ctx.World
			minY := w.MinY() + 1
			y := bp.Y
			found := 0
			for lvl := 0; lvl < levels; lvl++ {
				hitSolid := false
				moved := false
				for ny := y - 1; ny >= minY; ny-- {
					curAir := block.IsAir(w.BlockState(plugin.BlockPos{X: bp.X, Y: ny, Z: bp.Z}))
					if !curAir {
						hitSolid = true
						continue
					}
					if hitSolid {
						// Drop to floor below this air pocket
						floor := ny
						for floor > minY && block.IsAir(w.BlockState(plugin.BlockPos{X: bp.X, Y: floor - 1, Z: bp.Z})) {
							floor--
						}
						if block.IsAir(w.BlockState(plugin.BlockPos{X: bp.X, Y: floor + 1, Z: bp.Z})) {
							y = floor
							found++
							moved = true
							break
						}
					}
				}
				if !moved {
					break
				}
			}
			if found == 0 {
				return fmt.Errorf("no free floor below you")
			}
			loc := ctx.Player.Location()
			loc.Pos = plugin.Vec3{X: float64(bp.X) + 0.5, Y: float64(y), Z: float64(bp.Z) + 0.5}
			ctx.Player.Teleport(loc, plugin.TeleportCommand)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Descended %d level(s).", found))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "ceil",
		Aliases:     []string{"/ceil", "/ceiling"},
		Permission:  "worldedit.navigation.ceiling",
		Description: "Go to the ceiling above you",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			clearance := 0
			if len(ctx.Args) > 0 {
				clearance, _ = strconv.Atoi(ctx.Args[0])
			}
			bp := ctx.Player.Pos().Block()
			w := ctx.World
			maxY := w.MinY() + w.Height() - 1
			ceilY := -1
			for y := bp.Y + 2; y <= maxY; y++ {
				if !block.IsAir(w.BlockState(plugin.BlockPos{X: bp.X, Y: y, Z: bp.Z})) {
					ceilY = y
					break
				}
			}
			if ceilY < 0 {
				return fmt.Errorf("no ceiling found above you")
			}
			targetY := ceilY - 2 - clearance
			if targetY < w.MinY() {
				targetY = w.MinY()
			}
			glass, _ := block.ParseState("glass", false)
			below := plugin.BlockPos{X: bp.X, Y: targetY - 1, Z: bp.Z}
			if block.IsAir(w.BlockState(below)) {
				es := ctx.WE.NewEditSession(w, ctx.Sender, ctx.RawLine)
				es.SetBlock(below, glass)
				_ = es.Commit()
			}
			loc := ctx.Player.Location()
			loc.Pos = plugin.Vec3{X: float64(bp.X) + 0.5, Y: float64(targetY), Z: float64(bp.Z) + 0.5}
			ctx.Player.Teleport(loc, plugin.TeleportCommand)
			ctx.WE.Msgs.Info(ctx.Sender, "Whoosh!")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "thru",
		Aliases:     []string{"/thru"},
		Permission:  "worldedit.navigation.thru.command",
		Description: "Pass through walls",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			tools.PassThruWall(ctx.WE, ctx.Player, ctx.WE.Config().WorldEdit.NavigationWand.MaxDistance)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "jumpto",
		Aliases:     []string{"j", "/jumpto", "/j"},
		Permission:  "worldedit.navigation.jumpto.command",
		Description: "Teleport to the block you are looking at",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			tools.JumpToTarget(ctx.WE, ctx.Player, ctx.WE.Config().WorldEdit.NavigationWand.MaxDistance)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "up",
		Aliases:     []string{"/up"},
		Permission:  "worldedit.navigation.up",
		Description: "Go upwards some distance, placing a glass block beneath your feet",
		Run: func(ctx *CmdContext) error {
			if !ctx.RequirePlayer() {
				return nil
			}
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: /up <distance>")
			}
			dist, err := strconv.Atoi(ctx.Args[0])
			if err != nil {
				return fmt.Errorf("invalid distance %q", ctx.Args[0])
			}
			bp := ctx.Player.Pos().Block()
			targetY := bp.Y + dist
			if targetY < ctx.World.MinY()+1 {
				targetY = ctx.World.MinY() + 1
			}
			if targetY > ctx.World.MinY()+ctx.World.Height()-2 {
				targetY = ctx.World.MinY() + ctx.World.Height() - 2
			}
			glass, _ := block.ParseState("glass", false)
			below := plugin.BlockPos{X: bp.X, Y: targetY - 1, Z: bp.Z}
			if block.IsAir(ctx.World.BlockState(below)) {
				es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
				es.SetBlock(below, glass)
				_ = es.Commit()
			}
			loc := ctx.Player.Location()
			loc.Pos = plugin.Vec3{X: float64(bp.X) + 0.5, Y: float64(targetY), Z: float64(bp.Z) + 0.5}
			ctx.Player.Teleport(loc, plugin.TeleportCommand)
			ctx.WE.Msgs.Info(ctx.Sender, "Whoosh!")
			return nil
		},
	})
}
