// Package stage turns a styled, laid-out scene into a display list. It owns
// how each kind and state looks, reading every colour and size from the
// theme's tokens. It is the only place that knows both the composition model
// and the render model.
package stage

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/layout"
	"github.com/nessalabs/nessa-video/internal/motion"
	"github.com/nessalabs/nessa-video/internal/render"
	"github.com/nessalabs/nessa-video/internal/theme"
)

// Stage paints scenes for one video and theme.
type Stage struct {
	Theme   *theme.Theme
	Measure layout.Measurer
	W, H    int // frame size in pixels
}

// Design returns the design-space size and the scale to frame pixels.
// The short side of the frame is the theme's design short side (720 by
// default), so 1920x1080 and 1080x1920 share type and line sizes.
func (s *Stage) Design() (w, h, scale float64) {
	short := s.num("design-short-side", 720)
	scale = math.Min(float64(s.W), float64(s.H)) / short
	return float64(s.W) / scale, float64(s.H) / scale, scale
}

func (s *Stage) num(token string, def float64) float64 {
	if v, ok := s.Theme.Tokens[token]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func (s *Stage) col(token string) color.NRGBA {
	v, ok := s.Theme.Tokens[token]
	if !ok {
		v = token // allow raw #rrggbb values
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(strings.TrimPrefix(v, "#"), "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.NRGBA{0xEE, 0xEE, 0xEE, 255}
	}
	return color.NRGBA{r, g, b, 255}
}

func alpha(cl color.NRGBA, a float64) color.NRGBA {
	cl.A = uint8(math.Round(float64(cl.A) * math.Max(0, math.Min(1, a))))
	return cl
}

// Options returns layout options built from theme tokens.
func (s *Stage) Options() layout.Options {
	t := func(font, size string, def float64) layout.Type {
		return layout.Type{Font: font, Size: s.num(size, def)}
	}
	return layout.Options{
		Type: map[string]layout.Type{
			"title":   t("sans600", "size-title", 40),
			"p":       t("sans400", "size-p", 18),
			"label":   t("mono", "size-label", 13),
			"code":    t("mono", "size-code", 15),
			"metric":  t("sans500", "size-metric", 40),
			"box":     t("sans500", "size-box", 15),
			"caption": t("sans400", "size-caption", 20),
		},
		Gap:       s.num("gap", 24),
		MaxTextW:  640,
		BlockSize: [2]float64{480, 200},
	}
}

// Paint returns the display list for a scene's final state.
func (s *Stage) Paint(sc *c.Scene) *render.List {
	dw, dh, scale := s.Design()
	l := &render.List{W: s.W, H: s.H, BG: s.col("bg")}
	opt := s.Options()

	area := layout.Rect{X: 64, Y: 64, W: dw - 128, H: dh - 64 - 128}
	lay := layout.Scene(sc.Root, area, s.Measure, opt)
	end := motion.EndOf(sc)

	base := render.Affine{scale, 0, 0, scale, 0, 0}
	view := base
	if end.Camera != nil {
		if r, ok := lay.Boxes[end.Camera]; ok && r.W > 0 {
			z := math.Min(math.Min(area.W/(r.W+96), area.H/(r.H+96)), 2.5)
			z = math.Max(z, 1)
			view = base.Mul(render.Translate(area.CX(), area.CY())).
				Mul(render.Affine{z, 0, 0, z, 0, 0}).
				Mul(render.Translate(-r.CX(), -r.CY()))
		}
	}

	p := &painter{s: s, l: l, m: view, lay: lay, end: end, opt: opt}
	p.element(sc.Root, "rest")
	sc.Root.Walk(func(e *c.Element) {
		if e.Edge != nil && !end.Gone[e] {
			p.edge(sc, e)
		}
	})

	if end.Caption != "" {
		t := opt.Type["caption"]
		p.m = base
		p.runs(end.Caption, dw/2, dh-72, t, s.col("ink-2"), s.col("hi"), render.Middle)
	}
	return l
}

type painter struct {
	s   *Stage
	l   *render.List
	m   render.Affine
	lay layout.Result
	end motion.End
	opt layout.Options
}

func (p *painter) stroke(d string, cl color.NRGBA, dash bool) {
	op := render.Op{Kind: render.PathOp, M: p.m, D: d, Stroke: cl, StrokeW: p.s.num("stroke", 1.2), Draw: 1}
	if dash {
		op.Dash = []float64{4, 4}
	}
	p.l.Ops = append(p.l.Ops, op)
}

func (p *painter) fill(d string, cl color.NRGBA) {
	p.l.Ops = append(p.l.Ops, render.Op{Kind: render.PathOp, M: p.m, D: d, Fill: cl, Draw: 1})
}

func (p *painter) text(s string, x, y float64, t layout.Type, cl color.NRGBA, an render.Anchor) {
	p.l.Ops = append(p.l.Ops, render.Op{Kind: render.TextOp, M: p.m, Text: s, X: x, Y: y, Font: t.Font, Size: t.Size, Anchor: an, Color: cl})
}

// runs draws text with ==emphasis== spans in the bright colour.
func (p *painter) runs(s string, x, y float64, t layout.Type, base, hi color.NRGBA, an render.Anchor) {
	parts := strings.Split(s, "==")
	total := p.s.Measure.Measure(layout.Plain(s), t.Font, t.Size)
	switch an {
	case render.Middle:
		x -= total / 2
	case render.End:
		x -= total
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		cl := base
		if i%2 == 1 {
			cl = hi
		}
		under := strings.Contains(part, "__")
		part = strings.ReplaceAll(part, "__", "")
		p.text(part, x, y, t, cl, render.Start)
		w := p.s.Measure.Measure(part, t.Font, t.Size)
		if under {
			p.stroke(fmt.Sprintf("M%g %gH%g", x, y+4, x+w), hi, false)
		}
		x += w
	}
}

func rr(r layout.Rect, rad float64) string {
	rad = math.Min(rad, math.Min(r.W, r.H)/2)
	x, y, w, h := r.X, r.Y, r.W, r.H
	if rad <= 0 {
		return fmt.Sprintf("M%g %gH%gV%gH%gZ", x, y, x+w, y+h, x)
	}
	return fmt.Sprintf("M%g %gH%gA%g %g 0 0 1 %g %gV%gA%g %g 0 0 1 %g %gH%gA%g %g 0 0 1 %g %gV%gA%g %g 0 0 1 %g %gZ",
		x+rad, y, x+w-rad, rad, rad, x+w, y+rad, y+h-rad, rad, rad, x+w-rad, y+h, x+rad, rad, rad, x, y+h-rad, y+rad, rad, rad, x+rad, y)
}

func circle(cx, cy, r float64) string {
	return fmt.Sprintf("M%g %gA%g %g 0 1 0 %g %gA%g %g 0 1 0 %g %gZ", cx-r, cy, r, r, cx+r, cy, r, r, cx-r, cy)
}

// look is how a state colours lines and text.
type look struct {
	line, text color.NRGBA
	fill       color.NRGBA
	dash       bool
	brackets   bool
	dot        bool
}

func (p *painter) look(state string) look {
	s := p.s
	switch state {
	case "focus":
		return look{line: s.col("line-2"), text: s.col("hi"), fill: alpha(color.NRGBA{255, 255, 255, 255}, .04)}
	case "dim":
		return look{line: s.col("line-0"), text: s.col("ink-dim")}
	case "active":
		return look{line: s.col("line-1"), text: s.col("ink"), dot: true}
	case "select":
		return look{line: s.col("line-1"), text: s.col("ink"), brackets: true}
	case "trace":
		return look{line: s.col("line-2"), text: s.col("ink"), dash: true}
	}
	return look{line: s.col("line-1"), text: s.col("ink")}
}

// element paints e and its children. inherited carries dim and focus down
// so a dimmed component dims its parts.
func (p *painter) element(e *c.Element, inherited string) {
	if p.end.Gone[e] {
		return
	}
	state := p.end.StateOf(e)
	if state == "rest" && (inherited == "dim" || inherited == "focus") {
		state = inherited
	}
	lk := p.look(state)
	r, placed := p.lay.Boxes[e]
	kind := e.Kind()

	textColor := lk.text
	if v, ok := e.Computed["color"]; ok && state == "rest" {
		textColor = p.s.col(v.Value)
	}

	if placed {
		switch kind {
		case "box", "card", "stack", "decision", "store", "circle":
			p.shape(e, r, lk)
		case "title":
			t := layout.TypeOf(e, p.opt)
			for i, ln := range p.lay.Lines[e] {
				p.runs(ln, r.X, r.Y+t.Size*(1.0+1.35*float64(i)), t, p.s.col("title-ink"), p.s.col("hi"), render.Start)
			}
		case "p", "label", "metric", "code":
			t := layout.TypeOf(e, p.opt)
			for i, ln := range p.lay.Lines[e] {
				p.runs(ln, r.X, r.Y+t.Size*(1.0+1.35*float64(i)), t, textColor, p.s.col("hi"), render.Start)
			}
		case "terminal":
			p.terminal(e, r, lk)
		case "diff", "array", "chart", "timeline", "mermaid", "math":
			p.stroke(rr(r, 10), p.s.col("line-0"), true)
			t := p.opt.Type["label"]
			p.text(kind+" · drawn in a later milestone", r.CX(), r.CY()+4, t, p.s.col("ink-dim"), render.Middle)
		}
		if !c.IsBuiltinKind(kind) && kind != "scene" && (state == "focus" || state == "select" || state == "trace") {
			// a component instance with a state draws a frame around itself
			fr := layout.Rect{X: r.X - 12, Y: r.Y - 10, W: r.W + 24, H: r.H + 20}
			if lk.fill.A > 0 {
				p.fill(rr(fr, 8), lk.fill)
			}
			p.stroke(rr(fr, 8), lk.line, lk.dash)
		}
	}
	pass := inherited
	if state == "dim" || state == "focus" {
		pass = state
	}
	for _, ch := range e.Children {
		p.element(ch, pass)
	}
}

func (p *painter) shape(e *c.Element, r layout.Rect, lk look) {
	kind := e.Kind()
	radius := 6.0
	if v, ok := e.Computed["r"]; ok && kind != "circle" {
		if f, err := strconv.ParseFloat(v.Value, 64); err == nil {
			radius = f
		}
	}
	var d string
	switch kind {
	case "circle":
		d = circle(r.CX(), r.CY(), r.W/2)
		if r.W <= 10 { // a small dot reads as a status light
			p.fill(d, lk.line)
			if lk.text == p.s.col("hi") {
				p.fill(d, p.s.col("hi"))
			}
			return
		}
	case "decision":
		d = fmt.Sprintf("M%g %gL%g %gL%g %gL%g %gZ", r.CX(), r.Y, r.Right(), r.CY(), r.CX(), r.Bottom(), r.X, r.CY())
	case "store":
		ry := math.Min(10, r.H/6)
		d = fmt.Sprintf("M%g %gV%gA%g %g 0 0 0 %g %gV%g", r.X, r.Y+ry, r.Bottom()-ry, r.W/2, ry, r.Right(), r.Bottom()-ry, r.Y+ry)
		p.stroke(fmt.Sprintf("M%g %gA%g %g 0 1 0 %g %gA%g %g 0 1 0 %g %gZ", r.X, r.Y+ry, r.W/2, ry, r.Right(), r.Y+ry, r.W/2, ry, r.X, r.Y+ry), lk.line, lk.dash)
	case "stack":
		for i := 2; i >= 1; i-- {
			off := float64(i) * 6
			p.stroke(rr(layout.Rect{X: r.X + off, Y: r.Y, W: r.W - 12, H: r.H - 12}, radius), p.s.col("line-0"), false)
		}
		d = rr(layout.Rect{X: r.X, Y: r.Y + 12, W: r.W - 12, H: r.H - 12}, radius)
	case "card":
		d = rr(r, 10)
		p.stroke(fmt.Sprintf("M%g %gH%g", r.X, r.Y+28, r.Right()), p.s.col("line-0"), false)
	default:
		d = rr(r, radius)
	}
	if lk.fill.A > 0 {
		p.fill(d, lk.fill)
	}
	p.stroke(d, lk.line, lk.dash)
	if lk.brackets {
		p.stroke(fmt.Sprintf("M%g %gv-8h8M%g %gh8v8M%g %gv8h-8M%g %gh-8v-8",
			r.X-6, r.Y+2, r.Right()-2, r.Y-6, r.Right()+6, r.Bottom()-2, r.X+2, r.Bottom()+6), p.s.col("line-2"), false)
	}
	if lk.dot {
		p.fill(circle(r.X+12, r.Y+12, 3), p.s.col("hi"))
	}
	if e.Text != "" {
		t := layout.TypeOf(e, p.opt)
		if kind == "card" {
			p.runs(e.Text, r.X+12, r.Y+19, layout.Type{Font: "sans500", Size: 13}, lk.text, p.s.col("hi"), render.Start)
		} else {
			p.runs(e.Text, r.CX(), r.CY()+t.Size*0.36, t, lk.text, p.s.col("hi"), render.Middle)
		}
	}
}

func (p *painter) terminal(e *c.Element, r layout.Rect, lk look) {
	p.fill(rr(r, 12), p.s.col("panel"))
	p.stroke(rr(r, 12), lk.line, lk.dash)
	p.stroke(fmt.Sprintf("M%g %gH%g", r.X, r.Y+44, r.Right()), p.s.col("line-0"), false)
	p.stroke(fmt.Sprintf("M%g %gl7 6-7 6M%g %gh10", r.X+22, r.Y+16, r.X+36, r.Y+28), p.s.col("line-1"), false)
	lab := p.opt.Type["label"]
	p.text("terminal", r.Right()-20, r.Y+27, lab, p.s.col("ink-3"), render.End)
	t := p.opt.Type["code"]
	for i, ln := range e.Body {
		y := r.Y + 44 + 30 + 30*float64(i)
		x := r.X + 24
		if strings.HasPrefix(ln, "$ ") || ln == "$" {
			p.text("$", x, y, t, p.s.col("ink-3"), render.Start)
			x += p.s.Measure.Measure("$ ", t.Font, t.Size)
			ln = strings.TrimPrefix(strings.TrimPrefix(ln, "$"), " ")
			p.runs(ln, x, y, t, p.s.col("ink"), p.s.col("hi"), render.Start)
			continue
		}
		p.runs(ln, x, y, t, p.s.col("ink-2"), p.s.col("hi"), render.Start)
	}
}

// edge routes a connector with straight or right-angled segments only.
func (p *painter) edge(sc *c.Scene, e *c.Element) {
	scope := sc.Root
	if o := e.OwnerElement(); o != nil {
		scope = o
	}
	if e.Edge.From == nil || e.Edge.To == nil {
		return
	}
	fromEls, toEls := c.Select(scope, e.Edge.From), c.Select(scope, e.Edge.To)
	if len(fromEls) == 0 || len(toEls) == 0 {
		return
	}
	a, okA := p.lay.Boxes[fromEls[0]]
	b, okB := p.lay.Boxes[toEls[0]]
	if !okA || !okB || p.end.Gone[fromEls[0]] || p.end.Gone[toEls[0]] {
		return
	}
	state := p.end.StateOf(e)
	lk := p.look(state)
	cl := lk.line
	dashed := e.Edge.Op == "..>" || e.Edge.Op == ".." || lk.dash
	head := e.Edge.Op != "--" && e.Edge.Op != ".."
	const gap = 8.0

	var pts [][2]float64
	top, bot := math.Max(a.Y, b.Y), math.Min(a.Bottom(), b.Bottom())
	left, right := math.Max(a.X, b.X), math.Min(a.Right(), b.Right())
	switch {
	case top < bot: // rows overlap: one horizontal line
		y := (top + bot) / 2
		if a.CX() < b.CX() {
			pts = [][2]float64{{a.Right() + gap, y}, {b.X - gap, y}}
		} else {
			pts = [][2]float64{{a.X - gap, y}, {b.Right() + gap, y}}
		}
	case left < right: // columns overlap: one vertical line
		x := (left + right) / 2
		if a.CY() < b.CY() {
			pts = [][2]float64{{x, a.Bottom() + gap}, {x, b.Y - gap}}
		} else {
			pts = [][2]float64{{x, a.Y - gap}, {x, b.Bottom() + gap}}
		}
	default: // elbow: out sideways, across, in sideways
		var x1, x2 float64
		if a.CX() < b.CX() {
			x1, x2 = a.Right()+gap, b.X-gap
		} else {
			x1, x2 = a.X-gap, b.Right()+gap
		}
		mx := (x1 + x2) / 2
		pts = [][2]float64{{x1, a.CY()}, {mx, a.CY()}, {mx, b.CY()}, {x2, b.CY()}}
	}
	var d strings.Builder
	for i, pt := range pts {
		if i == 0 {
			fmt.Fprintf(&d, "M%g %g", pt[0], pt[1])
		} else {
			fmt.Fprintf(&d, "L%g %g", pt[0], pt[1])
		}
	}
	p.stroke(d.String(), cl, dashed)
	if head {
		p.chevron(pts[len(pts)-2], pts[len(pts)-1], cl)
		if e.Edge.Op == "<->" {
			p.chevron(pts[1], pts[0], cl)
		}
	}
	if e.Text != "" {
		// label above the longest segment
		// label the longest horizontal segment, or the longest one if none is horizontal
		best, bl := 0, -1.0
		for pass := 0; pass < 2 && bl < 0; pass++ {
			for i := 0; i+1 < len(pts); i++ {
				horiz := pts[i][1] == pts[i+1][1]
				if pass == 0 && !horiz {
					continue
				}
				if l := math.Hypot(pts[i+1][0]-pts[i][0], pts[i+1][1]-pts[i][1]); l > bl {
					best, bl = i, l
				}
			}
		}
		mx, my := (pts[best][0]+pts[best+1][0])/2, (pts[best][1]+pts[best+1][1])/2
		t := p.opt.Type["label"]
		if pts[best][1] == pts[best+1][1] {
			p.runs(e.Text, mx, my-10, t, p.s.col("ink"), p.s.col("hi"), render.Middle)
		} else {
			p.runs(e.Text, mx+10, my+4, t, p.s.col("ink"), p.s.col("hi"), render.Start)
		}
	}
}

func (p *painter) chevron(from, to [2]float64, cl color.NRGBA) {
	dx, dy := to[0]-from[0], to[1]-from[1]
	n := math.Hypot(dx, dy)
	if n == 0 {
		return
	}
	ux, uy := dx/n, dy/n
	px, py := -uy, ux
	const L, W = 7.0, 4.0
	p.stroke(fmt.Sprintf("M%g %gL%g %gL%g %g",
		to[0]-ux*L+px*W, to[1]-uy*L+py*W, to[0], to[1], to[0]-ux*L-px*W, to[1]-uy*L-py*W), cl, false)
}
