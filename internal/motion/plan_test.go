package motion

import (
	"math"
	"testing"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/resolve"
	"github.com/nessalabs/nessa-video/internal/selector"
)

func el(t *testing.T, sc *c.Scene, sel string) *c.Element {
	t.Helper()
	s, err := selector.ParseSelector(sel)
	if err != nil {
		t.Fatal(err)
	}
	els := c.Select(sc.Root, s)
	if len(els) == 0 {
		t.Fatalf("%s matches nothing", sel)
	}
	return els[0]
}

func plan(t *testing.T, src string) *Plan {
	t.Helper()
	r := resolve.Script(src)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	return New(r.Video, DefaultTimings())
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestEnterAndSteps(t *testing.T) {
	p := plan(t, `video "t"
scene one
  box #a
  box #b enter=none
  #a -> #b
  focus #a
  with dim #b for=1s
  wait 0.5s
  leave #a at=+0.2s
  say "hello there friend"
`)
	sp := p.Scenes[0]
	tm := DefaultTimings()
	a, b := el(t, sp.Scene, "#a"), el(t, sp.Scene, "#b")

	if ap := sp.At(0).Look(a); ap.Draw != 0 || ap.Opacity != 0 {
		t.Errorf("#a before entering = %+v", ap)
	}
	// #a draws (theme default for box) starting at the lead time
	if ap := sp.At(tm.Lead + tm.Draw).Look(a); ap.Draw != 1 || ap.Opacity != 1 {
		t.Errorf("#a after drawing = %+v", ap)
	}
	// #b has enter=none: present as soon as its slot starts
	if ap := sp.At(tm.Lead + tm.Stagger).Look(b); ap.Opacity != 1 {
		t.Errorf("#b with enter=none = %+v", ap)
	}

	// the connector draws after both ends; steps start after entering ends
	edge := el(t, sp.Scene, "edge")
	ea := sp.enter[edge]
	if !near(ea.Start, tm.Lead+tm.Draw) {
		t.Errorf("edge starts at %v, want %v", ea.Start, tm.Lead+tm.Draw)
	}
	stepStart := ea.Start + tm.Draw + 0.15
	ch := sp.changes[a]
	if len(ch) != 1 || !near(ch[0].Start, stepStart) || ch[0].To != "focus" {
		t.Fatalf("focus change = %+v, want start %v", ch, stepStart)
	}
	// `with` starts together with the previous step and `for` sets its length
	if chb := sp.changes[b]; len(chb) != 1 || !near(chb[0].Start, stepStart) || chb[0].Dur != 1 {
		t.Errorf("with dim = %+v", chb)
	}
	// relative `at` counts from the previous step's start (the wait)
	waitStart := stepStart + 1 // after the longer of the two parallel steps
	if ex := sp.exit[a]; !near(ex.Start, waitStart+0.2) {
		t.Errorf("leave starts at %v, want %v", ex.Start, waitStart+0.2)
	}
	if ap := sp.End().Look(a); ap.Opacity > 0.001 {
		t.Errorf("#a should be gone at the end, opacity %v", ap.Opacity)
	}
	if text, op := sp.At(sp.Dur - 0.5).Caption(); text != "hello there friend" || op <= 0 {
		t.Errorf("caption = %q at %v", text, op)
	}
}

func TestCarryAndClear(t *testing.T) {
	p := plan(t, `video "t"
scene one
  box #a
  box #b
  focus #a
scene two
  box #c
scene three
  clear keep=#a
  box #d
`)
	one, two, three := p.Scenes[0], p.Scenes[1], p.Scenes[2]
	// carried elements do not enter again and keep their state
	a2 := el(t, two.Scene, "#a")
	if _, entering := two.enter[a2]; entering {
		t.Error("#a should not enter again in scene two")
	}
	if ap := two.At(0).Look(a2); ap.State != "focus" || ap.Opacity != 1 {
		t.Errorf("#a in scene two = %+v", ap)
	}
	// scene two's dropped elements leave at its end because three clears them
	if _, ok := two.exit[el(t, two.Scene, "#b")]; !ok {
		t.Error("#b should leave at the end of scene two")
	}
	if _, ok := two.exit[el(t, two.Scene, "#a")]; ok {
		t.Error("#a is kept and should not leave")
	}
	if p.Total != one.Dur+two.Dur+three.Dur {
		t.Error("total is not the sum of scenes")
	}
	if sp, lt := p.At(one.Dur + 0.1); sp != two || !near(lt, 0.1) {
		t.Error("At picks the wrong scene")
	}
}

func TestCameraAndSend(t *testing.T) {
	p := plan(t, `video "t"
scene s
  box #a
  box #b
  #a -> #b
  camera zoom #b
  send #a
`)
	sp := p.Scenes[0]
	from, to, _ := sp.End().Camera()
	if from != nil || to != el(t, sp.Scene, "#b") {
		t.Errorf("camera = %v -> %v", from, to)
	}
	if len(sp.sends) != 1 {
		t.Fatalf("sends = %d", len(sp.sends))
	}
	mid := sp.sends[0].Start + sp.sends[0].Dur/2
	if pk := sp.At(mid).Packets(); len(pk) != 1 || pk[0].Progress <= 0 || pk[0].Progress >= 1 {
		t.Errorf("packets mid-send = %+v", pk)
	}
}

func TestMark(t *testing.T) {
	p := plan(t, `video "t"
scene s
  code #c go focus=1
    a()
    b()
  mark #c lines=2
`)
	sp := p.Scenes[0]
	e := el(t, sp.Scene, "#c")
	if from, to, _ := sp.At(0).Focus(e); from == nil || to == nil || to[0].From != 1 {
		t.Fatalf("initial focus = %v %v", from, to)
	}
	from, to, mix := sp.End().Focus(e)
	if from[0].From != 1 || to[0].From != 2 || mix != 1 {
		t.Fatalf("end focus = %v -> %v at %v", from, to, mix)
	}
}

func TestArrayAndDiagramSteps(t *testing.T) {
	p := plan(t, `video "t"
scene s
  array #a 5 2 9 ptr="lo=0"
  mermaid #f
    flowchart LR
      A --> B --> C
  timeline #tl
    x | 0-10 a
  ptr #a lo=2
  swap #a 0 2
  mark #a cells=1
  walk #f A B C
  play #tl
`)
	sp := p.Scenes[0]
	a := el(t, sp.Scene, "#a")
	end := sp.End()
	if ps := end.Pointers(a); len(ps) != 1 || ps[0].Pos != 2 {
		t.Errorf("pointers at end = %+v", ps)
	}
	if sl := end.Slots(a); sl[0] != 2 || sl[2] != 0 || sl[1] != 1 {
		t.Errorf("slots at end = %v", sl)
	}
	sw := sp.swaps[a][0]
	if sl := sp.At(sw.Start + sw.Dur/2).Slots(a); sl[0] <= 0 || sl[0] >= 2 {
		t.Errorf("slot mid-swap = %v", sl)
	}
	if _, to, _ := end.Cells(a); len(to) != 1 || to[0] != 1 {
		t.Errorf("marked cells = %v", to)
	}
	for _, id := range []string{"#f/A", "#f/B", "#f/C"} {
		if ap := end.Look(el(t, sp.Scene, id)); ap.State != "focus" {
			t.Errorf("%s after walk = %s", id, ap.State)
		}
	}
	links := 0
	for _, e := range c.Select(sp.Scene.Root, mustSel(t, "#f edge")) {
		if end.Look(e).State == "focus" {
			links++
		}
	}
	if links != 2 {
		t.Errorf("walked links = %d", links)
	}
	tl := el(t, sp.Scene, "#tl")
	if pos, ok := end.Playhead(tl); !ok || pos != 1 {
		t.Errorf("playhead at end = %v %v", pos, ok)
	}
}

func mustSel(t *testing.T, s string) *selector.Selector {
	t.Helper()
	sel, err := selector.ParseSelector(s)
	if err != nil {
		t.Fatal(err)
	}
	return sel
}
