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
	"sync"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/layout"
	"github.com/nessalabs/nessa-video/internal/motion"
	"github.com/nessalabs/nessa-video/internal/render"
	"github.com/nessalabs/nessa-video/internal/source"
	"github.com/nessalabs/nessa-video/internal/source/syntax"
)

// Stage paints scenes for one video and theme.
type Stage struct {
	Tokens  map[string]string // the video's theme tokens
	Measure layout.Measurer
	W, H    int // frame size in pixels

	tokens sync.Map // *composition.CodeBlock -> [][]syntax.Token
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
	if v, ok := s.Tokens[token]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func (s *Stage) col(token string) color.NRGBA {
	v, ok := s.Tokens[token]
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
			"math":    t("sans400", "size-math", 30),
			"box":     t("sans500", "size-box", 15),
			"caption": t("sans400", "size-caption", 20),
		},
		Gap:       s.num("gap", 24),
		MaxTextW:  640,
		BlockSize: [2]float64{480, 200},
	}
}

// Area returns the design-space rectangle scenes are laid out in.
func (s *Stage) Area() layout.Rect {
	dw, dh, _ := s.Design()
	return layout.Rect{X: 64, Y: 64, W: dw - 128, H: dh - 64 - 128}
}

// Layout places a scene's elements. Positions do not change while a scene
// plays, so callers lay out once and paint many moments.
func (s *Stage) Layout(sc *c.Scene) layout.Result {
	return layout.Scene(sc.Root, s.Area(), s.Measure, s.Options())
}

// view returns the camera transform that frames target (nil for the stage).
func (s *Stage) view(lay layout.Result, target *c.Element) (z, cx, cy float64) {
	area := s.Area()
	z, cx, cy = lay.Fit, area.CX(), area.CY()
	if z == 0 {
		z = 1
	}
	if target == nil {
		return
	}
	r, ok := lay.Boxes[target]
	if !ok || r.W == 0 {
		return
	}
	z = math.Max(1, math.Min(math.Min(area.W/(r.W+96), area.H/(r.H+96)), 2.5))
	return z, r.CX(), r.CY()
}

// Frame returns the display list for one moment of a scene.
func (s *Stage) Frame(sc *c.Scene, lay layout.Result, m motion.Moment) *render.List {
	dw, dh, scale := s.Design()
	l := &render.List{W: s.W, H: s.H, BG: s.col("bg")}
	opt := s.Options()
	area := s.Area()

	base := render.Affine{scale, 0, 0, scale, 0, 0}
	from, to, mix := m.Camera()
	z0, x0, y0 := s.view(lay, from)
	z1, x1, y1 := s.view(lay, to)
	lerp := func(a, b float64) float64 { return a + (b-a)*mix }
	z, cx, cy := lerp(z0, z1), lerp(x0, x1), lerp(y0, y1)
	view := base.Mul(render.Translate(area.CX(), area.CY())).
		Mul(render.Affine{z, 0, 0, z, 0, 0}).
		Mul(render.Translate(-cx, -cy))

	p := &painter{s: s, l: l, view: view, m: view, lay: lay, mo: m, opt: opt}
	p.element(sc.Root, "rest", 1, 0)
	sc.Root.Walk(func(e *c.Element) {
		if e.Edge != nil {
			p.edge(sc, e)
		}
	})
	for _, pk := range m.Packets() {
		p.packet(sc, pk)
	}

	if text, op := m.Caption(); text != "" && op > 0 {
		t := opt.Type["caption"]
		p.m = base
		p.opacity = op
		p.draw = 1
		if words, elapsed := m.Spoken(); words != nil {
			p.followWords(text, words, elapsed, dw/2, dh-72, t)
		} else {
			p.runs(text, dw/2, dh-72, t, s.col("ink-2"), s.col("hi"), render.Middle)
		}
	}
	return l
}

// Still paints a scene as it looks once all of its steps have run.
func (s *Stage) Still(sp *motion.ScenePlan) *render.List {
	return s.Frame(sp.Scene, s.Layout(sp.Scene), sp.End())
}

