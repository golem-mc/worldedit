package storage

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golem-mc/golem/nbt"
	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/block"
	"github.com/google/uuid"
)

// ClipboardFormat identifies a schematic / clipboard serialization format.
type ClipboardFormat string

const (
	FormatFastV3    ClipboardFormat = "FAST_V3"
	FormatFastV2    ClipboardFormat = "FAST_V2"
	FormatSpongeV1  ClipboardFormat = "SPONGE_V1_SCHEMATIC"
	FormatMCEdit    ClipboardFormat = "MCEDIT_SCHEMATIC"
	FormatStructure ClipboardFormat = "MINECRAFT_STRUCTURE"
	FormatPNG       ClipboardFormat = "PNG"
)

// FormatInfo describes a supported schematic format and its upstream aliases.
type FormatInfo struct {
	Format    ClipboardFormat
	Extension string
	Aliases   []string
	CanRead   bool
	CanWrite  bool
}

// AllFormats lists all supported clipboard formats in FAWE priority order.
var AllFormats = []FormatInfo{
	{Format: FormatFastV3, Extension: "schem", Aliases: []string{"fast", "fawe", "schem", "sponge.3", "slow", "safe", "schem3", "sponge3", "fast3"}, CanRead: true, CanWrite: true},
	{Format: FormatFastV2, Extension: "schem", Aliases: []string{"fast.2", "fawe.2", "schem.2", "sponge.2", "slow.2", "safe.2", "schem2", "sponge2", "fast2", "sponge"}, CanRead: true, CanWrite: true},
	{Format: FormatSpongeV1, Extension: "schem", Aliases: []string{"sponge.1", "schem1", "sponge1"}, CanRead: true, CanWrite: false},
	{Format: FormatMCEdit, Extension: "schematic", Aliases: []string{"mcedit", "mce", "schematic"}, CanRead: true, CanWrite: false},
	{Format: FormatStructure, Extension: "nbt", Aliases: []string{"structure", "nbt"}, CanRead: true, CanWrite: true},
	{Format: FormatPNG, Extension: "png", Aliases: []string{"png", "image"}, CanRead: false, CanWrite: true},
}

// FindFormat looks up a ClipboardFormat by name or alias.
func FindFormat(name string) (FormatInfo, bool) {
	q := strings.ToLower(strings.TrimSpace(name))
	for _, f := range AllFormats {
		if strings.ToLower(string(f.Format)) == q {
			return f, true
		}
		for _, a := range f.Aliases {
			if a == q {
				return f, true
			}
		}
	}
	return FormatInfo{}, false
}

// WriteSchematic writes cb to w in the requested format.
func WriteSchematic(w io.Writer, cb *Clipboard, format ClipboardFormat) error {
	switch format {
	case FormatFastV3, "":
		return writeSpongeV3(w, cb)
	case FormatFastV2:
		return writeSpongeV2(w, cb)
	case FormatStructure:
		return writeStructureNBT(w, cb)
	case FormatPNG:
		return writeIsometricPNG(w, cb)
	case FormatMCEdit:
		return fmt.Errorf("the formats .schematic, .mcedit and .mce are discontinued; use .schem (Sponge v2/v3)")
	default:
		return fmt.Errorf("format %s does not support writing", format)
	}
}

// SaveSchematicFile saves cb to disk in format (defaulting to FAST_V3 / .schem).
func SaveSchematicFile(path string, cb *Clipboard, format ClipboardFormat) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return WriteSchematic(f, cb, format)
}

// ReadSchematicFile reads a schematic from path, auto-detecting the format if format == "".
func ReadSchematicFile(path string, format ClipboardFormat) (*Clipboard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cb, err := ReadSchematicBytes(data, format)
	if err != nil {
		return nil, err
	}
	cb.Name = filepath.Base(path)
	return cb, nil
}

