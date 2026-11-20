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
					p.runs(sp.Label, b.X+8, b.CY()+4, small, cl, s.col("hi"), render.Start)
				}
			}
		}
	}
	p.opacity = opacity * ap.FillIn
	p.stroke(fmt.Sprintf("M%g %gH%g", x0, axisY, x1), s.col("line-0"), false)
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

// sequenceBlock draws a sequence diagram: participants at the top, dashed
// lifelines, messages as arrows with labels, notes and alt/opt/loop frames.
// `play` reveals the rows one after another; otherwise all rows show.
func (p *painter) sequenceBlock(e *c.Element, r layout.Rect, lk look, ap motion.Appearance) {
	sb := e.Sequence
	s := p.s
	opacity := p.opacity
	box := p.opt.Type["box"]
	box.Size = 14
	lab := p.opt.Type["label"]
	xs := p.lay.SeqX[e]
	xOf := func(i int) float64 {
		if i < len(xs) {
			return r.X + xs[i]
		}
		return r.X + layout.SeqColumn*(float64(i)+.5)
	}
	bottom := r.Bottom() - 8

	p.draw = 1
	for i, name := range sb.Participants {
		x := xOf(i)
		hw := layout.SeqHeadWidth(s.Measure, name, box)
		head := layout.Rect{X: x - hw/2, Y: r.Y, W: hw, H: 38}
		p.opacity = opacity
		p.draw = ap.Draw
		p.stroke(rr(head, 6), lk.line, false)
		p.stroke(fmt.Sprintf("M%g %gV%g", x, head.Bottom(), bottom), s.col("line-0"), true)
		p.draw = 1
		p.opacity = opacity * ap.FillIn
		p.runs(name, x, head.CY()+box.Size*.36, box, lk.text, p.s.col("hi"), render.Middle)
	}

	rows := 0
	for _, it := range sb.Items {
		if it.Kind != "close" {
			rows++
		}
	}
	shown := float64(rows)
	if pos, ok := p.mo.Playhead(e); ok {
		shown = pos * float64(rows)
	} else if _, playing := p.mo.HasPlay(e); playing {
		shown = 0
	}

	type frame struct {
		y      float64
		lo, hi int
		label  string
		elses  []float64
		labels []string
		x0, x1 float64 // extra width needed by notes inside
	}
	var stack []*frame
	y := r.Y + layout.SeqHead
	row, num := 0, 0
	widen := func(a, b int) {
		for _, f := range stack {
			f.lo, f.hi = min(f.lo, a, b), max(f.hi, a, b)
		}
	}
	for _, it := range sb.Items {
		vis := clamp01(shown - float64(row))
		switch it.Kind {
		case "open":
			stack = append(stack, &frame{y: y, lo: len(sb.Participants), hi: -1, label: it.Text})
			y += layout.SeqFrame
			row++
		case "else":
			if len(stack) > 0 {
				f := stack[len(stack)-1]
				f.elses = append(f.elses, y)
				f.labels = append(f.labels, it.Text)
			}
			y += layout.SeqFrame
			row++
		case "close":
			if len(stack) == 0 {
				continue
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if f.hi < 0 {
				f.lo, f.hi = 0, len(sb.Participants)-1
			}
			x0, x1 := xOf(f.lo)-72, xOf(f.hi)+72
			if f.x1 > f.x0 {
				x0, x1 = math.Min(x0, f.x0-12), math.Max(x1, f.x1+12)
			}
			fr := layout.Rect{X: x0, Y: f.y + 4, W: x1 - x0, H: y - f.y - 4}
			p.opacity = opacity * ap.FillIn
			// a tab names the frame (loop, alt, opt, par); its condition
			// follows in brackets. Both sit on the background so lifelines
			// stop short of the text instead of running through it.
			kind, cond, _ := strings.Cut(f.label, " ")
			kw := s.Measure.Measure(kind, lab.Font, lab.Size)
			tab := fmt.Sprintf("M%g %gA6 6 0 0 1 %g %gH%gV%gL%g %gH%gZ", x0, fr.Y+6, x0+6, fr.Y, x0+kw+20, fr.Y+14, x0+kw+14, fr.Y+21, x0)
			p.fill(tab, s.col("bg"))
			p.fill(tab, alpha(s.col("hi"), .06))
			p.stroke(rr(fr, 6), s.col("line-1"), false)
			p.stroke(tab, s.col("line-1"), false)
			p.text(kind, x0+10, fr.Y+15, lab, s.col("ink-2"), render.Start)
			plate := func(text string, x, base float64) {
				w := s.Measure.Measure(text, lab.Font, lab.Size)
				p.fill(rr(layout.Rect{X: x - 6, Y: base - lab.Size, W: w + 12, H: lab.Size + 6}, 3), s.col("bg"))
				p.text(text, x, base, lab, s.col("ink-3"), render.Start)
			}
			if cond = strings.TrimSpace(cond); cond != "" {
				plate("["+cond+"]", x0+kw+30, fr.Y+15)
			}
			for k, ey := range f.elses {
				p.stroke(fmt.Sprintf("M%g %gH%g", x0, ey+4, x1), s.col("line-1"), true)
				if t := strings.TrimSpace(f.labels[k]); t != "" {
					plate("["+t+"]", x0+10, ey+21)
				}
			}
			if len(stack) > 0 {
				widen(f.lo, f.hi)
				up := stack[len(stack)-1]
				if up.x1 <= up.x0 {
					up.x0, up.x1 = x0, x1
				} else {
					up.x0, up.x1 = math.Min(up.x0, x0), math.Max(up.x1, x1)
				}
			}
		case "note":
			widen(it.From, it.To)
			if vis > 0 {
				x0, x1 := xOf(it.From)-70, xOf(it.To)+70
				if tw := s.Measure.Measure(it.Text, lab.Font, lab.Size) + 28; tw > x1-x0 {
					mid := (x0 + x1) / 2
					x0, x1 = mid-tw/2, mid+tw/2
				}
				for _, f := range stack {
					if f.x1 <= f.x0 {
						f.x0, f.x1 = x0, x1
					} else {
						f.x0, f.x1 = math.Min(f.x0, x0), math.Max(f.x1, x1)
					}
				}
				nb := layout.Rect{X: x0, Y: y + 8, W: x1 - x0, H: layout.SeqRow - 14}
				p.opacity = opacity * ap.FillIn * vis
				p.fill(rr(nb, 5), s.col("panel"))
				p.stroke(rr(nb, 5), s.col("line-1"), false)
				p.runs(it.Text, nb.CX(), nb.CY()+4, lab, s.col("ink"), s.col("hi"), render.Middle)
			}
			y += layout.SeqRow
			row++
		case "msg":
			widen(it.From, it.To)
			num++
			if vis > 0 {
				p.opacity = opacity * ap.FillIn
				cl := s.col("line-1")
				if vis < 1 {
					cl = s.col("line-2")
				}
				ay := y + layout.SeqRow - 12
				x0, x1 := xOf(it.From), xOf(it.To)
				text := it.Text
				if sb.Numbered {
					text = strconv.Itoa(num) + ". " + text
				}
				p.draw = vis
				if it.From == it.To {
					// out, down and back to the same lifeline; the label
					// sits beside the loop, centred on it
					d := fmt.Sprintf("M%g %gH%gV%gH%g", x0+4, ay-18, x0+36, ay, x0+4)
					p.stroke(d, cl, it.Dashed)
					p.draw = 1
					if vis >= 1 && it.Arrow {
						p.chevron([2]float64{x0 + 36, ay}, [2]float64{x0 + 4, ay}, cl)
					}
					p.opacity = opacity * ap.FillIn * clamp01(vis*2-1)
					p.runs(text, x0+48, ay-9+lab.Size*.36, lab, s.col("ink"), s.col("hi"), render.Start)
				} else {
					dir := 1.0
					if x1 < x0 {
						dir = -1
					}
					end := x1 - dir*4
					p.stroke(fmt.Sprintf("M%g %gH%g", x0+dir*4, ay, end), cl, it.Dashed)
					p.draw = 1
					if vis >= 1 {
						if it.Cross {
							p.stroke(fmt.Sprintf("M%g %gl8 8M%g %gl-8 8", end-4, ay-4, end+4, ay-4), cl, false)
						} else if it.Arrow {
							p.chevron([2]float64{x0, ay}, [2]float64{end, ay}, cl)
						}
					}
					p.opacity = opacity * ap.FillIn * clamp01(vis*2-1)
					p.runs(text, (x0+x1)/2, ay-9, lab, s.col("ink"), s.col("hi"), render.Middle)
				}
				p.draw = 1
			}
			y += layout.SeqRow
			row++
		}
	}
	p.opacity = opacity
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// gitBlock draws a git history between two of its states: the one after
// the commands played so far and the next, so commits draw in, rebased
// copies glide to their new place while the old ones fade, branch labels
// slide to their new heads, and a fast-forward recolours commits as they
// join their new lane.
func (p *painter) gitBlock(e *c.Element, r layout.Rect, ap motion.Appearance) {
	states := e.Git.States()
	n := len(e.Git.Cmds)
	shown := float64(n)
	if pos, ok := p.mo.Playhead(e); ok {
		shown = pos * float64(n)
	} else if _, pending := p.mo.HasPlay(e); pending {
		shown = 0
	}
	k := int(math.Floor(shown + 1e-9))
	frac := shown - float64(k)
	if k >= n {
		k, frac = n, 0
	}
	a, b := states[k], states[min(k+1, n)]
	f := motion.Ease(frac)
	if k == n {
		f = 1
	}
	base := p.opacity
	palette := []color.NRGBA{p.s.toneCol("info"), p.s.col("#A78BFA"), p.s.toneCol("good"), p.s.toneCol("warn"), p.s.col("#F08AB8")}
	laneCol := func(i int) color.NRGBA { return palette[i%len(palette)] }
	// centre the widest the history gets in the block's box
	cols := 0
	for _, st := range states {
		for _, cm := range st.Commits {
			cols = max(cols, cm.Col+1)
		}
	}
	x0 := r.CX() - float64(max(cols-1, 0))*layout.GitCol/2
	at := func(cm *c.GitCommit) (float64, float64) {
		return x0 + float64(cm.Col)*layout.GitCol, r.Y + layout.GitTop + float64(cm.Lane)*layout.GitLane
	}
	type drawn struct {
		x, y, appear, alpha float64
		col                 color.NRGBA
		cm                  *c.GitCommit
	}
	pos := map[string]*drawn{}
	var order []*drawn
	for _, cb := range b.Commits {
		bx, by := at(cb)
		d := &drawn{x: bx, y: by, appear: 1, alpha: 1, col: laneCol(cb.Lane), cm: cb}
		ghost := func(g bool) float64 {
			if g {
				return .3
			}
			return 1
		}
		if ca := a.Find(cb.ID); ca != nil {
			ax, ay := at(ca)
			d.x, d.y = ax+(bx-ax)*f, ay+(by-ay)*f
			d.col = lerpColor(laneCol(ca.Lane), laneCol(cb.Lane), f)
			d.alpha = ghost(ca.Ghost) + (ghost(cb.Ghost)-ghost(ca.Ghost))*f
		} else {
			d.appear = f
			d.alpha = ghost(cb.Ghost)
		}
		pos[cb.ID] = d
		order = append(order, d)
	}
	for _, ca := range a.Commits {
		if _, ok := pos[ca.ID]; !ok { // a ghost leaving
			ax, ay := at(ca)
			d := &drawn{x: ax, y: ay, appear: 1, alpha: .3 * (1 - f), col: laneCol(ca.Lane), cm: ca}
			pos[ca.ID] = d
			order = append(order, d)
		}
	}
	// links first, under the dots
	for _, d := range order {
		for i, pid := range d.cm.Parents {
			pd, ok := pos[pid]
			if !ok {
				continue
			}
			cl := d.col
			if i == 1 || pd.cm.Lane > d.cm.Lane {
				cl = pd.col // a line coming up into a lane keeps its branch's colour
			}
			var path string
			if math.Abs(pd.y-d.y) < .5 {
				path = fmt.Sprintf("M%g %gL%g %g", pd.x, pd.y, d.x, d.y)
			} else {
				mx := (pd.x + d.x) / 2
				path = fmt.Sprintf("M%g %gC%g %g %g %g %g %g", pd.x, pd.y, mx, pd.y, mx, d.y, d.x, d.y)
			}
			p.opacity = base * math.Min(d.alpha, pd.alpha)
			p.draw = math.Min(1, d.appear*1.6)
			p.strokeW(path, cl, d.cm.Ghost, 2.6)
		}
	}
	lab := p.opt.Type["label"]
	p.draw = 1
	for _, d := range order {
		s := 1.0
		if d.appear < 1 {
			s = outBack(math.Max(0, (d.appear-.35)/.65))
		}
		if s <= 0.01 {
			continue
		}
		p.opacity = base * d.alpha
		if d.cm.Merge && len(d.cm.Parents) > 1 {
			if pd, ok := pos[d.cm.Parents[1]]; ok {
				p.strokeW(circle(d.x, d.y, 16*s), pd.col, false, 2.4)
			}
		}
		p.fill(circle(d.x, d.y, 13*s), p.s.col("bg"))
		p.fill(circle(d.x, d.y, 10*s), d.col)
		p.opacity = base * d.alpha * math.Min(1, s)
		p.runs(d.cm.ID, d.x, d.y+36, lab, p.s.col("ink-dim"), p.s.col("hi"), render.Middle)
	}
	// branch labels, side by side when they point at the same commit
	headAt := func(st c.GitState, br string) *drawn {
		if id, ok := st.Heads[br]; ok {
			return pos[id]
		}
		return nil
	}
	type tag struct {
		name             string
		x, y, w, alpha   float64
		col              color.NRGBA
		current, hasHead bool
	}
	groups := map[string][]*tag{}
	var gkeys []string
	for li, br := range b.Lanes {
		db := headAt(b, br)
		if db == nil {
			continue
		}
		t := &tag{name: br, x: db.x, y: db.y, alpha: 1, col: laneCol(li), current: br == b.Current}
		if da := headAt(a, br); da != nil {
			t.x, t.y = da.x+(db.x-da.x)*f, da.y+(db.y-da.y)*f
		} else {
			t.alpha = f
		}
		t.w = p.s.Measure.Measure(br, lab.Font, lab.Size) + 18
		key := b.Heads[br]
		if _, ok := groups[key]; !ok {
			gkeys = append(gkeys, key)
		}
		groups[key] = append(groups[key], t)
	}
	for _, key := range gkeys {
		ts := groups[key]
		total := -6.0
		for _, t := range ts {
			total += t.w + 6
		}
		off := -total / 2
		for _, t := range ts {
			cx, cy := t.x+off+t.w/2, t.y-36
			off += t.w + 6
			box := layout.Rect{X: cx - t.w/2, Y: cy - 10, W: t.w, H: 20}
			p.opacity = base * t.alpha
			p.fill(rr(box, 10), p.s.col("bg"))
			p.strokeW(rr(box, 10), t.col, false, 1.2)
			p.runs(t.name, cx, cy+lab.Size*.36, lab, t.col, t.col, render.Middle)
		}
	}
	// the command being run
	line := ""
	if k < n && frac > 0 {
		line = e.Git.Cmds[k].Line
	} else if k > 0 {
		line = e.Git.Cmds[k-1].Line
	}
	if line != "" {
		code := p.opt.Type["code"]
		code.Size = 15
		p.opacity = base
		p.runs("$ "+line, x0-10, r.Bottom()-10, code, p.s.col("ink-dim"), p.s.col("hi"), render.Start)
	}
	p.opacity = base
}

func outBack(f float64) float64 {
	f = math.Max(0, math.Min(1, f))
	return 1 + 2.2*math.Pow(f-1, 3) + 1.2*math.Pow(f-1, 2)
}
