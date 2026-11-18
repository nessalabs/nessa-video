// Package layout sizes and places a scene's elements. It implements a small
// subset of flexbox and grid: rows, columns, grids, layers and absolute
// places. It works in design units; the stage scales them to frame pixels.
package layout

import (
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
}

type engine struct {
	m    Measurer
	opt  Options
	size map[*c.Element][2]float64
	res  Result
}

// Scene lays out root's children as a centred column inside area.
func Scene(root *c.Element, area Rect, m Measurer, opt Options) Result {
	e := &engine{m: m, opt: opt, size: map[*c.Element][2]float64{},
		res: Result{Boxes: map[*c.Element]Rect{}, Lines: map[*c.Element][]string{}}}
	w, h := e.measure(root)
	r := Rect{area.X + (area.W-w)/2, area.Y + (area.H-h)/2, w, h}
	e.place(root, r)
	return e.res
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

func inFlow(el *c.Element) bool { return el.Kind() != "edge" && el.Kind() != "use" }

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
		horizontal := kind == "row"
		var main, cross float64
		n := 0
		for _, ch := range el.Children {
			if !inFlow(ch) {
				continue
			}
			w, h := e.measure(ch)
			if w == 0 && h == 0 {
				continue // empty slot or part
			}
			if horizontal {
				main += w
				cross = max(cross, h)
			} else {
				main += h
				cross = max(cross, w)
			}
			n++
		}
		if n > 1 {
			main += gap * float64(n-1)
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
	case kind == "mermaid":
		return e.graph(el, nil)
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
		horizontal := kind == "row"
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
		if len(kids) > 1 {
			used += gap * float64(len(kids)-1)
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
		case "between":
			if len(kids) > 1 {
				gap += space / float64(len(kids)-1)
			}
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
				pos += s[0] + gap
			} else {
				x := crossPos(align, inner.X, inner.W, s[0])
				w := s[0]
				if align == "stretch" {
					w = inner.W
				}
				e.place(ch, Rect{x, pos, w, s[1]})
				pos += s[1] + gap
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
	case kind == "mermaid":
		e.graph(el, &inner)
	}
}

// graph sizes (at == nil) or places a Mermaid diagram: ranks become columns
// for LR and rows for TD, and nodes keep their order within a rank.
func (e *engine) graph(el *c.Element, at *Rect) (float64, float64) {
	lr := str(el, "dir", "TD") == "LR" || str(el, "dir", "") == "RL"
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