// ReadSchematicBytes decodes a schematic from raw bytes (auto-detecting if format == "").
func ReadSchematicBytes(data []byte, format ClipboardFormat) (*Clipboard, error) {
	raw, err := maybeGunzip(data)
	if err != nil {
		return nil, fmt.Errorf("decompress schematic: %w", err)
	}
	rootName, rootTag, err := nbt.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode schematic NBT: %w", err)
	}
	root, ok := rootTag.(nbt.Compound)
	if !ok {
		return nil, fmt.Errorf("schematic root tag is not a compound")
	}

	if format == "" {
		format = detectFormatNBT(rootName, root)
	}
	switch format {
	case FormatFastV3:
		return readSpongeV3(root)
	case FormatFastV2:
		return readSpongeV2(root)
	case FormatSpongeV1:
		return readSpongeV1(root)
	case FormatMCEdit:
		return readMCEdit(root)
	case FormatStructure:
		return readStructureNBT(root)
	default:
		return nil, fmt.Errorf("unrecognized or unsupported schematic format %q", format)
	}
}

func detectFormatNBT(rootName string, root nbt.Compound) ClipboardFormat {
	if schem, ok := nbt.Get[nbt.Compound](root, "Schematic"); ok {
		if ver, ok := nbt.IntField(schem, "Version"); ok && ver == 3 {
			return FormatFastV3
		}
	}
	if ver, ok := nbt.IntField(root, "Version"); ok {
		switch ver {
		case 3:
			return FormatFastV3
		case 2:
			return FormatFastV2
		case 1:
			return FormatSpongeV1
		}
	}
	if _, ok := root["Materials"]; ok {
		return FormatMCEdit
	}
	if _, ok := root["size"]; ok {
		if _, ok := root["palette"]; ok {
			return FormatStructure
		}
	}
	_ = rootName
	return FormatFastV3
}

func maybeGunzip(data []byte) ([]byte, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		return io.ReadAll(gr)
	}
	return data, nil
}

func writeGzippedNBT(w io.Writer, rootName string, root nbt.Compound) error {
	raw, err := nbt.Encode(rootName, root)
	if err != nil {
		return err
	}
	gw := gzip.NewWriter(w)
	if _, err := gw.Write(raw); err != nil {
		_ = gw.Close()
		return err
	}
	return gw.Close()
}

func appendVarInt(dst []byte, v int) []byte {
	uv := uint32(v)
	for uv >= 0x80 {
		dst = append(dst, byte(uv&0x7f)|0x80)
		uv >>= 7
	}
	return append(dst, byte(uv))
}

func readVarInts(data []byte, count int) ([]int, error) {
	out := make([]int, 0, count)
	i := 0
	for i < len(data) && len(out) < count {
		var val uint32
		var shift uint
		for {
			if i >= len(data) {
				return out, fmt.Errorf("truncated varint at index %d", len(out))
			}
			b := data[i]
			i++
			val |= uint32(b&0x7f) << shift
			if b&0x80 == 0 {
				break
			}
			shift += 7
			if shift > 35 {
				return out, fmt.Errorf("varint too long")
			}
		}
		out = append(out, int(val))
	}
	return out, nil
}

// --- Sponge Schematic Version 3 ---

