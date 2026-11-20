// Package tools implements WorldEdit and FastAsyncWorldEdit interactive tools,
// wands, super-pickaxe, and brushes.
package tools

import (
	"fmt"
	"maps"
	"math"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/core"
	"github.com/golem-mc/worldedit/region"
)

// InstallHandlers hooks default selection wand, navigation wand, and super-pickaxe handlers into we.
func InstallHandlers(we *core.WorldEdit) {
	we.WandHandler = handleSelectionWand
	we.NavWandHandler = handleNavigationWand
	we.SuperPickHandler = handleSuperPickaxe
}

func handleSelectionWand(ctx core.ToolContext, leftClick bool) bool {
	if ctx.Clicked == nil {
		return false
	}
	return SelectPoint(ctx.WE, ctx.Player, ctx.Session, ctx.World, ctx.Clicked.Pos, leftClick)
}

// SelectPoint selects primary (leftClick=true) or secondary (leftClick=false) point on the player's selector.
func SelectPoint(we *core.WorldEdit, p plugin.Player, sess *core.Session, w plugin.World, pos plugin.BlockPos, primary bool) bool {
	sel := sess.Selector(w)
	cfg := we.Config()
	maxPts := cfg.WorldEdit.Limits.MaxPolygonalPoints.Maximum
	if sel.Type() == region.SelectorConvex {
		maxPts = cfg.WorldEdit.Limits.MaxPolyhedronPoints.Maximum
	}

	// Fuzzy selector performs flood-fill expansion from the clicked block.
	if fz, ok := sel.(*region.FuzzySelector); ok {
		target := w.BlockState(pos)
		pts := core.FloodFill(w, pos, func(_ plugin.BlockPos, st plugin.BlockState) bool {
			return st.Block() == target.Block()
		}, 64, 50000)
		fz.SetFloodPositions(pos, pts, primary)
		if primary {
			we.Msgs.InfoKey(p, "fawe.worldedit.selector.selector.fuzzy.pos1", region.FormatPos(pos), fmt.Sprintf("(%d)", len(pts)))
		} else {
			we.Msgs.InfoKey(p, "fawe.worldedit.selector.selector.fuzzy.pos2", region.FormatPos(pos), fmt.Sprintf("(%d)", len(pts)))
		}
		we.DispatchCUI(p, sess)
		we.Server.Events().Fire(&core.RegionChangeEvent{Player: p, Selector: sel})
		return true
	}

	var msg string
	if primary {
		msg, _ = sel.SelectPrimary(pos, maxPts)
	} else {
		msg, _ = sel.SelectSecondary(pos, maxPts)
	}
	we.Msgs.Info(p, msg)
	we.DispatchCUI(p, sess)
	we.Server.Events().Fire(&core.RegionChangeEvent{Player: p, Selector: sel})
	return true
}

func handleNavigationWand(ctx core.ToolContext, leftClick bool) bool {
	maxDist := ctx.WE.Config().WorldEdit.NavigationWand.MaxDistance
	if leftClick {
		return JumpToTarget(ctx.WE, ctx.Player, maxDist)
	}
	return PassThruWall(ctx.WE, ctx.Player, maxDist)
}

// JumpToTarget teleports player atop the block they are looking at within maxDist.
func JumpToTarget(we *core.WorldEdit, p plugin.Player, maxDist int) bool {
	if maxDist <= 0 {
		maxDist = 100
	}
	blk, ok := p.TargetBlock(maxDist)
	if !ok {
		we.Msgs.Error(p, "No block in sight!")
		return true
	}
	w := p.World()
	y := blk.Pos.Y + 1
	for y < w.MinY()+w.Height()-1 && (!block.IsAir(w.BlockState(plugin.BlockPos{X: blk.Pos.X, Y: y, Z: blk.Pos.Z})) ||
		!block.IsAir(w.BlockState(plugin.BlockPos{X: blk.Pos.X, Y: y + 1, Z: blk.Pos.Z}))) {
		y++
	}
	loc := p.Location()
	loc.Pos = plugin.Vec3{X: float64(blk.Pos.X) + 0.5, Y: float64(y), Z: float64(blk.Pos.Z) + 0.5}
	p.Teleport(loc, plugin.TeleportCommand)
	we.Msgs.Info(p, "Poof!")
	return true
}

