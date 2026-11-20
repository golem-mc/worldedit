package block

import (
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/region"
)

type memExtent struct {
	blocks map[plugin.BlockPos]plugin.BlockState
	biome  registry.Biome
}

func (m *memExtent) BlockState(pos plugin.BlockPos) plugin.BlockState { return m.blocks[pos] }
func (m *memExtent) Biome(plugin.BlockPos) registry.Biome             { return m.biome }

func TestParseStateAndLegacy(t *testing.T) {
	stone, err := ParseState("stone", true)
	if err != nil {
		t.Fatal(err)
	}
	if FormatState(stone) != "minecraft:stone" {
		t.Fatalf("got %q", FormatState(stone))
	}

	stairs, err := ParseState("minecraft:oak_stairs[facing=north,half=top]", false)
	if err != nil {
		t.Fatal(err)
	}
	props := registry.StateProps(uint16(stairs))
	if props["facing"] != "north" || props["half"] != "top" {
		t.Fatalf("unexpected stairs props: %v", props)
	}

	// Legacy ID 35:14 -> red_wool
	wool, err := ParseState("35:14", true)
	if err != nil {
		t.Fatal(err)
	}
	if BlockName(wool) != "minecraft:red_wool" {
		t.Fatalf("35:14 = %q, want minecraft:red_wool", BlockName(wool))
	}
}

func TestRotateAndFlipState(t *testing.T) {
	stairs, err := ParseState("oak_stairs[facing=north,half=bottom]", false)
	if err != nil {
		t.Fatal(err)
	}
	rot90 := RotateStateY(stairs, 90)
	if p := registry.StateProps(uint16(rot90)); p["facing"] != "east" {
		t.Fatalf("rot90 facing = %q, want east", p["facing"])
	}
	flippedZ := FlipState(stairs, "z")
	if p := registry.StateProps(uint16(flippedZ)); p["facing"] != "south" {
		t.Fatalf("flipZ facing = %q, want south", p["facing"])
	}
	flippedY := FlipState(stairs, "y")
	if p := registry.StateProps(uint16(flippedY)); p["half"] != "top" {
		t.Fatalf("flipY half = %q, want top", p["half"])
	}
}

func TestMasksAndPatterns(t *testing.T) {
	stone, _ := ParseState("stone", false)
	dirt, _ := ParseState("dirt", false)
	air, _ := ParseState("air", false)

	ext := &memExtent{
		blocks: map[plugin.BlockPos]plugin.BlockState{
			{X: 0, Y: 0, Z: 0}: stone,
			{X: 0, Y: 1, Z: 0}: dirt,
			{X: 0, Y: 2, Z: 0}: air,
		},
	}
	ctx := ParseContext{
		Extent:      ext,
		Selection:   region.NewCuboid(plugin.BlockPos{X: 0, Y: 0, Z: 0}, plugin.BlockPos{X: 2, Y: 2, Z: 2}),
		AllowLegacy: true,
	}

	m, err := ParseMask("!air >stone", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Test(ext, plugin.BlockPos{X: 0, Y: 1, Z: 0}) {
		t.Fatal("dirt above stone should match '!air >stone'")
	}
	if m.Test(ext, plugin.BlockPos{X: 0, Y: 0, Z: 0}) {
		t.Fatal("stone above air should not match '!air >stone'")
	}

	exprMask, err := ParseMask("=x+y+z>0", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exprMask.Test(ext, plugin.BlockPos{X: 0, Y: 1, Z: 0}) || exprMask.Test(ext, plugin.BlockPos{X: 0, Y: 0, Z: 0}) {
		t.Fatal("expression mask mismatch")
	}

	pat, err := ParsePattern("#mask[stone][dirt][stone]", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pat.Apply(ext, plugin.BlockPos{X: 0, Y: 0, Z: 0}) != dirt {
		t.Fatal("#mask pattern should replace stone with dirt")
	}
	if pat.Apply(ext, plugin.BlockPos{X: 0, Y: 1, Z: 0}) != stone {
		t.Fatal("#mask pattern should replace non-stone with stone")
	}

	mix, err := ParsePattern("50%stone,50%dirt", ctx)
	if err != nil {
		t.Fatal(err)
	}
	seenStone, seenDirt := false, false
	for i := range 64 {
		st := mix.Apply(ext, plugin.BlockPos{X: i, Y: 0, Z: 0})
		if st == stone {
			seenStone = true
		} else if st == dirt {
			seenDirt = true
		}
	}
	if !seenStone || !seenDirt {
		t.Fatal("expected both stone and dirt from 50/50 pattern")
	}

	tr, err := ParseTransform("#offset[1][2][3]", ctx)
	if err != nil {
		t.Fatal(err)
	}
	np, _ := tr.Apply(plugin.BlockPos{X: 5, Y: 5, Z: 5}, stone, plugin.BlockPos{})
	if np != (plugin.BlockPos{X: 6, Y: 7, Z: 8}) {
		t.Fatalf("offset transform = %v", np)
	}
}