func writeSpongeV3(w io.Writer, cb *Clipboard) error {
	offset := cb.Offset()
	palette := make(map[string]int)
	paletteCompound := nbt.Compound{}
	var blockData []byte

	for y := 0; y < cb.Height; y++ {
		for z := 0; z < cb.Length; z++ {
			for x := 0; x < cb.Width; x++ {
				key := block.FormatState(cb.GetRel(x, y, z))
				id, ok := palette[key]
				if !ok {
					id = len(palette)
					palette[key] = id
					paletteCompound[key] = nbt.Int(id)
				}
				blockData = appendVarInt(blockData, id)
			}
		}
	}

	blocksTag := nbt.Compound{
		"Palette": paletteCompound,
		"Data":    nbt.ByteArray(blockData),
	}
	if len(cb.BlockEntities) > 0 {
		items := make([]nbt.Tag, 0, len(cb.BlockEntities))
		for pos, comp := range cb.BlockEntities {
			be := nbt.Compound{
				"Pos": nbt.IntArray{int32(pos.X), int32(pos.Y), int32(pos.Z)},
			}
			if id, ok := nbt.Get[nbt.String](comp, "id"); ok {
				be["Id"] = id
			} else if id, ok := nbt.Get[nbt.String](comp, "Id"); ok {
				be["Id"] = id
			}
			be["Data"] = nbt.Copy(comp)
			items = append(items, be)
		}
		blocksTag["BlockEntities"] = nbt.List{Elem: nbt.TypeCompound, Items: items}
	}

	schem := nbt.Compound{
		"Version":     nbt.Int(3),
		"DataVersion": nbt.Int(4189),
		"Width":       nbt.Short(cb.Width),
		"Height":      nbt.Short(cb.Height),
		"Length":      nbt.Short(cb.Length),
		"Offset":      nbt.IntArray{int32(offset.X), int32(offset.Y), int32(offset.Z)},
		"Blocks":      blocksTag,
		"Metadata": nbt.Compound{
			"Date": nbt.Long(time.Now().UnixMilli()),
			"WorldEdit": nbt.Compound{
				"Version":         nbt.String("2.12.0-golem"),
				"EditingPlatform": nbt.String("golem"),
				"Origin":          nbt.IntArray{int32(cb.Origin.X), int32(cb.Origin.Y), int32(cb.Origin.Z)},
			},
		},
	}

	if len(cb.Biomes) > 0 {
		bpal := make(map[string]int)
		bpalComp := nbt.Compound{}
		var bdata []byte
		for y := 0; y < cb.Height; y++ {
			for z := 0; z < cb.Length; z++ {
				for x := 0; x < cb.Width; x++ {
					b := cb.GetRelBiome(x, y, z)
					bname := b.Name()
					if bname == "" {
						bname = "minecraft:plains"
					}
					id, ok := bpal[bname]
					if !ok {
						id = len(bpal)
						bpal[bname] = id
						bpalComp[bname] = nbt.Int(id)
					}
					bdata = appendVarInt(bdata, id)
				}
			}
		}
		schem["Biomes"] = nbt.Compound{
			"Palette": bpalComp,
			"Data":    nbt.ByteArray(bdata),
		}
	}

	if len(cb.Entities) > 0 {
		items := make([]nbt.Tag, 0, len(cb.Entities))
		for _, ent := range cb.Entities {
			ec := nbt.Compound{
				"Id": nbt.String(ent.Type),
				"Pos": nbt.List{
					Elem:  nbt.TypeDouble,
					Items: []nbt.Tag{nbt.Double(ent.Pos.X), nbt.Double(ent.Pos.Y), nbt.Double(ent.Pos.Z)},
				},
			}
			if ent.Data != nil {
				ec["Data"] = nbt.Copy(ent.Data)
			}
			items = append(items, ec)
		}
		schem["Entities"] = nbt.List{Elem: nbt.TypeCompound, Items: items}
	}

	return writeGzippedNBT(w, "", nbt.Compound{"Schematic": schem})
}

