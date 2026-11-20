package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golem-mc/golem/nbt"
	"github.com/golem-mc/golem/plugin"
	"github.com/google/uuid"
	"github.com/pierrec/lz4/v4"
)

// BlockChange records a single reversible block mutation.
type BlockChange struct {
	Pos     plugin.BlockPos
	From    plugin.BlockState
	To      plugin.BlockState
	FromNBT nbt.Compound
	ToNBT   nbt.Compound
}

// ChangeSet holds the reversible history of one EditSession.
type ChangeSet struct {
	mu         sync.Mutex
	ID         int
	PlayerUUID uuid.UUID
	WorldName  string
	Command    string
	Timestamp  time.Time
	Min, Max   plugin.BlockPos
	hasBounds  bool
	changes    []BlockChange
	diskPath   string
	size       int64
	flushed    bool
	storeRedo  bool
}

// NewChangeSet creates an in-memory ChangeSet (optionally flushed to diskPath via Flush).
func NewChangeSet(id int, player uuid.UUID, worldName, command string, storeRedo bool, diskPath string) *ChangeSet {
	return &ChangeSet{
		ID:         id,
		PlayerUUID: player,
		WorldName:  worldName,
		Command:    command,
		Timestamp:  time.Now(),
		diskPath:   diskPath,
		storeRedo:  storeRedo,
	}
}

// Add appends a block change to the changeset.
func (cs *ChangeSet) Add(ch BlockChange) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if !cs.hasBounds {
		cs.Min, cs.Max = ch.Pos, ch.Pos
		cs.hasBounds = true
	} else {
		cs.Min.X = min(cs.Min.X, ch.Pos.X)
		cs.Min.Y = min(cs.Min.Y, ch.Pos.Y)
		cs.Min.Z = min(cs.Min.Z, ch.Pos.Z)
		cs.Max.X = max(cs.Max.X, ch.Pos.X)
		cs.Max.Y = max(cs.Max.Y, ch.Pos.Y)
		cs.Max.Z = max(cs.Max.Z, ch.Pos.Z)
	}
	cs.changes = append(cs.changes, ch)
	cs.size++
}

// Size returns the number of recorded changes.
func (cs *ChangeSet) Size() int64 {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.size
}

// Bounds returns the minimum and maximum changed positions.
func (cs *ChangeSet) Bounds() (minP, maxP plugin.BlockPos, ok bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.Min, cs.Max, cs.hasBounds
}

// DiskPath returns the path to the on-disk .bd file if disk history is enabled.
func (cs *ChangeSet) DiskPath() string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.diskPath
}

// DiskSize returns the on-disk byte size (0 if in memory).
func (cs *ChangeSet) DiskSize() int64 {
	cs.mu.Lock()
	p := cs.diskPath
	cs.mu.Unlock()
	if p == "" {
		return 0
	}
	fi, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// Flush writes the changeset to disk (LZ4 compressed) if diskPath is configured,
// releasing the in-memory slice to save RAM (matching FAWE's DiskStorageHistory).
func (cs *ChangeSet) Flush() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.diskPath == "" || cs.flushed || len(cs.changes) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(cs.diskPath), 0o755); err != nil {
		return err
	}
	data, err := encodeChangeSet(cs)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cs.diskPath, data, 0o644); err != nil {
		return err
	}
	cs.changes = nil
	cs.flushed = true
	return nil
}

// Changes returns all recorded changes (reading back from disk if flushed).
func (cs *ChangeSet) Changes() ([]BlockChange, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if !cs.flushed {
		return slices.Clone(cs.changes), nil
	}
	loaded, err := LoadChangeSetFromDisk(cs.diskPath)
	if err != nil {
		return nil, err
	}
	return loaded.changes, nil
}

// Distribution counts how many times each 'To' block state appears in the changeset.
func (cs *ChangeSet) Distribution() (map[plugin.BlockState]int64, error) {
	chs, err := cs.Changes()
	if err != nil {
		return nil, err
	}
	out := make(map[plugin.BlockState]int64)
	for _, ch := range chs {
		out[ch.To]++
	}
	return out, nil
}

