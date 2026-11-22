package plot

import (
	"math"
	"sort"
	"strconv"
)

// Ranges are the x and y extents a plot shows or zooms to, in plot units.
type Ranges struct{ X0, X1, Y0, Y1 float64 }

// View is what a plot shows: the plot point at the centre of its box and
// a magnification per axis against the plot's own ranges (1 shows them).
// A view does not depend on the size of the box, so a box that grows or
// moves (an isolated plot) keeps its view and only its scale changes.
type View struct{ CX, CY, MX, MY float64 }

// Frame is a plot's own ranges and its box, in pixels.
type Frame struct {
	Base   Ranges
	Square bool    // one unit is as long on both axes
	W, H   float64 // the box the axes span
}

// Home is the view that shows the plot's own ranges.
func (f Frame) Home() View {
	return View{(f.Base.X0 + f.Base.X1) / 2, (f.Base.Y0 + f.Base.Y1) / 2, 1, 1}
}

// fit is the scale, in pixels per unit, that fits ranges r in the box.
func (f Frame) fit(r Ranges) (kx, ky float64) {
	kx, ky = f.W/(r.X1-r.X0), f.H/(r.Y1-r.Y0)
	if f.Square {
		k := math.Min(kx, ky)
		return k, k
	}
	return kx, ky
}

// Scale is a view's scale in pixels per unit on each axis.
func (f Frame) Scale(v View) (kx, ky float64) {
	kx, ky = f.fit(f.Base)
	return kx * v.MX, ky * v.MY
}

// Target is the view that shows ranges r (nil for the plot's own).
func (f Frame) Target(r *Ranges) View {
	if r == nil {
		return f.Home()
	}
	bx, by := f.fit(f.Base)
	zx, zy := f.fit(*r)
	return View{(r.X0 + r.X1) / 2, (r.Y0 + r.Y1) / 2, zx / bx, zy / by}
}

// Screen maps a plot point to pixels from the box's centre (y down).
func (f Frame) Screen(v View, x, y float64) (float64, float64) {
	kx, ky := f.Scale(v)
	return (x - v.CX) * kx, -(y - v.CY) * ky
}

// Visible is the part of the plane a view shows in the box.
func (f Frame) Visible(v View) Ranges {
	kx, ky := f.Scale(v)
	hw, hh := f.W/2/kx, f.H/2/ky
	return Ranges{v.CX - hw, v.CX + hw, v.CY - hh, v.CY + hh}
}

// Lerp moves from view a toward b, s of the way (0 to 1, already eased).
// The scale moves on a log scale, so each moment zooms by the same
// factor; the centre moves so the point that stays still on screen stays
// exactly still, and b's centre travels in a straight line to the middle
// of the box: w = (1 - k0/k) / (1 - k0/kZ).
func Lerp(a, b View, s float64) View {
	axis := func(c0, c1, m0, m1 float64) (float64, float64) {
		m := m0 * math.Pow(m1/m0, s)
		w := s
		if r := m0 / m1; math.Abs(1-r) > 1e-9 {
			w = (1 - m0/m) / (1 - r)
		}
		return c0 + (c1-c0)*w, m
	}
	cx, mx := axis(a.CX, b.CX, a.MX, b.MX)
	cy, my := axis(a.CY, b.CY, a.MY, b.MY)
	if a.MX/b.MX != a.MY/b.MY && math.Abs(a.MX/b.MX-a.MY/b.MY) > 1e-9 {
		// axes zooming by different factors: move both centres by the
		// same share, so the view still travels in a straight line
		w := s
		if r := a.MX / b.MX; math.Abs(1-r) > 1e-9 {
			w = (1 - a.MX/mx) / (1 - r)
		}
		cx, cy = a.CX+(b.CX-a.CX)*w, a.CY+(b.CY-a.CY)*w
	}
	return View{cx, cy, mx, my}
}

// Zoom is one change of view: to Ranges (nil goes back to the plot's own)
// from Start over Dur seconds.
type Zoom struct {
	To         *Ranges
	Start, Dur float64
}

// Smootherstep is 6x⁵ - 15x⁴ + 10x³, the ease camera moves use: it starts
// and stops with no jolt in speed or acceleration.
func Smootherstep(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	return x * x * x * (x*(x*6-15) + 10)
}

// ViewAt is the view at time t after the zooms, which are in time order.
// A zoom that starts while another runs carries on from wherever that one
// had got to, so the view never jumps.
func (f Frame) ViewAt(zooms []Zoom, t float64) View {
	var at func(i int, t float64) View
	at = func(i int, t float64) View {
		for i >= 0 && t < zooms[i].Start {
			i--
		}
		if i < 0 {
			return f.Home()
		}
		z := zooms[i]
		from := at(i-1, z.Start)
		s := 1.0
		if z.Dur > 0 {
			s = Smootherstep((t - z.Start) / z.Dur)
		}
		return Lerp(from, f.Target(z.To), s)
	}
	return at(len(zooms)-1, t)
}