func readSpongeV3(root nbt.Compound) (*Clipboard, error) {
	schem, ok := nbt.Get[nbt.Compound](root, "Schematic")
	if !ok {
		schem = root
	}
	w, _ := nbt.ShortField(schem, "Width")
	h, _ := nbt.ShortField(schem, "Height")
	l, _ := nbt.ShortField(schem, "Length")
	width, height, length := int(uint16(w)), int(uint16(h)), int(uint16(l))
	if width <= 0 || height <= 0 || length <= 0 {
		return nil, fmt.Errorf("invalid schematic dimensions %dx%dx%d", width, height, length)
	}
	var offset plugin.BlockPos
	if arr, ok := nbt.Get[nbt.IntArray](schem, "Offset"); ok && len(arr) == 3 {
		offset = plugin.BlockPos{X: int(arr[0]), Y: int(arr[1]), Z: int(arr[2])}
	}
	origin := plugin.BlockPos{X: -offset.X, Y: -offset.Y, Z: -offset.Z}
	minP := plugin.BlockPos{}
	if meta, ok := nbt.Get[nbt.Compound](schem, "Metadata"); ok {
		if we, ok := nbt.Get[nbt.Compound](meta, "WorldEdit"); ok {
			if origArr, ok := nbt.Get[nbt.IntArray](we, "Origin"); ok && len(origArr) == 3 {
				origin = plugin.BlockPos{X: int(origArr[0]), Y: int(origArr[1]), Z: int(origArr[2])}
				minP = plugin.BlockPos{X: origin.X + offset.X, Y: origin.Y + offset.Y, Z: origin.Z + offset.Z}
			}
		}
	}

	cb := &Clipboard{
		ID:            uuid.New(),
		Min:           minP,
		Origin:        origin,
		Width:         width,
		Height:        height,
		Length:        length,
		Blocks:        make([]plugin.BlockState, width*height*length),
		BlockEntities: make(map[plugin.BlockPos]nbt.Compound),
	}

	blocksTag, ok := nbt.Get[nbt.Compound](schem, "Blocks")
	if !ok {
		return nil, fmt.Errorf("Sponge v3 schematic missing Blocks compound")
	}
	palTag, _ := nbt.Get[nbt.Compound](blocksTag, "Palette")
	idToState := parseStringPalette(palTag)
	rawData, _ := nbt.Get[nbt.ByteArray](blocksTag, "Data")
	indices, err := readVarInts(rawData, width*height*length)
	if err != nil {
		return nil, err
	}
	idx := 0
	for y := 0; y < height; y++ {
		for z := 0; z < length; z++ {
			for x := 0; x < width; x++ {
				if idx < len(indices) {
					cb.SetRel(x, y, z, idToState[indices[idx]])
				}
				idx++
			}
		}
	}

	if beList, ok := nbt.Get[nbt.List](blocksTag, "BlockEntities"); ok {
		for _, item := range beList.Items {
			be, ok := item.(nbt.Compound)
			if !ok {
				continue
			}
			arr, ok := nbt.Get[nbt.IntArray](be, "Pos")
			if !ok || len(arr) != 3 {
				continue
			}
			pos := plugin.BlockPos{X: int(arr[0]), Y: int(arr[1]), Z: int(arr[2])}
			if data, ok := nbt.Get[nbt.Compound](be, "Data"); ok {
				cp := nbt.Copy(data).(nbt.Compound)
				if id, ok := nbt.Get[nbt.String](be, "Id"); ok {
					cp["id"] = id
				}
				cb.BlockEntities[pos] = cp
			}
		}
	}

	if biomesTag, ok := nbt.Get[nbt.Compound](schem, "Biomes"); ok {
		bpalTag, _ := nbt.Get[nbt.Compound](biomesTag, "Palette")
		idToBiome := parseBiomePalette(bpalTag)
		bdata, _ := nbt.Get[nbt.ByteArray](biomesTag, "Data")
		if bidx, err := readVarInts(bdata, width*height*length); err == nil {
			i := 0
			for y := 0; y < height; y++ {
				for z := 0; z < length; z++ {
					for x := 0; x < width; x++ {
						if i < len(bidx) {
							cb.SetRelBiome(x, y, z, idToBiome[bidx[i]])
						}
						i++
					}
				}
			}
		}
	}

	if entList, ok := nbt.Get[nbt.List](schem, "Entities"); ok {
		for _, item := range entList.Items {
			ec, ok := item.(nbt.Compound)
			if !ok {
				continue
			}
			id, _ := nbt.Get[nbt.String](ec, "Id")
			pos := readDoubleVec3(ec, "Pos")
			data, _ := nbt.Get[nbt.Compound](ec, "Data")
			cb.Entities = append(cb.Entities, StoredEntity{Type: string(id), Pos: pos, Data: data})
		}
	}
	return cb, nil
}

