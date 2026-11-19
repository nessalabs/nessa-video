// Package motion gives steps their meaning over time. It turns a video's
// scenes into a plan: when each element enters and leaves, when states
// change, where the camera points and which caption shows. A frame is then
// a pure function of time, so frames can be drawn in parallel.
package motion

import (
	"math"
	"strconv"
	"strings"
	"time"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/source"
)

// Timings are durations in seconds, normally read from the theme.
type Timings struct {
	Draw, Fade, Rise, State, Camera, Send, CharTyping float64
	Stagger, PartStagger, Lead, Hold, WordsPerSec     float64
}

// DefaultTimings match the hairline theme.
func DefaultTimings() Timings {
	return Timings{Draw: .6, Fade: .35, Rise: .45, State: .3, Camera: .7, Send: .8, CharTyping: .02,
		Stagger: .15, PartStagger: .06, Lead: .2, Hold: .8, WordsPerSec: 2.8}
}

// Anim is one timed effect on one element, in scene seconds.
type Anim struct {
	Mode       string // draw, fade, rise, type, none
	Start, Dur float64
}

func (a Anim) progress(t float64) float64 {
	if a.Dur <= 0 {
		if t >= a.Start {
			return 1
		}
		return 0
	}
	return Ease(clamp((t - a.Start) / a.Dur))
}

func (a Anim) end() float64 { return a.Start + a.Dur }

// StateChange moves an element from one state to another.
type StateChange struct {
	El       *c.Element
	From, To string
	Start    float64
	Dur      float64
}

// CameraMove frames a target (nil for the whole stage).
type CameraMove struct {
	From, To   *c.Element
	Start, Dur float64
}

// Caption is a line of text shown from Start until the next caption or the
// scene's end. Step is the say step's index in its scene; Speech is the
// narration length when the line is spoken.
type Caption struct {
	Text   string
	Start  float64
	Step   int
	Speech float64
	Words  []c.WordTime
	Said   []c.SaidWord // the spoken words, timed from the voice when it spoke
}

// Mark changes which lines of a code block, or cells of an array, are emphasised.
type Mark struct {
	Lines      source.Selection
	Cells      []int
	Start, Dur float64
}

// PointerMove moves a named array pointer to another cell.
type PointerMove struct {
	Name       string
	To         int
	Start, Dur float64
}

// SwapMove exchanges two array cells.
type SwapMove struct {
	I, J       int
	Start, Dur float64
}

// Send moves a packet along a connector.
type Send struct {
	Edge       *c.Element
	Start, Dur float64
}

// ScenePlan is the timeline of one scene.
type ScenePlan struct {
	Scene      *c.Scene
	Start, Dur float64 // Start is in video seconds

	enter    map[*c.Element]Anim
	exit     map[*c.Element]Anim
	types    map[*c.Element]Anim
	initial  map[*c.Element]string
	changes  map[*c.Element][]StateChange
	camera   []CameraMove
	camInit  *c.Element
	captions []Caption
	sends    []Send
	timeline []StepTime
	marks    map[*c.Element][]Mark
	ptrs     map[*c.Element][]PointerMove
	swaps    map[*c.Element][]SwapMove
	plays    map[*c.Element][]PlaySeg
	endState map[*c.Element]string // by identity
	endCam   *c.Element            // identity
	closeAt  float64               // when exits for the next scene's clear begin; 0 if none
	stepsEnd float64               // when the last step finishes
}

// Plan is the whole video's timeline.
type Plan struct {
	Scenes []*ScenePlan
	Total  float64
}

// New plans every scene of v.
func New(v *c.Video, tm Timings) *Plan {
	p := &Plan{}
	var prev *ScenePlan
	for i, sc := range v.Scenes {
		sp := planScene(sc, prev, tm)
		// elements that the next scene clears leave at the end of this one
		if i+1 < len(v.Scenes) {
			sp.planClearExits(v.Scenes[i+1], tm)
		}
		sp.Start = p.Total
		p.Total += sp.Dur
		p.Scenes = append(p.Scenes, sp)
		prev = sp
	}
	return p
}

