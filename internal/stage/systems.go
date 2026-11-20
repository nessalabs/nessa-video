package stage

import (
	"fmt"
	"image/color"
	"math"
	"strconv"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/layout"
	"github.com/nessalabs/nessa-video/internal/markup"
	"github.com/nessalabs/nessa-video/internal/motion"
	"github.com/nessalabs/nessa-video/internal/render"
)

// This file draws the parts of system diagrams: connectors with a request
// lane and a reply lane, labelled request cards, status lights with an
// inline countdown ring, dot meters, and colours that change smoothly.

// LaneOffset is how far each lane of a two-lane connector sits from the
// connector's centre line, in design units.
const LaneOffset = 11.0

// Card sizes: a request card's height and its side padding.
const cardH, cardPad = 22.0, 12.0

// anchorOf is the part of e that connectors meet: a part marked
// anchor=true, or e itself.
func anchorOf(e *c.Element) *c.Element {
	var out *c.Element
	e.Walk(func(x *c.Element) {
		if out == nil && x != e && x.Edge == nil {
			if v, ok := x.Computed["anchor"]; ok && v.Value == "true" {
				out = x
			}
		}
	})
	if out == nil {
		return e
	}
	return out
}

// offsetPath moves a polyline sideways by d, to the right of its direction
// of travel (below a line that runs left to right), keeping its corners
// square.
func offsetPath(pts [][2]float64, d float64) [][2]float64 {
	n := len(pts)
	if n < 2 {
		return pts
	}
	normal := func(a, b [2]float64) (float64, float64) {
		dx, dy := b[0]-a[0], b[1]-a[1]
		l := math.Hypot(dx, dy)
		if l == 0 {
			return 0, 0
		}
		return -dy / l, dx / l
	}
	out := make([][2]float64, n)
	for i := range pts {
		var nx, ny float64
		switch {
		case i == 0:
			nx, ny = normal(pts[0], pts[1])
		case i == n-1:
			nx, ny = normal(pts[n-2], pts[n-1])
		default:
			ax, ay := normal(pts[i-1], pts[i])
			bx, by := normal(pts[i], pts[i+1])
			k := 1 + ax*bx + ay*by
			if k < 0.1 {
				k = 0.1
			}
			nx, ny = (ax+bx)/k, (ay+by)/k
		}
		out[i] = [2]float64{pts[i][0] + nx*d, pts[i][1] + ny*d}
	}
	return out
}

func reversed(pts [][2]float64) [][2]float64 {
	out := make([][2]float64, len(pts))
	for i, p := range pts {
		out[len(pts)-1-i] = p
	}
	return out
}

func polyline(pts [][2]float64) string {
	d := ""
	for i, pt := range pts {
		if i == 0 {
			d += fmt.Sprintf("M%g %g", pt[0], pt[1])
		} else {
			d += fmt.Sprintf("L%g %g", pt[0], pt[1])
		}
	}
	return d
}

// pointAt is the point a share p of the way along a polyline.
func pointAt(pts [][2]float64, p float64) (float64, float64) {
	var total float64
	for i := 0; i+1 < len(pts); i++ {
		total += math.Hypot(pts[i+1][0]-pts[i][0], pts[i+1][1]-pts[i][1])
	}
	want := total * p
	for i := 0; i+1 < len(pts); i++ {
		seg := math.Hypot(pts[i+1][0]-pts[i][0], pts[i+1][1]-pts[i][1])
		if want <= seg || i+2 == len(pts) {
			f := 0.0
			if seg > 0 {
				f = math.Min(1, want/seg)
			}
			return pts[i][0] + (pts[i+1][0]-pts[i][0])*f, pts[i][1] + (pts[i+1][1]-pts[i][1])*f
		}
		want -= seg
	}
	return pts[0][0], pts[0][1]
}

// toneOf blends what tone (or outline) gives a colour: base where e has
// none, the tone's colour where it has one. dim keeps a tone at the
// brightness of base, as toned text is.
func (p *painter) toneOf(e *c.Element, key string, base color.NRGBA, dim bool) (color.NRGBA, string) {
	from, to, mix := p.mo.Tone(e, key)
	col := func(t string) color.NRGBA {
		if t == "" {
			return base
		}
		tt, ok := markup.Tones[t]
		if !ok {
			tt = t
		}
		tc := p.s.toneCol(tt)
		if dim {
			return p.s.dimLike(tc, base)
		}
		return tc
	}
	now := to
	if mix < .5 {
		now = from
	}
	if t, ok := markup.Tones[now]; ok {
		now = t
	}
	return lerpColor(col(from), col(to), mix), now
}

