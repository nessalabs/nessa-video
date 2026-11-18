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

func inCells(set []int, i int) float64 {
	if set == nil {
		return 1
	}
	for _, x := range set {
		if x == i {
			return 1
		}
	}
	return 0
}

// arrayBlock draws cells with values, indexes below and pointers above.
// Marked cells are bright and the rest dim; swaps slide cells past each other.
func (p *painter) arrayBlock(e *c.Element, r layout.Rect, lk look, ap motion.Appearance) {
	ab := e.Array
	s := p.s
	opacity := p.opacity
	slots := p.mo.Slots(e)
	from, to, mix := p.mo.Cells(e)
	marking := from != nil || to != nil
	t := p.opt.Type["box"]
	t.Size = 20
	lab := p.opt.Type["label"]
	y := r.Y + layout.PointerTop
	step := layout.CellSize + layout.CellGap

	for i := range ab.Values {
		p.opacity = opacity * ap.FillIn
		p.draw = 1
		p.text(strconv.Itoa(i), r.X+float64(i)*step+layout.CellSize/2, y+layout.CellSize+20, lab, s.col("ink-dim"), render.Middle)
	}
	for v, val := range ab.Values {
		slot := float64(v)
		if v < len(slots) {
			slot = slots[v]
		}
		cell := layout.Rect{X: r.X + slot*step, Y: y, W: layout.CellSize, H: layout.CellSize}
		cl := lk
		if marking {
			// emphasis follows the cell a value sits in once a swap finishes
			at := int(math.Round(slot))
			f := inCells(from, at) + (inCells(to, at)-inCells(from, at))*mix
			cl = p.blend("dim", "focus", f)
		}
		p.opacity = opacity
		p.draw = ap.Draw
		if cl.fill.A > 0 {
			p.opacity = opacity * ap.FillIn
			p.fill(rr(cell, 8), cl.fill)
			p.opacity = opacity
		}
		p.stroke(rr(cell, 8), cl.line, cl.dash)
		p.draw = 1
		p.opacity = opacity * ap.FillIn
		p.text(val, cell.CX(), cell.CY()+t.Size*0.36, t, cl.text, render.Middle)
	}
	for _, ptr := range p.mo.Pointers(e) {
		x := r.X + ptr.Pos*step + layout.CellSize/2
		p.opacity = opacity * ap.FillIn
		p.text(ptr.Name, x, r.Y+14, lab, s.col("hi"), render.Middle)
		p.stroke(fmt.Sprintf("M%g %gV%g", x, r.Y+20, y-6), s.col("line-2"), false)
		p.stroke(fmt.Sprintf("M%g %gL%g %gL%g %g", x-4, y-11, x, y-6, x+4, y-11), s.col("line-2"), false)
	}
	p.opacity = opacity
}

// nice rounds up to 1, 2, 2.5 or 5 times a power of ten.
func nice(v float64) float64 {
	if v <= 0 {
		return 1
	}
	e := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if m*e >= v {
			return m * e
		}
	}
	return 10 * e
}

func fmtNum(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// seriesColor gives the last series the bright line and earlier ones grey.
func (p *painter) seriesColor(i, n int) (line, fill float64, token string) {
	if i == n-1 {
		return 1, .22, "line-2"
	}
	return 1, .12, "line-1"
}

// chartBlock draws a bar or line chart. Bars grow and lines trace as the
// chart enters.
func (p *painter) chartBlock(e *c.Element, r layout.Rect, lk look, ap motion.Appearance) {
	cb := e.Chart
	s := p.s
	opacity := p.opacity
	lab := p.opt.Type["label"]
	small := lab
	small.Size = 11
	p.draw = 1
	p.opacity = opacity * ap.FillIn

	// legend
	lx := r.X + 40
	for i, name := range cb.Series {
		_, fa, tok := p.seriesColor(i, len(cb.Series))
		p.fill(rr(layout.Rect{X: lx, Y: r.Y + 4, W: 10, H: 10}, 2), alpha(s.col(tok), math.Max(fa*3, .6)))
		p.text(name, lx+16, r.Y+14, lab, s.col("ink-2"), render.Start)
		lx += 32 + s.Measure.Measure(name, lab.Font, lab.Size)
	}

	x0, y0, x1, y1 := r.X+40, r.Y+36, r.Right()-8, r.Bottom()-28
	top := 0.0
	for _, row := range cb.Rows {
		for _, v := range row.Values {
			top = math.Max(top, v)
		}
	}
	top = nice(top)
	yOf := func(v float64) float64 { return y1 - (y1-y0)*v/top }
	for _, g := range []float64{0, .5, 1} {
		yy := yOf(top * g)
		p.stroke(fmt.Sprintf("M%g %gH%g", x0, yy, x1), s.col("line-0"), g > 0)
		p.text(fmtNum(top*g), x0-8, yy+4, small, s.col("ink-dim"), render.End)
	}
	n := len(cb.Rows)
	gw := (x1 - x0) / float64(n)
	for i, row := range cb.Rows {
		p.text(row.Label, x0+gw*(float64(i)+.5), y1+20, small, s.col("ink-3"), render.Middle)
	}

	grow := ap.Draw
	if cb.Kind == "bar" {
		ns := float64(len(cb.Series))
		bw := math.Min(28, gw*.6/ns)
		for i, row := range cb.Rows {
			gx := x0 + gw*(float64(i)+.5) - bw*ns/2 - 3*(ns-1)/2
			for j, v := range row.Values {
				_, fa, tok := p.seriesColor(j, len(cb.Series))
				h := (y1 - yOf(v)) * grow
				if h <= 0 {
					continue
				}
				b := layout.Rect{X: gx + float64(j)*(bw+3), Y: y1 - h, W: bw, H: h}
				d := fmt.Sprintf("M%g %gV%gQ%g %g %g %gH%gQ%g %g %g %gV%g", b.X, y1, b.Y+3, b.X, b.Y, b.X+3, b.Y, b.Right()-3, b.Right(), b.Y, b.Right(), b.Y+3, y1)
				p.fill(d+"Z", alpha(s.col(tok), fa))
				p.stroke(d, s.col(tok), false)
			}
		}
	} else {
		for j := range cb.Series {
			_, _, tok := p.seriesColor(j, len(cb.Series))
			var d strings.Builder
			for i, row := range cb.Rows {
				if j >= len(row.Values) {
					continue
				}
				x := x0 + gw*(float64(i)+.5)
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&d, "%s%g %g", cmd, x, yOf(row.Values[j]))
			}
			p.draw = grow
			p.stroke(d.String(), s.col(tok), false)
			p.draw = 1
			if grow >= 1 && len(cb.Rows) > 0 {
				last := cb.Rows[len(cb.Rows)-1]
				if j < len(last.Values) {
					p.fill(circle(x0+gw*(float64(n)-.5), yOf(last.Values[j]), 3.5), s.col(tok))
				}
			}
		}
	}
	p.opacity = opacity
}

