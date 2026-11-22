package resolve

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
)

func TestPlotBlock(t *testing.T) {
	res := Script(`video "t"
scene one
  plot #p x=0..4pi y=-1..1 square=true a=0 k=1/2
    y = sin(x) * exp(-x/4)
    curve x=sin(3t) y=sin(2t) t=0..2pi
    polar r = 1 + cos(theta) theta=0..2pi
    zeta(1/2 + i*t) t=0..30 marks=zeros
    point (1, 0.8) "peak"
  play #p to=t=14.13
  set #p a=1 for=2.5s
  note #p at=(0,0) "first zero"
  note #p (pi, 0) "pi"
  zoom #p x=-.35..0.55 y=-.45..0.45
  zoom #p
`)
	if len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
	pb := res.Video.Scenes[0].Root.Children[0].Plot
	if pb == nil || !pb.Square || !pb.XPi || pb.YPi || math.Abs(pb.X[1]-4*math.Pi) > 1e-12 || pb.Y[0] != -1 {
		t.Fatalf("plot = %+v", pb)
	}
	if len(pb.Params) != 2 || pb.Params[1].Name != "k" || pb.Params[1].Value != .5 {
		t.Fatalf("params = %+v", pb.Params)
	}
	if len(pb.Curves) != 4 || len(pb.Points) != 1 || pb.Points[0].Label != "peak" {
		t.Fatalf("curves %d, points %+v", len(pb.Curves), pb.Points)
	}
	kinds := []string{"y", "curve", "polar", "complex"}
	vars := []string{"x", "t", "theta", "t"}
	for i, cv := range pb.Curves {
		if cv.Kind != kinds[i] || cv.Var != vars[i] {
			t.Errorf("curve %d is %s of %s", i, cv.Kind, cv.Var)
		}
	}
	z := pb.Curves[3]
	if !z.MarkZeros || len(z.Zeros) != 3 || math.Abs(z.Zeros[0]-14.1347) > 1e-3 {
		t.Fatalf("zeros = %v", z.Zeros)
	}
	// to= reads the first curve with that parameter: here the t=0..2pi one
	if f, ok := pb.Fraction("t=pi"); !ok || math.Abs(f-.5) > 1e-12 {
		t.Fatalf("t=pi is %v of the trace", f)
	}
	if x, y, ok := pb.Curves[2].At(0, pb.Defaults()); !ok || math.Abs(x-2) > 1e-12 || y != 0 {
		t.Fatalf("polar at 0 = %v, %v", x, y)
	}
	// a real curve taking a complex value is undefined there, so it breaks
	res = Script("video \"t\"\nscene one\n  plot #p x=-1..1\n    y = sqrt(x)\n")
	if _, _, ok := res.Video.Scenes[0].Root.Children[0].Plot.Curves[0].At(-.5, nil); ok {
		t.Fatal("sqrt(-0.5) should be undefined on a real plot")
	}
}

func TestPlotErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"    y = sin(q)", `unknown name "q"`},
		{"    y = sin(", `cannot read`},
		{"    zeta(1/2 + i*t)", `give the range of t`},
		{"    curve x=sin(t) t=0..1", `needs x= and y=`},
		{"    y = x marks=poles", `unknown marks value`},
		{"    y = x\n  set #p b=1", `no parameter "b"`},
		{"    y = x\n  play #p to=t=3", `no curve with`},
		{"    y = x\n  note #p \"x\"", `(x, y)`},
		{"    y = x\n  zoom #p x=2..1", `bad x range`},
		{"    y = x\n  zoom #b", `not a plot`},
	} {
		res := Script("video \"t\"\nscene one\n  box #b \"b\"\n  plot #p x=0..1 a=0\n" + tc.src + "\n")
		if !res.Diags.HasErrors() || !strings.Contains(fmt.Sprint(res.Diags), tc.want) {
			t.Errorf("%s: want an error with %q, got %v", tc.src, tc.want, res.Diags)
		}
	}
	res := Script("video \"t\"\nscene one\n  plot #p x=0..1 sin=2\n    y = x\n")
	if !strings.Contains(fmt.Sprint(res.Diags), "cannot name a plot parameter") {
		t.Errorf("a function name as a parameter: %v", res.Diags)
	}
}

func TestPlotExampleResolves(t *testing.T) {
	src, err := os.ReadFile("../../examples/plot.reel")
	if err != nil {
		t.Fatal(err)
	}
	if res := Script(string(src)); len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
}
