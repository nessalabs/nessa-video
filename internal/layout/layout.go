// Package layout sizes and places a scene's elements. It implements a small
// subset of flexbox and grid: rows, columns, grids, layers and absolute
// places. It works in design units; the stage scales them to frame pixels.
package layout

import (
	"fmt"
	"github.com/nessalabs/nessa-video/internal/markup"
	"math"
	"sort"
	"strconv"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
)

// Rect is a box in design units.
type Rect struct{ X, Y, W, H float64 }

func (r Rect) CX() float64     { return r.X + r.W/2 }
func (r Rect) CY() float64     { return r.Y + r.H/2 }
func (r Rect) Right() float64  { return r.X + r.W }
func (r Rect) Bottom() float64 { return r.Y + r.H }

// Union returns the smallest rect containing r and o.
func (r Rect) Union(o Rect) Rect {
	if r.W == 0 && r.H == 0 {
		return o
	}
	x, y := min(r.X, o.X), min(r.Y, o.Y)
	return Rect{x, y, max(r.Right(), o.Right()) - x, max(r.Bottom(), o.Bottom()) - y}
}

// Measurer is the port for text measurement. The rasterizer adapter implements it.
type Measurer interface {
	Measure(s, font string, size float64) float64
}

// Typesetter is the optional port for maths. It returns the formula's
// outline as SVG path data with its top-left corner at 0,0, and its size.
type Typesetter interface {
	Typeset(tex string, size float64) (path string, w, h float64, err error)
}

// Block sizes in design units, shared with the stage.
const (
	CellSize   = 64.0
	CellGap    = 8.0
	PointerTop = 40.0
	IndexBand  = 26.0
	LaneHeight = 40.0
	LaneLabel  = 120.0
	AxisBand   = 34.0
	MathSize   = 30.0
	SeqColumn  = 180.0
	SeqHead    = 56.0
	SeqRow     = 44.0
	SeqFrame   = 30.0 // a frame's opening or else line: room for its tab
	BarPitch   = 40.0 // a horizontal bar chart's row
	ChartHead  = 40.0 // a horizontal bar chart's header line
	Readout    = 30.0 // the line under a timeline that a mark explains
)

// SeqRowHeight is the height of one sequence diagram item.
func SeqRowHeight(kind string) float64 {
	switch kind {
	case "close":
		return 0
	case "open", "else":
		return SeqFrame
	}
	return SeqRow
}

// Type is a font and size for one kind of text.
type Type struct {
	Font string
	Size float64
}

// Options carry theme values the layout needs.
type Options struct {
	Type      map[string]Type // by element kind: title, p, label, code, metric, box, caption
	Gap       float64         // default gap for rows and columns
	MaxTextW  float64         // wrap width for p
	BlockSize [2]float64      // placeholder size for blocks not drawn yet
}

// Result maps every placed element to its box.
type Result struct {
	Boxes map[*c.Element]Rect
	Lines map[*c.Element][]string // wrapped text lines
	// Fit scales the whole scene down around the area's centre when it is
	// still too big after rows and columns have been flipped. 1 means no scaling.
	Fit     float64
	Content Rect // the laid-out content before Fit
	// SeqX holds each sequence diagram's lifeline positions, from its left edge.
	SeqX map[*c.Element][]float64
	// Came holds, for each top-level element carried from the previous
	// scene, where it was on screen when that scene ended, in design units
	// after the camera. The stage glides it from there to its new place.
	Came map[*c.Element]Rect
	// Text holds text that `set` steps changed by this layout; TextWas and
	// TextMix let the stage cross-fade from the text before.
	Text     map[*c.Element]string
	TextWas  map[*c.Element]string
	TextMix  float64
	LinesWas map[*c.Element][]string
	// Live holds the later layouts of a scene whose live containers change.
	Live []Phase
}

type engine struct {
	m    Measurer
	opt  Options
	st   State // children kept out of the flow and text set by steps
	size map[*c.Element][2]float64
	res  Result
	flip map[*c.Element]bool     // rows and columns turned the other way to fit
	room map[*c.Element]float64  // minimum gap after a child, to fit a connector label
	alts map[*c.Element][]string // every text a set step gives an element in the scene
}

// Scene lays out root's children as a centred column inside area.
//
// The engine fits content to the frame by itself, the way a responsive page
// does: while the content is too wide, the widest row (or left-to-right
// diagram) without an explicit `dir` is turned into a column; while it is
// too tall, the tallest column is turned into a row if that helps. Whatever
// still does not fit is scaled down. Containers with their own `dir`, or
// with `fit=false`, keep their direction.
func Scene(root *c.Element, area Rect, m Measurer, opt Options) Result {
	e := &engine{m: m, opt: opt, flip: map[*c.Element]bool{}, room: map[*c.Element]float64{}}
	e.labelRoom(root)
	w, h := e.fitFlips(root, area)
	return e.finish(root, area, w, h, 0)
}

// fresh clears what one measuring pass computed.
func (e *engine) fresh() {
	e.size = map[*c.Element][2]float64{}
	e.res = Result{Boxes: map[*c.Element]Rect{}, Lines: map[*c.Element][]string{}, Fit: 1, SeqX: map[*c.Element][]float64{}, Text: e.st.Text}
}

// fitFlips measures root, turning rows and columns until it fits.
func (e *engine) fitFlips(root *c.Element, area Rect) (w, h float64) {
	for round := 0; ; round++ {
		e.fresh()
		w, h = e.measure(root)
		if round >= 16 || (w <= area.W && h <= area.H) {
			break
		}
		if w > area.W && e.flipWidest(root, area) {
			continue
		}
		if h > area.H && e.flipTallest(root, area) {
			continue
		}
		break
	}
	// Turning containers is only worth it if the result needs less scaling
	// than leaving them as written; a turned row that still has to shrink
	// a lot is usually worse to read.
	fitOf := func(w, h float64) float64 { return min(1, area.W/w, area.H/h) }
	if len(e.flip) > 0 && fitOf(w, h) < 1 {
		flips, fitFlipped := e.flip, fitOf(w, h)
		e.flip = map[*c.Element]bool{}
		e.fresh()
		w2, h2 := e.measure(root)
		if fitOf(w2, h2) > fitFlipped+0.02 {
			return w2, h2
		}
		e.flip = flips
		e.fresh()
		w, h = e.measure(root)
	}
	return w, h
}

