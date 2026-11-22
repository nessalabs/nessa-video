package stage

import (
	"fmt"
	"image/color"
	"math"
	"regexp"
	"strconv"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/layout"
	"github.com/nessalabs/nessa-video/internal/motion"
	"github.com/nessalabs/nessa-video/internal/plot"
	"github.com/nessalabs/nessa-video/internal/render"
)

// Plots: hairline axes in the line colour, tick numbers in small muted
// mono, the equations in mono at the top left, curves 2px in the accent
// with a leading dot while they trace, amber for marks and notes.

// Plot geometry, in design units: the equation band above the axes, the
// gutters tick labels sit in when an axis runs along an edge, and the
// room a label needs before its ticks get one.
const (
	plotHead     = 30.0 // the first equation line
	plotHeadLine = 19.0 // each further equation line
	plotLeft     = 42.0
	plotBottom   = 24.0
	plotRight    = 8.0
	plotRoomX    = 56.0
	plotRoomY    = 30.0
)

// plotArea is the box a plot's axes span inside its element box.
func plotArea(r layout.Rect, curves int) layout.Rect {
	head := plotHead + plotHeadLine*float64(max(curves-1, 0))
	return layout.Rect{X: r.X + plotLeft, Y: r.Y + head, W: math.Max(10, r.W-plotLeft-plotRight), H: math.Max(10, r.H-head-plotBottom)}
}

var prettyNames = regexp.MustCompile(`\b(pi|theta|zeta)\b`)

// pretty shows pi, theta and zeta as π, θ and ζ in an equation.
func pretty(src string) string {
	return prettyNames.ReplaceAllStringFunc(src, func(s string) string {
		return map[string]string{"pi": "π", "theta": "θ", "zeta": "ζ"}[s]
	})
}

func overlaps(a, b layout.Rect) bool {
	return a.X < b.Right() && b.X < a.Right() && a.Y < b.Bottom() && b.Y < a.Bottom()
}

func within(a, b layout.Rect) bool {
	return a.X >= b.X && a.Y >= b.Y && a.Right() <= b.Right() && a.Bottom() <= b.Bottom()
}

// greek are the letters plot text draws through the TeX engine, since
// the bundled fonts carry Latin only, and how far each sits below the
// baseline as a share of the size.
var greek = map[rune]struct {
	tex  string
	drop float64
}{'π': {`\pi`, 0}, 'θ': {`\theta`, 0}, 'ζ': {`\zeta`, 0.2}}

// greekSet is the size Greek letters are typeset at and scaled from; the
// TeX engine writes glyphs set much smaller than this with numbers its own
// path reader cannot read back.
const greekSet = 100.0

// mathRuns splits plot text into runs of plain text and Greek letters.
func mathRuns(s string) []string {
	var out []string
	for _, r := range s {
		_, g := greek[r]
		if g || len(out) == 0 {
			out = append(out, string(r))
			continue
		}
		last := []rune(out[len(out)-1])
		if _, lg := greek[last[0]]; lg {
			out = append(out, string(r))
		} else {
			out[len(out)-1] += string(r)
		}
	}
	return out
}

// plotTextWidth measures text drawn by plotText.
func (p *painter) plotTextWidth(s string, t layout.Type) float64 {
	w := 0.0
	ts, _ := p.s.Measure.(layout.Typesetter)
	for _, run := range mathRuns(s) {
		if g, ok := greek[[]rune(run)[0]]; ok && ts != nil {
			if _, gw, _, err := ts.Typeset(g.tex, greekSet); err == nil {
				w += gw*t.Size*1.1/greekSet + 1.5
				continue
			}
		}
		w += p.s.Measure.Measure(run, t.Font, t.Size)
	}
	return w
}

// plotText draws text that may hold π, θ or ζ: letters the fonts lack are
// set by the TeX engine and placed on the text's baseline.
func (p *painter) plotText(s string, x, y float64, t layout.Type, cl color.NRGBA, an render.Anchor) {
	ts, ok := p.s.Measure.(layout.Typesetter)
	if !ok || !strings.ContainsAny(s, "πθζ") {
		p.text(s, x, y, t, cl, an)
		return
	}
	w := p.plotTextWidth(s, t)
	switch an {
	case render.Middle:
		x -= w / 2
	case render.End:
		x -= w
	}
	for _, run := range mathRuns(s) {
		if g, ok := greek[[]rune(run)[0]]; ok {
			if d, gw, gh, err := ts.Typeset(g.tex, greekSet); err == nil {
				k := t.Size * 1.1 / greekSet
				m := p.m
				p.m = p.m.Mul(render.Translate(x+0.75, y-gh*k+g.drop*t.Size)).Mul(render.Affine{k, 0, 0, k, 0, 0})
				p.fill(d, cl)
				p.m = m
				x += gw*k + 1.5
				continue
			}
		}
		p.text(run, x, y, t, cl, render.Start)
		x += p.s.Measure.Measure(run, t.Font, t.Size)
	}
}

