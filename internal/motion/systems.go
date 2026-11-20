package motion

import (
	"math"
	"strconv"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/selector"
)

// This file holds the motion behind system diagrams: requests that travel
// several connectors and come back on a lane of their own, packets that
// spend a meter's tokens, colours that change smoothly (`set … tone=`),
// children moved to the front of a list, and countdown rings.

// Leg is one stretch of a send: travelling a connector (travel), turning
// round at the far end while the packet crosses to the reply lane and its
// text changes (turn), or taking a token from a meter (spend).
type Leg struct {
	Kind            string // travel, turn, spend
	Edge            *c.Element
	Start, Dur      float64
	Back            bool // travelling the connector in reverse, on the reply lane
	FadeIn, FadeOut bool
}

func (l Leg) end() float64 { return l.Start + l.Dur }

// Send timing, in seconds: the pause at a node a request passes through,
// the turn at the far end (a labelled packet changes its text there), and
// the hand-over of a token.
const (
	hopGap    = 0.25
	turnDot   = 0.4
	turnCard  = 0.9
	spendTime = 0.75
)

// ToneSet is a `set … tone=` or `outline=` step: El's tone (Key) becomes To
// from Start over Dur.
type ToneSet struct {
	El         *c.Element
	Key, To    string
	Start, Dur float64
}

// OrderChange moves El to the front (first) or back (last) of its
// container; its siblings glide to make room.
type OrderChange struct {
	El         *c.Element
	Where      string
	Start, Dur float64
}

// pathFrom finds the connectors a send from e takes: the one starting at e
// (and ending at to, when given), or else the shortest chain of connectors
// from e to an element to matches, so `send #client to=#db` passes
// through whatever sits between them.
func pathFrom(sc *c.Scene, e *c.Element, to *selector.Selector) []*c.Element {
	if edge := edgeFrom(sc, e, to); edge != nil {
		return []*c.Element{edge}
	}
	if to == nil {
		return nil
	}
	var edges []*c.Element
	sc.Root.Walk(func(x *c.Element) {
		if x.Edge != nil {
			edges = append(edges, x)
		}
	})
	type node struct {
		at   *c.Element
		path []*c.Element
	}
	queue := []node{{at: e}}
	seen := map[*c.Element]bool{e: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, x := range edges {
			ends := endsOf(sc, x)
			if len(ends) < 2 || !within(ends[0], cur.at) || seen[ends[1]] {
				continue
			}
			path := append(append([]*c.Element{}, cur.path...), x)
			if matchesIn(to, ends[1]) {
				return path
			}
			seen[ends[1]] = true
			queue = append(queue, node{ends[1], path})
		}
	}
	return nil
}

// usesLegs reports whether a send is planned as legs: anything beyond a
// plain dot along one connector (and drops and pings, which keep their
// own motion).
func (s *Send) usesLegs() bool {
	if s.Ping || s.Drop {
		return false
	}
	return s.Reply || len(s.Hops) > 1 || s.Text != "" || s.Spend != nil || s.Carry != nil && s.Reply
}

// planLegs lays out a send's legs from start, each travel taking d
// seconds, and returns when the last one ends.
func (s *Send) planLegs(start, d float64) float64 {
	t := start
	n := len(s.Hops)
	turn := turnDot
	if s.Text != "" || s.Carry != nil {
		turn = turnCard
	}
	for i, e := range s.Hops {
		last := i == n-1
		spendHere := i == 0 && s.Spend != nil
		s.Legs = append(s.Legs, Leg{Kind: "travel", Edge: e, Start: t, Dur: d, FadeIn: true,
			FadeOut: !spendHere && (!last || !s.Reply)})
		t += d
		if spendHere {
			s.SpendAt = t
			s.Legs = append(s.Legs, Leg{Kind: "spend", Edge: e, Start: t, Dur: spendTime, FadeOut: !last || !s.Reply})
			t += spendTime
		}
		if !last {
			t += hopGap
		}
	}
	if !s.Reply {
		return t
	}
	s.Legs = append(s.Legs, Leg{Kind: "turn", Edge: s.Hops[n-1], Start: t, Dur: turn})
	t += turn
	for i := n - 1; i >= 0; i-- {
		s.Legs = append(s.Legs, Leg{Kind: "travel", Edge: s.Hops[i], Start: t, Dur: d, Back: true,
			FadeIn: i < n-1, FadeOut: true})
		t += d
		if i > 0 {
			t += hopGap
		}
	}
	return t
}

