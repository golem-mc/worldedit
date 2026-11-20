// Package region implements WorldEdit and FastAsyncWorldEdit 3D regions
// and interactive region selectors (cuboid, extend, poly, ellipsoid, sphere,
// cylinder, convex polyhedron, and fuzzy).
package region

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/golem-mc/golem/plugin"
)

// ErrIncomplete is returned when a selector does not yet define a complete region.
var ErrIncomplete = errors.New("region is incomplete")

// ErrUnsupportedOp is returned when an operation (such as Contract) is not supported by a region type.
var ErrUnsupportedOp = errors.New("operation not supported for this region type")

// Region represents a 3D set of block positions.
type Region interface {
	TypeName() string
	Min() plugin.BlockPos
	Max() plugin.BlockPos
	Center() plugin.Vec3
	Width() int
	Height() int
	Length() int
	Volume() int64
	Contains(pos plugin.BlockPos) bool
	Chunks() []plugin.ChunkPos
	Clone() Region
	Expand(changes ...plugin.BlockPos) error
	Contract(changes ...plugin.BlockPos) error
	Shift(change plugin.BlockPos) error
	// ForEach calls fn for every block position in the region in Y-Z-X or chunk-friendly order.
	// Iteration stops early when fn returns false.
	ForEach(fn func(pos plugin.BlockPos) bool)
}

func bboxChunks(minP, maxP plugin.BlockPos) []plugin.ChunkPos {
	cx1, cx2 := minP.X>>4, maxP.X>>4
	cz1, cz2 := minP.Z>>4, maxP.Z>>4
	out := make([]plugin.ChunkPos, 0, (cx2-cx1+1)*(cz2-cz1+1))
	for cx := cx1; cx <= cx2; cx++ {
		for cz := cz1; cz <= cz2; cz++ {
			out = append(out, plugin.ChunkPos{X: cx, Z: cz})
		}
	}
	return out
}

// Cuboid is an axis-aligned rectangular box defined by Pos1 and Pos2.
type Cuboid struct {
	Pos1, Pos2 plugin.BlockPos
}

// NewCuboid creates a cuboid region from two corners.
func NewCuboid(pos1, pos2 plugin.BlockPos) *Cuboid {
	return &Cuboid{Pos1: pos1, Pos2: pos2}
}

func (c *Cuboid) TypeName() string { return "cuboid" }

func (c *Cuboid) Min() plugin.BlockPos {
	return plugin.BlockPos{
		X: min(c.Pos1.X, c.Pos2.X),
		Y: min(c.Pos1.Y, c.Pos2.Y),
		Z: min(c.Pos1.Z, c.Pos2.Z),
	}
}

func (c *Cuboid) Max() plugin.BlockPos {
	return plugin.BlockPos{
		X: max(c.Pos1.X, c.Pos2.X),
		Y: max(c.Pos1.Y, c.Pos2.Y),
		Z: max(c.Pos1.Z, c.Pos2.Z),
	}
}

func (c *Cuboid) Center() plugin.Vec3 {
	lo, hi := c.Min(), c.Max()
	return plugin.Vec3{
		X: float64(lo.X+hi.X) / 2.0,
		Y: float64(lo.Y+hi.Y) / 2.0,
		Z: float64(lo.Z+hi.Z) / 2.0,
	}
}

func (c *Cuboid) Width() int  { lo, hi := c.Min(), c.Max(); return hi.X - lo.X + 1 }
func (c *Cuboid) Height() int { lo, hi := c.Min(), c.Max(); return hi.Y - lo.Y + 1 }
func (c *Cuboid) Length() int { lo, hi := c.Min(), c.Max(); return hi.Z - lo.Z + 1 }
func (c *Cuboid) Volume() int64 {
	return int64(c.Width()) * int64(c.Height()) * int64(c.Length())
}

func (c *Cuboid) Contains(p plugin.BlockPos) bool {
	lo, hi := c.Min(), c.Max()
	return p.X >= lo.X && p.X <= hi.X && p.Y >= lo.Y && p.Y <= hi.Y && p.Z >= lo.Z && p.Z <= hi.Z
}

func (c *Cuboid) Chunks() []plugin.ChunkPos { return bboxChunks(c.Min(), c.Max()) }
func (c *Cuboid) Clone() Region             { cp := *c; return &cp }

