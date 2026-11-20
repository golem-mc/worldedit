// Package expr implements the WorldEdit mathematical expression language
// used by //calc, //generate, //generatebiome, //deform, expression masks (=...),
// expression patterns (#expression), and /br deform.
package expr

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ErrTimeout is returned when expression evaluation exceeds its deadline or step budget.
var ErrTimeout = errors.New("expression evaluation timed out")

// Env holds variables and callbacks for evaluating a compiled expression.
type Env struct {
	Vars     map[string]float64
	Deadline time.Time
	MaxSteps int
	steps    int

	// Optional world query callbacks for query(x,y,z), queryRel(dx,dy,dz), queryAbs(x,y,z).
	Query    func(x, y, z float64) float64
	QueryRel func(dx, dy, dz float64) float64
	// Optional noise callbacks.
	Perlin      func(seed int64, x, y, z, freq float64, octaves int) float64
	Simplex     func(seed int64, x, y, z, freq float64, octaves int) float64
	Voronoi     func(seed int64, x, y, z, freq float64) float64
	RidgedMulti func(seed int64, x, y, z, freq float64, octaves int) float64
}

// NewEnv initializes an evaluation environment with standard constants (pi, e).
func NewEnv(timeout time.Duration) *Env {
	e := &Env{
		Vars:     map[string]float64{"pi": math.Pi, "e": math.E, "true": 1, "false": 0},
		MaxSteps: 200000,
	}
	if timeout > 0 {
		e.Deadline = time.Now().Add(timeout)
	}
	return e
}

func (e *Env) tick() error {
	e.steps++
	if e.MaxSteps > 0 && e.steps > e.MaxSteps {
		return ErrTimeout
	}
	if e.steps&1023 == 0 && !e.Deadline.IsZero() && time.Now().After(e.Deadline) {
		return ErrTimeout
	}
	return nil
}

// Program is a compiled WorldEdit expression.
type Program struct {
	source string
	stmts  []stmt
}

// Source returns the original expression string.
func (p *Program) Source() string { return p.source }

// Compile parses a WorldEdit expression into an executable Program.
func Compile(src string) (*Program, error) {
	tokens, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	stmts, err := p.parseProgram()
	if err != nil {
		return nil, err
	}
	return &Program{source: src, stmts: stmts}, nil
}

// Eval executes the program against env and returns the final expression value.
func (p *Program) Eval(env *Env) (float64, error) {
	if env == nil {
		env = NewEnv(100 * time.Millisecond)
	}
	if env.Vars == nil {
		env.Vars = map[string]float64{"pi": math.Pi, "e": math.E, "true": 1, "false": 0}
	}
	env.steps = 0
	var last float64
	for _, s := range p.stmts {
		val, returned, err := s.exec(env)
		if err != nil {
			return 0, err
		}
		last = val
		if returned {
			return val, nil
		}
	}
	return last, nil
}

type tokKind int

const (
	tokEOF tokKind = iota
	tokNum
	tokIdent
	tokOp
	tokLParen
	tokRParen
	tokLBrace
	tokRBrace
	tokComma
	tokSemi
	tokQuestion
	tokColon
)

type token struct {
	kind tokKind
	text string
	num  float64
	pos  int
}

