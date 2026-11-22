package plot

import (
	"math"
	"sort"
	"strconv"
)

// Piece is one unbroken run of a sampled curve: points in screen pixels,
// and the parameter each was sampled at.
type Piece struct {
	Pts [][2]float64
	Ts  []float64
}

// Box is a screen rectangle curves are clipped to.
type Box struct{ X0, Y0, X1, Y1 float64 }

// Sampling limits: the first even pass, how deep refinement goes, the
// longest segment and the most a midpoint may stray from its chord.
const (
	firstPass = 256
	maxDepth  = 10
	maxSeg    = 6.0  // px
	maxDev    = 0.35 // px
	jump      = 40.0 // px: a segment still this long at full depth is a break
	maxPoints = 40000
)

// Sample samples a curve for t in [t0, t1]. at returns the screen point
// for a parameter and whether it exists there. Refinement is adaptive:
// segments that are long or bend away from their chord are split, and a
// segment that never settles (an asymptote, a jump) or touches a place
// where the curve is undefined breaks the curve into pieces.
func Sample(at func(t float64) (float64, float64, bool), t0, t1 float64, box Box) []Piece {
	type pt struct {
		t, x, y float64
		ok      bool
	}
	eval := func(t float64) pt {
		x, y, ok := at(t)
		ok = ok && !math.IsNaN(x) && !math.IsNaN(y) && !math.IsInf(x, 0) && !math.IsInf(y, 0)
		return pt{t, x, y, ok}
	}
	var out []Piece
	var cur *Piece
	count := 0
	emit := func(p pt) {
		if !p.ok {
			cur = nil
			return
		}
		if cur == nil {
			out = append(out, Piece{})
			cur = &out[len(out)-1]
		}
		cur.Pts = append(cur.Pts, [2]float64{p.x, p.y})
		cur.Ts = append(cur.Ts, p.t)
		count++
	}
	brk := func() { cur = nil }
	// outside reports whether a segment lies wholly beyond one side of the
	// box, with a margin, so there is nothing on screen to refine
	margin := 40.0
	outside := func(a, b pt) bool {
		return a.x < box.X0-margin && b.x < box.X0-margin || a.x > box.X1+margin && b.x > box.X1+margin ||
			a.y < box.Y0-margin && b.y < box.Y0-margin || a.y > box.Y1+margin && b.y > box.Y1+margin
	}
	var refine func(a, b pt, depth int)
	refine = func(a, b pt, depth int) {
		if count > maxPoints {
			emit(b)
			return
		}
		if !a.ok || !b.ok {
			if a.ok != b.ok && depth < maxDepth {
				m := eval((a.t + b.t) / 2)
				refine(a, m, depth+1)
				refine(m, b, depth+1)
				return
			}
			emit(b)
			return
		}
		if outside(a, b) {
			// far off screen: keep the shape coarse, but never join across
			// a huge jump (tan going from +inf to -inf)
			if math.Abs(a.y-b.y) > 4*(box.Y1-box.Y0)+4*jump && depth < maxDepth {
				m := eval((a.t + b.t) / 2)
				refine(a, m, depth+1)
				refine(m, b, depth+1)
				return
			}
			emit(b)
			return
		}
		L := math.Hypot(b.x-a.x, b.y-a.y)
		m := eval((a.t + b.t) / 2)
		dev := math.Inf(1)
		if m.ok {
			dev = math.Hypot(m.x-(a.x+b.x)/2, m.y-(a.y+b.y)/2)
		}
		if depth < maxDepth && (L > maxSeg || dev > maxDev) {
			refine(a, m, depth+1)
			refine(m, b, depth+1)
			return
		}
		if L > jump || !m.ok {
			brk()
		}
		emit(b)
	}
	prev := eval(t0)
	emit(prev)
	for i := 1; i <= firstPass; i++ {
		p := eval(t0 + (t1-t0)*float64(i)/firstPass)
		refine(prev, p, 0)
		prev = p
	}
	return out
}

// Cut keeps the part of a sampled curve with t up to tEnd, ending exactly
// on the curve at tEnd (end is that point's screen position).
func Cut(pieces []Piece, tEnd float64, end [2]float64, endOK bool) []Piece {
	var out []Piece
	for _, pc := range pieces {
		if len(pc.Ts) == 0 || pc.Ts[0] > tEnd {
			break
		}
		k := sort.SearchFloat64s(pc.Ts, tEnd) // first t >= tEnd
		if k >= len(pc.Ts) {
			out = append(out, pc)
			continue
		}
		cut := Piece{Pts: pc.Pts[:k:k], Ts: pc.Ts[:k:k]}
		if endOK {
			cut.Pts = append(cut.Pts, end)
			cut.Ts = append(cut.Ts, tEnd)
		}
		if len(cut.Pts) > 0 {
			out = append(out, cut)
		}
		break
	}
	return out
}

