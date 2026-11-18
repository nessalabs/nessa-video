package main

// The M0 storyboard: four shots from the hairline mocks, built directly as
// display lists. It exists only to benchmark rasterizers.

import (
	"fmt"
	"image/color"
	"math"

	. "github.com/nessalabs/nessa-video/internal/render"
)

const (
	W, H     = 1920, 1080
	ShotLen  = 3.0
	NumShots = 4
	FPS      = 30
)

func Duration() float64 { return ShotLen * NumShots }
func Frames() int       { return int(Duration() * FPS) }

var (
	bg   = hex(0x08090A)
	k0   = hex(0x34373F)
	k1   = hex(0x6A6E7A)
	k2   = hex(0xD4D7DE)
	hi   = hex(0xF4F5F7)
	txt  = hex(0xEDEDEF)
	mut  = hex(0x8A8F98)
	mut2 = hex(0x9BA0AB)
	dim  = hex(0x5A5E68)
	none = color.NRGBA{}
)

func hex(v uint32) color.NRGBA {
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}
func a(c color.NRGBA, o float64) color.NRGBA {
	c.A = uint8(math.Round(float64(c.A) * clamp(o)))
	return c
}
func clamp(x float64) float64 { return math.Max(0, math.Min(1, x)) }
func ease(x float64) float64 {
	x = clamp(x)
	if x < .5 {
		return 4 * x * x * x
	}
	return 1 - math.Pow(-2*x+2, 3)/2
}
func easeOut(x float64) float64 { x = clamp(x); return 1 - math.Pow(1-x, 3) }

// p returns eased progress of an animation starting at s lasting d.
func p(t, s, d float64) float64 { return ease((t - s) / d) }

func rr(x, y, w, h, r float64) string {
	return fmt.Sprintf("M%g %gH%gA%g %g 0 0 1 %g %gV%gA%g %g 0 0 1 %g %gH%gA%g %g 0 0 1 %g %gV%gA%g %g 0 0 1 %g %gZ",
		x+r, y, x+w-r, r, r, x+w, y+r, y+h-r, r, r, x+w-r, y+h, x+r, r, r, x, y+h-r, y+r, r, r, x+r, y)
}
func circ(cx, cy, r float64) string {
	return fmt.Sprintf("M%g %gA%g %g 0 1 0 %g %gA%g %g 0 1 0 %g %gZ", cx-r, cy, r, r, cx+r, cy, r, r, cx-r, cy)
}

type b struct{ l *List }

func (b b) stroke(m Affine, d string, c color.NRGBA, draw float64) {
	if c.A == 0 || draw <= 0 {
		return
	}
	b.l.Ops = append(b.l.Ops, Op{Kind: PathOp, M: m, D: d, Stroke: c, StrokeW: 1.2, Draw: draw})
}
func (b b) dashed(m Affine, d string, c color.NRGBA) {
	if c.A == 0 {
		return
	}
	b.l.Ops = append(b.l.Ops, Op{Kind: PathOp, M: m, D: d, Stroke: c, StrokeW: 1.2, Draw: 1, Dash: []float64{4, 5}})
}
func (b b) fill(m Affine, d string, c color.NRGBA) {
	if c.A == 0 {
		return
	}
	b.l.Ops = append(b.l.Ops, Op{Kind: PathOp, M: m, D: d, Fill: c, Draw: 1})
}
func (b b) both(m Affine, d string, s, f color.NRGBA) {
	if s.A == 0 && f.A == 0 {
		return
	}
	b.l.Ops = append(b.l.Ops, Op{Kind: PathOp, M: m, D: d, Stroke: s, StrokeW: 1.2, Fill: f, Draw: 1})
}
func (b b) text(m Affine, x, y float64, s, font string, size float64, an Anchor, c color.NRGBA) {
	if c.A == 0 {
		return
	}
	b.l.Ops = append(b.l.Ops, Op{Kind: TextOp, M: m, Text: s, X: x, Y: y, Font: font, Size: size, Anchor: an, Color: c})
}

// arrow draws a horizontal or elbow line with a chevron head at (hx,hy) pointing in dir (+1 right).
func (b b) arrow(m Affine, d string, hx, hy float64, c color.NRGBA, draw float64) {
	b.stroke(m, d, c, draw)
	if draw >= 0.98 {
		b.stroke(m, fmt.Sprintf("M%g %gL%g %gL%g %g", hx-7, hy-4, hx, hy, hx-7, hy+4), c, 1)
	}
}