// --- Sponge Schematic Version 2 ---

func writeSpongeV2(w io.Writer, cb *Clipboard) error {
	offset := cb.Offset()
	palette := make(map[string]int)
	paletteCompound := nbt.Compound{}
	var blockData []byte

	for y := 0; y < cb.Height; y++ {
		for z := 0; z < cb.Length; z++ {
			for x := 0; x < cb.Width; x++ {
				key := block.FormatState(cb.GetRel(x, y, z))
				id, ok := palette[key]
				if !ok {
					id = len(palette)
					palette[key] = id
					paletteCompound[key] = nbt.Int(id)
				}
				blockData = appendVarInt(blockData, id)
			}
		}
	}

	schem := nbt.Compound{
		"Version":     nbt.Int(2),
		"DataVersion": nbt.Int(4189),
		"Width":       nbt.Short(cb.Width),
		"Height":      nbt.Short(cb.Height),
		"Length":      nbt.Short(cb.Length),
		"Offset":      nbt.IntArray{int32(cb.Min.X), int32(cb.Min.Y), int32(cb.Min.Z)},
		"PaletteMax":  nbt.Int(len(palette)),
		"Palette":     paletteCompound,
		"BlockData":   nbt.ByteArray(blockData),
		"Metadata": nbt.Compound{
			"WEOffsetX": nbt.Int(offset.X),
			"WEOffsetY": nbt.Int(offset.Y),
			"WEOffsetZ": nbt.Int(offset.Z),
		},
	}

	if len(cb.BlockEntities) > 0 {
		items := make([]nbt.Tag, 0, len(cb.BlockEntities))
		for pos, comp := range cb.BlockEntities {
			be := nbt.Copy(comp).(nbt.Compound)
			if id, ok := be["id"]; ok {
				be["Id"] = id
				delete(be, "id")
			}
			delete(be, "x")
			delete(be, "y")
			delete(be, "z")
			be["Pos"] = nbt.IntArray{int32(pos.X), int32(pos.Y), int32(pos.Z)}
			items = append(items, be)
		}
		schem["BlockEntities"] = nbt.List{Elem: nbt.TypeCompound, Items: items}
	}

	return writeGzippedNBT(w, "Schematic", schem)
}

func readSpongeV2(root nbt.Compound) (*Clipboard, error) {
	return readSpongeV1OrV2(root, 2)
}

func readSpongeV1(root nbt.Compound) (*Clipboard, error) {
	return readSpongeV1OrV2(root, 1)
}

