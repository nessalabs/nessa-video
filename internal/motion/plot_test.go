package motion

import (
	"testing"
)

const zetaScript = `video "t"
scene one
  plot #z x=-2..3.5 y=-2..2 square=true a=0
    zeta(1/2 + i*t) t=0..30
  play #z to=t=15
  note #z at=(0,0) "first zero"
  set #z a=1
  zoom #z x=-.5..0.5 y=-.5..0.5
  play #z
`

// play to= traces up to the value and stops; the next play carries on
// from there, each taking its share of the full trace time.
func TestPlotPlayTo(t *testing.T) {
	p := plan(t, zetaScript)
	sp := p.Scenes[0]
	z := el(t, sp.Scene, "#z")
	tl := sp.Timeline()
	first, last := tl[0], tl[4]
	if !near(first.Dur, PlotTrace*.5) || !near(last.Dur, PlotTrace*.5) {
		t.Fatalf("each half of the trace should take %v, got %v and %v", PlotTrace*.5, first.Dur, last.Dur)
	}
	if tl[1].Dur != NoteTime || tl[2].Dur != PlotSetTime || tl[3].Dur != ZoomTime {
		t.Fatalf("note, set and zoom take %v %v %v", tl[1].Dur, tl[2].Dur, tl[3].Dur)
	}
	if pos, _ := sp.At(first.Start - .01).PlotTrace(z); pos != 0 {
		t.Fatalf("nothing is traced before play, got %v", pos)
	}
	mid, _ := sp.At(first.Start + first.Dur/2).PlotTrace(z)
	if !near(mid, .25) {
		t.Fatalf("halfway through the first play the trace is at %v, want .25", mid)
	}
	for _, at := range []float64{first.Start + first.Dur, last.Start - .01} {
		pos, dot := sp.At(at).PlotTrace(z)
		if !near(pos, .5) || dot != 1 {
			t.Fatalf("paused at %v: trace %v, dot %v", at, pos, dot)
		}
	}
	end, dot := sp.At(last.Start + last.Dur + DotFade + .01).PlotTrace(z)
	if end != 1 || dot != 0 {
		t.Fatalf("after the last play: trace %v, dot %v", end, dot)
	}
	// the trace reaches 3/4 halfway through the second play
	since, ok := sp.At(last.Start+last.Dur).PlotReached(z, .75)
	if !ok || !near(since, last.Dur/2) {
		t.Fatalf("3/4 reached %v ago (%v), want %v", since, ok, last.Dur/2)
	}
	vals, ro, show := sp.At(tl[2].Start + tl[2].Dur/2).PlotParams(z)
	if !near(vals[0], .5) || ro != 0 || show != 1 {
		t.Fatalf("halfway through set a=1: %v, readout %d at %v", vals, ro, show)
	}
	if zs := sp.At(0).PlotZooms(z); len(zs) != 1 || zs[0].To == nil || zs[0].To.X0 != -.5 {
		t.Fatalf("zooms = %+v", zs)
	}
}

// Without a play step a plot traces itself in as it enters, and a plot
// carried into the next scene stays traced.
func TestPlotTracesInAndCarries(t *testing.T) {
	p := plan(t, `video "t"
scene one
  plot #p x=0..1
    y = x^2
scene two
  title "next"
`)
	one := p.Scenes[0]
	pz := el(t, one.Scene, "#p")
	if _, dur, ok := one.At(0).Grow(pz); !ok || dur != PlotTrace {
		t.Fatalf("a plot with no play should trace in over %v", PlotTrace)
	}
	if pos, _ := one.At(one.Dur).PlotTrace(pz); pos != 1 {
		t.Fatalf("traced in by the end, got %v", pos)
	}
	two := p.Scenes[1]
	if pos, dot := two.At(0).PlotTrace(el(t, two.Scene, "#p")); pos != 1 || dot != 0 {
		t.Fatalf("carried plot: trace %v dot %v", pos, dot)
	}
}
