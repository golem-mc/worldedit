// Package storage implements WorldEdit/FAWE clipboards, schematic I/O
// (Sponge v1/v2/v3, MCEdit .schematic, Minecraft Structure .nbt, isometric PNG),
// memory & LZ4-compressed disk history, the SQLite rollback summary database,
// and Anvil .mca snapshot reading.
package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"

	"github.com/golem-mc/golem/nbt"
	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/region"
	"github.com/google/uuid"
	"github.com/pierrec/lz4/v4"
)

// StoredEntity captures an entity stored in a clipboard or schematic.
type StoredEntity struct {
	Type  string
	Pos   plugin.Vec3 // relative to clipboard Min
	Yaw   float32
	Pitch float32
	Data  nbt.Compound
}

// Clipboard holds a copied 3D volume of blocks, optional block entity NBT,
// optional biomes, and optional entities.
type Clipboard struct {
	ID            uuid.UUID
	Min           plugin.BlockPos
	Origin        plugin.BlockPos
	Width         int
	Height        int
	Length        int
	Blocks        []plugin.BlockState
	BlockEntities map[plugin.BlockPos]nbt.Compound // keyed by relative (x,y,z) from Min
	Biomes        []registry.Biome                 // optional, same indexing as Blocks
	Entities      []StoredEntity
	Name          string // optional schematic source filename
	DiskPath      string
}

// NewClipboard allocates a clipboard covering the bounding box of r with origin.
func NewClipboard(r region.Region, origin plugin.BlockPos) *Clipboard {
	lo := r.Min()
	w, h, l := r.Width(), r.Height(), r.Length()
	return &Clipboard{
		ID:            uuid.New(),
		Min:           lo,
		Origin:        origin,
		Width:         w,
		Height:        h,
		Length:        l,
		Blocks:        make([]plugin.BlockState, w*h*l),
		BlockEntities: make(map[plugin.BlockPos]nbt.Compound),
	}
}

func (c *Clipboard) index(rx, ry, rz int) int {
	return (ry*c.Length+rz)*c.Width + rx
}

// InBounds reports whether relative coordinates (rx, ry, rz) lie inside the clipboard.
func (c *Clipboard) InBounds(rx, ry, rz int) bool {
	return rx >= 0 && rx < c.Width && ry >= 0 && ry < c.Height && rz >= 0 && rz < c.Length
}

// SetRel sets the block at relative coordinates (0..Width-1, 0..Height-1, 0..Length-1).
func (c *Clipboard) SetRel(rx, ry, rz int, st plugin.BlockState) {
	if !c.InBounds(rx, ry, rz) {
		return
	}
	c.Blocks[c.index(rx, ry, rz)] = st
}

// GetRel gets the block at relative coordinates.
func (c *Clipboard) GetRel(rx, ry, rz int) plugin.BlockState {
	if !c.InBounds(rx, ry, rz) {
		return 0
	}
	return c.Blocks[c.index(rx, ry, rz)]
}

// SetRelBiome sets the biome at relative coordinates, allocating the biome slice on first use.
func (c *Clipboard) SetRelBiome(rx, ry, rz int, b registry.Biome) {
	if !c.InBounds(rx, ry, rz) {
		return
	}
	if len(c.Biomes) == 0 {
		c.Biomes = make([]registry.Biome, c.Width*c.Height*c.Length)
		for i := range c.Biomes {
			c.Biomes[i] = registry.NoBiome
		}
	}
	c.Biomes[c.index(rx, ry, rz)] = b
}

// GetRelBiome gets the biome at relative coordinates.
func (c *Clipboard) GetRelBiome(rx, ry, rz int) registry.Biome {
	if len(c.Biomes) == 0 || !c.InBounds(rx, ry, rz) {
		return registry.NoBiome
	}
	return c.Biomes[c.index(rx, ry, rz)]
}

// BlockAt implements block.ClipboardLookup.
func (c *Clipboard) BlockAt(rel plugin.BlockPos) (plugin.BlockState, bool) {
	if !c.InBounds(rel.X, rel.Y, rel.Z) {
		return 0, false
	}
	return c.GetRel(rel.X, rel.Y, rel.Z), true
}

// Dimensions implements block.ClipboardLookup.
func (c *Clipboard) Dimensions() (w, h, l int) {
	return c.Width, c.Height, c.Length
}

// Volume returns Width * Height * Length.
func (c *Clipboard) Volume() int64 {
	return int64(c.Width) * int64(c.Height) * int64(c.Length)
}

// Offset returns Min - Origin (the WorldEdit paste offset).
func (c *Clipboard) Offset() plugin.BlockPos {
	return plugin.BlockPos{
		X: c.Min.X - c.Origin.X,
		Y: c.Min.Y - c.Origin.Y,
		Z: c.Min.Z - c.Origin.Z,
	}
}