func readSpongeV1OrV2(root nbt.Compound, ver int) (*Clipboard, error) {
	w, _ := nbt.ShortField(root, "Width")
	h, _ := nbt.ShortField(root, "Height")
	l, _ := nbt.ShortField(root, "Length")
	width, height, length := int(uint16(w)), int(uint16(h)), int(uint16(l))
	if width <= 0 || height <= 0 || length <= 0 {
		return nil, fmt.Errorf("invalid Sponge v%d dimensions %dx%dx%d", ver, width, height, length)
	}
	var minP plugin.BlockPos
	if arr, ok := nbt.Get[nbt.IntArray](root, "Offset"); ok && len(arr) == 3 {
		minP = plugin.BlockPos{X: int(arr[0]), Y: int(arr[1]), Z: int(arr[2])}
	}
	origin := minP
	if meta, ok := nbt.Get[nbt.Compound](root, "Metadata"); ok {
		if ox, okX := nbt.IntField(meta, "WEOffsetX"); okX {
			oy, _ := nbt.IntField(meta, "WEOffsetY")
			oz, _ := nbt.IntField(meta, "WEOffsetZ")
			origin = plugin.BlockPos{X: minP.X - int(ox), Y: minP.Y - int(oy), Z: minP.Z - int(oz)}
		}
	}
	cb := &Clipboard{
		ID:            uuid.New(),
		Min:           minP,
		Origin:        origin,
		Width:         width,
		Height:        height,
		Length:        length,
		Blocks:        make([]plugin.BlockState, width*height*length),
		BlockEntities: make(map[plugin.BlockPos]nbt.Compound),
	}

	palTag, _ := nbt.Get[nbt.Compound](root, "Palette")
	idToState := parseStringPalette(palTag)
	rawData, _ := nbt.Get[nbt.ByteArray](root, "BlockData")
	indices, err := readVarInts(rawData, width*height*length)
	if err != nil {
		return nil, err
	}
	idx := 0
	for y := 0; y < height; y++ {
		for z := 0; z < length; z++ {
			for x := 0; x < width; x++ {
				if idx < len(indices) {
					cb.SetRel(x, y, z, idToState[indices[idx]])
				}
				idx++
			}
		}
	}

	beKey := "BlockEntities"
	if _, ok := root[beKey]; !ok {
		beKey = "TileEntities"
	}
	if beList, ok := nbt.Get[nbt.List](root, beKey); ok {
		for _, item := range beList.Items {
			be, ok := item.(nbt.Compound)
			if !ok {
				continue
			}
			arr, ok := nbt.Get[nbt.IntArray](be, "Pos")
			if !ok || len(arr) != 3 {
				continue
			}
			pos := plugin.BlockPos{X: int(arr[0]), Y: int(arr[1]), Z: int(arr[2])}
			cp := nbt.Copy(be).(nbt.Compound)
			if id, ok := cp["Id"]; ok {
				cp["id"] = id
			}
			cb.BlockEntities[pos] = cp
		}
	}
	return cb, nil
}

// --- Legacy MCEdit .schematic reader ---

func readMCEdit(root nbt.Compound) (*Clipboard, error) {
	w, _ := nbt.ShortField(root, "Width")
	h, _ := nbt.ShortField(root, "Height")
	l, _ := nbt.ShortField(root, "Length")
	width, height, length := int(uint16(w)), int(uint16(h)), int(uint16(l))
	if width <= 0 || height <= 0 || length <= 0 {
		return nil, fmt.Errorf("invalid MCEdit dimensions %dx%dx%d", width, height, length)
	}
	ox, _ := nbt.IntField(root, "WEOffsetX")
	oy, _ := nbt.IntField(root, "WEOffsetY")
	oz, _ := nbt.IntField(root, "WEOffsetZ")
	minP := plugin.BlockPos{X: int(ox), Y: int(oy), Z: int(oz)}
	cb := &Clipboard{
		ID:            uuid.New(),
		Min:           minP,
		Origin:        plugin.BlockPos{},
		Width:         width,
		Height:        height,
		Length:        length,
		Blocks:        make([]plugin.BlockState, width*height*length),
		BlockEntities: make(map[plugin.BlockPos]nbt.Compound),
	}
	blocks, _ := nbt.Get[nbt.ByteArray](root, "Blocks")
	addBlocks, _ := nbt.Get[nbt.ByteArray](root, "AddBlocks")
	dataBytes, _ := nbt.Get[nbt.ByteArray](root, "Data")
	for y := 0; y < height; y++ {
		for z := 0; z < length; z++ {
			for x := 0; x < width; x++ {
				idx := (y*length+z)*width + x
				if idx >= len(blocks) {
					continue
				}
				bid := int(blocks[idx])
				if idx/2 < len(addBlocks) {
					ab := addBlocks[idx/2]
					if idx%2 == 0 {
						bid |= int(ab&0x0f) << 8
					} else {
						bid |= int((ab>>4)&0x0f) << 8
					}
				}
				bdata := 0
				if idx < len(dataBytes) {
					bdata = int(dataBytes[idx]) & 0x0f
				}
				if st, ok := block.LegacyBlockState(fmt.Sprintf("%d:%d", bid, bdata)); ok {
					cb.SetRel(x, y, z, st)
				}
			}
		}
	}
	if teList, ok := nbt.Get[nbt.List](root, "TileEntities"); ok {
		for _, item := range teList.Items {
			te, ok := item.(nbt.Compound)
			if !ok {
				continue
			}
			x, _ := nbt.IntField(te, "x")
			y, _ := nbt.IntField(te, "y")
			z, _ := nbt.IntField(te, "z")
			cb.BlockEntities[plugin.BlockPos{X: int(x), Y: int(y), Z: int(z)}] = nbt.Copy(te).(nbt.Compound)
		}
	}
	return cb, nil
}

