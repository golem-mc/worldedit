// Package block provides WorldEdit/FAWE block state parsing (including legacy
// numerical IDs and block tags), color lookup, block state rotation/flipping,
// procedural noise, masks, patterns, and transforms.
package block

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
)

//go:embed data/legacy.json
var legacyJSON []byte

var (
	legacyOnce   sync.Once
	legacyBlocks map[string]string
	legacyItems  map[string]string
)

func loadLegacy() {
	legacyOnce.Do(func() {
		var raw struct {
			Blocks map[string]string `json:"blocks"`
			Items  map[string]string `json:"items"`
		}
		if err := json.Unmarshal(legacyJSON, &raw); err == nil {
			legacyBlocks = raw.Blocks
			legacyItems = raw.Items
		}
		if legacyBlocks == nil {
			legacyBlocks = map[string]string{}
		}
		if legacyItems == nil {
			legacyItems = map[string]string{}
		}
	})
}

// LegacyBlockState maps a legacy numerical "id" or "id:data" string (e.g. "35:14", "1")
// to a modern plugin.BlockState.
func LegacyBlockState(spec string) (plugin.BlockState, bool) {
	loadLegacy()
	idStr, dataStr, hasData := strings.Cut(spec, ":")
	idNum, err := strconv.Atoi(idStr)
	if err != nil || idNum < 0 {
		return 0, false
	}
	dataNum := 0
	if hasData {
		d, err := strconv.Atoi(dataStr)
		if err != nil || d < 0 || d > 15 {
			return 0, false
		}
		dataNum = d
	}
	key := fmt.Sprintf("%d:%d", idNum, dataNum)
	modern, ok := legacyBlocks[key]
	if !ok {
		modern, ok = legacyBlocks[fmt.Sprintf("%d:0", idNum)]
	}
	if !ok {
		return 0, false
	}
	// Handle renamed legacy 1.13->1.21 blocks (e.g., minecraft:grass -> short_grass, minecraft:sign -> oak_sign)
	if st, err := ParseState(modern, false); err == nil {
		return st, true
	}
	if alias, ok := legacyRenames[modern]; ok {
		if st, err := ParseState(alias, false); err == nil {
			return st, true
		}
	}
	base, props, hasProps := strings.Cut(modern, "[")
	if alias, ok := legacyRenames[base]; ok {
		if hasProps {
			modern = alias + "[" + props
		} else {
			modern = alias
		}
		if st, err := ParseState(modern, false); err == nil {
			return st, true
		}
	}
	return 0, false
}

var legacyRenames = map[string]string{
	"minecraft:grass":     "minecraft:short_grass",
	"minecraft:sign":      "minecraft:oak_sign",
	"minecraft:wall_sign": "minecraft:oak_wall_sign",
}

// ParseState parses a Minecraft block state string (e.g. "stone",
// "minecraft:oak_stairs[facing=north,half=top]", or legacy "35:14" when allowLegacy is true).
func ParseState(input string, allowLegacy bool) (plugin.BlockState, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, fmt.Errorf("empty block state")
	}
	// Strip optional trailing NBT `{...}` (handled separately by full-block callers if needed)
	if idx := strings.IndexByte(s, '{'); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	// Legacy numeric ID check: digits or digits:digits
	if allowLegacy && isLegacyNumeric(s) {
		if st, ok := LegacyBlockState(s); ok {
			return st, nil
		}
	}

	namePart := s
	var propMap map[string]string
	if bracket := strings.IndexByte(s, '['); bracket >= 0 {
		if !strings.HasSuffix(s, "]") {
			return 0, fmt.Errorf("unclosed property bracket in %q", input)
		}
		namePart = s[:bracket]
		propStr := s[bracket+1 : len(s)-1]
		if strings.TrimSpace(propStr) != "" {
			propMap = make(map[string]string)
			for _, pair := range strings.Split(propStr, ",") {
				k, v, ok := strings.Cut(pair, "=")
				if !ok {
					return 0, fmt.Errorf("invalid property %q in %q", pair, input)
				}
				k = strings.TrimSpace(strings.ToLower(k))
				v = strings.TrimSpace(strings.ToLower(v))
				propMap[k] = v
			}
		}
	}

	qual := registry.Qualify(strings.ToLower(namePart))
	// Validate explicit property names and values if specified.
	if len(propMap) > 0 {
		for k, v := range propMap {
			allowed, ok := registry.BlockPropertyValues(qual, k)
			if !ok {
				return 0, fmt.Errorf("block %q has no property %q", qual, k)
			}
			if !allowed[v] {
				return 0, fmt.Errorf("invalid value %q for property %q on %q", v, k, qual)
			}
		}
	}
	id, ok := registry.FindState(qual, propMap)
	if !ok {
		return 0, fmt.Errorf("unknown block %q", input)
	}
	return plugin.BlockState(id), nil
}

