package stage

import (
	"testing"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/motion"
)

// A send with a reply gives its connectors two lanes: the request rides
// the upper one, the reply the lower one, and a send to a far element
// passes through the ones between.
func TestReplyTravelsItsOwnLane(t *testing.T) {
	_, p, sc := liveScene(t, `video "t"
scene one
  row gap=80
    box #a "a"
    box #b "b"
    box #c "c"
  #a -> #b
  #b -> #c
  send #a to=#c "req" reply="200"
`)
	sp := p.Scenes[0]
	st := sp.Timeline()[0]
	var lanes []bool
	sc.Root.Walk(func(e *c.Element) {
		if e.Edge != nil {
			lanes = append(lanes, sp.At(0).Lanes(e))
		}
	})
	if len(lanes) != 2 || !lanes[0] || !lanes[1] {
		t.Fatalf("both connectors should have two lanes: %v", lanes)
	}
	var sawGo, sawBack, sawFar bool
	for x := st.Start; x < st.Start+st.Dur; x += 0.05 {
		for _, pk := range sp.At(x).Packets() {
			if pk.Lane == -1 && pk.Text == "req" {
				sawGo = true
			}
			if pk.Lane == 1 && pk.Text == "200" {
				sawBack = true
			}
			if _, to := pk.Edge.Edge.Ends(sc.Root); len(to) > 0 && to[0].ID() == "c" {
				sawFar = true
			}
		}
	}
	if !sawGo || !sawBack || !sawFar {
		t.Fatalf("request out %v, reply back %v, reached c %v", sawGo, sawBack, sawFar)
	}
	// on screen, the lanes sit apart, so request and reply never share a line
	line := [][2]float64{{0, 100}, {200, 100}}
	up, down := offsetPath(line, -LaneOffset), offsetPath(line, LaneOffset)
	if gap := down[0][1] - up[0][1]; gap < cardH {
		t.Fatalf("lanes %.0f apart; cards on them would overlap", gap)
	}
}

// A request that spends a meter's token takes one: the meter drops by one
// and the token ends on the card.
func TestSendSpendsAToken(t *testing.T) {
	s, p, sc := liveScene(t, `video "t"
scene one
  row gap=80
    box #a "a"
    limiter #rl tokens=3
    box #api "api"
  #a -> #rl
  #rl -> #api
  send #a to=#api "req" spend=#rl
`)
	sp := p.Scenes[0]
	st := sp.Timeline()[0]
	lay := s.Layouts(p)[sc].At(sp.Dur)
	m := byID(sc, "rl")
	var meter *c.Element
	m.Walk(func(x *c.Element) {
		if x.Kind() == "meter" {
			meter = x
		}
	})
	if lay.Text[meter] != "2" {
		t.Fatalf("tokens left %q, want 2", lay.Text[meter])
	}
	seated := false
	for _, pk := range sp.At(st.Start + st.Dur - 0.3).Packets() {
		seated = seated || pk.SpendFly >= 1
	}
	if !seated {
		t.Fatal("the token should ride the card to the api")
	}
}

// set … tone= changes a colour smoothly, and on a component it recolours
// the parts toned like its first toned part, not the rest.
func TestSetToneOnAComponent(t *testing.T) {
	_, p, sc := liveScene(t, `video "t"
scene one
  breaker #cb "Closed"
  set #cb "Open" tone=bad
`)
	sp := p.Scenes[0]
	st := sp.Timeline()[0]
	state, box, fails := byID(sc, "cb"), (*c.Element)(nil), (*c.Element)(nil)
	var status *c.Element
	state.Walk(func(x *c.Element) {
		switch x.PartName() {
		case "state":
			status = x
		case "box":
			box = x
		case "fails":
			fails = x
		}
	})
	if _, to, _ := sp.At(st.Start+2).Tone(status, "tone"); to != "bad" {
		t.Fatalf("status tone %q", to)
	}
	if _, to, _ := sp.At(st.Start+2).Tone(box, "outline"); to != "bad" {
		t.Fatalf("box outline %q", to)
	}
	if from, to, mix := sp.At(st.Start+0.1).Tone(status, "tone"); from != "good" || to != "bad" || mix <= 0 || mix >= 1 {
		t.Fatalf("mid change: %q -> %q at %.2f", from, to, mix)
	}
	if len(sp.At(0).Packets()) != 0 || fails == nil {
		t.Fatal("setup")
	}
	if _, to, _ := sp.At(st.Start+2).Tone(fails, "tone"); to != "bad" {
		t.Fatalf("failure dots keep their own tone, got %q", to)
	}
}

// play on a status light shows its countdown ring, full at the start of
// every play.
func TestStatusCountdown(t *testing.T) {
	_, p, sc := liveScene(t, `video "t"
scene one
  status #s "Open" tone=bad
  play #s for=2s
  play #s for=2s
`)
	sp := p.Scenes[0]
	tl := sp.Timeline()
	s := byID(sc, "s")
	if _, op, ok := sp.At(0).Countdown(s); ok && op > 0 {
		t.Fatal("no ring before the play")
	}
	left, op, _ := sp.At(tl[0].Start + 1).Countdown(s)
	if op < 0.99 || left < 0.4 || left > 0.6 {
		t.Fatalf("half way: left %.2f, opacity %.2f", left, op)
	}
	if left, _, _ := sp.At(tl[1].Start + 0.01).Countdown(s); left < 0.95 {
		t.Fatalf("a second play starts full, left %.2f", left)
	}
}

// move #x first takes a child to the front of its list among the children
// on screen; the rest glide down.
func TestMoveFirst(t *testing.T) {
	s, p, sc := liveScene(t, `video "t"
scene one
  col #l flow=live gap=4
    box #n "n"
    box #a "a"
    box #b "b"
  move #b first
  show #n
`)
	lay := s.Layouts(p)[sc]
	tl := p.Scenes[0].Timeline()
	mid := lay.At(tl[0].Start + tl[0].Dur + 0.01)
	a, b := byID(sc, "a"), byID(sc, "b")
	if mid.Boxes[b].Y >= mid.Boxes[a].Y {
		t.Fatalf("b should be above a: %v %v", mid.Boxes[b], mid.Boxes[a])
	}
	end := lay.At(p.Scenes[0].Dur)
	n := byID(sc, "n")
	if end.Boxes[n].Y >= end.Boxes[b].Y {
		t.Fatalf("n, shown later, keeps its written place at the top: %v %v", end.Boxes[n], end.Boxes[b])
	}
	_ = motion.RingFade
}