func encodeChangeSet(cs *ChangeSet) ([]byte, error) {
	var raw bytes.Buffer
	raw.WriteString("FBD1")
	_ = binary.Write(&raw, binary.BigEndian, int32(cs.ID))
	raw.Write(cs.PlayerUUID[:])
	_ = binary.Write(&raw, binary.BigEndian, cs.Timestamp.UnixMilli())
	writeLenString(&raw, cs.WorldName)
	writeLenString(&raw, cs.Command)
	_ = binary.Write(&raw, binary.BigEndian, int32(cs.Min.X))
	_ = binary.Write(&raw, binary.BigEndian, int32(cs.Min.Y))
	_ = binary.Write(&raw, binary.BigEndian, int32(cs.Min.Z))
	_ = binary.Write(&raw, binary.BigEndian, int32(cs.Max.X))
	_ = binary.Write(&raw, binary.BigEndian, int32(cs.Max.Y))
	_ = binary.Write(&raw, binary.BigEndian, int32(cs.Max.Z))
	_ = binary.Write(&raw, binary.BigEndian, uint32(len(cs.changes)))
	for _, ch := range cs.changes {
		_ = binary.Write(&raw, binary.BigEndian, int32(ch.Pos.X))
		_ = binary.Write(&raw, binary.BigEndian, int32(ch.Pos.Y))
		_ = binary.Write(&raw, binary.BigEndian, int32(ch.Pos.Z))
		_ = binary.Write(&raw, binary.BigEndian, uint16(ch.From))
		_ = binary.Write(&raw, binary.BigEndian, uint16(ch.To))
		flags := byte(0)
		if ch.FromNBT != nil {
			flags |= 1
		}
		if ch.ToNBT != nil {
			flags |= 2
		}
		raw.WriteByte(flags)
		if ch.FromNBT != nil {
			b, err := nbt.Encode("", ch.FromNBT)
			if err != nil {
				return nil, err
			}
			_ = binary.Write(&raw, binary.BigEndian, uint32(len(b)))
			raw.Write(b)
		}
		if ch.ToNBT != nil {
			b, err := nbt.Encode("", ch.ToNBT)
			if err != nil {
				return nil, err
			}
			_ = binary.Write(&raw, binary.BigEndian, uint32(len(b)))
			raw.Write(b)
		}
	}
	var out bytes.Buffer
	zw := lz4.NewWriter(&out)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// LoadChangeSetFromDisk decodes an LZ4-compressed .bd history file.
func LoadChangeSetFromDisk(path string) (*ChangeSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	zr := lz4.NewReader(bytes.NewReader(data))
	var raw bytes.Buffer
	if _, err := raw.ReadFrom(zr); err != nil {
		return nil, err
	}
	r := bytes.NewReader(raw.Bytes())
	magic := make([]byte, 4)
	if _, err := r.Read(magic); err != nil || string(magic) != "FBD1" {
		return nil, fmt.Errorf("invalid history file %q", path)
	}
	var id int32
	if err := binary.Read(r, binary.BigEndian, &id); err != nil {
		return nil, err
	}
	var uid uuid.UUID
	if _, err := r.Read(uid[:]); err != nil {
		return nil, err
	}
	var tsMs int64
	if err := binary.Read(r, binary.BigEndian, &tsMs); err != nil {
		return nil, err
	}
	worldName, err := readLenString(r)
	if err != nil {
		return nil, err
	}
	cmd, err := readLenString(r)
	if err != nil {
		return nil, err
	}
	var minX, minY, minZ, maxX, maxY, maxZ int32
	for _, dst := range []*int32{&minX, &minY, &minZ, &maxX, &maxY, &maxZ} {
		if err := binary.Read(r, binary.BigEndian, dst); err != nil {
			return nil, err
		}
	}
	var count uint32
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return nil, err
	}
	cs := &ChangeSet{
		ID:         int(id),
		PlayerUUID: uid,
		WorldName:  worldName,
		Command:    cmd,
		Timestamp:  time.UnixMilli(tsMs),
		Min:        plugin.BlockPos{X: int(minX), Y: int(minY), Z: int(minZ)},
		Max:        plugin.BlockPos{X: int(maxX), Y: int(maxY), Z: int(maxZ)},
		hasBounds:  count > 0,
		size:       int64(count),
		diskPath:   path,
		changes:    make([]BlockChange, count),
		storeRedo:  true,
	}
	for i := uint32(0); i < count; i++ {
		var x, y, z int32
		var from, to uint16
		_ = binary.Read(r, binary.BigEndian, &x)
		_ = binary.Read(r, binary.BigEndian, &y)
		_ = binary.Read(r, binary.BigEndian, &z)
		_ = binary.Read(r, binary.BigEndian, &from)
		_ = binary.Read(r, binary.BigEndian, &to)
		flags, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		ch := BlockChange{
			Pos:  plugin.BlockPos{X: int(x), Y: int(y), Z: int(z)},
			From: plugin.BlockState(from),
			To:   plugin.BlockState(to),
		}
		if flags&1 != 0 {
			var sz uint32
			_ = binary.Read(r, binary.BigEndian, &sz)
			buf := make([]byte, sz)
			if _, err := r.Read(buf); err != nil {
				return nil, err
			}
			if _, tag, err := nbt.Decode(buf); err == nil {
				if comp, ok := tag.(nbt.Compound); ok {
					ch.FromNBT = comp
				}
			}
		}
		if flags&2 != 0 {
			var sz uint32
			_ = binary.Read(r, binary.BigEndian, &sz)
			buf := make([]byte, sz)
			if _, err := r.Read(buf); err != nil {
				return nil, err
			}
			if _, tag, err := nbt.Decode(buf); err == nil {
				if comp, ok := tag.(nbt.Compound); ok {
					ch.ToNBT = comp
				}
			}
		}
		cs.changes[i] = ch
	}
	return cs, nil
}

