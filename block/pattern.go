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
)

// Pattern chooses a BlockState to place at a given world position.
type Pattern interface {
	Apply(ext Extent, pos plugin.BlockPos) plugin.BlockState
	String() string
}

type patternFunc struct {
	desc string
	fn   func(ext Extent, pos plugin.BlockPos) plugin.BlockState
}

func (p patternFunc) Apply(ext Extent, pos plugin.BlockPos) plugin.BlockState { return p.fn(ext, pos) }
func (p patternFunc) String() string                                          { return p.desc }

// SinglePattern always returns st.
func SinglePattern(st plugin.BlockState) Pattern {
	return patternFunc{FormatState(st), func(Extent, plugin.BlockPos) plugin.BlockState { return st }}
}

type weightedEntry struct {
	weight float64
	pat    Pattern
}

type randomPattern struct {
	desc    string
	entries []weightedEntry
	total   float64
}

func (r *randomPattern) Apply(ext Extent, pos plugin.BlockPos) plugin.BlockState {
	if len(r.entries) == 0 {
		return 0
	}
	pick := rand.Float64() * r.total
	acc := 0.0
	for _, e := range r.entries {
		acc += e.weight
		if pick <= acc {
			return e.pat.Apply(ext, pos)
		}
	}
	return r.entries[len(r.entries)-1].pat.Apply(ext, pos)
}

func (r *randomPattern) String() string { return r.desc }

// ParsePattern parses a WorldEdit + FAWE pattern expression (including weighted
// random mixes like "50%stone,30%cobblestone,20%andesite", tags, random states,
// and FAWE rich patterns like #simplex, #perlin, #voronoi, #linear, #clipboard,
// #mask, #color, #darken, #lighten, #offset, #expression, #typeswap, etc.).
func ParsePattern(input string, ctx ParseContext) (Pattern, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	parts := splitTopLevel(s, ',')
	if len(parts) == 1 {
		// Still check for optional leading weight "50%stone" -> single pattern
		_, sub := splitWeight(parts[0])
		return parseSinglePattern(strings.TrimSpace(sub), ctx)
	}
	rp := &randomPattern{desc: s}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		w, sub := splitWeight(part)
		pat, err := parseSinglePattern(strings.TrimSpace(sub), ctx)
		if err != nil {
			return nil, err
		}
		rp.entries = append(rp.entries, weightedEntry{weight: w, pat: pat})
		rp.total += w
	}
	if len(rp.entries) == 0 {
		return nil, fmt.Errorf("empty pattern %q", input)
	}
	return rp, nil
}

func splitWeight(s string) (float64, string) {
	idx := strings.IndexByte(s, '%')
	if idx > 0 {
		if w, err := strconv.ParseFloat(strings.TrimSpace(s[:idx]), 64); err == nil && w > 0 {
			return w, s[idx+1:]
		}
	}
	return 1.0, s
}

