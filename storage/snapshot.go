package storage

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/golem-mc/golem/nbt"
	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
)

// Snapshot describes a world backup (either a directory or a .zip archive containing Anvil region/*.mca files).
type Snapshot struct {
	Name  string
	Path  string
	IsZip bool
	Time  time.Time
}

// SnapshotRepo discovers snapshots in a configured directory.
type SnapshotRepo struct {
	Dir string
}

// List returns all discovered snapshots sorted newest-first.
func (r SnapshotRepo) List() ([]Snapshot, error) {
	if r.Dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Snapshot
	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(r.Dir, name)
		isZip := !e.IsDir() && strings.HasSuffix(strings.ToLower(name), ".zip")
		if !e.IsDir() && !isZip {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		ts := parseSnapshotTime(strings.TrimSuffix(name, ".zip"), fi.ModTime())
		out = append(out, Snapshot{Name: name, Path: full, IsZip: isZip, Time: ts})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

func parseSnapshotTime(name string, fallback time.Time) time.Time {
	layouts := []string{
		"2006-01-02-15-04-05",
		"2006-01-02_15-04-05",
		"2006-01-02",
		"20060102150405",
	}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, name, time.Local); err == nil {
			return t
		}
	}
	return fallback
}

// FindBefore returns the newest snapshot strictly before target.
func (r SnapshotRepo) FindBefore(target time.Time) (Snapshot, bool, error) {
	list, err := r.List()
	if err != nil {
		return Snapshot{}, false, err
	}
	for _, s := range list {
		if s.Time.Before(target) {
			return s, true, nil
		}
	}
	return Snapshot{}, false, nil
}

// FindAfter returns the oldest snapshot strictly after target.
func (r SnapshotRepo) FindAfter(target time.Time) (Snapshot, bool, error) {
	list, err := r.List()
	if err != nil {
		return Snapshot{}, false, err
	}
	for _, l := range slices.Backward(list) {
		if l.Time.After(target) {
			return l, true, nil
		}
	}
	return Snapshot{}, false, nil
}

// AnvilChunk holds decoded block states for one chunk (16xHeightx16) read from an Anvil .mca file.
type AnvilChunk struct {
	CX, CZ        int
	InhabitedTime int64
	Sections      map[int][]plugin.BlockState // sectionY -> 4096 states indexed (ly*16+lz)*16+lx
}

// BlockState reads the block at world coordinates inside this chunk.
func (c *AnvilChunk) BlockState(x, y, z int) plugin.BlockState {
	sy := y >> 4
	sec, ok := c.Sections[sy]
	if !ok {
		return 0
	}
	lx, ly, lz := x&15, y&15, z&15
	return sec[(ly*16+lz)*16+lx]
}

