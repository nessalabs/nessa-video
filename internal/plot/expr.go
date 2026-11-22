// Package plot is the mathematics behind the plot block: a small
// expression language evaluated in complex numbers, the special functions
// it offers (gamma, zeta), adaptive sampling of curves, and the view model
// that zooms a plot's axes toward a point.
package plot

import (
	"fmt"
	"math"
	"math/cmplx"
	"strconv"
	"strings"
	"unicode"
)

// Expr is a compiled expression. Variables are read from a slice in the
// order of the names it was compiled with.
type Expr struct {
	Src   string
	root  node
	Names []string // the variables it may read, by slot
}

// Eval evaluates the expression with vars[i] the value of Names[i].
func (e *Expr) Eval(vars []complex128) complex128 { return e.root.eval(vars) }

// Uses reports whether the expression reads the variable name.
func (e *Expr) Uses(name string) bool {
	slot := -1
	for i, n := range e.Names {
		if n == name {
			slot = i
		}
	}
	return slot >= 0 && e.root.uses(slot)
}

type node interface {
	eval(v []complex128) complex128
	uses(slot int) bool
}

type num complex128

func (n num) eval([]complex128) complex128 { return complex128(n) }
func (num) uses(int) bool                  { return false }

type variable int

func (s variable) eval(v []complex128) complex128 { return v[s] }
func (s variable) uses(slot int) bool             { return int(s) == slot }

type binary struct {
	op   byte
	a, b node
}

func (b binary) eval(v []complex128) complex128 {
	x, y := b.a.eval(v), b.b.eval(v)
	switch b.op {
	case '+':
		return x + y
	case '-':
		return x - y
	case '*':
		return x * y
	case '/':
		if imag(y) == 0 && imag(x) == 0 {
			return complex(real(x)/real(y), 0)
		}
		return x / y
	default: // '^'
		return power(x, y)
	}
}
func (b binary) uses(slot int) bool { return b.a.uses(slot) || b.b.uses(slot) }

type neg struct{ a node }

func (n neg) eval(v []complex128) complex128 { return negate(n.a.eval(v)) }

// negate flips a number's sign without leaving a real number with a
// negative zero imaginary part, which would put sqrt(-4) on the wrong side
// of its branch cut.
func negate(z complex128) complex128 {
	if imag(z) == 0 {
		return complex(-real(z), 0)
	}
	return -z
}
func (n neg) uses(slot int) bool { return n.a.uses(slot) }

type call struct {
	fn   func(args []complex128) complex128
	args []node
}

func (c call) eval(v []complex128) complex128 {
	var buf [4]complex128
	args := buf[:0]
	for _, a := range c.args {
		args = append(args, a.eval(v))
	}
	return c.fn(args)
}
func (c call) uses(slot int) bool {
	for _, a := range c.args {
		if a.uses(slot) {
			return true
		}
	}
	return false
}

// power raises x to y, staying real when both are real and the result is.
func power(x, y complex128) complex128 {
	if imag(x) == 0 && imag(y) == 0 {
		a, b := real(x), real(y)
		if a >= 0 || b == math.Trunc(b) {
			return complex(math.Pow(a, b), 0)
		}
	}
	if x == 0 {
		if real(y) > 0 {
			return 0
		}
		return cmplx.Inf()
	}
	return cmplx.Pow(x, y)
}

// real1 lifts a real function to complex numbers: a real argument with a
// real result stays real, anything else uses the complex function.
func real1(rf func(float64) float64, cf func(complex128) complex128) func([]complex128) complex128 {
	return func(a []complex128) complex128 {
		if imag(a[0]) == 0 {
			if r := rf(real(a[0])); !math.IsNaN(r) {
				return complex(r, 0)
			}
		}
		return cf(a[0])
	}
}