// At finds the scene playing at video time t and the time within it.
func (p *Plan) At(t float64) (*ScenePlan, float64) {
	for _, sp := range p.Scenes {
		if t < sp.Start+sp.Dur {
			return sp, math.Max(0, t-sp.Start)
		}
	}
	last := p.Scenes[len(p.Scenes)-1]
	return last, last.Dur
}

func modeDur(mode string, tm Timings) float64 {
	switch mode {
	case "draw":
		return tm.Draw
	case "rise":
		return tm.Rise
	case "none":
		return 0
	default:
		return tm.Fade
	}
}

func prop(e *c.Element, key, def string) string {
	if v, ok := e.Computed[key]; ok {
		return v.Value
	}
	return def
}

func seconds(s string) (float64, bool) {
	d, err := time.ParseDuration(strings.TrimPrefix(s, "+"))
	if err != nil {
		return 0, false
	}
	return d.Seconds(), true
}

func planScene(sc *c.Scene, prev *ScenePlan, tm Timings) *ScenePlan {
	sp := &ScenePlan{Scene: sc, enter: map[*c.Element]Anim{}, exit: map[*c.Element]Anim{},
		types: map[*c.Element]Anim{}, initial: map[*c.Element]string{}, changes: map[*c.Element][]StateChange{},
		marks: map[*c.Element][]Mark{},
		ptrs:  map[*c.Element][]PointerMove{}, swaps: map[*c.Element][]SwapMove{},
		plays:    map[*c.Element][]PlaySeg{},
		endState: map[*c.Element]string{}}

	// carried elements keep their state and the camera keeps its target
	byIdentity := map[*c.Element]*c.Element{}
	prevByID := map[*c.Element]*c.Element{}
	if prev != nil {
		prev.Scene.Root.Walk(func(e *c.Element) { prevByID[e.Identity()] = e })
	}
	sc.Root.Walk(func(e *c.Element) {
		byIdentity[e.Identity()] = e
		if prev != nil && e.Carried {
			if s, ok := prev.endState[e.Identity()]; ok {
				sp.initial[e] = s
			}
			// pointers and marks stay where the last scene left them
			if pe := prevByID[e.Identity()]; pe != nil {
				for _, mv := range prev.ptrs[pe] {
					mv.Start, mv.Dur = -1, 0
					sp.ptrs[e] = append(sp.ptrs[e], mv)
				}
				if mk := prev.marks[pe]; len(mk) > 0 {
					last := mk[len(mk)-1]
					last.Start, last.Dur = -1, 0
					sp.marks[e] = append(sp.marks[e], last)
				}
			}
		}
	})
	if prev != nil && prev.endCam != nil {
		sp.camInit = byIdentity[prev.endCam]
	}

	// elements a show step names wait for it instead of entering now
	waiting := map[*c.Element]bool{}
	for _, st := range sc.Steps {
		if st.Verb == "show" && st.Target != nil {
			for _, e := range c.Select(sc.Root, st.Target) {
				waiting[e] = true
			}
		}
	}
	// inWaiting reports whether e, or something holding it, waits for show
	inWaiting := func(e *c.Element) bool {
		for x := e; x != nil; x = x.ParentElement() {
			if waiting[x] {
				return true
			}
		}
		return false
	}

	// entering: new top-level elements one after another, parts staggered
	cursor := tm.Lead
	var enterEnd float64
	showing := false // place is entering an element for a show step
	var place func(e *c.Element, start float64)
	place = func(e *c.Element, start float64) {
		if e.Carried || e.Edge != nil || (waiting[e] && !showing) {
			return
		}
		mode := prop(e, "enter", "fade")
		a := Anim{Mode: mode, Start: start, Dur: modeDur(mode, tm)}
		sp.enter[e] = a
		enterEnd = math.Max(enterEnd, a.end())
		stagger := tm.PartStagger
		if v, ok := seconds(prop(e, "stagger", "")); ok {
			stagger = v
		}
		i := 0
		for _, ch := range e.Children {
			if ch.Edge != nil {
				continue
			}
			place(ch, start+float64(i)*stagger)
			i++
		}
	}
	for _, e := range sc.Root.Children {
		if e.Carried || e.Edge != nil {
			continue
		}
		place(e, cursor)
		cursor += tm.Stagger
	}
	// connectors draw once both ends are visible; those touching an element
	// that waits for show are timed after the steps
	var later []*c.Element
	connect := func(e *c.Element, from float64) float64 {
		start := from
		for _, end := range endsOf(sc, e) {
			if a, ok := sp.enter[end]; ok {
				start = math.Max(start, a.end())
			}
		}
		mode := prop(e, "enter", "draw")
		a := Anim{Mode: mode, Start: start, Dur: modeDur(mode, tm)}
		sp.enter[e] = a
		return a.end()
	}
	sc.Root.Walk(func(e *c.Element) {
		if e.Edge == nil || e.Carried {
			return
		}
		for _, end := range endsOf(sc, e) {
			if inWaiting(end) {
				later = append(later, e)
				return
			}
		}
		if o := e.OwnerElement(); o != nil && inWaiting(o) {
			later = append(later, e)
			return
		}
		enterEnd = math.Max(enterEnd, connect(e, tm.Lead))
	})

	// steps
	state := func(e *c.Element) string {
		if ch := sp.changes[e]; len(ch) > 0 {
			return ch[len(ch)-1].To
		}
		if s, ok := sp.initial[e]; ok {
			return s
		}
		return prop(e, "state", "rest")
	}
	cam := sp.camInit
	t := enterEnd
	if t > 0 {
		t += 0.15
	}
	prevStart := t
	var captionNeed, voiceFree float64
	var hold float64
	var line *spokenLine // the last say line, for on=
	// the scene's opening line may start as the scene draws in when only
	// instant steps (clear, or with-less state set-up) come before it
	firstStep := 0
	for firstStep < len(sc.Steps) && sc.Steps[firstStep].Verb == "clear" {
		firstStep++
	}
	for si, st := range sc.Steps {
		start := t
		if st.With {
			start = prevStart
		}
		phrase, onWord := st.Lookup("on")
		if onWord && line != nil {
			start = line.at(phrase)
		}
		if v, ok := st.Lookup("at"); ok {
			if d, ok := seconds(v); ok {
				if strings.HasPrefix(v, "+") {
					start = prevStart + d
				} else {
					start = d
				}
			}
		}
		var targets []*c.Element
		if st.Target != nil {
			targets = c.Select(sc.Root, st.Target)
		}
		dur := 0.0
		switch st.Verb {
		case "focus", "dim", "active", "select", "trace", "rest", "mark":
			dur = tm.State
		case "leave":
			if len(targets) > 0 {
				dur = modeDur(prop(targets[0], "exit", "fade"), tm)
			}
		case "camera":
			dur = tm.Camera
		case "send":
			dur = tm.Send
		case "ptr":
			dur = .45
		case "swap":
			dur = .6
		case "play":
			dur = 2
		case "show":
			for _, e := range targets {
				dur = math.Max(dur, modeDur(prop(e, "enter", "fade"), tm))
			}
		case "walk":
			dur = float64(len(st.Args)) * .45
		case "type":
			chars := 0
			for _, e := range targets {
				for _, ln := range e.Body {
					chars += len(ln)
				}
				chars += len(e.Text)
			}
			dur = math.Max(.4, float64(chars)*tm.CharTyping)
		case "wait":
			if len(st.Args) > 0 {
				dur, _ = seconds(st.Args[0])
			}
		case "hold":
			if len(st.Args) > 0 {
				h, _ := seconds(st.Args[0])
				hold += h
			}
		}
		// a play started while a line is spoken lasts the rest of the line,
		// so a diagram unfolds as it is talked through
		_, stepwise := st.Lookup("to")
		if st.Verb == "play" && stepwise {
			dur = .8 // one step of a diagram told piece by piece
		} else if st.Verb == "play" && line != nil && start >= line.start && start < line.start+line.speech {
			dur = math.Max(1, line.start+line.speech-start)
		}
		if v, ok := st.Lookup("for"); ok {
			if d, ok := seconds(v); ok {
				dur = d
			}
		}

		switch st.Verb {
		case "focus", "dim", "active", "select", "trace", "rest":
			for _, e := range targets {
				sp.changes[e] = append(sp.changes[e], StateChange{El: e, From: state(e), To: st.Verb, Start: start, Dur: dur})
			}
		case "leave":
			for _, e := range targets {
				mode := prop(e, "exit", "fade")
				if v, ok := st.Lookup("exit"); ok {
					mode = v
				}
				sp.exit[e] = Anim{Mode: mode, Start: start, Dur: dur}
			}
		case "camera":
			var to *c.Element
			if len(st.Args) == 0 || st.Args[0] != "reset" {
				if len(targets) > 0 {
					to = targets[0]
				}
			}
			sp.camera = append(sp.camera, CameraMove{From: cam, To: to, Start: start, Dur: dur})
			cam = to
		case "send":
			for _, e := range targets {
				if edge := edgeFrom(sc, e); edge != nil {
					sp.sends = append(sp.sends, Send{Edge: edge, Start: start, Dur: dur})
				}
			}
		case "type":
			for _, e := range targets {
				sp.types[e] = Anim{Mode: "type", Start: start, Dur: dur}
			}
		case "mark":
			mk := Mark{Start: start, Dur: dur}
			if v, ok := st.Lookup("lines"); ok {
				mk.Lines, _ = source.ParseSelection(v)
			}
			if v, ok := st.Lookup("cells"); ok {
				mk.Cells, _ = source.ParseCells(v)
			}
			for _, e := range targets {
				sp.marks[e] = append(sp.marks[e], mk)
			}
		case "ptr":
			for _, e := range targets {
				for _, d := range st.Decls {
					if i, err := strconv.Atoi(d.Value); err == nil && !c.TimingProp(d.Key) {
						sp.ptrs[e] = append(sp.ptrs[e], PointerMove{Name: d.Key, To: i, Start: start, Dur: dur})
					}
				}
			}
		case "swap":
			if len(st.Args) == 2 {
				i, err1 := strconv.Atoi(st.Args[0])
				j, err2 := strconv.Atoi(st.Args[1])
				if err1 == nil && err2 == nil {
					for _, e := range targets {
						sp.swaps[e] = append(sp.swaps[e], SwapMove{I: i, J: j, Start: start, Dur: dur})
					}
				}
			}
		case "play":
			for _, e := range targets {
				from := 0.0
				if segs := sp.plays[e]; len(segs) > 0 {
					from = segs[len(segs)-1].To
				}
				to := 1.0
				if v, ok := st.Lookup("to"); ok {
					to = playTo(e, v)
				}
				sp.plays[e] = append(sp.plays[e], PlaySeg{Start: start, Dur: dur, From: from, To: to})
			}
		case "show":
			showing = true
			for _, e := range targets {
				place(e, start)
			}
			showing = false
		case "walk":
			for _, e := range targets {
				var prev *c.Element
				for k, id := range st.Args {
					node := partOf(e, id)
					if node == nil {
						continue
					}
					at := start + float64(k)*.45
					sp.changes[node] = append(sp.changes[node], StateChange{El: node, From: state(node), To: "focus", Start: at, Dur: tm.State})
					if prev != nil {
						if link := linkBetween(e, prev, node); link != nil {
							sp.changes[link] = append(sp.changes[link], StateChange{El: link, From: state(link), To: "focus", Start: at - .2, Dur: tm.State})
						}
					}
					prev = node
				}
			}
		case "say":
			// The first line starts as the scene draws in, so the voice
			// carries from one scene into the next without a silent gap.
			_, timed := st.Lookup("at")
			if si == firstStep && !st.With && !timed && !onWord {
				start = math.Min(start, tm.Lead+0.3)
			}
			// narration never overlaps: a line waits for the previous one
			start = math.Max(start, voiceFree)
			speech := st.Speech
			if speech <= 0 {
				words := len(strings.Fields(strings.ReplaceAll(st.Spoken(), "==", "")))
				speech = float64(words) / tm.WordsPerSec
			}
			sp.captions = append(sp.captions, Caption{Text: st.Text, Start: start, Step: si, Speech: st.Speech, Words: st.Words, Said: st.Said})
			voiceFree = start + speech + 0.2
			line = &spokenLine{start: start, speech: speech, said: st.Said, spoken: st.Spoken()}
			captionNeed = math.Max(captionNeed, start+speech)
			// the next step waits for this line, like any other step;
			// `with` puts a step alongside the line instead
			if _, set := st.Lookup("for"); !set {
				dur = speech + 0.2
			}
		}
		text := st.Text
		if onWord {
			text = "on " + phrase
		}
		sp.timeline = append(sp.timeline, StepTime{Verb: st.Verb, Line: st.Pos.Line, Start: start, Dur: dur, Target: targetSrc(st), Text: text})
		prevStart = start
		t = math.Max(t, start+dur)
		if !st.With && !onWord {
			t = start + dur
		}
	}
	for _, e := range later {
		connect(e, 0)
	}
	sp.stepsEnd = t
	sp.Dur = math.Max(t+tm.Hold+hold, captionNeed+0.5)
	if sc.Duration > 0 {
		sp.Dur = sc.Duration.Seconds()
	}

	// end state, keyed by identity, for the next scene
	sc.Root.Walk(func(e *c.Element) { sp.endState[e.Identity()] = state(e) })
	if cam != nil {
		sp.endCam = cam.Identity()
	}
	return sp
}

