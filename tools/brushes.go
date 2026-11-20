package tools

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/core"
	"github.com/golem-mc/worldedit/expr"
	"github.com/golem-mc/worldedit/region"
	"github.com/golem-mc/worldedit/storage"
)

// BrushKind identifies a WorldEdit or FAWE brush behavior.
type BrushKind string

const (
	BrushSphere            BrushKind = "sphere"
	BrushCylinder          BrushKind = "cylinder"
	BrushClipboard         BrushKind = "clipboard"
	BrushSmooth            BrushKind = "smooth"
	BrushSnowSmooth        BrushKind = "snowsmooth"
	BrushExtinguish        BrushKind = "extinguish"
	BrushGravity           BrushKind = "gravity"
	BrushButcher           BrushKind = "butcher"
	BrushBiome             BrushKind = "biome"
	BrushMorph             BrushKind = "morph"
	BrushBlendBall         BrushKind = "blendball"
	BrushErode             BrushKind = "erode"
	BrushPull              BrushKind = "pull"
	BrushCircle            BrushKind = "circle"
	BrushRecursive         BrushKind = "recursive"
	BrushLine              BrushKind = "line"
	BrushSpline            BrushKind = "spline"
	BrushSurfaceSpline     BrushKind = "surfacespline"
	BrushCatenary          BrushKind = "catenary"
	BrushSweep             BrushKind = "sweep"
	BrushRock              BrushKind = "rock"
	BrushShatter           BrushKind = "shatter"
	BrushLayer             BrushKind = "layer"
	BrushScatter           BrushKind = "scatter"
	BrushSplatter          BrushKind = "splatter"
	BrushPopulateSchematic BrushKind = "populateschematic"
	BrushScatterCommand    BrushKind = "scattercommand"
	BrushHeight            BrushKind = "height"
	BrushCliff             BrushKind = "cliff"
	BrushFlatten           BrushKind = "flatten"
	BrushStencil           BrushKind = "stencil"
	BrushSurface           BrushKind = "surface"
	BrushCopyPasta         BrushKind = "copypaste"
	BrushCommand           BrushKind = "command"
	BrushDeform            BrushKind = "deform"
	BrushSet               BrushKind = "set"
	BrushForest            BrushKind = "forest"
	BrushRaise             BrushKind = "raise"
	BrushLower             BrushKind = "lower"
	BrushSnow              BrushKind = "snow"
)

// BrushSettings holds one configurable brush action (primary or secondary).
type BrushSettings struct {
	Kind         BrushKind
	Pattern      block.Pattern
	PatternStr   string
	Size         int
	Height       int
	Hollow       bool
	Iterations   int
	Layers       []block.Pattern
	Commands     []string
	Expression   string
	TreeType     plugin.TreeType
	Biome        registry.Biome
	YScale       float64
	Density      float64
	ErodeFaces   int
	ErodeIter    int
	FillFaces    int
	FillIter     int
	ClipboardAir bool
	// Stateful points for multi-click brushes (line, spline, catenary, copypaste)
	Points []plugin.BlockPos
}

// BrushTool is a core.Tool bound to an item that supports FAWE primary/secondary
// brush actions, masks, source masks, trace masks, transforms, target modes, and scroll modes.
type BrushTool struct {
	PrimaryAction   *BrushSettings
	SecondaryAction *BrushSettings
	Mask            block.Mask
	SourceMask      block.Mask
	TraceMask       block.Mask
	Transform       block.Transform
	Range           int
	TargetMode      string // "TARGET_BLOCK_RANGE", "FORWARD_POINT_PITCH", "TARGET_POINT_HEIGHT", "TARGET_FACE_RANGE"
	TargetOffset    int
	ScrollMode      string
}

// NewBrushTool constructs a BrushTool with default range and primary action.
func NewBrushTool(action *BrushSettings) *BrushTool {
	return &BrushTool{
		PrimaryAction: action,
		Range:         120,
		TargetMode:    "TARGET_BLOCK_RANGE",
	}
}

func (b *BrushTool) Name() string {
	if b.PrimaryAction != nil {
		return "brush:" + string(b.PrimaryAction.Kind)
	}
	return "brush"
}

// Active returns the primary action (used by /size, /material, /mask, etc.).
func (b *BrushTool) Active() *BrushSettings {
	if b.PrimaryAction == nil {
		b.PrimaryAction = &BrushSettings{Kind: BrushSphere, Size: 5}
	}
	return b.PrimaryAction
}

func (b *BrushTool) Primary(ctx core.ToolContext) bool {
	if b.SecondaryAction != nil {
		return b.runAction(ctx, b.SecondaryAction, true)
	}
	// Multi-click brushes use left-click to add points or copy
	if b.PrimaryAction != nil {
		switch b.PrimaryAction.Kind {
		case BrushCopyPasta, BrushLine, BrushSpline, BrushSurfaceSpline, BrushCatenary, BrushSweep:
			return b.runAction(ctx, b.PrimaryAction, true)
		}
	}
	return false
}

func (b *BrushTool) Secondary(ctx core.ToolContext) bool {
	if b.PrimaryAction == nil {
		return false
	}
	return b.runAction(ctx, b.PrimaryAction, false)
}

func (b *BrushTool) resolveTarget(ctx core.ToolContext) (plugin.BlockPos, bool) {
	r := b.Range
	if r <= 0 {
		r = 120
	}
	switch b.TargetMode {
	case "FORWARD_POINT_PITCH":
		dist := float64(min(r, 20))
		p := ctx.Player.EyePos().Add(ctx.Player.Location().Direction().Scale(dist)).Block()
		p.Y += b.TargetOffset
		return p, true
	case "TARGET_POINT_HEIGHT":
		if ctx.Clicked != nil {
			p := ctx.Clicked.Pos
			p.Y += b.TargetOffset
			return p, true
		}
	}
	if ctx.Clicked != nil && b.TargetMode != "TARGET_FACE_RANGE" {
		p := ctx.Clicked.Pos
		p.Y += b.TargetOffset
		return p, true
	}
	hit, ok := ctx.Player.RayTrace(float64(r), false)
	if !ok || hit.Block == nil {
		return plugin.BlockPos{}, false
	}
	p := hit.Block.Pos
	if b.TargetMode == "TARGET_FACE_RANGE" {
		p = p.Offset(hit.Face, 1)
	}
	p.Y += b.TargetOffset
	return p, true
}