func isLegacyNumeric(s string) bool {
	if s == "" {
		return false
	}
	seenColon := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			continue
		}
		if c == ':' && !seenColon && i > 0 && i < len(s)-1 {
			seenColon = true
			continue
		}
		return false
	}
	return true
}

// FormatState renders a BlockState as "minecraft:name" or "minecraft:name[k=v,...]".
func FormatState(st plugin.BlockState) string {
	name, props, ok := registry.BlockStateName(uint16(st))
	if !ok {
		return "minecraft:air"
	}
	if props == "" {
		return name
	}
	return name + "[" + props + "]"
}

// BlockName returns the namespaced block ID of st (e.g. "minecraft:stone").
func BlockName(st plugin.BlockState) string {
	name, _, ok := registry.BlockStateName(uint16(st))
	if !ok {
		return "minecraft:air"
	}
	return name
}

// IsAir reports whether st is air, cave_air, or void_air.
func IsAir(st plugin.BlockState) bool {
	switch BlockName(st) {
	case "minecraft:air", "minecraft:cave_air", "minecraft:void_air":
		return true
	default:
		return false
	}
}

// IsLiquid reports whether st is water or lava.
func IsLiquid(st plugin.BlockState) bool {
	switch BlockName(st) {
	case "minecraft:water", "minecraft:lava":
		return true
	default:
		return false
	}
}

// IsSolid reports whether st is neither air nor liquid nor common non-solid flora.
func IsSolid(st plugin.BlockState) bool {
	if IsAir(st) || IsLiquid(st) {
		return false
	}
	name := BlockName(st)
	if strings.Contains(name, "torch") || strings.Contains(name, "flower") ||
		strings.Contains(name, "sapling") || strings.Contains(name, "carpet") ||
		strings.Contains(name, "rail") || strings.Contains(name, "button") ||
		strings.Contains(name, "pressure_plate") || strings.Contains(name, "vine") ||
		name == "minecraft:short_grass" || name == "minecraft:tall_grass" ||
		name == "minecraft:fern" || name == "minecraft:large_fern" ||
		name == "minecraft:dead_bush" || name == "minecraft:fire" ||
		name == "minecraft:soul_fire" || name == "minecraft:snow" ||
		name == "minecraft:redstone_wire" || name == "minecraft:cobweb" {
		return false
	}
	return true
}

// TagStates resolves a block tag (with or without leading '#' or 'minecraft:') into default states.
func TagStates(tag string) ([]plugin.BlockState, bool) {
	t := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(tag), "#"), "#")
	qual := registry.Qualify(strings.ToLower(t))
	members, ok := registry.BlockTagMembers(qual)
	if !ok || len(members) == 0 {
		return nil, false
	}
	states := make([]plugin.BlockState, 0, len(members))
	for _, name := range members {
		if id, ok := registry.FindState(name, nil); ok {
			states = append(states, plugin.BlockState(id))
		}
	}
	return states, len(states) > 0
}

