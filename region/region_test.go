package region

import (
	"testing"

	"github.com/golem-mc/golem/plugin"
)

func TestCuboidOperations(t *testing.T) {
	c := NewCuboid(plugin.BlockPos{X: 10, Y: 5, Z: 20}, plugin.BlockPos{X: 0, Y: 1, Z: 10})
	if c.Min() != (plugin.BlockPos{X: 0, Y: 1, Z: 10}) {
		t.Fatalf("min = %v", c.Min())
	}
	if c.Max() != (plugin.BlockPos{X: 10, Y: 5, Z: 20}) {
		t.Fatalf("max = %v", c.Max())
	}
	if c.Volume() != 11*5*11 {
		t.Fatalf("volume = %d, want %d", c.Volume(), 11*5*11)
	}
	if err := c.Expand(plugin.BlockPos{X: 2, Y: -1, Z: 0}); err != nil {
		t.Fatal(err)
	}
	if c.Min() != (plugin.BlockPos{X: 0, Y: 0, Z: 10}) || c.Max() != (plugin.BlockPos{X: 12, Y: 5, Z: 20}) {
		t.Fatalf("after expand: min=%v max=%v", c.Min(), c.Max())
	}
	if err := c.Contract(plugin.BlockPos{X: 2, Y: 0, Z: 0}); err != nil {
		t.Fatal(err)
	}
	if c.Min().X != 2 {
		t.Fatalf("after contract min.X = %d, want 2", c.Min().X)
	}
	if err := c.Shift(plugin.BlockPos{X: 1, Y: 2, Z: 3}); err != nil {
		t.Fatal(err)
	}
	if c.Min() != (plugin.BlockPos{X: 3, Y: 2, Z: 13}) {
		t.Fatalf("after shift min = %v", c.Min())
	}

	small := NewCuboid(plugin.BlockPos{X: 0, Y: 0, Z: 0}, plugin.BlockPos{X: 2, Y: 2, Z: 2})
	if got := len(Walls(small)); got != 24 {
		t.Fatalf("walls count = %d, want 24", got)
	}
	if got := len(Faces(small)); got != 26 {
		t.Fatalf("faces count = %d, want 26", got)
	}
}

func TestPolygon2D(t *testing.T) {
	poly := NewPolygon2D([]Point2D{{0, 0}, {4, 0}, {4, 4}, {0, 4}}, 10, 12)
	if !poly.Contains(plugin.BlockPos{X: 2, Y: 11, Z: 2}) {
		t.Fatal("center should be inside polygon")
	}
	if poly.Contains(plugin.BlockPos{X: 5, Y: 11, Z: 2}) {
		t.Fatal("outside point should not be inside polygon")
	}
	if poly.Contains(plugin.BlockPos{X: 2, Y: 15, Z: 2}) {
		t.Fatal("above maxY should not be inside polygon")
	}
	if poly.Volume() != 25*3 {
		t.Fatalf("volume = %d, want 75", poly.Volume())
	}
}

func TestEllipsoidAndCylinder(t *testing.T) {
	e := NewEllipsoid(plugin.BlockPos{X: 0, Y: 0, Z: 0}, plugin.Vec3{X: 3, Y: 3, Z: 3})
	if !e.Contains(plugin.BlockPos{X: 0, Y: 0, Z: 0}) || !e.Contains(plugin.BlockPos{X: 3, Y: 0, Z: 0}) {
		t.Fatal("center and axis points should be inside sphere")
	}
	if e.Contains(plugin.BlockPos{X: 3, Y: 3, Z: 3}) {
		t.Fatal("corner (3,3,3) should be outside sphere of radius 3")
	}

	cyl := NewCylinder(plugin.BlockPos{X: 0, Y: 5, Z: 0}, 2, 2, 5, 7)
	if !cyl.Contains(plugin.BlockPos{X: 2, Y: 6, Z: 0}) {
		t.Fatal("edge point should be inside cylinder")
	}
	if cyl.Contains(plugin.BlockPos{X: 2, Y: 8, Z: 0}) {
		t.Fatal("above maxY should be outside cylinder")
	}
}

func TestConvexPolyhedron(t *testing.T) {
	tetra := NewConvexPolyhedron([]plugin.BlockPos{
		{X: 0, Y: 0, Z: 0},
		{X: 6, Y: 0, Z: 0},
		{X: 0, Y: 6, Z: 0},
		{X: 0, Y: 0, Z: 6},
	})
	if !tetra.Contains(plugin.BlockPos{X: 1, Y: 1, Z: 1}) {
		t.Fatal("(1,1,1) should be inside tetrahedron")
	}
	if tetra.Contains(plugin.BlockPos{X: 4, Y: 4, Z: 4}) {
		t.Fatal("(4,4,4) should be outside tetrahedron")
	}
}

func TestSelectorsAndCUI(t *testing.T) {
	sel := NewSelector(SelectorCuboid)
	if sel.IsDefined() {
		t.Fatal("fresh cuboid selector should not be defined")
	}
	sel.SelectPrimary(plugin.BlockPos{X: 1, Y: 2, Z: 3}, 0)
	sel.SelectSecondary(plugin.BlockPos{X: 4, Y: 5, Z: 6}, 0)
	if !sel.IsDefined() {
		t.Fatal("cuboid selector should be defined")
	}
	if evs := sel.CUIEvents(); len(evs) != 3 || evs[0] != "s|cuboid" {
		t.Fatalf("unexpected CUI events: %v", evs)
	}

	ext := NewSelector(SelectorExtend)
	ext.SelectPrimary(plugin.BlockPos{X: 5, Y: 5, Z: 5}, 0)
	ext.SelectSecondary(plugin.BlockPos{X: 8, Y: 7, Z: 6}, 0)
	ext.SelectSecondary(plugin.BlockPos{X: 2, Y: 3, Z: 9}, 0)
	r, err := ext.Region()
	if err != nil {
		t.Fatal(err)
	}
	if r.Min() != (plugin.BlockPos{X: 2, Y: 3, Z: 5}) || r.Max() != (plugin.BlockPos{X: 8, Y: 7, Z: 9}) {
		t.Fatalf("extend bounds = %v..%v", r.Min(), r.Max())
	}
}