type painter struct {
	s       *Stage
	l       *render.List
	view    render.Affine // camera transform
	m       render.Affine // camera plus the current element's rise
	lay     layout.Result
	mo      motion.Moment
	opt     layout.Options
	opacity float64 // multiplies every colour drawn
	draw    float64 // stroke progress for the element being drawn
}

func (p *painter) stroke(d string, cl color.NRGBA, dash bool) {
	cl = alpha(cl, p.opacity)
	if cl.A == 0 || p.draw <= 0 {
		return
	}
	op := render.Op{Kind: render.PathOp, M: p.m, D: d, Stroke: cl, StrokeW: p.s.num("stroke", 1.2), Draw: p.draw}
	if dash {
		op.Dash = []float64{4, 4}
	}
	p.l.Ops = append(p.l.Ops, op)
}

func (p *painter) fill(d string, cl color.NRGBA) {
	cl = alpha(cl, p.opacity)
	if cl.A == 0 {
		return
	}
	p.l.Ops = append(p.l.Ops, render.Op{Kind: render.PathOp, M: p.m, D: d, Fill: cl, Draw: 1})
}

func (p *painter) text(s string, x, y float64, t layout.Type, cl color.NRGBA, an render.Anchor) {
	cl = alpha(cl, p.opacity)
	if cl.A == 0 || s == "" {
		return
	}
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
			d := p.draw
			p.draw = 1
			p.stroke(fmt.Sprintf("M%g %gH%g", x, y+4, x+w), hi, false)
			p.draw = d
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

func lerpColor(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), f(a.A, b.A)}
}

func (p *painter) blend(from, to string, mix float64) look {
	a, b := p.look(from), p.look(to)
	if mix >= 1 {
		return b
	}
	out := look{line: lerpColor(a.line, b.line, mix), text: lerpColor(a.text, b.text, mix), fill: lerpColor(a.fill, b.fill, mix)}
	if mix >= .5 {
		out.dash, out.brackets, out.dot = b.dash, b.brackets, b.dot
	} else {
		out.dash, out.brackets, out.dot = a.dash, a.brackets, a.dot
	}
	return out
}

