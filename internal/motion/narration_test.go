package motion

import (
	"testing"

	"github.com/nessalabs/nessa-video/internal/resolve"
)

// A say line holds the step after it until the line is spoken; `with`
// runs a step alongside the line instead.
func TestSayPacesSteps(t *testing.T) {
	r := resolve.Script(`video "t"
scene one
  box #a
  box #b
  say "first line"
  dim #a
  say "second line"
  with focus #b
`)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	steps := r.Video.Scenes[0].Steps
	steps[0].Speech = 2
	steps[2].Speech = 3
	sp := New(r.Video, DefaultTimings()).Scenes[0]
	tl := sp.Timeline()
	if len(tl) != 4 {
		t.Fatalf("timeline = %+v", tl)
	}
	say1, mark, say2, focus := tl[0], tl[1], tl[2], tl[3]
	if !near(say1.Dur, 2.2) {
		t.Errorf("say lasts %.2f; want speech + 0.2", say1.Dur)
	}
	if !near(mark.Start, say1.Start+2.2) {
		t.Errorf("mark starts at %.2f; want after the line (%.2f)", mark.Start, say1.Start+2.2)
	}
	if say2.Start < mark.Start {
		t.Errorf("second line at %.2f starts before the mark at %.2f", say2.Start, mark.Start)
	}
	if !near(focus.Start, say2.Start) {
		t.Errorf("with focus starts at %.2f; want with its line at %.2f", focus.Start, say2.Start)
	}
	if focus.Verb != "focus" || focus.Target != "#b" {
		t.Errorf("timeline entry = %+v", focus)
	}
}

func TestSelectorGroupInStep(t *testing.T) {
	p := plan(t, `video "t"
scene one
  box #a
  box #b
  box #c
  focus #a, #b
`)
	sp := p.Scenes[0]
	m := sp.At(sp.Dur)
	for _, id := range []string{"#a", "#b"} {
		if s := m.Look(el(t, sp.Scene, id)).State; s != "focus" {
			t.Errorf("%s state = %q; want focus", id, s)
		}
	}
	if s := m.Look(el(t, sp.Scene, "#c")).State; s == "focus" {
		t.Errorf("#c is focused but was not named")
	}
}
