package stage

import (
	"testing"

	"github.com/nessalabs/nessa-video/internal/render"
)

// A set that gives a table cell longer text cross-fades it in place: the
// columns already have room for it, so nothing moves.
func TestTableSetKeepsColumns(t *testing.T) {
	s, p, sc := liveScene(t, `video "t"
scene one
  table #t
    row
      p #a "Grace"
      p #b "Invited"
      p #c "Team"
  set #b "Active since yesterday"
`)
	lay := s.Layouts(p)[sc]
	b, cc := byID(sc, "b"), byID(sc, "c")
	tl := p.Scenes[0].Timeline()
	before, mid, after := lay.At(0), lay.At(tl[0].Start+tl[0].Dur/2), lay.At(tl[0].Start+tl[0].Dur+.01)
	if before.Boxes[cc].X != after.Boxes[cc].X || before.Boxes[cc].X != mid.Boxes[cc].X {
		t.Fatalf("the next column moved: %v, %v, %v", before.Boxes[cc], mid.Boxes[cc], after.Boxes[cc])
	}
	if tl[0].Dur < .7 {
		t.Fatalf("a cell cross-fades over at least .7s, got %v", tl[0].Dur)
	}
	// halfway, both the old and the new text are drawn
	pt := &painter{s: s, lay: mid, mo: p.Scenes[0].At(tl[0].Start + tl[0].Dur/2), opt: s.Options(), l: &render.List{}}
	if !pt.cellFade(b, mid.Boxes[b], s.Options().Type["p"], s.col("ink"), s.col("hi"), 1) || len(pt.l.Ops) < 2 {
		t.Fatalf("expected old and new text cross-fading, got %d ops", len(pt.l.Ops))
	}
}

// A row shown later slides into place and wears a highlight that fades.
func TestShownRowGreets(t *testing.T) {
	_, p, sc := liveScene(t, `video "t"
scene one
  table #t
    row
      p "a"
    row #n
      p "b"
  show #n
`)
	n := byID(sc, "n")
	sp := p.Scenes[0]
	start := sp.Timeline()[0].Start
	at := func(t float64) *painter { return &painter{mo: sp.At(t)} }
	if at(start+.25).rowSlide(n) <= 0 || at(start+.25).rowGlow(n) != 1 {
		t.Fatal("a new row should slide in with its highlight on")
	}
	if at(start+3.5).rowSlide(n) != 0 || at(start+3.5).rowGlow(n) != 0 {
		t.Fatal("the slide and highlight should be over")
	}
	first := byID(sc, "t").Children[0]
	if at(start+.5).rowGlow(first) != 0 {
		t.Fatal("rows on screen from the start get no highlight")
	}
}

// Digits take one width, so a counting label never shifts.
func TestTabularDigits(t *testing.T) {
	s := &Stage{Measure: prop{}}
	width := func(txt string) float64 {
		p := &painter{s: s, l: &render.List{}, opacity: 1, draw: 1}
		p.tab(txt, 0, 0, s.Options().Type["label"], s.col("ink"), render.End)
		return -p.l.Ops[0].X
	}
	if a, b := width("111 ms"), width("888 ms"); a != b {
		t.Fatalf("111 ms starts at %v, 888 ms at %v", a, b)
	}
}

// prop measures 1 narrower than other digits.
type prop struct{}

func (prop) Measure(s, font string, size float64) float64 {
	w := 0.0
	for _, r := range s {
		if r == '1' {
			w += size * .3
		} else {
			w += size * .6
		}
	}
	return w
}
