package commands

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/expr"
	"github.com/golem-mc/worldedit/tools"
)

func (r *Router) registerGeneration() {
	makeCyl := func(hollow bool) func(*CmdContext) error {
		return func(ctx *CmdContext) error {
			if len(ctx.Args) < 2 {
				return fmt.Errorf("usage: //cyl <pattern> <radius[,rz]> [height]")
			}
			pat, err := block.ParsePattern(ctx.Args[0], ctx.ParseContext())
			if err != nil {
				return err
			}
			rx, rz, err := parseRadii2(ctx.Args[1])
			if err != nil {
				return err
			}
			height := 1
			if len(ctx.Args) >= 3 {
				height, err = strconv.Atoi(ctx.Args[2])
				if err != nil || height == 0 {
					return fmt.Errorf("invalid height %q", ctx.Args[2])
				}
			}
			if err := checkMaxRadius(ctx, max(rx, rz)); err != nil {
				return err
			}
			origin := placementOrigin(ctx)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			yStart, yEnd := 0, height-1
			if height < 0 {
				yStart, yEnd = height+1, 0
			}
			irx, irz := int(math.Ceil(rx)), int(math.Ceil(rz))
			for dy := yStart; dy <= yEnd; dy++ {
				for dz := -irz; dz <= irz; dz++ {
					for dx := -irx; dx <= irx; dx++ {
						nx := float64(dx) / (rx + 0.5)
						nz := float64(dz) / (rz + 0.5)
						d2 := nx*nx + nz*nz
						if d2 > 1.0 {
							continue
						}
						if hollow {
							inx := float64(dx) / max(0.01, rx-0.5)
							inz := float64(dz) / max(0.01, rz-0.5)
							if inx*inx+inz*inz < 1.0 {
								continue
							}
						}
						es.SetPattern(plugin.BlockPos{X: origin.X + dx, Y: origin.Y + dy, Z: origin.Z + dz}, pat)
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

	r.Add(CmdSpec{Name: "/cyl", Permission: "worldedit.generation.cylinder", Description: "Generates a cylinder", Run: makeCyl(false)})
	r.Add(CmdSpec{Name: "/hcyl", Permission: "worldedit.generation.cylinder", Description: "Generates a hollow cylinder", Run: makeCyl(true)})

	makeSphere := func(hollow bool) func(*CmdContext) error {
		return func(ctx *CmdContext) error {
			if len(ctx.Args) < 2 {
				return fmt.Errorf("usage: //sphere [-r] <pattern> <radius[,ry,rz]>")
			}
			pat, err := block.ParsePattern(ctx.Args[0], ctx.ParseContext())
			if err != nil {
				return err
			}
			rx, ry, rz, err := parseRadii3(ctx.Args[1])
			if err != nil {
				return err
			}
			if err := checkMaxRadius(ctx, max(rx, max(ry, rz))); err != nil {
				return err
			}
			origin := placementOrigin(ctx)
			if ctx.Flags['r'] {
				origin.Y += int(math.Ceil(ry))
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			irx, iry, irz := int(math.Ceil(rx)), int(math.Ceil(ry)), int(math.Ceil(rz))
			for dy := -iry; dy <= iry; dy++ {
				for dz := -irz; dz <= irz; dz++ {
					for dx := -irx; dx <= irx; dx++ {
						nx := float64(dx) / (rx + 0.5)
						ny := float64(dy) / (ry + 0.5)
						nz := float64(dz) / (rz + 0.5)
						if nx*nx+ny*ny+nz*nz > 1.0 {
							continue
						}
						if hollow {
							inx := float64(dx) / max(0.01, rx-0.5)
							iny := float64(dy) / max(0.01, ry-0.5)
							inz := float64(dz) / max(0.01, rz-0.5)
							if inx*inx+iny*iny+inz*inz < 1.0 {
								continue
							}
						}
						es.SetPattern(plugin.BlockPos{X: origin.X + dx, Y: origin.Y + dy, Z: origin.Z + dz}, pat)
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

	r.Add(CmdSpec{Name: "/sphere", Permission: "worldedit.generation.sphere", Description: "Generates a filled sphere", Run: makeSphere(false)})
	r.Add(CmdSpec{Name: "/hsphere", Permission: "worldedit.generation.sphere", Description: "Generates a hollow sphere", Run: makeSphere(true)})

	makePyramid := func(hollow bool) func(*CmdContext) error {
		return func(ctx *CmdContext) error {
			if len(ctx.Args) < 2 {
				return fmt.Errorf("usage: //pyramid <pattern> <size>")
			}
			pat, err := block.ParsePattern(ctx.Args[0], ctx.ParseContext())
			if err != nil {
				return err
			}
			size, err := strconv.Atoi(ctx.Args[1])
			if err != nil || size < 1 {
				return fmt.Errorf("invalid size %q", ctx.Args[1])
			}
			if err := checkMaxRadius(ctx, float64(size)); err != nil {
				return err
			}
			origin := placementOrigin(ctx)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for dy := 0; dy < size; dy++ {
				half := size - 1 - dy
				for dz := -half; dz <= half; dz++ {
					for dx := -half; dx <= half; dx++ {
						if hollow && dy < size-1 && absInt(dx) < half && absInt(dz) < half {
							continue
						}
						es.SetPattern(plugin.BlockPos{X: origin.X + dx, Y: origin.Y + dy, Z: origin.Z + dz}, pat)
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

	r.Add(CmdSpec{Name: "/pyramid", Permission: "worldedit.generation.pyramid", Description: "Generate a filled pyramid", Run: makePyramid(false)})
	r.Add(CmdSpec{Name: "/hpyramid", Permission: "worldedit.generation.pyramid", Description: "Generate a hollow pyramid", Run: makePyramid(true)})

	r.Add(CmdSpec{
		Name:        "/cone",
		Permission:  "worldedit.generation.cone",
		Description: "Generates a cone",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 3 {
				return fmt.Errorf("usage: //cone [-h] <pattern> <radius[,rz]> <height> [thickness]")
			}
			pat, err := block.ParsePattern(ctx.Args[0], ctx.ParseContext())
			if err != nil {
				return err
			}
			rx, rz, err := parseRadii2(ctx.Args[1])
			if err != nil {
				return err
			}
			height, err := strconv.Atoi(ctx.Args[2])
			if err != nil || height < 1 {
				return fmt.Errorf("invalid height %q", ctx.Args[2])
			}
			hollow := ctx.Flags['h']
			origin := placementOrigin(ctx)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for dy := 0; dy < height; dy++ {
				frac := 1.0 - float64(dy)/float64(height)
				crx, crz := rx*frac, rz*frac
				irx, irz := int(math.Ceil(crx)), int(math.Ceil(crz))
				for dz := -irz; dz <= irz; dz++ {
					for dx := -irx; dx <= irx; dx++ {
						nx := float64(dx) / (crx + 0.5)
						nz := float64(dz) / (crz + 0.5)
						if nx*nx+nz*nz > 1.0 {
							continue
						}
						if hollow && dy < height-1 {
							inx := float64(dx) / max(0.01, crx-0.5)
							inz := float64(dz) / max(0.01, crz-0.5)
							if inx*inx+inz*inz < 1.0 {
								continue
							}
						}
						es.SetPattern(plugin.BlockPos{X: origin.X + dx, Y: origin.Y + dy, Z: origin.Z + dz}, pat)
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
		Name:        "/generate",
		Aliases:     []string{"/gen", "/g"},
		Permission:  "worldedit.generation.shape",
		Description: "Generates a shape according to a mathematical formula",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) < 2 {
				return fmt.Errorf("usage: //generate [-r|-o|-c] <pattern> <expression>")
			}
			pat, err := block.ParsePattern(ctx.Args[0], ctx.ParseContext())
			if err != nil {
				return err
			}
			prog, err := expr.Compile(strings.Join(ctx.Args[1:], " "))
			if err != nil {
				return err
			}
			rawCoords := ctx.Flags['r']
			offsetOrigin := ctx.Flags['o']
			centerCoords := ctx.Flags['c']
			lo, hi := reg.Min(), reg.Max()
			cx := float64(lo.X+hi.X) / 2.0
			cy := float64(lo.Y+hi.Y) / 2.0
			cz := float64(lo.Z+hi.Z) / 2.0
			sx := max(1.0, float64(hi.X-lo.X)/2.0)
			sy := max(1.0, float64(hi.Y-lo.Y)/2.0)
			sz := max(1.0, float64(hi.Z-lo.Z)/2.0)
			orig := placementOrigin(ctx)
			timeout := time.Duration(max(50, ctx.WE.Config().LimitFor(ctx.Sender).MaxExpressionMS)) * time.Millisecond

			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			reg.ForEach(func(p plugin.BlockPos) bool {
				env := expr.NewEnv(timeout)
				switch {
				case rawCoords:
					env.Vars["x"], env.Vars["y"], env.Vars["z"] = float64(p.X), float64(p.Y), float64(p.Z)
				case offsetOrigin:
					env.Vars["x"], env.Vars["y"], env.Vars["z"] = float64(p.X-orig.X), float64(p.Y-orig.Y), float64(p.Z-orig.Z)
				case centerCoords:
					env.Vars["x"], env.Vars["y"], env.Vars["z"] = float64(p.X)-cx, float64(p.Y)-cy, float64(p.Z)-cz
				default:
					env.Vars["x"] = (float64(p.X) - cx) / sx
					env.Vars["y"] = (float64(p.Y) - cy) / sy
					env.Vars["z"] = (float64(p.Z) - cz) / sz
				}
				env.Perlin = block.Perlin3D
				env.Simplex = block.Simplex3D
				env.Voronoi = block.Voronoi3D
				env.RidgedMulti = block.RidgedMulti3D
				if v, err := prog.Eval(env); err == nil && v > 0 {
					es.SetPattern(p, pat)
				}
				return true
			})
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/generatebiome",
		Aliases:     []string{"/genbiome", "/gb"},
		Permission:  "worldedit.generation.shape.biome",
		Description: "Sets biome according to a formula",
		Run: func(ctx *CmdContext) error {
			ctx.WE.Msgs.Info(ctx.Sender, "Live biome modification is not exposed by golem's World API (biomes are assigned at chunk generation time).")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "forestgen",
		Aliases:     []string{"/forestgen"},
		Permission:  "worldedit.generation.forest",
		Description: "Generate a forest around you",
		Run: func(ctx *CmdContext) error {
			size := 10
			tt := plugin.TreeOak
			density := 5.0
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					size = n
				}
			}
			if len(ctx.Args) >= 2 {
				if parsed, ok := tools.ParseTreeType(ctx.Args[1]); ok {
					tt = parsed
				}
			}
			if len(ctx.Args) >= 3 {
				if d, err := strconv.ParseFloat(ctx.Args[2], 64); err == nil && d > 0 {
					density = d
				}
			}
			origin := placementOrigin(ctx)
			planted := 0
			for dx := -size; dx <= size; dx++ {
				for dz := -size; dz <= size; dz++ {
					if rand.Float64()*100.0 >= density {
						continue
					}
					topY := ctx.World.HighestBlockY(origin.X+dx, origin.Z+dz, plugin.WorldSurface)
					if ctx.World.GenerateTree(tt, plugin.BlockPos{X: origin.X + dx, Y: topY + 1, Z: origin.Z + dz}) {
						planted++
					}
				}
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("%d trees created.", planted))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "pumpkins",
		Aliases:     []string{"/pumpkins"},
		Permission:  "worldedit.generation.pumpkins",
		Description: "Generate pumpkin patches",
		Run: func(ctx *CmdContext) error {
			size := 10
			if len(ctx.Args) >= 1 {
				if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
					size = n
				}
			}
			pumpkin, _ := block.ParseState("pumpkin", false)
			leaves, _ := block.ParseState("oak_leaves[persistent=true]", false)
			origin := placementOrigin(ctx)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			patches := max(1, size/3)
			for i := 0; i < patches; i++ {
				px := origin.X + rand.IntN(size*2+1) - size
				pz := origin.Z + rand.IntN(size*2+1) - size
				for j := 0; j < 6; j++ {
					x := px + rand.IntN(5) - 2
					z := pz + rand.IntN(5) - 2
					topY := ctx.World.HighestBlockY(x, z, plugin.WorldSurface)
					pos := plugin.BlockPos{X: x, Y: topY + 1, Z: z}
					if rand.Float64() < 0.6 {
						es.SetBlock(pos, pumpkin)
					} else {
						es.SetBlock(pos, leaves)
					}
				}
			}
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Generated %d pumpkin patch blocks.", es.Changed()))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/caves",
		Permission:  "worldedit.generation.caves",
		Description: "Generates a cave network inside the selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			air, _ := block.ParseState("air", false)
			seed := rand.Int64()
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			reg.ForEach(func(p plugin.BlockPos) bool {
				if block.IsAir(ctx.World.BlockState(p)) {
					return true
				}
				n := block.RidgedMulti3D(seed, float64(p.X), float64(p.Y), float64(p.Z), 0.08, 2)
				if n > 0.82 {
					es.SetBlock(p, air)
				}
				return true
			})
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/ore",
		Permission:  "worldedit.generation.ore",
		Description: "Generates veins of a specified ore pattern inside the selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			if len(ctx.Args) < 2 {
				return fmt.Errorf("usage: //ore <mask> <pattern> [size] [freq] [rarity] [minY] [maxY]")
			}
			pctx := ctx.ParseContext()
			m, err := block.ParseMask(ctx.Args[0], pctx)
			if err != nil {
				return err
			}
			pat, err := block.ParsePattern(ctx.Args[1], pctx)
			if err != nil {
				return err
			}
			veinSize := 8
			freq := 10
			if len(ctx.Args) >= 3 {
				if n, err := strconv.Atoi(ctx.Args[2]); err == nil && n > 0 {
					veinSize = n
				}
			}
			if len(ctx.Args) >= 4 {
				if n, err := strconv.Atoi(ctx.Args[3]); err == nil && n > 0 {
					freq = n
				}
			}
			lo, hi := reg.Min(), reg.Max()
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for v := 0; v < freq; v++ {
				cx := lo.X + rand.IntN(max(1, hi.X-lo.X+1))
				cy := lo.Y + rand.IntN(max(1, hi.Y-lo.Y+1))
				cz := lo.Z + rand.IntN(max(1, hi.Z-lo.Z+1))
				for step := 0; step < veinSize; step++ {
					p := plugin.BlockPos{X: cx, Y: cy, Z: cz}
					if reg.Contains(p) && m.Test(es, p) {
						es.SetPattern(p, pat)
					}
					cx += rand.IntN(3) - 1
					cy += rand.IntN(3) - 1
					cz += rand.IntN(3) - 1
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
		Name:        "/ores",
		Permission:  "worldedit.generation.ore",
		Description: "Generates vanilla ore veins inside the selection",
		Run: func(ctx *CmdContext) error {
			reg, err := ctx.Session.Selection(ctx.World)
			if err != nil {
				return fmt.Errorf("make a region selection first")
			}
			maskStr := "stone,deepslate"
			if len(ctx.Args) > 0 {
				maskStr = ctx.Args[0]
			}
			m, err := block.ParseMask(maskStr, ctx.ParseContext())
			if err != nil {
				return err
			}
			ores := []struct {
				name string
				prob float64
			}{
				{"coal_ore", 0.012},
				{"iron_ore", 0.008},
				{"copper_ore", 0.006},
				{"gold_ore", 0.003},
				{"redstone_ore", 0.004},
				{"lapis_ore", 0.002},
				{"diamond_ore", 0.0015},
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			reg.ForEach(func(p plugin.BlockPos) bool {
				if !m.Test(es, p) {
					return true
				}
				roll := rand.Float64()
				acc := 0.0
				for _, o := range ores {
					acc += o.prob
					if roll < acc {
						if st, err := block.ParseState(o.name, false); err == nil {
							es.SetBlock(p, st)
						}
						break
					}
				}
				return true
			})
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", es.Changed())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/blob",
		Aliases:     []string{"/rock"},
		Permission:  "worldedit.generation.blob",
		Description: "Creates a distorted rock/blob sphere",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 2 {
				return fmt.Errorf("usage: //blob <pattern> <radius[,ry,rz]>")
			}
			pat, err := block.ParsePattern(ctx.Args[0], ctx.ParseContext())
			if err != nil {
				return err
			}
			rx, ry, rz, err := parseRadii3(ctx.Args[1])
			if err != nil {
				return err
			}
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			n := tools.GenerateBlob(es, placementOrigin(ctx), rx, ry, rz, pat)
			if err := es.Commit(); err != nil {
				return err
			}
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.visitor.visitor.block", n)
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/img",
		Aliases:     []string{"/image", "image"},
		Permission:  "worldedit.generation.image",
		Description: "Generate pixel art in the world from a local heightmap/image file",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: //img <filename> [max-width]")
			}
			fname := filepath.Base(ctx.Args[0])
			full := filepath.Join(ctx.WE.HeightmapDir(), fname)
			f, err := os.Open(full)
			if err != nil {
				return fmt.Errorf("cannot open image %q in heightmap/: %w", fname, err)
			}
			defer f.Close()
			img, _, err := image.Decode(f)
			if err != nil {
				return fmt.Errorf("decode image: %w", err)
			}
			b := img.Bounds()
			if b.Dx()*b.Dy() > ctx.WE.Config().FAWE.Web.MaxImageSize {
				ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.image-dimensions", ctx.WE.Config().FAWE.Web.MaxImageSize)
				return nil
			}
			origin := placementOrigin(ctx)
			es := ctx.WE.NewEditSession(ctx.World, ctx.Sender, ctx.RawLine)
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					r16, g16, b16, a16 := img.At(x, y).RGBA()
					if a16 < 0x8000 {
						continue
					}
					st := block.NearestBlockForRGB(uint8(r16>>8), uint8(g16>>8), uint8(b16>>8))
					es.SetBlock(plugin.BlockPos{X: origin.X + (x - b.Min.X), Y: origin.Y, Z: origin.Z + (y - b.Min.Y)}, st)
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
		Name:        "/feature",
		Aliases:     []string{"/placefeature"},
		Permission:  "worldedit.generation.feature",
		Description: "Generate a Minecraft tree/feature at the placement position",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: //feature <tree_type>")
			}
			tt, ok := tools.ParseTreeType(ctx.Args[0])
			if !ok {
				return fmt.Errorf("unsupported feature/tree type %q", ctx.Args[0])
			}
			if !ctx.World.GenerateTree(tt, placementOrigin(ctx)) {
				return fmt.Errorf("feature %q could not generate there", ctx.Args[0])
			}
			ctx.WE.Msgs.Info(ctx.Sender, "Generated feature "+ctx.Args[0])
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/structure",
		Permission:  "worldedit.generation.structure",
		Description: "Place a structure (delegates to /place structure)",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: //structure <structure_id>")
			}
			_, err := ctx.WE.Server.Dispatch(ctx.Sender, "place structure "+ctx.Args[0])
			return err
		},
	})
}

func placementOrigin(ctx *CmdContext) plugin.BlockPos {
	if ctx.Player != nil {
		return ctx.WE.PlacementPos(ctx.Player, ctx.Session)
	}
	if sel, err := ctx.Session.Selection(ctx.World); err == nil {
		return sel.Min()
	}
	return plugin.BlockPos{Y: 64}
}

func parseRadii2(s string) (float64, float64, error) {
	parts := strings.Split(s, ",")
	rx, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || rx < 0 {
		return 0, 0, fmt.Errorf("invalid radius %q", s)
	}
	if len(parts) == 1 {
		return rx, rx, nil
	}
	rz, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || rz < 0 {
		return 0, 0, fmt.Errorf("invalid radius %q", s)
	}
	return rx, rz, nil
}

func parseRadii3(s string) (float64, float64, float64, error) {
	parts := strings.Split(s, ",")
	rx, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || rx < 0 {
		return 0, 0, 0, fmt.Errorf("invalid radius %q", s)
	}
	if len(parts) == 1 {
		return rx, rx, rx, nil
	}
	if len(parts) == 2 {
		ry, err := strconv.ParseFloat(parts[1], 64)
		if err != nil || ry < 0 {
			return 0, 0, 0, fmt.Errorf("invalid radius %q", s)
		}
		return rx, ry, rx, nil
	}
	ry, err1 := strconv.ParseFloat(parts[1], 64)
	rz, err2 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || ry < 0 || rz < 0 {
		return 0, 0, 0, fmt.Errorf("invalid radius %q", s)
	}
	return rx, ry, rz, nil
}

func checkMaxRadius(ctx *CmdContext, radius float64) error {
	lim := ctx.WE.Config().LimitFor(ctx.Sender)
	if !lim.Unlimited && lim.MaxRadius > 0 && radius > float64(lim.MaxRadius) {
		return fmt.Errorf("maximum radius allowed is %d", lim.MaxRadius)
	}
	return nil
}