// Functions lists the functions expressions may call, with their arity
// (-1 for one or more).
var functions = map[string]struct {
	arity int
	fn    func([]complex128) complex128
}{
	"sin":  {1, real1(math.Sin, cmplx.Sin)},
	"cos":  {1, real1(math.Cos, cmplx.Cos)},
	"tan":  {1, real1(math.Tan, cmplx.Tan)},
	"asin": {1, real1(math.Asin, cmplx.Asin)},
	"acos": {1, real1(math.Acos, cmplx.Acos)},
	"atan": {1, real1(math.Atan, cmplx.Atan)},
	"exp":  {1, real1(math.Exp, cmplx.Exp)},
	"log":  {1, real1(math.Log, cmplx.Log)},
	"ln":   {1, real1(math.Log, cmplx.Log)},
	"sqrt": {1, real1(math.Sqrt, cmplx.Sqrt)},
	"abs":  {1, func(a []complex128) complex128 { return complex(cmplx.Abs(a[0]), 0) }},
	"floor": {1, func(a []complex128) complex128 {
		return complex(math.Floor(real(a[0])), 0)
	}},
	"re":    {1, func(a []complex128) complex128 { return complex(real(a[0]), 0) }},
	"im":    {1, func(a []complex128) complex128 { return complex(imag(a[0]), 0) }},
	"arg":   {1, func(a []complex128) complex128 { return complex(cmplx.Phase(a[0]), 0) }},
	"conj":  {1, func(a []complex128) complex128 { return cmplx.Conj(a[0]) }},
	"gamma": {1, func(a []complex128) complex128 { return Gamma(a[0]) }},
	"zeta":  {1, func(a []complex128) complex128 { return Zeta(a[0]) }},
	"min": {-1, func(a []complex128) complex128 {
		m := real(a[0])
		for _, x := range a[1:] {
			m = math.Min(m, real(x))
		}
		return complex(m, 0)
	}},
	"max": {-1, func(a []complex128) complex128 {
		m := real(a[0])
		for _, x := range a[1:] {
			m = math.Max(m, real(x))
		}
		return complex(m, 0)
	}},
}

// IsFunction reports whether name is a built-in function.
func IsFunction(name string) bool { _, ok := functions[name]; return ok }

// FunctionNames lists the built-in functions, for messages.
func FunctionNames() string {
	return "sin cos tan asin acos atan exp log ln sqrt abs floor min max re im arg conj gamma zeta"
}

var constants = map[string]complex128{"pi": complex(math.Pi, 0), "π": complex(math.Pi, 0), "e": complex(math.E, 0), "i": complex(0, 1)}

// token kinds
const (
	tEOF = iota
	tNum
	tIdent
	tOp // + - * / ^ ( ) ,
)

type token struct {
	kind int
	text string
	val  float64
	pos  int
}

func lex(src string) ([]token, error) {
	var out []token
	rs := []rune(src)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case unicode.IsDigit(r) || r == '.' && i+1 < len(rs) && unicode.IsDigit(rs[i+1]):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			// an exponent only when digits follow, so 2e is 2 times e
			if j < len(rs) && (rs[j] == 'e' || rs[j] == 'E') {
				k := j + 1
				if k < len(rs) && (rs[k] == '+' || rs[k] == '-') {
					k++
				}
				if k < len(rs) && unicode.IsDigit(rs[k]) {
					for k < len(rs) && unicode.IsDigit(rs[k]) {
						k++
					}
					j = k
				}
			}
			s := string(rs[i:j])
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", s)
			}
			out = append(out, token{kind: tNum, text: s, val: f, pos: i})
			i = j
		case unicode.IsLetter(r) || r == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			out = append(out, token{kind: tIdent, text: string(rs[i:j]), pos: i})
			i = j
		case strings.ContainsRune("+-*/^(),·×−", r):
			op := string(r)
			switch r {
			case '·', '×':
				op = "*"
			case '−':
				op = "-"
			}
			if r == '*' && i+1 < len(rs) && rs[i+1] == '*' { // ** is ^
				op = "^"
				i++
			}
			out = append(out, token{kind: tOp, text: op, pos: i})
			i++
		default:
			return nil, fmt.Errorf("unexpected %q", string(r))
		}
	}
	return append(out, token{kind: tEOF, pos: len(rs)}), nil
}

type parser struct {
	toks  []token
	i     int
	names []string
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) next() token { t := p.toks[p.i]; p.i++; return t }

// binding powers
const (
	bpSum   = 10
	bpProd  = 20
	bpUnary = 25
	bpPow   = 30
)

// Compile parses src. names are the variables it may read; any other
// identifier must be a constant (pi, e, i) or a function.
func Compile(src string, names ...string) (*Expr, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	if len(toks) == 1 {
		return nil, fmt.Errorf("empty expression")
	}
	p := &parser{toks: toks, names: names}
	n, err := p.expr(0)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, fmt.Errorf("unexpected %q", t.text)
	}
	return &Expr{Src: src, root: fold(n), Names: names}, nil
}

// startsOperand reports whether t can begin an operand, for implicit
// multiplication such as 2x, 3(t+1) or (x+1)(x-1).
func startsOperand(t token) bool {
	return t.kind == tNum || t.kind == tIdent || t.kind == tOp && t.text == "("
}