// SaveToDisk writes a compact LZ4-compressed binary clipboard file (FAWE disk clipboard).
func (c *Clipboard) SaveToDisk(path string, saveNBT bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var raw bytes.Buffer
	raw.WriteString("FCBD") // FAWE Clipboard Disk magic
	_ = binary.Write(&raw, binary.BigEndian, uint32(1))
	_ = binary.Write(&raw, binary.BigEndian, int32(c.Min.X))
	_ = binary.Write(&raw, binary.BigEndian, int32(c.Min.Y))
	_ = binary.Write(&raw, binary.BigEndian, int32(c.Min.Z))
	_ = binary.Write(&raw, binary.BigEndian, int32(c.Origin.X))
	_ = binary.Write(&raw, binary.BigEndian, int32(c.Origin.Y))
	_ = binary.Write(&raw, binary.BigEndian, int32(c.Origin.Z))
	_ = binary.Write(&raw, binary.BigEndian, int32(c.Width))
	_ = binary.Write(&raw, binary.BigEndian, int32(c.Height))
	_ = binary.Write(&raw, binary.BigEndian, int32(c.Length))
	for _, st := range c.Blocks {
		_ = binary.Write(&raw, binary.BigEndian, uint16(st))
	}
	if saveNBT && len(c.BlockEntities) > 0 {
		_ = binary.Write(&raw, binary.BigEndian, uint32(len(c.BlockEntities)))
		for pos, comp := range c.BlockEntities {
			_ = binary.Write(&raw, binary.BigEndian, int32(pos.X))
			_ = binary.Write(&raw, binary.BigEndian, int32(pos.Y))
			_ = binary.Write(&raw, binary.BigEndian, int32(pos.Z))
			enc, err := nbt.Encode("", comp)
			if err != nil {
				return err
			}
			_ = binary.Write(&raw, binary.BigEndian, uint32(len(enc)))
			raw.Write(enc)
		}
	} else {
		_ = binary.Write(&raw, binary.BigEndian, uint32(0))
	}
	var compressed bytes.Buffer
	zw := lz4.NewWriter(&compressed)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	c.DiskPath = path
	return os.WriteFile(path, compressed.Bytes(), 0o644)
}

// LoadClipboardFromDisk reads a clipboard written by SaveToDisk.
func LoadClipboardFromDisk(path string) (*Clipboard, error) {
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
	if _, err := r.Read(magic); err != nil || string(magic) != "FCBD" {
		return nil, fmt.Errorf("invalid disk clipboard magic")
	}
	var ver uint32
	if err := binary.Read(r, binary.BigEndian, &ver); err != nil || ver != 1 {
		return nil, fmt.Errorf("unsupported disk clipboard version %d", ver)
	}
	var minX, minY, minZ, origX, origY, origZ, w, h, l int32
	for _, dst := range []*int32{&minX, &minY, &minZ, &origX, &origY, &origZ, &w, &h, &l} {
		if err := binary.Read(r, binary.BigEndian, dst); err != nil {
			return nil, err
		}
	}
	n := int(w) * int(h) * int(l)
	cb := &Clipboard{
		ID:            uuid.New(),
		Min:           plugin.BlockPos{X: int(minX), Y: int(minY), Z: int(minZ)},
		Origin:        plugin.BlockPos{X: int(origX), Y: int(origY), Z: int(origZ)},
		Width:         int(w),
		Height:        int(h),
		Length:        int(l),
		Blocks:        make([]plugin.BlockState, n),
		BlockEntities: make(map[plugin.BlockPos]nbt.Compound),
		DiskPath:      path,
	}
	for i := 0; i < n; i++ {
		var st uint16
		if err := binary.Read(r, binary.BigEndian, &st); err != nil {
			return nil, err
		}
		cb.Blocks[i] = plugin.BlockState(st)
	}
	var beCount uint32
	if err := binary.Read(r, binary.BigEndian, &beCount); err == nil {
		for i := uint32(0); i < beCount; i++ {
			var x, y, z int32
			var sz uint32
			_ = binary.Read(r, binary.BigEndian, &x)
			_ = binary.Read(r, binary.BigEndian, &y)
			_ = binary.Read(r, binary.BigEndian, &z)
			_ = binary.Read(r, binary.BigEndian, &sz)
			buf := make([]byte, sz)
			if _, err := r.Read(buf); err != nil {
				return nil, err
			}
			_, tag, err := nbt.Decode(buf)
			if err == nil {
				if comp, ok := tag.(nbt.Compound); ok {
					cb.BlockEntities[plugin.BlockPos{X: int(x), Y: int(y), Z: int(z)}] = comp
				}
			}
		}
	}
	return cb, nil
}

