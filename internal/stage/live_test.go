package stage

import (
	"testing"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/motion"
	"github.com/nessalabs/nessa-video/internal/resolve"
)

func liveScene(t *testing.T, src string) (*Stage, *motion.Plan, *c.Scene) {
	t.Helper()
	r := resolve.Script(src)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	v := r.Video
	p := motion.New(v, motion.TimingsFrom(v.Tokens))
	return &Stage{Tokens: v.Tokens, Measure: mono{}, W: v.Width, H: v.Height}, p, v.Scenes[0]
}

func byID(sc *c.Scene, id string) *c.Element {
	var out *c.Element
	sc.Root.Walk(func(e *c.Element) {
		if e.ID() == id {
			out = e
		}
	})
	return out
}

// In a live row, a shown child takes no room until it is shown, and a
// child that leaves gives its room back; the others glide.
func TestLiveRowMakesRoomAndClosesGaps(t *testing.T) {
	s, p, sc := liveScene(t, `video "t"
scene one
  row #r flow=live gap=10
    box #a "a" w=50 h=40
    box #n "n" w=50 h=40
    box #b "b" w=50 h=40
  show #n
  leave #a
`)
	lay := s.Layouts(p)[sc]
	a, n, b := byID(sc, "a"), byID(sc, "n"), byID(sc, "b")
	tl := p.Scenes[0].Timeline()
	before, between, after := lay.At(0), lay.At(tl[0].Start+tl[0].Dur+.01), lay.At(tl[1].Start+tl[1].Dur+.01)
	if gap := before.Boxes[b].X - before.Boxes[a].Right(); gap > 11 {
		t.Fatalf("before the show, b is %.0f after a; #n should take no room", gap)
	}
	if between.Boxes[n].X <= between.Boxes[a].Right() || between.Boxes[b].X <= between.Boxes[n].Right() {
		t.Fatalf("after the show, n should sit between a and b: %v %v %v", between.Boxes[a], between.Boxes[n], between.Boxes[b])
	}
	if gap := after.Boxes[b].X - after.Boxes[n].Right(); gap > 11 || after.Boxes[n].X >= between.Boxes[n].X {
		t.Fatalf("after a leaves, n and b should close up to the left: %v %v", after.Boxes[n], after.Boxes[b])
	}
	mid := lay.At(tl[0].Start + tl[0].Dur*0.5).Boxes[b].X
	if !(mid > before.Boxes[b].X && mid < between.Boxes[b].X) {
		t.Fatalf("b should glide while room opens: %.1f not between %.1f and %.1f", mid, before.Boxes[b].X, between.Boxes[b].X)
	}
}

// set changes an element's text, also through a component's parts, and the
// layout takes its new size.
func TestSetChangesText(t *testing.T) {
	s, p, sc := liveScene(t, `video "t"
scene one
  row gap=10
    box #a "1"
    node #n "3"
  set #a "a much longer label"
  set #n "4"
`)
	lay := s.Layouts(p)[sc]
	end := lay.At(p.Scenes[0].Dur)
	a := byID(sc, "a")
	if end.Text[a] != "a much longer label" || end.Boxes[a].W <= lay.Boxes[a].W {
		t.Fatalf("text %q, width %.0f -> %.0f", end.Text[a], lay.Boxes[a].W, end.Boxes[a].W)
	}
	found := false
	byID(sc, "n").Walk(func(e *c.Element) {
		found = found || (e.Kind() == "box" && end.Text[e] == "4")
	})
	if !found {
		t.Fatal("set on a node should change the value its box shows")
	}
}

// A connector whose end matches several elements joins each of them, and
// send to= picks one.
func TestConnectorFansOut(t *testing.T) {
	_, p, sc := liveScene(t, `video "t"
scene one
  row gap=60
    box #lb "lb"
    col gap=10
      box .srv #s1 "1"
      box .srv #s2 "2"
      box .srv #s3 "3"
  #lb -> .srv
  send #lb to=#s2
`)
	n := 0
	sc.Root.Walk(func(e *c.Element) {
		if e.Edge != nil {
			n++
		}
	})
	if n != 3 {
		t.Fatalf("%d connectors; want one per server", n)
	}
	pk := p.Scenes[0].At(p.Scenes[0].Timeline()[0].Start + .3).Packets()
	if len(pk) != 1 {
		t.Fatalf("%d packets", len(pk))
	}
	if _, to := pk[0].Edge.Edge.Ends(sc.Root); len(to) == 0 || to[0].ID() != "s2" {
		t.Fatalf("send to=#s2 went to %v", to)
	}
}

// A table lines its cells up in columns, and a row shown later takes no
// room until it is shown.
func TestTableAlignsColumns(t *testing.T) {
	s, p, sc := liveScene(t, `video "t"
scene one
  table #t
    row
      label #h1 "key"
      label #h2 "value"
    row #r1
      p #k1 "a much longer key"
      p #v1 "1"
    row #r2
      p #k2 "b"
      p #v2 "2"
  show #r2
`)
	lay := s.Layouts(p)[sc]
	start := lay.At(0)
	if start.Boxes[byID(sc, "v1")].X != start.Boxes[byID(sc, "h2")].X {
		t.Fatalf("value column should line up: %v %v", start.Boxes[byID(sc, "v1")], start.Boxes[byID(sc, "h2")])
	}
	tl := p.Scenes[0].Timeline()
	end := lay.At(tl[0].Start + tl[0].Dur + .01)
	if end.Boxes[byID(sc, "t")].H <= start.Boxes[byID(sc, "t")].H {
		t.Fatalf("table should grow when #r2 is shown: %v then %v", start.Boxes[byID(sc, "t")], end.Boxes[byID(sc, "t")])
	}
	if end.Boxes[byID(sc, "v2")].X != end.Boxes[byID(sc, "v1")].X {
		t.Fatal("new row's cells should line up too")
	}
}