// MapColorPalette maps Minecraft's vanilla MapColor IDs (0..61) to sRGB colors.
var MapColorPalette = [64]color.RGBA{
	0:  {0, 0, 0, 0},        // NONE
	1:  {127, 178, 56, 255}, // GRASS
	2:  {247, 233, 163, 255},
	3:  {199, 199, 199, 255},
	4:  {255, 0, 0, 255},
	5:  {160, 160, 255, 255},
	6:  {167, 167, 167, 255},
	7:  {0, 124, 0, 255},
	8:  {255, 255, 255, 255},
	9:  {164, 168, 184, 255},
	10: {151, 109, 77, 255},
	11: {112, 112, 112, 255},
	12: {64, 64, 255, 255},
	13: {143, 119, 72, 255},
	14: {255, 252, 245, 255},
	15: {216, 127, 51, 255},
	16: {178, 76, 216, 255},
	17: {102, 153, 216, 255},
	18: {229, 229, 51, 255},
	19: {127, 204, 25, 255},
	20: {242, 127, 165, 255},
	21: {76, 76, 76, 255},
	22: {153, 153, 153, 255},
	23: {76, 127, 153, 255},
	24: {127, 63, 178, 255},
	25: {51, 76, 178, 255},
	26: {102, 76, 51, 255},
	27: {102, 127, 51, 255},
	28: {153, 51, 51, 255},
	29: {25, 25, 25, 255},
	30: {250, 238, 77, 255},
	31: {92, 219, 213, 255},
	32: {74, 128, 255, 255},
	33: {0, 217, 58, 255},
	34: {129, 86, 49, 255},
	35: {112, 2, 0, 255},
	36: {209, 177, 161, 255},
	37: {159, 82, 36, 255},
	38: {149, 87, 108, 255},
	39: {112, 108, 138, 255},
	40: {186, 133, 36, 255},
	41: {103, 117, 53, 255},
	42: {160, 77, 78, 255},
	43: {57, 41, 35, 255},
	44: {135, 107, 98, 255},
	45: {87, 92, 92, 255},
	46: {122, 73, 88, 255},
	47: {76, 62, 92, 255},
	48: {76, 50, 35, 255},
	49: {76, 82, 42, 255},
	50: {142, 60, 46, 255},
	51: {37, 22, 16, 255},
	52: {189, 48, 49, 255},
	53: {148, 63, 97, 255},
	54: {92, 25, 29, 255},
	55: {22, 126, 134, 255},
	56: {58, 142, 140, 255},
	57: {86, 44, 62, 255},
	58: {20, 180, 133, 255},
	59: {100, 100, 100, 255},
	60: {216, 175, 147, 255},
	61: {127, 167, 150, 255},
}

// MapColorRGB returns the approximate RGB color for a block state.
func MapColorRGB(st plugin.BlockState) color.RGBA {
	id := registry.BlockMapColor(uint16(st))
	if int(id) < len(MapColorPalette) {
		return MapColorPalette[id]
	}
	return color.RGBA{128, 128, 128, 255}
}

type colorBlockEntry struct {
	state plugin.BlockState
	c     color.RGBA
}

var (
	colorPaletteOnce sync.Once
	colorPalette     []colorBlockEntry
)