// PassThruWall teleports player through the wall they are looking at within maxDist.
func PassThruWall(we *core.WorldEdit, p plugin.Player, maxDist int) bool {
	if maxDist <= 0 {
		maxDist = 100
	}
	w := p.World()
	dir := p.Location().Direction()
	eye := p.EyePos()
	hitSolid := false
	for step := 1.0; step <= float64(maxDist); step += 0.5 {
		pt := eye.Add(dir.Scale(step))
		bp := pt.Block()
		st := w.BlockState(bp)
		if !block.IsAir(st) {
			hitSolid = true
			continue
		}
		if hitSolid {
			// Found open space past the wall; ensure headroom
			above := w.BlockState(plugin.BlockPos{X: bp.X, Y: bp.Y + 1, Z: bp.Z})
			if block.IsAir(above) {
				loc := p.Location()
				loc.Pos = plugin.Vec3{X: float64(bp.X) + 0.5, Y: float64(bp.Y), Z: float64(bp.Z) + 0.5}
				p.Teleport(loc, plugin.TeleportCommand)
				we.Msgs.Info(p, "Whoosh!")
				return true
			}
		}
	}
	we.Msgs.Error(p, "No free spot ahead of you found.")
	return true
}

func handleSuperPickaxe(we *core.WorldEdit, sess *core.Session, e *plugin.BlockBreakEvent) bool {
	enabled, mode, radius := sess.SuperPickaxe()
	if !enabled {
		return false
	}
	p := e.Player
	w := p.World()
	origin := e.Block.Pos
	air, _ := block.ParseState("air", false)
	es := we.NewEditSession(w, p, "/superpickaxe")
	switch mode {
	case core.SuperPickSingle:
		es.SetBlock(origin, air)
	case core.SuperPickArea:
		for dx := -radius; dx <= radius; dx++ {
			for dy := -radius; dy <= radius; dy++ {
				for dz := -radius; dz <= radius; dz++ {
					pos := plugin.BlockPos{X: origin.X + dx, Y: origin.Y + dy, Z: origin.Z + dz}
					if !block.IsAir(w.BlockState(pos)) {
						es.SetBlock(pos, air)
					}
				}
			}
		}
	case core.SuperPickRecursive:
		target := e.State.Block()
		pts := core.FloodFill(w, origin, func(_ plugin.BlockPos, st plugin.BlockState) bool {
			return st.Block() == target
		}, radius, 4096)
		for _, pos := range pts {
			es.SetBlock(pos, air)
		}
	}
	_ = es.Commit()
	return true
}

// --- Bound Tools ---

// SelectionWandTool acts as the selection wand on any bound item.
type SelectionWandTool struct{}

func (SelectionWandTool) Name() string                        { return "selwand" }
func (SelectionWandTool) Primary(ctx core.ToolContext) bool   { return handleSelectionWand(ctx, true) }
func (SelectionWandTool) Secondary(ctx core.ToolContext) bool { return handleSelectionWand(ctx, false) }

// NavigationWandTool acts as the navigation wand on any bound item.
type NavigationWandTool struct{}

func (NavigationWandTool) Name() string                      { return "navwand" }
func (NavigationWandTool) Primary(ctx core.ToolContext) bool { return handleNavigationWand(ctx, true) }
func (NavigationWandTool) Secondary(ctx core.ToolContext) bool {
	return handleNavigationWand(ctx, false)
}

// InfoTool displays block info on right-click.
type InfoTool struct{}

func (InfoTool) Name() string                  { return "info" }
func (InfoTool) Primary(core.ToolContext) bool { return false }
func (InfoTool) Secondary(ctx core.ToolContext) bool {
	if ctx.Clicked == nil {
		return false
	}
	pos := ctx.Clicked.Pos
	st := ctx.World.BlockState(pos)
	sky, blk := ctx.World.Light(pos)
	biome := ctx.World.Biome(pos).Name()
	ctx.WE.Msgs.Info(ctx.Player, fmt.Sprintf("@%s %s (light: %d/%d, biome: %s)",
		region.FormatPos(pos), block.FormatState(st), sky, blk, biome))
	return true
}