// --- Vanilla Minecraft Structure (.nbt) ---

func writeStructureNBT(w io.Writer, cb *Clipboard) error {
	palIdx := make(map[plugin.BlockState]int)
	var palItems []nbt.Tag
	var blockItems []nbt.Tag

	for y := 0; y < cb.Height; y++ {
		for z := 0; z < cb.Length; z++ {
			for x := 0; x < cb.Width; x++ {
				st := cb.GetRel(x, y, z)
				idx, ok := palIdx[st]
				if !ok {
					idx = len(palItems)
					palIdx[st] = idx
					name, props, _ := registry.BlockStateName(uint16(st))
					entry := nbt.Compound{"Name": nbt.String(name)}
					if props != "" {
						pc := nbt.Compound{}
						for k, v := range registry.ParseStateProps(props) {
							pc[k] = nbt.String(v)
						}
						entry["Properties"] = pc
					}
					palItems = append(palItems, entry)
				}
				bEntry := nbt.Compound{
					"pos":   nbt.List{Elem: nbt.TypeInt, Items: []nbt.Tag{nbt.Int(x), nbt.Int(y), nbt.Int(z)}},
					"state": nbt.Int(idx),
				}
				if be, ok := cb.BlockEntities[plugin.BlockPos{X: x, Y: y, Z: z}]; ok {
					bEntry["nbt"] = nbt.Copy(be)
				}
				blockItems = append(blockItems, bEntry)
			}
		}
	}

	root := nbt.Compound{
		"DataVersion": nbt.Int(4189),
		"size":        nbt.List{Elem: nbt.TypeInt, Items: []nbt.Tag{nbt.Int(cb.Width), nbt.Int(cb.Height), nbt.Int(cb.Length)}},
		"palette":     nbt.List{Elem: nbt.TypeCompound, Items: palItems},
		"blocks":      nbt.List{Elem: nbt.TypeCompound, Items: blockItems},
		"entities":    nbt.List{Elem: nbt.TypeCompound, Items: nil},
	}
	return writeGzippedNBT(w, "", root)
}

func readStructureNBT(root nbt.Compound) (*Clipboard, error) {
	sizeList, ok := nbt.Get[nbt.List](root, "size")
	if !ok || len(sizeList.Items) != 3 {
		return nil, fmt.Errorf("structure .nbt missing size[3]")
	}
	w, _ := nbt.IntElement(sizeList.Items[0])
	h, _ := nbt.IntElement(sizeList.Items[1])
	l, _ := nbt.IntElement(sizeList.Items[2])
	width, height, length := int(w), int(h), int(l)
	cb := &Clipboard{
		ID:            uuid.New(),
		Width:         width,
		Height:        height,
		Length:        length,
		Blocks:        make([]plugin.BlockState, width*height*length),
		BlockEntities: make(map[plugin.BlockPos]nbt.Compound),
	}

	palList, _ := nbt.Get[nbt.List](root, "palette")
	palette := make([]plugin.BlockState, len(palList.Items))
	for i, item := range palList.Items {
		if id, ok := registry.StateOfTag(item); ok {
			palette[i] = plugin.BlockState(id)
		}
	}

	blocksList, _ := nbt.Get[nbt.List](root, "blocks")
	for _, item := range blocksList.Items {
		bc, ok := item.(nbt.Compound)
		if !ok {
			continue
		}
		stateIdx, _ := nbt.IntField(bc, "state")
		posList, ok := nbt.Get[nbt.List](bc, "pos")
		if !ok || len(posList.Items) != 3 || int(stateIdx) >= len(palette) {
			continue
		}
		x, _ := nbt.IntElement(posList.Items[0])
		y, _ := nbt.IntElement(posList.Items[1])
		z, _ := nbt.IntElement(posList.Items[2])
		cb.SetRel(int(x), int(y), int(z), palette[stateIdx])
		if be, ok := nbt.Get[nbt.Compound](bc, "nbt"); ok {
			cb.BlockEntities[plugin.BlockPos{X: int(x), Y: int(y), Z: int(z)}] = nbt.Copy(be).(nbt.Compound)
		}
	}
	return cb, nil
}

