package worldedit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/core"
	"github.com/golem-mc/worldedit/internal/wetest"
	"github.com/golem-mc/worldedit/region"
)

func blockAt(w plugin.World, x, y, z int) string {
	return block.BlockName(w.BlockState(plugin.BlockPos{X: x, Y: y, Z: z}))
}

func TestSelectionSetReplaceUndoRedoAndHistory(t *testing.T) {
	env := wetest.Start(t)
	alice := env.Join("Alice", true)
	w := env.Srv.Host().Overworld()

	// Verify DataDir was resolved to an absolute path.
	if !filepath.IsAbs(env.WE.DataDir()) {
		t.Fatalf("DataDir should be absolute, got %q", env.WE.DataDir())
	}

	env.Expect(env.Run(alice, "//pos1 2,65,2"), "First position set")
	env.Expect(env.Run(alice, "//pos2 4,67,4"), "Second position set")
	env.Expect(env.Run(alice, "//size"), "Volume: 27")

	// Fill with stone
	env.Expect(env.Run(alice, "//set stone"), "27 blocks affected")
	if got := blockAt(w, 3, 66, 3); got != "minecraft:stone" {
		t.Fatalf("after //set stone, (3,66,3) = %s", got)
	}

	// Count & distr
	env.Expect(env.Run(alice, "//count stone"), "Counted 27 blocks")
	env.Expect(env.Run(alice, "//distr"), "minecraft:stone")

	// Replace stone with gold_block
	env.Expect(env.Run(alice, "//replace stone gold_block"), "27 blocks affected")
	if got := blockAt(w, 3, 66, 3); got != "minecraft:gold_block" {
		t.Fatalf("after //replace, (3,66,3) = %s", got)
	}

	// Undo -> back to stone
	env.Expect(env.Run(alice, "/undo"), "Undo successful")
	if got := blockAt(w, 3, 66, 3); got != "minecraft:stone" {
		t.Fatalf("after /undo, (3,66,3) = %s", got)
	}

	// Redo -> back to gold_block
	env.Expect(env.Run(alice, "/redo"), "Redo successful")
	if got := blockAt(w, 3, 66, 3); got != "minecraft:gold_block" {
		t.Fatalf("after /redo, (3,66,3) = %s", got)
	}

	// History DB list & rollback
	env.Expect(env.Run(alice, "//history list"), "//replace stone gold_block")
	env.Expect(env.Run(alice, "//history rollback -u Alice -r 50 -t 10m"), "Applied rollback")
	if got := blockAt(w, 3, 66, 3); got != "minecraft:air" {
		t.Fatalf("after rollback, (3,66,3) = %s, want minecraft:air", got)
	}
}