// InspectTool queries FAWE's SQLite rollback database for edits touching the clicked block.
type InspectTool struct{}

func (InspectTool) Name() string { return "inspect" }
func (InspectTool) Primary(ctx core.ToolContext) bool {
	return inspectAt(ctx)
}
func (InspectTool) Secondary(ctx core.ToolContext) bool {
	return inspectAt(ctx)
}

func inspectAt(ctx core.ToolContext) bool {
	if ctx.Clicked == nil {
		return false
	}
	pos := ctx.Clicked.Pos
	rdb := ctx.WE.RollbackDB(ctx.World.Key().String())
	if rdb == nil {
		ctx.WE.Msgs.Error(ctx.Player, "Rollback database is disabled.")
		return true
	}
	edits, err := rdb.QueryEdits(nil, time.Time{}, pos, pos, false, 10)
	if err != nil {
		ctx.WE.Msgs.Error(ctx.Player, "Failed to query rollback database: "+err.Error())
		return true
	}
	if len(edits) == 0 {
		ctx.WE.Msgs.Info(ctx.Player, fmt.Sprintf("No recorded edits at %s.", region.FormatPos(pos)))
		return true
	}
	for _, ed := range edits {
		ago := time.Since(ed.Time).Truncate(time.Second)
		who := ed.Player.String()
		if op, ok := ctx.WE.Server.OfflinePlayer(ed.Player); ok && op.Name() != "" {
			who = op.Name()
		}
		ctx.WE.Msgs.Info(ctx.Player, fmt.Sprintf("#%d %s (%s ago) %d blocks: %s", ed.ID, who, ago, ed.Size, ed.Command))
	}
	ctx.WE.Msgs.InfoKey(ctx.Player, "fawe.worldedit.tool.tool.inspect.info.footer", len(edits))
	return true
}

// TreeTool plants trees of TreeType on right-click.
type TreeTool struct {
	TreeType plugin.TreeType
	TypeName string
}

func (t *TreeTool) Name() string                  { return "tree:" + t.TypeName }
func (t *TreeTool) Primary(core.ToolContext) bool { return false }
func (t *TreeTool) Secondary(ctx core.ToolContext) bool {
	if ctx.Clicked == nil {
		return false
	}
	pos := ctx.Clicked.Pos.Offset(plugin.Up, 1)
	if !ctx.World.GenerateTree(t.TreeType, pos) {
		ctx.WE.Msgs.Error(ctx.Player, "A tree can't go there.")
	}
	return true
}

// ReplacerTool right-clicks to replace a block with Pattern, left-clicks to copy clicked block into Pattern.
type ReplacerTool struct {
	Pattern block.Pattern
}

func (t *ReplacerTool) Name() string { return "repl" }
func (t *ReplacerTool) Primary(ctx core.ToolContext) bool {
	if ctx.Clicked == nil {
		return false
	}
	st := ctx.World.BlockState(ctx.Clicked.Pos)
	t.Pattern = block.SinglePattern(st)
	ctx.WE.Msgs.Info(ctx.Player, "Block replacer switched to: "+block.FormatState(st))
	return true
}
func (t *ReplacerTool) Secondary(ctx core.ToolContext) bool {
	if ctx.Clicked == nil || t.Pattern == nil {
		return false
	}
	es := ctx.WE.NewEditSession(ctx.World, ctx.Player, "/repl")
	es.SetPattern(ctx.Clicked.Pos, t.Pattern)
	_ = es.Commit()
	return true
}

// DataCyclerTool cycles block state properties on right-click and picks property on left-click.
type DataCyclerTool struct {
	propIdx int
}