// ClipboardTransform records non-destructive rotations and flips applied to a ClipboardHolder.
type ClipboardTransform struct {
	RotY  int // degrees CW around Y
	RotX  int
	RotZ  int
	FlipX bool
	FlipY bool
	FlipZ bool
}

// IsIdentity reports whether no rotation or flip is active.
func (t ClipboardTransform) IsIdentity() bool {
	return t.RotY%360 == 0 && t.RotX%360 == 0 && t.RotZ%360 == 0 && !t.FlipX && !t.FlipY && !t.FlipZ
}

// TransformOffset transforms a relative offset vector (dx, dy, dz) from the clipboard origin.
func (t ClipboardTransform) TransformOffset(v plugin.Vec3) plugin.Vec3 {
	x, y, z := v.X, v.Y, v.Z
	if t.FlipX {
		x = -x
	}
	if t.FlipY {
		y = -y
	}
	if t.FlipZ {
		z = -z
	}
	if t.RotX%360 != 0 {
		rad := float64(t.RotX) * math.Pi / 180.0
		c, s := math.Cos(rad), math.Sin(rad)
		y, z = y*c-z*s, y*s+z*c
	}
	if t.RotZ%360 != 0 {
		rad := float64(t.RotZ) * math.Pi / 180.0
		c, s := math.Cos(rad), math.Sin(rad)
		x, y = x*c-y*s, x*s+y*c
	}
	if t.RotY%360 != 0 {
		rad := float64(t.RotY) * math.Pi / 180.0
		c, s := math.Cos(rad), math.Sin(rad)
		x, z = x*c-z*s, x*s+z*c
	}
	return plugin.Vec3{X: x, Y: y, Z: z}
}

// TransformBlock rotates and flips a BlockState according to t.
func (t ClipboardTransform) TransformBlock(st plugin.BlockState) plugin.BlockState {
	if t.FlipX {
		st = block.FlipState(st, "x")
	}
	if t.FlipY {
		st = block.FlipState(st, "y")
	}
	if t.FlipZ {
		st = block.FlipState(st, "z")
	}
	if t.RotY%360 != 0 {
		st = block.RotateStateY(st, t.RotY)
	}
	return st
}

// ClipboardHolder manages one or more clipboards (multi-clipboard support) plus non-destructive transform.
type ClipboardHolder struct {
	mu         sync.RWMutex
	Clipboards []*Clipboard
	Transform  ClipboardTransform
	LazyCut    region.Region // non-nil when //lazycut is pending removal on paste
}

// NewClipboardHolder wraps a single clipboard.
func NewClipboardHolder(cb *Clipboard) *ClipboardHolder {
	return &ClipboardHolder{Clipboards: []*Clipboard{cb}}
}

// Primary returns the first clipboard, or nil if empty.
func (h *ClipboardHolder) Primary() *Clipboard {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.Clipboards) == 0 {
		return nil
	}
	return h.Clipboards[0]
}

// Pick returns a random clipboard if multiple are loaded (via //schem loadall), else the single one.
func (h *ClipboardHolder) Pick() *Clipboard {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.Clipboards) == 0 {
		return nil
	}
	if len(h.Clipboards) == 1 {
		return h.Clipboards[0]
	}
	return h.Clipboards[rand.IntN(len(h.Clipboards))]
}

// Add appends a clipboard (used by //schem loadall).
func (h *ClipboardHolder) Add(cb *Clipboard) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Clipboards = append(h.Clipboards, cb)
}

// UnloadByName removes matching clipboards from a multi-clipboard holder.
func (h *ClipboardHolder) UnloadByName(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	removed := false
	for _, cb := range h.Clipboards {
		if cb.Name == name {
			removed = true
			continue
		}
		h.Clipboards[n] = cb
		n++
	}
	h.Clipboards = h.Clipboards[:n]
	return removed
}

// Rotate adds a non-destructive rotation in degrees.
func (h *ClipboardHolder) Rotate(rotY, rotX, rotZ int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Transform.RotY = (h.Transform.RotY + rotY) % 360
	h.Transform.RotX = (h.Transform.RotX + rotX) % 360
	h.Transform.RotZ = (h.Transform.RotZ + rotZ) % 360
}

// Flip toggles flipping along axis ("x", "y", or "z").
func (h *ClipboardHolder) Flip(axis string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch axis {
	case "x":
		h.Transform.FlipX = !h.Transform.FlipX
	case "y":
		h.Transform.FlipY = !h.Transform.FlipY
	case "z":
		h.Transform.FlipZ = !h.Transform.FlipZ
	}
}