func lex(src string) ([]token, error) {
	var out []token
	i := 0
	n := len(src)
	for i < n {
		c := src[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			i++
			continue
		}
		if c == '/' && i+1 < n && src[i+1] == '/' {
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < n && src[i+1] == '*' {
			i += 2
			for i+1 < n && (src[i] != '*' || src[i+1] != '/') {
				i++
			}
			i += 2
			continue
		}
		start := i
		switch c {
		case '(':
			out = append(out, token{kind: tokLParen, text: "(", pos: start})
			i++
			continue
		case ')':
			out = append(out, token{kind: tokRParen, text: ")", pos: start})
			i++
			continue
		case '{':
			out = append(out, token{kind: tokLBrace, text: "{", pos: start})
			i++
			continue
		case '}':
			out = append(out, token{kind: tokRBrace, text: "}", pos: start})
			i++
			continue
		case ',':
			out = append(out, token{kind: tokComma, text: ",", pos: start})
			i++
			continue
		case ';':
			out = append(out, token{kind: tokSemi, text: ";", pos: start})
			i++
			continue
		case '?':
			out = append(out, token{kind: tokQuestion, text: "?", pos: start})
			i++
			continue
		case ':':
			out = append(out, token{kind: tokColon, text: ":", pos: start})
			i++
			continue
		}
		// Number
		if (c >= '0' && c <= '9') || (c == '.' && i+1 < n && src[i+1] >= '0' && src[i+1] <= '9') {
			if c == '0' && i+1 < n && (src[i+1] == 'x' || src[i+1] == 'X') {
				i += 2
				for i < n && strings.ContainsRune("0123456789abcdefABCDEF", rune(src[i])) {
					i++
				}
				v, err := strconv.ParseInt(src[start+2:i], 16, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid hex literal at %d: %w", start, err)
				}
				out = append(out, token{kind: tokNum, text: src[start:i], num: float64(v), pos: start})
				continue
			}
			for i < n && src[i] >= '0' && src[i] <= '9' {
				i++
			}
			if i < n && src[i] == '.' {
				i++
				for i < n && src[i] >= '0' && src[i] <= '9' {
					i++
				}
			}
			if i < n && (src[i] == 'e' || src[i] == 'E') {
				i++
				if i < n && (src[i] == '+' || src[i] == '-') {
					i++
				}
				for i < n && src[i] >= '0' && src[i] <= '9' {
					i++
				}
			}
			v, err := strconv.ParseFloat(src[start:i], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number %q at %d", src[start:i], start)
			}
			out = append(out, token{kind: tokNum, text: src[start:i], num: v, pos: start})
			continue
		}
		// Identifier
		if unicode.IsLetter(rune(c)) || c == '_' {
			for i < n && (unicode.IsLetter(rune(src[i])) || unicode.IsDigit(rune(src[i])) || src[i] == '_') {
				i++
			}
			out = append(out, token{kind: tokIdent, text: src[start:i], pos: start})
			continue
		}
		// Two-character operators
		if i+1 < n {
			two := src[i : i+2]
			switch two {
			case "==", "!=", "<=", ">=", "~=", "&&", "||", "**", "<<", ">>", "++", "--", "+=", "-=", "*=", "/=", "%=", "^=":
				out = append(out, token{kind: tokOp, text: two, pos: start})
				i += 2
				continue
			}
		}
		// Single-character operators
		if strings.ContainsRune("+-*/%^<>=!~", rune(c)) {
			out = append(out, token{kind: tokOp, text: src[i : i+1], pos: start})
			i++
			continue
		}
		return nil, fmt.Errorf("unexpected character %q at position %d", c, start)
	}
	out = append(out, token{kind: tokEOF, pos: n})
	return out, nil
}

type stmt interface {
	exec(env *Env) (val float64, returned bool, err error)
}

type exprNode interface {
	eval(env *Env) (float64, error)
}

type exprStmt struct{ e exprNode }

func (s exprStmt) exec(env *Env) (float64, bool, error) {
	v, err := s.e.eval(env)
	return v, false, err
}

type blockStmt struct{ stmts []stmt }

func (b blockStmt) exec(env *Env) (float64, bool, error) {
	var last float64
	for _, s := range b.stmts {
		v, ret, err := s.exec(env)
		if err != nil {
			return 0, false, err
		}
		last = v
		if ret {
			return v, true, nil
		}
	}
	return last, false, nil
}

type returnStmt struct{ e exprNode }

func (r returnStmt) exec(env *Env) (float64, bool, error) {
	if r.e == nil {
		return 0, true, nil
	}
	v, err := r.e.eval(env)
	return v, true, err
}

type ifStmt struct {
	cond exprNode
	then stmt
	els  stmt
}

func (s ifStmt) exec(env *Env) (float64, bool, error) {
	if err := env.tick(); err != nil {
		return 0, false, err
	}
	c, err := s.cond.eval(env)
	if err != nil {
		return 0, false, err
	}
	if c != 0 {
		return s.then.exec(env)
	}
	if s.els != nil {
		return s.els.exec(env)
	}
	return 0, false, nil
}

type whileStmt struct {
	cond exprNode
	body stmt
}

func (s whileStmt) exec(env *Env) (float64, bool, error) {
	var last float64
	for {
		if err := env.tick(); err != nil {
			return 0, false, err
		}
		c, err := s.cond.eval(env)
		if err != nil {
			return 0, false, err
		}
		if c == 0 {
			return last, false, nil
		}
		v, ret, err := s.body.exec(env)
		if err != nil {
			return 0, false, err
		}
		last = v
		if ret {
			return v, true, nil
		}
	}
}

type forStmt struct {
	init exprNode
	cond exprNode
	post exprNode
	body stmt
}

func (s forStmt) exec(env *Env) (float64, bool, error) {
	if s.init != nil {
		if _, err := s.init.eval(env); err != nil {
			return 0, false, err
		}
	}
	var last float64
	for {
		if err := env.tick(); err != nil {
			return 0, false, err
		}
		if s.cond != nil {
			c, err := s.cond.eval(env)
			if err != nil {
				return 0, false, err
			}
			if c == 0 {
				return last, false, nil
			}
		}
		v, ret, err := s.body.exec(env)
		if err != nil {
			return 0, false, err
		}
		last = v
		if ret {
			return v, true, nil
		}
		if s.post != nil {
			if _, err := s.post.eval(env); err != nil {
				return 0, false, err
			}
		}
	}
}

type numExpr struct{ v float64 }

func (n numExpr) eval(*Env) (float64, error) { return n.v, nil }

type varExpr struct{ name string }

func (v varExpr) eval(env *Env) (float64, error) {
	return env.Vars[v.name], nil
}

type assignExpr struct {
	name string
	op   string
	rhs  exprNode
}

func (a assignExpr) eval(env *Env) (float64, error) {
	if err := env.tick(); err != nil {
		return 0, err
	}
	rv, err := a.rhs.eval(env)
	if err != nil {
		return 0, err
	}
	cur := env.Vars[a.name]
	switch a.op {
	case "=":
		cur = rv
	case "+=":
		cur += rv
	case "-=":
		cur -= rv
	case "*=":
		cur *= rv
	case "/=":
		cur /= rv
	case "%=":
		cur = math.Mod(cur, rv)
	case "^=":
		cur = math.Pow(cur, rv)
	}
	env.Vars[a.name] = cur
	return cur, nil
}

type incDecExpr struct {
	name   string
	delta  float64
	prefix bool
}

func (id incDecExpr) eval(env *Env) (float64, error) {
	cur := env.Vars[id.name]
	next := cur + id.delta
	env.Vars[id.name] = next
	if id.prefix {
		return next, nil
	}
	return cur, nil
}

type unaryExpr struct {
	op string
	x  exprNode
}

func (u unaryExpr) eval(env *Env) (float64, error) {
	v, err := u.x.eval(env)
	if err != nil {
		return 0, err
	}
	switch u.op {
	case "-":
		return -v, nil
	case "+":
		return v, nil
	case "!":
		if v == 0 {
			return 1, nil
		}
		return 0, nil
	case "~":
		return float64(^int64(v)), nil
	}
	return v, nil
}

type binExpr struct {
	op   string
	l, r exprNode
}

func (b binExpr) eval(env *Env) (float64, error) {
	if err := env.tick(); err != nil {
		return 0, err
	}
	lv, err := b.l.eval(env)
	if err != nil {
		return 0, err
	}
	switch b.op {
	case "&&":
		if lv == 0 {
			return 0, nil
		}
		rv, err := b.r.eval(env)
		if err != nil {
			return 0, err
		}
		if rv != 0 {
			return 1, nil
		}
		return 0, nil
	case "||":
		if lv != 0 {
			return 1, nil
		}
		rv, err := b.r.eval(env)
		if err != nil {
			return 0, err
		}
		if rv != 0 {
			return 1, nil
		}
		return 0, nil
	}
	rv, err := b.r.eval(env)
	if err != nil {
		return 0, err
	}
	switch b.op {
	case "+":
		return lv + rv, nil
	case "-":
		return lv - rv, nil
	case "*":
		return lv * rv, nil
	case "/":
		return lv / rv, nil
	case "%":
		return math.Mod(lv, rv), nil
	case "^", "**":
		return math.Pow(lv, rv), nil
	case "<<":
		return float64(int64(lv) << uint(rv)), nil
	case ">>":
		return float64(int64(lv) >> uint(rv)), nil
	case "==":
		return boolNum(lv == rv), nil
	case "!=":
		return boolNum(lv != rv), nil
	case "~=":
		return boolNum(math.Abs(lv-rv) < 1e-6), nil
	case "<":
		return boolNum(lv < rv), nil
	case "<=":
		return boolNum(lv <= rv), nil
	case ">":
		return boolNum(lv > rv), nil
	case ">=":
		return boolNum(lv >= rv), nil
	}
	return 0, fmt.Errorf("unknown operator %q", b.op)
}

func boolNum(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

type ternaryExpr struct {
	cond, t, f exprNode
}

func (te ternaryExpr) eval(env *Env) (float64, error) {
	c, err := te.cond.eval(env)
	if err != nil {
		return 0, err
	}
	if c != 0 {
		return te.t.eval(env)
	}
	return te.f.eval(env)
}

type callExpr struct {
	name string
	args []exprNode
}

func (c callExpr) eval(env *Env) (float64, error) {
	if err := env.tick(); err != nil {
		return 0, err
	}
	lname := strings.ToLower(c.name)

	// Special mutating built-ins: rotate(x, y, angle) and swap(a, b)
	if lname == "rotate" && len(c.args) == 3 {
		v1, ok1 := c.args[0].(varExpr)
		v2, ok2 := c.args[1].(varExpr)
		if !ok1 || !ok2 {
			return 0, errors.New("rotate requires two variable names as first two arguments")
		}
		ang, err := c.args[2].eval(env)
		if err != nil {
			return 0, err
		}
		x, y := env.Vars[v1.name], env.Vars[v2.name]
		cosA, sinA := math.Cos(ang), math.Sin(ang)
		env.Vars[v1.name] = x*cosA - y*sinA
		env.Vars[v2.name] = x*sinA + y*cosA
		return 0, nil
	}
	if lname == "swap" && len(c.args) == 2 {
		v1, ok1 := c.args[0].(varExpr)
		v2, ok2 := c.args[1].(varExpr)
		if !ok1 || !ok2 {
			return 0, errors.New("swap requires two variable names")
		}
		env.Vars[v1.name], env.Vars[v2.name] = env.Vars[v2.name], env.Vars[v1.name]
		return 0, nil
	}

	vals := make([]float64, len(c.args))
	for i, a := range c.args {
		v, err := a.eval(env)
		if err != nil {
			return 0, err
		}
		vals[i] = v
	}

	need := func(n int) error {
		if len(vals) != n {
			return fmt.Errorf("%s expects %d argument(s), got %d", c.name, n, len(vals))
		}
		return nil
	}

	switch lname {
	case "abs":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Abs(vals[0]), nil
	case "acos":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Acos(vals[0]), nil
	case "asin":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Asin(vals[0]), nil
	case "atan":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Atan(vals[0]), nil
	case "atan2":
		if err := need(2); err != nil {
			return 0, err
		}
		return math.Atan2(vals[0], vals[1]), nil
	case "cbrt":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Cbrt(vals[0]), nil
	case "ceil":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Ceil(vals[0]), nil
	case "clamp":
		if err := need(3); err != nil {
			return 0, err
		}
		return max(vals[1], min(vals[2], vals[0])), nil
	case "cos":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Cos(vals[0]), nil
	case "cosh":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Cosh(vals[0]), nil
	case "exp":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Exp(vals[0]), nil
	case "floor":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Floor(vals[0]), nil
	case "ln", "log":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Log(vals[0]), nil
	case "log10":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Log10(vals[0]), nil
	case "max":
		if len(vals) < 1 {
			return 0, errors.New("max expects at least 1 argument")
		}
		m := vals[0]
		for _, v := range vals[1:] {
			m = max(m, v)
		}
		return m, nil
	case "min":
		if len(vals) < 1 {
			return 0, errors.New("min expects at least 1 argument")
		}
		m := vals[0]
		for _, v := range vals[1:] {
			m = min(m, v)
		}
		return m, nil
	case "pow":
		if err := need(2); err != nil {
			return 0, err
		}
		return math.Pow(vals[0], vals[1]), nil
	case "random":
		if len(vals) == 0 {
			return rand.Float64(), nil
		}
		if len(vals) == 1 {
			return rand.Float64() * vals[0], nil
		}
		return vals[0] + rand.Float64()*(vals[1]-vals[0]), nil
	case "rint":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.RoundToEven(vals[0]), nil
	case "round":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Round(vals[0]), nil
	case "sign", "signum":
		if err := need(1); err != nil {
			return 0, err
		}
		if vals[0] > 0 {
			return 1, nil
		}
		if vals[0] < 0 {
			return -1, nil
		}
		return 0, nil
	case "sin":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Sin(vals[0]), nil
	case "sinh":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Sinh(vals[0]), nil
	case "sqrt":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Sqrt(vals[0]), nil
	case "tan":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Tan(vals[0]), nil
	case "tanh":
		if err := need(1); err != nil {
			return 0, err
		}
		return math.Tanh(vals[0]), nil
	case "deg", "todegrees":
		if err := need(1); err != nil {
			return 0, err
		}
		return vals[0] * 180.0 / math.Pi, nil
	case "rad", "toradians":
		if err := need(1); err != nil {
			return 0, err
		}
		return vals[0] * math.Pi / 180.0, nil
	case "query", "queryabs":
		if len(vals) < 3 {
			return 0, fmt.Errorf("%s expects at least 3 arguments", c.name)
		}
		if env.Query != nil {
			return env.Query(vals[0], vals[1], vals[2]), nil
		}
		return 0, nil
	case "queryrel":
		if len(vals) < 3 {
			return 0, fmt.Errorf("%s expects at least 3 arguments", c.name)
		}
		if env.QueryRel != nil {
			return env.QueryRel(vals[0], vals[1], vals[2]), nil
		}
		return 0, nil
	case "perlin":
		if len(vals) < 5 {
			return 0, errors.New("perlin expects (seed, x, y, z, frequency[, octaves])")
		}
		oct := 1
		if len(vals) >= 6 {
			oct = max(1, int(vals[5]))
		}
		if env.Perlin != nil {
			return env.Perlin(int64(vals[0]), vals[1], vals[2], vals[3], vals[4], oct), nil
		}
		return 0, nil
	case "simplex":
		if len(vals) < 5 {
			return 0, errors.New("simplex expects (seed, x, y, z, frequency[, octaves])")
		}
		oct := 1
		if len(vals) >= 6 {
			oct = max(1, int(vals[5]))
		}
		if env.Simplex != nil {
			return env.Simplex(int64(vals[0]), vals[1], vals[2], vals[3], vals[4], oct), nil
		}
		return 0, nil
	case "voronoi":
		if len(vals) < 5 {
			return 0, errors.New("voronoi expects (seed, x, y, z, frequency)")
		}
		if env.Voronoi != nil {
			return env.Voronoi(int64(vals[0]), vals[1], vals[2], vals[3], vals[4]), nil
		}
		return 0, nil
	case "ridgedmulti":
		if len(vals) < 5 {
			return 0, errors.New("ridgedmulti expects (seed, x, y, z, frequency[, octaves])")
		}
		oct := 1
		if len(vals) >= 6 {
			oct = max(1, int(vals[5]))
		}
		if env.RidgedMulti != nil {
			return env.RidgedMulti(int64(vals[0]), vals[1], vals[2], vals[3], vals[4], oct), nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown function %q", c.name)
	}
}

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) cur() token { return p.tokens[p.pos] }
func (p *parser) next() token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *parser) expect(kind tokKind, desc string) (token, error) {
	t := p.cur()
	if t.kind != kind {
		return token{}, fmt.Errorf("expected %s at %d, got %q", desc, t.pos, t.text)
	}
	p.next()
	return t, nil
}

func (p *parser) parseProgram() ([]stmt, error) {
	var stmts []stmt
	for p.cur().kind != tokEOF {
		for p.cur().kind == tokSemi {
			p.next()
		}
		if p.cur().kind == tokEOF {
			break
		}
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, s)
		for p.cur().kind == tokSemi {
			p.next()
		}
	}
	return stmts, nil
}

