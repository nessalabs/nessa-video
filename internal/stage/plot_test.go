package stage

import (
	"math"
	"testing"

	"github.com/nessalabs/nessa-video/internal/render"
)

// An isolated plot moves and resizes its own box instead of scaling its
// drawing: tick numbers keep their size, and the box glides without a
// jump from its place to the spotlight.
func TestIsolatedPlotKeepsTextSize(t *testing.T) {
	s, p, sc := liveScene(t, `video "t"
scene one
  plot #z x=-2..3.5 y=-2..2 square=true
    zeta(1/2 + i*t) t=0..30
  aside #why layout=left
    p "On the critical line"
  play #z to=t=14.13
  isolate #z aside=#why
  zoom #z x=-.35..0.55 y=-.45..0.45
`)
	sp := p.Scenes[0]
	lay := s.Layouts(p)[sc]
	tl := sp.Timeline()
	iso, zoom := tl[1], tl[2]
	tickScale := func(tm float64) (float64, bool) {
		l := s.Frame(sc, lay, sp.At(tm))
		for _, op := range l.Ops {
			if op.Kind == render.TextOp && op.Size == 11 {
				return op.M.Scale(), true
			}
		}
		return 0, false
	}
	before, ok1 := tickScale(iso.Start - .01)
	after, ok2 := tickScale(iso.Start + iso.Dur + .01)
	if !ok1 || !ok2 || math.Abs(before-after) > 1e-9 {
		t.Fatalf("tick labels scale %v before isolating, %v after", before, after)
	}
	// through the isolation and the zoom the curve is drawn every frame,
	// and the equation (top left of the box) glides without a jump (at
	// its fastest, mid-isolation, it covers about 40 px a sample)
	var prev [2]float64
	for i := 0; i <= 60; i++ {
		tm := iso.Start + (zoom.Start+zoom.Dur-iso.Start)*float64(i)/60
		l := s.Frame(sc, lay, sp.At(tm))
		curve := false
		var eq [2]float64
		for _, op := range l.Ops {
			if op.Kind == render.PathOp && op.StrokeW == 2 {
				curve = true
			}
			if op.Kind == render.TextOp && op.Size == 13 && eq == [2]float64{} {
				eq = [2]float64{op.M[0]*op.X + op.M[4], op.M[3]*op.Y + op.M[5]}
			}
		}
		if !curve {
			t.Fatalf("no curve drawn at %v", tm)
		}
		if i > 0 && math.Hypot(eq[0]-prev[0], eq[1]-prev[1]) > 60 {
			t.Fatalf("the equation jumps from %v to %v at %v", prev, eq, tm)
		}
		prev = eq
	}
}
