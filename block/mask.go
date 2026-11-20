package block

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/expr"
	"github.com/golem-mc/worldedit/region"
)

// Extent is the read-only world/buffer interface inspected by masks and patterns.
type Extent interface {
	BlockState(pos plugin.BlockPos) plugin.BlockState
	Biome(pos plugin.BlockPos) registry.Biome
}

// ClipboardLookup allows patterns like #clipboard to read blocks relative to the clipboard origin.
type ClipboardLookup interface {
	BlockAt(rel plugin.BlockPos) (plugin.BlockState, bool)
	Dimensions() (w, h, l int)
}

// ParseContext supplies session-derived state when parsing rich masks, patterns, and transforms.
type ParseContext struct {
	Extent        Extent
	Selection     region.Region
	Origin        plugin.BlockPos
	HandBlock     plugin.BlockState
	HasHandBlock  bool
	HotbarBlocks  []plugin.BlockState
	Clipboard     ClipboardLookup
	AllowLegacy   bool
	ExprTimeout   time.Duration
	DynamicRegion func() region.Region
}

// Mask decides whether a block at pos matches a filter condition.
type Mask interface {
	Test(ext Extent, pos plugin.BlockPos) bool
	String() string
}

type maskFunc struct {
	desc string
	fn   func(ext Extent, pos plugin.BlockPos) bool
}

func (m maskFunc) Test(ext Extent, pos plugin.BlockPos) bool { return m.fn(ext, pos) }
func (m maskFunc) String() string                            { return m.desc }

// TrueMask matches every block.
func TrueMask() Mask {
	return maskFunc{"#true", func(Extent, plugin.BlockPos) bool { return true }}
}

// NegateMask inverts m.
func NegateMask(m Mask) Mask {
	return maskFunc{"!" + m.String(), func(ext Extent, pos plugin.BlockPos) bool { return !m.Test(ext, pos) }}
}

// AndMask matches when every child mask matches.
func AndMask(masks ...Mask) Mask {
	if len(masks) == 1 {
		return masks[0]
	}
	parts := make([]string, len(masks))
	for i, m := range masks {
		parts[i] = m.String()
	}
	return maskFunc{strings.Join(parts, " "), func(ext Extent, pos plugin.BlockPos) bool {
		for _, m := range masks {
			if !m.Test(ext, pos) {
				return false
			}
		}
		return true
	}}
}

// OrMask matches when any child mask matches.
func OrMask(masks ...Mask) Mask {
	if len(masks) == 1 {
		return masks[0]
	}
	parts := make([]string, len(masks))
	for i, m := range masks {
		parts[i] = m.String()
	}
	return maskFunc{strings.Join(parts, ","), func(ext Extent, pos plugin.BlockPos) bool {
		for _, m := range masks {
			if m.Test(ext, pos) {
				return true
			}
		}
		return false
	}}
}

// ParseMask parses a WorldEdit + FAWE mask expression.
// Space-separated clauses are intersected (AND); comma-separated clauses are unioned (OR).
func ParseMask(input string, ctx ParseContext) (Mask, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil, fmt.Errorf("empty mask")
	}
	andParts := splitTopLevel(s, ' ')
	if len(andParts) > 1 {
		var masks []Mask
		for _, p := range andParts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			m, err := ParseMask(p, ctx)
			if err != nil {
				return nil, err
			}
			masks = append(masks, m)
		}
		return AndMask(masks...), nil
	}
	orParts := splitTopLevel(s, ',')
	if len(orParts) > 1 {
		var masks []Mask
		for _, p := range orParts {
			m, err := parseSingleMask(strings.TrimSpace(p), ctx)
			if err != nil {
				return nil, err
			}
			masks = append(masks, m)
		}
		return OrMask(masks...), nil
	}
	return parseSingleMask(s, ctx)
}

