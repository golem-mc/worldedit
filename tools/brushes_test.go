package tools_test

import (
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/region"
	"github.com/golem-mc/worldedit/storage"
	"github.com/golem-mc/worldedit/tools"
)

func TestClipboardPasteRegionDiagonal(t *testing.T) {
	source := region.NewCuboid(plugin.BlockPos{X: 1, Z: 1}, plugin.BlockPos{X: 2, Z: 2})
	cb := storage.NewClipboard(source, plugin.BlockPos{})
	target := plugin.BlockPos{X: 8, Y: 70, Z: 8}
	got := tools.ClipboardPasteRegion(cb, storage.ClipboardTransform{RotY: 45}, target, true)

	// The corners other than the source min and max give the X extrema.
	wantMin := plugin.BlockPos{X: 7, Y: 70, Z: 9}
	wantMax := plugin.BlockPos{X: 9, Y: 70, Z: 11}
	if got.Min() != wantMin || got.Max() != wantMax {
		t.Fatalf("paste bounds %v -> %v, want %v -> %v", got.Min(), got.Max(), wantMin, wantMax)
	}
}