// Clip cuts polylines to the box, splitting them where they leave it.
func Clip(pieces []Piece, b Box) [][][2]float64 {
	var out [][][2]float64
	for _, pc := range pieces {
		var run [][2]float64
		flush := func() {
			if len(run) > 1 {
				out = append(out, run)
			}
			run = nil
		}
		for i := 1; i < len(pc.Pts); i++ {
			p, q, ok := clipSeg(pc.Pts[i-1], pc.Pts[i], b)
			if !ok {
				flush()
				continue
			}
			if len(run) == 0 || run[len(run)-1] != p {
				flush()
				run = append(run, p)
			}
			run = append(run, q)
			if q != pc.Pts[i] { // left the box
				flush()
			}
		}
		if len(pc.Pts) == 1 && inside(pc.Pts[0], b) {
			run = append(run, pc.Pts[0], pc.Pts[0])
		}
		flush()
	}
	return out
}

func inside(p [2]float64, b Box) bool {
	return p[0] >= b.X0 && p[0] <= b.X1 && p[1] >= b.Y0 && p[1] <= b.Y1
}

// clipSeg is Liang–Barsky: the part of segment p-q inside the box.
func clipSeg(p, q [2]float64, b Box) ([2]float64, [2]float64, bool) {
	t0, t1 := 0.0, 1.0
	dx, dy := q[0]-p[0], q[1]-p[1]
	for _, c := range [4][2]float64{{-dx, p[0] - b.X0}, {dx, b.X1 - p[0]}, {-dy, p[1] - b.Y0}, {dy, b.Y1 - p[1]}} {
		pp, qq := c[0], c[1]
		if pp == 0 {
			if qq < 0 {
				return p, q, false
			}
			continue
		}
		r := qq / pp
		if pp < 0 {
			t0 = math.Max(t0, r)
		} else {
			t1 = math.Min(t1, r)
		}
		if t0 > t1 {
			return p, q, false
		}
	}
	a, z := p, q
	if t0 > 0 {
		a = [2]float64{p[0] + dx*t0, p[1] + dy*t0}
	}
	if t1 < 1 {
		z = [2]float64{p[0] + dx*t1, p[1] + dy*t1}
	}
	return a, z, true
}

// Path writes polylines as SVG path data with two decimals.
func Path(runs [][][2]float64) string {
	var b []byte
	for _, run := range runs {
		for i, p := range run {
			if i == 0 {
				b = append(b, 'M')
			} else {
				b = append(b, 'L')
			}
			b = strconv.AppendFloat(b, math.Round(p[0]*100)/100, 'f', -1, 64)
			b = append(b, ' ')
			b = strconv.AppendFloat(b, math.Round(p[1]*100)/100, 'f', -1, 64)
		}
	}
	return string(b)
}

// Zeros finds where dist(t), a curve's distance from what it should touch
// (the origin, or the x axis), falls to zero for t in [t0, t1]: local
// minima that refine to practically nothing. They come back in order.
func Zeros(dist func(t float64) float64, t0, t1 float64) []float64 {
	const n = 3000
	vals := make([]float64, n+1)
	ts := make([]float64, n+1)
	var scale float64
	for i := range vals {
		ts[i] = t0 + (t1-t0)*float64(i)/n
		vals[i] = math.Abs(dist(ts[i]))
		if !math.IsNaN(vals[i]) && !math.IsInf(vals[i], 0) {
			scale = math.Max(scale, vals[i])
		}
	}
	tol := 1e-5 * math.Max(1, scale)
	var out []float64
	for i := 1; i < n; i++ {
		if !(vals[i] <= vals[i-1] && vals[i] <= vals[i+1]) {
			continue
		}
		// golden-section search for the minimum in the neighbouring cells
		a, b := ts[i-1], ts[i+1]
		g := (math.Sqrt(5) - 1) / 2
		c, d := b-g*(b-a), a+g*(b-a)
		fc, fd := math.Abs(dist(c)), math.Abs(dist(d))
		for it := 0; it < 60; it++ {
			if fc < fd {
				b, d, fd = d, c, fc
				c = b - g*(b-a)
				fc = math.Abs(dist(c))
			} else {
				a, c, fc = c, d, fd
				d = a + g*(b-a)
				fd = math.Abs(dist(d))
			}
		}
		t := (a + b) / 2
		if math.Abs(dist(t)) < tol {
			if len(out) == 0 || t-out[len(out)-1] > (t1-t0)/n {
				out = append(out, t)
			}
		}
	}
	return out
}

// Roots finds where y(t) changes sign, by bisection, for t in [t0, t1].
func Roots(y func(t float64) float64, t0, t1 float64) []float64 {
	const n = 2000
	var out []float64
	prevT, prevY := t0, y(t0)
	if prevY == 0 {
		out = append(out, t0)
	}
	for i := 1; i <= n; i++ {
		t := t0 + (t1-t0)*float64(i)/n
		v := y(t)
		if v == 0 {
			out = append(out, t)
		} else if prevY != 0 && !math.IsNaN(v) && !math.IsNaN(prevY) && (v > 0) != (prevY > 0) {
			a, b, fa := prevT, t, prevY
			for it := 0; it < 60; it++ {
				m := (a + b) / 2
				fm := y(m)
				if (fm > 0) == (fa > 0) {
					a, fa = m, fm
				} else {
					b = m
				}
			}
			// a sign change through a pole is not a root
			if r := (a + b) / 2; math.Abs(y(r)) < 1e-6*math.Max(1, math.Abs(prevY)+math.Abs(v)) {
				out = append(out, r)
			}
		}
		prevT, prevY = t, v
	}
	return out
}