func parseSingleMask(s string, ctx ParseContext) (Mask, error) { //nolint:funlen // one switch over the mask syntaxes reads better whole
	if s == "" {
		return nil, fmt.Errorf("empty mask clause")
	}
	// Negation: !mask
	if strings.HasPrefix(s, "!") {
		sub, err := parseSingleMask(s[1:], ctx)
		if err != nil {
			return nil, err
		}
		return NegateMask(sub), nil
	}
	// Above / below offset masks: >mask, <mask
	if strings.HasPrefix(s, ">") {
		sub, err := ParseMask(s[1:], ctx)
		if err != nil {
			return nil, err
		}
		return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
			return sub.Test(ext, plugin.BlockPos{X: pos.X, Y: pos.Y - 1, Z: pos.Z})
		}}, nil
	}
	if strings.HasPrefix(s, "<") {
		sub, err := ParseMask(s[1:], ctx)
		if err != nil {
			return nil, err
		}
		return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
			return sub.Test(ext, plugin.BlockPos{X: pos.X, Y: pos.Y + 1, Z: pos.Z})
		}}, nil
	}
	// Adjacent mask: ~mask or ~[mask][min][max]
	if strings.HasPrefix(s, "~") {
		rest := s[1:]
		if strings.HasPrefix(rest, "[") {
			_, args := parseRichArgs("adj" + rest)
			if len(args) < 1 {
				return nil, fmt.Errorf("invalid adjacent mask %q", s)
			}
			sub, err := ParseMask(args[0], ctx)
			if err != nil {
				return nil, err
			}
			minC, maxC := 1, 6
			if len(args) >= 2 {
				minC, _ = strconv.Atoi(args[1])
			}
			if len(args) >= 3 {
				maxC, _ = strconv.Atoi(args[2])
			}
			return adjacentMask(s, sub, minC, maxC, false), nil
		}
		sub, err := ParseMask(rest, ctx)
		if err != nil {
			return nil, err
		}
		return adjacentMask(s, sub, 1, 6, false), nil
	}
	// Random noise mask: %chance
	if strings.HasPrefix(s, "%") {
		pct, err := strconv.ParseFloat(s[1:], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid noise percentage %q", s)
		}
		prob := pct / 100.0
		return maskFunc{s, func(Extent, plugin.BlockPos) bool {
			return rand.Float64() < prob
		}}, nil
	}
	// Expression mask: =expr
	if strings.HasPrefix(s, "=") {
		prog, err := expr.Compile(s[1:])
		if err != nil {
			return nil, fmt.Errorf("invalid expression mask: %w", err)
		}
		timeout := ctx.ExprTimeout
		if timeout <= 0 {
			timeout = 50 * time.Millisecond
		}
		return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
			env := expr.NewEnv(timeout)
			env.Vars["x"] = float64(pos.X)
			env.Vars["y"] = float64(pos.Y)
			env.Vars["z"] = float64(pos.Z)
			env.Perlin = Perlin3D
			env.Simplex = Simplex3D
			env.Voronoi = Voronoi3D
			env.RidgedMulti = RidgedMulti3D
			v, err := prog.Eval(env)
			return err == nil && v > 0
		}}, nil
	}
	// Biome mask: $biome
	if after, ok := strings.CutPrefix(s, "$"); ok {
		bname := registry.Qualify(strings.ToLower(after))
		target, ok := registry.BiomeByName(bname)
		if !ok {
			return nil, fmt.Errorf("unknown biome %q", s[1:])
		}
		return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
			return ext.Biome(pos) == target
		}}, nil
	}
	// Angle mask: /[min][max]
	if strings.HasPrefix(s, "/") {
		_, args := parseRichArgs("angle" + s[1:])
		if len(args) < 2 {
			return nil, fmt.Errorf("angle mask requires /[min][max]")
		}
		return buildAngleMask(s, args[0], args[1], false)
	}
	// Block category / tag mask: ##tag
	if strings.HasPrefix(s, "##") {
		states, ok := TagStates(s[2:])
		if !ok {
			return nil, fmt.Errorf("unknown block tag %q", s)
		}
		allowed := make(map[registry.Block]bool, len(states))
		for _, st := range states {
			allowed[st.Block()] = true
		}
		return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
			return allowed[ext.BlockState(pos).Block()]
		}}, nil
	}
	// Built-in & FAWE rich #masks
	if strings.HasPrefix(s, "#") {
		name, args := parseRichArgs(s)
		lname := strings.ToLower(strings.TrimPrefix(name, "#"))
		switch lname {
		case "existing":
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool { return !IsAir(ext.BlockState(pos)) }}, nil
		case "air":
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool { return IsAir(ext.BlockState(pos)) }}, nil
		case "solid":
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool { return IsSolid(ext.BlockState(pos)) }}, nil
		case "liquid":
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool { return IsLiquid(ext.BlockState(pos)) }}, nil
		case "true":
			return TrueMask(), nil
		case "false":
			return maskFunc{"#false", func(Extent, plugin.BlockPos) bool { return false }}, nil
		case "surface", "exposed":
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
				if IsAir(ext.BlockState(pos)) {
					return false
				}
				return IsAir(ext.BlockState(plugin.BlockPos{X: pos.X + 1, Y: pos.Y, Z: pos.Z})) ||
					IsAir(ext.BlockState(plugin.BlockPos{X: pos.X - 1, Y: pos.Y, Z: pos.Z})) ||
					IsAir(ext.BlockState(plugin.BlockPos{X: pos.X, Y: pos.Y + 1, Z: pos.Z})) ||
					IsAir(ext.BlockState(plugin.BlockPos{X: pos.X, Y: pos.Y - 1, Z: pos.Z})) ||
					IsAir(ext.BlockState(plugin.BlockPos{X: pos.X, Y: pos.Y, Z: pos.Z + 1})) ||
					IsAir(ext.BlockState(plugin.BlockPos{X: pos.X, Y: pos.Y, Z: pos.Z - 1}))
			}}, nil
		case "wall":
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
				if IsAir(ext.BlockState(pos)) {
					return false
				}
				return IsAir(ext.BlockState(plugin.BlockPos{X: pos.X + 1, Y: pos.Y, Z: pos.Z})) ||
					IsAir(ext.BlockState(plugin.BlockPos{X: pos.X - 1, Y: pos.Y, Z: pos.Z})) ||
					IsAir(ext.BlockState(plugin.BlockPos{X: pos.X, Y: pos.Y, Z: pos.Z + 1})) ||
					IsAir(ext.BlockState(plugin.BlockPos{X: pos.X, Y: pos.Y, Z: pos.Z - 1}))
			}}, nil
		case "region", "selection", "sel":
			sel := ctx.Selection
			return maskFunc{s, func(_ Extent, pos plugin.BlockPos) bool {
				return sel != nil && sel.Contains(pos)
			}}, nil
		case "dregion", "dselection", "dsel":
			dyn := ctx.DynamicRegion
			sel := ctx.Selection
			return maskFunc{s, func(_ Extent, pos plugin.BlockPos) bool {
				r := sel
				if dyn != nil {
					if cur := dyn(); cur != nil {
						r = cur
					}
				}
				return r != nil && r.Contains(pos)
			}}, nil
		case "hand":
			if !ctx.HasHandBlock {
				return nil, fmt.Errorf("not holding a block for #hand mask")
			}
			hb := ctx.HandBlock.Block()
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
				return ext.BlockState(pos).Block() == hb
			}}, nil
		case "hotbar":
			allowed := make(map[registry.Block]bool, len(ctx.HotbarBlocks))
			for _, st := range ctx.HotbarBlocks {
				allowed[st.Block()] = true
			}
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
				return allowed[ext.BlockState(pos).Block()]
			}}, nil
		case "xaxis":
			ox := ctx.Origin.X
			return maskFunc{s, func(_ Extent, pos plugin.BlockPos) bool { return pos.X == ox }}, nil
		case "yaxis":
			oy := ctx.Origin.Y
			return maskFunc{s, func(_ Extent, pos plugin.BlockPos) bool { return pos.Y == oy }}, nil
		case "zaxis":
			oz := ctx.Origin.Z
			return maskFunc{s, func(_ Extent, pos plugin.BlockPos) bool { return pos.Z == oz }}, nil
		case "offset":
			if len(args) < 4 {
				return nil, fmt.Errorf("#offset requires [dx][dy][dz][mask]")
			}
			dx, _ := strconv.Atoi(args[0])
			dy, _ := strconv.Atoi(args[1])
			dz, _ := strconv.Atoi(args[2])
			sub, err := ParseMask(args[3], ctx)
			if err != nil {
				return nil, err
			}
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
				return sub.Test(ext, plugin.BlockPos{X: pos.X + dx, Y: pos.Y + dy, Z: pos.Z + dz})
			}}, nil
		case "adjacent", "beside":
			if len(args) < 1 {
				return nil, fmt.Errorf("#%s requires [mask][min][max]", lname)
			}
			sub, err := ParseMask(args[0], ctx)
			if err != nil {
				return nil, err
			}
			minC, maxC := 1, 6
			if len(args) >= 2 {
				minC, _ = strconv.Atoi(args[1])
			}
			if len(args) >= 3 {
				maxC, _ = strconv.Atoi(args[2])
			}
			return adjacentMask(s, sub, minC, maxC, false), nil
		case "adjacent2d":
			if len(args) < 1 {
				return nil, fmt.Errorf("#adjacent2d requires [mask][min][max]")
			}
			sub, err := ParseMask(args[0], ctx)
			if err != nil {
				return nil, err
			}
			minC, maxC := 1, 4
			if len(args) >= 2 {
				minC, _ = strconv.Atoi(args[1])
			}
			if len(args) >= 3 {
				maxC, _ = strconv.Atoi(args[2])
			}
			return adjacentMask(s, sub, minC, maxC, true), nil
		case "radius":
			if len(args) < 2 {
				return nil, fmt.Errorf("#radius requires [min][max]")
			}
			lo, _ := strconv.ParseFloat(args[0], 64)
			hi, _ := strconv.ParseFloat(args[1], 64)
			orig := ctx.Origin
			return maskFunc{s, func(_ Extent, pos plugin.BlockPos) bool {
				dx := float64(pos.X - orig.X)
				dy := float64(pos.Y - orig.Y)
				dz := float64(pos.Z - orig.Z)
				d := math.Sqrt(dx*dx + dy*dy + dz*dz)
				return d >= lo && d <= hi
			}}, nil
		case "simplex":
			if len(args) < 3 {
				return nil, fmt.Errorf("#simplex requires [scale][min][max]")
			}
			scale, _ := strconv.ParseFloat(args[0], 64)
			lo, _ := strconv.ParseFloat(args[1], 64)
			hi, _ := strconv.ParseFloat(args[2], 64)
			if scale == 0 {
				scale = 10
			}
			freq := 1.0 / scale
			return maskFunc{s, func(_ Extent, pos plugin.BlockPos) bool {
				v := Simplex3D(1337, float64(pos.X), float64(pos.Y), float64(pos.Z), freq, 1) * 100.0
				return v >= lo && v <= hi
			}}, nil
		case "angle", "surfaceangle", "roc":
			if len(args) < 2 {
				return nil, fmt.Errorf("#%s requires [min][max]", lname)
			}
			return buildAngleMask(s, args[0], args[1], lname == "surfaceangle")
		case "extrema":
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
				if IsAir(ext.BlockState(pos)) {
					return false
				}
				return IsAir(ext.BlockState(plugin.BlockPos{X: pos.X, Y: pos.Y + 1, Z: pos.Z}))
			}}, nil
		default:
			// Also allow single-# block tag like #minecraft:planks or #logs
			if states, ok := TagStates(s[1:]); ok {
				allowed := make(map[registry.Block]bool, len(states))
				for _, st := range states {
					allowed[st.Block()] = true
				}
				return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
					return allowed[ext.BlockState(pos).Block()]
				}}, nil
			}
			return nil, fmt.Errorf("unknown mask %q", s)
		}
	}

	// Fuzzy block state match (^block[prop=val]) or exact/default block match
	fuzzyProps := false
	if strings.HasPrefix(s, "^") {
		fuzzyProps = true
		s = s[1:]
	}
	hasExplicitProps := strings.Contains(s, "[")
	st, err := ParseState(s, ctx.AllowLegacy)
	if err != nil {
		return nil, err
	}
	if hasExplicitProps {
		_, wantProps, _ := registry.StateNameProps(uint16(st))
		wantBlock := st.Block()
		if fuzzyProps {
			// Match only the explicitly given properties (parsed from s).
			_, rawProps, _ := strings.Cut(s, "[")
			rawProps = strings.TrimSuffix(rawProps, "]")
			subset := registry.ParseStateProps(rawProps)
			return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
				cur := ext.BlockState(pos)
				if cur.Block() != wantBlock {
					return false
				}
				curProps := registry.StateProps(uint16(cur))
				for k, v := range subset {
					if curProps[k] != v {
						return false
					}
				}
				return true
			}}, nil
		}
		_ = wantProps
		return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
			return ext.BlockState(pos) == st
		}}, nil
	}
	wantBlock := st.Block()
	return maskFunc{s, func(ext Extent, pos plugin.BlockPos) bool {
		return ext.BlockState(pos).Block() == wantBlock
	}}, nil
}