// planClearExits makes elements dropped by the next scene's clear leave at
// the end of this scene, extending it by the exit time.
func (sp *ScenePlan) planClearExits(next *c.Scene, tm Timings) {
	if len(next.Steps) == 0 && len(next.Root.Children) == len(sp.Scene.Root.Children) {
		return
	}
	kept := map[*c.Element]bool{}
	for _, e := range next.Root.Children {
		if e.Carried {
			kept[e.Identity()] = true
		}
	}
	// holds reports whether e is kept or contains something kept
	var holds func(e *c.Element) bool
	holds = func(e *c.Element) bool {
		if kept[e.Identity()] {
			return true
		}
		for _, ch := range e.Children {
			if holds(ch) {
				return true
			}
		}
		return false
	}
	var longest float64
	sp.closeAt = sp.Dur
	var leave func(e *c.Element)
	leave = func(e *c.Element) {
		if kept[e.Identity()] {
			return
		}
		if holds(e) { // a container of something kept: only its other parts leave
			for _, ch := range e.Children {
				leave(ch)
			}
			return
		}
		if _, gone := sp.exit[e]; gone {
			return
		}
		mode := prop(e, "exit", "fade")
		d := modeDur(mode, tm)
		sp.exit[e] = Anim{Mode: mode, Start: sp.Dur, Dur: d}
		longest = math.Max(longest, d)
	}
	for _, e := range sp.Scene.Root.Children {
		leave(e)
	}
	sp.Dur += longest
}