// element paints e and its children. inherited carries dim and focus down
// so a dimmed component dims its parts; opacity and rise multiply and add
// down the tree, so a component fading in fades its parts too.
func (p *painter) element(e *c.Element, inherited string, opacity, rise float64) {
	if v, ok := e.Computed["hidden"]; ok && v.Value == "true" {
		return
	}
	ap := p.mo.Look(e)
	opacity *= ap.Opacity
	if opacity <= 0.002 {
		return
	}
	rise += ap.Rise
	eff := func(s string) string {
		if s == "rest" && (inherited == "dim" || inherited == "focus") {
			return inherited
		}
		return s
	}
	lk := p.blend(eff(ap.From), eff(ap.State), ap.Mix)
	state := eff(ap.State)
	r, placed := p.lay.Boxes[e]
	kind := e.Kind()

	p.m = p.view.Mul(render.Translate(0, -rise))
	p.opacity = opacity
	p.draw = ap.Draw

	textColor := lk.text
	if v, ok := e.Computed["color"]; ok && state == "rest" {
		textColor = p.s.col(v.Value)
	}

	// dim fades a block's contents too, not only its outline
	if e.Code != nil || e.Array != nil || e.Chart != nil || e.Timeline != nil || e.Sequence != nil || kind == "math" {
		dimness := 0.0
		if eff(ap.State) == "dim" {
			dimness += ap.Mix
		}
		if eff(ap.From) == "dim" {
			dimness += 1 - ap.Mix
		}
		p.opacity *= 1 - 0.55*math.Min(1, dimness)
	}
	switch {
	case !placed || e.Edge != nil:
	case e.Code != nil:
		p.codePanel(e, r, lk, ap)
	case e.Array != nil:
		p.arrayBlock(e, r, lk, ap)
	case e.Chart != nil:
		p.chartBlock(e, r, lk, ap)
	case e.Timeline != nil:
		p.timelineBlock(e, r, lk, ap)
	case e.Sequence != nil:
		p.sequenceBlock(e, r, lk, ap)
	case kind == "math":
		p.mathBlock(e, r, textColor, ap)
	}
	if placed && e.Edge == nil && e.Code == nil && e.Array == nil && e.Chart == nil && e.Timeline == nil && e.Sequence == nil && kind != "math" {
		switch kind {
		case "box", "card", "stack", "decision", "store", "circle":
			p.shape(e, r, lk, ap)
		case "metric":
			p.metric(e, r, lk, ap, state)
		case "title", "p", "label", "code":
			base, hi := textColor, p.s.col("hi")
			if kind == "title" {
				base = p.s.col("title-ink")
			}
			t := layout.TypeOf(e, p.opt)
			p.draw = 1
			p.opacity = opacity * ap.FillIn
			lines := reveal(p.lay.Lines[e], ap.Reveal)
			for i, ln := range lines {
				p.runs(ln, r.X, r.Y+t.Size*(1.0+1.35*float64(i)), t, base, hi, render.Start)
			}
		case "terminal":
			p.terminal(e, r, lk, ap)
		case "diff", "array", "chart", "timeline":
			// a block whose data could not be read; reel check reports why
			p.stroke(rr(r, 10), p.s.col("line-0"), true)
		}
		if !c.IsBuiltinKind(kind) && kind != "scene" && (state == "focus" || state == "select" || state == "trace") {
			// a component instance with a state draws a frame around itself
			fr := layout.Rect{X: r.X - 12, Y: r.Y - 10, W: r.W + 24, H: r.H + 20}
			p.draw = 1
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
		if ch.Edge == nil {
			p.element(ch, pass, opacity, rise)
		}
	}
}

// reveal returns the lines with only the first share of visible characters
// shown. Inline marks (== and __) take no time and are never cut in half,
// and a mark left open is closed so the shown part keeps its emphasis.
func reveal(lines []string, share float64) []string {
	if share >= 1 {
		return lines
	}
	total := 0
	for _, l := range lines {
		total += len([]rune(layout.Plain(l)))
	}
	left := int(math.Round(share * float64(total)))
	var out []string
	for _, l := range lines {
		if left <= 0 {
			break
		}
		var b strings.Builder
		open := map[string]bool{}
		r := []rune(l)
		for i := 0; i < len(r); i++ {
			if i+1 < len(r) && (r[i] == '=' && r[i+1] == '=' || r[i] == '_' && r[i+1] == '_') {
				m := string(r[i : i+2])
				open[m] = !open[m]
				b.WriteString(m)
				i++
				continue
			}
			if left <= 0 {
				break
			}
			b.WriteRune(r[i])
			left--
		}
		for _, m := range []string{"__", "=="} {
			if open[m] {
				b.WriteString(m)
			}
		}
		out = append(out, b.String())
	}
	return out
}

func (p *painter) shape(e *c.Element, r layout.Rect, lk look, ap motion.Appearance) {
	opacity := p.opacity
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
			p.opacity = opacity * ap.FillIn
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
		p.opacity = opacity * ap.FillIn
		p.fill(d, lk.fill)
		p.opacity = opacity
	}
	p.stroke(d, lk.line, lk.dash)
	p.opacity = opacity * ap.FillIn
	p.draw = 1
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

func (p *painter) terminal(e *c.Element, r layout.Rect, lk look, ap motion.Appearance) {
	p.draw = 1
	p.fill(rr(r, 12), p.s.col("panel"))
	p.stroke(rr(r, 12), lk.line, lk.dash)
	p.stroke(fmt.Sprintf("M%g %gH%g", r.X, r.Y+44, r.Right()), p.s.col("line-0"), false)
	p.stroke(fmt.Sprintf("M%g %gl7 6-7 6M%g %gh10", r.X+22, r.Y+16, r.X+36, r.Y+28), p.s.col("line-1"), false)
	lab := p.opt.Type["label"]
	p.text("terminal", r.Right()-20, r.Y+27, lab, p.s.col("ink-3"), render.End)
	t := p.opt.Type["code"]
	body := reveal(e.Body, ap.Reveal)
	for i, ln := range body {
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

// route returns a connector's points: straight when its ends line up,
// otherwise out sideways, across and in. Nil if an end is missing.
func (p *painter) route(sc *c.Scene, e *c.Element) [][2]float64 {
	scope := sc.Root
	if o := e.OwnerElement(); o != nil {
		scope = o
	}
	if e.Edge.From == nil || e.Edge.To == nil {
		return nil
	}
	fromEls, toEls := c.Select(scope, e.Edge.From), c.Select(scope, e.Edge.To)
	if len(fromEls) == 0 || len(toEls) == 0 {
		return nil
	}
	a, okA := p.lay.Boxes[fromEls[0]]
	b, okB := p.lay.Boxes[toEls[0]]
	if !okA || !okB {
		return nil
	}
	const gap = 8.0
	if pts := p.detour(fromEls[0], toEls[0], a, b); pts != nil {
		return pts
	}
	top, bot := math.Max(a.Y, b.Y), math.Min(a.Bottom(), b.Bottom())
	left, right := math.Max(a.X, b.X), math.Min(a.Right(), b.Right())
	switch {
	case top < bot:
		y := (top + bot) / 2
		if a.CX() < b.CX() {
			return [][2]float64{{a.Right() + gap, y}, {b.X - gap, y}}
		}
		return [][2]float64{{a.X - gap, y}, {b.Right() + gap, y}}
	case left < right:
		x := (left + right) / 2
		if a.CY() < b.CY() {
			return [][2]float64{{x, a.Bottom() + gap}, {x, b.Y - gap}}
		}
		return [][2]float64{{x, a.Y - gap}, {x, b.Bottom() + gap}}
	default:
		var x1, x2 float64
		if a.CX() < b.CX() {
			x1, x2 = a.Right()+gap, b.X-gap
		} else {
			x1, x2 = a.X-gap, b.Right()+gap
		}
		mx := (x1 + x2) / 2
		return [][2]float64{{x1, a.CY()}, {mx, a.CY()}, {mx, b.CY()}, {x2, b.CY()}}
	}
}

// edge draws a connector.
func (p *painter) edge(sc *c.Scene, e *c.Element) {
	pts := p.route(sc, e)
	if pts == nil {
		return
	}
	ap := p.mo.Look(e)
	if ap.Opacity <= 0.002 {
		return
	}
	lk := p.blend(ap.From, ap.State, ap.Mix)
	cl := lk.line
	dashed := e.Edge.Op == "..>" || e.Edge.Op == ".." || lk.dash
	head := e.Edge.Op != "--" && e.Edge.Op != ".."
	p.m = p.view
	p.opacity = ap.Opacity
	p.draw = ap.Draw

	var d strings.Builder
	for i, pt := range pts {
		if i == 0 {
			fmt.Fprintf(&d, "M%g %g", pt[0], pt[1])
		} else {
			fmt.Fprintf(&d, "L%g %g", pt[0], pt[1])
		}
	}
	p.stroke(d.String(), cl, dashed)
	p.draw = 1
	if head && ap.Draw >= 0.98 {
		p.chevron(pts[len(pts)-2], pts[len(pts)-1], cl)
		if e.Edge.Op == "<->" {
			p.chevron(pts[1], pts[0], cl)
		}
	}
	if e.Text != "" {
		p.opacity = ap.Opacity * ap.FillIn
		// label the last segment when it has room, so two links leaving one
		// node keep their labels apart; otherwise the longest horizontal one
		best, bl := 0, -1.0
		lt := p.opt.Type["label"]
		lw := p.s.Measure.Measure(layout.Plain(e.Text), lt.Font, lt.Size)
		if k := len(pts) - 2; k >= 0 {
			if l := math.Hypot(pts[k+1][0]-pts[k][0], pts[k+1][1]-pts[k][1]); l >= lw+24 || pts[k][0] == pts[k+1][0] && l >= 40 {
				best, bl = k, l
			}
		}
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

// packet draws a dot part of the way along a connector.
func (p *painter) packet(sc *c.Scene, pk motion.Packet) {
	pts := p.route(sc, pk.Edge)
	if pts == nil {
		return
	}
	var total float64
	for i := 0; i+1 < len(pts); i++ {
		total += math.Hypot(pts[i+1][0]-pts[i][0], pts[i+1][1]-pts[i][1])
	}
	want := total * pk.Progress
	for i := 0; i+1 < len(pts); i++ {
		seg := math.Hypot(pts[i+1][0]-pts[i][0], pts[i+1][1]-pts[i][1])
		if want <= seg || i+2 == len(pts) {
			f := 0.0
			if seg > 0 {
				f = math.Min(1, want/seg)
			}
			x := pts[i][0] + (pts[i+1][0]-pts[i][0])*f
			y := pts[i][1] + (pts[i+1][1]-pts[i][1])*f
			p.m = p.view
			p.opacity = math.Min(1, math.Min(pk.Progress, 1-pk.Progress)*8)
			p.fill(circle(x, y, 4), p.s.col("hi"))
			return
		}
		want -= seg
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

func (s *Stage) highlighted(cb *c.CodeBlock) [][]syntax.Token {
	if v, ok := s.tokens.Load(cb); ok {
		return v.([][]syntax.Token)
	}
	lines := make([]string, len(cb.Lines))
	for i, l := range cb.Lines {
		lines[i] = layout.Untab(l.Text)
	}
	toks := syntax.Highlight(cb.Lang, lines)
	s.tokens.Store(cb, toks)
	return toks
}

func inSel(sel source.Selection, l source.Line) float64 {
	if sel == nil || sel.Has(l) {
		return 1
	}
	return 0
}

// codePanel draws a code or diff block: a header with the file path,
// numbered rows, tinted added and removed lines, syntax colours, and dimmed
// lines outside the focus.
func (p *painter) codePanel(e *c.Element, r layout.Rect, lk look, ap motion.Appearance) {
	cb := e.Code
	s := p.s
	opacity := p.opacity
	p.opacity = opacity * ap.FillIn
	p.draw = 1
	p.fill(rr(r, 12), s.col("panel"))
	p.opacity = opacity
	p.draw = ap.Draw
	p.stroke(rr(r, 12), lk.line, lk.dash)
	p.draw = 1
	p.opacity = opacity * ap.FillIn

	lab := p.opt.Type["label"]
	title := cb.Path
	if title == "" {
		title = strings.ToLower(cb.Lang)
	}
	if title == "" {
		title = "code"
	}
	p.text(title, r.X+18, r.Y+25, lab, s.col("ink-3"), render.Start)
	if cb.Diff {
		adds, dels := cb.Added, cb.Removed
		del := fmt.Sprintf("−%d", dels)
		dw := s.Measure.Measure(del, lab.Font, lab.Size)
		p.text(del, r.Right()-18, r.Y+25, lab, s.col("diff-del"), render.End)
		p.text(fmt.Sprintf("+%d", adds), r.Right()-26-dw, r.Y+25, lab, s.col("diff-add"), render.End)
	} else if cb.Header != "" {
		p.text(cb.Header, r.Right()-18, r.Y+25, lab, s.col("ink-dim"), render.End)
	}
	p.stroke(fmt.Sprintf("M%g %gH%g", r.X, r.Y+layout.CodeHeader, r.Right()), s.col("line-0"), false)

	toks := s.highlighted(cb)
	t := p.opt.Type["code"]
	from, to, mix := p.mo.Focus(e)
	total := 0
	for _, l := range cb.Lines {
		total += len([]rune(layout.Untab(l.Text)))
	}
	left := int(math.Round(ap.Reveal * float64(total)))
	white := color.NRGBA{255, 255, 255, 255}

	for i, l := range cb.Lines {
		y := r.Y + layout.CodeHeader + layout.CodePad + float64(i)*layout.CodeRow
		f := inSel(from, l) + (inSel(to, l)-inSel(from, l))*mix
		rowOp := opacity * ap.FillIn * (0.55 + 0.45*f)
		row := layout.Rect{X: r.X + 1, Y: y, W: r.W - 2, H: layout.CodeRow}

		p.opacity = opacity * ap.FillIn
		if l.Kind == source.Gap {
			mid := y + layout.CodeRow/2
			p.stroke(fmt.Sprintf("M%g %gH%g", r.X+layout.CodeGutter, mid, r.Right()-18), s.col("line-0"), true)
			lab := p.opt.Type["label"]
			txt := fmt.Sprintf(" %d lines ", l.Old)
			if l.Old == 1 {
				txt = " 1 line "
			}
			tw := s.Measure.Measure(txt, lab.Font, lab.Size)
			p.fill(rr(layout.Rect{X: r.CX() - tw/2, Y: mid - 9, W: tw, H: 18}, 4), s.col("panel"))
			p.text(txt, r.CX(), mid+4, lab, s.col("ink-dim"), render.Middle)
			continue
		}
		switch l.Kind {
		case source.Added:
			p.fill(rr(row, 0), alpha(s.col("diff-add"), 0.10*(0.4+0.6*f)))
		case source.Removed:
			p.fill(rr(row, 0), alpha(s.col("diff-del"), 0.10*(0.4+0.6*f)))
		}
		if to != nil && inSel(to, l) > 0 {
			p.opacity = opacity * ap.FillIn * mix
			p.fill(rr(layout.Rect{X: r.X + 1, Y: y, W: 2, H: layout.CodeRow}, 0), alpha(white, 0.85))
		}

		p.opacity = rowOp
		n := l.New
		if l.Kind == source.Removed {
			n = l.Old
		}
		base := y + layout.CodeRow/2 + t.Size*0.36
		p.text(strconv.Itoa(n), r.X+46, base, t, s.col("ink-dim"), render.End)
		switch l.Kind {
		case source.Added:
			p.text("+", r.X+54, base, t, s.col("diff-add"), render.Start)
		case source.Removed:
			p.text("−", r.X+54, base, t, s.col("diff-del"), render.Start)
		}
		x := r.X + layout.CodeGutter
		if i < len(toks) {
			for _, tk := range toks[i] {
				if left <= 0 {
					break
				}
				txt := tk.Text
				if rs := []rune(txt); len(rs) > left {
					txt = string(rs[:left])
				}
				left -= len([]rune(txt))
				cl := s.col("syntax-" + string(tk.Class))
				if l.Kind == source.Removed {
					cl = lerpColor(cl, s.col("ink-dim"), 0.45)
				}
				p.text(txt, x, base, t, cl, render.Start)
				x += s.Measure.Measure(txt, t.Font, t.Size)
			}
		}
	}
	p.opacity = opacity
}

// detour routes a connector around siblings that sit between its ends, so a
// link that skips ranks goes over the boxes instead of through them.
// It returns nil when the direct route is clear.
func (p *painter) detour(ea, eb *c.Element, a, b layout.Rect) [][2]float64 {
	parent := ea.ParentElement()
	if parent == nil || parent != eb.ParentElement() {
		return nil
	}
	const gap, lift = 8.0, 30.0
	horizontal := math.Max(a.Y, b.Y) < math.Min(a.Bottom(), b.Bottom())
	vertical := math.Max(a.X, b.X) < math.Min(a.Right(), b.Right())
	if !horizontal && !vertical {
		return nil
	}
	var blockers []layout.Rect
	for _, sib := range parent.Children {
		if sib == ea || sib == eb || sib.Edge != nil {
			continue
		}
		r, ok := p.lay.Boxes[sib]
		if !ok || r.W == 0 {
			continue
		}
		if horizontal {
			y := (math.Max(a.Y, b.Y) + math.Min(a.Bottom(), b.Bottom())) / 2
			lo, hi := math.Min(a.Right(), b.Right()), math.Max(a.X, b.X)
			if r.X < hi && r.Right() > lo && y > r.Y && y < r.Bottom() {
				blockers = append(blockers, r)
			}
		} else {
			x := (math.Max(a.X, b.X) + math.Min(a.Right(), b.Right())) / 2
			lo, hi := math.Min(a.Bottom(), b.Bottom()), math.Max(a.Y, b.Y)
			if r.Y < hi && r.Bottom() > lo && x > r.X && x < r.Right() {
				blockers = append(blockers, r)
			}
		}
	}
	if len(blockers) == 0 {
		return nil
	}
	if horizontal {
		y := math.Min(a.Y, b.Y)
		for _, r := range blockers {
			y = math.Min(y, r.Y)
		}
		y -= lift
		return [][2]float64{{a.CX(), a.Y - gap}, {a.CX(), y}, {b.CX(), y}, {b.CX(), b.Y - gap}}
	}
	x := math.Max(a.Right(), b.Right())
	for _, r := range blockers {
		x = math.Max(x, r.Right())
	}
	x += lift
	return [][2]float64{{a.Right() + gap, a.CY()}, {x, a.CY()}, {x, b.CY()}, {b.Right() + gap, b.CY()}}
}

// metric draws a large number with an optional label and source note
// under it. Focus brightens it and underlines it; dim greys it out.
func (p *painter) metric(e *c.Element, r layout.Rect, lk look, ap motion.Appearance, state string) {
	s := p.s
	t := layout.TypeOf(e, p.opt)
	opacity := p.opacity
	p.draw = 1
	p.opacity = opacity * ap.FillIn
	num := s.col("ink-2")
	switch state {
	case "focus", "select", "active", "trace":
		num = s.col("hi")
	case "dim":
		num = s.col("ink-dim")
	}
	lines := p.lay.Lines[e]
	y := r.Y + t.Size
	for i, ln := range lines {
		p.runs(ln, r.CX(), y+t.Size*1.35*float64(i), t, num, s.col("hi"), render.Middle)
	}
	y += t.Size*1.35*float64(max(1, len(lines))-1) + 6
	if state == "focus" {
		w := s.Measure.Measure(layout.Plain(e.Text), t.Font, t.Size)
		p.stroke(fmt.Sprintf("M%g %gH%g", r.CX()-w/2, y+4, r.CX()+w/2), s.col("line-2"), false)
	}
	lab := p.opt.Type["label"]
	for _, key := range []string{"label", "note"} {
		v, ok := e.Computed[key]
		if !ok || v.Value == "" {
			continue
		}
		y += lab.Size * 1.9
		cl := s.col("ink-3")
		if key == "note" {
			cl = s.col("ink-dim")
		}
		if state == "dim" {
			cl = s.col("ink-dim")
		}
		p.runs(v.Value, r.CX(), y, lab, cl, s.col("hi"), render.Middle)
	}
	p.opacity = opacity
}

// capWord is one caption word and whether it sits inside ==emphasis==.
type capWord struct {
	text string
	emph bool
}

// captionWords splits a caption into words in the same order as
// strings.Fields(layout.Plain(text)), keeping which ones are emphasised.
func captionWords(text string) []capWord {
	var out []capWord
	emph := false
	for _, tok := range strings.Fields(text) {
		var b strings.Builder
		wordEmph := false
		for i := 0; i < len(tok); i++ {
			if i+1 < len(tok) && tok[i] == '=' && tok[i+1] == '=' {
				emph = !emph
				i++
				continue
			}
			if i+1 < len(tok) && tok[i] == '_' && tok[i+1] == '_' {
				i++
				continue
			}
			if emph {
				wordEmph = true
			}
			b.WriteByte(tok[i])
		}
		if b.Len() > 0 {
			out = append(out, capWord{b.String(), wordEmph})
		}
	}
	return out
}

// followWords draws a caption that follows the voice: words not yet said
// are dim, the word being said is bright, and said words settle to the
// caption colour, or stay bright when emphasised.
func (p *painter) followWords(text string, times []c.WordTime, elapsed, cx, y float64, t layout.Type) {
	s := p.s
	words := captionWords(text)
	space := s.Measure.Measure(" ", t.Font, t.Size)
	total := 0.0
	widths := make([]float64, len(words))
	for i, w := range words {
		widths[i] = s.Measure.Measure(w.text, t.Font, t.Size)
		total += widths[i]
	}
	total += space * float64(max(len(words)-1, 0))
	x := cx - total/2
	dim, said, now := s.col("ink-dim"), s.col("ink-2"), s.col("hi")
	for i, w := range words {
		cl := dim
		if i < len(times) {
			wt := times[i]
			rest := said
			if w.emph {
				rest = now
			}
			switch {
			case elapsed < wt.Start:
				cl = dim
			case elapsed < wt.End:
				cl = lerpColor(dim, now, math.Min(1, (elapsed-wt.Start)/0.08))
			default:
				cl = lerpColor(now, rest, math.Min(1, (elapsed-wt.End)/0.2))
			}
		} else if w.emph {
			cl = now
		}
		p.text(w.text, x, y, t, cl, render.Start)
		x += widths[i] + space
	}
}