func (t *DataCyclerTool) Name() string { return "cycler" }
func (t *DataCyclerTool) Primary(ctx core.ToolContext) bool {
	if ctx.Clicked == nil {
		return false
	}
	st := ctx.World.BlockState(ctx.Clicked.Pos)
	names := registry.BlockPropertyNames(block.BlockName(st))
	if len(names) == 0 {
		ctx.WE.Msgs.Info(ctx.Player, "That block has no properties to cycle.")
		return true
	}
	t.propIdx = (t.propIdx + 1) % len(names)
	ctx.WE.Msgs.Info(ctx.Player, "Selected property: "+names[t.propIdx])
	return true
}
func (t *DataCyclerTool) Secondary(ctx core.ToolContext) bool {
	if ctx.Clicked == nil {
		return false
	}
	st := ctx.World.BlockState(ctx.Clicked.Pos)
	bname := block.BlockName(st)
	names := registry.BlockPropertyNames(bname)
	if len(names) == 0 {
		ctx.WE.Msgs.Info(ctx.Player, "That block has no properties to cycle.")
		return true
	}
	prop := names[t.propIdx%len(names)]
	order, ok := registry.BlockPropertyOrder(bname, prop)
	if !ok || len(order) == 0 {
		return true
	}
	curProps := registry.StateProps(uint16(st))
	nextVal := order[0]
	for i, v := range order {
		if v == curProps[prop] {
			nextVal = order[(i+1)%len(order)]
			break
		}
	}
	merged := make(map[string]string, len(curProps))
	maps.Copy(merged, curProps)
	merged[prop] = nextVal
	if id, ok := registry.FindState(bname, merged); ok {
		es := ctx.WE.NewEditSession(ctx.World, ctx.Player, "/cycler")
		es.SetBlock(ctx.Clicked.Pos, plugin.BlockState(id))
		_ = es.Commit()
		ctx.WE.Msgs.Info(ctx.Player, fmt.Sprintf("Set %s=%s", prop, nextVal))
	}
	return true
}

// FloodFillTool flood-fills connected matching blocks within Range.
type FloodFillTool struct {
	Pattern block.Pattern
	Range   int
}

func (t *FloodFillTool) Name() string                  { return "floodfill" }
func (t *FloodFillTool) Primary(core.ToolContext) bool { return false }
func (t *FloodFillTool) Secondary(ctx core.ToolContext) bool {
	if ctx.Clicked == nil || t.Pattern == nil {
		return false
	}
	target := ctx.World.BlockState(ctx.Clicked.Pos).Block()
	pts := core.FloodFill(ctx.World, ctx.Clicked.Pos, func(_ plugin.BlockPos, st plugin.BlockState) bool {
		return st.Block() == target
	}, t.Range, 50000)
	es := ctx.WE.NewEditSession(ctx.World, ctx.Player, "/floodfill")
	for _, p := range pts {
		es.SetPattern(p, t.Pattern)
	}
	_ = es.Commit()
	ctx.WE.Msgs.Info(ctx.Player, fmt.Sprintf("Flood-filled %d blocks.", es.Changed()))
	return true
}

// DelTreeTool removes floating trees (connected logs & leaves).
type DelTreeTool struct{}

func (DelTreeTool) Name() string                  { return "deltree" }
func (DelTreeTool) Primary(core.ToolContext) bool { return false }
func (DelTreeTool) Secondary(ctx core.ToolContext) bool {
	if ctx.Clicked == nil {
		return false
	}
	isTreeBlock := func(st plugin.BlockState) bool {
		n := block.BlockName(st)
		return strings.Contains(n, "_log") || strings.Contains(n, "_wood") ||
			strings.Contains(n, "_leaves") || strings.Contains(n, "_stem") ||
			strings.Contains(n, "hyphae") || strings.Contains(n, "wart_block")
	}
	if !isTreeBlock(ctx.World.BlockState(ctx.Clicked.Pos)) {
		ctx.WE.Msgs.Error(ctx.Player, "That is not a floating tree.")
		return true
	}
	pts := core.FloodFill(ctx.World, ctx.Clicked.Pos, func(_ plugin.BlockPos, st plugin.BlockState) bool {
		return isTreeBlock(st)
	}, 32, 8192)
	air, _ := block.ParseState("air", false)
	es := ctx.WE.NewEditSession(ctx.World, ctx.Player, "/deltree")
	for _, p := range pts {
		es.SetBlock(p, air)
	}
	_ = es.Commit()
	ctx.WE.Msgs.Info(ctx.Player, fmt.Sprintf("Removed %d tree blocks.", es.Changed()))
	return true
}

// FarWandTool selects points at a distance via raytrace.
type FarWandTool struct {
	Range int
}