func (c *Cuboid) Expand(changes ...plugin.BlockPos) error {
	lo, hi := c.Min(), c.Max()
	for _, d := range changes {
		if d.X > 0 {
			hi.X += d.X
		} else {
			lo.X += d.X
		}
		if d.Y > 0 {
			hi.Y += d.Y
		} else {
			lo.Y += d.Y
		}
		if d.Z > 0 {
			hi.Z += d.Z
		} else {
			lo.Z += d.Z
		}
	}
	c.Pos1, c.Pos2 = lo, hi
	return nil
}

func (c *Cuboid) Contract(changes ...plugin.BlockPos) error {
	lo, hi := c.Min(), c.Max()
	for _, d := range changes {
		if d.X < 0 {
			hi.X += d.X
		} else {
			lo.X += d.X
		}
		if d.Y < 0 {
			hi.Y += d.Y
		} else {
			lo.Y += d.Y
		}
		if d.Z < 0 {
			hi.Z += d.Z
		} else {
			lo.Z += d.Z
		}
	}
	if lo.X > hi.X {
		lo.X = hi.X
	}
	if lo.Y > hi.Y {
		lo.Y = hi.Y
	}
	if lo.Z > hi.Z {
		lo.Z = hi.Z
	}
	c.Pos1, c.Pos2 = lo, hi
	return nil
}

func (c *Cuboid) Shift(d plugin.BlockPos) error {
	c.Pos1 = plugin.BlockPos{X: c.Pos1.X + d.X, Y: c.Pos1.Y + d.Y, Z: c.Pos1.Z + d.Z}
	c.Pos2 = plugin.BlockPos{X: c.Pos2.X + d.X, Y: c.Pos2.Y + d.Y, Z: c.Pos2.Z + d.Z}
	return nil
}

func (c *Cuboid) ForEach(fn func(pos plugin.BlockPos) bool) {
	lo, hi := c.Min(), c.Max()
	for y := lo.Y; y <= hi.Y; y++ {
		for z := lo.Z; z <= hi.Z; z++ {
			for x := lo.X; x <= hi.X; x++ {
				if !fn(plugin.BlockPos{X: x, Y: y, Z: z}) {
					return
				}
			}
		}
	}
}

// Point2D is an integer 2D horizontal coordinate.
type Point2D struct{ X, Z int }

// Polygon2D is a 2D polygon extruded vertically between MinY and MaxY.
type Polygon2D struct {
	Points     []Point2D
	MinY, MaxY int
}

// NewPolygon2D creates a 2D polygonal region.
func NewPolygon2D(points []Point2D, minY, maxY int) *Polygon2D {
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	return &Polygon2D{Points: slices.Clone(points), MinY: minY, MaxY: maxY}
}

func (p *Polygon2D) TypeName() string { return "poly2d" }

func (p *Polygon2D) Min() plugin.BlockPos {
	if len(p.Points) == 0 {
		return plugin.BlockPos{}
	}
	minX, minZ := p.Points[0].X, p.Points[0].Z
	for _, pt := range p.Points[1:] {
		minX = min(minX, pt.X)
		minZ = min(minZ, pt.Z)
	}
	return plugin.BlockPos{X: minX, Y: p.MinY, Z: minZ}
}

func (p *Polygon2D) Max() plugin.BlockPos {
	if len(p.Points) == 0 {
		return plugin.BlockPos{}
	}
	maxX, maxZ := p.Points[0].X, p.Points[0].Z
	for _, pt := range p.Points[1:] {
		maxX = max(maxX, pt.X)
		maxZ = max(maxZ, pt.Z)
	}
	return plugin.BlockPos{X: maxX, Y: p.MaxY, Z: maxZ}
}

func (p *Polygon2D) Center() plugin.Vec3 {
	lo, hi := p.Min(), p.Max()
	return plugin.Vec3{
		X: float64(lo.X+hi.X) / 2.0,
		Y: float64(lo.Y+hi.Y) / 2.0,
		Z: float64(lo.Z+hi.Z) / 2.0,
	}
}

func (p *Polygon2D) Width() int  { lo, hi := p.Min(), p.Max(); return hi.X - lo.X + 1 }
func (p *Polygon2D) Height() int { return p.MaxY - p.MinY + 1 }
func (p *Polygon2D) Length() int { lo, hi := p.Min(), p.Max(); return hi.Z - lo.Z + 1 }

