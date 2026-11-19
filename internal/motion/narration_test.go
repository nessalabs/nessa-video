package motion

import (
	"fmt"
	"strings"
	"testing"

	c "github.com/nessalabs/nessa-video/internal/composition"
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

// on= starts a step just before its word is said, from the voice's timings
// when there are some and from the word's place in the line otherwise.
func TestOnWord(t *testing.T) {
	src := `video "t"
scene one
  box #a
  box #b
  say "Watch" speak="first the cache then the database"
  focus #a on="cache"
  focus #b on="the database"
`
	r := resolve.Script(src)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	say := &r.Video.Scenes[0].Steps[0]
	say.Speech = 3
	say.Said = []c.SaidWord{{"first", 0, .3}, {"the", .3, .5}, {"cache,", .5, 1}, {"then", 1.2, 1.5}, {"the", 1.5, 1.6}, {"database.", 1.6, 2.4}}
	tl := New(r.Video, DefaultTimings()).Scenes[0].Timeline()
	if !near(tl[1].Start, tl[0].Start+.5-HighlightLead) {
		t.Errorf("focus #a at %.2f; want just before \"cache\" at %.2f", tl[1].Start, tl[0].Start+.5)
	}
	if !near(tl[2].Start, tl[0].Start+1.5-HighlightLead) {
		t.Errorf("focus #b at %.2f; want just before \"the database\" at %.2f", tl[2].Start, tl[0].Start+1.5)
	}
	// without timings the place in the line stands in
	r2 := resolve.Script(src)
	r2.Video.Scenes[0].Steps[0].Speech = 3
	tl2 := New(r2.Video, DefaultTimings()).Scenes[0].Timeline()
	if want := tl2[0].Start + 3*2.0/6 - HighlightLead; !near(tl2[1].Start, want) {
		t.Errorf("estimated focus #a at %.2f; want %.2f", tl2[1].Start, want)
	}
}

func TestOnWordMustBeSaid(t *testing.T) {
	for src, want := range map[string]string{
		"video \"t\"\nscene s\n  box #a\n  focus #a on=\"x\"\n":                     "has no say line before it",
		"video \"t\"\nscene s\n  box #a\n  say \"one two\"\n  focus #a on=\"three\"\n": "never says",
	} {
		if got := fmt.Sprint(resolve.Script(src).Diags); !strings.Contains(got, want) {
			t.Errorf("want %q, got %s", want, got)
		}
	}
}

// A scene's opening line starts while the scene draws in.
func TestOpeningLineStartsEarly(t *testing.T) {
	p := plan(t, "video \"t\"\nscene a\n  box #a\n  box #b\n  box #c\n  say \"hello\"\n")
	if s := p.Scenes[0].Timeline()[0].Start; s > DefaultTimings().Lead+0.3+1e-9 {
		t.Errorf("opening line starts at %.2f; want while the scene draws in", s)
	}
}