// Tick is one tick on an axis: its value, how strongly the tick shows and
// how strongly its label shows (0 to 1), and the label.
type Tick struct {
	V            float64
	Alpha, Label float64
	Text         string
}

// tickSteps lists candidate steps from coarse to fine.
func tickSteps(pi bool) []float64 {
	var out []float64
	if pi {
		for e := 6; e >= -6; e-- {
			out = append(out, math.Pi*math.Pow(2, float64(e)))
		}
		return out
	}
	for e := 6; e >= -6; e-- {
		p := math.Pow(10, float64(e))
		out = append(out, 5*p, 2*p, p)
	}
	return out
}

// stepAlpha is how strongly ticks spaced step apart show at k pixels per
// unit: none below 34 px apart, fully from 70.
func stepAlpha(step, k float64) float64 {
	return math.Max(0, math.Min(1, (step*k-34)/36))
}

// labelAlpha is the same for labels, which need room for their text.
func labelAlpha(step, k, room float64) float64 {
	return math.Max(0, math.Min(1, (step*k-room)/36))
}

// Ticks lists ticks for an axis showing lo to hi at k pixels per unit.
// Every candidate step has an alpha from its spacing; a tick's alpha is
// the largest of the steps it is a multiple of, so as room opens finer
// ticks fade in between the ones already there instead of popping. room
// is the spacing in pixels a label needs; pi labels in multiples of π.
func Ticks(lo, hi, k, room float64, pi bool) []Tick {
	steps := tickSteps(pi)
	finest := -1
	for i, s := range steps {
		if stepAlpha(s, k) > 0 {
			finest = i
		}
	}
	if finest < 0 {
		return nil
	}
	fs := steps[finest]
	// every multiple of every step that shows, once each, in order
	seen := map[int64]bool{}
	var vals []float64
	for _, s := range steps[:finest+1] {
		if stepAlpha(s, k) <= 0 {
			continue
		}
		for n := math.Ceil(lo/s - 1e-9); n*s <= hi+1e-9*s; n++ {
			v := n * s
			key := int64(math.Round(v / fs * 64))
			if !seen[key] {
				seen[key] = true
				vals = append(vals, v)
			}
		}
	}
	sort.Float64s(vals)
	var out []Tick
	for _, v := range vals {
		var a, la float64
		for _, s := range steps[:finest+1] {
			q := v / s
			if math.Abs(q-math.Round(q)) < 1e-6 && (math.Round(q) != 0 || v == 0) {
				a = math.Max(a, stepAlpha(s, k))
				if pi || !twoStep(s) {
					la = math.Max(la, labelAlpha(s, k, room))
				}
			}
		}
		if a <= 0 {
			continue
		}
		out = append(out, Tick{V: v, Alpha: a, Label: la, Text: tickText(v, fs, pi)})
	}
	return out
}

// twoStep reports whether a decimal step is 2·10ⁿ. Labels skip those
// steps: 0.2 is not a multiple of 0.5, so labelling both would leave an
// uneven row of numbers (0.4 0.5 0.6) while one fades into the other.
// Labels follow 1, 0.5, 0.1, 0.05 …, each a multiple of the next.
func twoStep(s float64) bool {
	m := s / math.Pow(10, math.Floor(math.Log10(s)+1e-9))
	return math.Abs(m-2) < 1e-6
}

// tickText writes a tick's value: decimals as fine as the step needs, or
// a multiple of π as π, 2π, π/2, 3π/4.
func tickText(v, step float64, pi bool) string {
	if math.Abs(v) < step*1e-6 {
		return "0"
	}
	if pi {
		q := v / math.Pi
		for _, d := range []int{1, 2, 4, 8, 16, 32, 64} {
			n := q * float64(d)
			if math.Abs(n-math.Round(n)) < 1e-6 {
				num := int(math.Round(n))
				s := ""
				switch num {
				case 1:
					s = "π"
				case -1:
					s = "-π"
				default:
					s = strconv.Itoa(num) + "π"
				}
				if d > 1 {
					s += "/" + strconv.Itoa(d)
				}
				return s
			}
		}
	}
	dec := int(math.Max(0, math.Ceil(-math.Log10(step)-1e-9)))
	p := math.Pow(10, float64(dec))
	return strconv.FormatFloat(math.Round(v*p)/p, 'f', -1, 64)
}