// finish places root centred in area. fit, when not 0, is the scale to
// use instead of the one this layout alone would need.
func (e *engine) finish(root *c.Element, area Rect, w, h, fit float64) Result {
	r := Rect{area.X + (area.W-w)/2, area.Y + (area.H-h)/2, w, h}
	e.place(root, r)
	e.res.Content = r
	// an aside takes no room in the scene: it is placed in the part of the
	// frame its isolated element leaves free
	root.Walk(func(a *c.Element) {
		if str(a, "travel", "") == "true" { // rides connectors; needs only a size
			w, h := e.measure(a)
			e.place(a, Rect{area.CX() - w/2, area.CY() - h/2, w, h})
			return
		}
		if a.Kind() != "aside" {
			return
		}
		aw, ah := e.measure(a)
		lay := str(a, "layout", "left")
		reg := AsideRegion(area, lay)
		x := reg.X
		switch lay {
		case "center":
			x = reg.CX() - aw/2
		case "right":
			x = reg.Right() - aw
		}
		y := reg.CY() - ah/2
		if lay == "center" {
			y = reg.Y
		}
		e.place(a, Rect{x, y, aw, ah})
	})
	if w > area.W || h > area.H {
		e.res.Fit = min(area.W/w, area.H/h)
	}
	if fit > 0 {
		e.res.Fit = fit
	}
	return e.res
}

// State is what steps have changed in a scene's layout at some moment:
// children of live containers that are out of the flow (not shown yet, or
// gone), and text set by `set`.
type State struct {
	Out  map[*c.Element]bool
	Text map[*c.Element]string
}

// Phase is a later layout of a scene that elements glide to, from Start
// over Dur seconds of scene time.
type Phase struct {
	Start, Dur float64
	Result
}

// SceneStates lays a scene out once per state. The first state is the
// result; later ones become its phases, starting at the given times. All
// share the turns and scale of the layout with everything in the flow, so
// a growing list moves its neighbours instead of reshaping the frame.
func SceneStates(root *c.Element, area Rect, m Measurer, opt Options, states []State, at [][2]float64) Result {
	e := &engine{m: m, opt: opt, flip: map[*c.Element]bool{}, room: map[*c.Element]float64{}, alts: map[*c.Element][]string{}}
	for _, st := range states {
		for el, t := range st.Text {
			if len(e.alts[el]) == 0 {
				e.alts[el] = []string{el.Text}
			}
			e.alts[el] = append(e.alts[el], t)
		}
	}
	e.labelRoom(root)
	w, h := e.fitFlips(root, area)
	full := e.finish(root, area, w, h, 0)
	var out Result
	for i, st := range states {
		e.st = st
		e.fresh()
		w, h := e.measure(root)
		r := e.finish(root, area, w, h, full.Fit)
		anchor(root, full, r)
		if i == 0 {
			out = r
			continue
		}
		out.Live = append(out.Live, Phase{Start: at[i][0], Dur: at[i][1], Result: r})
	}
	return out
}

// anchor moves a state's layout so its content starts where the full
// layout's does, top left. Centred afresh, content that grows would push
// everything before it the other way; anchored, an item added to a list
// moves only the items after it, and the whole always fits where the
// full layout does.
func anchor(root *c.Element, full, r Result) {
	span := func(res Result) (Rect, bool) {
		var u Rect
		ok := false
		for _, ch := range root.Children {
			if b, has := res.Boxes[ch]; has && (b.W > 0 || b.H > 0) {
				if !ok {
					u, ok = b, true
				} else {
					u = u.Union(b)
				}
			}
		}
		return u, ok
	}
	f, ok1 := span(full)
	g, ok2 := span(r)
	if !ok1 || !ok2 {
		return
	}
	dx, dy := f.X-g.X, f.Y-g.Y
	if dx == 0 && dy == 0 {
		return
	}
	for el, b := range r.Boxes {
		if el != root {
			r.Boxes[el] = Rect{b.X + dx, b.Y + dy, b.W, b.H}
		}
	}
}

// At returns the layout as it is t seconds into the scene: each phase that
// has started replaces the boxes before it, gliding there over its length.
func (r Result) At(t float64) Result {
	if len(r.Live) == 0 {
		return r
	}
	cur := r
	cur.Live = nil
	for _, ph := range r.Live {
		if t < ph.Start {
			break
		}
		f := 1.0
		if ph.Dur > 0 {
			f = (t - ph.Start) / ph.Dur
		}
		if f >= 1 {
			next := ph.Result
			next.Came = r.Came
			cur = next
			continue
		}
		f = f * f * (3 - 2*f)
		mix := make(map[*c.Element]Rect, len(ph.Boxes))
		for el, b := range ph.Boxes {
			if a, ok := cur.Boxes[el]; ok {
				b = Rect{a.X + (b.X-a.X)*f, a.Y + (b.Y-a.Y)*f, a.W + (b.W-a.W)*f, a.H + (b.H-a.H)*f}
			}
			mix[el] = b
		}
		for el, a := range cur.Boxes { // leaving the flow: stay put while it goes
			if _, ok := mix[el]; !ok {
				mix[el] = a
			}
		}
		next := ph.Result
		next.Boxes = mix
		next.Came = r.Came
		next.TextMix = f
		next.TextWas = cur.Text
		next.LinesWas = cur.Lines
		cur = next
		break
	}
	return cur
}

