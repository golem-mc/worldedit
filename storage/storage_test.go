package storage

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image/png"
	"path/filepath"
	"testing"
	"time"

	"github.com/golem-mc/golem/nbt"
	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/region"
	"github.com/google/uuid"
)

func TestSchematicsRoundTrip(t *testing.T) {
	stone, _ := block.ParseState("stone", false)
	stairs, _ := block.ParseState("oak_stairs[facing=east,half=top]", false)
	r := region.NewCuboid(plugin.BlockPos{X: 10, Y: 64, Z: 20}, plugin.BlockPos{X: 12, Y: 66, Z: 22})
	cb := NewClipboard(r, plugin.BlockPos{X: 10, Y: 64, Z: 20})
	cb.SetRel(0, 0, 0, stone)
	cb.SetRel(1, 2, 1, stairs)
	cb.BlockEntities[plugin.BlockPos{X: 0, Y: 0, Z: 0}] = nbt.Compound{"id": nbt.String("minecraft:chest")}

	for _, fm := range []ClipboardFormat{FormatFastV3, FormatFastV2, FormatStructure} {
		var buf bytes.Buffer
		if err := WriteSchematic(&buf, cb, fm); err != nil {
			t.Fatalf("WriteSchematic(%s): %v", fm, err)
		}
		got, err := ReadSchematicBytes(buf.Bytes(), "")
		if err != nil {
			t.Fatalf("ReadSchematicBytes(%s auto): %v", fm, err)
		}
		if got.Width != 3 || got.Height != 3 || got.Length != 3 {
			t.Fatalf("%s dims = %dx%dx%d, want 3x3x3", fm, got.Width, got.Height, got.Length)
		}
		if got.GetRel(0, 0, 0) != stone || got.GetRel(1, 2, 1) != stairs {
			t.Fatalf("%s blocks mismatch: got (%v, %v)", fm, block.FormatState(got.GetRel(0, 0, 0)), block.FormatState(got.GetRel(1, 2, 1)))
		}
	}

	// Verify BlockEntity NBT preserved in Sponge v3
	var v3buf bytes.Buffer
	if err := WriteSchematic(&v3buf, cb, FormatFastV3); err != nil {
		t.Fatal(err)
	}
	v3cb, err := ReadSchematicBytes(v3buf.Bytes(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v3cb.BlockEntities[plugin.BlockPos{X: 0, Y: 0, Z: 0}]; !ok {
		t.Fatal("expected BlockEntity at (0,0,0) in Sponge v3")
	}

	// Verify legacy MCEdit .schematic reader (35:14 = red_wool, 1:0 = stone)
	mceRoot := nbt.Compound{
		"Materials": nbt.String("Alpha"),
		"Width":     nbt.Short(2),
		"Height":    nbt.Short(1),
		"Length":    nbt.Short(1),
		"Blocks":    nbt.ByteArray{1, 35},
		"Data":      nbt.ByteArray{0, 14},
	}
	var mceBuf bytes.Buffer
	if err := writeGzippedNBT(&mceBuf, "Schematic", mceRoot); err != nil {
		t.Fatal(err)
	}
	mceCB, err := ReadSchematicBytes(mceBuf.Bytes(), "")
	if err != nil {
		t.Fatalf("ReadSchematicBytes(MCEdit): %v", err)
	}
	if block.BlockName(mceCB.GetRel(0, 0, 0)) != "minecraft:stone" || block.BlockName(mceCB.GetRel(1, 0, 0)) != "minecraft:red_wool" {
		t.Fatalf("MCEdit blocks = (%s, %s)", block.BlockName(mceCB.GetRel(0, 0, 0)), block.BlockName(mceCB.GetRel(1, 0, 0)))
	}

	// PNG isometric export
	var pngBuf bytes.Buffer
	if err := WriteSchematic(&pngBuf, cb, FormatPNG); err != nil {
		t.Fatalf("WriteSchematic(PNG): %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(pngBuf.Bytes())); err != nil {
		t.Fatalf("invalid PNG output: %v", err)
	}

	// Disk clipboard round-trip
	diskPath := filepath.Join(t.TempDir(), "clip.bd")
	if err := cb.SaveToDisk(diskPath, true); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadClipboardFromDisk(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GetRel(1, 2, 1) != stairs {
		t.Fatalf("disk clipboard block mismatch")
	}
}

func TestDiskHistoryAndRollbackSQLite(t *testing.T) {
	dir := t.TempDir()
	uid := uuid.New()
	stone, _ := block.ParseState("stone", false)
	dirt, _ := block.ParseState("dirt", false)

	bdPath := HistoryFilePath(dir, "minecraft:overworld", uid, 0)
	cs := NewChangeSet(0, uid, "minecraft:overworld", "//set stone", true, bdPath)
	cs.Add(BlockChange{Pos: plugin.BlockPos{X: 5, Y: 64, Z: 5}, From: dirt, To: stone})
	cs.Add(BlockChange{Pos: plugin.BlockPos{X: 6, Y: 65, Z: 6}, From: dirt, To: stone})
	if err := cs.Flush(); err != nil {
		t.Fatal(err)
	}
	if cs.DiskSize() == 0 {
		t.Fatal("expected non-empty disk history file")
	}
	chs, err := cs.Changes()
	if err != nil || len(chs) != 2 || chs[0].To != stone {
		t.Fatalf("Changes() after flush = %v, err=%v", chs, err)
	}

	// Rollback SQLite database
	rdb, err := OpenRollbackDB(dir, "minecraft:overworld", 7)
	if err != nil {
		t.Fatal(err)
	}
	defer rdb.Close()

	lo, hi, _ := cs.Bounds()
	if err := rdb.LogEdit(EditSummary{
		Player:  uid,
		ID:      0,
		Time:    time.Now(),
		Min:     lo,
		Max:     hi,
		Size:    cs.Size(),
		Command: cs.Command,
		World:   "minecraft:overworld",
	}); err != nil {
		t.Fatal(err)
	}
	found, err := rdb.QueryEdits(&uid, time.Now().Add(-time.Hour), plugin.BlockPos{X: 5, Y: 64, Z: 5}, plugin.BlockPos{X: 5, Y: 64, Z: 5}, false, 10)
	if err != nil || len(found) != 1 || found[0].Command != "//set stone" || found[0].Min.Y != 64 {
		t.Fatalf("QueryEdits = %+v, err=%v", found, err)
	}

	// Test ImportFromDisk into a fresh DB
	if err := rdb.Delete(uid, 0); err != nil {
		t.Fatal(err)
	}
	imported, err := rdb.ImportFromDisk()
	if err != nil || imported != 1 {
		t.Fatalf("ImportFromDisk = %d, err=%v", imported, err)
	}
}

func TestReadAnvilChunk(t *testing.T) {
	// Synthesize a minimal 1-chunk .mca file with stone in section Y=4 (y=64..79).
	chunkNBT := nbt.Compound{
		"InhabitedTime": nbt.Long(42),
		"sections": nbt.List{
			Elem: nbt.TypeCompound,
			Items: []nbt.Tag{
				nbt.Compound{
					"Y": nbt.Byte(4),
					"block_states": nbt.Compound{
						"palette": nbt.List{
							Elem: nbt.TypeCompound,
							Items: []nbt.Tag{
								nbt.Compound{"Name": nbt.String("minecraft:stone")},
							},
						},
					},
				},
			},
		},
	}
	rawNBT, err := nbt.Encode("", chunkNBT)
	if err != nil {
		t.Fatal(err)
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	_, _ = zw.Write(rawNBT)
	_ = zw.Close()

	mca := make([]byte, 8192+4096)
	// Chunk (0,0) at sector 2, 1 sector
	binary.BigEndian.PutUint32(mca[0:4], (2<<8)|1)
	binary.BigEndian.PutUint32(mca[8192:8196], uint32(zbuf.Len()+1))
	mca[8196] = 2 // zlib
	copy(mca[8197:], zbuf.Bytes())

	ch, err := ReadAnvilChunk(mca, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ch.InhabitedTime != 42 {
		t.Fatalf("InhabitedTime = %d, want 42", ch.InhabitedTime)
	}
	stone, _ := block.ParseState("stone", false)
	if got := ch.BlockState(3, 64, 5); got != stone {
		t.Fatalf("anvil block = %q, want stone", block.FormatState(got))
	}
}