func (p *Polygon2D) Volume() int64 {
	if len(p.Points) < 3 {
		return 0
	}
	var count int64
	lo, hi := p.Min(), p.Max()
	for z := lo.Z; z <= hi.Z; z++ {
		for x := lo.X; x <= hi.X; x++ {
			if p.contains2D(x, z) {
				count++
			}
		}
	}
	return count * int64(p.Height())
}

func (p *Polygon2D) Contains(pos plugin.BlockPos) bool {
	if pos.Y < p.MinY || pos.Y > p.MaxY || len(p.Points) < 3 {
		return false
	}
	return p.contains2D(pos.X, pos.Z)
}

// contains2D implements WorldEdit's exact integer ray-casting + boundary test.
func (p *Polygon2D) contains2D(tx, tz int) bool {
	n := len(p.Points)
	if n < 3 {
		return false
	}
	inside := false
	xold, zold := p.Points[n-1].X, p.Points[n-1].Z
	for i := 0; i < n; i++ {
		xnew, znew := p.Points[i].X, p.Points[i].Z
		if xnew == tx && znew == tz {
			return true
		}
		var x1, z1, x2, z2 int
		if xnew > xold {
			x1, z1 = xold, zold
			x2, z2 = xnew, znew
		} else {
			x1, z1 = xnew, znew
			x2, z2 = xold, zold
		}
		if x1 <= tx && tx <= x2 {
			cross := int64(tz-z1)*int64(x2-x1) - int64(z2-z1)*int64(tx-x1)
			if cross == 0 {
				if (z1 <= tz && tz <= z2) || (z2 <= tz && tz <= z1) {
					return true
				}
			} else if cross < 0 && x1 != tx {
				inside = !inside
			}
		}
		xold, zold = xnew, znew
	}
	return inside
}

func (p *Polygon2D) Chunks() []plugin.ChunkPos { return bboxChunks(p.Min(), p.Max()) }
func (p *Polygon2D) Clone() Region             { return NewPolygon2D(p.Points, p.MinY, p.MaxY) }

func (p *Polygon2D) Expand(changes ...plugin.BlockPos) error {
	for _, d := range changes {
		if d.X != 0 || d.Z != 0 {
			return ErrUnsupportedOp
		}
		if d.Y > 0 {
			p.MaxY += d.Y
		} else {
			p.MinY += d.Y
		}
	}
	return nil
}

func (p *Polygon2D) Contract(changes ...plugin.BlockPos) error {
	for _, d := range changes {
		if d.X != 0 || d.Z != 0 {
			return ErrUnsupportedOp
		}
		if d.Y < 0 {
			p.MaxY += d.Y
		} else {
			p.MinY += d.Y
		}
	}
	if p.MinY > p.MaxY {
		p.MinY = p.MaxY
	}
	return nil
}

func (p *Polygon2D) Shift(d plugin.BlockPos) error {
	for i := range p.Points {
		p.Points[i].X += d.X
		p.Points[i].Z += d.Z
	}
	p.MinY += d.Y
	p.MaxY += d.Y
	return nil
}

func (p *Polygon2D) ForEach(fn func(pos plugin.BlockPos) bool) {
	if len(p.Points) < 3 {
		return
	}
	lo, hi := p.Min(), p.Max()
	for z := lo.Z; z <= hi.Z; z++ {
		for x := lo.X; x <= hi.X; x++ {
			if !p.contains2D(x, z) {
				continue
			}
			for y := p.MinY; y <= p.MaxY; y++ {
				if !fn(plugin.BlockPos{X: x, Y: y, Z: z}) {
					return
				}
			}
		}
	}
}

// Ellipsoid is a 3D ellipsoid (or sphere when Radius.X == Radius.Y == Radius.Z).
type Ellipsoid struct {
	CenterPos plugin.BlockPos
	Radius    plugin.Vec3
}

// NewEllipsoid creates an ellipsoid region.
func NewEllipsoid(center plugin.BlockPos, radius plugin.Vec3) *Ellipsoid {
	return &Ellipsoid{
		CenterPos: center,
		Radius: plugin.Vec3{
			X: math.Abs(radius.X),
			Y: math.Abs(radius.Y),
			Z: math.Abs(radius.Z),
		},
	}
}