func (p *parser) expr(minBP int) (node, error) {
	left, err := p.prefix()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		var op byte
		var lbp, rbp int
		implicit := false
		switch {
		case t.kind == tOp && (t.text == "+" || t.text == "-"):
			op, lbp, rbp = t.text[0], bpSum, bpSum+1
		case t.kind == tOp && (t.text == "*" || t.text == "/"):
			op, lbp, rbp = t.text[0], bpProd, bpProd+1
		case t.kind == tOp && t.text == "^":
			op, lbp, rbp = '^', bpPow, bpPow-1 // right associative, and 2^-x works through prefix
		case startsOperand(t):
			op, lbp, rbp, implicit = '*', bpProd, bpProd+1, true
		default:
			return left, nil
		}
		if lbp < minBP {
			return left, nil
		}
		if !implicit {
			p.next()
		}
		right, err := p.expr(rbp)
		if err != nil {
			return nil, err
		}
		left = binary{op: op, a: left, b: right}
	}
}

func (p *parser) prefix() (node, error) {
	t := p.next()
	switch t.kind {
	case tNum:
		return num(complex(t.val, 0)), nil
	case tIdent:
		if f, ok := functions[t.text]; ok {
			if n := p.peek(); n.kind != tOp || n.text != "(" {
				return nil, fmt.Errorf("%s needs its argument in brackets, e.g. %s(x)", t.text, t.text)
			}
			p.next()
			var args []node
			for {
				a, err := p.expr(0)
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				n := p.next()
				if n.kind == tOp && n.text == "," {
					continue
				}
				if n.kind == tOp && n.text == ")" {
					break
				}
				return nil, fmt.Errorf("expected ) after the arguments of %s", t.text)
			}
			if f.arity > 0 && len(args) != f.arity {
				return nil, fmt.Errorf("%s takes %d argument(s), got %d", t.text, f.arity, len(args))
			}
			return call{fn: f.fn, args: args}, nil
		}
		for i, n := range p.names {
			if n == t.text {
				return variable(i), nil
			}
		}
		if v, ok := constants[t.text]; ok {
			return num(v), nil
		}
		return nil, unknownName(t.text, p.names)
	case tOp:
		switch t.text {
		case "-":
			a, err := p.expr(bpUnary)
			if err != nil {
				return nil, err
			}
			return neg{a}, nil
		case "+":
			return p.expr(bpUnary)
		case "(":
			a, err := p.expr(0)
			if err != nil {
				return nil, err
			}
			if n := p.next(); n.kind != tOp || n.text != ")" {
				return nil, fmt.Errorf("missing )")
			}
			return a, nil
		}
	case tEOF:
		return nil, fmt.Errorf("expression ends too soon")
	}
	return nil, fmt.Errorf("unexpected %q", t.text)
}

func unknownName(name string, names []string) error {
	known := append([]string{}, names...)
	known = append(known, "pi", "e", "i")
	return fmt.Errorf("unknown name %q (variables here: %s; functions: %s)", name, strings.Join(known, ", "), FunctionNames())
}

// fold evaluates constant subexpressions once.
func fold(n node) node {
	switch x := n.(type) {
	case binary:
		x.a, x.b = fold(x.a), fold(x.b)
		if a, ok := x.a.(num); ok {
			if b, ok := x.b.(num); ok {
				_, _ = a, b
				return num(x.eval(nil))
			}
		}
		return x
	case neg:
		x.a = fold(x.a)
		if a, ok := x.a.(num); ok {
			return num(negate(complex128(a)))
		}
		return x
	case call:
		all := true
		for i := range x.args {
			x.args[i] = fold(x.args[i])
			_, isNum := x.args[i].(num)
			all = all && isNum
		}
		if all {
			return num(x.eval(nil))
		}
		return x
	}
	return n
}

// Const evaluates an expression with no variables, such as a range end
// like 4pi or -1/2, to a real number.
func Const(src string) (float64, error) {
	e, err := Compile(src)
	if err != nil {
		return 0, err
	}
	v := e.Eval(nil)
	if imag(v) != 0 || math.IsNaN(real(v)) || math.IsInf(real(v), 0) {
		return 0, fmt.Errorf("%q is not a real number", src)
	}
	return real(v), nil
}

// Range reads "a..b" into its ends, each a constant expression.
func Range(s string) (lo, hi float64, err error) {
	a, b, ok := strings.Cut(s, "..")
	if !ok {
		return 0, 0, fmt.Errorf("a range is written from..to, e.g. 0..2pi")
	}
	if lo, err = Const(a); err != nil {
		return 0, 0, err
	}
	if hi, err = Const(b); err != nil {
		return 0, 0, err
	}
	if hi <= lo {
		return 0, 0, fmt.Errorf("range %s runs backwards or is empty", s)
	}
	return lo, hi, nil
}