func (p *parser) parseStmt() (stmt, error) {
	if p.cur().kind == tokLBrace {
		p.next()
		var stmts []stmt
		for p.cur().kind != tokRBrace && p.cur().kind != tokEOF {
			for p.cur().kind == tokSemi {
				p.next()
			}
			if p.cur().kind == tokRBrace {
				break
			}
			s, err := p.parseStmt()
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, s)
			for p.cur().kind == tokSemi {
				p.next()
			}
		}
		if _, err := p.expect(tokRBrace, "'}'"); err != nil {
			return nil, err
		}
		return blockStmt{stmts: stmts}, nil
	}
	if p.cur().kind == tokIdent {
		switch p.cur().text {
		case "if":
			p.next()
			if _, err := p.expect(tokLParen, "'('"); err != nil {
				return nil, err
			}
			cond, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(tokRParen, "')'"); err != nil {
				return nil, err
			}
			thenS, err := p.parseStmt()
			if err != nil {
				return nil, err
			}
			for p.cur().kind == tokSemi {
				p.next()
			}
			var els stmt
			if p.cur().kind == tokIdent && p.cur().text == "else" {
				p.next()
				els, err = p.parseStmt()
				if err != nil {
					return nil, err
				}
			}
			return ifStmt{cond: cond, then: thenS, els: els}, nil
		case "while":
			p.next()
			if _, err := p.expect(tokLParen, "'('"); err != nil {
				return nil, err
			}
			cond, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(tokRParen, "')'"); err != nil {
				return nil, err
			}
			body, err := p.parseStmt()
			if err != nil {
				return nil, err
			}
			return whileStmt{cond: cond, body: body}, nil
		case "for":
			p.next()
			if _, err := p.expect(tokLParen, "'('"); err != nil {
				return nil, err
			}
			var initE, condE, postE exprNode
			var err error
			if p.cur().kind != tokSemi {
				initE, err = p.parseExpr()
				if err != nil {
					return nil, err
				}
			}
			if _, err := p.expect(tokSemi, "';'"); err != nil {
				return nil, err
			}
			if p.cur().kind != tokSemi {
				condE, err = p.parseExpr()
				if err != nil {
					return nil, err
				}
			}
			if _, err := p.expect(tokSemi, "';'"); err != nil {
				return nil, err
			}
			if p.cur().kind != tokRParen {
				postE, err = p.parseExpr()
				if err != nil {
					return nil, err
				}
			}
			if _, err := p.expect(tokRParen, "')'"); err != nil {
				return nil, err
			}
			body, err := p.parseStmt()
			if err != nil {
				return nil, err
			}
			return forStmt{init: initE, cond: condE, post: postE, body: body}, nil
		case "return":
			p.next()
			if p.cur().kind == tokSemi || p.cur().kind == tokRBrace || p.cur().kind == tokEOF {
				return returnStmt{}, nil
			}
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			return returnStmt{e: e}, nil
		}
	}
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return exprStmt{e: e}, nil
}

