package region

import (
	"fmt"
	"math"
	"strings"

	"github.com/golem-mc/golem/plugin"
)

// SelectorType names one of the WorldEdit/FAWE selection modes.
type SelectorType string

const (
	SelectorCuboid    SelectorType = "cuboid"
	SelectorExtend    SelectorType = "extend"
	SelectorPoly      SelectorType = "poly"
	SelectorEllipsoid SelectorType = "ellipsoid"
	SelectorSphere    SelectorType = "sphere"
	SelectorCyl       SelectorType = "cyl"
	SelectorConvex    SelectorType = "convex"
	SelectorFuzzy     SelectorType = "fuzzy"
)

// ParseSelectorType normalizes aliases (e.g. "poly2d" -> "poly", "cylinder" -> "cyl").
func ParseSelectorType(s string) (SelectorType, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cuboid", "cub":
		return SelectorCuboid, true
	case "extend", "ext":
		return SelectorExtend, true
	case "poly", "poly2d", "polygon":
		return SelectorPoly, true
	case "ellipsoid", "ellip":
		return SelectorEllipsoid, true
	case "sphere":
		return SelectorSphere, true
	case "cyl", "cylinder":
		return SelectorCyl, true
	case "convex", "polyhedron":
		return SelectorConvex, true
	case "fuzzy", "magic":
		return SelectorFuzzy, true
	default:
		return "", false
	}
}

// Selector manages interactive point selection for a player.
type Selector interface {
	Type() SelectorType
	SelectPrimary(pos plugin.BlockPos, maxPoints int) (msg string, changed bool)
	SelectSecondary(pos plugin.BlockPos, maxPoints int) (msg string, changed bool)
	Clear()
	IsDefined() bool
	Region() (Region, error)
	PrimaryPos() (plugin.BlockPos, bool)
	// CUIEvents returns WECUI protocol payloads describing the current selection.
	CUIEvents() []string
}

// NewSelector constructs a fresh selector of the given type.
func NewSelector(t SelectorType) Selector {
	switch t {
	case SelectorExtend:
		return &ExtendSelector{}
	case SelectorPoly:
		return &PolySelector{}
	case SelectorEllipsoid:
		return &EllipsoidSelector{}
	case SelectorSphere:
		return &EllipsoidSelector{sphere: true}
	case SelectorCyl:
		return &CylinderSelector{}
	case SelectorConvex:
		return &ConvexSelector{}
	case SelectorFuzzy:
		return &FuzzySelector{}
	default:
		return &CuboidSelector{}
	}
}

// CuboidSelector selects a cuboid from Pos1 and Pos2.
type CuboidSelector struct {
	Pos1, Pos2 *plugin.BlockPos
}

func (s *CuboidSelector) Type() SelectorType { return SelectorCuboid }

func (s *CuboidSelector) SelectPrimary(pos plugin.BlockPos, _ int) (string, bool) {
	if s.Pos1 != nil && *s.Pos1 == pos {
		return fmt.Sprintf("First position set to %s.", FormatPos(pos)), false
	}
	p := pos
	s.Pos1 = &p
	if s.IsDefined() {
		r, _ := s.Region()
		return fmt.Sprintf("First position set to %s (%d).", FormatPos(pos), r.Volume()), true
	}
	return fmt.Sprintf("First position set to %s.", FormatPos(pos)), true
}

func (s *CuboidSelector) SelectSecondary(pos plugin.BlockPos, _ int) (string, bool) {
	if s.Pos2 != nil && *s.Pos2 == pos {
		return fmt.Sprintf("Second position set to %s.", FormatPos(pos)), false
	}
	p := pos
	s.Pos2 = &p
	if s.IsDefined() {
		r, _ := s.Region()
		return fmt.Sprintf("Second position set to %s (%d).", FormatPos(pos), r.Volume()), true
	}
	return fmt.Sprintf("Second position set to %s.", FormatPos(pos)), true
}

