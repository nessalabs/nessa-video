package plot

import (
	"math"
	"math/cmplx"
	"strings"
	"testing"
)

func eval(t *testing.T, src string, names []string, vals ...complex128) complex128 {
	t.Helper()
	e, err := Compile(src, names...)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return e.Eval(vals)
}

func near(a, b complex128, tol float64) bool { return cmplx.Abs(a-b) <= tol }

func TestParse(t *testing.T) {
	x := []string{"x"}
	cases := []struct {
		src  string
		x    float64
		want complex128
	}{
		{"1 + 2 * 3", 0, 7},
		{"2^3^2", 0, 512},
		{"-x^2", 3, -9},
		{"2x", 4, 8},
		{"3(x+1)", 1, 6},
		{"(x+1)(x-1)", 3, 8},
		{"2pi", 0, complex(2*math.Pi, 0)},
		{"sin(x) * exp(-x/4)", 1, complex(math.Sin(1)*math.Exp(-0.25), 0)},
		{"1/2 + i*x", 2, complex(0.5, 2)},
		{"e^(i*pi)", 0, -1},
		{"max(1, x, 3)", 5, 5},
		{"min(1, x, 3)", -2, -2},
		{"abs(3 + 4i)", 0, 5},
		{"floor(2.7)", 0, 2},
		{"sqrt(-4)", 0, 2i},
		{"ln(e^2)", 0, 2},
		{"x/2/2", 8, 2},
		{"2e-1", 0, 0.2},
		{"2e", 0, complex(2*math.E, 0)},
		{"atan(1)*4", 0, complex(math.Pi, 0)},
		{"x**2", 3, 9},
		{"a*sin(3x)/3", 1, complex(math.Sin(3)/3, 0)},
	}
	for _, c := range cases {
		names, vals := x, []complex128{complex(c.x, 0)}
		if strings.Contains(c.src, "a*") {
			names, vals = []string{"x", "a"}, []complex128{complex(c.x, 0), 1}
		}
		if got := eval(t, c.src, names, vals...); !near(got, c.want, 1e-12) {
			t.Errorf("%s at x=%v = %v, want %v", c.src, c.x, got, c.want)
		}
	}
	for _, bad := range []string{"", "1 +", "sin x", "foo(1)", "y + 1", "(1", "1)", "max()", "sin(1, 2)", "1 $ 2"} {
		if _, err := Compile(bad, "x"); err == nil {
			t.Errorf("%q should not compile", bad)
		}
	}
	if _, err := Compile("y + 1", "x"); err == nil || !strings.Contains(err.Error(), `"y"`) {
		t.Errorf("an unknown name should be named, got %v", err)
	}
	e, _ := Compile("sin(x) + a", "x", "a", "t")
	if !e.Uses("a") || !e.Uses("x") || e.Uses("t") {
		t.Error("Uses should report the variables read")
	}
}

func TestConstAndRange(t *testing.T) {
	lo, hi, err := Range("0..4pi")
	if err != nil || lo != 0 || math.Abs(hi-4*math.Pi) > 1e-12 {
		t.Fatalf("0..4pi = %v %v %v", lo, hi, err)
	}
	lo, hi, err = Range("-.35..0.55")
	if err != nil || lo != -.35 || hi != .55 {
		t.Fatalf("-.35..0.55 = %v %v %v", lo, hi, err)
	}
	if _, _, err := Range("1..0"); err == nil {
		t.Fatal("a backwards range is an error")
	}
	if v, err := Const("1/2"); err != nil || v != .5 {
		t.Fatalf("1/2 = %v %v", v, err)
	}
}

func TestGamma(t *testing.T) {
	for n, want := range map[float64]float64{1: 1, 2: 1, 5: 24, 0.5: math.Sqrt(math.Pi), -0.5: -2 * math.Sqrt(math.Pi)} {
		if got := Gamma(complex(n, 0)); !near(got, complex(want, 0), 1e-10*math.Max(1, want)) {
			t.Errorf("Γ(%v) = %v, want %v", n, got, want)
		}
	}
	if got := Gamma(1 + 1i); !near(got, complex(0.4980156681183560, -0.1549498283018106), 1e-12) {
		t.Errorf("Γ(1+i) = %v", got)
	}
}