func (t *FarWandTool) Name() string { return "farwand" }
func (t *FarWandTool) Primary(ctx core.ToolContext) bool {
	return t.pick(ctx, true)
}
func (t *FarWandTool) Secondary(ctx core.ToolContext) bool {
	return t.pick(ctx, false)
}
func (t *FarWandTool) pick(ctx core.ToolContext, primary bool) bool {
	r := t.Range
	if r <= 0 {
		r = 100
	}
	blk, ok := ctx.Player.TargetBlock(r)
	if !ok {
		ctx.WE.Msgs.Error(ctx.Player, "No block in sight!")
		return true
	}
	return SelectPoint(ctx.WE, ctx.Player, ctx.Session, ctx.World, blk.Pos, primary)
}

// LongRangeBuildTool places PrimaryPat on left-click and SecondaryPat on right-click at range.
type LongRangeBuildTool struct {
	PrimaryPat   block.Pattern
	SecondaryPat block.Pattern
	Range        int
}

func (t *LongRangeBuildTool) Name() string { return "lrbuild" }
func (t *LongRangeBuildTool) Primary(ctx core.ToolContext) bool {
	return t.place(ctx, t.PrimaryPat)
}
func (t *LongRangeBuildTool) Secondary(ctx core.ToolContext) bool {
	return t.place(ctx, t.SecondaryPat)
}
func (t *LongRangeBuildTool) place(ctx core.ToolContext, pat block.Pattern) bool {
	if pat == nil {
		return false
	}
	r := t.Range
	if r <= 0 {
		r = 100
	}
	hit, ok := ctx.Player.RayTrace(float64(r), false)
	if !ok || hit.Block == nil {
		ctx.WE.Msgs.Error(ctx.Player, "No block in sight!")
		return true
	}
	target := hit.Block.Pos.Offset(hit.Face, 1)
	if block.IsAir(pat.Apply(nil, target)) {
		target = hit.Block.Pos
	}
	es := ctx.WE.NewEditSession(ctx.World, ctx.Player, "/lrbuild")
	es.SetPattern(target, pat)
	_ = es.Commit()
	return true
}

// StackerTool stacks the clicked block Count times along the clicked face.
type StackerTool struct {
	Count int
	Mask  block.Mask
}

func (t *StackerTool) Name() string                  { return "stacker" }
func (t *StackerTool) Primary(core.ToolContext) bool { return false }
func (t *StackerTool) Secondary(ctx core.ToolContext) bool {
	if ctx.Clicked == nil {
		return false
	}
	st := ctx.World.BlockState(ctx.Clicked.Pos)
	es := ctx.WE.NewEditSession(ctx.World, ctx.Player, "/stacker")
	cur := ctx.Clicked.Pos
	for i := 0; i < t.Count; i++ {
		cur = cur.Offset(ctx.Face, 1)
		if t.Mask != nil && !t.Mask.Test(es, cur) {
			break
		}
		es.SetBlock(cur, st)
	}
	_ = es.Commit()
	return true
}

// ParseTreeType resolves WorldEdit tree type names to plugin.TreeType.
func ParseTreeType(name string) (plugin.TreeType, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "tree", "oak":
		return plugin.TreeOak, true
	case "big", "bigtree", "largeoak", "fancy_oak":
		return plugin.TreeFancyOak, true
	case "redwood", "spruce":
		return plugin.TreeSpruce, true
	case "tallredwood", "tallspruce", "pine":
		return plugin.TreePine, true
	case "megaredwood", "megaspruce":
		return plugin.TreeMegaSpruce, true
	case "megapine":
		return plugin.TreeMegaPine, true
	case "birch":
		return plugin.TreeBirch, true
	case "tallbirch":
		return plugin.TreeTallBirch, true
	case "jungle":
		return plugin.TreeJungle, true
	case "megajungle":
		return plugin.TreeMegaJungle, true
	case "junglebush", "bush":
		return plugin.TreeJungleBush, true
	case "acacia":
		return plugin.TreeAcacia, true
	case "darkoak", "dark_oak":
		return plugin.TreeDarkOak, true
	case "swamp":
		return plugin.TreeSwamp, true
	case "mangrove":
		return plugin.TreeMangrove, true
	case "tallmangrove":
		return plugin.TreeTallMangrove, true
	case "cherry":
		return plugin.TreeCherry, true
	case "azalea":
		return plugin.TreeAzalea, true
	case "redmushroom", "red_mushroom":
		return plugin.TreeRedMushroom, true
	case "brownmushroom", "brown_mushroom":
		return plugin.TreeBrownMushroom, true
	case "chorus", "chorus_plant":
		return plugin.TreeChorusPlant, true
	case "crimson", "crimson_fungus":
		return plugin.TreeCrimsonFungus, true
	case "warped", "warped_fungus":
		return plugin.TreeWarpedFungus, true
	case "pale_oak", "paleoak":
		return plugin.TreePaleOak, true
	default:
		return 0, false
	}
}