func (e *Ellipsoid) TypeName() string { return "ellipsoid" }

func (e *Ellipsoid) Min() plugin.BlockPos {
	return plugin.BlockPos{
		X: e.CenterPos.X - int(math.Floor(e.Radius.X)),
		Y: e.CenterPos.Y - int(math.Floor(e.Radius.Y)),
		Z: e.CenterPos.Z - int(math.Floor(e.Radius.Z)),
	}
}

func (e *Ellipsoid) Max() plugin.BlockPos {
	return plugin.BlockPos{
		X: e.CenterPos.X + int(math.Floor(e.Radius.X)),
		Y: e.CenterPos.Y + int(math.Floor(e.Radius.Y)),
		Z: e.CenterPos.Z + int(math.Floor(e.Radius.Z)),
	}
}

func (e *Ellipsoid) Center() plugin.Vec3 {
	return plugin.Vec3{X: float64(e.CenterPos.X), Y: float64(e.CenterPos.Y), Z: float64(e.CenterPos.Z)}
}

func (e *Ellipsoid) Width() int  { lo, hi := e.Min(), e.Max(); return hi.X - lo.X + 1 }
func (e *Ellipsoid) Height() int { lo, hi := e.Min(), e.Max(); return hi.Y - lo.Y + 1 }
func (e *Ellipsoid) Length() int { lo, hi := e.Min(), e.Max(); return hi.Z - lo.Z + 1 }

func (e *Ellipsoid) Volume() int64 {
	return int64(math.Floor((4.0 / 3.0) * math.Pi * (e.Radius.X + 0.5) * (e.Radius.Y + 0.5) * (e.Radius.Z + 0.5)))
}

func (e *Ellipsoid) Contains(pos plugin.BlockPos) bool {
	rx, ry, rz := e.Radius.X+0.5, e.Radius.Y+0.5, e.Radius.Z+0.5
	dx := float64(pos.X-e.CenterPos.X) / rx
	dy := float64(pos.Y-e.CenterPos.Y) / ry
	dz := float64(pos.Z-e.CenterPos.Z) / rz
	return dx*dx+dy*dy+dz*dz <= 1.0
}

func (e *Ellipsoid) Chunks() []plugin.ChunkPos { return bboxChunks(e.Min(), e.Max()) }
func (e *Ellipsoid) Clone() Region             { cp := *e; return &cp }

func (e *Ellipsoid) Expand(changes ...plugin.BlockPos) error {
	for _, d := range changes {
		e.Radius.X += math.Abs(float64(d.X))
		e.Radius.Y += math.Abs(float64(d.Y))
		e.Radius.Z += math.Abs(float64(d.Z))
	}
	return nil
}

func (e *Ellipsoid) Contract(changes ...plugin.BlockPos) error {
	for _, d := range changes {
		e.Radius.X = max(0, e.Radius.X-math.Abs(float64(d.X)))
		e.Radius.Y = max(0, e.Radius.Y-math.Abs(float64(d.Y)))
		e.Radius.Z = max(0, e.Radius.Z-math.Abs(float64(d.Z)))
	}
	return nil
}

func (e *Ellipsoid) Shift(d plugin.BlockPos) error {
	e.CenterPos = plugin.BlockPos{X: e.CenterPos.X + d.X, Y: e.CenterPos.Y + d.Y, Z: e.CenterPos.Z + d.Z}
	return nil
}

func (e *Ellipsoid) ForEach(fn func(pos plugin.BlockPos) bool) {
	lo, hi := e.Min(), e.Max()
	for y := lo.Y; y <= hi.Y; y++ {
		for z := lo.Z; z <= hi.Z; z++ {
			for x := lo.X; x <= hi.X; x++ {
				p := plugin.BlockPos{X: x, Y: y, Z: z}
				if e.Contains(p) {
					if !fn(p) {
						return
					}
				}
			}
		}
	}
}

// Cylinder is a vertical elliptical cylinder between MinY and MaxY.
type Cylinder struct {
	CenterPos  plugin.BlockPos
	RadiusX    float64
	RadiusZ    float64
	MinY, MaxY int
}

// NewCylinder creates a cylindrical region.
func NewCylinder(center plugin.BlockPos, rx, rz float64, minY, maxY int) *Cylinder {
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	return &Cylinder{
		CenterPos: center,
		RadiusX:   math.Abs(rx),
		RadiusZ:   math.Abs(rz),
		MinY:      minY,
		MaxY:      maxY,
	}
}