func (p *parser) parseExpr() (exprNode, error) {
	return p.parseAssign()
}

func (p *parser) parseAssign() (exprNode, error) {
	left, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if p.cur().kind == tokOp {
		switch p.cur().text {
		case "=", "+=", "-=", "*=", "/=", "%=", "^=":
			ve, ok := left.(varExpr)
			if !ok {
				return nil, fmt.Errorf("left side of %s must be a variable", p.cur().text)
			}
			op := p.next().text
			rhs, err := p.parseAssign()
			if err != nil {
				return nil, err
			}
			return assignExpr{name: ve.name, op: op, rhs: rhs}, nil
		}
	}
	return left, nil
}

func (p *parser) parseTernary() (exprNode, error) {
	cond, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.cur().kind == tokQuestion {
		p.next()
		t, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokColon, "':'"); err != nil {
			return nil, err
		}
		f, err := p.parseTernary()
		if err != nil {
			return nil, err
		}
		return ternaryExpr{cond: cond, t: t, f: f}, nil
	}
	return cond, nil
}

func (p *parser) parseOr() (exprNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tokOp && p.cur().text == "||" {
		op := p.next().text
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = binExpr{op: op, l: left, r: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (exprNode, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tokOp && p.cur().text == "&&" {
		op := p.next().text
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = binExpr{op: op, l: left, r: right}
	}
	return left, nil
}

func (p *parser) parseEquality() (exprNode, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tokOp && (p.cur().text == "==" || p.cur().text == "!=" || p.cur().text == "~=") {
		op := p.next().text
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = binExpr{op: op, l: left, r: right}
	}
	return left, nil
}

func (p *parser) parseComparison() (exprNode, error) {
	left, err := p.parseShift()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tokOp && (p.cur().text == "<" || p.cur().text == "<=" || p.cur().text == ">" || p.cur().text == ">=") {
		op := p.next().text
		right, err := p.parseShift()
		if err != nil {
			return nil, err
		}
		left = binExpr{op: op, l: left, r: right}
	}
	return left, nil
}

func (p *parser) parseShift() (exprNode, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tokOp && (p.cur().text == "<<" || p.cur().text == ">>") {
		op := p.next().text
		right, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		left = binExpr{op: op, l: left, r: right}
	}
	return left, nil
}

func (p *parser) parseAdd() (exprNode, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tokOp && (p.cur().text == "+" || p.cur().text == "-") {
		op := p.next().text
		right, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		left = binExpr{op: op, l: left, r: right}
	}
	return left, nil
}

func (p *parser) parseMul() (exprNode, error) {
	left, err := p.parsePow()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tokOp && (p.cur().text == "*" || p.cur().text == "/" || p.cur().text == "%") {
		op := p.next().text
		right, err := p.parsePow()
		if err != nil {
			return nil, err
		}
		left = binExpr{op: op, l: left, r: right}
	}
	return left, nil
}

func (p *parser) parsePow() (exprNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	if p.cur().kind == tokOp && (p.cur().text == "^" || p.cur().text == "**") {
		op := p.next().text
		right, err := p.parsePow() // right-associative
		if err != nil {
			return nil, err
		}
		return binExpr{op: op, l: left, r: right}, nil
	}
	return left, nil
}

func (p *parser) parseUnary() (exprNode, error) {
	if p.cur().kind == tokOp {
		switch p.cur().text {
		case "+", "-", "!", "~":
			op := p.next().text
			x, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return unaryExpr{op: op, x: x}, nil
		case "++", "--":
			op := p.next().text
			tok, err := p.expect(tokIdent, "variable")
			if err != nil {
				return nil, err
			}
			delta := 1.0
			if op == "--" {
				delta = -1.0
			}
			return incDecExpr{name: tok.text, delta: delta, prefix: true}, nil
		}
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (exprNode, error) {
	t := p.cur()
	switch t.kind {
	case tokNum:
		p.next()
		return numExpr{v: t.num}, nil
	case tokIdent:
		p.next()
		if p.cur().kind == tokLParen {
			p.next()
			var args []exprNode
			if p.cur().kind != tokRParen {
				for {
					a, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if p.cur().kind != tokComma {
						break
					}
					p.next()
				}
			}
			if _, err := p.expect(tokRParen, "')'"); err != nil {
				return nil, err
			}
			return callExpr{name: t.text, args: args}, nil
		}
		if p.cur().kind == tokOp && (p.cur().text == "++" || p.cur().text == "--") {
			op := p.next().text
			delta := 1.0
			if op == "--" {
				delta = -1.0
			}
			return incDecExpr{name: t.text, delta: delta, prefix: false}, nil
		}
		return varExpr{name: t.text}, nil
	case tokLParen:
		p.next()
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokRParen, "')'"); err != nil {
			return nil, err
		}
		return e, nil
	default:
		return nil, fmt.Errorf("unexpected token %q at position %d", t.text, t.pos)
	}
}
