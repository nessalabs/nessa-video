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
)

// Charts, timelines and table rows in motion. One accent at a time,
// everything else grey; numbers keep their width as they count.

// tab draws text whose digits all take the width of the widest digit, so
// a number counting up or down never shifts the text around it.
func (p *painter) tab(s string, x, y float64, t layout.Type, cl color.NRGBA, an render.Anchor) {
	m := p.s.Measure
	dw := 0.0
	for d := '0'; d <= '9'; d++ {
		dw = math.Max(dw, m.Measure(string(d), t.Font, t.Size))
	}
	// runs of digits are laid out one digit per cell, other text as is
	type run struct {
		s     string
		digit bool
		w     float64
	}
	var runs []run
	for _, r := range s {
		d := r >= '0' && r <= '9'
		if d || len(runs) == 0 || runs[len(runs)-1].digit {
			runs = append(runs, run{s: string(r), digit: d})
		} else {
			runs[len(runs)-1].s += string(r)
		}
	}
	total := 0.0
	for i := range runs {
		if runs[i].digit {
			runs[i].w = dw
		} else {
			runs[i].w = m.Measure(runs[i].s, t.Font, t.Size)
		}
		total += runs[i].w
	}
	switch an {
	case render.Middle:
		x -= total / 2
	case render.End:
		x -= total
	}
	for _, r := range runs {
		if r.digit {
			p.text(r.s, x+dw/2, y, t, cl, render.Middle)
		} else {
			p.text(r.s, x, y, t, cl, render.Start)
		}
		x += r.w
	}
}

// niceTop rounds a chart's axis up to a tidy number close above its
// largest value, so the data fills the plot.
func niceTop(v float64) float64 {
	if v <= 0 {
		return 1
	}
	e := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 1.2, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10} {
		if m*e >= v*1.02 {
			return m * e
		}
	}
	return 10 * e
}

// accent is the one colour a chart or timeline singles a row out in.
func (p *painter) accent() color.NRGBA {
	if _, ok := p.s.Tokens["chart-accent"]; ok {
		return p.s.col("chart-accent")
	}
	return p.s.toneCol("info")
}

// valueFormat counts a chart's values in whole numbers when every value
// it shows is whole, else to one decimal.
func valueFormat(cb *c.ChartBlock) func(float64) string {
	whole := true
	for _, r := range cb.Rows {
		for _, v := range r.Values {
			whole = whole && v == math.Trunc(v)
		}
	}
	return func(v float64) string {
		if whole {
			return strconv.FormatFloat(math.Round(v), 'f', 0, 64)
		}
		return strconv.FormatFloat(v, 'f', 1, 64)
	}
}

// seriesTone is a series' colour: the last series of a line chart is the
// accent, every other line and bar grey.
func (p *painter) seriesTone(cb *c.ChartBlock, j int) color.NRGBA {
	if j == len(cb.Series)-1 {
		if cb.Kind == "line" {
			return p.accent()
		}
		if len(cb.Series) > 1 {
			return p.s.col("line-2")
		}
	}
	return p.s.col("line-1")
}

