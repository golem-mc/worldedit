package block

import (
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
)

// RotateStateY rotates a block state's directional properties clockwise around Y by deg
// (normalized to multiples of 90 degrees).
func RotateStateY(st plugin.BlockState, deg int) plugin.BlockState {
	steps := ((deg/90)%4 + 4) % 4
	if steps == 0 {
		return st
	}
	name, props, ok := registry.StateNameProps(uint16(st))
	if !ok || len(props) == 0 {
		return st
	}
	out := make(map[string]string, len(props))
	maps.Copy(out, props)
	for range steps {
		out = rotatePropsCW90(out)
	}
	if id, ok := registry.FindState(name, out); ok {
		return plugin.BlockState(id)
	}
	return st
}

func rotateDirCW90(dir string) string {
	switch dir {
	case "north":
		return "east"
	case "east":
		return "south"
	case "south":
		return "west"
	case "west":
		return "north"
	default:
		return dir
	}
}

func rotatePropsCW90(props map[string]string) map[string]string {
	out := make(map[string]string, len(props))
	maps.Copy(out, props)
	if f, ok := props["facing"]; ok {
		out["facing"] = rotateDirCW90(f)
	}
	if a, ok := props["axis"]; ok {
		switch a {
		case "x":
			out["axis"] = "z"
		case "z":
			out["axis"] = "x"
		}
	}
	if r, ok := props["rotation"]; ok {
		if n, err := strconv.Atoi(r); err == nil {
			out["rotation"] = strconv.Itoa((n + 4) % 16)
		}
	}
	if shape, ok := props["shape"]; ok {
		switch shape {
		case "north_south":
			out["shape"] = "east_west"
		case "east_west":
			out["shape"] = "north_south"
		case "ascending_north":
			out["shape"] = "ascending_east"
		case "ascending_east":
			out["shape"] = "ascending_south"
		case "ascending_south":
			out["shape"] = "ascending_west"
		case "ascending_west":
			out["shape"] = "ascending_north"
		case "south_east":
			out["shape"] = "south_west"
		case "south_west":
			out["shape"] = "north_west"
		case "north_west":
			out["shape"] = "north_east"
		case "north_east":
			out["shape"] = "south_east"
		}
	}
	// Rotate cardinal connection booleans/wall sides (north, east, south, west).
	_, hasN := props["north"]
	_, hasE := props["east"]
	_, hasS := props["south"]
	_, hasW := props["west"]
	if hasN && hasE && hasS && hasW {
		out["east"] = props["north"]
		out["south"] = props["east"]
		out["west"] = props["south"]
		out["north"] = props["west"]
	}
	return out
}

// FlipState flips a block state's directional properties across the given axis ("x", "y", or "z").
func FlipState(st plugin.BlockState, axis string) plugin.BlockState {
	name, props, ok := registry.StateNameProps(uint16(st))
	if !ok || len(props) == 0 {
		return st
	}
	out := make(map[string]string, len(props))
	maps.Copy(out, props)
	switch strings.ToLower(axis) {
	case "x":
		if f, ok := props["facing"]; ok {
			switch f {
			case "east":
				out["facing"] = "west"
			case "west":
				out["facing"] = "east"
			}
		}
		if e, okE := props["east"]; okE {
			if w, okW := props["west"]; okW {
				out["east"], out["west"] = w, e
			}
		}
		if h, ok := props["hinge"]; ok {
			switch h {
			case "left":
				out["hinge"] = "right"
			case "right":
				out["hinge"] = "left"
			}
		}
		if r, ok := props["rotation"]; ok {
			if n, err := strconv.Atoi(r); err == nil {
				out["rotation"] = strconv.Itoa((16 - n) % 16)
			}
		}
	case "z":
		if f, ok := props["facing"]; ok {
			switch f {
			case "north":
				out["facing"] = "south"
			case "south":
				out["facing"] = "north"
			}
		}
		if n, okN := props["north"]; okN {
			if s, okS := props["south"]; okS {
				out["north"], out["south"] = s, n
			}
		}
		if h, ok := props["hinge"]; ok {
			switch h {
			case "left":
				out["hinge"] = "right"
			case "right":
				out["hinge"] = "left"
			}
		}
		if r, ok := props["rotation"]; ok {
			if n, err := strconv.Atoi(r); err == nil {
				out["rotation"] = strconv.Itoa((8 - n + 16) % 16)
			}
		}
	case "y":
		if f, ok := props["facing"]; ok {
			switch f {
			case "up":
				out["facing"] = "down"
			case "down":
				out["facing"] = "up"
			}
		}
		if h, ok := props["half"]; ok {
			switch h {
			case "top":
				out["half"] = "bottom"
			case "bottom":
				out["half"] = "top"
			case "upper":
				out["half"] = "lower"
			case "lower":
				out["half"] = "upper"
			}
		}
		if tp, ok := props["type"]; ok {
			switch tp {
			case "top":
				out["type"] = "bottom"
			case "bottom":
				out["type"] = "top"
			}
		}
		if face, ok := props["face"]; ok {
			switch face {
			case "floor":
				out["face"] = "ceiling"
			case "ceiling":
				out["face"] = "floor"
			}
		}
	}
	if id, ok := registry.FindState(name, out); ok {
		return plugin.BlockState(id)
	}
	return st
}