func (s *CuboidSelector) Clear()          { s.Pos1, s.Pos2 = nil, nil }
func (s *CuboidSelector) IsDefined() bool { return s.Pos1 != nil && s.Pos2 != nil }

func (s *CuboidSelector) Region() (Region, error) {
	if !s.IsDefined() {
		return nil, ErrIncomplete
	}
	return NewCuboid(*s.Pos1, *s.Pos2), nil
}

func (s *CuboidSelector) PrimaryPos() (plugin.BlockPos, bool) {
	if s.Pos1 == nil {
		return plugin.BlockPos{}, false
	}
	return *s.Pos1, true
}

func (s *CuboidSelector) CUIEvents() []string {
	ev := []string{"s|cuboid"}
	vol := int64(-1)
	if s.IsDefined() {
		r, _ := s.Region()
		vol = r.Volume()
	}
	if s.Pos1 != nil {
		ev = append(ev, fmt.Sprintf("p|0|%d|%d|%d|%d", s.Pos1.X, s.Pos1.Y, s.Pos1.Z, vol))
	}
	if s.Pos2 != nil {
		ev = append(ev, fmt.Sprintf("p|1|%d|%d|%d|%d", s.Pos2.X, s.Pos2.Y, s.Pos2.Z, vol))
	}
	return ev
}

// ExtendSelector resets to a 1x1x1 cuboid on primary click and expands to encompass every secondary click.
type ExtendSelector struct {
	CuboidSelector
}

func (s *ExtendSelector) Type() SelectorType { return SelectorExtend }

func (s *ExtendSelector) SelectPrimary(pos plugin.BlockPos, _ int) (string, bool) {
	p1, p2 := pos, pos
	s.Pos1, s.Pos2 = &p1, &p2
	return fmt.Sprintf("Selection started at %s (1).", FormatPos(pos)), true
}

func (s *ExtendSelector) SelectSecondary(pos plugin.BlockPos, _ int) (string, bool) {
	if !s.IsDefined() {
		return s.SelectPrimary(pos, 0)
	}
	lo := plugin.BlockPos{
		X: min(s.Pos1.X, s.Pos2.X, pos.X),
		Y: min(s.Pos1.Y, s.Pos2.Y, pos.Y),
		Z: min(s.Pos1.Z, s.Pos2.Z, pos.Z),
	}
	hi := plugin.BlockPos{
		X: max(s.Pos1.X, s.Pos2.X, pos.X),
		Y: max(s.Pos1.Y, s.Pos2.Y, pos.Y),
		Z: max(s.Pos1.Z, s.Pos2.Z, pos.Z),
	}
	s.Pos1, s.Pos2 = &lo, &hi
	r, _ := s.Region()
	return fmt.Sprintf("Selection expanded to %s (%d).", FormatPos(pos), r.Volume()), true
}

// PolySelector selects a 2D polygon by starting with primary click and appending vertices on secondary click.
type PolySelector struct {
	Points     []Point2D
	MinY, MaxY int
	hasY       bool
}

func (s *PolySelector) Type() SelectorType { return SelectorPoly }

func (s *PolySelector) SelectPrimary(pos plugin.BlockPos, _ int) (string, bool) {
	s.Points = []Point2D{{X: pos.X, Z: pos.Z}}
	s.MinY, s.MaxY, s.hasY = pos.Y, pos.Y, true
	return fmt.Sprintf("Started new polygon with vertex #1 at %s.", FormatPos(pos)), true
}

func (s *PolySelector) SelectSecondary(pos plugin.BlockPos, maxPoints int) (string, bool) {
	if !s.hasY {
		return s.SelectPrimary(pos, maxPoints)
	}
	if maxPoints > 0 && len(s.Points) >= maxPoints {
		return fmt.Sprintf("Maximum number of polygon points (%d) reached.", maxPoints), false
	}
	s.Points = append(s.Points, Point2D{X: pos.X, Z: pos.Z})
	s.MinY = min(s.MinY, pos.Y)
	s.MaxY = max(s.MaxY, pos.Y)
	return fmt.Sprintf("Added polygon vertex #%d at %s.", len(s.Points), FormatPos(pos)), true
}