// Build returns the display list at time t (seconds).
func Build(t float64) *List {
	l := &List{W: W, H: H, BG: bg}
	shot := int(t / ShotLen)
	if shot >= NumShots {
		shot = NumShots - 1
	}
	lt := t - float64(shot)*ShotLen
	base := Affine{1.5, 0, 0, 1.5, 0, 0} // mocks are drawn at 1280x720
	B := b{l}
	switch shot {
	case 0:
		shot1(B, base, lt)
	case 1:
		shot2(B, base, lt)
	case 2:
		shot3(B, base, lt)
	case 3:
		shot4(B, base, lt)
	}
	names := []string{"establish", "request", "focus", "inspect"}
	caps := []string{"A client and one server", "The request travels to the server", "Zoom in: one process holds both requests", "Open the terminal: both callers got one shared load"}
	B.text(base, 48, 46, fmt.Sprintf("shot %d / 4 · %s", shot+1, names[shot]), "mono", 13, Start, mut)
	B.text(base, 1232, 46, fmt.Sprintf("00:%02d", int(t)), "mono", 13, End, mut)
	B.text(base, 640, 660, caps[shot], "sans400", 20, Middle, a(mut2, p(lt, .2, .4)))
	B.fill(base, rr(48, 694, 1184, 2, 0), a(hex(0xFFFFFF), .08))
	B.fill(base, rr(48, 694, 1184*t/Duration(), 2, 0), k2)
	return l
}

func laptop(B b, m Affine, x float64, d1, d2, d3 float64, c color.NRGBA) {
	B.stroke(m, rr(280+x, 270, 200, 132, 8), c, d1)
	B.stroke(m, rr(292+x, 282, 176, 108, 3), k0, d2)
	B.stroke(m, fmt.Sprintf("M%g 410H%gL%g 426H%gZ", 252+x, 508+x, 494+x, 266+x), c, d3)
}

func server(B b, m Affine, draw, detail float64, unit1 bool) {
	B.stroke(m, rr(820, 250, 160, 180, 8), k1, draw)
	B.stroke(m, "M820 310H980M820 370H980", a(k0, detail), 1)
	for i, y := range []float64{280, 340, 400} {
		c := k1
		if i == 0 && unit1 {
			c = hi
		}
		B.fill(m, circ(840, y, 3), a(c, detail))
		B.stroke(m, fmt.Sprintf("M856 %gH908M944 %gv16M952 %gv16M960 %gv16", y, y-8, y-8, y-8), a(k0, detail), 1)
	}
}

func shot1(B b, m Affine, t float64) {
	laptop(B, m, 0, p(t, 0, 1), p(t, .15, 1), p(t, .25, 1), k1)
	B.text(m, 380, 470, "laptop · web", "mono", 13, Middle, a(mut, p(t, .6, .4)))
	server(B, m, p(t, .4, 1), p(t, .8, .4), false)
	B.text(m, 900, 470, "server · api-1", "mono", 13, Middle, a(mut, p(t, 1, .4)))
}

func shot2(B b, m Affine, t float64) {
	laptop(B, m, 0, 1, 1, 1, k1)
	B.stroke(m, "M310 316h64M310 334h100M310 352h44", k1, 1)
	B.text(m, 380, 470, "laptop · web", "mono", 13, Middle, mut)
	u := p(t, 1.3, .4)
	server(B, m, 1, 1, u > .5)
	B.both(m, rr(826, 256, 148, 48, 5), a(k2, u), a(hex(0xFFFFFF), .04*u))
	B.text(m, 900, 470, "server · api-1", "mono", 13, Middle, mut)
	B.arrow(m, "M516 280H808", 808, 280, k2, p(t, .2, .8))
	B.text(m, 662, 264, "GET /v1/items", "mono", 13, Middle, a(txt, p(t, .4, .4)))
	if t > .3 && t < 1.3 {
		x := 516 + 292*p(t, .3, .9)
		B.fill(m, circ(x, 280, 4), hi)
	}
}