// plotLabel draws a label on a surface backing beside the point (x, y),
// on the first side where it stays inside box and clear of avoid, and
// returns where it went.
func (p *painter) plotLabel(text string, x, y, gap float64, t layout.Type, cl color.NRGBA, box layout.Rect, avoid []layout.Rect) layout.Rect {
	w := p.plotTextWidth(text, t) + 12
	h := t.Size + 9
	cands := []layout.Rect{
		{X: x + gap, Y: y - gap - h, W: w, H: h}, // above right
		{X: x - gap - w, Y: y - gap - h, W: w, H: h},
		{X: x + gap, Y: y + gap, W: w, H: h},
		{X: x - gap - w, Y: y + gap, W: w, H: h},
	}
	best, bestScore := cands[0], math.Inf(1)
	for i, r := range cands {
		score := float64(i) * 0.1
		if !within(r, box) {
			score += 10
		}
		for _, a := range avoid {
			if overlaps(r, a) {
				score += 3
			}
		}
		if score < bestScore {
			best, bestScore = r, score
		}
	}
	p.fill(rr(best, 4), p.s.col("bg"))
	p.plotText(text, best.X+6, best.Y+h-6, t, cl, render.Start)
	return best
}

// plotBlock draws a plot at one moment: its axes with ticks that fade in
// as room opens, the equations, curves traced up to the playhead with a
// dot at the tip, zero marks, points and notes. The view comes from the
// plot's zooms and is applied to whatever box the plot has now, so a plot
// moving into the spotlight keeps its centre and only gains room.
func (p *painter) plotBlock(e *c.Element, r layout.Rect, ap motion.Appearance) {
	pb := e.Plot
	s := p.s
	if p.drawingIso && e == p.iso && p.isoBox != nil {
		r = *p.isoBox
	}
	opacity := p.opacity
	small := layout.Type{Font: "mono", Size: 11}
	eq := layout.Type{Font: "mono", Size: 13}
	box := plotArea(r, len(pb.Curves))
	frame := plot.Frame{Base: plot.Ranges{X0: pb.X[0], X1: pb.X[1], Y0: pb.Y[0], Y1: pb.Y[1]}, Square: pb.Square, W: box.W, H: box.H}
	view := frame.ViewAt(p.mo.PlotZooms(e), p.mo.T)
	kx, ky := frame.Scale(view)
	vis := frame.Visible(view)
	toS := func(x, y float64) (float64, float64) {
		sx, sy := frame.Screen(view, x, y)
		return box.CX() + sx, box.CY() + sy
	}
	warn := s.toneCol("warn")
	axisCl := s.col("line-1")
	tickCl := s.col("ink-3")

	// axes: through the origin, or along the nearer edge when it is out
	// of view; they always span the whole box
	ox, oy := toS(0, 0)
	ax := math.Max(box.X, math.Min(box.Right(), ox))
	ay := math.Max(box.Y, math.Min(box.Bottom(), oy))
	p.draw = ap.Draw
	p.opacity = opacity
	p.strokeW(fmt.Sprintf("M%g %gH%g", box.X, ay, box.Right()), axisCl, false, 1)
	p.strokeW(fmt.Sprintf("M%g %gV%g", ax, box.Bottom(), box.Y), axisCl, false, 1)
	p.draw = 1
	// labels fade near the box's ends, so a tick scrolling out of view
	// goes quietly
	const fadeOut = 12.0
	edge := func(v, lo, hi float64) float64 {
		return clamp01((v-lo+fadeOut)/fadeOut) * clamp01((hi-v+fadeOut)/fadeOut)
	}
	fill := opacity * ap.FillIn
	for _, tk := range plot.Ticks(vis.X0-fadeOut/kx, vis.X1+fadeOut/kx, kx, plotRoomX, pb.XPi) {
		px, _ := toS(tk.V, 0)
		if px >= box.X-0.5 && px <= box.Right()+0.5 {
			p.opacity = fill * tk.Alpha
			p.strokeW(fmt.Sprintf("M%g %gV%g", px, ay-3, ay+3), axisCl, false, 1)
		}
		if tk.V == 0 && ax == ox { // the origin's 0 is written once, below
			p.opacity = fill * tk.Label
			p.text("0", ax-6, ay+16, small, tickCl, render.End)
			continue
		}
		p.opacity = fill * tk.Label * edge(px, box.X, box.Right())
		p.plotText(tk.Text, px, ay+16, small, tickCl, render.Middle)
	}
	for _, tk := range plot.Ticks(vis.Y0-fadeOut/ky, vis.Y1+fadeOut/ky, ky, plotRoomY, pb.YPi) {
		_, py := toS(0, tk.V)
		if py >= box.Y-0.5 && py <= box.Bottom()+0.5 {
			p.opacity = fill * tk.Alpha
			p.strokeW(fmt.Sprintf("M%g %gH%g", ax-3, py, ax+3), axisCl, false, 1)
		}
		if tk.V == 0 && ay == oy {
			continue
		}
		p.opacity = fill * tk.Label * edge(py, box.Y, box.Bottom())
		p.plotText(tk.Text, ax-7, py+4, small, tickCl, render.End)
	}
	// where tick labels sit, for notes to keep clear of
	avoid := []layout.Rect{
		{X: box.X, Y: ay - 4, W: box.W, H: 24},
		{X: ax - 46, Y: box.Y, W: 50, H: box.H},
	}

	// equations top left, the parameter readout top right
	params, ro, show := p.mo.PlotParams(e)
	p.opacity = fill
	for i, cv := range pb.Curves {
		y := r.Y + 16 + plotHeadLine*float64(i)
		x := r.X + plotLeft
		if len(pb.Curves) > 1 {
			p.strokeW(fmt.Sprintf("M%g %gH%g", x, y-4, x+14), p.curveColor(i), false, 2)
			x += 22
		}
		p.plotText(pretty(cv.Src), x, y, eq, s.col("ink-2"), render.Start)
	}
	if ro >= 0 && show > 0 {
		p.opacity = fill * show
		txt := pb.Params[ro].Name + " = " + strconv.FormatFloat(params[ro], 'f', 2, 64)
		p.tab(txt, r.Right()-plotRight, r.Y+16, eq, s.col("ink"), render.End)
	}

	// curves
	pos, dot := p.mo.PlotTrace(e)
	clip := plot.Box{X0: box.X, Y0: box.Y, X1: box.Right(), Y1: box.Bottom()}
	inBox := func(x, y float64) bool {
		return x >= box.X-0.5 && x <= box.Right()+0.5 && y >= box.Y-0.5 && y <= box.Bottom()+0.5
	}
	type tip struct {
		x, y float64
		cl   color.NRGBA
	}
	var tips []tip
	for i := range pb.Curves {
		cv := &pb.Curves[i]
		if pos <= 0 {
			break
		}
		at := func(t float64) (float64, float64, bool) {
			x, y, ok := cv.At(t, params)
			px, py := toS(x, y)
			return px, py, ok
		}
		pieces := plot.Sample(at, cv.From, cv.To, clip)
		if pos < 1 {
			tEnd := cv.From + (cv.To-cv.From)*pos
			ex, ey, ok := at(tEnd)
			pieces = plot.Cut(pieces, tEnd, [2]float64{ex, ey}, ok)
			if ok && inBox(ex, ey) {
				tips = append(tips, tip{ex, ey, p.curveColor(i)})
			}
		} else if n := len(pieces); n > 0 {
			last := pieces[n-1].Pts[len(pieces[n-1].Pts)-1]
			if inBox(last[0], last[1]) {
				tips = append(tips, tip{last[0], last[1], p.curveColor(i)})
			}
		}
		p.opacity = opacity
		if d := plot.Path(plot.Clip(pieces, clip)); d != "" {
			p.strokeW(d, p.curveColor(i), false, 2)
		}
		// zero marks: an amber ring pulses where the trace passes zero
		if cv.MarkZeros {
			latest, latestSince := -1, 0.0
			for k, z := range cv.Zeros {
				since, ok := p.mo.PlotReached(e, (z-cv.From)/(cv.To-cv.From))
				if !ok {
					continue
				}
				zx, zy, ok := at(z)
				if !ok || !inBox(zx, zy) {
					continue
				}
				grow := motion.Ease(clamp01(since / 0.45))
				p.opacity = opacity
				p.strokeW(circle(zx, zy, 6*grow), warn, false, 1.5)
				if u := since / 1.1; u >= 0 && u < 1 {
					p.opacity = opacity * (1 - u) * 0.8
					p.strokeW(circle(zx, zy, 6+16*motion.Ease(u)), warn, false, 1)
				}
				latest, latestSince = k, since
			}
			if latest >= 0 {
				z := cv.Zeros[latest]
				zx, zy, _ := at(z)
				p.opacity = opacity * clamp01(latestSince/0.35)
				name := cv.Var
				if name == "theta" {
					name = "θ"
				}
				txt := fmt.Sprintf("zero at %s = %s", name, strconv.FormatFloat(z, 'f', 2, 64))
				avoid = append(avoid, p.plotLabel(txt, zx, zy, 11, small, warn, box, avoid))
			}
		}
	}

	// points, once the curves are drawn
	if len(pb.Points) > 0 {
		show := 1.0
		if len(pb.Curves) > 0 {
			if since, ok := p.mo.PlotReached(e, 1); ok {
				show = clamp01(since / 0.4)
			} else {
				show = 0
			}
		}
		vars := append([]complex128{0}, toComplex(params)...)
		for _, pt := range pb.Points {
			x, y := real(pt.X.Eval(vars)), real(pt.Y.Eval(vars))
			px, py := toS(x, y)
			if !inBox(px, py) || show <= 0 {
				continue
			}
			p.opacity = opacity * show
			p.fill(circle(px, py, 3.5), s.col("hi"))
			if pt.Label != "" {
				avoid = append(avoid, p.plotLabel(pt.Label, px, py, 7, small, s.col("ink-2"), box, avoid))
			}
		}
	}

	// notes: an amber ring with a label on a surface backing
	for _, n := range p.mo.PlotNotes(e) {
		px, py := toS(n.X, n.Y)
		if !inBox(px, py) {
			continue
		}
		in := motion.Ease(n.In)
		p.opacity = opacity * in
		p.strokeW(circle(px, py, 4+3*in), warn, false, 1.5)
		p.opacity = opacity * clamp01((n.In-0.3)/0.7)
		avoid = append(avoid, p.plotLabel(n.Text, px, py, 12, layout.Type{Font: "mono", Size: 12}, warn, box, avoid))
	}

	// the leading dot, on top of everything
	for _, tp := range tips {
		if dot <= 0 {
			break
		}
		p.opacity = opacity * dot
		p.fill(circle(tp.x, tp.y, 6), s.col("bg"))
		p.fill(circle(tp.x, tp.y, 4), tp.cl)
	}
	p.opacity = opacity
}