func (c *Cylinder) TypeName() string { return "cylinder" }

func (c *Cylinder) Min() plugin.BlockPos {
	return plugin.BlockPos{
		X: c.CenterPos.X - int(math.Floor(c.RadiusX)),
		Y: c.MinY,
		Z: c.CenterPos.Z - int(math.Floor(c.RadiusZ)),
	}
}

func (c *Cylinder) Max() plugin.BlockPos {
	return plugin.BlockPos{
		X: c.CenterPos.X + int(math.Floor(c.RadiusX)),
		Y: c.MaxY,
		Z: c.CenterPos.Z + int(math.Floor(c.RadiusZ)),
	}
}

func (c *Cylinder) Center() plugin.Vec3 {
	return plugin.Vec3{
		X: float64(c.CenterPos.X),
		Y: float64(c.MinY+c.MaxY) / 2.0,
		Z: float64(c.CenterPos.Z),
	}
}

func (c *Cylinder) Width() int  { lo, hi := c.Min(), c.Max(); return hi.X - lo.X + 1 }
func (c *Cylinder) Height() int { return c.MaxY - c.MinY + 1 }
func (c *Cylinder) Length() int { lo, hi := c.Min(), c.Max(); return hi.Z - lo.Z + 1 }

func (c *Cylinder) Volume() int64 {
	return int64(math.Floor(math.Pi*(c.RadiusX+0.5)*(c.RadiusZ+0.5))) * int64(c.Height())
}

func (c *Cylinder) Contains(pos plugin.BlockPos) bool {
	if pos.Y < c.MinY || pos.Y > c.MaxY {
		return false
	}
	dx := float64(pos.X-c.CenterPos.X) / (c.RadiusX + 0.5)
	dz := float64(pos.Z-c.CenterPos.Z) / (c.RadiusZ + 0.5)
	return dx*dx+dz*dz <= 1.0
}

func (c *Cylinder) Chunks() []plugin.ChunkPos { return bboxChunks(c.Min(), c.Max()) }
func (c *Cylinder) Clone() Region             { cp := *c; return &cp }

func (c *Cylinder) Expand(changes ...plugin.BlockPos) error {
	for _, d := range changes {
		c.RadiusX += math.Abs(float64(d.X))
		c.RadiusZ += math.Abs(float64(d.Z))
		if d.Y > 0 {
			c.MaxY += d.Y
		} else {
			c.MinY += d.Y
		}
	}
	return nil
}

func (c *Cylinder) Contract(changes ...plugin.BlockPos) error {
	for _, d := range changes {
		c.RadiusX = max(0, c.RadiusX-math.Abs(float64(d.X)))
		c.RadiusZ = max(0, c.RadiusZ-math.Abs(float64(d.Z)))
		if d.Y < 0 {
			c.MaxY += d.Y
		} else {
			c.MinY += d.Y
		}
	}
	if c.MinY > c.MaxY {
		c.MinY = c.MaxY
	}
	return nil
}

func (c *Cylinder) Shift(d plugin.BlockPos) error {
	c.CenterPos = plugin.BlockPos{X: c.CenterPos.X + d.X, Y: c.CenterPos.Y + d.Y, Z: c.CenterPos.Z + d.Z}
	c.MinY += d.Y
	c.MaxY += d.Y
	return nil
}

func (c *Cylinder) ForEach(fn func(pos plugin.BlockPos) bool) {
	lo, hi := c.Min(), c.Max()
	for z := lo.Z; z <= hi.Z; z++ {
		for x := lo.X; x <= hi.X; x++ {
			dx := float64(x-c.CenterPos.X) / (c.RadiusX + 0.5)
			dz := float64(z-c.CenterPos.Z) / (c.RadiusZ + 0.5)
			if dx*dx+dz*dz > 1.0 {
				continue
			}
			for y := c.MinY; y <= c.MaxY; y++ {
				if !fn(plugin.BlockPos{X: x, Y: y, Z: z}) {
					return
				}
			}
		}
	}
}

type hullPlane struct {
	normal plugin.Vec3
	d      float64 // normal . x <= d for points inside
}