func parseSinglePattern(s string, ctx ParseContext) (Pattern, error) { //nolint:funlen // one switch over the pattern syntaxes reads better whole
	if s == "" {
		return nil, fmt.Errorf("empty pattern clause")
	}
	// Random state pattern: *block
	if strings.HasPrefix(s, "*") {
		qual := registry.Qualify(strings.ToLower(s[1:]))
		all, ok := registry.AllStates[qual]
		if !ok || len(all) == 0 {
			st, err := ParseState(s[1:], ctx.AllowLegacy)
			if err != nil {
				return nil, err
			}
			return SinglePattern(st), nil
		}
		return patternFunc{s, func(Extent, plugin.BlockPos) plugin.BlockState {
			return plugin.BlockState(all[rand.IntN(len(all))].ID)
		}}, nil
	}
	// Type-only / state-preserving pattern: ^block or ^block[props]
	if strings.HasPrefix(s, "^") {
		raw := s[1:]
		st, err := ParseState(raw, ctx.AllowLegacy)
		if err != nil {
			return nil, err
		}
		targetBlock := st.Block()
		return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
			existing := ext.BlockState(pos)
			if merged, ok := registry.WithPropertiesOf(targetBlock, uint16(existing)); ok {
				return plugin.BlockState(merged)
			}
			return st
		}}, nil
	}
	// Block category / tag pattern: ##tag
	if strings.HasPrefix(s, "##") {
		states, ok := TagStates(s[2:])
		if !ok {
			return nil, fmt.Errorf("unknown block tag %q", s)
		}
		return patternFunc{s, func(Extent, plugin.BlockPos) plugin.BlockState {
			return states[rand.IntN(len(states))]
		}}, nil
	}
	// Rich #patterns
	if strings.HasPrefix(s, "#") {
		name, args := parseRichArgs(s)
		lname := strings.ToLower(strings.TrimPrefix(name, "#"))
		switch lname {
		case "existing":
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				return ext.BlockState(pos)
			}}, nil
		case "hand":
			if !ctx.HasHandBlock {
				return nil, fmt.Errorf("not holding a block for #hand pattern")
			}
			return SinglePattern(ctx.HandBlock), nil
		case "hotbar":
			if len(ctx.HotbarBlocks) == 0 {
				return nil, fmt.Errorf("no blocks in hotbar for #hotbar pattern")
			}
			blocks := append([]plugin.BlockState(nil), ctx.HotbarBlocks...)
			return patternFunc{s, func(Extent, plugin.BlockPos) plugin.BlockState {
				return blocks[rand.IntN(len(blocks))]
			}}, nil
		case "clipboard", "copy", "fullcopy":
			if ctx.Clipboard == nil {
				return nil, fmt.Errorf("clipboard is empty")
			}
			cb := ctx.Clipboard
			w, h, l := cb.Dimensions()
			if w <= 0 || h <= 0 || l <= 0 {
				return nil, fmt.Errorf("clipboard is empty")
			}
			orig := ctx.Origin
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				rx := ((pos.X-orig.X)%w + w) % w
				ry := ((pos.Y-orig.Y)%h + h) % h
				rz := ((pos.Z-orig.Z)%l + l) % l
				if st, ok := cb.BlockAt(plugin.BlockPos{X: rx, Y: ry, Z: rz}); ok {
					return st
				}
				return ext.BlockState(pos)
			}}, nil
		case "linear", "l":
			pats, err := parsePatternList(args, ctx)
			if err != nil {
				return nil, err
			}
			idx := 0
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				p := pats[idx%len(pats)]
				idx++
				return p.Apply(ext, pos)
			}}, nil
		case "linear2d", "l2d":
			pats, err := parsePatternList(args, ctx)
			if err != nil {
				return nil, err
			}
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				i := ((pos.X+pos.Z)%len(pats) + len(pats)) % len(pats)
				return pats[i].Apply(ext, pos)
			}}, nil
		case "linear3d", "l3d":
			pats, err := parsePatternList(args, ctx)
			if err != nil {
				return nil, err
			}
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				i := ((pos.X+pos.Y+pos.Z)%len(pats) + len(pats)) % len(pats)
				return pats[i].Apply(ext, pos)
			}}, nil
		case "simplex", "perlin", "voronoi", "ridgedmulti":
			if len(args) < 2 {
				return nil, fmt.Errorf("#%s requires [scale][pattern]", lname)
			}
			scale, err := strconv.ParseFloat(args[0], 64)
			if err != nil || scale == 0 {
				scale = 10
			}
			pats, err := parsePatternList(args[1:], ctx)
			if err != nil {
				return nil, err
			}
			freq := 1.0 / scale
			seed := rand.Int64()
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				var v float64
				switch lname {
				case "perlin":
					v = Perlin3D(seed, float64(pos.X), float64(pos.Y), float64(pos.Z), freq, 2)
				case "voronoi":
					v = Voronoi3D(seed, float64(pos.X), float64(pos.Y), float64(pos.Z), freq)
				case "ridgedmulti":
					v = RidgedMulti3D(seed, float64(pos.X), float64(pos.Y), float64(pos.Z), freq, 2)
				default:
					v = Simplex3D(seed, float64(pos.X), float64(pos.Y), float64(pos.Z), freq, 2)
				}
				idx := max(int(math.Floor(v*float64(len(pats)))), 0)
				if idx >= len(pats) {
					idx = len(pats) - 1
				}
				return pats[idx].Apply(ext, pos)
			}}, nil
		case "mask":
			if len(args) < 3 {
				return nil, fmt.Errorf("#mask requires [mask][truePattern][falsePattern]")
			}
			m, err := ParseMask(args[0], ctx)
			if err != nil {
				return nil, err
			}
			pt, err := ParsePattern(args[1], ctx)
			if err != nil {
				return nil, err
			}
			pf, err := ParsePattern(args[2], ctx)
			if err != nil {
				return nil, err
			}
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				if m.Test(ext, pos) {
					return pt.Apply(ext, pos)
				}
				return pf.Apply(ext, pos)
			}}, nil
		case "offset":
			if len(args) < 4 {
				return nil, fmt.Errorf("#offset requires [dx][dy][dz][pattern]")
			}
			dx, _ := strconv.Atoi(args[0])
			dy, _ := strconv.Atoi(args[1])
			dz, _ := strconv.Atoi(args[2])
			sub, err := ParsePattern(args[3], ctx)
			if err != nil {
				return nil, err
			}
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				return sub.Apply(ext, plugin.BlockPos{X: pos.X + dx, Y: pos.Y + dy, Z: pos.Z + dz})
			}}, nil
		case "randomoffset", "solidrandomoffset", "surfacerandomoffset":
			if len(args) < 4 {
				return nil, fmt.Errorf("#%s requires [dx][dy][dz][pattern]", lname)
			}
			dx, _ := strconv.Atoi(args[0])
			dy, _ := strconv.Atoi(args[1])
			dz, _ := strconv.Atoi(args[2])
			sub, err := ParsePattern(args[3], ctx)
			if err != nil {
				return nil, err
			}
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				ox, oy, oz := 0, 0, 0
				if dx > 0 {
					ox = rand.IntN(2*dx+1) - dx
				}
				if dy > 0 {
					oy = rand.IntN(2*dy+1) - dy
				}
				if dz > 0 {
					oz = rand.IntN(2*dz+1) - dz
				}
				return sub.Apply(ext, plugin.BlockPos{X: pos.X + ox, Y: pos.Y + oy, Z: pos.Z + oz})
			}}, nil
		case "relative", "rel":
			if len(args) < 1 {
				return nil, fmt.Errorf("#relative requires [pattern]")
			}
			sub, err := ParsePattern(args[0], ctx)
			if err != nil {
				return nil, err
			}
			orig := ctx.Origin
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				return sub.Apply(ext, plugin.BlockPos{X: pos.X - orig.X, Y: pos.Y - orig.Y, Z: pos.Z - orig.Z})
			}}, nil
		case "nox", "noy", "noz":
			if len(args) < 1 {
				return nil, fmt.Errorf("#%s requires [pattern]", lname)
			}
			sub, err := ParsePattern(args[0], ctx)
			if err != nil {
				return nil, err
			}
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				p := pos
				switch lname {
				case "nox":
					p.X = 0
				case "noy":
					p.Y = 0
				case "noz":
					p.Z = 0
				}
				return sub.Apply(ext, p)
			}}, nil
		case "buffered", "buffered2d":
			if len(args) < 1 {
				return nil, fmt.Errorf("#%s requires [pattern]", lname)
			}
			sub, err := ParsePattern(args[0], ctx)
			if err != nil {
				return nil, err
			}
			cache := make(map[plugin.BlockPos]plugin.BlockState)
			is2d := lname == "buffered2d"
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				k := pos
				if is2d {
					k.Y = 0
				}
				if st, ok := cache[k]; ok {
					return st
				}
				st := sub.Apply(ext, pos)
				cache[k] = st
				return st
			}}, nil
		case "expression", "expr":
			if len(args) < 1 {
				return nil, fmt.Errorf("#expression requires [expr]")
			}
			prog, err := expr.Compile(args[0])
			if err != nil {
				return nil, err
			}
			timeout := ctx.ExprTimeout
			if timeout <= 0 {
				timeout = 50 * time.Millisecond
			}
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				env := expr.NewEnv(timeout)
				env.Vars["x"] = float64(pos.X)
				env.Vars["y"] = float64(pos.Y)
				env.Vars["z"] = float64(pos.Z)
				env.Perlin = Perlin3D
				env.Simplex = Simplex3D
				env.Voronoi = Voronoi3D
				env.RidgedMulti = RidgedMulti3D
				v, err := prog.Eval(env)
				if err != nil || v <= 0 {
					return 0
				}
				if st, ok := LegacyBlockState(strconv.Itoa(int(math.Round(v)))); ok {
					return st
				}
				return plugin.BlockState(int(v) % registry.StateCount)
			}}, nil
		case "color":
			if len(args) < 1 {
				return nil, fmt.Errorf("#color requires [hex]")
			}
			r, g, b, ok := ParseHexColor(args[0])
			if !ok {
				return nil, fmt.Errorf("invalid hex color %q", args[0])
			}
			return SinglePattern(NearestBlockForRGB(r, g, b)), nil
		case "darken":
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				return AdjustColorBlock(ext.BlockState(pos), 0.78, 1.0)
			}}, nil
		case "lighten":
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				return AdjustColorBlock(ext.BlockState(pos), 1.25, 1.0)
			}}, nil
		case "saturate":
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				return AdjustColorBlock(ext.BlockState(pos), 1.0, 1.4)
			}}, nil
		case "desaturate":
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				return AdjustColorBlock(ext.BlockState(pos), 1.0, 0.5)
			}}, nil
		case "averagecolor":
			if len(args) < 1 {
				return nil, fmt.Errorf("#averagecolor requires [hex]")
			}
			r, g, b, ok := ParseHexColor(args[0])
			if !ok {
				return nil, fmt.Errorf("invalid hex color %q", args[0])
			}
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				cur := MapColorRGB(ext.BlockState(pos))
				return NearestBlockForRGB(uint8((int(cur.R)+int(r))/2), uint8((int(cur.G)+int(g))/2), uint8((int(cur.B)+int(b))/2))
			}}, nil
		case "anglecolor":
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				exposed := IsAir(ext.BlockState(plugin.BlockPos{X: pos.X, Y: pos.Y + 1, Z: pos.Z}))
				if exposed {
					return AdjustColorBlock(ext.BlockState(pos), 1.15, 1.0)
				}
				return AdjustColorBlock(ext.BlockState(pos), 0.85, 1.0)
			}}, nil
		case "typeswap":
			if len(args) < 2 {
				return nil, fmt.Errorf("#typeswap requires [from][to]")
			}
			from, to := args[0], args[1]
			return patternFunc{s, func(ext Extent, pos plugin.BlockPos) plugin.BlockState {
				cur := ext.BlockState(pos)
				name, props, ok := registry.BlockStateName(uint16(cur))
				if !ok || !strings.Contains(name, from) {
					return cur
				}
				nextName := strings.ReplaceAll(name, from, to)
				if id, ok := registry.FindState(nextName, registry.ParseStateProps(props)); ok {
					return plugin.BlockState(id)
				}
				return cur
			}}, nil
		default:
			// Single-# block tag like #minecraft:planks
			if states, ok := TagStates(s[1:]); ok {
				return patternFunc{s, func(Extent, plugin.BlockPos) plugin.BlockState {
					return states[rand.IntN(len(states))]
				}}, nil
			}
			return nil, fmt.Errorf("unknown pattern %q", s)
		}
	}

	st, err := ParseState(s, ctx.AllowLegacy)
	if err != nil {
		return nil, err
	}
	return SinglePattern(st), nil
}

func parsePatternList(args []string, ctx ParseContext) ([]Pattern, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("expected at least one sub-pattern")
	}
	var pats []Pattern
	for _, a := range args {
		for _, sub := range splitTopLevel(a, ',') {
			sub = strings.TrimSpace(sub)
			if sub == "" {
				continue
			}
			p, err := ParsePattern(sub, ctx)
			if err != nil {
				return nil, err
			}
			pats = append(pats, p)
		}
	}
	if len(pats) == 0 {
		return nil, fmt.Errorf("expected at least one sub-pattern")
	}
	return pats, nil
}