// HistoryFilePath returns FAWE's standard disk history path: <historyDir>/<world>/<uuid>/<id>.bd
func HistoryFilePath(historyDir, worldName string, player uuid.UUID, id int) string {
	return filepath.Join(historyDir, SanitizeWorldDir(worldName), player.String(), fmt.Sprintf("%d.bd", id))
}

// NextHistoryID scans the player's disk history directory to determine the next edit index.
func NextHistoryID(historyDir, worldName string, player uuid.UUID) int {
	dir := filepath.Join(historyDir, SanitizeWorldDir(worldName), player.String())
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	next := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bd") {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".bd")); err == nil && n >= next {
			next = n + 1
		}
	}
	return next
}

// PurgeOldHistory removes .bd files older than maxAge under historyDir.
func PurgeOldHistory(historyDir string, maxAge time.Duration) int {
	if maxAge <= 0 {
		return 0
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	_ = filepath.WalkDir(historyDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".bd") {
			return nil
		}
		if fi, err := d.Info(); err == nil && fi.ModTime().Before(cutoff) {
			if os.Remove(path) == nil {
				removed++
			}
		}
		return nil
	})
	return removed
}

// SanitizeWorldDir converts a dimension key like "minecraft:overworld" into a safe directory name.
func SanitizeWorldDir(worldName string) string {
	s := strings.TrimPrefix(worldName, "minecraft:")
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, "/", "_")
	if s == "" {
		return "world"
	}
	return s
}

func writeLenString(b *bytes.Buffer, s string) {
	_ = binary.Write(b, binary.BigEndian, uint16(len(s)))
	b.WriteString(s)
}

func readLenString(r *bytes.Reader) (string, error) {
	var n uint16
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return "", err
	}
	buf := make([]byte, n)
	if _, err := r.Read(buf); err != nil {
		return "", err
	}
	return string(buf), nil
}