func (s *PolySelector) Clear()          { s.Points = nil; s.hasY = false }
func (s *PolySelector) IsDefined() bool { return len(s.Points) >= 3 && s.hasY }

func (s *PolySelector) Region() (Region, error) {
	if !s.IsDefined() {
		return nil, ErrIncomplete
	}
	return NewPolygon2D(s.Points, s.MinY, s.MaxY), nil
}

func (s *PolySelector) PrimaryPos() (plugin.BlockPos, bool) {
	if len(s.Points) == 0 {
		return plugin.BlockPos{}, false
	}
	return plugin.BlockPos{X: s.Points[0].X, Y: s.MinY, Z: s.Points[0].Z}, true
}

func (s *PolySelector) CUIEvents() []string {
	ev := []string{"s|polygon2d"}
	if s.hasY {
		ev = append(ev, fmt.Sprintf("mm|%d|%d", s.MinY, s.MaxY))
	}
	for i, pt := range s.Points {
		ev = append(ev, fmt.Sprintf("p2|%d|%d|%d|0", i, pt.X, pt.Z))
	}
	return ev
}

// EllipsoidSelector selects an ellipsoid (or sphere when sphere is true).
type EllipsoidSelector struct {
	sphere  bool
	Center  *plugin.BlockPos
	Radius  plugin.Vec3
	started bool
}

func (s *EllipsoidSelector) Type() SelectorType {
	if s.sphere {
		return SelectorSphere
	}
	return SelectorEllipsoid
}

func (s *EllipsoidSelector) SelectPrimary(pos plugin.BlockPos, _ int) (string, bool) {
	p := pos
	s.Center = &p
	s.Radius = plugin.Vec3{}
	s.started = false
	return fmt.Sprintf("Center set to %s.", FormatPos(pos)), true
}

func (s *EllipsoidSelector) SelectSecondary(pos plugin.BlockPos, _ int) (string, bool) {
	if s.Center == nil {
		return s.SelectPrimary(pos, 0)
	}
	dx := math.Abs(float64(pos.X - s.Center.X))
	dy := math.Abs(float64(pos.Y - s.Center.Y))
	dz := math.Abs(float64(pos.Z - s.Center.Z))
	if s.sphere {
		r := math.Sqrt(dx*dx + dy*dy + dz*dz)
		s.Radius = plugin.Vec3{X: r, Y: r, Z: r}
	} else {
		s.Radius = plugin.Vec3{X: max(s.Radius.X, dx), Y: max(s.Radius.Y, dy), Z: max(s.Radius.Z, dz)}
	}
	s.started = true
	r, _ := s.Region()
	return fmt.Sprintf("Radius set to (%.2f, %.2f, %.2f) (%d).", s.Radius.X, s.Radius.Y, s.Radius.Z, r.Volume()), true
}

func (s *EllipsoidSelector) Clear()          { s.Center = nil; s.Radius = plugin.Vec3{}; s.started = false }
func (s *EllipsoidSelector) IsDefined() bool { return s.Center != nil && s.started }

func (s *EllipsoidSelector) Region() (Region, error) {
	if !s.IsDefined() {
		return nil, ErrIncomplete
	}
	return NewEllipsoid(*s.Center, s.Radius), nil
}

func (s *EllipsoidSelector) PrimaryPos() (plugin.BlockPos, bool) {
	if s.Center == nil {
		return plugin.BlockPos{}, false
	}
	return *s.Center, true
}