// ConvexPolyhedron is a 3D convex hull defined by its vertices.
type ConvexPolyhedron struct {
	Vertices []plugin.BlockPos
	planes   []hullPlane
	minP     plugin.BlockPos
	maxP     plugin.BlockPos
}

// NewConvexPolyhedron builds a convex polyhedral region from vertices.
func NewConvexPolyhedron(vertices []plugin.BlockPos) *ConvexPolyhedron {
	cp := &ConvexPolyhedron{Vertices: slices.Clone(vertices)}
	cp.rebuild()
	return cp
}

func vecSub(a, b plugin.Vec3) plugin.Vec3 {
	return plugin.Vec3{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z}
}
func vecDot(a, b plugin.Vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func vecCross(a, b plugin.Vec3) plugin.Vec3 {
	return plugin.Vec3{
		X: a.Y*b.Z - a.Z*b.Y,
		Y: a.Z*b.X - a.X*b.Z,
		Z: a.X*b.Y - a.Y*b.X,
	}
}

func (c *ConvexPolyhedron) rebuild() {
	c.planes = nil
	if len(c.Vertices) == 0 {
		c.minP, c.maxP = plugin.BlockPos{}, plugin.BlockPos{}
		return
	}
	c.minP, c.maxP = c.Vertices[0], c.Vertices[0]
	for _, v := range c.Vertices[1:] {
		c.minP.X = min(c.minP.X, v.X)
		c.minP.Y = min(c.minP.Y, v.Y)
		c.minP.Z = min(c.minP.Z, v.Z)
		c.maxP.X = max(c.maxP.X, v.X)
		c.maxP.Y = max(c.maxP.Y, v.Y)
		c.maxP.Z = max(c.maxP.Z, v.Z)
	}
	n := len(c.Vertices)
	if n < 4 {
		return
	}
	pts := make([]plugin.Vec3, n)
	var centroid plugin.Vec3
	for i, v := range c.Vertices {
		pts[i] = plugin.Vec3{X: float64(v.X), Y: float64(v.Y), Z: float64(v.Z)}
		centroid.X += pts[i].X
		centroid.Y += pts[i].Y
		centroid.Z += pts[i].Z
	}
	centroid = centroid.Scale(1.0 / float64(n))

	// Construct half-space planes for all supporting triangles of the convex hull.
	const eps = 1e-6
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			for k := j + 1; k < n; k++ {
				normal := vecCross(vecSub(pts[j], pts[i]), vecSub(pts[k], pts[i]))
				length := normal.Len()
				if length < eps {
					continue
				}
				normal = normal.Scale(1.0 / length)
				d := vecDot(normal, pts[i])
				// Orient outward relative to centroid.
				if vecDot(normal, centroid) > d+eps {
					normal = normal.Scale(-1)
					d = -d
				}
				// Check if all vertices lie on the inside (<= d + eps).
				valid := true
				for m := 0; m < n; m++ {
					if vecDot(normal, pts[m]) > d+1e-4 {
						valid = false
						break
					}
				}
				if !valid {
					continue
				}
				// Deduplicate parallel coplanar faces.
				dup := false
				for _, pl := range c.planes {
					if vecDot(pl.normal, normal) > 1-1e-4 && math.Abs(pl.d-d) < 1e-3 {
						dup = true
						break
					}
				}
				if !dup {
					c.planes = append(c.planes, hullPlane{normal: normal, d: d})
				}
			}
		}
	}
}

func (c *ConvexPolyhedron) TypeName() string     { return "convex" }
func (c *ConvexPolyhedron) Min() plugin.BlockPos { return c.minP }
func (c *ConvexPolyhedron) Max() plugin.BlockPos { return c.maxP }
func (c *ConvexPolyhedron) Center() plugin.Vec3 {
	return plugin.Vec3{
		X: float64(c.minP.X+c.maxP.X) / 2.0,
		Y: float64(c.minP.Y+c.maxP.Y) / 2.0,
		Z: float64(c.minP.Z+c.maxP.Z) / 2.0,
	}
}
func (c *ConvexPolyhedron) Width() int  { return c.maxP.X - c.minP.X + 1 }
func (c *ConvexPolyhedron) Height() int { return c.maxP.Y - c.minP.Y + 1 }
func (c *ConvexPolyhedron) Length() int { return c.maxP.Z - c.minP.Z + 1 }