func (b *BrushTool) runAction(ctx core.ToolContext, act *BrushSettings, isLeft bool) bool { //nolint:funlen // one switch over the brush kinds reads better whole
	if ctx.Sneaking && len(act.Points) > 0 {
		act.Points = nil
		ctx.WE.Msgs.InfoKey(ctx.Player, "fawe.worldedit.brush.brush.reset")
		return true
	}
	pos, ok := b.resolveTarget(ctx)
	if !ok {
		return false
	}

	es := ctx.WE.NewEditSession(ctx.World, ctx.Player, "/br "+string(act.Kind))
	es.Origin = pos
	if b.Mask != nil {
		if es.DestMask != nil {
			es.DestMask = block.AndMask(es.DestMask, b.Mask)
		} else {
			es.DestMask = b.Mask
		}
	}
	if b.SourceMask != nil {
		es.SourceMask = b.SourceMask
	}
	if b.Transform != nil {
		es.Transform = b.Transform
	}

	size := max(1, act.Size)
	if lim := ctx.WE.Config().LimitFor(ctx.Player); !lim.Unlimited && lim.MaxBrushRadius > 0 && size > lim.MaxBrushRadius {
		size = lim.MaxBrushRadius
	}
	pat := act.Pattern
	if pat == nil {
		stone, _ := block.ParseState("stone", false)
		pat = block.SinglePattern(stone)
	}

	switch act.Kind {
	case BrushSphere, BrushSet:
		r := float64(size)
		r2 := (r + 0.5) * (r + 0.5)
		inner2 := (r - 0.5) * (r - 0.5)
		for dy := -size; dy <= size; dy++ {
			for dz := -size; dz <= size; dz++ {
				for dx := -size; dx <= size; dx++ {
					d2 := float64(dx*dx + dy*dy + dz*dz)
					if d2 <= r2 && (!act.Hollow || d2 >= inner2) {
						es.SetPattern(plugin.BlockPos{X: pos.X + dx, Y: pos.Y + dy, Z: pos.Z + dz}, pat)
					}
				}
			}
		}

	case BrushCylinder:
		h := max(1, act.Height)
		r2 := (float64(size) + 0.5) * (float64(size) + 0.5)
		inner2 := (float64(size) - 0.5) * (float64(size) - 0.5)
		for dy := range h {
			for dz := -size; dz <= size; dz++ {
				for dx := -size; dx <= size; dx++ {
					d2 := float64(dx*dx + dz*dz)
					if d2 <= r2 && (!act.Hollow || d2 >= inner2) {
						es.SetPattern(plugin.BlockPos{X: pos.X + dx, Y: pos.Y + dy, Z: pos.Z + dz}, pat)
					}
				}
			}
		}

	case BrushCircle:
		r2 := (float64(size) + 0.5) * (float64(size) + 0.5)
		for dz := -size; dz <= size; dz++ {
			for dx := -size; dx <= size; dx++ {
				if float64(dx*dx+dz*dz) <= r2 {
					es.SetPattern(plugin.BlockPos{X: pos.X + dx, Y: pos.Y, Z: pos.Z + dz}, pat)
				}
			}
		}

	case BrushClipboard:
		holder := ctx.Session.Clipboard()
		if holder == nil || holder.Pick() == nil {
			ctx.WE.Msgs.ErrorKey(ctx.Player, "fawe.error.empty-clipboard", "clipboard brush")
			return true
		}
		PasteClipboard(es, holder.Pick(), holder.Transform, pos, act.ClipboardAir, false, true)

	case BrushSmooth, BrushSnowSmooth:
		iters := max(1, act.Iterations)
		SmoothRegion(es, region.NewCuboid(
			plugin.BlockPos{X: pos.X - size, Y: pos.Y - size, Z: pos.Z - size},
			plugin.BlockPos{X: pos.X + size, Y: pos.Y + size, Z: pos.Z + size},
		), iters, act.Kind == BrushSnowSmooth)

	case BrushExtinguish:
		air, _ := block.ParseState("air", false)
		r2 := (float64(size) + 0.5) * (float64(size) + 0.5)
		for dy := -size; dy <= size; dy++ {
			for dz := -size; dz <= size; dz++ {
				for dx := -size; dx <= size; dx++ {
					if float64(dx*dx+dy*dy+dz*dz) <= r2 {
						p := plugin.BlockPos{X: pos.X + dx, Y: pos.Y + dy, Z: pos.Z + dz}
						n := block.BlockName(ctx.World.BlockState(p))
						if n == "minecraft:fire" || n == "minecraft:soul_fire" {
							es.SetBlock(p, air)
						}
					}
				}
			}
		}

	case BrushGravity:
		ApplyGravity(es, region.NewCuboid(
			plugin.BlockPos{X: pos.X - size, Y: pos.Y - size, Z: pos.Z - size},
			plugin.BlockPos{X: pos.X + size, Y: pos.Y + size, Z: pos.Z + size},
		))

	case BrushButcher:
		box := plugin.Box{
			Min: plugin.Vec3{X: float64(pos.X - size), Y: float64(pos.Y - size), Z: float64(pos.Z - size)},
			Max: plugin.Vec3{X: float64(pos.X + size), Y: float64(pos.Y + size), Z: float64(pos.Z + size)},
		}
		killed := 0
		for _, ent := range ctx.World.EntitiesIn(box) {
			if _, isPlayer := ent.(plugin.Player); isPlayer {
				continue
			}
			ent.Remove()
			killed++
		}
		ctx.WE.Msgs.Info(ctx.Player, fmt.Sprintf("Killed %d entities.", killed))
		return true

	case BrushBlendBall, BrushMorph:
		BlendBall(es, pos, size)

	case BrushErode:
		ErodeTerrain(es, pos, size, max(1, act.ErodeFaces), max(1, act.ErodeIter), max(1, act.FillFaces), max(0, act.FillIter))

	case BrushPull:
		// Pull raises terrain toward the player's eye level
		PullTerrain(es, pos, size, act.YScale)

	case BrushRecursive:
		target := ctx.World.BlockState(pos).Block()
		pts := core.FloodFill(ctx.World, pos, func(_ plugin.BlockPos, st plugin.BlockState) bool {
			return st.Block() == target
		}, size, 16384)
		for _, p := range pts {
			es.SetPattern(p, pat)
		}

	case BrushLine:
		if isLeft {
			act.Points = []plugin.BlockPos{pos}
			ctx.WE.Msgs.InfoKey(ctx.Player, "fawe.worldedit.brush.brush.line.primary", region.FormatPos(pos))
			return true
		}
		if len(act.Points) == 0 {
			act.Points = []plugin.BlockPos{pos}
			ctx.WE.Msgs.InfoKey(ctx.Player, "fawe.worldedit.brush.brush.line.primary", region.FormatPos(pos))
			return true
		}
		pts := DrawLine(act.Points[len(act.Points)-1], pos, size/2)
		for _, p := range pts {
			es.SetPattern(p, pat)
		}
		act.Points = []plugin.BlockPos{pos}

	case BrushSpline, BrushSurfaceSpline:
		if isLeft {
			act.Points = append(act.Points, pos)
			ctx.WE.Msgs.Info(ctx.Player, fmt.Sprintf("Added spline point #%d at %s.", len(act.Points), region.FormatPos(pos)))
			return true
		}
		act.Points = append(act.Points, pos)
		if len(act.Points) < 2 {
			ctx.WE.Msgs.ErrorKey(ctx.Player, "fawe.worldedit.brush.brush.spline.secondary.error")
			return true
		}
		for _, p := range DrawCatmullRomSpline(act.Points, max(0, size/2)) {
			es.SetPattern(p, pat)
		}
		act.Points = nil
		ctx.WE.Msgs.InfoKey(ctx.Player, "fawe.worldedit.brush.brush.line.secondary")

	case BrushCatenary:
		if isLeft || len(act.Points) == 0 {
			act.Points = []plugin.BlockPos{pos}
			ctx.WE.Msgs.InfoKey(ctx.Player, "fawe.worldedit.brush.brush.catenary.direction", region.FormatPos(pos))
			return true
		}
		a, c := act.Points[0], pos
		sag := max(1.0, act.YScale)
		steps := max(16, int(math.Ceil(plugin.Vec3{X: float64(c.X - a.X), Y: float64(c.Y - a.Y), Z: float64(c.Z - a.Z)}.Len()*2)))
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)
			drop := 4 * sag * t * (1 - t)
			p := plugin.BlockPos{
				X: int(math.Round(float64(a.X) + float64(c.X-a.X)*t)),
				Y: int(math.Round(float64(a.Y) + float64(c.Y-a.Y)*t - drop)),
				Z: int(math.Round(float64(a.Z) + float64(c.Z-a.Z)*t)),
			}
			es.SetPattern(p, pat)
		}
		act.Points = nil

	case BrushSweep:
		if isLeft {
			act.Points = append(act.Points, pos)
			ctx.WE.Msgs.Info(ctx.Player, fmt.Sprintf("Added sweep point #%d at %s.", len(act.Points), region.FormatPos(pos)))
			return true
		}
		holder := ctx.Session.Clipboard()
		if holder == nil || holder.Primary() == nil {
			ctx.WE.Msgs.ErrorKey(ctx.Player, "fawe.error.empty-clipboard", "sweep brush")
			return true
		}
		act.Points = append(act.Points, pos)
		for _, pt := range DrawCatmullRomSpline(act.Points, 0) {
			PasteClipboard(es, holder.Primary(), holder.Transform, pt, true, false, false)
		}
		act.Points = nil

	case BrushRock:
		GenerateBlob(es, pos, float64(size), float64(size), float64(size), pat)

	case BrushShatter:
		lines := max(4, act.Iterations)
		for range lines {
			ang := rand.Float64() * 2 * math.Pi
			end := plugin.BlockPos{
				X: pos.X + int(math.Round(math.Cos(ang)*float64(size))),
				Y: pos.Y + rand.IntN(size*2+1) - size,
				Z: pos.Z + int(math.Round(math.Sin(ang)*float64(size))),
			}
			for _, p := range DrawLine(pos, end, 0) {
				es.SetPattern(p, pat)
			}
		}

	case BrushLayer:
		layers := act.Layers
		if len(layers) == 0 {
			layers = []block.Pattern{pat}
		}
		r2 := (float64(size) + 0.5) * (float64(size) + 0.5)
		for dz := -size; dz <= size; dz++ {
			for dx := -size; dx <= size; dx++ {
				if float64(dx*dx+dz*dz) > r2 {
					continue
				}
				topY := ctx.World.HighestBlockY(pos.X+dx, pos.Z+dz, plugin.WorldSurface)
				for i, lp := range layers {
					es.SetPattern(plugin.BlockPos{X: pos.X + dx, Y: topY - i, Z: pos.Z + dz}, lp)
				}
			}
		}

	case BrushScatter, BrushSplatter, BrushSurface, BrushStencil:
		density := act.Density
		if density <= 0 {
			density = 0.25
		}
		r2 := (float64(size) + 0.5) * (float64(size) + 0.5)
		for dz := -size; dz <= size; dz++ {
			for dx := -size; dx <= size; dx++ {
				if float64(dx*dx+dz*dz) > r2 || rand.Float64() > density {
					continue
				}
				topY := ctx.World.HighestBlockY(pos.X+dx, pos.Z+dz, plugin.WorldSurface)
				target := plugin.BlockPos{X: pos.X + dx, Y: topY, Z: pos.Z + dz}
				if act.Kind == BrushScatter {
					target.Y++
				}
				es.SetPattern(target, pat)
			}
		}

	case BrushPopulateSchematic:
		holder := ctx.Session.Clipboard()
		if holder == nil || holder.Pick() == nil {
			ctx.WE.Msgs.ErrorKey(ctx.Player, "fawe.error.empty-clipboard", "populateschematic brush")
			return true
		}
		density := act.Density
		if density <= 0 {
			density = 0.05
		}
		r2 := (float64(size) + 0.5) * (float64(size) + 0.5)
		for dz := -size; dz <= size; dz += max(2, size/2) {
			for dx := -size; dx <= size; dx += max(2, size/2) {
				if float64(dx*dx+dz*dz) > r2 || rand.Float64() > density {
					continue
				}
				topY := ctx.World.HighestBlockY(pos.X+dx, pos.Z+dz, plugin.WorldSurface)
				PasteClipboard(es, holder.Pick(), holder.Transform, plugin.BlockPos{X: pos.X + dx, Y: topY + 1, Z: pos.Z + dz}, true, false, false)
			}
		}

	case BrushHeight, BrushCliff, BrushFlatten:
		yscale := act.YScale
		if yscale == 0 {
			yscale = 1.0
		}
		r2 := (float64(size) + 0.5) * (float64(size) + 0.5)
		air, _ := block.ParseState("air", false)
		for dz := -size; dz <= size; dz++ {
			for dx := -size; dx <= size; dx++ {
				d2 := float64(dx*dx + dz*dz)
				if d2 > r2 {
					continue
				}
				topY := ctx.World.HighestBlockY(pos.X+dx, pos.Z+dz, plugin.WorldSurface)
				if topY < ctx.World.MinY() {
					continue
				}
				topState := ctx.World.BlockState(plugin.BlockPos{X: pos.X + dx, Y: topY, Z: pos.Z + dz})
				falloff := 1.0 - math.Sqrt(d2)/(float64(size)+0.5)
				var targetY int
				switch act.Kind {
				case BrushFlatten:
					targetY = int(math.Round(float64(topY) + float64(pos.Y-topY)*falloff))
				case BrushCliff:
					if falloff > 0.3 {
						targetY = pos.Y + int(math.Round(yscale*float64(size)*0.5))
					} else {
						targetY = topY
					}
				default:
					delta := int(math.Round(falloff * float64(size) * yscale))
					if isLeft {
						delta = -delta
					}
					targetY = topY + delta
				}
				if targetY > topY {
					for y := topY + 1; y <= targetY; y++ {
						es.SetBlock(plugin.BlockPos{X: pos.X + dx, Y: y, Z: pos.Z + dz}, topState)
					}
				} else if targetY < topY {
					for y := topY; y > targetY; y-- {
						es.SetBlock(plugin.BlockPos{X: pos.X + dx, Y: y, Z: pos.Z + dz}, air)
					}
				}
			}
		}

	case BrushCopyPasta:
		if isLeft {
			// Left click copies connected solid object around pos within size
			pts := core.FloodFill(ctx.World, pos, func(_ plugin.BlockPos, st plugin.BlockState) bool {
				return !block.IsAir(st)
			}, size, 16384)
			if len(pts) == 0 {
				return true
			}
			fz := region.NewFuzzy(pts)
			cb := CopyRegionToClipboard(ctx.World, fz, pos, false)
			ctx.Session.SetClipboard(storage.NewClipboardHolder(cb))
			ctx.WE.Msgs.InfoKey(ctx.Player, "fawe.worldedit.copy.command.copy", len(pts))
			return true
		}
		holder := ctx.Session.Clipboard()
		if holder == nil || holder.Primary() == nil {
			ctx.WE.Msgs.ErrorKey(ctx.Player, "fawe.error.empty-clipboard", "copypaste brush")
			return true
		}
		PasteClipboard(es, holder.Primary(), holder.Transform, pos, true, false, false)

	case BrushCommand, BrushScatterCommand:
		// Temporarily set selection to brush area and dispatch commands
		sel := ctx.Session.Selector(ctx.World)
		sel.SelectPrimary(plugin.BlockPos{X: pos.X - size, Y: pos.Y - size, Z: pos.Z - size}, 0)
		sel.SelectSecondary(plugin.BlockPos{X: pos.X + size, Y: pos.Y + size, Z: pos.Z + size}, 0)
		for _, cmd := range act.Commands {
			cmd = strings.TrimPrefix(strings.TrimSpace(cmd), "/")
			if cmd != "" {
				ctx.Player.RunCommand(cmd)
			}
		}
		return true

	case BrushDeform:
		if act.Expression != "" {
			if prog, err := expr.Compile(act.Expression); err == nil {
				DeformRegion(es, region.NewCuboid(
					plugin.BlockPos{X: pos.X - size, Y: pos.Y - size, Z: pos.Z - size},
					plugin.BlockPos{X: pos.X + size, Y: pos.Y + size, Z: pos.Z + size},
				), prog, 50*time.Millisecond)
			}
		}

	case BrushForest:
		density := act.Density
		if density <= 0 {
			density = 0.08
		}
		r2 := (float64(size) + 0.5) * (float64(size) + 0.5)
		for dz := -size; dz <= size; dz++ {
			for dx := -size; dx <= size; dx++ {
				if float64(dx*dx+dz*dz) <= r2 && rand.Float64() < density {
					topY := ctx.World.HighestBlockY(pos.X+dx, pos.Z+dz, plugin.WorldSurface)
					ctx.World.GenerateTree(act.TreeType, plugin.BlockPos{X: pos.X + dx, Y: topY + 1, Z: pos.Z + dz})
				}
			}
		}
		return true

	case BrushRaise, BrushLower:
		delta := 1
		if act.Kind == BrushLower {
			delta = -1
		}
		air, _ := block.ParseState("air", false)
		r2 := (float64(size) + 0.5) * (float64(size) + 0.5)
		for dz := -size; dz <= size; dz++ {
			for dx := -size; dx <= size; dx++ {
				if float64(dx*dx+dz*dz) > r2 {
					continue
				}
				topY := ctx.World.HighestBlockY(pos.X+dx, pos.Z+dz, plugin.WorldSurface)
				if delta > 0 {
					st := ctx.World.BlockState(plugin.BlockPos{X: pos.X + dx, Y: topY, Z: pos.Z + dz})
					es.SetBlock(plugin.BlockPos{X: pos.X + dx, Y: topY + 1, Z: pos.Z + dz}, st)
				} else {
					es.SetBlock(plugin.BlockPos{X: pos.X + dx, Y: topY, Z: pos.Z + dz}, air)
				}
			}
		}

	case BrushSnow:
		SimulateSnow(es, pos, size, true)

	case BrushBiome:
		ctx.WE.Msgs.Info(ctx.Player, "Biome brush: live biome mutation is not exposed by golem's World API (biomes are set at chunk generation time).")
		return true
	}

	_ = es.Commit()
	return true
}