func TestClipboardRotateFlipAndSchematics(t *testing.T) {
	env := wetest.Start(t)
	alice := env.Join("Alice", true)
	w := env.Srv.Host().Overworld()

	// Build a 3x1x1 asymmetrical line of oak_stairs and diamond_block
	env.Run(alice, "//pos1 2,65,2")
	env.Run(alice, "//pos2 4,65,2")
	env.Run(alice, "//set stone")
	env.Run(alice, "//pos1 2,65,2")
	env.Run(alice, "//pos2 2,65,2")
	env.Run(alice, "//set oak_stairs[facing=north,half=bottom]")
	env.Run(alice, "//pos1 4,65,2")
	env.Run(alice, "//pos2 4,65,2")
	env.Run(alice, "//set diamond_block")

	// Select 2..4, 65, 2 and copy with origin at pos1 (toggleplace)
	env.Run(alice, "/toggleplace")
	env.Run(alice, "//pos1 2,65,2")
	env.Run(alice, "//pos2 4,65,2")
	env.Expect(env.Run(alice, "//copy"), "3 blocks were copied")

	// Save Sponge v3, Sponge v2, Vanilla .nbt, and PNG
	env.Expect(env.Run(alice, "/schem save house_v3"), "saved")
	// Saving again without -f should fail; with -f should overwrite
	env.Expect(env.Run(alice, "/schem save house_v3"), "already exists")
	env.Expect(env.Run(alice, "/schem save -f house_v3"), "overwriting")
	env.Expect(env.Run(alice, "/schem save fast.2 house_v2"), "saved")
	env.Expect(env.Run(alice, "/schem save structure house_nbt"), "saved")
	env.Expect(env.Run(alice, "/schem save png house_iso"), "saved")
	env.Expect(env.Run(alice, "/schem list"), "house_v3.schem")
	env.Expect(env.Run(alice, "/schem formats"), "FAST_V3")
	env.Expect(env.Run(alice, "/schem loadall house_v3,house_v2"), "Loaded 2 schematic")
	env.Expect(env.Run(alice, "/schem unload house_v2.schem"), "Unloaded")

	// Verify PNG file exists on disk
	if _, err := os.Stat(filepath.Join(env.WE.SchematicsDir(), "house_iso.png")); err != nil {
		t.Fatalf("house_iso.png missing: %v", err)
	}

	// Clear clipboard, reload house_v3, rotate 90 deg, paste at (10, 65, 10)
	env.Expect(env.Run(alice, "/clearclipboard"), "Clipboard cleared")
	env.Expect(env.Run(alice, "/schem load house_v3"), "loaded")
	env.Expect(env.Run(alice, "//rotate 90"), "rotated")
	env.Run(alice, "//pos1 10,65,10")
	env.Expect(env.Run(alice, "//paste"), "pasted")

	// Origin (0,0,0) had north-facing oak_stairs -> rotated 90° CW -> east-facing!
	st := w.BlockState(plugin.BlockPos{X: 10, Y: 65, Z: 10})
	if !strings.Contains(block.FormatState(st), "oak_stairs") || !strings.Contains(block.FormatState(st), "facing=east") {
		t.Fatalf("rotated pasted stairs at (10,65,10) = %s", block.FormatState(st))
	}
	// Offset (+2, 0, 0) rotated 90° CW around Y -> (0, 0, +2) -> (10, 65, 12) should be diamond_block
	if got := blockAt(w, 10, 65, 12); got != "minecraft:diamond_block" {
		t.Fatalf("rotated diamond_block at (10,65,12) = %s", got)
	}

	env.Expect(env.Run(alice, "/schem delete house_v2"), "deleted")
}

func TestGenerationRegionOpsAndUtility(t *testing.T) {
	env := wetest.Start(t)
	alice := env.Join("Alice", true)
	w := env.Srv.Host().Overworld()

	env.Run(alice, "/toggleplace")
	env.Run(alice, "//pos1 0,70,0")
	env.Expect(env.Run(alice, "//sphere glass 2"), "blocks affected")
	if got := blockAt(w, 0, 70, 0); got != "minecraft:glass" {
		t.Fatalf("sphere center = %s", got)
	}

	// Hollow walls + faces + center + generate formula
	env.Run(alice, "//pos1 10,65,10")
	env.Run(alice, "//pos2 14,67,14")
	env.Expect(env.Run(alice, "//walls bricks"), "blocks affected")
	if got := blockAt(w, 10, 65, 10); got != "minecraft:bricks" {
		t.Fatalf("wall corner = %s", got)
	}
	if got := blockAt(w, 12, 66, 12); got != "minecraft:air" {
		t.Fatalf("wall interior should remain air, got %s", got)
	}

	env.Expect(env.Run(alice, "//center emerald_block"), "1 blocks affected")
	if got := blockAt(w, 12, 66, 12); got != "minecraft:emerald_block" {
		t.Fatalf("center = %s", got)
	}

	// Math expression //calc and //generate (including negative coordinates)
	env.Expect(env.Run(alice, "//calc 6 * 7"), "= 42")
	env.Run(alice, "//pos1 -5,65,-5")
	env.Run(alice, "//pos2 -3,65,-3")
	env.Expect(env.Run(alice, "//generate iron_block x*x+z*z<=1"), "blocks affected")
	if got := blockAt(w, -4, 65, -4); got != "minecraft:iron_block" {
		t.Fatalf("generated center = %s", got)
	}

	// //stack & //move & //hollow
	env.Run(alice, "//pos1 20,65,20")
	env.Run(alice, "//pos2 22,67,22")
	env.Expect(env.Run(alice, "//set stone"), "27 blocks affected")
	env.Expect(env.Run(alice, "//hollow"), "1 blocks affected")
	if got := blockAt(w, 21, 66, 21); got != "minecraft:air" {
		t.Fatalf("hollow interior = %s, want air", got)
	}
	env.Expect(env.Run(alice, "//stack 1 east"), "blocks affected")
	if got := blockAt(w, 23, 65, 20); got != "minecraft:stone" {
		t.Fatalf("stacked corner = %s, want stone", got)
	}
	env.Expect(env.Run(alice, "//move 5 up"), "blocks affected")
	if got := blockAt(w, 20, 70, 20); got != "minecraft:stone" {
		t.Fatalf("moved corner = %s, want stone", got)
	}

	// Navigation /up places glass below feet
	env.Expect(env.Run(alice, "/up 5"), "Whoosh")
	// Info commands
	env.Expect(env.Run(alice, "/we version"), "FastAsyncWorldEdit")
	env.Expect(env.Run(alice, "/we reload"), "reloaded")
	env.Expect(env.Run(alice, "/biomelist"), "plains")
	env.Expect(env.Run(alice, "/biomeinfo -p"), "Biome at")
	env.Expect(env.Run(alice, "/chunkinfo"), "Chunk")
	env.Expect(env.Run(alice, "/masks"), "Masks:")
	env.Expect(env.Run(alice, "/transforms"), "Transforms:")
}