// chartBlock draws a bar or line chart. Its frame draws in as it enters,
// then bars grow one after another and lines trace left to right with a
// dot at their tip. Set steps ease a bar to its new value while its label
// counts; sort slides bars into their new order.
func (p *painter) chartBlock(e *c.Element, r layout.Rect, lk look, ap motion.Appearance) {
	cb := e.Chart
	s := p.s
	opacity := p.opacity
	lab := p.opt.Type["label"]
	small := lab
	small.Size = 11
	now := p.mo.Chart(e)
	since, _, growing := p.mo.Grow(e)
	grow := func(delay, dur float64) float64 {
		if !growing {
			return 1
		}
		return motion.Ease(clamp01((since - delay) / dur))
	}
	format := valueFormat(cb)
	unit := ""
	if cb.Unit != "" {
		unit = " " + cb.Unit
	}
	p.draw = 1
	p.opacity = opacity * ap.FillIn

	// header: the legend (or a lone series' name) left, the caption right
	lx := r.X
	if !cb.Horizontal {
		lx = r.X + 40
	}
	if len(cb.Series) == 1 {
		p.text(cb.Series[0], lx, r.Y+14, lab, s.col("ink-3"), render.Start)
	} else {
		for i, name := range cb.Series {
			cl := p.seriesTone(cb, i)
			if cb.Kind == "line" {
				p.fill(circle(lx+4, r.Y+10, 3.5), cl)
			} else {
				p.fill(rr(layout.Rect{X: lx, Y: r.Y + 5, W: 9, H: 9}, 2), cl)
			}
			p.text(name, lx+16, r.Y+14, lab, s.col("ink-2"), render.Start)
			lx += 34 + s.Measure.Measure(name, lab.Font, lab.Size)
		}
	}
	if e.Text != "" {
		p.runs(e.Text, r.Right()-8, r.Y+14, lab, s.col("ink-3"), s.col("hi"), render.End)
	}

	top := now.Top
	if cb.HasThreshold {
		top = math.Max(top, cb.Threshold)
	}
	top = niceTop(top)
	acc := p.accent()

	if cb.Horizontal {
		lw := 0.0
		for _, row := range cb.Rows {
			lw = math.Max(lw, s.Measure.Measure(row.Label, lab.Font, lab.Size))
		}
		x0, x1 := r.X+lw+24, r.Right()-72
		y0 := r.Y + layout.ChartHead
		p.strokeW(fmt.Sprintf("M%g %gV%g", x0, y0-6, r.Bottom()-4), s.col("line-0"), false, .75)
		ns := float64(len(cb.Series))
		th := math.Min(12, 26/ns)
		for _, i := range accentLast(now) {
			row := cb.Rows[i]
			yc := y0 + layout.BarPitch*(now.Slot[i]+.5)
			a := now.Accent[i]
			g := grow(motion.BarStagger*float64(i), motion.BarGrow)
			p.opacity = opacity * ap.FillIn
			p.text(row.Label, r.X, yc+4, lab, lerpColor(s.col("ink-2"), s.col("ink"), a), render.Start)
			var end float64
			for j, v := range now.Values[i] {
				w := (x1 - x0) * math.Max(0, v) / top * g
				by := yc - th*ns/2 - 2*(ns-1)/2 + float64(j)*(th+2)
				cl := lerpColor(p.seriesTone(cb, j), acc, a)
				if w > 0.5 {
					p.opacity = opacity
					p.fill(rr(layout.Rect{X: x0, Y: by, W: w, H: th}, th/2), cl)
				}
				end = math.Max(end, w)
			}
			if vals := now.Values[i]; len(vals) > 0 && g > 0 {
				p.opacity = opacity * ap.FillIn * math.Min(1, g*3)
				p.tab(format(vals[len(vals)-1])+unit, x0+end+10, yc+4, small, lerpColor(s.col("ink-3"), s.col("ink"), a), render.Start)
			}
		}
		p.opacity = opacity
		return
	}

	x0, y0, x1, y1 := r.X+40, r.Y+36, r.Right()-8, r.Bottom()-28
	yOf := func(v float64) float64 { return y1 - (y1-y0)*v/top }
	for _, g := range []float64{0, .5, 1} {
		yy := yOf(top * g)
		p.strokeW(fmt.Sprintf("M%g %gH%g", x0, yy, x1), alpha(s.col("line-0"), .7), false, .75)
		p.tab(fmtNum(top*g), x0-8, yy+4, small, s.col("ink-dim"), render.End)
	}
	n := len(cb.Rows)
	gw := (x1 - x0) / float64(max(n, 1))
	slotX := func(slot float64) float64 { return x0 + gw*(slot+.5) }

	if cb.Kind == "bar" {
		ns := float64(len(cb.Series))
		bw := math.Min(28, gw*.6/ns)
		if ns == 1 {
			bw = math.Min(36, gw*.5)
		}
		for _, i := range accentLast(now) {
			row := cb.Rows[i]
			cx := slotX(now.Slot[i])
			a := now.Accent[i]
			p.opacity = opacity * ap.FillIn
			p.text(row.Label, cx, y1+20, small, lerpColor(s.col("ink-3"), s.col("ink"), a), render.Middle)
			g := grow(motion.BarStagger*float64(i), motion.BarGrow)
			gx := cx - bw*ns/2 - 3*(ns-1)/2
			topY := y1
			for j, v := range now.Values[i] {
				h := (y1 - yOf(math.Max(0, v))) * g
				if h <= 0.5 {
					continue
				}
				b := layout.Rect{X: gx + float64(j)*(bw+3), Y: y1 - h, W: bw, H: h}
				topY = math.Min(topY, b.Y)
				rad := math.Min(4, h)
				d := fmt.Sprintf("M%g %gV%gQ%g %g %g %gH%gQ%g %g %g %gV%gZ", b.X, y1, b.Y+rad, b.X, b.Y, b.X+rad, b.Y, b.Right()-rad, b.Right(), b.Y, b.Right(), b.Y+rad, y1)
				cl := p.seriesTone(cb, j)
				if j == len(row.Values)-1 {
					cl = lerpColor(cl, acc, a)
				}
				p.opacity = opacity
				if ns == 1 {
					p.fill(d, cl) // flat
				} else {
					p.fill(d, alpha(cl, .4+.4*a))
					p.strokeW(d, cl, false, .9)
				}
			}
			if ns == 1 && g > 0 {
				p.opacity = opacity * ap.FillIn * math.Min(1, g*3)
				p.tab(format(now.Values[i][0])+unit, cx, topY-8, small, lerpColor(s.col("ink-3"), s.col("ink"), a), render.Middle)
			}
		}
		p.opacity = opacity
		return
	}

	// lines
	for i, row := range cb.Rows {
		p.opacity = opacity * ap.FillIn
		p.text(row.Label, slotX(float64(i)), y1+20, small, s.col("ink-3"), render.Middle)
	}
	f := grow(0, motion.LineGrow)
	xa, xb := slotX(0), slotX(float64(n-1))
	tipX := xa + (xb-xa)*f
	if cb.HasThreshold {
		ty := yOf(cb.Threshold)
		p.opacity = opacity * ap.FillIn
		p.strokeW(fmt.Sprintf("M%g %gH%g", x0, ty, x1), s.col("ink-3"), true, 1)
		if cb.ThresholdLabel != "" {
			p.text(cb.ThresholdLabel, x0+4, ty-8, small, s.col("ink-3"), render.Start)
		}
	}
	type mark struct{ x, y, appear float64 }
	var marks []mark
	var crossLabel []string
	for j := range cb.Series {
		cl := p.seriesTone(cb, j)
		var d strings.Builder
		var tip [2]float64
		for i := 0; i < n; i++ {
			v := now.Values[i][j]
			x, y := slotX(float64(i)), yOf(v)
			if i > 0 && x > tipX {
				px, py := slotX(float64(i-1)), yOf(now.Values[i-1][j])
				k := (tipX - px) / (x - px)
				x, y = tipX, py+(y-py)*k
			}
			cmd := "L"
			if i == 0 {
				cmd = "M"
			}
			fmt.Fprintf(&d, "%s%g %g", cmd, x, y)
			tip = [2]float64{x, y}
			if x >= tipX {
				break
			}
		}
		if f <= 0 {
			continue
		}
		p.opacity = opacity
		if n > 1 {
			p.strokeW(d.String(), cl, false, 2)
		}
		p.fill(circle(tip[0], tip[1], 3.5), cl)
		// where the line first crosses the threshold
		if !cb.HasThreshold {
			continue
		}
		for i := 0; i+1 < n; i++ {
			a, b := now.Values[i][j]-cb.Threshold, now.Values[i+1][j]-cb.Threshold
			if a == 0 || a*b > 0 || a == b {
				continue
			}
			k := a / (a - b)
			cx := slotX(float64(i) + k)
			// the moment the tip reaches it, inverting the ease
			lo, hi := 0.0, 1.0
			want := (cx - xa) / math.Max(1e-9, xb-xa)
			for it := 0; it < 30; it++ {
				mid := (lo + hi) / 2
				if motion.Ease(mid) < want {
					lo = mid
				} else {
					hi = mid
				}
			}
			appear := 1.0
			if growing {
				appear = motion.Ease(clamp01((since - lo*motion.LineGrow) / motion.CrossMarkTime))
			}
			if appear > 0 {
				marks = append(marks, mark{cx, yOf(cb.Threshold), appear})
				crossLabel = append(crossLabel, crossText(cb, i, k))
			}
			break
		}
	}
	for i, mk := range marks {
		p.opacity = opacity * mk.appear
		p.fill(circle(mk.x, mk.y, 5*mk.appear), s.col("bg"))
		p.strokeW(circle(mk.x, mk.y, 5*mk.appear), s.col("ink"), false, 1)
		w := s.Measure.Measure(crossLabel[i], small.Font, small.Size)
		if mk.x-14-w > x0 {
			p.tab(crossLabel[i], mk.x-12, mk.y-12, small, s.col("ink"), render.End)
		} else {
			p.tab(crossLabel[i], mk.x+12, mk.y-12, small, s.col("ink"), render.Start)
		}
	}
	p.opacity = opacity
}