// SavedBrush is the JSON representation used by /br savebrush and /br loadbrush.
type SavedBrush struct {
	Kind       BrushKind `json:"kind"`
	Pattern    string    `json:"pattern"`
	Size       int       `json:"size"`
	Height     int       `json:"height,omitempty"`
	Hollow     bool      `json:"hollow,omitempty"`
	Iterations int       `json:"iterations,omitempty"`
	Range      int       `json:"range,omitempty"`
	TargetMode string    `json:"target_mode,omitempty"`
}

// SaveBrushFile persists bt to <brushesDir>/<name>.json.
func SaveBrushFile(brushesDir, name string, bt *BrushTool) (string, error) {
	if err := os.MkdirAll(brushesDir, 0o755); err != nil {
		return "", err
	}
	clean := filepath.Base(name)
	if !strings.HasSuffix(clean, ".json") {
		clean += ".json"
	}
	act := bt.Active()
	sb := SavedBrush{
		Kind:       act.Kind,
		Pattern:    act.PatternStr,
		Size:       act.Size,
		Height:     act.Height,
		Hollow:     act.Hollow,
		Iterations: act.Iterations,
		Range:      bt.Range,
		TargetMode: bt.TargetMode,
	}
	data, err := json.MarshalIndent(sb, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(brushesDir, clean)
	return path, os.WriteFile(path, data, 0o644)
}

// LoadBrushFile restores a BrushTool from <brushesDir>/<name>.json.
func LoadBrushFile(brushesDir, name string, pctx block.ParseContext) (*BrushTool, error) {
	clean := filepath.Base(name)
	if !strings.HasSuffix(clean, ".json") {
		clean += ".json"
	}
	data, err := os.ReadFile(filepath.Join(brushesDir, clean))
	if err != nil {
		return nil, err
	}
	var sb SavedBrush
	if err := json.Unmarshal(data, &sb); err != nil {
		return nil, err
	}
	var pat block.Pattern
	if sb.Pattern != "" {
		pat, err = block.ParsePattern(sb.Pattern, pctx)
		if err != nil {
			return nil, err
		}
	}
	bt := NewBrushTool(&BrushSettings{
		Kind:       sb.Kind,
		Pattern:    pat,
		PatternStr: sb.Pattern,
		Size:       sb.Size,
		Height:     sb.Height,
		Hollow:     sb.Hollow,
		Iterations: sb.Iterations,
	})
	if sb.Range > 0 {
		bt.Range = sb.Range
	}
	if sb.TargetMode != "" {
		bt.TargetMode = sb.TargetMode
	}
	return bt, nil
}

// --- Shared World Operation Helpers (used by both brushes and region/clipboard/utility commands) ---

// CopyRegionToClipboard copies all blocks (and optionally entities/biomes) in r into a new Clipboard.
func CopyRegionToClipboard(w plugin.World, r region.Region, origin plugin.BlockPos, copyEntities bool) *storage.Clipboard {
	cb := storage.NewClipboard(r, origin)
	lo := r.Min()
	r.ForEach(func(pos plugin.BlockPos) bool {
		rx, ry, rz := pos.X-lo.X, pos.Y-lo.Y, pos.Z-lo.Z
		st := w.BlockState(pos)
		cb.SetRel(rx, ry, rz, st)
		if be := core.CaptureBlockEntityNBT(w, pos); be != nil {
			cb.BlockEntities[plugin.BlockPos{X: rx, Y: ry, Z: rz}] = be
		}
		return true
	})
	if copyEntities {
		hi := r.Max()
		box := plugin.Box{
			Min: plugin.Vec3{X: float64(lo.X), Y: float64(lo.Y), Z: float64(lo.Z)},
			Max: plugin.Vec3{X: float64(hi.X + 1), Y: float64(hi.Y + 1), Z: float64(hi.Z + 1)},
		}
		for _, ent := range w.EntitiesIn(box) {
			if _, isPlayer := ent.(plugin.Player); isPlayer {
				continue
			}
			rel := ent.Pos().Sub(plugin.Vec3{X: float64(lo.X), Y: float64(lo.Y), Z: float64(lo.Z)})
			loc := ent.Location()
			se := storage.StoredEntity{
				Type:  ent.Type().Name(),
				Pos:   rel,
				Yaw:   loc.Yaw,
				Pitch: loc.Pitch,
			}
			if mob, ok := ent.(plugin.Mob); ok {
				se.Data = mob.SavedData()
			}
			cb.Entities = append(cb.Entities, se)
		}
	}
	return cb
}

// PasteClipboard pastes cb with transform tr at targetPos into es.
func PasteClipboard(es *core.EditSession, cb *storage.Clipboard, tr storage.ClipboardTransform, targetPos plugin.BlockPos, skipAir, pasteEntities, useTransform bool) int64 {
	start := es.Changed()
	offset := cb.Offset()
	for ry := 0; ry < cb.Height; ry++ {
		for rz := 0; rz < cb.Length; rz++ {
			for rx := 0; rx < cb.Width; rx++ {
				st := cb.GetRel(rx, ry, rz)
				if skipAir && block.IsAir(st) {
					continue
				}
				rel := plugin.Vec3{
					X: float64(offset.X + rx),
					Y: float64(offset.Y + ry),
					Z: float64(offset.Z + rz),
				}
				if useTransform && !tr.IsIdentity() {
					rel = tr.TransformOffset(rel)
					st = tr.TransformBlock(st)
				}
				wp := plugin.BlockPos{
					X: targetPos.X + int(math.Round(rel.X)),
					Y: targetPos.Y + int(math.Round(rel.Y)),
					Z: targetPos.Z + int(math.Round(rel.Z)),
				}
				be := cb.BlockEntities[plugin.BlockPos{X: rx, Y: ry, Z: rz}]
				es.SetBlockWithNBT(wp, st, be)
			}
		}
	}
	if pasteEntities && len(cb.Entities) > 0 {
		for _, se := range cb.Entities {
			k, err := plugin.ParseKey(se.Type)
			if err != nil {
				continue
			}
			et, ok := es.WE.Server.Registries().EntityType(k)
			if !ok {
				continue
			}
			rel := plugin.Vec3{
				X: float64(offset.X) + se.Pos.X,
				Y: float64(offset.Y) + se.Pos.Y,
				Z: float64(offset.Z) + se.Pos.Z,
			}
			if useTransform && !tr.IsIdentity() {
				rel = tr.TransformOffset(rel)
			}
			loc := plugin.Location{
				World: es.World,
				Pos:   plugin.Vec3{X: float64(targetPos.X) + rel.X, Y: float64(targetPos.Y) + rel.Y, Z: float64(targetPos.Z) + rel.Z},
				Yaw:   se.Yaw,
				Pitch: se.Pitch,
			}
			_, _ = es.World.Spawn(et, loc, func(e plugin.Entity) {
				if mob, ok := e.(plugin.Mob); ok && se.Data != nil {
					mob.MergeData(se.Data)
				}
			})
		}
	}
	return es.Changed() - start
}

// SmoothRegion smooths terrain heightmap across r for iterations passes.
func SmoothRegion(es *core.EditSession, r region.Region, iterations int, snowLayers bool) int64 {
	lo, hi := r.Min(), r.Max()
	w, l := hi.X-lo.X+1, hi.Z-lo.Z+1
	heights := make([][]float64, w)
	topBlocks := make([][]plugin.BlockState, w)
	for x := range w {
		heights[x] = make([]float64, l)
		topBlocks[x] = make([]plugin.BlockState, l)
		for z := range l {
			h := float64(lo.Y - 1)
			var top plugin.BlockState
			for y := hi.Y; y >= lo.Y; y-- {
				st := es.World.BlockState(plugin.BlockPos{X: lo.X + x, Y: y, Z: lo.Z + z})
				if !block.IsAir(st) {
					h = float64(y)
					top = st
					break
				}
			}
			heights[x][z] = h
			topBlocks[x][z] = top
		}
	}
	for it := 0; it < max(1, iterations); it++ {
		next := make([][]float64, w)
		for x := range w {
			next[x] = make([]float64, l)
			for z := range l {
				sum, cnt := 0.0, 0.0
				for dx := -1; dx <= 1; dx++ {
					for dz := -1; dz <= 1; dz++ {
						nx, nz := x+dx, z+dz
						if nx >= 0 && nx < w && nz >= 0 && nz < l && heights[nx][nz] >= float64(lo.Y) {
							wt := 1.0
							if dx == 0 && dz == 0 {
								wt = 2.0
							}
							sum += heights[nx][nz] * wt
							cnt += wt
						}
					}
				}
				if cnt > 0 {
					next[x][z] = sum / cnt
				} else {
					next[x][z] = heights[x][z]
				}
			}
		}
		heights = next
	}
	air, _ := block.ParseState("air", false)
	start := es.Changed()
	for x := range w {
		for z := range l {
			if topBlocks[x][z] == 0 {
				continue
			}
			newH := int(math.Floor(heights[x][z]))
			frac := heights[x][z] - math.Floor(heights[x][z])
			for y := lo.Y; y <= hi.Y; y++ {
				p := plugin.BlockPos{X: lo.X + x, Y: y, Z: lo.Z + z}
				if !r.Contains(p) {
					continue
				}
				if y < newH {
					if block.IsAir(es.World.BlockState(p)) {
						es.SetBlock(p, topBlocks[x][z])
					}
				} else if y == newH {
					es.SetBlock(p, topBlocks[x][z])
				} else if y == newH+1 && snowLayers && frac >= 0.125 {
					layers := max(1, min(8, int(math.Round(frac*8))))
					if sn, err := block.ParseState(fmt.Sprintf("minecraft:snow[layers=%d]", layers), false); err == nil {
						es.SetBlock(p, sn)
					}
				} else {
					if !block.IsAir(es.World.BlockState(p)) {
						es.SetBlock(p, air)
					}
				}
			}
		}
	}
	return es.Changed() - start
}

// BlendBall replaces each block inside sphere(center, radius) with the most common neighbor block.
func BlendBall(es *core.EditSession, center plugin.BlockPos, radius int) int64 {
	r2 := (float64(radius) + 0.5) * (float64(radius) + 0.5)
	snap := make(map[plugin.BlockPos]plugin.BlockState)
	for dy := -radius - 1; dy <= radius+1; dy++ {
		for dz := -radius - 1; dz <= radius+1; dz++ {
			for dx := -radius - 1; dx <= radius+1; dx++ {
				p := plugin.BlockPos{X: center.X + dx, Y: center.Y + dy, Z: center.Z + dz}
				snap[p] = es.World.BlockState(p)
			}
		}
	}
	start := es.Changed()
	for dy := -radius; dy <= radius; dy++ {
		for dz := -radius; dz <= radius; dz++ {
			for dx := -radius; dx <= radius; dx++ {
				if float64(dx*dx+dy*dy+dz*dz) > r2 {
					continue
				}
				p := plugin.BlockPos{X: center.X + dx, Y: center.Y + dy, Z: center.Z + dz}
				freq := make(map[plugin.BlockState]int)
				var best plugin.BlockState
				bestCnt := 0
				for oy := -1; oy <= 1; oy++ {
					for oz := -1; oz <= 1; oz++ {
						for ox := -1; ox <= 1; ox++ {
							st := snap[plugin.BlockPos{X: p.X + ox, Y: p.Y + oy, Z: p.Z + oz}]
							freq[st]++
							if freq[st] > bestCnt {
								bestCnt = freq[st]
								best = st
							}
						}
					}
				}
				if bestCnt >= 14 && best != snap[p] {
					es.SetBlock(p, best)
				}
			}
		}
	}
	return es.Changed() - start
}

// ErodeTerrain erodes blocks exposed to >= erodeFaces air neighbors and fills pockets.
func ErodeTerrain(es *core.EditSession, center plugin.BlockPos, radius, erodeFaces, erodeIter, fillFaces, fillIter int) {
	air, _ := block.ParseState("air", false)
	dirs := [6]plugin.BlockPos{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}, {Z: 1}, {Z: -1}}
	r2 := (float64(radius) + 0.5) * (float64(radius) + 0.5)
	for range erodeIter {
		var toRemove []plugin.BlockPos
		for dy := -radius; dy <= radius; dy++ {
			for dz := -radius; dz <= radius; dz++ {
				for dx := -radius; dx <= radius; dx++ {
					if float64(dx*dx+dy*dy+dz*dz) > r2 {
						continue
					}
					p := plugin.BlockPos{X: center.X + dx, Y: center.Y + dy, Z: center.Z + dz}
					if block.IsAir(es.World.BlockState(p)) {
						continue
					}
					airNeighbors := 0
					for _, d := range dirs {
						if block.IsAir(es.World.BlockState(plugin.BlockPos{X: p.X + d.X, Y: p.Y + d.Y, Z: p.Z + d.Z})) {
							airNeighbors++
						}
					}
					if airNeighbors >= erodeFaces {
						toRemove = append(toRemove, p)
					}
				}
			}
		}
		for _, p := range toRemove {
			es.SetBlock(p, air)
		}
	}
	for range fillIter {
		type fill struct {
			p  plugin.BlockPos
			st plugin.BlockState
		}
		var toFill []fill
		for dy := -radius; dy <= radius; dy++ {
			for dz := -radius; dz <= radius; dz++ {
				for dx := -radius; dx <= radius; dx++ {
					if float64(dx*dx+dy*dy+dz*dz) > r2 {
						continue
					}
					p := plugin.BlockPos{X: center.X + dx, Y: center.Y + dy, Z: center.Z + dz}
					if !block.IsAir(es.World.BlockState(p)) {
						continue
					}
					solidNeighbors := 0
					var sample plugin.BlockState
					for _, d := range dirs {
						nst := es.World.BlockState(plugin.BlockPos{X: p.X + d.X, Y: p.Y + d.Y, Z: p.Z + d.Z})
						if !block.IsAir(nst) {
							solidNeighbors++
							sample = nst
						}
					}
					if solidNeighbors >= fillFaces && sample != 0 {
						toFill = append(toFill, fill{p, sample})
					}
				}
			}
		}
		for _, f := range toFill {
			es.SetBlock(f.p, f.st)
		}
	}
}