// curveColor is the accent for a plot's first curve and grey for others,
// one accent at a time.
func (p *painter) curveColor(i int) color.NRGBA {
	if i == 0 {
		return p.accent()
	}
	return p.s.col("line-2")
}

func toComplex(xs []float64) []complex128 {
	out := make([]complex128, len(xs))
	for i, x := range xs {
		out[i] = complex(x, 0)
	}
	return out
}

// isolatedPlot moves an isolated plot's box to the spotlight and lets it
// take the room there, instead of scaling the drawing: text and lines
// keep their size, ticks fade in as the axes gain room, and the view's
// centre and scale carry on as they were.
func (p *painter) isolatedPlot(e *c.Element, r layout.Rect, f float64, spot layout.Rect, plainView render.Affine, scale float64) {
	inv := func(px, py float64) (float64, float64) {
		return (px - plainView[4]) / plainView[0], (py - plainView[5]) / plainView[3]
	}
	const m = 12.0
	x0, y0 := inv((spot.X+m)*scale, (spot.Y+m)*scale)
	x1, y1 := inv((spot.Right()-m)*scale, (spot.Bottom()-m)*scale)
	to := layout.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
	box := layout.Rect{X: r.X + (to.X-r.X)*f, Y: r.Y + (to.Y-r.Y)*f, W: r.W + (to.W-r.W)*f, H: r.H + (to.H-r.H)*f}
	p.view, p.m = plainView, plainView
	held := 1.0
	for x := e.ParentElement(); x != nil; x = x.ParentElement() {
		held *= p.mo.Look(x).Opacity
	}
	p.isoBox = &box
	p.drawingIso, p.dimAll = true, 1
	p.element(e, "rest", held, 0)
	p.drawingIso, p.isoBox = false, nil
}