func endsOf(sc *c.Scene, edge *c.Element) []*c.Element {
	scope := sc.Root
	if o := edge.OwnerElement(); o != nil {
		scope = o
	}
	var out []*c.Element
	if edge.Edge.From != nil {
		if els := c.Select(scope, edge.Edge.From); len(els) > 0 {
			out = append(out, els[0])
		}
	}
	if edge.Edge.To != nil {
		if els := c.Select(scope, edge.Edge.To); len(els) > 0 {
			out = append(out, els[0])
		}
	}
	return out
}

// edgeFrom finds the first connector that starts at e.
func edgeFrom(sc *c.Scene, e *c.Element) *c.Element {
	var found *c.Element
	sc.Root.Walk(func(x *c.Element) {
		if found != nil || x.Edge == nil {
			return
		}
		if ends := endsOf(sc, x); len(ends) > 0 && ends[0] == e {
			found = x
		}
	})
	return found
}

// Ease is the standard ease-in-out curve used for all motion.
func Ease(x float64) float64 {
	x = clamp(x)
	if x < .5 {
		return 4 * x * x * x
	}
	return 1 - math.Pow(-2*x+2, 3)/2
}

func clamp(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// TimingsFrom reads timing tokens like "motion-draw" = "0.6" over the defaults.
func TimingsFrom(tokens map[string]string) Timings {
	tm := DefaultTimings()
	set := func(key string, dst *float64) {
		if v, ok := tokens[key]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				*dst = f
			}
		}
	}
	set("motion-draw", &tm.Draw)
	set("motion-fade", &tm.Fade)
	set("motion-rise", &tm.Rise)
	set("motion-state", &tm.State)
	set("motion-camera", &tm.Camera)
	set("motion-send", &tm.Send)
	set("motion-stagger", &tm.Stagger)
	set("motion-hold", &tm.Hold)
	return tm
}