// accentLast orders a chart's rows for drawing: the accented ones last,
// so a bar sliding past others while a sort runs stays on top.
func accentLast(now motion.ChartNow) []int {
	var out, hot []int
	for i, a := range now.Accent {
		if a > .5 {
			hot = append(hot, i)
		} else {
			out = append(out, i)
		}
	}
	return append(out, hot...)
}

// crossText is the label of a threshold crossing k of the way from row i
// to the next: cross= with {x} filled in (the place on the x axis when row
// labels are numbers, else the nearer row's label).
func crossText(cb *c.ChartBlock, i int, k float64) string {
	tmpl := cb.Cross
	if tmpl == "" {
		tmpl = "Crosses at {x}"
	}
	x := cb.Rows[i].Label
	if k >= .5 {
		x = cb.Rows[i+1].Label
	}
	a, err1 := strconv.ParseFloat(cb.Rows[i].Label, 64)
	b, err2 := strconv.ParseFloat(cb.Rows[i+1].Label, 64)
	if err1 == nil && err2 == nil {
		v := a + (b-a)*k
		if a == math.Trunc(a) && b == math.Trunc(b) {
			x = strconv.Itoa(int(math.Round(v)))
		} else {
			x = strconv.FormatFloat(v, 'f', 1, 64)
		}
	}
	return strings.ReplaceAll(tmpl, "{x}", x)
}