// labelRoom finds connectors with labels whose ends sit in neighbouring
// children of one container, and records the gap their label needs.
func (e *engine) labelRoom(root *c.Element) {
	t := e.opt.Type["label"]
	root.Walk(func(edge *c.Element) {
		if edge.Edge == nil || edge.Text == "" || edge.Edge.From == nil || edge.Edge.To == nil {
			return
		}
		scope := root
		if o := edge.OwnerElement(); o != nil {
			scope = o
		}
		from, to := edge.Edge.Ends(scope)
		if len(from) == 0 || len(to) == 0 {
			return
		}
		need := e.m.Measure(Plain(edge.Text), t.Font, t.Size) + 40
		// walk up from both ends to the container holding both
		chain := map[*c.Element]*c.Element{} // ancestor -> its child on the path from `from`
		for x := from[0]; x.ParentElement() != nil; x = x.ParentElement() {
			chain[x.ParentElement()] = x
		}
		for y := to[0]; y.ParentElement() != nil; y = y.ParentElement() {
			p := y.ParentElement()
			a, ok := chain[p]
			if !ok {
				continue
			}
			ia, ib := a.Index(), y.Index()
			if ia > ib {
				ia, ib = ib, ia
			}
			if ib-ia == 1 {
				left := p.Children[ia-1]
				e.room[left] = max(e.room[left], need)
			}
			return
		}
	})
}

// gapAfter is the gap after child ch in a container, widened for a label
// when the container lays out horizontally.
func (e *engine) gapAfter(ch *c.Element, gap float64, horizontal bool) float64 {
	if horizontal {
		return max(gap, e.room[ch])
	}
	return gap
}

// flexible reports whether the engine may turn el the other way. A `dir`
// set by a rule or on the element's line is kept; a Mermaid header's
// direction is only a preference. `fit=false` keeps a container as it is.
func flexible(el *c.Element) bool {
	if str(el, "fit", "true") == "false" {
		return false
	}
	v, set := el.Computed["dir"]
	if !set {
		return true
	}
	return el.Kind() == "mermaid" && v.Origin.Rule == nil && v.Origin.Pos == el.Pos
}

// flipWidest turns the widest horizontal container wider than the area into
// a vertical one. It reports whether it changed anything.
func (e *engine) flipWidest(root *c.Element, area Rect) bool {
	var best *c.Element
	var bw float64
	root.Walk(func(el *c.Element) {
		if el == root || e.flip[el] || !flexible(el) || !e.horizontal(el) {
			return
		}
		if k := el.Kind(); k != "row" && k != "mermaid" {
			return
		}
		if w := e.size[el][0]; w > area.W && w > bw {
			best, bw = el, w
		}
	})
	if best == nil {
		return false
	}
	e.flip[best] = true
	return true
}

// flipTallest turns the tallest vertical container taller than the area
// into a horizontal one, when its children would then fit across.
func (e *engine) flipTallest(root *c.Element, area Rect) bool {
	var best *c.Element
	var bh float64
	root.Walk(func(el *c.Element) {
		if e.flip[el] || !flexible(el) || e.horizontal(el) {
			return
		}
		if k := el.Kind(); k != "col" && k != "scene" && k != "mermaid" {
			return
		}
		var across float64
		for _, ch := range el.Children {
			if e.inFlow(ch) {
				across += e.size[ch][0] + e.opt.Gap
			}
		}
		if h := e.size[el][1]; h > area.H && h > bh && across <= area.W {
			best, bh = el, h
		}
	})
	if best == nil {
		return false
	}
	e.flip[best] = true
	return true
}

// horizontal reports whether a container lays its children out in a row.
func (e *engine) horizontal(el *c.Element) bool {
	h := el.Kind() == "row"
	if el.Kind() == "mermaid" {
		h = false
	}
	switch str(el, "dir", "") {
	case "row", "LR", "RL":
		h = true
	case "col", "TD", "TB":
		h = false
	}
	if e.flip[el] {
		h = !h
	}
	return h
}

// Prop helpers read computed values.

func num(el *c.Element, key string, def float64) float64 {
	if v, ok := el.Computed[key]; ok {
		if f, err := strconv.ParseFloat(strings.TrimSuffix(v.Value, "px"), 64); err == nil {
			return f
		}
	}
	return def
}

func str(el *c.Element, key, def string) string {
	if v, ok := el.Computed[key]; ok {
		return v.Value
	}
	return def
}

// TypeOf returns the font and size for an element's text, honouring the
// cascaded font and size properties.
func TypeOf(el *c.Element, opt Options) Type {
	t, ok := opt.Type[el.Kind()]
	if !ok {
		t = opt.Type["box"]
	}
	if f := str(el, "font", ""); f != "" {
		t.Font = FontKey(f)
	}
	t.Size = num(el, "size", t.Size)
	return t
}

// FontKey maps a font property like `sans-600` or `mono` to a font key.
func FontKey(f string) string {
	switch f {
	case "mono":
		return "mono"
	case "sans-500":
		return "sans500"
	case "sans-600":
		return "sans600"
	case "sans", "sans-400":
		return "sans400"
	}
	return f
}

// Plain strips inline marks (==x==, __x__) from text.
func Plain(s string) string {
	return markup.Strip(s)
}

var textKinds = map[string]bool{"title": true, "p": true, "label": true, "metric": true, "code": true}

func (e *engine) inFlow(el *c.Element) bool {
	return el.Kind() != "edge" && el.Kind() != "use" && el.Kind() != "badge" && el.Kind() != "aside" && str(el, "travel", "") != "true" && str(el, "hidden", "false") != "true" && !e.st.Out[el]
}

// textOf is an element's text, as a `set` step may have changed it.
func (e *engine) textOf(el *c.Element) string {
	if t, ok := e.st.Text[el]; ok {
		return t
	}
	return el.Text
}

func (e *engine) measure(el *c.Element) (float64, float64) {
	w, h := e.intrinsic(el)
	w, h = num(el, "w", w), num(el, "h", h)
	e.size[el] = [2]float64{w, h}
	return w, h
}