// partOf finds the part named id inside owner.
func partOf(owner *c.Element, id string) *c.Element {
	var found *c.Element
	owner.Walk(func(x *c.Element) {
		if found == nil && x.OwnerElement() == owner && x.PartName() == id {
			found = x
		}
	})
	return found
}

// linkBetween finds a connector inside owner from a to b.
func linkBetween(owner, a, b *c.Element) *c.Element {
	var found *c.Element
	for _, x := range owner.Children {
		if found != nil || x.Edge == nil || x.Edge.From == nil || x.Edge.To == nil {
			continue
		}
		if x.Edge.From.Matches(a) && x.Edge.To.Matches(b) {
			found = x
		}
	}
	return found
}

// StepTime is when one step runs, in scene seconds.
type StepTime struct {
	Verb, Target, Text string
	Line               int
	Start, Dur         float64
}

// Timeline returns when each step of the scene runs.
func (sp *ScenePlan) Timeline() []StepTime { return sp.timeline }

func targetSrc(st c.Step) string {
	if st.Target != nil {
		return st.Target.Src
	}
	return strings.Join(st.Args, " ")
}

// Cue is one narrated line placed on the video's timeline.
type Cue struct {
	At    float64 // seconds into the video
	Scene int     // index into the video's scenes
	Step  int     // index of the say step in that scene
}