func initColorPalette() {
	colorPaletteOnce.Do(func() {
		// Curated solid opaque building blocks for color matching (concrete, terracotta, wool, stone, planks).
		candidates := []string{
			"minecraft:white_concrete", "minecraft:orange_concrete", "minecraft:magenta_concrete",
			"minecraft:light_blue_concrete", "minecraft:yellow_concrete", "minecraft:lime_concrete",
			"minecraft:pink_concrete", "minecraft:gray_concrete", "minecraft:light_gray_concrete",
			"minecraft:cyan_concrete", "minecraft:purple_concrete", "minecraft:blue_concrete",
			"minecraft:brown_concrete", "minecraft:green_concrete", "minecraft:red_concrete",
			"minecraft:black_concrete",
			"minecraft:white_terracotta", "minecraft:orange_terracotta", "minecraft:magenta_terracotta",
			"minecraft:light_blue_terracotta", "minecraft:yellow_terracotta", "minecraft:lime_terracotta",
			"minecraft:pink_terracotta", "minecraft:gray_terracotta", "minecraft:light_gray_terracotta",
			"minecraft:cyan_terracotta", "minecraft:purple_terracotta", "minecraft:blue_terracotta",
			"minecraft:brown_terracotta", "minecraft:green_terracotta", "minecraft:red_terracotta",
			"minecraft:black_terracotta", "minecraft:terracotta",
			"minecraft:stone", "minecraft:deepslate", "minecraft:cobblestone", "minecraft:dirt",
			"minecraft:grass_block", "minecraft:sand", "minecraft:red_sand", "minecraft:snow_block",
			"minecraft:oak_planks", "minecraft:spruce_planks", "minecraft:birch_planks",
			"minecraft:jungle_planks", "minecraft:acacia_planks", "minecraft:dark_oak_planks",
			"minecraft:prismarine", "minecraft:dark_prismarine", "minecraft:gold_block",
			"minecraft:diamond_block", "minecraft:emerald_block", "minecraft:lapis_block",
			"minecraft:redstone_block", "minecraft:netherrack", "minecraft:obsidian",
			"minecraft:quartz_block", "minecraft:packed_ice", "minecraft:clay",
		}
		for _, name := range candidates {
			if id, ok := registry.FindState(name, nil); ok {
				st := plugin.BlockState(id)
				c := MapColorRGB(st)
				if c.A > 0 {
					colorPalette = append(colorPalette, colorBlockEntry{state: st, c: c})
				}
			}
		}
		sort.Slice(colorPalette, func(i, j int) bool { return colorPalette[i].state < colorPalette[j].state })
	})
}

// NearestBlockForRGB picks the solid building block whose map color is closest to (r, g, b).
func NearestBlockForRGB(r, g, b uint8) plugin.BlockState {
	initColorPalette()
	if len(colorPalette) == 0 {
		id, _ := registry.FindState("minecraft:stone", nil)
		return plugin.BlockState(id)
	}
	best := colorPalette[0].state
	bestDist := math.MaxFloat64
	for _, e := range colorPalette {
		dr := float64(int(r) - int(e.c.R))
		dg := float64(int(g) - int(e.c.G))
		db := float64(int(b) - int(e.c.B))
		d := dr*dr*0.299 + dg*dg*0.587 + db*db*0.114
		if d < bestDist {
			bestDist = d
			best = e.state
		}
	}
	return best
}

// AdjustColorBlock shifts the brightness or saturation of st and returns the closest matching block.
func AdjustColorBlock(st plugin.BlockState, brightnessFactor, satFactor float64) plugin.BlockState {
	c := MapColorRGB(st)
	if c.A == 0 {
		return st
	}
	r := float64(c.R) * brightnessFactor
	g := float64(c.G) * brightnessFactor
	b := float64(c.B) * brightnessFactor
	if satFactor != 1.0 {
		luma := 0.299*r + 0.587*g + 0.114*b
		r = luma + (r-luma)*satFactor
		g = luma + (g-luma)*satFactor
		b = luma + (b-luma)*satFactor
	}
	clampU8 := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(math.Round(v))
	}
	return NearestBlockForRGB(clampU8(r), clampU8(g), clampU8(b))
}

// ParseHexColor parses "#rrggbb" or "rrggbb" into RGB.
func ParseHexColor(s string) (r, g, b uint8, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8((v >> 16) & 0xff), uint8((v >> 8) & 0xff), uint8(v & 0xff), true
}