func TestZeta(t *testing.T) {
	if got := Zeta(2); !near(got, complex(math.Pi*math.Pi/6, 0), 1e-12) {
		t.Errorf("ζ(2) = %v", got)
	}
	if got := Zeta(4); !near(got, complex(math.Pow(math.Pi, 4)/90, 0), 1e-12) {
		t.Errorf("ζ(4) = %v", got)
	}
	if got := Zeta(0); !near(got, -0.5, 1e-12) {
		t.Errorf("ζ(0) = %v", got)
	}
	if got := Zeta(-1); !near(got, complex(-1.0/12, 0), 1e-10) {
		t.Errorf("ζ(-1) = %v", got)
	}
	if got := Zeta(0.5); !near(got, -1.4603545088095868, 1e-10) {
		t.Errorf("ζ(1/2) = %v", got)
	}
	for _, z := range []float64{14.134725141734693, 21.022039638771555, 25.010857580145688, 37.586178158825671} {
		if got := cmplx.Abs(Zeta(complex(0.5, z))); got > 1e-8 {
			t.Errorf("|ζ(1/2 + %vi)| = %v, want ~0", z, got)
		}
	}
	if got := eval(t, "zeta(1/2 + i*t)", []string{"t"}, 14.134725141734693); cmplx.Abs(got) > 1e-8 {
		t.Errorf("zeta(1/2+i*t) at the first zero = %v", got)
	}
	if !cmplx.IsInf(Zeta(1)) {
		t.Error("ζ(1) is the pole")
	}
}

func TestZerosOfZeta(t *testing.T) {
	f := func(t float64) float64 { return cmplx.Abs(Zeta(complex(0.5, t))) }
	zs := Zeros(f, 0, 30)
	want := []float64{14.1347, 21.0220, 25.0109}
	if len(zs) != len(want) {
		t.Fatalf("zeros in 0..30: %v", zs)
	}
	for i := range want {
		if math.Abs(zs[i]-want[i]) > 1e-3 {
			t.Errorf("zero %d at %v, want %v", i, zs[i], want[i])
		}
	}
}

func TestRoots(t *testing.T) {
	rs := Roots(math.Sin, 0.5, 10)
	if len(rs) != 3 || math.Abs(rs[0]-math.Pi) > 1e-9 {
		t.Fatalf("roots of sin in 0.5..10: %v", rs)
	}
	if rs := Roots(math.Tan, 1, 2); len(rs) != 0 {
		t.Fatalf("tan has a pole, not a root, between 1 and 2: %v", rs)
	}
}

// tan breaks at its asymptotes instead of joining +inf to -inf with a
// vertical line, and 1/x breaks at 0.
func TestSamplingBreaks(t *testing.T) {
	box := Box{0, 0, 600, 400}
	at := func(f func(float64) float64) func(float64) (float64, float64, bool) {
		return func(x float64) (float64, float64, bool) {
			return 300 + x*40, 200 - f(x)*40, true
		}
	}
	pieces := Sample(at(math.Tan), -7, 7, box)
	if len(pieces) != 5 { // asymptotes at ±π/2, ±3π/2
		t.Fatalf("tan on -7..7 has %d pieces, want 5", len(pieces))
	}
	for _, pc := range Clip(pieces, box) {
		for i := 1; i < len(pc); i++ {
			if math.Abs(pc[i][1]-pc[i-1][1]) > 100 {
				t.Fatalf("a segment jumps %v px", pc[i][1]-pc[i-1][1])
			}
		}
	}
	inv := Sample(at(func(x float64) float64 { return 1 / x }), -5, 5, box)
	if len(inv) != 2 {
		t.Fatalf("1/x has %d pieces, want 2", len(inv))
	}
	// sqrt is undefined left of 0: one piece, starting near 0
	sq := Sample(func(x float64) (float64, float64, bool) { return x * 100, 200 - math.Sqrt(x)*100, true }, -1, 1, box)
	if len(sq) != 1 || sq[0].Ts[0] > 0.01 {
		t.Fatalf("sqrt pieces: %d, starting at %v", len(sq), sq[0].Ts[0])
	}
	// a smooth curve is one piece with short segments
	sin := Sample(at(math.Sin), -7, 7, box)
	if len(sin) != 1 {
		t.Fatalf("sin has %d pieces", len(sin))
	}
	pts := sin[0].Pts
	for i := 1; i < len(pts); i++ {
		if d := math.Hypot(pts[i][0]-pts[i-1][0], pts[i][1]-pts[i-1][1]); d > maxSeg+1e-9 {
			t.Fatalf("segment of %v px", d)
		}
	}
}

func TestClipAndCut(t *testing.T) {
	box := Box{0, 0, 100, 100}
	runs := Clip([]Piece{{Pts: [][2]float64{{-50, 50}, {50, 50}, {150, 50}, {150, 60}, {50, 60}}}}, box)
	if len(runs) != 2 || runs[0][0] != [2]float64{0, 50} || runs[0][len(runs[0])-1] != [2]float64{100, 50} || runs[1][0] != [2]float64{100, 60} {
		t.Fatalf("clipped: %v", runs)
	}
	pc := []Piece{{Pts: [][2]float64{{0, 0}, {1, 1}, {2, 2}}, Ts: []float64{0, 1, 2}}}
	cut := Cut(pc, 1.5, [2]float64{1.5, 1.5}, true)
	if len(cut) != 1 || len(cut[0].Pts) != 3 || cut[0].Pts[2] != [2]float64{1.5, 1.5} {
		t.Fatalf("cut: %v", cut)
	}
	if len(pc[0].Pts) != 3 || pc[0].Pts[2] != [2]float64{2, 2} {
		t.Fatal("cutting must not change the samples")
	}
}

