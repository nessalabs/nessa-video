package resolve

import (
	"fmt"
	"math"
	"math/cmplx"
	"regexp"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/diag"
	"github.com/nessalabs/nessa-video/internal/plot"
	"github.com/nessalabs/nessa-video/internal/script"
)

// Plots: the line takes the ranges and parameters, each indented line is
// a curve or a point.
//
//	plot #p x=0..4pi y=-1..1 square=true a=0
//	  y = sin(x) * exp(-x/4)
//	  curve x=sin(3t) y=sin(2t) t=0..2pi
//	  polar r = 1 + cos(theta) theta=0..2pi
//	  zeta(1/2 + i*t) t=0..30 marks=zeros
//	  point (1, 0.8) "peak"

var (
	plotFn    = regexp.MustCompile(`^y\s*=\s*(.*)$`)
	plotPolar = regexp.MustCompile(`^polar\s+(?:r\s*=\s*)?(.*)$`)
	plotCurve = regexp.MustCompile(`^curve\s+(.*)$`)
	plotPoint = regexp.MustCompile(`^point\s+(.*)$`)
	plotOpt   = regexp.MustCompile(`^([A-Za-z_θ][\w]*)=(.*)$`)
)

// reserved names a plot parameter cannot take: curve parameters and
// constants
var plotReserved = map[string]bool{"x": true, "y": true, "t": true, "theta": true, "θ": true, "r": true, "pi": true, "e": true, "i": true, "square": true}