func (s *EllipsoidSelector) CUIEvents() []string {
	ev := []string{"s|ellipsoid"}
	if s.Center != nil {
		ev = append(ev, fmt.Sprintf("e|0|%d|%d|%d", s.Center.X, s.Center.Y, s.Center.Z))
	}
	if s.started {
		ev = append(ev, fmt.Sprintf("e|1|%.2f|%.2f|%.2f", s.Radius.X, s.Radius.Y, s.Radius.Z))
	}
	return ev
}

// CylinderSelector selects a vertical elliptical cylinder.
type CylinderSelector struct {
	Center     *plugin.BlockPos
	RadiusX    float64
	RadiusZ    float64
	MinY, MaxY int
	hasRadius  bool
}

func (s *CylinderSelector) Type() SelectorType { return SelectorCyl }

func (s *CylinderSelector) SelectPrimary(pos plugin.BlockPos, _ int) (string, bool) {
	p := pos
	s.Center = &p
	s.MinY, s.MaxY = pos.Y, pos.Y
	s.RadiusX, s.RadiusZ, s.hasRadius = 0, 0, false
	return fmt.Sprintf("Cylinder center set to %s.", FormatPos(pos)), true
}

func (s *CylinderSelector) SelectSecondary(pos plugin.BlockPos, _ int) (string, bool) {
	if s.Center == nil {
		return s.SelectPrimary(pos, 0)
	}
	dx := math.Abs(float64(pos.X - s.Center.X))
	dz := math.Abs(float64(pos.Z - s.Center.Z))
	s.RadiusX = max(s.RadiusX, dx)
	s.RadiusZ = max(s.RadiusZ, dz)
	s.MinY = min(s.MinY, pos.Y)
	s.MaxY = max(s.MaxY, pos.Y)
	s.hasRadius = true
	r, _ := s.Region()
	return fmt.Sprintf("Cylinder radius set to (%.2f, %.2f), Y=[%d..%d] (%d).", s.RadiusX, s.RadiusZ, s.MinY, s.MaxY, r.Volume()), true
}

func (s *CylinderSelector) Clear() {
	s.Center = nil
	s.RadiusX, s.RadiusZ, s.hasRadius = 0, 0, false
}

func (s *CylinderSelector) IsDefined() bool { return s.Center != nil && s.hasRadius }

func (s *CylinderSelector) Region() (Region, error) {
	if !s.IsDefined() {
		return nil, ErrIncomplete
	}
	return NewCylinder(*s.Center, s.RadiusX, s.RadiusZ, s.MinY, s.MaxY), nil
}

func (s *CylinderSelector) PrimaryPos() (plugin.BlockPos, bool) {
	if s.Center == nil {
		return plugin.BlockPos{}, false
	}
	return *s.Center, true
}

func (s *CylinderSelector) CUIEvents() []string {
	ev := []string{"s|cylinder"}
	if s.Center != nil {
		ev = append(ev, fmt.Sprintf("cyl|%d|%d|%d|%.2f|%.2f", s.Center.X, s.Center.Y, s.Center.Z, s.RadiusX, s.RadiusZ))
		ev = append(ev, fmt.Sprintf("mm|%d|%d", s.MinY, s.MaxY))
	}
	return ev
}

// ConvexSelector selects a 3D convex polyhedron by vertices.
type ConvexSelector struct {
	Vertices []plugin.BlockPos
}

func (s *ConvexSelector) Type() SelectorType { return SelectorConvex }

func (s *ConvexSelector) SelectPrimary(pos plugin.BlockPos, _ int) (string, bool) {
	s.Vertices = []plugin.BlockPos{pos}
	return fmt.Sprintf("Started new convex polyhedron with vertex #1 at %s.", FormatPos(pos)), true
}

func (s *ConvexSelector) SelectSecondary(pos plugin.BlockPos, maxPoints int) (string, bool) {
	if len(s.Vertices) == 0 {
		return s.SelectPrimary(pos, maxPoints)
	}
	if maxPoints > 0 && len(s.Vertices) >= maxPoints {
		return fmt.Sprintf("Maximum number of polyhedron points (%d) reached.", maxPoints), false
	}
	s.Vertices = append(s.Vertices, pos)
	return fmt.Sprintf("Added vertex #%d at %s.", len(s.Vertices), FormatPos(pos)), true
}