// timelineBlock draws lanes of spans on a shared axis. With a `play` step
// the outlines draw first and each span fills in as the playhead passes,
// which reads the time aloud above it. `mark … span=` singles a span out:
// the rest dims, the span turns amber and a readout line names it.
func (p *painter) timelineBlock(e *c.Element, r layout.Rect, lk look, ap motion.Appearance) {
	tb := e.Timeline
	s := p.s
	opacity := p.opacity
	lab := p.opt.Type["label"]
	small := lab
	small.Size = 11
	last := tb.End()
	end := nice(last)
	x0, x1 := r.X+layout.LaneLabel, r.Right()-8
	xOf := func(v float64) float64 { return x0 + (x1-x0)*v/end }
	top := r.Y + 14
	axisY := top + layout.LaneHeight*float64(len(tb.Lanes)) + 8

	pos, playing := p.mo.Playhead(e)
	_, pending := p.mo.HasPlay(e)
	fillTo := math.Inf(1) // spans fill up to here
	if playing {
		fillTo = xOf(pos * last)
	} else if pending {
		fillTo = math.Inf(-1)
	}

	from, to, mix, note := p.mo.Picked(e)
	picked := func(names []string, l, k int) float64 {
		for _, n := range names {
			if tb.Picks(n, l, k) {
				return 1
			}
		}
		return 0
	}
	active := 0.0 // how much a pick is in effect
	if len(from) > 0 {
		active += 1 - mix
	}
	if len(to) > 0 {
		active += mix
	}
	warn := s.toneCol("warn")

	p.draw = 1
	for i, lane := range tb.Lanes {
		y := top + float64(i)*layout.LaneHeight
		p.opacity = opacity * ap.FillIn
		p.text(lane.Label, r.X, y+layout.LaneHeight/2+4, lab, s.col("ink-3"), render.Start)
		p.fill(rr(layout.Rect{X: x0, Y: y + 6, W: x1 - x0, H: layout.LaneHeight - 12}, 6), alpha(s.col("hi"), .025))
		for k, sp := range lane.Spans {
			b := layout.Rect{X: xOf(sp.From), Y: y + 9, W: xOf(sp.To) - xOf(sp.From), H: layout.LaneHeight - 18}
			if b.W <= 0 {
				continue
			}
			pk := picked(from, i, k)*(1-mix) + picked(to, i, k)*mix
			dim := 1 - .6*active*(1-pk)
			tok, fillA, dash := "line-1", .34, true
			if sp.Strong {
				tok, fillA, dash = "line-2", .24, false
			}
			line := lerpColor(s.col(tok), warn, pk)
			fillC := lerpColor(s.col(tok), warn, pk)
			fillA += .2 * pk
			// outline, drawn in as the block enters
			p.opacity = opacity * dim
			p.draw = ap.Draw
			p.strokeW(rr(b, 5), line, dash && pk < .5, 1)
			p.draw = 1
			// fill, behind the playhead
			if w := math.Min(b.W, fillTo-b.X); w > 0 {
				fb := b
				fb.W = w
				if math.IsInf(fillTo, 1) {
					fb.W = b.W * ap.Draw
				}
				p.fill(rr(fb, math.Min(5, fb.W/2)), alpha(fillC, fillA))
			}
			if sp.Label != "" {
				tw := s.Measure.Measure(sp.Label, small.Font, small.Size)
				if tw+16 <= b.W {
					p.opacity = opacity * ap.FillIn * dim
					cl := s.col("ink-2")
					if sp.Strong {
						cl = s.col("hi")
					}
					p.runs(sp.Label, b.X+8, b.CY()+4, small, cl, s.col("hi"), render.Start)
				}
			}
			if pk > 0 { // its length beside it
				text := fmtNum(sp.To-sp.From) + unitOf(tb)
				p.opacity = opacity * pk
				tw := s.Measure.Measure(text, small.Font, small.Size)
				if b.Right()+10+tw <= r.Right() {
					p.tab(text, b.Right()+8, b.CY()+4, small, warn, render.Start)
				} else {
					p.tab(text, b.X-8, b.CY()+4, small, warn, render.End)
				}
			}
		}
	}
	p.opacity = opacity * ap.FillIn
	p.strokeW(fmt.Sprintf("M%g %gH%g", x0, axisY, x1), s.col("line-0"), false, .75)
	stepV := nice(end / 4)
	if tb.Unit == "none" { // an order of events, not a measurement: no numbers
		stepV = math.Inf(1)
	}
	for v := 0.0; v <= end+1e-9 && !math.IsInf(stepV, 1); v += stepV {
		x := xOf(v)
		p.stroke(fmt.Sprintf("M%g %gv5", x, axisY), s.col("line-0"), false)
		label := fmtNum(v)
		if v+stepV > end+1e-9 && tb.Unit != "" {
			label += " " + tb.Unit
		}
		p.tab(label, x, axisY+20, small, s.col("ink-dim"), render.Middle)
	}
	// the readout under the axis
	if tb.Readout && active > 0 {
		ry := axisY + 20 + layout.Readout
		say := func(names []string, op float64) {
			if len(names) == 0 || op <= 0 {
				return
			}
			p.opacity = opacity * ap.FillIn * op
			p.fill(circle(x0+3, ry-4, 3), warn)
			p.runs(readoutText(tb, names, note), x0+14, ry, lab, s.col("ink-2"), s.col("hi"), render.Start)
		}
		if mix < 1 {
			say(from, 1-mix)
		}
		say(to, mix)
	}
	if playing {
		x := xOf(pos * last)
		op := 1.0
		if ago, done := p.mo.PlayEnded(e); done {
			op = 1 - motion.Ease(clamp01(ago/0.4))
		}
		if op > 0 {
			p.opacity = opacity * op
			p.strokeW(fmt.Sprintf("M%g %gV%g", x, top+2, axisY), s.col("hi"), false, 1)
			if tb.Unit != "none" {
				p.tab(fmtNum(math.Round(pos*last))+unitOf(tb), x, top-4, small, s.col("hi"), render.Middle)
			} else {
				p.fill(circle(x, top, 3), s.col("hi"))
			}
		}
	}
	p.opacity = opacity
}