// Transform modifies block placement coordinates and states (FAWE global/brush transforms).
type Transform interface {
	Apply(pos plugin.BlockPos, st plugin.BlockState, origin plugin.BlockPos) (plugin.BlockPos, plugin.BlockState)
	String() string
}

type offsetTransform struct {
	dx, dy, dz int
}

func (t offsetTransform) Apply(pos plugin.BlockPos, st plugin.BlockState, _ plugin.BlockPos) (plugin.BlockPos, plugin.BlockState) {
	return plugin.BlockPos{X: pos.X + t.dx, Y: pos.Y + t.dy, Z: pos.Z + t.dz}, st
}
func (t offsetTransform) String() string { return fmt.Sprintf("#offset[%d][%d][%d]", t.dx, t.dy, t.dz) }

type spreadTransform struct {
	dx, dy, dz int
}

func (t spreadTransform) Apply(pos plugin.BlockPos, st plugin.BlockState, _ plugin.BlockPos) (plugin.BlockPos, plugin.BlockState) {
	ox, oy, oz := 0, 0, 0
	if t.dx > 0 {
		ox = rand.IntN(2*t.dx+1) - t.dx
	}
	if t.dy > 0 {
		oy = rand.IntN(2*t.dy+1) - t.dy
	}
	if t.dz > 0 {
		oz = rand.IntN(2*t.dz+1) - t.dz
	}
	return plugin.BlockPos{X: pos.X + ox, Y: pos.Y + oy, Z: pos.Z + oz}, st
}
func (t spreadTransform) String() string { return fmt.Sprintf("#spread[%d][%d][%d]", t.dx, t.dy, t.dz) }

type scaleTransform struct {
	sx, sy, sz float64
}

func (t scaleTransform) Apply(pos plugin.BlockPos, st plugin.BlockState, origin plugin.BlockPos) (plugin.BlockPos, plugin.BlockState) {
	rx := float64(pos.X-origin.X) * t.sx
	ry := float64(pos.Y-origin.Y) * t.sy
	rz := float64(pos.Z-origin.Z) * t.sz
	return plugin.BlockPos{
		X: origin.X + int(math.Round(rx)),
		Y: origin.Y + int(math.Round(ry)),
		Z: origin.Z + int(math.Round(rz)),
	}, st
}
func (t scaleTransform) String() string {
	return fmt.Sprintf("#scale[%.2f][%.2f][%.2f]", t.sx, t.sy, t.sz)
}

type rotateTransform struct {
	degY int
}

func (t rotateTransform) Apply(pos plugin.BlockPos, st plugin.BlockState, origin plugin.BlockPos) (plugin.BlockPos, plugin.BlockState) {
	rad := float64(t.degY) * math.Pi / 180.0
	c, s := math.Cos(rad), math.Sin(rad)
	dx, dz := float64(pos.X-origin.X), float64(pos.Z-origin.Z)
	nx := dx*c - dz*s
	nz := dx*s + dz*c
	return plugin.BlockPos{
		X: origin.X + int(math.Round(nx)),
		Y: pos.Y,
		Z: origin.Z + int(math.Round(nz)),
	}, RotateStateY(st, t.degY)
}
func (t rotateTransform) String() string { return fmt.Sprintf("#rotate[%d]", t.degY) }

type patternTransform struct {
	pat Pattern
	ext Extent
}

func (t patternTransform) Apply(pos plugin.BlockPos, _ plugin.BlockState, _ plugin.BlockPos) (plugin.BlockPos, plugin.BlockState) {
	return pos, t.pat.Apply(t.ext, pos)
}
func (t patternTransform) String() string { return "#pattern" }

