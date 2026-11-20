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
	say.Said = []c.SaidWord{{Text: "first", Start: 0, End: .3}, {Text: "the", Start: .3, End: .5}, {Text: "cache,", Start: .5, End: 1}, {Text: "then", Start: 1.2, End: 1.5}, {Text: "the", Start: 1.5, End: 1.6}, {Text: "database.", Start: 1.6, End: 2.4}}
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
		"video \"t\"\nscene s\n  box #a\n  focus #a on=\"x\"\n":                        "has no say line before it",
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

// A play started during a line unfolds over the rest of that line.
func TestPlayFollowsTheLine(t *testing.T) {
	r := resolve.Script("video \"t\"\nscene a\n  timeline #tl\n    a | 0-10 x\n  say \"Watch\" speak=\"one two three four five six\"\n  play #tl on=\"two\"\n")
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	r.Video.Scenes[0].Steps[0].Speech = 6
	tl := New(r.Video, DefaultTimings()).Scenes[0].Timeline()
	say, play := tl[0], tl[1]
	if end := play.Start + play.Dur; !near(end, say.Start+6) {
		t.Errorf("play ends at %.2f; want with the line at %.2f", end, say.Start+6)
	}
}

// show draws an element in on a word; play to= reveals a diagram in steps.
func TestShowAndStepwisePlay(t *testing.T) {
	r := resolve.Script(`video "t"
scene a
  box #a "a"
  box #b "b"
  #a -> #b
  mermaid #seq
    sequenceDiagram
      A->>B: one
      B-->>A: two
      A->>B: three
      B-->>A: four
  say "Watch" speak="first the box, then one, then the rest"
  show #b on="box"
  play #seq to=1 on="one"
  play #seq on="rest"
`)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	r.Video.Scenes[0].Steps[0].Speech = 4
	sp := New(r.Video, DefaultTimings()).Scenes[0]
	tl := sp.Timeline()
	b := el(t, sp.Scene, "#b")
	if op := sp.At(tl[1].Start - 0.05).Look(b).Opacity; op != 0 {
		t.Errorf("#b visible (%.2f) before its show step", op)
	}
	if op := sp.At(tl[1].Start + tl[1].Dur + 0.05).Look(b).Opacity; op < 0.99 {
		t.Errorf("#b not shown after its step: %.2f", op)
	}
	// the connector to #b draws once #b is in
	edge := el(t, sp.Scene, "edge")
	if a := sp.enter[edge]; a.Start < tl[1].Start {
		t.Errorf("connector starts at %.2f, before #b shows at %.2f", a.Start, tl[1].Start)
	}
	seq := el(t, sp.Scene, "#seq")
	if pos, _ := sp.At(tl[3].Start - 0.01).Playhead(seq); !near(pos, 0.25) {
		t.Errorf("after to=1 of 4 rows the playhead is at %.2f, want 0.25", pos)
	}
	if pos, _ := sp.End().Playhead(seq); !near(pos, 1) {
		t.Errorf("playhead at the end = %.2f, want 1", pos)
	}
}

// A flowchart with reveal=steps starts empty and grows as walk visits it.
func TestFlowchartBuildsAsItIsWalked(t *testing.T) {
	r := resolve.Script(`video "t"
scene a
  mermaid #f reveal=steps
    flowchart LR
      A[a] --> B[b]
      B --> C[c]
      B --> D[d]
  say "Watch" speak="first a then b then c"
  walk #f A B on="first"
  walk #f B C on="then c"
`)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	r.Video.Scenes[0].Steps[0].Speech = 4
	sp := New(r.Video, DefaultTimings()).Scenes[0]
	tl := sp.Timeline()
	a, b, c, d := el(t, sp.Scene, "#f/A"), el(t, sp.Scene, "#f/B"), el(t, sp.Scene, "#f/C"), el(t, sp.Scene, "#f/D")
	if op := sp.At(tl[1].Start - 0.2).Look(a).Opacity; op != 0 {
		t.Errorf("a is visible (%.2f) before the walk reaches it", op)
	}
	if op := sp.At(tl[2].Start - 0.05).Look(c).Opacity; op != 0 {
		t.Errorf("c is visible (%.2f) before the second walk", op)
	}
	if op := sp.End().Look(b).Opacity; op < 0.99 {
		t.Errorf("b should be in by the end: %.2f", op)
	}
	if op := sp.End().Look(d).Opacity; op != 0 {
		t.Errorf("d was never walked and should stay hidden: %.2f", op)
	}
}

// A part a diagram never showed stays hidden when the diagram is carried,
// until a walk in the new scene reaches it.
func TestHiddenPartsStayHiddenWhenCarried(t *testing.T) {
	r := resolve.Script(`video "t"
scene one
  mermaid #f reveal=steps
    flowchart LR
      A[a] --> B[b]
      B --> C[c]
  walk #f A B
scene two
  say "Watch" speak="and then c"
  walk #f B C on="then c"
`)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	r.Video.Scenes[1].Steps[0].Speech = 2
	sp := New(r.Video, DefaultTimings()).Scenes[1]
	c := el(t, sp.Scene, "#f/C")
	if op := sp.At(0.1).Look(c).Opacity; op != 0 {
		t.Errorf("c pops in at the cut: %.2f", op)
	}
	if op := sp.End().Look(c).Opacity; op < 0.99 {
		t.Errorf("c should be in once walked: %.2f", op)
	}
}