func (e *engine) intrinsic(el *c.Element) (float64, float64) {
	kind := el.Kind()
	pad := num(el, "pad", 0)
	switch {
	case kind == "row" || kind == "col" || kind == "aside" || kind == "scene" || !c.IsBuiltinKind(kind):
		gap := num(el, "gap", e.opt.Gap)
		if kind == "scene" {
			gap = num(el, "gap", e.opt.Gap*2)
		}
		horizontal := e.horizontal(el)
		var main, cross float64
		var prev *c.Element
		for _, ch := range el.Children {
			if !e.inFlow(ch) {
				continue
			}
			w, h := e.measure(ch)
			if w == 0 && h == 0 {
				continue // empty slot or part
			}
			if prev != nil {
				main += e.gapAfter(prev, gap, horizontal)
			}
			if horizontal {
				main += w
				cross = max(cross, h)
			} else {
				main += h
				cross = max(cross, w)
			}
			prev = ch
		}
		if horizontal {
			return main + 2*pad, cross + 2*pad
		}
		return cross + 2*pad, main + 2*pad
	case kind == "table":
		return e.table(el, nil)
	case kind == "tree":
		return e.tree(el, nil)
	case kind == "grid":
		cols := int(num(el, "cols", 2))
		gap := num(el, "gap", e.opt.Gap)
		var cw, rh float64
		n := 0
		for _, ch := range el.Children {
			if e.inFlow(ch) {
				w, h := e.measure(ch)
				cw, rh = max(cw, w), max(rh, h)
				n++
			}
		}
		rows := (n + cols - 1) / max(cols, 1)
		w := float64(cols)*cw + gap*float64(cols-1)
		h := float64(rows)*rh + gap*float64(max(rows-1, 0))
		return w + 2*pad, h + 2*pad
	case kind == "layer":
		var w, h float64
		for _, ch := range el.Children {
			if e.inFlow(ch) {
				cw, chh := e.measure(ch)
				w, h = max(w, cw), max(h, chh)
			}
		}
		return w, h
	case kind == "place":
		var r Rect
		for _, ch := range el.Children {
			if e.inFlow(ch) {
				w, h := e.measure(ch)
				r = r.Union(Rect{num(ch, "x", 0), num(ch, "y", 0), w, h})
			}
		}
		return r.Right(), r.Bottom()
	case el.Code != nil:
		return e.codePanel(el)
	case kind == "arrow":
		long := num(el, "len", 40)
		if d := str(el, "dir", "right"); d == "down" || d == "up" {
			return 16, long
		}
		return long, 16
	case el.Array != nil:
		n := float64(len(el.Array.Values))
		return n*CellSize + (n-1)*CellGap, PointerTop + CellSize + IndexBand
	case el.Chart != nil:
		if el.Chart.Horizontal {
			return 560, ChartHead + BarPitch*float64(len(el.Chart.Rows)) + 8
		}
		return 460, 300
	case el.Timeline != nil:
		h := 28 + LaneHeight*float64(len(el.Timeline.Lanes)) + AxisBand
		if el.Timeline.Readout {
			h += Readout
		}
		return 600, h
	case kind == "math":
		if ts, ok := e.m.(Typesetter); ok {
			if _, w, h, err := ts.Typeset(el.Text, num(el, "size", MathSize)); err == nil {
				return w, h
			}
		}
		return e.opt.BlockSize[0], 80
	case el.Git != nil:
		states := el.Git.States()
		cols, lanes := 0, 1
		for _, st := range states {
			lanes = max(lanes, len(st.Lanes))
			for _, cm := range st.Commits {
				cols = max(cols, cm.Col+1)
			}
		}
		return GitPad*2 + float64(max(cols-1, 0))*GitCol, GitTop + float64(lanes-1)*GitLane + GitBottom
	case el.Sequence != nil:
		h := SeqHead + 28
		for _, it := range el.Sequence.Items {
			h += SeqRowHeight(it.Kind)
		}
		w := e.sequenceColumns(el)
		return w, h
	case kind == "mermaid":
		return e.graph(el, nil)
	case kind == "metric":
		w, h := e.text(el)
		lab := e.opt.Type["label"]
		for _, key := range []string{"label", "note"} {
			if v := str(el, key, ""); v != "" {
				w = max(w, e.m.Measure(Plain(v), lab.Font, lab.Size))
				h += lab.Size * 1.9
			}
		}
		return w, h
	case textKinds[kind]:
		return e.text(el)
	case kind == "box" || kind == "decision":
		if e.textOf(el) != "" {
			t := TypeOf(el, e.opt)
			tw := e.m.Measure(Plain(e.textOf(el)), t.Font, t.Size)
			if kind == "decision" {
				return max(120, tw+72), 80
			}
			return max(96, tw+40), 48
		}
		if kind == "decision" {
			return 120, 80
		}
		return 120, 64
	case kind == "card":
		return e.card(el)
	case kind == "circle":
		r := num(el, "r", 28)
		return 2 * r, 2 * r
	case kind == "meter":
		n := max(num(el, "max", 5), 1)
		if str(el, "kind", "slots") == "bar" {
			return num(el, "len", 160), MeterSlot
		}
		return n*MeterSlot + (n-1)*MeterGap, MeterSlot
	case kind == "badge":
		t := e.opt.Type["label"]
		return e.m.Measure(Plain(e.textOf(el)), t.Font, t.Size) + 18, 22
	case kind == "timer":
		r := num(el, "r", 16)
		w := 2 * r
		if txt := e.textOf(el); txt != "" {
			t := e.opt.Type["label"]
			w += 10 + e.m.Measure(Plain(txt), t.Font, t.Size)
		}
		return w, 2 * r
	case kind == "store":
		return 100, 80
	case kind == "stack":
		return 132, 76
	case kind == "terminal":
		t := e.opt.Type["code"]
		w := 520.0
		for _, ln := range el.Body {
			w = max(w, e.m.Measure(Plain(ln), t.Font, t.Size)+56)
		}
		return w, 44 + 24 + 30*float64(len(el.Body)) + 12
	default: // blocks not drawn yet
		return e.opt.BlockSize[0], e.opt.BlockSize[1]
	}
}

