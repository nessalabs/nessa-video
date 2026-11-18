// Package layout sizes and places a scene's elements. It implements a small
// subset of flexbox and grid: rows, columns, grids, layers and absolute
// places. It works in design units; the stage scales them to frame pixels.
package layout

import (
	"fmt"
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
)

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
}

type engine struct {
	m    Measurer
	opt  Options
	size map[*c.Element][2]float64
	res  Result
	flip map[*c.Element]bool    // rows and columns turned the other way to fit
	room map[*c.Element]float64 // minimum gap after a child, to fit a connector label
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
	var w, h float64
	for round := 0; ; round++ {
		e.size = map[*c.Element][2]float64{}
		e.res = Result{Boxes: map[*c.Element]Rect{}, Lines: map[*c.Element][]string{}, Fit: 1, SeqX: map[*c.Element][]float64{}}
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
		e.size = map[*c.Element][2]float64{}
		e.res = Result{Boxes: map[*c.Element]Rect{}, Lines: map[*c.Element][]string{}, Fit: 1, SeqX: map[*c.Element][]float64{}}
		w2, h2 := e.measure(root)
		if fitOf(w2, h2) > fitFlipped+0.02 {
			w, h = w2, h2
		} else {
			e.flip = flips
			e.size = map[*c.Element][2]float64{}
			e.res = Result{Boxes: map[*c.Element]Rect{}, Lines: map[*c.Element][]string{}, Fit: 1, SeqX: map[*c.Element][]float64{}}
			w, h = e.measure(root)
		}
	}
	r := Rect{area.X + (area.W-w)/2, area.Y + (area.H-h)/2, w, h}
	e.place(root, r)
	e.res.Content = r
	if w > area.W || h > area.H {
		e.res.Fit = min(area.W/w, area.H/h)
	}
	return e.res
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
		from, to := c.Select(scope, edge.Edge.From), c.Select(scope, edge.Edge.To)
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
			if inFlow(ch) {
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
	return strings.NewReplacer("==", "", "__", "").Replace(s)
}

var textKinds = map[string]bool{"title": true, "p": true, "label": true, "metric": true, "code": true}

func inFlow(el *c.Element) bool {
	return el.Kind() != "edge" && el.Kind() != "use" && str(el, "hidden", "false") != "true"
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
	case kind == "row" || kind == "col" || kind == "scene" || !c.IsBuiltinKind(kind):
		gap := num(el, "gap", e.opt.Gap)
		if kind == "scene" {
			gap = num(el, "gap", e.opt.Gap*2)
		}
		horizontal := e.horizontal(el)
		var main, cross float64
		var prev *c.Element
		for _, ch := range el.Children {
			if !inFlow(ch) {
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
	case kind == "grid":
		cols := int(num(el, "cols", 2))
		gap := num(el, "gap", e.opt.Gap)
		var cw, rh float64
		n := 0
		for _, ch := range el.Children {
			if inFlow(ch) {
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
			if inFlow(ch) {
				cw, chh := e.measure(ch)
				w, h = max(w, cw), max(h, chh)
			}
		}
		return w, h
	case kind == "place":
		var r Rect
		for _, ch := range el.Children {
			if inFlow(ch) {
				w, h := e.measure(ch)
				r = r.Union(Rect{num(ch, "x", 0), num(ch, "y", 0), w, h})
			}
		}
		return r.Right(), r.Bottom()
	case el.Code != nil:
		return e.codePanel(el)
	case el.Array != nil:
		n := float64(len(el.Array.Values))
		return n*CellSize + (n-1)*CellGap, PointerTop + CellSize + IndexBand
	case el.Chart != nil:
		return 460, 300
	case el.Timeline != nil:
		return 600, 28 + LaneHeight*float64(len(el.Timeline.Lanes)) + AxisBand
	case kind == "math":
		if ts, ok := e.m.(Typesetter); ok {
			if _, w, h, err := ts.Typeset(el.Text, num(el, "size", MathSize)); err == nil {
				return w, h
			}
		}
		return e.opt.BlockSize[0], 80
	case el.Sequence != nil:
		rows := 0.0
		for _, it := range el.Sequence.Items {
			if it.Kind != "close" {
				rows++
			}
		}
		w := e.sequenceColumns(el)
		return w, SeqHead + rows*SeqRow + 28
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
		if el.Text != "" {
			t := TypeOf(el, e.opt)
			tw := e.m.Measure(Plain(el.Text), t.Font, t.Size)
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
		return 200, 120
	case kind == "circle":
		r := num(el, "r", 28)
		return 2 * r, 2 * r
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
		lines = e.wrap(el.Text, t, num(el, "w", e.opt.MaxTextW))
	default:
		lines = strings.Split(el.Text, "\n")
	}
	e.res.Lines[el] = lines
	var w float64
	for _, l := range lines {
		w = max(w, e.m.Measure(Plain(l), t.Font, t.Size))
	}
	if el.Text == "" && len(el.Body) == 0 {
		return 0, 0
	}
	return w, line * float64(len(lines))
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
	return out
}

func (e *engine) place(el *c.Element, r Rect) {
	e.res.Boxes[el] = r
	kind := el.Kind()
	pad := num(el, "pad", 0)
	inner := Rect{r.X + pad, r.Y + pad, r.W - 2*pad, r.H - 2*pad}
	switch {
	case kind == "row" || kind == "col" || kind == "scene" || !c.IsBuiltinKind(kind):
		horizontal := e.horizontal(el)
		gap := num(el, "gap", e.opt.Gap)
		if kind == "scene" {
			gap = num(el, "gap", e.opt.Gap*2)
		}
		align := str(el, "align", "center")
		var kids []*c.Element
		var used float64
		for _, ch := range el.Children {
			if !inFlow(ch) {
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
	case kind == "grid":
		cols := max(int(num(el, "cols", 2)), 1)
		gap := num(el, "gap", e.opt.Gap)
		cw := (inner.W - gap*float64(cols-1)) / float64(cols)
		var rh float64
		for _, ch := range el.Children {
			if inFlow(ch) {
				rh = max(rh, e.size[ch][1])
			}
		}
		i := 0
		for _, ch := range el.Children {
			if !inFlow(ch) {
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
			if inFlow(ch) {
				s := e.size[ch]
				e.place(ch, Rect{inner.CX() - s[0]/2, inner.CY() - s[1]/2, s[0], s[1]})
			}
		}
	case kind == "place":
		for _, ch := range el.Children {
			if inFlow(ch) {
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
	const rankGap, nodeGap = 80.0, 36.0
	type rk struct {
		nodes []*c.Element
		main  float64 // size along the rank direction (column width for LR)
		cross float64 // total size across
	}
	var ranks []*rk
	for _, ch := range el.Children {
		if !inFlow(ch) {
			continue
		}
		w, h := e.measure(ch)
		ri := int(num(ch, "rank", 0))
		for len(ranks) <= ri {
			ranks = append(ranks, &rk{})
		}
		r := ranks[ri]
		r.nodes = append(r.nodes, ch)
		along, across := h, w
		if lr {
			along, across = w, h
		}
		r.main = max(r.main, along)
		if len(r.nodes) > 1 {
			r.cross += nodeGap
		}
		r.cross += across
	}
	var total, crossMax float64
	for i, r := range ranks {
		if i > 0 {
			total += rankGap
		}
		total += r.main
		crossMax = max(crossMax, r.cross)
	}
	if at == nil {
		if lr {
			return total, crossMax
		}
		return crossMax, total
	}
	pos := 0.0
	for _, r := range ranks {
		off := (crossMax - r.cross) / 2
		for _, ch := range r.nodes {
			s := e.size[ch]
			if lr {
				e.place(ch, Rect{at.X + pos + (r.main-s[0])/2, at.Y + off, s[0], s[1]})
				off += s[1] + nodeGap
			} else {
				e.place(ch, Rect{at.X + off, at.Y + pos + (r.main-s[1])/2, s[0], s[1]})
				off += s[0] + nodeGap
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
	return max(136, m.Measure(label, t.Font, t.Size)+36)
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
	text := func(s string) float64 { return e.m.Measure(s, lab.Font, lab.Size) }
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