// Narration lists every say step that has speech, in time order.
func (p *Plan) Narration() []Cue {
	var out []Cue
	for i, sp := range p.Scenes {
		for _, cp := range sp.captions {
			if cp.Speech > 0 {
				out = append(out, Cue{At: sp.Start + cp.Start, Scene: i, Step: cp.Step})
			}
		}
	}
	return out
}

// spokenLine is a say line as on= sees it: when it starts and when each
// of its words is said.
type spokenLine struct {
	start, speech float64
	said          []c.SaidWord
	spoken        string
}

// HighlightLead starts a step this long before its word is said, so the
// eye arrives as the ear does.
const HighlightLead = 0.12

// at returns the scene time a step with on=phrase starts: just before the
// phrase's first word is said. Without word timings it estimates the word's
// place from its position in the line. A phrase the line does not say
// starts with the line; reel check reports it.
func (l *spokenLine) at(phrase string) float64 {
	if len(l.said) > 0 {
		var words []string
		var idx []int
		for i, w := range l.said {
			for _, n := range c.SpokenWords(w.Text) {
				words = append(words, n)
				idx = append(idx, i)
			}
		}
		if k := c.FindWords(words, phrase); k >= 0 {
			return l.start + math.Max(0, l.said[idx[k]].Start-HighlightLead)
		}
		return l.start
	}
	words := c.SpokenWords(l.spoken)
	k := c.FindWords(words, phrase)
	if k < 0 || len(words) == 0 {
		return l.start
	}
	return l.start + math.Max(0, l.speech*float64(k)/float64(len(words))-HighlightLead)
}

// PlaySeg moves a block's playhead from one place to another, 0 to 1.
type PlaySeg struct {
	Start, Dur, From, To float64
}

// playTo turns a play step's to= into a playhead position: a count of rows
// for a sequence diagram, a time for a timeline, else a fraction.
func playTo(e *c.Element, v string) float64 {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 1
	}
	switch {
	case e.Sequence != nil:
		rows := 0
		for _, it := range e.Sequence.Items {
			if it.Kind != "close" {
				rows++
			}
		}
		if rows > 0 {
			return clamp(f / float64(rows))
		}
	case e.Timeline != nil:
		end := 0.0
		for _, l := range e.Timeline.Lanes {
			for _, sp := range l.Spans {
				end = math.Max(end, sp.To)
			}
		}
		if end > 0 {
			return clamp(f / end)
		}
	}
	return clamp(f)
}