func shot3(B b, m Affine, t float64) {
	B.stroke(m, rr(80, 270, 200, 132, 8), a(k1, .3), 1)
	B.stroke(m, "M52 410H308L294 426H66Z", a(k1, .3), 1)
	B.arrow(m, "M308 336H352V232H396", 396, 232, k2, 1)
	z := 0.55 + 0.45*easeOut(t/.8)
	g := m.Mul(ScaleAt(z, 640, 335))
	o := 0.6 + 0.4*easeOut(t/.8)
	B.stroke(g, rr(400, 150, 480, 370, 14), a(k1, o), 1)
	B.both(g, rr(412, 162, 456, 136, 8), a(k2, o), a(hex(0xFFFFFF), .04))
	B.fill(g, circ(440, 196, 3.5), a(hi, o))
	B.text(g, 458, 202, "api", "sans500", 18, Start, a(txt, o))
	B.text(g, 500, 201, "pid 4121", "mono", 14, Start, a(mut, o))
	B.text(g, 848, 201, "2 in flight", "mono", 14, End, a(txt, o))
	fa, fb, fl := p(t, .9, .4), p(t, 1.2, .4), p(t, 1.7, .4)
	B.stroke(g, rr(458, 226, 84, 44, 6), a(k1, fa), 1)
	B.text(g, 500, 253, "req A", "mono", 14, Middle, a(txt, fa))
	B.stroke(g, rr(554, 226, 84, 44, 6), a(k1, fb), 1)
	B.text(g, 596, 253, "req B", "mono", 14, Middle, a(txt, fb))
	B.arrow(g, "M646 248H690", 690, 248, k2, p(t, 1.4, .4))
	B.stroke(g, rr(696, 226, 152, 44, 6), a(k2, fl), 1)
	B.text(g, 772, 253, "load × 1", "mono", 14, Middle, a(txt, fl))
	B.stroke(g, "M400 310H880M400 415H880", a(k0, o), 1)
	B.fill(g, circ(440, 362, 3.5), a(k1, o))
	B.text(g, 458, 368, "cache", "sans500", 18, Start, a(hex(0xB4B8C2), o))
	B.text(g, 522, 367, "pid 4102", "mono", 14, Start, a(mut, o))
	B.text(g, 848, 367, "running", "mono", 14, End, a(mut, o))
	B.fill(g, circ(440, 467, 3.5), a(k0, o))
	B.text(g, 458, 473, "worker", "sans500", 18, Start, a(k1, o))
	B.text(g, 530, 472, "pid 4130", "mono", 14, Start, a(dim, o))
	B.text(g, 848, 472, "idle", "mono", 14, End, a(dim, o))
	B.text(m, 640, 560, "server · api-1", "mono", 13, Middle, mut)
}

func shot4(B b, m Affine, t float64) {
	g := m.Mul(Translate(240*(1-easeOut(t/.6)), 0))
	B.stroke(g, rr(120, 200, 300, 300, 10), k1, 1)
	B.both(g, rr(130, 210, 280, 110, 6), k2, a(hex(0xFFFFFF), .04))
	B.fill(g, circ(150, 242, 3), hi)
	B.text(g, 166, 247, "api", "sans500", 15, Start, txt)
	B.text(g, 398, 246, "2 in flight", "mono", 13, End, txt)
	B.stroke(g, rr(166, 270, 52, 30, 5), k1, 1)
	B.text(g, 192, 290, "A", "mono", 13, Middle, txt)
	B.stroke(g, rr(226, 270, 52, 30, 5), k1, 1)
	B.text(g, 252, 290, "B", "mono", 13, Middle, txt)
	B.stroke(g, "M120 330H420M120 415H420", k0, 1)
	B.fill(g, circ(150, 372, 3), k1)
	B.text(g, 166, 377, "cache", "sans500", 15, Start, hex(0xB4B8C2))
	B.text(g, 398, 376, "running", "mono", 13, End, mut)
	B.fill(g, circ(150, 457, 3), k0)
	B.text(g, 166, 462, "worker", "sans500", 15, Start, k1)
	B.text(g, 398, 461, "idle", "mono", 13, End, dim)
	B.text(g, 270, 536, "server · api-1", "mono", 13, Middle, mut)
	c := p(t, .4, .4)
	B.dashed(m, "M428 265H634", a(k1, c))
	B.stroke(m, "M633 261L640 265L633 269", a(k1, c), 1)
	B.text(m, 534, 252, "stdout", "mono", 13, Middle, a(mut, c))
	w := p(t, .2, .5)
	tm := m.Mul(Translate(24*(1-w), 0))
	B.both(tm, rr(648, 140, 512, 420, 12), a(k1, w), a(hex(0x0B0C0E), w))
	B.stroke(tm, "M648 184H1160", a(k0, w), 1)
	B.stroke(tm, "M670 156l7 6-7 6M684 168h10", a(k1, w), 1)
	B.text(tm, 1140, 167, "terminal", "mono", 13, End, a(mut, w))
	lines := []struct {
		s string
		c color.NRGBA
		y float64
	}{
		{"$ curl -i api.internal/v1/items", txt, 226},
		{"HTTP/1.1 200 OK", txt, 262},
		{"x-cache: MISS", mut, 292},
		{"x-load:  shared", hi, 322},
		{"content-length: 412", dim, 352},
		{"$ ▍", txt, 404},
	}
	for i, ln := range lines {
		B.text(tm, 672, ln.y, ln.s, "mono", 15, Start, a(ln.c, p(t, .7+float64(i)*.25, .2)))
	}
}