func (r *resolver) plotBlock(e *c.Element, n *script.Node) {
	pb := &c.PlotBlock{X: [2]float64{-1, 1}, Y: [2]float64{-1, 1}}
	for _, p := range n.Props {
		switch {
		case p.Key == "x" || p.Key == "y":
			lo, hi, err := plot.Range(p.Value)
			if err != nil {
				r.diags.Errorf(p.Pos, "e.g. x=0..4pi or y=-1..1", "bad %s range %q: %v", p.Key, p.Value, err)
				continue
			}
			if p.Key == "x" {
				pb.X, pb.XPi = [2]float64{lo, hi}, strings.Contains(p.Value, "pi") || strings.Contains(p.Value, "π")
			} else {
				pb.Y, pb.YPi = [2]float64{lo, hi}, strings.Contains(p.Value, "pi") || strings.Contains(p.Value, "π")
			}
		case p.Key == "square":
			r.oneOf(p.Value, p.Pos, []string{"true", "false"}, "square value")
			pb.Square = p.Value == "true"
		case c.IsProp(p.Key):
		case plotReserved[p.Key] || plot.IsFunction(p.Key):
			r.diags.Errorf(p.Pos, "call it something else, e.g. a or k", "%q cannot name a plot parameter", p.Key)
		default:
			v, err := plot.Const(p.Value)
			if err != nil {
				r.diags.Errorf(p.Pos, fmt.Sprintf("e.g. %s=0", p.Key), "bad value for parameter %s: %v", p.Key, err)
			}
			if i := pb.Param(p.Key); i >= 0 {
				pb.Params[i].Value = v
			} else {
				pb.Params = append(pb.Params, c.PlotParam{Name: p.Key, Value: v})
			}
		}
	}
	params := pb.ParamNames()
	compile := func(src, v string, pos diag.Pos) *plot.Expr {
		names := append([]string{v}, params...)
		if v == "theta" {
			names[0] = "theta"
		}
		ex, err := plot.Compile(src, names...)
		if err != nil {
			r.diags.Errorf(pos, "e.g. sin(x) * exp(-x/4), 1 + cos(theta), zeta(1/2 + i*t)", "cannot read %q: %v", src, err)
			return nil
		}
		return ex
	}
	for i, ln := range n.Raw {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "# ") {
			continue
		}
		pos := diag.Pos{Line: n.Pos.Line + 1 + i, Col: n.Pos.Col + 2}
		head, rest := "complex", t
		if m := plotFn.FindStringSubmatch(t); m != nil {
			head, rest = "y", m[1]
		} else if m := plotPolar.FindStringSubmatch(t); m != nil {
			head, rest = "polar", m[1]
		} else if m := plotCurve.FindStringSubmatch(t); m != nil {
			head, rest = "curve", m[1]
		} else if m := plotPoint.FindStringSubmatch(t); m != nil {
			head, rest = "point", m[1]
		}
		var exprParts []string
		opts := map[string]string{}
		var optOrder []string
		label := ""
		for _, f := range plotFields(rest) {
			if strings.HasPrefix(f, `"`) {
				label = strings.Trim(f, `"`)
			} else if m := plotOpt.FindStringSubmatch(f); m != nil && head != "point" {
				opts[m[1]] = strings.Trim(m[2], `"`)
				optOrder = append(optOrder, m[1])
			} else {
				exprParts = append(exprParts, f)
			}
		}
		src := strings.Join(exprParts, " ")
		if head == "point" {
			x, y, ok := splitPlace(src)
			if !ok {
				r.diags.Errorf(pos, `e.g. point (1, 0.8) "peak"`, "a point is written (x, y)")
				continue
			}
			pt := c.PlotPoint{X: compile(x, "_", pos), Y: compile(y, "_", pos), Label: label}
			if pt.X != nil && pt.Y != nil {
				pb.Points = append(pb.Points, pt)
			}
			continue
		}
		cv := c.PlotCurve{Kind: head}
		// the parameter: its range is the option whose value is a..b
		rangeOf := func(def string) (string, string) {
			for _, k := range optOrder {
				if strings.Contains(opts[k], "..") && !(head == "curve" && (k == "x" || k == "y")) {
					return k, opts[k]
				}
			}
			return def, ""
		}
		var rng string
		switch head {
		case "y":
			cv.Var, rng = "x", opts["x"]
			cv.From, cv.To = pb.X[0], pb.X[1]
			cv.A = compile(src, "x", pos)
			cv.Src = "y = " + src
		case "polar":
			cv.Var, rng = rangeOf("theta")
			if cv.Var == "θ" {
				cv.Var = "theta"
			}
			cv.From, cv.To = 0, 2*math.Pi
			src = strings.ReplaceAll(src, "θ", "theta")
			cv.A = compile(src, "theta", pos)
			cv.Src = "r = " + src
		case "curve":
			cv.Var, rng = rangeOf("t")
			xs, ys := opts["x"], opts["y"]
			if xs == "" || ys == "" {
				r.diags.Errorf(pos, "e.g. curve x=sin(3t) y=sin(2t) t=0..2pi", "a curve needs x= and y=")
				continue
			}
			cv.A, cv.B = compile(xs, cv.Var, pos), compile(ys, cv.Var, pos)
			cv.Src = fmt.Sprintf("x = %s,  y = %s", xs, ys)
		default:
			cv.Var, rng = rangeOf("t")
			cv.A = compile(src, cv.Var, pos)
			cv.Src = src
		}
		if cv.A == nil || head == "curve" && cv.B == nil {
			continue
		}
		if head != "y" && rng == "" {
			if head != "polar" {
				r.diags.Errorf(pos, fmt.Sprintf("e.g. %s=0..2pi", cv.Var), "give the range of %s", cv.Var)
				continue
			}
		}
		if rng != "" {
			lo, hi, err := plot.Range(rng)
			if err != nil {
				r.diags.Errorf(pos, fmt.Sprintf("e.g. %s=0..2pi", cv.Var), "bad range for %s: %v", cv.Var, err)
				continue
			}
			cv.From, cv.To = lo, hi
		}
		for k, v := range opts {
			switch {
			case k == "marks":
				if r.oneOf(v, pos, []string{"zeros"}, "marks value") {
					cv.MarkZeros = true
				}
			case k == cv.Var || head == "curve" && (k == "x" || k == "y"):
			default:
				r.diags.Errorf(pos, "a curve takes its parameter's range and marks=zeros", "unknown curve option %q", k)
			}
		}
		if cv.MarkZeros {
			cv.Zeros = curveZeros(&cv, pb.Defaults())
		}
		pb.Curves = append(pb.Curves, cv)
	}
	if len(pb.Curves) == 0 && len(pb.Points) == 0 {
		r.diags.Errorf(n.Pos, "indent curves under the line, e.g. y = sin(x)", "plot has nothing to draw")
	}
	e.Plot = pb
}

