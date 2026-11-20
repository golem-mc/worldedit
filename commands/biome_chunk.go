package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
)

func (r *Router) registerBiomeAndChunk() {
	r.Add(CmdSpec{
		Name:        "biomelist",
		Aliases:     []string{"biomels", "/biomelist", "/listbiomes"},
		Permission:  "worldedit.biome.list",
		Description: "Lists all available biomes",
		Run: func(ctx *CmdContext) error {
			var names []string
			for i := range registry.Biome(registry.BiomeCount) {
				if n := i.Name(); n != "" {
					names = append(names, strings.TrimPrefix(n, "minecraft:"))
				}
			}
			sort.Strings(names)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Available biomes (%d): %s", len(names), strings.Join(names, ", ")))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "biomeinfo",
		Aliases:     []string{"/biomeinfo"},
		Permission:  "worldedit.biome.info",
		Description: "Get the biome(s) of your selection, targeted block (-t), or current position (-p)",
		Run: func(ctx *CmdContext) error {
			if ctx.Flags['p'] || (ctx.Player != nil && func() bool { _, err := ctx.Session.Selection(ctx.World); return err != nil }()) {
				pos := placementOrigin(ctx)
				b := ctx.World.Biome(pos)
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Biome at (%d, %d, %d): %s", pos.X, pos.Y, pos.Z, b.Name()))
				return nil
			}
			if ctx.Flags['t'] && ctx.Player != nil {
				blk, ok := ctx.Player.TargetBlock(120)
				if !ok {
					return fmt.Errorf("no block in sight")
				}
				ctx.WE.Msgs.Info(ctx.Sender, "Biome: "+ctx.World.Biome(blk.Pos).Name())
				return nil
			}
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first (or pass -p)")
			}
			seen := make(map[string]bool)
			lo, hi := reg.Min(), reg.Max()
			for x := lo.X; x <= hi.X; x += 4 {
				for z := lo.Z; z <= hi.Z; z += 4 {
					seen[ctx.World.Biome(plugin.BlockPos{X: x, Y: lo.Y, Z: z}).Name()] = true
				}
			}
			var out []string
			for k := range seen {
				out = append(out, k)
			}
			sort.Strings(out)
			ctx.WE.Msgs.Info(ctx.Sender, "Biomes in selection: "+strings.Join(out, ", "))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/setbiome",
		Permission:  "worldedit.biome.set",
		Description: "Sets the biome of your selection",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: //setbiome [-p] <biome>")
			}
			bname := registry.Qualify(strings.ToLower(ctx.Args[0]))
			if _, ok := registry.BiomeByName(bname); !ok {
				return fmt.Errorf("unknown biome %q", ctx.Args[0])
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Biome %s validated; live chunk biome mutation is not exposed by golem's World API (biomes are written during chunk generation).", bname))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "chunkinfo",
		Aliases:     []string{"/chunkinfo"},
		Permission:  "worldedit.chunkinfo",
		Description: "Get information about the chunk you are inside",
		Run: func(ctx *CmdContext) error {
			pos := placementOrigin(ctx)
			cp := pos.Chunk()
			rx, rz := cp.X>>5, cp.Z>>5
			var inhabited int64
			var entCount, beCount int
			if ch, ok := ctx.World.Chunk(cp); ok {
				inhabited = ch.InhabitedTime()
				entCount = len(ch.Entities())
				beCount = len(ch.BlockEntities())
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Chunk (%d, %d) | Region file: r.%d.%d.mca | Entities: %d | BlockEntities: %d | Inhabited: %d ticks",
				cp.X, cp.Z, rx, rz, entCount, beCount, inhabited))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "listchunks",
		Aliases:     []string{"/listchunks"},
		Permission:  "worldedit.listchunks",
		Description: "List chunks included in your selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			chunks := reg.Chunks()
			var parts []string
			for i, c := range chunks {
				if i >= 20 {
					parts = append(parts, fmt.Sprintf("... +%d more", len(chunks)-i))
					break
				}
				parts = append(parts, fmt.Sprintf("(%d,%d)", c.X, c.Z))
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Selection chunks (%d): %s", len(chunks), strings.Join(parts, ", ")))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "delchunks",
		Aliases:     []string{"/delchunks"},
		Permission:  "worldedit.delchunks",
		Description: "Report chunk files covering your selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Selection covers %d chunk(s). Offline chunk deletion can be performed via /anvil clear.", len(reg.Chunks())))
			return nil
		},
	})
}