// legPacket is the packet a send shows at scene time t, if any.
func (s *Send) legPacket(t float64) (Packet, bool) {
	for _, l := range s.Legs {
		if t < l.Start || t > l.end() {
			continue
		}
		f := 1.0
		if l.Dur > 0 {
			f = clamp((t - l.Start) / l.Dur)
		}
		pk := Packet{Edge: l.Edge, Carry: s.Carry, Opacity: 1, Text: s.Text, Alt: s.ReplyText, Lane: -1, Spend: s.Spend, SpendSlot: s.SpendSlot}
		if s.Spend != nil && t >= s.SpendAt {
			pk.SpendFly = 1
		}
		switch l.Kind {
		case "travel":
			p := Ease(f)
			pk.Progress = p
			if l.Back {
				pk.Progress, pk.Lane = 1-p, 1
				if s.ReplyText != "" {
					pk.Text, pk.Alt = s.ReplyText, s.Text
				}
			}
			if l.FadeIn {
				pk.Opacity = math.Min(pk.Opacity, p*8)
			}
			if l.FadeOut {
				pk.Opacity = math.Min(pk.Opacity, (1-p)*8)
			}
		case "spend":
			pk.Progress = 1
			pk.SpendFly = Ease(clamp(f / 0.7))
			if l.FadeOut {
				pk.Opacity = 1 - Ease(clamp((f-0.8)/0.2))
			}
		case "turn":
			pk.Progress = 1
			pk.Lane = -1 + 2*Ease(clamp((f-0.3)/0.55))
			if s.ReplyText != "" && s.ReplyText != s.Text {
				pk.Was, pk.Text = s.Text, s.ReplyText
				pk.TextMix = Ease(clamp(f / 0.45))
			}
		}
		pk.Opacity = clamp(pk.Opacity)
		return pk, true
	}
	return Packet{}, false
}

// Lanes reports whether a connector draws two lanes: requests on one and
// replies coming back on the other. A connector a reply travels gets them;
// lanes=2 or lanes=1 on the connector decides for it.
func (m Moment) Lanes(edge *c.Element) bool { return m.plan.lanes[edge] }

// planLanes marks the connectors replies travel, then applies lanes=.
func (sp *ScenePlan) planLanes() {
	sp.lanes = map[*c.Element]bool{}
	for _, s := range sp.sends {
		if s.Reply && !s.Ping && !s.Drop {
			for _, e := range s.Hops {
				sp.lanes[e] = true
			}
		}
	}
	sp.Scene.Root.Walk(func(e *c.Element) {
		if e.Edge == nil {
			return
		}
		switch prop(e, "lanes", "") {
		case "2":
			sp.lanes[e] = true
		case "1":
			delete(sp.lanes, e)
		}
	})
}

// Tone returns e's tone for key (tone or outline) at this moment: the one
// being left, the one being moved to ("" for none) and progress.
func (m Moment) Tone(e *c.Element, key string) (from, to string, mix float64) {
	to = prop(e, key, "")
	if to == "none" {
		to = ""
	}
	from, mix = to, 1
	for _, ts := range m.plan.tones[e] {
		if ts.Key != key || m.T < ts.Start {
			continue
		}
		from, to = to, ts.To
		mix = 1
		if ts.Dur > 0 {
			mix = Ease((m.T - ts.Start) / ts.Dur)
		}
	}
	return from, to, mix
}

// Toned reports whether e has a tone for key at any time in the scene.
func (m Moment) Toned(e *c.Element, key string) bool {
	if v := prop(e, key, ""); v != "" && v != "none" {
		return true
	}
	for _, ts := range m.plan.tones[e] {
		if ts.Key == key {
			return true
		}
	}
	return false
}