func (s *ConvexSelector) Clear()          { s.Vertices = nil }
func (s *ConvexSelector) IsDefined() bool { return len(s.Vertices) >= 4 }

func (s *ConvexSelector) Region() (Region, error) {
	if !s.IsDefined() {
		return nil, ErrIncomplete
	}
	return NewConvexPolyhedron(s.Vertices), nil
}

func (s *ConvexSelector) PrimaryPos() (plugin.BlockPos, bool) {
	if len(s.Vertices) == 0 {
		return plugin.BlockPos{}, false
	}
	return s.Vertices[0], true
}

func (s *ConvexSelector) CUIEvents() []string {
	ev := []string{"s|polyhedron"}
	for i, v := range s.Vertices {
		ev = append(ev, fmt.Sprintf("p|%d|%d|%d|%d|0", i, v.X, v.Y, v.Z))
	}
	return ev
}

// FuzzySelector holds flood-filled connected positions.
type FuzzySelector struct {
	primary *plugin.BlockPos
	region  *Fuzzy
}

func (s *FuzzySelector) Type() SelectorType { return SelectorFuzzy }

func (s *FuzzySelector) SelectPrimary(pos plugin.BlockPos, _ int) (string, bool) {
	p := pos
	s.primary = &p
	s.region = NewFuzzy([]plugin.BlockPos{pos})
	return fmt.Sprintf("Region set and expanded from %s (1).", FormatPos(pos)), true
}

func (s *FuzzySelector) SelectSecondary(pos plugin.BlockPos, _ int) (string, bool) {
	if s.region == nil {
		return s.SelectPrimary(pos, 0)
	}
	pts := make([]plugin.BlockPos, 0, len(s.region.positions)+1)
	for p := range s.region.positions {
		pts = append(pts, p)
	}
	pts = append(pts, pos)
	s.region = NewFuzzy(pts)
	return fmt.Sprintf("Added expansion of %s (%d).", FormatPos(pos), s.region.Volume()), true
}

// SetFloodPositions replaces (if replace is true) or unions flood-filled positions.
func (s *FuzzySelector) SetFloodPositions(origin plugin.BlockPos, positions []plugin.BlockPos, replace bool) {
	p := origin
	if replace || s.region == nil {
		s.primary = &p
		s.region = NewFuzzy(positions)
		return
	}
	pts := make([]plugin.BlockPos, 0, len(s.region.positions)+len(positions))
	for existing := range s.region.positions {
		pts = append(pts, existing)
	}
	pts = append(pts, positions...)
	s.region = NewFuzzy(pts)
}

func (s *FuzzySelector) Clear()          { s.primary = nil; s.region = nil }
func (s *FuzzySelector) IsDefined() bool { return s.region != nil && s.region.Volume() > 0 }
func (s *FuzzySelector) Region() (Region, error) {
	if !s.IsDefined() {
		return nil, ErrIncomplete
	}
	return s.region.Clone(), nil
}
func (s *FuzzySelector) PrimaryPos() (plugin.BlockPos, bool) {
	if s.primary == nil {
		return plugin.BlockPos{}, false
	}
	return *s.primary, true
}
func (s *FuzzySelector) CUIEvents() []string {
	if !s.IsDefined() {
		return []string{"s|cuboid"}
	}
	lo, hi := s.region.Min(), s.region.Max()
	return []string{
		"s|cuboid",
		fmt.Sprintf("p|0|%d|%d|%d|%d", lo.X, lo.Y, lo.Z, s.region.Volume()),
		fmt.Sprintf("p|1|%d|%d|%d|%d", hi.X, hi.Y, hi.Z, s.region.Volume()),
	}
}