// PullTerrain lifts columns inside sphere toward center.Y + size*yscale.
func PullTerrain(es *core.EditSession, center plugin.BlockPos, radius int, yscale float64) {
	if yscale == 0 {
		yscale = 0.5
	}
	r2 := (float64(radius) + 0.5) * (float64(radius) + 0.5)
	for dz := -radius; dz <= radius; dz++ {
		for dx := -radius; dx <= radius; dx++ {
			d2 := float64(dx*dx + dz*dz)
			if d2 > r2 {
				continue
			}
			lift := int(math.Round((1.0 - math.Sqrt(d2)/(float64(radius)+0.5)) * float64(radius) * yscale))
			if lift <= 0 {
				continue
			}
			topY := es.World.HighestBlockY(center.X+dx, center.Z+dz, plugin.WorldSurface)
			st := es.World.BlockState(plugin.BlockPos{X: center.X + dx, Y: topY, Z: center.Z + dz})
			for i := 1; i <= lift; i++ {
				es.SetBlock(plugin.BlockPos{X: center.X + dx, Y: topY + i, Z: center.Z + dz}, st)
			}
		}
	}
}

// ApplyGravity drops floating solid blocks downward inside r.
func ApplyGravity(es *core.EditSession, r region.Region) int64 {
	lo, hi := r.Min(), r.Max()
	air, _ := block.ParseState("air", false)
	start := es.Changed()
	for x := lo.X; x <= hi.X; x++ {
		for z := lo.Z; z <= hi.Z; z++ {
			var col []plugin.BlockState
			for y := lo.Y; y <= hi.Y; y++ {
				p := plugin.BlockPos{X: x, Y: y, Z: z}
				if !r.Contains(p) {
					continue
				}
				st := es.World.BlockState(p)
				if !block.IsAir(st) {
					col = append(col, st)
				}
			}
			for i, y := 0, lo.Y; y <= hi.Y; y++ {
				p := plugin.BlockPos{X: x, Y: y, Z: z}
				if !r.Contains(p) {
					continue
				}
				if i < len(col) {
					es.SetBlock(p, col[i])
					i++
				} else {
					es.SetBlock(p, air)
				}
			}
		}
	}
	return es.Changed() - start
}