func (e *engine) text(el *c.Element) (float64, float64) {
	t := TypeOf(el, e.opt)
	line := t.Size * 1.35
	var lines []string
	switch {
	case el.Kind() == "code" && len(el.Body) > 0:
		lines = el.Body
	case el.Kind() == "p":
		lines = e.wrap(e.textOf(el), t, num(el, "w", e.opt.MaxTextW))
	default:
		lines = strings.Split(e.textOf(el), "\n")
	}
	e.res.Lines[el] = lines
	var w float64
	for _, l := range lines {
		w = max(w, e.m.Measure(Plain(l), t.Font, t.Size))
	}
	if e.textOf(el) == "" && len(el.Body) == 0 {
		return 0, 0
	}
	return w, line * float64(len(lines))
}

// Card sizes. Text wraps inside the card; title= sits above a rule.
const (
	CardPad     = 16.0 // around the body text
	CardHead    = 28.0 // height of the title strip
	CardMaxText = 248.0
	CardLine    = 1.45 // body line height, in font sizes
)

// card wraps a card's text to its width (w= or a readable default) and
// sizes the card to fit its title and body.
func (e *engine) card(el *c.Element) (float64, float64) {
	t := TypeOf(el, e.opt)
	head := 0.0
	w := 200.0
	if title := str(el, "title", ""); title != "" {
		head = CardHead
		w = max(w, e.m.Measure(Plain(title), "sans500", 13)+2*12)
	}
	if e.textOf(el) == "" {
		e.res.Lines[el] = nil
		return w, head + 64
	}
	maxText := CardMaxText
	if v := num(el, "w", 0); v > 0 {
		maxText = v - 2*CardPad
	}
	lines := e.wrap(e.textOf(el), t, maxText)
	e.res.Lines[el] = lines
	widest := 0.0
	for _, l := range lines {
		widest = max(widest, e.m.Measure(Plain(l), t.Font, t.Size))
	}
	w = max(w, widest+2*CardPad)
	return w, head + 2*CardPad + t.Size*CardLine*float64(len(lines)) - t.Size*(CardLine-1)
}

func (e *engine) wrap(s string, t Type, maxW float64) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		cur := ""
		for _, w := range strings.Fields(para) {
			try := strings.TrimSpace(cur + " " + w)
			if cur != "" && e.m.Measure(Plain(try), t.Font, t.Size) > maxW {
				out = append(out, cur)
				cur = w
			} else {
				cur = try
			}
		}
		out = append(out, cur)
	}
	return markup.Balance(out)
}

func (e *engine) place(el *c.Element, r Rect) {
	e.res.Boxes[el] = r
	// badges sit on their element's top right corner, outside the flow
	for _, ch := range el.Children {
		if ch.Kind() == "badge" && str(ch, "hidden", "false") != "true" {
			w, h := e.measure(ch)
			e.res.Boxes[ch] = Rect{r.Right() - w*0.6, r.Y - h*0.55, w, h}
		}
	}
	kind := el.Kind()
	pad := num(el, "pad", 0)
	inner := Rect{r.X + pad, r.Y + pad, r.W - 2*pad, r.H - 2*pad}
	switch {
	case kind == "row" || kind == "col" || kind == "aside" || kind == "scene" || !c.IsBuiltinKind(kind):
		horizontal := e.horizontal(el)
		gap := num(el, "gap", e.opt.Gap)
		if kind == "scene" {
			gap = num(el, "gap", e.opt.Gap*2)
		}
		align := str(el, "align", "center")
		var kids []*c.Element
		var used float64
		for _, ch := range el.Children {
			if !e.inFlow(ch) {
				continue
			}
			s := e.size[ch]
			if s[0] == 0 && s[1] == 0 {
				e.res.Boxes[ch] = Rect{inner.CX(), inner.CY(), 0, 0}
				continue
			}
			kids = append(kids, ch)
			if horizontal {
				used += s[0]
			} else {
				used += s[1]
			}
		}
		for k := 0; k+1 < len(kids); k++ {
			used += e.gapAfter(kids[k], gap, horizontal)
		}
		// cards side by side read as a set: give them one height
		if horizontal && str(el, "align", "") == "" && len(kids) > 1 {
			cards := true
			for _, k := range kids {
				cards = cards && k.Kind() == "card"
			}
			if cards {
				align = "stretch"
			}
		}
		pos := inner.X
		if !horizontal {
			pos = inner.Y
		}
		space := inner.W - used
		if !horizontal {
			space = inner.H - used
		}
		switch str(el, "justify", "start") {
		case "center":
			pos += space / 2
		case "end":
			pos += space
		}
		extra := 0.0
		if str(el, "justify", "start") == "between" && len(kids) > 1 {
			extra = space / float64(len(kids)-1)
		}
		for _, ch := range kids {
			s := e.size[ch]
			if horizontal {
				y := crossPos(align, inner.Y, inner.H, s[1])
				h := s[1]
				if align == "stretch" {
					h = inner.H
				}
				e.place(ch, Rect{pos, y, s[0], h})
				pos += s[0] + e.gapAfter(ch, gap, true) + extra
			} else {
				x := crossPos(align, inner.X, inner.W, s[0])
				w := s[0]
				if align == "stretch" {
					w = inner.W
				}
				e.place(ch, Rect{x, pos, w, s[1]})
				pos += s[1] + gap + extra
			}
		}
	case kind == "table":
		e.table(el, &inner)
	case kind == "tree":
		e.tree(el, &inner)
	case kind == "grid":
		cols := max(int(num(el, "cols", 2)), 1)
		gap := num(el, "gap", e.opt.Gap)
		cw := (inner.W - gap*float64(cols-1)) / float64(cols)
		var rh float64
		for _, ch := range el.Children {
			if e.inFlow(ch) {
				rh = max(rh, e.size[ch][1])
			}
		}
		i := 0
		for _, ch := range el.Children {
			if !e.inFlow(ch) {
				continue
			}
			s := e.size[ch]
			col, row := i%cols, i/cols
			cell := Rect{inner.X + float64(col)*(cw+gap), inner.Y + float64(row)*(rh+gap), cw, rh}
			e.place(ch, Rect{cell.CX() - s[0]/2, cell.CY() - s[1]/2, s[0], s[1]})
			i++
		}
	case kind == "layer":
		for _, ch := range el.Children {
			if e.inFlow(ch) {
				s := e.size[ch]
				e.place(ch, Rect{inner.CX() - s[0]/2, inner.CY() - s[1]/2, s[0], s[1]})
			}
		}
	case kind == "place":
		for _, ch := range el.Children {
			if e.inFlow(ch) {
				s := e.size[ch]
				e.place(ch, Rect{inner.X + num(ch, "x", 0), inner.Y + num(ch, "y", 0), s[0], s[1]})
			}
		}
	case kind == "mermaid" && el.Sequence == nil:
		e.graph(el, &inner)
	}
}