// --- Isometric PNG writer ---

func writeIsometricPNG(w io.Writer, cb *Clipboard) error {
	// Simple isometric projection: each voxel renders a shaded 2x2 diamond tile.
	tileW := 4
	tileH := 2
	imgW := max(16, (cb.Width+cb.Length)*tileW+16)
	imgH := max(16, (cb.Width+cb.Length)*tileH+cb.Height*tileH*2+16)
	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	ox := cb.Length*tileW + 8
	oy := cb.Height*tileH*2 + 8

	for y := 0; y < cb.Height; y++ {
		for z := 0; z < cb.Length; z++ {
			for x := 0; x < cb.Width; x++ {
				st := cb.GetRel(x, y, z)
				if block.IsAir(st) {
					continue
				}
				c := block.MapColorRGB(st)
				if c.A == 0 {
					c = color.RGBA{180, 180, 180, 255}
				}
				sx := ox + (x-z)*tileW
				sy := oy + (x+z)*tileH - y*tileH*2
				// Top face (bright), left face (mid), right face (dark)
				topC := c
				leftC := color.RGBA{uint8(int(c.R) * 80 / 100), uint8(int(c.G) * 80 / 100), uint8(int(c.B) * 80 / 100), 255}
				rightC := color.RGBA{uint8(int(c.R) * 65 / 100), uint8(int(c.G) * 65 / 100), uint8(int(c.B) * 65 / 100), 255}
				for dx := -1; dx <= 1; dx++ {
					img.SetRGBA(sx+dx, sy-1, topC)
					img.SetRGBA(sx+dx, sy, topC)
				}
				img.SetRGBA(sx-1, sy+1, leftC)
				img.SetRGBA(sx, sy+1, leftC)
				img.SetRGBA(sx+1, sy+1, rightC)
				img.SetRGBA(sx+2, sy+1, rightC)
			}
		}
	}
	return png.Encode(w, img)
}

func parseStringPalette(pal nbt.Compound) map[int]plugin.BlockState {
	out := make(map[int]plugin.BlockState, len(pal))
	for name, tag := range pal {
		id, ok := nbt.IntValue(tag)
		if !ok {
			continue
		}
		if st, err := block.ParseState(name, false); err == nil {
			out[int(id)] = st
		}
	}
	return out
}

func parseBiomePalette(pal nbt.Compound) map[int]registry.Biome {
	out := make(map[int]registry.Biome, len(pal))
	for name, tag := range pal {
		id, ok := nbt.IntValue(tag)
		if !ok {
			continue
		}
		if b, ok := registry.BiomeByName(registry.Qualify(name)); ok {
			out[int(id)] = b
		}
	}
	return out
}

func readDoubleVec3(c nbt.Compound, key string) plugin.Vec3 {
	lst, ok := nbt.Get[nbt.List](c, key)
	if !ok || len(lst.Items) != 3 {
		return plugin.Vec3{}
	}
	x, _ := nbt.NumberValue(lst.Items[0])
	y, _ := nbt.NumberValue(lst.Items[1])
	z, _ := nbt.NumberValue(lst.Items[2])
	return plugin.Vec3{X: x, Y: y, Z: z}
}