// timelineBlock draws lanes of spans on a shared axis, with a playhead
// when `play` runs.
func (p *painter) timelineBlock(e *c.Element, r layout.Rect, lk look, ap motion.Appearance) {
	tb := e.Timeline
	s := p.s
	opacity := p.opacity
	lab := p.opt.Type["label"]
	small := lab
	small.Size = 11
	end := nice(tb.End())
	x0, x1 := r.X+layout.LaneLabel, r.Right()-8
	xOf := func(v float64) float64 { return x0 + (x1-x0)*v/end }
	top := r.Y + 14
	axisY := top + layout.LaneHeight*float64(len(tb.Lanes)) + 8

	p.draw = 1
	for i, lane := range tb.Lanes {
		y := top + float64(i)*layout.LaneHeight
		p.opacity = opacity * ap.FillIn
		p.text(lane.Label, r.X, y+layout.LaneHeight/2+4, lab, s.col("ink-3"), render.Start)
		p.fill(rr(layout.Rect{X: x0, Y: y + 6, W: x1 - x0, H: layout.LaneHeight - 12}, 6), alpha(s.col("hi"), .025))
		for _, sp := range lane.Spans {
			w := (xOf(sp.To) - xOf(sp.From)) * ap.Draw
			if w <= 0 {
				continue
			}
			b := layout.Rect{X: xOf(sp.From), Y: y + 9, W: w, H: layout.LaneHeight - 18}
			tok, fill, dash := "line-1", .05, true
			if sp.Strong {
				tok, fill, dash = "line-2", .12, false
			}
			p.opacity = opacity
			p.fill(rr(b, 5), alpha(s.col(tok), fill))
			p.stroke(rr(b, 5), s.col(tok), dash)
			if sp.Label != "" && ap.Draw >= 1 {
				tw := s.Measure.Measure(sp.Label, small.Font, small.Size)
				if tw+16 <= b.W {
					p.opacity = opacity * ap.FillIn
					cl := s.col("ink-2")
					if sp.Strong {
						cl = s.col("hi")
					}
					p.text(sp.Label, b.X+8, b.CY()+4, small, cl, render.Start)
				}
			}
		}
	}
	p.opacity = opacity * ap.FillIn
	p.stroke(fmt.Sprintf("M%g %gH%g", x0, axisY, x1), s.col("line-0"), false)
	stepV := nice(end / 4)
	for v := 0.0; v <= end+1e-9; v += stepV {
		x := xOf(v)
		p.stroke(fmt.Sprintf("M%g %gv5", x, axisY), s.col("line-0"), false)
		label := fmtNum(v)
		if v+stepV > end+1e-9 && tb.Unit != "" {
			label += " " + tb.Unit
		}
		p.text(label, x, axisY+20, small, s.col("ink-dim"), render.Middle)
	}
	if pos, ok := p.mo.Playhead(e); ok {
		x := x0 + (x1-x0)*pos
		p.opacity = opacity
		p.stroke(fmt.Sprintf("M%g %gV%g", x, top, axisY), s.col("hi"), false)
		p.fill(circle(x, top, 3.5), s.col("hi"))
	}
	p.opacity = opacity
}

// mathBlock draws a formula typeset by the rasterizer's TeX engine.
func (p *painter) mathBlock(e *c.Element, r layout.Rect, cl color.NRGBA, ap motion.Appearance) {
	ts, ok := p.s.Measure.(layout.Typesetter)
	if !ok {
		return
	}
	d, _, _, err := ts.Typeset(e.Text, layout.TypeOf(e, p.opt).Size)
	if err != nil {
		lab := p.opt.Type["label"]
		p.draw = 1
		p.text("math: "+err.Error(), r.X, r.CY(), lab, p.s.col("diff-del"), render.Start)
		return
	}
	m := p.m
	p.m = p.m.Mul(render.Translate(r.X, r.Y))
	op := p.opacity
	p.opacity = op * ap.FillIn
	p.fill(d, cl)
	p.opacity = op
	p.m = m
}