// curveZeros finds where a curve touches zero: a function's roots, else
// where the curve passes through the origin.
func curveZeros(cv *c.PlotCurve, params []float64) []float64 {
	if cv.Kind == "y" {
		return plot.Roots(func(t float64) float64 {
			_, y, ok := cv.At(t, params)
			if !ok {
				return math.NaN()
			}
			return y
		}, cv.From, cv.To)
	}
	return plot.Zeros(func(t float64) float64 {
		x, y, ok := cv.At(t, params)
		if !ok {
			return math.Inf(1)
		}
		return cmplx.Abs(complex(x, y))
	}, cv.From, cv.To)
}

// plotFields splits a curve line at spaces outside quotes and brackets.
func plotFields(s string) []string {
	var out []string
	depth, quoted, start := 0, false, -1
	for i, ch := range s {
		switch {
		case ch == '"':
			quoted = !quoted
		case quoted:
		case ch == '(':
			depth++
		case ch == ')':
			depth--
		}
		if ch == ' ' && depth == 0 && !quoted {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

// splitPlace splits "(x, y)" into its two expressions.
func splitPlace(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	inner := s[1 : len(s)-1]
	depth := 0
	for i, ch := range inner {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return strings.TrimSpace(inner[:i]), strings.TrimSpace(inner[i+1:]), true
			}
		}
	}
	return "", "", false
}

// checkPlots checks the steps that act on plots: set NAME=VALUE, play
// to=, note and zoom.
func (r *resolver) checkPlots(s *c.Scene) {
	for _, st := range s.Steps {
		if st.Target == nil {
			continue
		}
		targets := c.Select(s.Root, st.Target)
		plots := func(what string) []*c.Element {
			var out []*c.Element
			for _, e := range targets {
				if e.Plot == nil {
					r.diags.Errorf(st.Pos, what, "%s is a %s, not a plot", st.Target.Src, e.Kind())
					continue
				}
				out = append(out, e)
			}
			return out
		}
		switch st.Verb {
		case "set":
			for _, e := range targets {
				if e.Plot == nil {
					continue
				}
				for _, d := range st.Decls {
					if c.TimingProp(d.Key) {
						continue
					}
					if e.Plot.Param(d.Key) < 0 {
						r.diags.Errorf(d.Pos, "parameters: "+strings.Join(e.Plot.ParamNames(), ", ")+" (declare one on the plot line, e.g. a=0)", "plot %s has no parameter %q", st.Target.Src, d.Key)
					} else if _, err := plot.Const(d.Value); err != nil {
						r.diags.Errorf(d.Pos, fmt.Sprintf("e.g. %s=1", d.Key), "bad value %q: %v", d.Value, err)
					}
				}
			}
		case "play":
			v, ok := st.Lookup("to")
			if !ok {
				continue
			}
			for _, e := range targets {
				if e.Plot != nil {
					if _, ok := e.Plot.Fraction(v); !ok {
						r.diags.Errorf(st.Pos, "e.g. play #p to=t=14.13 or to=x=2, naming a curve's parameter", "plot %s has no curve with %q", st.Target.Src, v)
					}
				}
			}
		case "note":
			for range plots("note pins a label to a place on a plot") {
				if st.Text == "" {
					r.diags.Errorf(st.Pos, `e.g. note #p (0,0) "first zero"`, "note needs quoted text")
				}
				if _, _, err := c.PlotPlace(notePlace(st)); err != nil {
					r.diags.Errorf(st.Pos, `e.g. note #p (0,0) "first zero" or note #p at=(0,0) "first zero"`, "%v", err)
				}
			}
		case "zoom":
			for range plots("zoom rescales a plot's axes; camera zoom #x moves the view") {
				for _, k := range []string{"x", "y"} {
					if v, ok := st.Lookup(k); ok {
						if _, _, err := plot.Range(v); err != nil {
							r.diags.Errorf(st.Pos, "e.g. zoom #p x=-.5..0.5 y=-.5..0.5, or zoom #p to go back", "bad %s range %q: %v", k, v, err)
						}
					}
				}
			}
		}
	}
}

// notePlace is where a note step pins its label: at=(x, y), or a bare
// (x, y) after the target.
func notePlace(st c.Step) string {
	if v, ok := st.Lookup("at"); ok && strings.HasPrefix(v, "(") {
		return v
	}
	return strings.Join(st.Args, " ")
}