type testRegionMgr struct {
	allowed region.Region
}

func (m testRegionMgr) Name() string { return "testguard" }
func (m testRegionMgr) AllowedRegions(plugin.Player, plugin.World) []region.Region {
	return []region.Region{m.allowed}
}

func TestAPIServiceRegionRestrictionsPermissionsAndBrushes(t *testing.T) {
	env := wetest.Start(t)
	alice := env.Join("Alice", true)
	bob := env.Join("Bob", false)
	w := env.Srv.Host().Overworld()

	// Verify API service lookup
	api, ok := plugin.Service[worldedit.API](env.Srv.Host())
	if !ok {
		t.Fatal("worldedit.API service not registered")
	}

	// Non-op Bob is denied //set
	env.Expect(env.Run(bob, "//set stone"), "lacking the permission node")

	// Register an EditSessionEvent observer
	seenAfter := false
	plugin.Listen(env.Srv.Host(), func(e *core.EditSessionEvent) {
		if e.Stage == core.EditStageAfter && e.Changed > 0 {
			seenAfter = true
		}
	})

	// Register a RegionManager restricting edits to [20..22, 65..67, 20..22]
	api.RegisterRegionManager(testRegionMgr{
		allowed: region.NewCuboid(plugin.BlockPos{X: 20, Y: 65, Z: 20}, plugin.BlockPos{X: 22, Y: 67, Z: 22}),
	})

	// Alice has op (fawe.bypass), so test //wer selects the allowed region
	env.Expect(env.Run(alice, "//wer"), "Selection set to your current allowed region")
	sel, err := api.Selection(alice.Handle())
	if err != nil || sel.Min() != (plugin.BlockPos{X: 20, Y: 65, Z: 20}) {
		t.Fatalf("after //wer selection = %v, err=%v", sel, err)
	}
	env.Expect(env.Run(alice, "//set obsidian"), "27 blocks affected")
	if !seenAfter {
		t.Fatal("expected EditSessionEvent (after) to fire")
	}
	if got := blockAt(w, 21, 66, 21); got != "minecraft:obsidian" {
		t.Fatalf("(21,66,21) = %s", got)
	}

	// Wand & Brush binding + dual /primary + /secondary + persistence
	env.Expect(env.Run(alice, "//wand"), "Left click: select pos #1")
	env.Expect(env.Run(alice, "/br sphere sand 3"), "Equipped brush")
	env.Expect(env.Run(alice, "/secondary cylinder clay 2 2"), "Equipped left-click brush")
	env.Expect(env.Run(alice, "/size 4"), "Brush size set to 4")
	env.Expect(env.Run(alice, "/mask air"), "Brush mask set")
	env.Expect(env.Run(alice, "/br save mybrush"), "Saved brush")
	env.Expect(env.Run(alice, "//listbrush"), "mybrush")
	env.Expect(env.Run(alice, "/none"), "Tool unbound")
	env.Expect(env.Run(alice, "/br load mybrush"), "Equipped brush")
}