// DeformRegion applies a coordinate-space WorldEdit expression to deform blocks inside r.
func DeformRegion(es *core.EditSession, r region.Region, prog *expr.Program, timeout time.Duration) int64 {
	lo, hi := r.Min(), r.Max()
	cx := float64(lo.X+hi.X) / 2.0
	cy := float64(lo.Y+hi.Y) / 2.0
	cz := float64(lo.Z+hi.Z) / 2.0
	sx := max(1.0, float64(hi.X-lo.X)/2.0)
	sy := max(1.0, float64(hi.Y-lo.Y)/2.0)
	sz := max(1.0, float64(hi.Z-lo.Z)/2.0)

	snap := make(map[plugin.BlockPos]plugin.BlockState)
	r.ForEach(func(p plugin.BlockPos) bool {
		snap[p] = es.World.BlockState(p)
		return true
	})

	start := es.Changed()
	r.ForEach(func(p plugin.BlockPos) bool {
		env := expr.NewEnv(timeout)
		env.Vars["x"] = (float64(p.X) - cx) / sx
		env.Vars["y"] = (float64(p.Y) - cy) / sy
		env.Vars["z"] = (float64(p.Z) - cz) / sz
		env.Perlin = block.Perlin3D
		env.Simplex = block.Simplex3D
		env.Voronoi = block.Voronoi3D
		env.RidgedMulti = block.RidgedMulti3D
		if _, err := prog.Eval(env); err != nil {
			return true
		}
		src := plugin.BlockPos{
			X: int(math.Round(env.Vars["x"]*sx + cx)),
			Y: int(math.Round(env.Vars["y"]*sy + cy)),
			Z: int(math.Round(env.Vars["z"]*sz + cz)),
		}
		st, ok := snap[src]
		if !ok {
			st = es.World.BlockState(src)
		}
		es.SetBlock(p, st)
		return true
	})
	return es.Changed() - start
}