func (c *ConvexPolyhedron) Contains(pos plugin.BlockPos) bool {
	if len(c.planes) == 0 {
		return false
	}
	if pos.X < c.minP.X || pos.X > c.maxP.X || pos.Y < c.minP.Y || pos.Y > c.maxP.Y || pos.Z < c.minP.Z || pos.Z > c.maxP.Z {
		return false
	}
	v := plugin.Vec3{X: float64(pos.X), Y: float64(pos.Y), Z: float64(pos.Z)}
	for _, pl := range c.planes {
		if vecDot(pl.normal, v) > pl.d+0.5 {
			return false
		}
	}
	return true
}

func (c *ConvexPolyhedron) Volume() int64 {
	var count int64
	c.ForEach(func(plugin.BlockPos) bool {
		count++
		return true
	})
	return count
}

func (c *ConvexPolyhedron) Chunks() []plugin.ChunkPos { return bboxChunks(c.minP, c.maxP) }
func (c *ConvexPolyhedron) Clone() Region             { return NewConvexPolyhedron(c.Vertices) }
func (c *ConvexPolyhedron) Expand(...plugin.BlockPos) error {
	return ErrUnsupportedOp
}
func (c *ConvexPolyhedron) Contract(...plugin.BlockPos) error {
	return ErrUnsupportedOp
}
func (c *ConvexPolyhedron) Shift(d plugin.BlockPos) error {
	for i := range c.Vertices {
		c.Vertices[i].X += d.X
		c.Vertices[i].Y += d.Y
		c.Vertices[i].Z += d.Z
	}
	c.rebuild()
	return nil
}

func (c *ConvexPolyhedron) ForEach(fn func(pos plugin.BlockPos) bool) {
	if len(c.planes) == 0 {
		return
	}
	for y := c.minP.Y; y <= c.maxP.Y; y++ {
		for z := c.minP.Z; z <= c.maxP.Z; z++ {
			for x := c.minP.X; x <= c.maxP.X; x++ {
				p := plugin.BlockPos{X: x, Y: y, Z: z}
				if c.Contains(p) {
					if !fn(p) {
						return
					}
				}
			}
		}
	}
}

// Fuzzy is a region composed of an arbitrary set of selected positions (from flood fill).
type Fuzzy struct {
	positions map[plugin.BlockPos]struct{}
	minP      plugin.BlockPos
	maxP      plugin.BlockPos
}

// NewFuzzy constructs a fuzzy region from a set of block positions.
func NewFuzzy(positions []plugin.BlockPos) *Fuzzy {
	f := &Fuzzy{positions: make(map[plugin.BlockPos]struct{}, len(positions))}
	for i, p := range positions {
		f.positions[p] = struct{}{}
		if i == 0 {
			f.minP, f.maxP = p, p
		} else {
			f.minP.X = min(f.minP.X, p.X)
			f.minP.Y = min(f.minP.Y, p.Y)
			f.minP.Z = min(f.minP.Z, p.Z)
			f.maxP.X = max(f.maxP.X, p.X)
			f.maxP.Y = max(f.maxP.Y, p.Y)
			f.maxP.Z = max(f.maxP.Z, p.Z)
		}
	}
	return f
}