type multiTransform struct {
	items []Transform
	mode  string // "random", "linear", "linear3d", "chain"
	idx   int
}

func (t *multiTransform) Apply(pos plugin.BlockPos, st plugin.BlockState, origin plugin.BlockPos) (plugin.BlockPos, plugin.BlockState) {
	if len(t.items) == 0 {
		return pos, st
	}
	switch t.mode {
	case "random":
		return t.items[rand.IntN(len(t.items))].Apply(pos, st, origin)
	case "linear":
		i := t.idx % len(t.items)
		t.idx++
		return t.items[i].Apply(pos, st, origin)
	case "linear3d":
		i := ((pos.X+pos.Y+pos.Z)%len(t.items) + len(t.items)) % len(t.items)
		return t.items[i].Apply(pos, st, origin)
	default:
		for _, sub := range t.items {
			pos, st = sub.Apply(pos, st, origin)
		}
		return pos, st
	}
}

func (t *multiTransform) String() string {
	parts := make([]string, len(t.items))
	for i, it := range t.items {
		parts[i] = it.String()
	}
	return strings.Join(parts, ",")
}

// ParseTransform parses a FAWE transform specification such as "#offset[1][2][3]",
// "#rotate[90]", "#scale[2][2][2]", "#spread[2][0][2]", "#pattern[stone]",
// "#random[t1,t2]", "#linear[t1,t2]", "#linear3d[t1,t2]".
func ParseTransform(input string, ctx ParseContext) (Transform, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil, fmt.Errorf("empty transform")
	}
	parts := splitTopLevel(s, ',')
	if len(parts) > 1 {
		var items []Transform
		for _, p := range parts {
			tr, err := ParseTransform(p, ctx)
			if err != nil {
				return nil, err
			}
			items = append(items, tr)
		}
		return &multiTransform{items: items, mode: "chain"}, nil
	}
	name, args := parseRichArgs(s)
	switch strings.ToLower(strings.TrimPrefix(name, "#")) {
	case "offset":
		if len(args) < 3 {
			return nil, fmt.Errorf("#offset requires [x][y][z]")
		}
		x, err1 := strconv.Atoi(args[0])
		y, err2 := strconv.Atoi(args[1])
		z, err3 := strconv.Atoi(args[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return nil, fmt.Errorf("invalid #offset arguments: %v", args)
		}
		return offsetTransform{dx: x, dy: y, dz: z}, nil
	case "spread":
		if len(args) < 3 {
			return nil, fmt.Errorf("#spread requires [x][y][z]")
		}
		x, err1 := strconv.Atoi(args[0])
		y, err2 := strconv.Atoi(args[1])
		z, err3 := strconv.Atoi(args[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return nil, fmt.Errorf("invalid #spread arguments: %v", args)
		}
		return spreadTransform{dx: x, dy: y, dz: z}, nil
	case "scale":
		if len(args) < 1 {
			return nil, fmt.Errorf("#scale requires [sx] or [sx][sy][sz]")
		}
		sx, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid #scale argument: %w", err)
		}
		sy, sz := sx, sx
		if len(args) >= 3 {
			sy, _ = strconv.ParseFloat(args[1], 64)
			sz, _ = strconv.ParseFloat(args[2], 64)
		}
		return scaleTransform{sx: sx, sy: sy, sz: sz}, nil
	case "rotate", "rot":
		if len(args) < 1 {
			return nil, fmt.Errorf("#rotate requires [degrees]")
		}
		deg, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, fmt.Errorf("invalid #rotate degrees: %w", err)
		}
		return rotateTransform{degY: deg}, nil
	case "pattern":
		if len(args) < 1 {
			return nil, fmt.Errorf("#pattern requires [pattern]")
		}
		pat, err := ParsePattern(args[0], ctx)
		if err != nil {
			return nil, err
		}
		return patternTransform{pat: pat, ext: ctx.Extent}, nil
	case "random", "linear", "linear3d":
		mode := strings.ToLower(strings.TrimPrefix(name, "#"))
		if len(args) < 1 {
			return nil, fmt.Errorf("#%s requires sub-transforms", mode)
		}
		var items []Transform
		for _, a := range args {
			for _, sub := range splitTopLevel(a, ',') {
				tr, err := ParseTransform(sub, ctx)
				if err != nil {
					return nil, err
				}
				items = append(items, tr)
			}
		}
		return &multiTransform{items: items, mode: mode}, nil
	default:
		return nil, fmt.Errorf("unknown transform %q", input)
	}
}