func adjacentMask(desc string, sub Mask, minC, maxC int, only2D bool) Mask {
	dirs := []plugin.BlockPos{{X: 1}, {X: -1}, {Z: 1}, {Z: -1}}
	if !only2D {
		dirs = append(dirs, plugin.BlockPos{Y: 1}, plugin.BlockPos{Y: -1})
	}
	return maskFunc{desc, func(ext Extent, pos plugin.BlockPos) bool {
		count := 0
		for _, d := range dirs {
			if sub.Test(ext, plugin.BlockPos{X: pos.X + d.X, Y: pos.Y + d.Y, Z: pos.Z + d.Z}) {
				count++
			}
		}
		return count >= minC && count <= maxC
	}}
}

func buildAngleMask(desc, minStr, maxStr string, surfaceOnly bool) (Mask, error) {
	isDegMin := strings.HasSuffix(minStr, "d")
	isDegMax := strings.HasSuffix(maxStr, "d")
	if isDegMin != isDegMax {
		return nil, fmt.Errorf("cannot combine degree with block-step in angle mask")
	}
	minVal, err1 := strconv.ParseFloat(strings.TrimSuffix(minStr, "d"), 64)
	maxVal, err2 := strconv.ParseFloat(strings.TrimSuffix(maxStr, "d"), 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("invalid angle bounds %q..%q", minStr, maxStr)
	}
	if isDegMin {
		minVal = math.Tan(minVal * math.Pi / 180.0)
		maxVal = math.Tan(maxVal * math.Pi / 180.0)
	}
	heightAt := func(ext Extent, x, y, z int) float64 {
		// Scan locally around y to find terrain surface height.
		for dy := 8; dy >= -8; dy-- {
			if !IsAir(ext.BlockState(plugin.BlockPos{X: x, Y: y + dy, Z: z})) {
				return float64(y + dy)
			}
		}
		return float64(y - 8)
	}
	return maskFunc{desc, func(ext Extent, pos plugin.BlockPos) bool {
		if surfaceOnly && !IsAir(ext.BlockState(plugin.BlockPos{X: pos.X, Y: pos.Y + 1, Z: pos.Z})) {
			return false
		}
		hx1 := heightAt(ext, pos.X+1, pos.Y, pos.Z)
		hx0 := heightAt(ext, pos.X-1, pos.Y, pos.Z)
		hz1 := heightAt(ext, pos.X, pos.Y, pos.Z+1)
		hz0 := heightAt(ext, pos.X, pos.Y, pos.Z-1)
		dx := (hx1 - hx0) / 2.0
		dz := (hz1 - hz0) / 2.0
		slope := math.Sqrt(dx*dx + dz*dz)
		return slope >= minVal && slope <= maxVal
	}}, nil
}

// splitTopLevel splits s on sep while respecting nested [...] and (...) brackets.
func splitTopLevel(s string, sep byte) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && s[i] == sep {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

// parseRichArgs splits "#name[arg1][arg2]" into ("#name", ["arg1", "arg2"]).
func parseRichArgs(s string) (string, []string) {
	idx := strings.IndexByte(s, '[')
	if idx < 0 {
		return s, nil
	}
	name := s[:idx]
	var args []string
	i := idx
	for i < len(s) {
		if s[i] != '[' {
			break
		}
		depth := 0
		start := i + 1
		for i < len(s) {
			if s[i] == '[' {
				depth++
			} else if s[i] == ']' {
				depth--
				if depth == 0 {
					args = append(args, s[start:i])
					i++
					break
				}
			}
			i++
		}
	}
	return name, args
}