// ReadSnapshotChunk reads chunk (cx, cz) from snapshot s.
func ReadSnapshotChunk(s Snapshot, cx, cz int) (*AnvilChunk, error) {
	rx, rz := cx>>5, cz>>5
	mcaName := fmt.Sprintf("r.%d.%d.mca", rx, rz)
	var mcaBytes []byte
	var err error
	if s.IsZip {
		mcaBytes, err = readFileFromZip(s.Path, mcaName)
	} else {
		candidates := []string{
			filepath.Join(s.Path, "region", mcaName),
			filepath.Join(s.Path, "world", "region", mcaName),
			filepath.Join(s.Path, mcaName),
		}
		for _, p := range candidates {
			if data, rerr := os.ReadFile(p); rerr == nil {
				mcaBytes = data
				err = nil
				break
			} else {
				err = rerr
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return ReadAnvilChunk(mcaBytes, cx, cz)
}

func readFileFromZip(zipPath, suffix string) ([]byte, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/"+suffix) || f.Name == suffix {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, os.ErrNotExist
}

// ReadAnvilChunk extracts and decodes a single chunk (cx, cz) from raw .mca region bytes.
func ReadAnvilChunk(mca []byte, cx, cz int) (*AnvilChunk, error) {
	if len(mca) < 8192 {
		return nil, fmt.Errorf("anvil file too short")
	}
	idx := (cx & 31) + (cz&31)*32
	entry := binary.BigEndian.Uint32(mca[idx*4 : idx*4+4])
	if entry == 0 {
		return nil, os.ErrNotExist
	}
	sectorOffset := int(entry>>8) * 4096
	if sectorOffset+5 > len(mca) {
		return nil, fmt.Errorf("chunk sector offset out of range")
	}
	length := int(binary.BigEndian.Uint32(mca[sectorOffset : sectorOffset+4]))
	if length <= 1 || sectorOffset+4+length > len(mca) {
		return nil, fmt.Errorf("invalid chunk payload length %d", length)
	}
	compType := mca[sectorOffset+4]
	payload := mca[sectorOffset+5 : sectorOffset+4+length]
	var raw []byte
	var err error
	switch compType {
	case 1: // GZip
		gr, gerr := gzip.NewReader(bytes.NewReader(payload))
		if gerr != nil {
			return nil, gerr
		}
		raw, err = io.ReadAll(gr)
		_ = gr.Close()
	case 2: // Zlib
		zr, zerr := zlib.NewReader(bytes.NewReader(payload))
		if zerr != nil {
			return nil, zerr
		}
		raw, err = io.ReadAll(zr)
		_ = zr.Close()
	case 3: // Uncompressed
		raw = payload
	default:
		return nil, fmt.Errorf("unsupported anvil compression type %d", compType)
	}
	if err != nil {
		return nil, err
	}
	_, rootTag, err := nbt.Decode(raw)
	if err != nil {
		return nil, err
	}
	root, ok := rootTag.(nbt.Compound)
	if !ok {
		return nil, fmt.Errorf("chunk root is not a compound")
	}
	// Support both 1.18+ top-level `sections` and pre-1.18 `Level.Sections`
	if lvl, ok := nbt.Get[nbt.Compound](root, "Level"); ok {
		root = lvl
	}
	inhabited, _ := nbt.LongValue(root["InhabitedTime"])
	chunk := &AnvilChunk{
		CX:            cx,
		CZ:            cz,
		InhabitedTime: inhabited,
		Sections:      make(map[int][]plugin.BlockState),
	}

	secList, ok := nbt.Get[nbt.List](root, "sections")
	if !ok {
		secList, _ = nbt.Get[nbt.List](root, "Sections")
	}
	for _, item := range secList.Items {
		sc, ok := item.(nbt.Compound)
		if !ok {
			continue
		}
		sy, ok := nbt.ByteField(sc, "Y")
		if !ok {
			continue
		}
		var palList nbt.List
		var data nbt.LongArray
		if bs, ok := nbt.Get[nbt.Compound](sc, "block_states"); ok {
			palList, _ = nbt.Get[nbt.List](bs, "palette")
			data, _ = nbt.Get[nbt.LongArray](bs, "data")
		} else {
			palList, _ = nbt.Get[nbt.List](sc, "Palette")
			data, _ = nbt.Get[nbt.LongArray](sc, "BlockStates")
		}
		if len(palList.Items) == 0 {
			continue
		}
		palette := make([]plugin.BlockState, len(palList.Items))
		for i, pe := range palList.Items {
			if id, ok := registry.StateOfTag(pe); ok {
				palette[i] = plugin.BlockState(id)
			}
		}
		states := make([]plugin.BlockState, 4096)
		if len(palette) == 1 || len(data) == 0 {
			for i := range states {
				states[i] = palette[0]
			}
		} else {
			bitsPer := max(4, bits.Len(uint(len(palette)-1)))
			mask := uint64((1 << bitsPer) - 1)
			perLong := 64 / bitsPer
			for i := range 4096 {
				li := i / perLong
				bi := (i % perLong) * bitsPer
				if li < len(data) {
					pidx := int((uint64(data[li]) >> bi) & mask)
					if pidx < len(palette) {
						states[i] = palette[pidx]
					}
				}
			}
		}
		chunk.Sections[int(sy)] = states
	}
	return chunk, nil
}