// graph sizes (at == nil) or places a Mermaid diagram: ranks become columns
// for LR and rows for TD, and nodes keep their order within a rank.
func (e *engine) graph(el *c.Element, at *Rect) (float64, float64) {
	lr := e.horizontal(el)
	rankGap, nodeGap := 72.0, 36.0
	if lr {
		rankGap = 112 // room for a label on each link's last run
	}
	type rk struct {
		nodes []*c.Element
		main  float64 // size along the rank direction (column width for LR)
	}
	var ranks []*rk
	sizes := map[*c.Element][2]float64{}
	for _, ch := range el.Children {
		if !e.inFlow(ch) {
			continue
		}
		w, h := e.measure(ch)
		sizes[ch] = [2]float64{w, h}
		ri := int(num(ch, "rank", 0))
		for len(ranks) <= ri {
			ranks = append(ranks, &rk{})
		}
		r := ranks[ri]
		r.nodes = append(r.nodes, ch)
		along := h
		if lr {
			along = w
		}
		r.main = max(r.main, along)
	}
	across := func(n *c.Element) float64 {
		if lr {
			return sizes[n][1]
		}
		return sizes[n][0]
	}
	// a node's parents, from the diagram's links
	parents := map[*c.Element][]*c.Element{}
	for _, ch := range el.Children {
		if ch.Edge == nil || ch.Edge.From == nil || ch.Edge.To == nil {
			continue
		}
		from, to := ch.Edge.Ends(el)
		if len(from) > 0 && len(to) > 0 {
			parents[to[0]] = append(parents[to[0]], from[0])
		}
	}
	// Place each rank across the flow: a node sits level with the middle of
	// its parents where it can, so a chain stays one straight line, and
	// siblings spread evenly around their parent.
	mid := map[*c.Element]float64{}
	for ri, r := range ranks {
		want := make([]float64, len(r.nodes))
		total := -nodeGap
		for _, n := range r.nodes {
			total += across(n) + nodeGap
		}
		pos := -total / 2
		for i, n := range r.nodes { // default: centred as a group
			want[i] = pos + across(n)/2
			pos += across(n) + nodeGap
		}
		if ri > 0 {
			for i, n := range r.nodes {
				var ps, pc float64
				for _, p := range parents[n] {
					if m, ok := mid[p]; ok {
						ps += m
						pc++
					}
				}
				if pc > 0 {
					want[i] = ps / pc
				}
			}
			// spread siblings that want the same place, keeping their middle
			order := make([]int, len(r.nodes))
			for i := range order {
				order[i] = i
			}
			sort.SliceStable(order, func(a, b int) bool { return want[order[a]] < want[order[b]] })
			placed := make([]float64, len(r.nodes))
			edge := math.Inf(-1)
			for _, i := range order {
				lo := want[i] - across(r.nodes[i])/2
				lo = max(lo, edge)
				placed[i] = lo + across(r.nodes[i])/2
				edge = lo + across(r.nodes[i]) + nodeGap
			}
			var shift, k float64
			for i := range order {
				shift += want[i] - placed[i]
				k++
			}
			for i := range placed {
				want[i] = placed[i] + shift/k
			}
		}
		for i, n := range r.nodes {
			mid[n] = want[i]
		}
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for n, m := range mid {
		lo, hi = min(lo, m-across(n)/2), max(hi, m+across(n)/2)
	}
	crossMax := max(0, hi-lo)
	var total float64
	for i, r := range ranks {
		if i > 0 {
			total += rankGap
		}
		total += r.main
	}
	if at == nil {
		if lr {
			return total, crossMax
		}
		return crossMax, total
	}
	pos := 0.0
	for _, r := range ranks {
		for _, ch := range r.nodes {
			s := e.size[ch]
			off := mid[ch] - lo - across(ch)/2
			if lr {
				e.place(ch, Rect{at.X + pos + (r.main-s[0])/2, at.Y + off, s[0], s[1]})
			} else {
				e.place(ch, Rect{at.X + off, at.Y + pos + (r.main-s[1])/2, s[0], s[1]})
			}
		}
		pos += r.main + rankGap
	}
	return 0, 0
}

func crossPos(align string, start, avail, size float64) float64 {
	switch align {
	case "start", "stretch":
		return start
	case "end":
		return start + avail - size
	default:
		return start + (avail-size)/2
	}
}

// Code panel metrics in design units, shared with the stage.
const (
	CodeHeader = 40.0
	CodeRow    = 26.0
	CodePad    = 12.0
	CodeGutter = 72.0 // number column, sign and gap before the code
)

func (e *engine) codePanel(el *c.Element) (float64, float64) {
	t := e.opt.Type["code"]
	w := 0.0
	for _, l := range el.Code.Lines {
		w = max(w, e.m.Measure(Untab(l.Text), t.Font, t.Size))
	}
	h := CodeHeader + 2*CodePad + CodeRow*float64(len(el.Code.Lines))
	return max(560, CodeGutter+w+32), h
}

// Untab expands tabs to four spaces for display.
func Untab(s string) string { return strings.ReplaceAll(s, "\t", "    ") }

// SeqHeadWidth is the width of a participant's box for its label.
func SeqHeadWidth(m Measurer, label string, t Type) float64 {
	return max(136, m.Measure(Plain(label), t.Font, t.Size)+36)
}

// sequenceColumns places a sequence diagram's lifelines the way Mermaid
// does: each gap between neighbours is wide enough for the participant
// boxes and for the longest message or note that crosses it, so labels
// never run over lifelines. It records the positions and returns the width.
func (e *engine) sequenceColumns(el *c.Element) float64 {
	sb := el.Sequence
	n := len(sb.Participants)
	lab := e.opt.Type["label"]
	head := e.opt.Type["box"]
	head.Size = 14
	text := func(s string) float64 { return e.m.Measure(Plain(s), lab.Font, lab.Size) }
	hw := make([]float64, n)
	for i, p := range sb.Participants {
		hw[i] = SeqHeadWidth(e.m, p, head)
	}
	gaps := make([]float64, max(n-1, 0))
	for i := range gaps {
		gaps[i] = max(SeqColumn, (hw[i]+hw[i+1])/2+32)
	}
	left, right := max(hw[0]/2, 84), max(hw[n-1]/2, 84)
	num := 0
	for _, it := range sb.Items {
		switch it.Kind {
		case "msg":
			num++
			label := it.Text
			if sb.Numbered {
				label = fmt.Sprintf("%d. %s", num, label)
			}
			need := text(label)
			if it.From == it.To { // a self call loops out to the right
				room := need + 72
				if it.From < n-1 {
					gaps[it.From] = max(gaps[it.From], room)
				} else {
					right = max(right, room)
				}
				continue
			}
			a, b := min(it.From, it.To), max(it.From, it.To)
			span := 0.0
			for k := a; k < b; k++ {
				span += gaps[k]
			}
			if extra := need + 56 - span; extra > 0 {
				for k := a; k < b; k++ {
					gaps[k] += extra / float64(b-a)
				}
			}
		case "note":
			w := text(it.Text) + 40
			if it.From == it.To {
				half := w/2 + 16
				if it.From > 0 {
					gaps[it.From-1] = max(gaps[it.From-1], half+hw[it.From-1]/2)
				} else {
					left = max(left, half)
				}
				if it.From < n-1 {
					gaps[it.From] = max(gaps[it.From], half+hw[it.From+1]/2)
				} else {
					right = max(right, half)
				}
			}
		}
	}
	xs := make([]float64, n)
	xs[0] = left
	for i := 1; i < n; i++ {
		xs[i] = xs[i-1] + gaps[i-1]
	}
	e.res.SeqX[el] = xs
	return xs[n-1] + right
}

// Table cell padding.
const tablePadX, tablePadY = 20.0, 12.0

// cells lists a table row's cells: its children on screen, looking
// through a row that only wraps another (a component's body), whose boxes
// are added to wraps.
func (e *engine) cells(row *c.Element) (cells, wraps []*c.Element) {
	for {
		cells = cells[:0]
		for _, ch := range row.Children {
			if e.inFlow(ch) {
				cells = append(cells, ch)
			}
		}
		if len(cells) != 1 || len(cells[0].Children) == 0 || cells[0].Code != nil {
			break
		}
		k := cells[0].Kind()
		if k != "row" && k != "col" && c.IsBuiltinKind(k) {
			break
		}
		wraps = append(wraps, cells[0])
		row = cells[0]
	}
	if len(row.Children) == 0 {
		// a row that is a single element is one cell
		return []*c.Element{row}, wraps
	}
	return append([]*c.Element(nil), cells...), wraps
}

// table sizes (at == nil) or places a table: each child is a row, each
// row's children are its cells, and cells in the same column share its
// width. Rows stack with no gap; the stage draws a rule between them.
func (e *engine) table(el *c.Element, at *Rect) (float64, float64) {
	var rows []*c.Element
	for _, ch := range el.Children {
		if e.inFlow(ch) {
			rows = append(rows, ch)
		}
	}
	var colW []float64
	rowH := make([]float64, len(rows))
	for i, r := range rows {
		cs, _ := e.cells(r)
		for j, cell := range cs {
			w, h := e.size[cell][0], e.size[cell][1]
			if at == nil {
				w, h = e.measure(cell)
			}
			w = max(w, e.widest(cell))
			for len(colW) <= j {
				colW = append(colW, 0)
			}
			colW[j] = max(colW[j], w)
			rowH[i] = max(rowH[i], h)
		}
		rowH[i] += 2 * tablePadY
	}
	var W, H float64
	for _, w := range colW {
		W += w + 2*tablePadX
	}
	for _, h := range rowH {
		H += h
	}
	if at == nil {
		for i, r := range rows {
			if r.Children != nil {
				e.size[r] = [2]float64{W, rowH[i]}
			}
		}
		return W, H
	}
	y := at.Y
	for i, r := range rows {
		box := Rect{at.X, y, at.W, rowH[i]}
		cs, wraps := e.cells(r)
		for _, w := range wraps {
			e.res.Boxes[w] = box
		}
		x := at.X
		for j, cell := range cs {
			if cell != r {
				e.res.Boxes[r] = box
			}
			s := e.size[cell]
			e.place(cell, Rect{x + tablePadX, y + (rowH[i]-s[1])/2, s[0], s[1]})
			x += colW[j] + 2*tablePadX
		}
		y += rowH[i]
	}
	return W, H
}

// widest is how wide a table cell gets with any text a set step gives it
// or its parts in the scene, so a column keeps one width and a changed
// cell cross-fades in place instead of moving its neighbours.
func (e *engine) widest(cell *c.Element) float64 {
	if len(e.alts) == 0 || e.st.Text == nil {
		return 0
	}
	var w float64
	changed := false
	cell.Walk(func(x *c.Element) {
		for _, t := range e.alts[x] {
			was, had := e.st.Text[x]
			e.st.Text[x] = t
			cw, _ := e.measure(cell)
			w = max(w, cw)
			if had {
				e.st.Text[x] = was
			} else {
				delete(e.st.Text, x)
			}
			changed = true
		}
	})
	if changed {
		e.measure(cell) // leave the sizes as this state has them
	}
	return w
}

// TreeKids lists a tree node's children in the tree: its child elements,
// leaving out connectors, uses and badges. Every one counts, shown yet or
// not, so a tree never moves when a node is added.
func TreeKids(n *c.Element) []*c.Element {
	var out []*c.Element
	for _, ch := range n.Children {
		if ch.Edge == nil && ch.Kind() != "use" && ch.Kind() != "badge" {
			out = append(out, ch)
		}
	}
	return out
}

// TreeOf returns the tree that n is a node of, or nil.
func TreeOf(n *c.Element) *c.Element {
	for p := n.ParentElement(); p != nil; p = p.ParentElement() {
		if p.Kind() == "tree" {
			return p
		}
		if p.Edge != nil || p.Kind() == "badge" || !c.IsBuiltinKind(p.Kind()) && p.Kind() != "scene" {
			return nil
		}
	}
	return nil
}

// tree sizes (at == nil) or places a tree. Positions come from the whole
// tree, nodes not shown yet included, so a node that appears never moves
// the others. kind=binary gives each node a left and a right slot (a lone
// child goes left unless it has side=right) whose spread halves at each
// level, like a binary search tree is drawn; otherwise leaves sit side by
// side and each parent is centred over its children.
func (e *engine) tree(el *c.Element, at *Rect) (float64, float64) {
	gap := num(el, "gap", 24)
	var nodeW, nodeH float64
	depth := 0
	var walk func(n *c.Element, d int)
	walk = func(n *c.Element, d int) {
		w, h := e.measure(n)
		nodeW, nodeH = max(nodeW, w), max(nodeH, h)
		depth = max(depth, d)
		for _, ch := range TreeKids(n) {
			walk(ch, d+1)
		}
	}
	roots := TreeKids(el)
	for _, r := range roots {
		walk(r, 0)
	}
	level := num(el, "rank", nodeH+44)
	pos := map[*c.Element][2]float64{}
	var W float64
	if str(el, "kind", "") == "binary" {
		unit := nodeW + gap
		span := unit * float64(int(1)<<depth) // room for every slot on the deepest level
		var put func(n *c.Element, x float64, d int)
		put = func(n *c.Element, x float64, d int) {
			pos[n] = [2]float64{x, float64(d) * level}
			kids := TreeKids(n)
			off := span / float64(int(4)<<d)
			for i, ch := range kids {
				right := i == 1 || len(kids) == 1 && str(ch, "side", "left") == "right"
				if right {
					put(ch, x+off, d+1)
				} else {
					put(ch, x-off, d+1)
				}
			}
		}
		for i, r := range roots {
			put(r, span*(float64(i)+0.5), 0)
		}
		W = span * float64(max(len(roots), 1))
		// trim empty slots at the sides
		lo, hi := W, 0.0
		for _, p := range pos {
			lo, hi = min(lo, p[0]-nodeW/2), max(hi, p[0]+nodeW/2)
		}
		for n, p := range pos {
			pos[n] = [2]float64{p[0] - lo, p[1]}
		}
		W = hi - lo
	} else {
		next := 0.0
		var put func(n *c.Element, d int) float64
		put = func(n *c.Element, d int) float64 {
			kids := TreeKids(n)
			var x float64
			if len(kids) == 0 {
				x = next + e.size[n][0]/2
				next += e.size[n][0] + gap
			} else {
				first := put(kids[0], d+1)
				lastX := first
				for _, ch := range kids[1:] {
					lastX = put(ch, d+1)
				}
				x = (first + lastX) / 2
			}
			pos[n] = [2]float64{x, float64(d) * level}
			return x
		}
		for _, r := range roots {
			put(r, 0)
		}
		W = max(next-gap, 0)
	}
	H := float64(depth)*level + nodeH
	if at == nil {
		return W, H
	}
	ox := at.X + (at.W-W)/2
	for n, p := range pos {
		s := e.size[n]
		e.place(n, Rect{ox + p[0] - s[0]/2, at.Y + p[1] + (nodeH-s[1])/2, s[0], s[1]})
	}
	return W, H
}

// Meter sizes: one slot's side and the gap between slots.
const MeterSlot, MeterGap = 14.0, 6.0

// SpotRegion is where an isolated element goes for a layout: the left part
// of the frame for left, the top for center, the right part for right.
func SpotRegion(area Rect, layout string) Rect {
	switch layout {
	case "center":
		return Rect{area.X, area.Y, area.W, area.H * 0.52}
	case "right":
		return Rect{area.X + area.W*0.56, area.Y, area.W * 0.44, area.H}
	}
	return Rect{area.X, area.Y, area.W * 0.44, area.H}
}

// AsideRegion is where an aside goes for a layout: whatever SpotRegion
// leaves free.
func AsideRegion(area Rect, layout string) Rect {
	switch layout {
	case "center":
		return Rect{area.X, area.Y + area.H*0.56, area.W, area.H * 0.44}
	case "right":
		return Rect{area.X, area.Y, area.W * 0.52, area.H}
	}
	return Rect{area.X + area.W*0.48, area.Y, area.W * 0.52, area.H}
}

// Git history geometry: space between commits, between lanes, and around.
const GitCol, GitLane, GitPad, GitTop, GitBottom = 124.0, 100.0, 64.0, 66.0, 84.0