func unitOf(tb *c.TimelineBlock) string {
	if tb.Unit == "" || tb.Unit == "none" {
		return ""
	}
	return " " + tb.Unit
}

// readoutText is what a timeline says about the spans a mark picks: its
// note=, or the first span's name and length against the whole.
func readoutText(tb *c.TimelineBlock, names []string, note string) string {
	if note != "" {
		return note
	}
	first, lastT := math.Inf(1), 0.0
	for _, l := range tb.Lanes {
		for _, sp := range l.Spans {
			first, lastT = math.Min(first, sp.From), math.Max(lastT, sp.To)
		}
	}
	for l, ln := range tb.Lanes {
		for k, sp := range ln.Spans {
			for _, n := range names {
				if tb.Picks(n, l, k) {
					name := sp.Label
					if name == "" {
						name = ln.Label
					}
					return fmt.Sprintf("%s: %s%s of %s%s", name, fmtNum(sp.To-sp.From), unitOf(tb), fmtNum(lastT-first), unitOf(tb))
				}
			}
		}
	}
	return ""
}

// inTable reports whether e sits in a table.
func inTable(e *c.Element) bool {
	for p := e.ParentElement(); p != nil; p = p.ParentElement() {
		if p.Kind() == "table" {
			return true
		}
	}
	return false
}