// DrawLine returns all block positions on a 3D Bresenham-style line from a to b with optional thickness.
func DrawLine(a, b plugin.BlockPos, thickness int) []plugin.BlockPos {
	dx := float64(b.X - a.X)
	dy := float64(b.Y - a.Y)
	dz := float64(b.Z - a.Z)
	steps := int(math.Ceil(max(math.Abs(dx), max(math.Abs(dy), math.Abs(dz)))))
	seen := make(map[plugin.BlockPos]struct{})
	var out []plugin.BlockPos
	add := func(center plugin.BlockPos) {
		if thickness <= 0 {
			if _, ok := seen[center]; !ok {
				seen[center] = struct{}{}
				out = append(out, center)
			}
			return
		}
		r2 := float64(thickness) + 0.5
		r2 *= r2
		for ox := -thickness; ox <= thickness; ox++ {
			for oy := -thickness; oy <= thickness; oy++ {
				for oz := -thickness; oz <= thickness; oz++ {
					if float64(ox*ox+oy*oy+oz*oz) <= r2 {
						p := plugin.BlockPos{X: center.X + ox, Y: center.Y + oy, Z: center.Z + oz}
						if _, ok := seen[p]; !ok {
							seen[p] = struct{}{}
							out = append(out, p)
						}
					}
				}
			}
		}
	}
	if steps == 0 {
		add(a)
		return out
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		add(plugin.BlockPos{
			X: int(math.Round(float64(a.X) + dx*t)),
			Y: int(math.Round(float64(a.Y) + dy*t)),
			Z: int(math.Round(float64(a.Z) + dz*t)),
		})
	}
	return out
}

// DrawCatmullRomSpline samples a smooth spline passing through pts.
func DrawCatmullRomSpline(pts []plugin.BlockPos, thickness int) []plugin.BlockPos {
	if len(pts) < 2 {
		return pts
	}
	if len(pts) == 2 {
		return DrawLine(pts[0], pts[1], thickness)
	}
	seen := make(map[plugin.BlockPos]struct{})
	var out []plugin.BlockPos
	appendPts := func(more []plugin.BlockPos) {
		for _, p := range more {
			if _, ok := seen[p]; !ok {
				seen[p] = struct{}{}
				out = append(out, p)
			}
		}
	}
	at := func(i int) plugin.Vec3 {
		if i < 0 {
			i = 0
		}
		if i >= len(pts) {
			i = len(pts) - 1
		}
		return plugin.Vec3{X: float64(pts[i].X), Y: float64(pts[i].Y), Z: float64(pts[i].Z)}
	}
	for i := 0; i < len(pts)-1; i++ {
		p0, p1, p2, p3 := at(i-1), at(i), at(i+1), at(i+2)
		dist := p2.Sub(p1).Len()
		segs := max(8, int(math.Ceil(dist*2)))
		prev := pts[i]
		for s := 1; s <= segs; s++ {
			t := float64(s) / float64(segs)
			t2, t3 := t*t, t*t*t
			eval := func(a, b, c, d float64) float64 {
				return 0.5 * ((2 * b) + (-a+c)*t + (2*a-5*b+4*c-d)*t2 + (-a+3*b-3*c+d)*t3)
			}
			cur := plugin.BlockPos{
				X: int(math.Round(eval(p0.X, p1.X, p2.X, p3.X))),
				Y: int(math.Round(eval(p0.Y, p1.Y, p2.Y, p3.Y))),
				Z: int(math.Round(eval(p0.Z, p1.Z, p2.Z, p3.Z))),
			}
			appendPts(DrawLine(prev, cur, thickness))
			prev = cur
		}
	}
	return out
}