// ring draws a countdown ring: a hairline track and an arc for what is left.
func (p *painter) ring(cx, cy, rad, left float64, cl color.NRGBA) {
	p.stroke(circle(cx, cy, rad), p.s.col("line-0"), false)
	if left <= 0.001 {
		return
	}
	if left >= 0.999 {
		p.stroke(circle(cx, cy, rad), cl, false)
		return
	}
	a := 2 * math.Pi * left
	x1, y1 := cx+rad*math.Sin(a), cy-rad*math.Cos(a)
	large := 0
	if left > 0.5 {
		large = 1
	}
	p.stroke(fmt.Sprintf("M%g %gA%g %g 0 %d 1 %g %g", cx, cy-rad, rad, rad, large, x1, y1), cl, false)
}

// status draws a status light: a toned dot and its label, and while a play
// step runs, a small countdown ring beside the label.
func (p *painter) status(e *c.Element, r layout.Rect, lk look, plain color.NRGBA) {
	t := p.opt.Type["label"]
	p.draw = 1
	cy := r.CY()
	dot := lk.line
	if p.mo.Toned(e, "tone") {
		dot, _ = p.toneOf(e, "tone", p.s.col("line-1"), false)
	}
	p.fill(circle(r.X+layout.StatusDot/2, cy, layout.StatusDot/2), dot)
	text, fade := p.textOf(e)
	keep := p.opacity
	p.opacity *= fade
	x := r.X + layout.StatusDot + layout.StatusGap
	p.runs(text, x, cy+t.Size*0.36, t, plain, p.s.col("hi"), render.Start)
	p.opacity = keep
	if left, op, ok := p.mo.Countdown(e); ok && op > 0 {
		// the ring sits after the label as it is now, gliding with it
		w := p.s.Measure.Measure(layout.Plain(text), t.Font, t.Size)
		if old, _ := p.changing(e); old || fade < 1 {
			now := e.Text
			if v, ok := p.lay.Text[e]; ok {
				now = v
			}
			w = p.s.Measure.Measure(layout.Plain(now), t.Font, t.Size)
		}
		rx := x + w + layout.StatusRingGap + layout.StatusRing
		p.opacity = keep * op
		p.ring(rx, cy, layout.StatusRing, left, dot)
		p.opacity = keep
	}
}

// dots draws a dot meter: an outline per slot, filled dots that grow back
// as they refill, and an optional count under it.
func (p *painter) dots(e *c.Element, r layout.Rect, on color.NRGBA, v float64) {
	n := int(math.Max(1, numProp(e, "max", 5)))
	rad := numProp(e, "r", layout.MeterDot)
	p.draw = 1
	for i := 0; i < n; i++ {
		cx, cy := meterDot(e, r, i)
		p.stroke(circle(cx, cy, rad-0.5), p.s.col("line-0"), false)
		f := math.Max(0, math.Min(1, v-float64(i)))
		if f <= 0 {
			continue
		}
		keep := p.opacity
		p.opacity *= math.Min(1, 0.25+f)
		p.fill(circle(cx, cy, rad*(0.4+0.6*f)), on)
		p.opacity = keep
	}
	if u := str(e, "unit"); u != "" {
		t := p.opt.Type["label"]
		count := int(math.Round(v))
		txt := strconv.Itoa(count) + " of " + strconv.Itoa(n) + " " + u
		p.text(txt, r.CX(), r.Bottom()-6, t, p.s.col("ink-3"), render.Middle)
	}
}

// meterDot is the centre of dot i of a dot meter placed at r.
func meterDot(e *c.Element, r layout.Rect, i int) (float64, float64) {
	n := math.Max(1, numProp(e, "max", 5))
	rad := numProp(e, "r", layout.MeterDot)
	w := n*2*rad + (n-1)*layout.MeterDotGap
	x0 := r.CX() - w/2
	return x0 + rad + float64(i)*(2*rad+layout.MeterDotGap), r.Y + rad
}