// cellFade draws a table cell whose text a set step is changing: the old
// text fades out as the new one fades in, in the same place. It reports
// whether it drew the cell.
func (p *painter) cellFade(e *c.Element, r layout.Rect, t layout.Type, base, hi color.NRGBA, op float64) bool {
	l := p.lay
	if l.TextMix <= 0 || l.TextMix >= 1 || !inTable(e) {
		return false
	}
	was, had := l.TextWas[e]
	now, has := l.Text[e]
	if had == has && was == now {
		return false
	}
	draw := func(lines []string, o float64) {
		p.opacity = op * o
		for i, ln := range lines {
			p.runs(ln, r.X, r.Y+t.Size*(1.0+1.35*float64(i)), t, base, hi, render.Start)
		}
	}
	old := l.LinesWas[e]
	if old == nil {
		old = []string{e.Text}
		if had {
			old = []string{was}
		}
	}
	draw(old, 1-l.TextMix)
	draw(l.Lines[e], l.TextMix)
	return true
}

// Row greetings: a table row that a show step brings in slides down into
// place over RowSlide and wears a soft highlight that fades after a
// moment, so the eye finds it.
const (
	RowSlide     = 0.6
	RowHighlight = 1.4 // how long the highlight holds
	RowFade      = 1.0 // how long it takes to fade
)

// rowSlide is how far above its place a newly shown table row still is.
func (p *painter) rowSlide(e *c.Element) float64 {
	if par := e.ParentElement(); par == nil || par.Kind() != "table" {
		return 0
	}
	ago, ok := p.mo.ShownFor(e)
	if !ok || ago < 0 || ago >= RowSlide {
		return 0
	}
	return 6 * (1 - motion.Ease(ago/RowSlide))
}

// rowGlow is how strong a newly shown table row's highlight is.
func (p *painter) rowGlow(e *c.Element) float64 {
	ago, ok := p.mo.ShownFor(e)
	if !ok || ago < 0 {
		return 0
	}
	return 1 - motion.Ease(clamp01((ago-RowHighlight)/RowFade))
}
