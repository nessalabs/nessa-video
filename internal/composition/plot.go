package composition

import (
	"fmt"
	"math"
	"strings"

	"github.com/nessalabs/nessa-video/internal/plot"
)

// PlotBlock is a plot of mathematical curves on x and y axes: functions of
// x, parametric and polar curves, and complex values of a parameter drawn
// as (Re, Im), with points, named parameters a `set` can ease, and the
// ranges the axes show.
type PlotBlock struct {
	X, Y     [2]float64 // the ranges the axes show
	XPi, YPi bool       // label the axis in multiples of π
	Square   bool       // one unit is as long on both axes
	Params   []PlotParam
	Curves   []PlotCurve
	Points   []PlotPoint
}

// PlotParam is a named number curves can use, with its starting value.
type PlotParam struct {
	Name  string
	Value float64
}

// PlotCurve is one curve of a plot, traced as its parameter runs From To.
type PlotCurve struct {
	Kind     string // "y" (a function of x), "curve" (parametric), "polar", "complex"
	Src      string // the equation as shown above the plot
	Var      string // the parameter: x, t, theta …
	From, To float64
	// y: A is y(x); curve: A is x(t), B is y(t); polar: A is r(theta);
	// complex: A is the value, drawn as (Re, Im). They read the parameter
	// first, then the plot's parameters in order.
	A, B *plot.Expr
	// Zeros are the parameter values where the curve passes through the
	// origin (or, for a function, crosses the x axis), for marks=zeros
	Zeros     []float64
	MarkZeros bool
}

// PlotPoint is a labelled dot at a place on the plot.
type PlotPoint struct {
	X, Y  *plot.Expr // read the plot's parameters in order
	Label string
}

// Param returns the index of the parameter called name, or -1.
func (p *PlotBlock) Param(name string) int {
	for i, pr := range p.Params {
		if pr.Name == name {
			return i
		}
	}
	return -1
}

// ParamNames lists the plot's parameters, in order.
func (p *PlotBlock) ParamNames() []string {
	var out []string
	for _, pr := range p.Params {
		out = append(out, pr.Name)
	}
	return out
}

// Defaults returns the starting values of the plot's parameters.
func (p *PlotBlock) Defaults() []float64 {
	out := make([]float64, len(p.Params))
	for i, pr := range p.Params {
		out[i] = pr.Value
	}
	return out
}

// At evaluates the curve at parameter t with the plot's parameters set to
// params, in plot coordinates; ok is false where the curve is undefined
// (a real curve taking a complex value, a pole).
func (c *PlotCurve) At(t float64, params []float64) (x, y float64, ok bool) {
	var buf [8]complex128
	vars := append(buf[:0], complex(t, 0))
	for _, v := range params {
		vars = append(vars, complex(v, 0))
	}
	isReal := func(z complex128) (float64, bool) {
		r, i := real(z), imag(z)
		return r, math.Abs(i) <= 1e-9*math.Max(1, math.Abs(r)) && !math.IsNaN(r) && !math.IsInf(r, 0)
	}
	switch c.Kind {
	case "y":
		v, ok := isReal(c.A.Eval(vars))
		return t, v, ok
	case "curve":
		a, ok1 := isReal(c.A.Eval(vars))
		b, ok2 := isReal(c.B.Eval(vars))
		return a, b, ok1 && ok2
	case "polar":
		r, ok := isReal(c.A.Eval(vars))
		return r * math.Cos(t), r * math.Sin(t), ok
	default:
		z := c.A.Eval(vars)
		x, y = real(z), imag(z)
		return x, y, !math.IsNaN(x) && !math.IsNaN(y) && !math.IsInf(x, 0) && !math.IsInf(y, 0)
	}
}

// Fraction turns a play step's to= into a share of the trace: "t=14.13"
// names a curve's parameter and a value on it, a bare number is a value of
// the first curve's parameter. ok is false when no curve has it.
func (p *PlotBlock) Fraction(v string) (float64, bool) {
	name, val, named := strings.Cut(v, "=")
	if !named {
		val, name = name, ""
	}
	f, err := plot.Const(strings.TrimSpace(val))
	if err != nil {
		return 0, false
	}
	for _, c := range p.Curves {
		if name == "" || c.Var == name || name == "θ" && c.Var == "theta" {
			return math.Max(0, math.Min(1, (f-c.From)/(c.To-c.From))), true
		}
	}
	return 0, false
}

// PlotPlace reads a place on a plot written (x, y), such as (0, 0) or
// (pi, -1/2), with constant coordinates.
func PlotPlace(s string) (x, y float64, err error) {
	s = strings.TrimSpace(s)
	bad := fmt.Errorf("write a place as (x, y), e.g. (0, 0)")
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return 0, 0, bad
	}
	a, b, ok := strings.Cut(s[1:len(s)-1], ",")
	if !ok {
		return 0, 0, bad
	}
	if x, err = plot.Const(strings.TrimSpace(a)); err != nil {
		return 0, 0, err
	}
	if y, err = plot.Const(strings.TrimSpace(b)); err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// PlotDecls returns the name=value pairs of a `set` on a plot: its
// declarations that name one of the plot's parameters.
func PlotDecls(ds []Decl, p *PlotBlock) []Decl {
	var out []Decl
	for _, d := range ds {
		if !TimingProp(d.Key) && p.Param(d.Key) >= 0 {
			out = append(out, d)
		}
	}
	return out
}