// GenerateBlob creates a noise-distorted rock/blob sphere around center.
func GenerateBlob(es *core.EditSession, center plugin.BlockPos, rx, ry, rz float64, pat block.Pattern) int64 {
	seed := rand.Int64()
	irx, iry, irz := int(math.Ceil(rx*1.4)), int(math.Ceil(ry*1.4)), int(math.Ceil(rz*1.4))
	start := es.Changed()
	for dy := -iry; dy <= iry; dy++ {
		for dz := -irz; dz <= irz; dz++ {
			for dx := -irx; dx <= irx; dx++ {
				nx := float64(dx) / (rx + 0.5)
				ny := float64(dy) / (ry + 0.5)
				nz := float64(dz) / (rz + 0.5)
				noise := block.Simplex3D(seed, float64(center.X+dx), float64(center.Y+dy), float64(center.Z+dz), 0.15, 2)
				if nx*nx+ny*ny+nz*nz <= 0.65+noise*0.7 {
					es.SetPattern(plugin.BlockPos{X: center.X + dx, Y: center.Y + dy, Z: center.Z + dz}, pat)
				}
			}
		}
	}
	return es.Changed() - start
}

// SimulateSnow places snow layers on exposed solid blocks and freezes exposed water within radius.
func SimulateSnow(es *core.EditSession, center plugin.BlockPos, radius int, stack bool) int64 {
	snow1, _ := block.ParseState("minecraft:snow[layers=1]", false)
	ice, _ := block.ParseState("minecraft:ice", false)
	r2 := float64(radius+1) * float64(radius+1)
	start := es.Changed()
	for dz := -radius; dz <= radius; dz++ {
		for dx := -radius; dx <= radius; dx++ {
			if float64(dx*dx+dz*dz) > r2 {
				continue
			}
			x, z := center.X+dx, center.Z+dz
			topY := es.World.HighestBlockY(x, z, plugin.WorldSurface)
			if topY < es.World.MinY() {
				continue
			}
			topPos := plugin.BlockPos{X: x, Y: topY, Z: z}
			topSt := es.World.BlockState(topPos)
			name := block.BlockName(topSt)
			if name == "minecraft:water" {
				es.SetBlock(topPos, ice)
			} else if name == "minecraft:snow" && stack {
				props := registry.StateProps(uint16(topSt))
				layers := 1
				if n, err := strconv.Atoi(props["layers"]); err == nil {
					layers = n
				}
				if layers < 8 {
					if next, err := block.ParseState(fmt.Sprintf("minecraft:snow[layers=%d]", layers+1), false); err == nil {
						es.SetBlock(topPos, next)
					}
				}
			} else if block.IsSolid(topSt) {
				above := plugin.BlockPos{X: x, Y: topY + 1, Z: z}
				if block.IsAir(es.World.BlockState(above)) {
					es.SetBlock(above, snow1)
				}
			}
		}
	}
	return es.Changed() - start
}