func (f *Fuzzy) TypeName() string     { return "fuzzy" }
func (f *Fuzzy) Min() plugin.BlockPos { return f.minP }
func (f *Fuzzy) Max() plugin.BlockPos { return f.maxP }
func (f *Fuzzy) Center() plugin.Vec3 {
	return plugin.Vec3{
		X: float64(f.minP.X+f.maxP.X) / 2.0,
		Y: float64(f.minP.Y+f.maxP.Y) / 2.0,
		Z: float64(f.minP.Z+f.maxP.Z) / 2.0,
	}
}
func (f *Fuzzy) Width() int {
	if len(f.positions) == 0 {
		return 0
	}
	return f.maxP.X - f.minP.X + 1
}
func (f *Fuzzy) Height() int {
	if len(f.positions) == 0 {
		return 0
	}
	return f.maxP.Y - f.minP.Y + 1
}
func (f *Fuzzy) Length() int {
	if len(f.positions) == 0 {
		return 0
	}
	return f.maxP.Z - f.minP.Z + 1
}
func (f *Fuzzy) Volume() int64                   { return int64(len(f.positions)) }
func (f *Fuzzy) Contains(p plugin.BlockPos) bool { _, ok := f.positions[p]; return ok }
func (f *Fuzzy) Chunks() []plugin.ChunkPos {
	seen := make(map[plugin.ChunkPos]struct{})
	for p := range f.positions {
		seen[p.Chunk()] = struct{}{}
	}
	out := make([]plugin.ChunkPos, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	return out
}
func (f *Fuzzy) Clone() Region {
	pts := make([]plugin.BlockPos, 0, len(f.positions))
	for p := range f.positions {
		pts = append(pts, p)
	}
	return NewFuzzy(pts)
}
func (f *Fuzzy) Expand(...plugin.BlockPos) error   { return ErrUnsupportedOp }
func (f *Fuzzy) Contract(...plugin.BlockPos) error { return ErrUnsupportedOp }
func (f *Fuzzy) Shift(d plugin.BlockPos) error {
	next := make(map[plugin.BlockPos]struct{}, len(f.positions))
	for p := range f.positions {
		next[plugin.BlockPos{X: p.X + d.X, Y: p.Y + d.Y, Z: p.Z + d.Z}] = struct{}{}
	}
	f.positions = next
	f.minP = plugin.BlockPos{X: f.minP.X + d.X, Y: f.minP.Y + d.Y, Z: f.minP.Z + d.Z}
	f.maxP = plugin.BlockPos{X: f.maxP.X + d.X, Y: f.maxP.Y + d.Y, Z: f.maxP.Z + d.Z}
	return nil
}
func (f *Fuzzy) ForEach(fn func(pos plugin.BlockPos) bool) {
	for p := range f.positions {
		if !fn(p) {
			return
		}
	}
}

// Walls returns all blocks on the 4 vertical faces of r (for Cuboid) or boundary of r.
func Walls(r Region) []plugin.BlockPos {
	lo, hi := r.Min(), r.Max()
	var out []plugin.BlockPos
	r.ForEach(func(p plugin.BlockPos) bool {
		if c, ok := r.(*Cuboid); ok {
			_ = c
			if p.X == lo.X || p.X == hi.X || p.Z == lo.Z || p.Z == hi.Z {
				out = append(out, p)
			}
		} else {
			if !r.Contains(plugin.BlockPos{X: p.X + 1, Y: p.Y, Z: p.Z}) ||
				!r.Contains(plugin.BlockPos{X: p.X - 1, Y: p.Y, Z: p.Z}) ||
				!r.Contains(plugin.BlockPos{X: p.X, Y: p.Y, Z: p.Z + 1}) ||
				!r.Contains(plugin.BlockPos{X: p.X, Y: p.Y, Z: p.Z - 1}) {
				out = append(out, p)
			}
		}
		return true
	})
	return out
}

// Faces returns all blocks on all 6 faces (walls + floor + ceiling) of r.
func Faces(r Region) []plugin.BlockPos {
	lo, hi := r.Min(), r.Max()
	var out []plugin.BlockPos
	r.ForEach(func(p plugin.BlockPos) bool {
		if _, ok := r.(*Cuboid); ok {
			if p.X == lo.X || p.X == hi.X || p.Y == lo.Y || p.Y == hi.Y || p.Z == lo.Z || p.Z == hi.Z {
				out = append(out, p)
			}
		} else {
			if !r.Contains(plugin.BlockPos{X: p.X + 1, Y: p.Y, Z: p.Z}) ||
				!r.Contains(plugin.BlockPos{X: p.X - 1, Y: p.Y, Z: p.Z}) ||
				!r.Contains(plugin.BlockPos{X: p.X, Y: p.Y + 1, Z: p.Z}) ||
				!r.Contains(plugin.BlockPos{X: p.X, Y: p.Y - 1, Z: p.Z}) ||
				!r.Contains(plugin.BlockPos{X: p.X, Y: p.Y, Z: p.Z + 1}) ||
				!r.Contains(plugin.BlockPos{X: p.X, Y: p.Y, Z: p.Z - 1}) {
				out = append(out, p)
			}
		}
		return true
	})
	return out
}

// FormatPos formats a BlockPos as "(x, y, z)".
func FormatPos(p plugin.BlockPos) string {
	return fmt.Sprintf("(%d, %d, %d)", p.X, p.Y, p.Z)
}