// Zooming toward a point: the target centre travels in a straight line to
// the middle of the box, the scale changes by the same factor each step,
// and the point that stays still on screen does not move at all.
func TestZoomKeepsFocusOnALine(t *testing.T) {
	f := Frame{Base: Ranges{-2, 4, -2.5, 2.5}, Square: true, W: 600, H: 400}
	home := f.Home()
	z := &Ranges{-.35, .55, -.45, .45}
	to := f.Target(z)
	var prev [2]float64
	sx0, sy0 := f.Screen(home, (z.X0+z.X1)/2, (z.Y0+z.Y1)/2)
	for i := 0; i <= 50; i++ {
		s := float64(i) / 50
		v := Lerp(home, to, s)
		x, y := f.Screen(v, (z.X0+z.X1)/2, (z.Y0+z.Y1)/2)
		if cross := x*sy0 - y*sx0; math.Abs(cross) > 1e-6 {
			t.Fatalf("at s=%v the target is off the line by %v", s, cross)
		}
		if i > 0 && math.Hypot(x, y) > math.Hypot(prev[0], prev[1])+1e-9 {
			t.Fatalf("at s=%v the target moved away from the centre", s)
		}
		prev = [2]float64{x, y}
		want := math.Pow(to.MX, s)
		if math.Abs(v.MX-want) > 1e-9*want {
			t.Fatalf("magnification at s=%v is %v, want %v", s, v.MX, want)
		}
	}
	if math.Hypot(prev[0], prev[1]) > 1e-9 {
		t.Fatalf("the target ends at %v, not the centre", prev)
	}
	// the fixed point F: (F - c0) k0 = (F - cZ) kZ stays put
	k0, _ := f.Scale(home)
	kZ, _ := f.Scale(to)
	F := (to.CX*kZ - home.CX*k0) / (kZ - k0)
	fx0, _ := f.Screen(home, F, 0)
	for _, s := range []float64{.1, .37, .8} {
		fx, _ := f.Screen(Lerp(home, to, s), F, 0)
		if math.Abs(fx-fx0) > 1e-6 {
			t.Fatalf("the fixed point moved from %v to %v", fx0, fx)
		}
	}
	vis := f.Visible(to)
	if vis.X0 > z.X0+1e-9 || vis.X1 < z.X1-1e-9 || vis.Y0 > z.Y0+1e-9 || vis.Y1 < z.Y1-1e-9 {
		t.Fatalf("zoomed view %v does not show %v", vis, *z)
	}
}

// A zoom that starts while another runs continues from where it had got
// to; going back ends exactly at home.
func TestViewAtIsContinuous(t *testing.T) {
	f := Frame{Base: Ranges{-2, 4, -2.5, 2.5}, Square: true, W: 600, H: 400}
	zooms := []Zoom{{To: &Ranges{-.35, .55, -.45, .45}, Start: 1, Dur: 2}, {To: nil, Start: 2.5, Dur: 2}}
	prev := f.ViewAt(zooms, 0)
	for ti := 1; ti <= 600; ti++ {
		tt := float64(ti) / 100
		v := f.ViewAt(zooms, tt)
		if math.Abs(math.Log(v.MX/prev.MX)) > 0.05 || math.Abs(v.CX-prev.CX) > 0.05 {
			t.Fatalf("view jumps at %v: %v -> %v", tt, prev, v)
		}
		prev = v
	}
	if prev != f.Home() {
		t.Fatalf("ends at %v, want home %v", prev, f.Home())
	}
}

// Finer ticks fade in as room opens; ticks already shown stay.
func TestTicksFade(t *testing.T) {
	alphaAt := func(k, v float64) float64 {
		for _, tk := range Ticks(-1, 1, k, 50, false) {
			if math.Abs(tk.V-v) < 1e-9 {
				return tk.Alpha
			}
		}
		return 0
	}
	if alphaAt(60, 0.5) != 0 || alphaAt(100, 1) != 1 {
		t.Fatalf("0.5 shows %v at 60 px/unit, 1 shows %v at 100", alphaAt(60, .5), alphaAt(100, 1))
	}
	prev := 0.0
	for k := 60.0; k < 200; k += 2 {
		a := alphaAt(k, 0.5)
		if a < prev || a-prev > 0.1 {
			t.Fatalf("0.5 tick alpha goes %v -> %v at k=%v", prev, a, k)
		}
		if k >= 70 && alphaAt(k, 1) != 1 {
			t.Fatalf("the 1 tick fades at k=%v", k)
		}
		prev = a
	}
	if prev != 1 {
		t.Fatalf("0.5 should be fully shown at 200 px/unit, got %v", prev)
	}
	var texts []string
	for _, x := range Ticks(0, 4*math.Pi, 40, 50, true) {
		if x.Label == 1 {
			texts = append(texts, x.Text)
		}
	}
	if strings.Join(texts, " ") != "0 π 2π 3π 4π" {
		t.Fatalf("pi labels: %v", texts)
	}
}
