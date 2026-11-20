package motion

import (
	"testing"

	"github.com/nessalabs/nessa-video/internal/resolve"
)

const barScript = `video "t"
scene one
  chart #c bar horizontal unit=ms focus=/search
    endpoint, p95
    /checkout, 820
    /search, 610
    /orders, 430
  set #c /checkout=240
  sort #c
`

// A set eases one bar to its new value and takes the accent; a sort then
// slides the bars into their new order.
func TestChartSetAndSort(t *testing.T) {
	p := plan(t, barScript)
	sp := p.Scenes[0]
	c := el(t, sp.Scene, "#c")
	tl := sp.Timeline()
	set, srt := tl[0], tl[1]
	if set.Dur != ChartSetTime || srt.Dur != SortTime {
		t.Fatalf("set takes %v and sort %v", set.Dur, srt.Dur)
	}
	if srt.Start < set.Start+set.Dur {
		t.Fatalf("sort should wait for the set to finish")
	}
	before := sp.At(set.Start - .01).Chart(c)
	if before.Accent[1] != 1 || before.Accent[0] != 0 {
		t.Fatalf("focus= should accent /search at rest: %v", before.Accent)
	}
	mid := sp.At(set.Start + set.Dur/2).Chart(c)
	if v := mid.Values[0][0]; !(v < 820 && v > 240) {
		t.Fatalf("checkout should be on its way from 820 to 240, is %v", v)
	}
	after := sp.At(set.Start + set.Dur + .01).Chart(c)
	if after.Values[0][0] != 240 || after.Accent[0] != 1 || after.Accent[1] != 0 {
		t.Fatalf("after the set: %v accent %v", after.Values, after.Accent)
	}
	if after.Top != 820 {
		t.Fatalf("the axis should hold every value the scene shows, top %v", after.Top)
	}
	sliding := sp.At(srt.Start + srt.Dur/2).Chart(c)
	if s := sliding.Slot[0]; !(s > 0 && s < 2) {
		t.Fatalf("checkout should be sliding down, at slot %v", s)
	}
	done := sp.At(srt.Start + srt.Dur + .01).Chart(c)
	want := []float64{2, 0, 1} // 240 last, 610 first, 430 second
	for i, w := range want {
		if done.Slot[i] != w {
			t.Fatalf("after sort, slots %v, want %v", done.Slot, want)
		}
	}
}

// Steps wait for a chart's bars to grow in.
func TestChartGrowsBeforeSteps(t *testing.T) {
	p := plan(t, barScript)
	sp := p.Scenes[0]
	c := el(t, sp.Scene, "#c")
	since, dur, ok := sp.At(0).Grow(c)
	if !ok || dur <= 0 {
		t.Fatalf("chart should grow in")
	}
	if first := sp.Timeline()[0].Start; first < -since+dur {
		t.Fatalf("first step at %v starts before the bars finish growing at %v", first, -since+dur)
	}
}

func TestChartSortAscending(t *testing.T) {
	p := plan(t, `video "t"
scene one
  chart #c bar
    k, v
    a, 3
    b, 1
    c, 2
  sort #c asc
`)
	sp := p.Scenes[0]
	c := el(t, sp.Scene, "#c")
	got := sp.At(sp.Dur).Chart(c).Slot
	if got[0] != 2 || got[1] != 0 || got[2] != 1 {
		t.Fatalf("ascending slots %v", got)
	}
}

// mark span= singles a timeline span out with its readout note, and the
// timeline gets room for that readout.
func TestMarkSpan(t *testing.T) {
	r := resolve.Script(`video "t"
scene one
  timeline #tl
    db | 0-10 users | 12-40 orders
  play #tl
  mark #tl span=orders note="slowest"
`)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	p := New(r.Video, DefaultTimings())
	sp := p.Scenes[0]
	tl := el(t, sp.Scene, "#tl")
	if !tl.Timeline.Readout {
		t.Fatal("a mark with span= should leave room for the readout")
	}
	_, to, mix, note := sp.At(sp.Dur).Picked(tl)
	if len(to) != 1 || to[0] != "orders" || mix != 1 || note != "slowest" {
		t.Fatalf("picked %v %v %q", to, mix, note)
	}
	if ago, done := sp.At(sp.Dur).PlayEnded(tl); !done || ago <= 0 {
		t.Fatal("the playhead should have reached the end")
	}
}