// card draws a request card centred on x, y: a pill with its text, the
// text cross-fading while it changes, and a token dot at its left once the
// request has taken one.
func (p *painter) card(pk motion.Packet, x, y float64) {
	t := p.opt.Type["label"]
	w := func(s string) float64 { return p.s.Measure.Measure(layout.Plain(s), t.Font, t.Size) }
	tw := w(pk.Text)
	if pk.Was != "" && pk.TextMix < 1 {
		tw = w(pk.Was) + (tw-w(pk.Was))*pk.TextMix
	}
	tok := 0.0
	if pk.Spend != nil {
		tok = pk.SpendFly
	}
	cw := tw + 2*cardPad + 14*tok
	box := layout.Rect{X: x - cw/2, Y: y - cardH/2, W: cw, H: cardH}
	p.draw = 1
	p.fill(rr(box, cardH/2), p.s.col("panel"))
	line := lerpColor(p.s.col("line-1"), p.s.toneCol("bad"), pk.Fail)
	p.stroke(rr(box, cardH/2), line, false)
	tx := x + 7*tok
	base := y + t.Size*0.36
	keep := p.opacity
	if pk.Was != "" && pk.TextMix < 1 {
		p.opacity = keep * (1 - math.Min(1, pk.TextMix*2))
		p.runs(pk.Was, tx, base, t, p.s.col("ink"), p.s.col("hi"), render.Middle)
		p.opacity = keep * math.Max(0, pk.TextMix*2-1)
	}
	p.runs(pk.Text, tx, base, t, p.s.col("ink"), p.s.col("hi"), render.Middle)
	p.opacity = keep
	if pk.Spend != nil && pk.SpendFly >= 1 {
		p.fill(circle(box.X+cardPad+1, y, 3.5), p.meterOn(pk.Spend))
	}
}

// meterOn is the colour a meter's filled slots take.
func (p *painter) meterOn(e *c.Element) color.NRGBA {
	cl, _ := p.toneOf(e, "tone", p.s.col("ink"), false)
	return cl
}

// token draws a token arcing from its meter dot to the card at x, y.
func (p *painter) token(pk motion.Packet, x, y float64) {
	mb, ok := p.lay.Boxes[pk.Spend]
	if !ok || pk.SpendFly <= 0 || pk.SpendFly >= 1 {
		return
	}
	t := p.opt.Type["label"]
	tw := p.s.Measure.Measure(layout.Plain(pk.Text), t.Font, t.Size)
	cw := tw + 2*cardPad + 14*pk.SpendFly
	if pk.Text == "" {
		cw = 2 * cardPad
	}
	dx, dy := x-cw/2+cardPad+1, y
	sx, sy := meterDot(pk.Spend, mb, pk.SpendSlot)
	f := pk.SpendFly
	px := sx + (dx-sx)*f
	py := sy + (dy-sy)*f - math.Sin(f*math.Pi)*26
	rad := numProp(pk.Spend, "r", layout.MeterDot)
	p.draw = 1
	p.fill(circle(px, py, rad+(3.5-rad)*f), p.meterOn(pk.Spend))
}

// trimPath shortens a polyline by a at its start and b at its end, so a
// card riding it stops short of the boxes it joins.
func trimPath(pts [][2]float64, a, b float64) [][2]float64 {
	var total float64
	for i := 0; i+1 < len(pts); i++ {
		total += math.Hypot(pts[i+1][0]-pts[i][0], pts[i+1][1]-pts[i][1])
	}
	if total <= a+b+1 {
		mx, my := pointAt(pts, 0.5)
		return [][2]float64{{mx, my}, {mx, my}}
	}
	x0, y0 := pointAt(pts, a/total)
	x1, y1 := pointAt(pts, 1-b/total)
	out := [][2]float64{{x0, y0}}
	var run float64
	for i := 0; i+1 < len(pts); i++ {
		run += math.Hypot(pts[i+1][0]-pts[i][0], pts[i+1][1]-pts[i][1])
		if run > a && run < total-b && i+2 < len(pts) {
			out = append(out, pts[i+1])
		}
	}
	return append(out, [2]float64{x1, y1})
}

// cardHalf is half the widest a packet's card gets.
func (p *painter) cardHalf(pk motion.Packet) float64 {
	t := p.opt.Type["label"]
	w := p.s.Measure.Measure(layout.Plain(pk.Text), t.Font, t.Size)
	for _, o := range []string{pk.Was, pk.Alt} {
		if o != "" {
			w = math.Max(w, p.s.Measure.Measure(layout.Plain(o), t.Font, t.Size))
		}
	}
	if pk.Spend != nil {
		w += 14
	}
	return w/2 + cardPad
}