func TestSelectorsExpandUtilityAndAnvil(t *testing.T) {
	env := wetest.Start(t)
	alice := env.Join("Alice", true)
	w := env.Srv.Host().Overworld()

	// Expand, contract, shift, outset, inset, trim
	env.Run(alice, "//pos1 0,65,0")
	env.Run(alice, "//pos2 2,65,2")
	env.Expect(env.Run(alice, "//expand 2 up"), "Region expanded")
	env.Expect(env.Run(alice, "//contract 1 up"), "Region contracted")
	env.Expect(env.Run(alice, "//shift 1 east"), "Region shifted")
	env.Expect(env.Run(alice, "//outset 1"), "Region outset")
	env.Expect(env.Run(alice, "//inset 1"), "Region inset")

	// Polygon2D selector
	env.Expect(env.Run(alice, "//sel poly"), "Selection mode set to poly")
	env.Run(alice, "//pos1 0,65,0")
	env.Run(alice, "//pos2 4,65,0")
	env.Run(alice, "//pos2 2,65,4")
	env.Expect(env.Run(alice, "//set sandstone"), "blocks affected")
	if got := blockAt(w, 2, 65, 1); got != "minecraft:sandstone" {
		t.Fatalf("polygon interior = %s, want sandstone", got)
	}
	env.Expect(env.Run(alice, "//sel cuboid"), "Selection mode set to cuboid")

	// /snow, /thaw, /green, //drain, /anvil
	env.Run(alice, "//pos1 30,65,30")
	env.Run(alice, "//pos2 32,65,32")
	env.Expect(env.Run(alice, "/anvil set dirt"), "9 blocks affected")
	env.Expect(env.Run(alice, "/anvil count dirt"), "Counted 9 blocks")
	env.Expect(env.Run(alice, "/anvil replace dirt water"), "9 blocks affected")
	env.Run(alice, "/toggleplace")
	env.Run(alice, "//pos1 31,65,31")
	env.Expect(env.Run(alice, "//drain 5"), "9 block(s) have been drained")
	if got := blockAt(w, 31, 65, 31); got != "minecraft:air" {
		t.Fatalf("after //drain, (31,65,31) = %s, want air", got)
	}
}

func TestDoubleSlashCommandsAreNative(t *testing.T) {
	env := wetest.Start(t)
	alice := env.Join("Alice", true)
	found := false
	for _, c := range env.Srv.Host().Commands().All() {
		if c.Name == "/set" && c.Plugin == "worldedit" {
			found = true
		}
	}
	if !found {
		t.Fatal(`the command tree has no "/set"`)
	}
	env.Expect(env.Run(alice, "//pos1 2,65,2"), "First position set")
	env.Expect(env.Run(alice, "//pos2 3,65,3"), "Second position set")
	n := 0
	for _, m := range env.Run(alice, "//set stone") {
		if strings.Contains(m, "blocks affected") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("//set ran %d times", n)
	}
	env.Expect(env.Run(alice, "/worldedit:/set dirt"), "4 blocks affected")
	env.Expect(env.Run(alice, "/fawe:/set glass"), "4 blocks affected")
	if got := blockAt(env.Srv.Host().Overworld(), 2, 65, 2); got != "minecraft:glass" {
		t.Fatalf("got %s", got)
	}
	if out := env.Console("//set stone"); len(out) == 0 {
		t.Fatal("console //set produced no output")
	}
	if out := env.Console("/set stone"); len(out) == 0 {
		t.Fatal("console /set produced no output")
	}
}