// toneFollowers are the parts of component e that a tone set on it
// recolours: those toned like its first toned part, so a breaker's status
// light and outline change together while its failure dots stay red.
func toneFollowers(e *c.Element) map[*c.Element]string {
	ref := ""
	var parts []*c.Element
	e.Walk(func(x *c.Element) {
		if x != e && x.Edge == nil {
			parts = append(parts, x)
		}
	})
	for _, x := range parts {
		for _, k := range []string{"tone", "outline"} {
			if v := prop(x, k, ""); v != "" && ref == "" {
				ref = v
			}
		}
	}
	out := map[*c.Element]string{}
	if ref == "" {
		return out
	}
	for _, x := range parts {
		for _, k := range []string{"tone", "outline"} {
			if prop(x, k, "") == ref {
				out[x] = k
			}
		}
	}
	return out
}

// planToneSet records a set step's tone= and outline= on its targets.
func (sp *ScenePlan) planToneSet(st c.Step, targets []*c.Element, start, dur float64) {
	for _, key := range []string{"tone", "outline"} {
		v, ok := st.Lookup(key)
		if !ok {
			continue
		}
		if v == "none" {
			v = ""
		}
		for _, e := range targets {
			if !c.IsBuiltinKind(e.Kind()) {
				for part, k := range toneFollowers(e) {
					sp.tones[part] = append(sp.tones[part], ToneSet{El: part, Key: k, To: v, Start: start, Dur: dur})
				}
				continue
			}
			sp.tones[e] = append(sp.tones[e], ToneSet{El: e, Key: key, To: v, Start: start, Dur: dur})
		}
	}
}

// Orders lists the moves to the front or back of a container, in time order.
func (sp *ScenePlan) Orders() []OrderChange { return sp.orders }

// countdowns are the elements whose play steps restart a ring each time:
// timers and status lights, or a component's.
func countdowns(e *c.Element) []*c.Element {
	if e.Kind() == "timer" || e.Kind() == "status" {
		return []*c.Element{e}
	}
	if c.IsBuiltinKind(e.Kind()) {
		return nil
	}
	var out []*c.Element
	e.Walk(func(x *c.Element) {
		if x != e && (x.Kind() == "timer" || x.Kind() == "status") {
			out = append(out, x)
		}
	})
	return out
}

// RingFade is how long a countdown ring takes to appear and to go.
const RingFade = 0.35

// Countdown returns a timer's or status light's ring: the share of it left
// (1 full, 0 empty) and how visible a ring that shows only while it runs
// is. ok is false before its first play step.
func (m Moment) Countdown(e *c.Element) (left, op float64, ok bool) {
	var cur *PlaySeg
	for i, sg := range m.plan.plays[e] {
		if m.T >= sg.Start-RingFade {
			cur = &m.plan.plays[e][i]
		}
	}
	if cur == nil {
		return 1, 0, false
	}
	f := 0.0
	if cur.Dur > 0 {
		f = clamp((m.T - cur.Start) / cur.Dur)
	}
	left = 1 - (cur.From + (cur.To-cur.From)*f)
	end := cur.Start + cur.Dur
	op = math.Min(clamp((m.T-cur.Start+RingFade)/RingFade), clamp((end+RingFade-m.T)/RingFade))
	return left, op, true
}

// meterNum reads a meter's value from its text.
func meterNum(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// firstMeter is e if it is a meter, or the first meter among its parts.
func firstMeter(e *c.Element) *c.Element {
	var out *c.Element
	e.Walk(func(x *c.Element) {
		if out == nil && x.Kind() == "meter" {
			out = x
		}
	})
	return out
}

// Passing returns how far e is through a move to the front or back of its
// list (0 when it is not moving), so the stage can let it pass over its
// neighbours softly instead of through them.
func (m Moment) Passing(e *c.Element) float64 {
	for _, o := range m.plan.orders {
		if o.El == e && m.T > o.Start && m.T < o.Start+o.Dur {
			return (m.T - o.Start) / o.Dur
		}
	}
	return 0
}
