package resolve

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/diag"
	"github.com/nessalabs/nessa-video/internal/script"
	"github.com/nessalabs/nessa-video/internal/selector"
)

// block fills the data of array, chart, timeline and mermaid elements.
func (r *resolver) block(e *c.Element, n *script.Node) {
	switch n.Kind {
	case "array":
		r.array(e, n)
	case "chart":
		r.chart(e, n)
	case "timeline":
		r.timeline(e, n)
	case "mermaid":
		r.mermaid(e, n)
	case "math":
		if e.Text == "" && len(n.Raw) > 0 {
			e.Text = strings.Join(n.Raw, " ")
		}
		if strings.TrimSpace(e.Text) == "" {
			r.diags.Errorf(n.Pos, `e.g. math "T(n) = 2T(n/2) + O(n)"`, "math needs a formula")
		}
	}
}

func (r *resolver) array(e *c.Element, n *script.Node) {
	ab := &c.ArrayBlock{Values: e.Args}
	e.Args = nil
	if len(ab.Values) == 0 {
		r.diags.Errorf(n.Pos, "e.g. array #a 1 3 4 6 8", "array needs values")
		return
	}
	if v, ok := n.Prop("ptr"); ok {
		ps, err := pointers(v, len(ab.Values))
		if err != nil {
			r.diags.Errorf(n.Pos, `e.g. ptr="lo=0 hi=7"`, "%v", err)
		}
		ab.Pointers = ps
	}
	e.Array = ab
}

// pointers reads "lo=0 hi=7" into named 0-based indexes below size.
func pointers(s string, size int) ([]c.Pointer, error) {
	var out []c.Pointer
	for _, f := range strings.Fields(s) {
		name, idx, ok := strings.Cut(f, "=")
		if !ok || name == "" {
			return out, fmt.Errorf("bad pointer %q", f)
		}
		i, err := strconv.Atoi(idx)
		if err != nil || i < 0 || i >= size {
			return out, fmt.Errorf("pointer %s=%s is outside the array (0 to %d)", name, idx, size-1)
		}
		out = append(out, c.Pointer{Name: name, Index: i})
	}
	return out, nil
}

func (r *resolver) chart(e *c.Element, n *script.Node) {
	cb := &c.ChartBlock{Kind: "bar"}
	if len(e.Args) > 0 {
		cb.Kind = e.Args[0]
		e.Args = nil
	}
	if cb.Kind != "bar" && cb.Kind != "line" {
		r.diags.Errorf(n.Pos, "use chart bar or chart line", "unknown chart kind %q", cb.Kind)
		return
	}
	var rows [][]string
	for _, ln := range n.Raw {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		var cells []string
		for _, c := range strings.Split(ln, ",") {
			cells = append(cells, strings.TrimSpace(c))
		}
		rows = append(rows, cells)
	}
	if len(rows) < 2 || len(rows[0]) < 2 {
		r.diags.Errorf(n.Pos, "first line names the columns, e.g. `burst, before, after`, then one line per row", "chart needs a header and at least one row")
		return
	}
	cb.Series = rows[0][1:]
	for i, row := range rows[1:] {
		pos := diag.Pos{Line: n.Pos.Line + 2 + i, Col: n.Pos.Col + 2}
		if len(row) != len(rows[0]) {
			r.diags.Errorf(pos, fmt.Sprintf("the header has %d columns", len(rows[0])), "row %q has %d columns", row[0], len(row))
			continue
		}
		cr := c.ChartRow{Label: row[0]}
		for _, v := range row[1:] {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				r.diags.Errorf(pos, "values must be numbers", "bad value %q in row %q", v, row[0])
				f = 0
			}
			cr.Values = append(cr.Values, f)
		}
		cb.Rows = append(cb.Rows, cr)
	}
	e.Chart = cb
}

func (r *resolver) timeline(e *c.Element, n *script.Node) {
	tb := &c.TimelineBlock{Unit: "ms"}
	if v, ok := n.Prop("unit"); ok {
		tb.Unit = v
	}
	for i, ln := range n.Raw {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		pos := diag.Pos{Line: n.Pos.Line + 1 + i, Col: n.Pos.Col + 2}
		parts := strings.Split(ln, "|")
		lane := c.Lane{Label: strings.TrimSpace(parts[0])}
		for _, sp := range parts[1:] {
			sp = strings.TrimSpace(sp)
			s := c.Span{}
			if strings.HasPrefix(sp, "*") {
				s.Strong = true
				sp = strings.TrimSpace(sp[1:])
			}
			rng, label, _ := strings.Cut(sp, " ")
			a, b, ok := strings.Cut(rng, "-")
			from, err1 := strconv.ParseFloat(a, 64)
			to, err2 := strconv.ParseFloat(b, 64)
			if !ok || err1 != nil || err2 != nil || to < from {
				r.diags.Errorf(pos, "e.g. `request A | 0-18 wait | *18-22 200`", "bad span %q", sp)
				continue
			}
			s.From, s.To, s.Label = from, to, strings.TrimSpace(label)
			lane.Spans = append(lane.Spans, s)
		}
		tb.Lanes = append(tb.Lanes, lane)
	}
	if len(tb.Lanes) == 0 {
		r.diags.Errorf(n.Pos, "one lane per line: `label | 0-18 wait | *18-22 done`", "timeline needs lanes")
		return
	}
	e.Timeline = tb
}

// Mermaid flowcharts become ordinary elements: nodes are parts of the
// mermaid element (so `#flow/H` targets node H) and links are connectors.

var (
	mmHeader = regexp.MustCompile(`^(flowchart|graph)\s*(LR|RL|TD|TB|BT)?\s*$`)
	mmNode   = regexp.MustCompile(`^([A-Za-z_][\w-]*)\s*(\[\(.*?\)\]|\(\(.*?\)\)|\[.*?\]|\{.*?\}|\(.*?\))?`)
	mmLink   = regexp.MustCompile(`^(-->\s*\|([^|]*)\||--\s*([^->][^>]*?)\s*-->|-\.->|-\.\s*([^.]+?)\s*\.->|==>|-->|---)`)
)

type mmEdge struct {
	from, to, label string
	dashed, arrow   bool
}

func (r *resolver) mermaid(e *c.Element, n *script.Node) {
	var lines []string
	for _, ln := range n.Raw {
		for _, part := range strings.Split(ln, ";") {
			if t := strings.TrimSpace(part); t != "" && !strings.HasPrefix(t, "%%") {
				lines = append(lines, t)
			}
		}
	}
	if len(lines) == 0 {
		r.diags.Errorf(n.Pos, "indent a flowchart under the line", "mermaid block is empty")
		return
	}
	if lines[0] == "sequenceDiagram" {
		r.sequenceDiagram(e, n, lines[1:])
		return
	}
	m := mmHeader.FindStringSubmatch(lines[0])
	if m == nil {
		r.diags.Errorf(n.Pos, "start with `flowchart LR`, `flowchart TD` or `sequenceDiagram`", "unsupported Mermaid diagram %q", lines[0])
		return
	}
	dir := m[2]
	if dir == "" || dir == "TB" {
		dir = "TD"
	}
	e.Inline = append(e.Inline, c.Decl{Key: "dir", Value: dir, Pos: n.Pos})

	type node struct{ id, shape, text string }
	var order []string
	nodes := map[string]*node{}
	var edges []mmEdge
	addNode := func(id, shape string) {
		nd, ok := nodes[id]
		if !ok {
			nd = &node{id: id, text: id, shape: "box"}
			nodes[id] = nd
			order = append(order, id)
		}
		if shape != "" {
			text := shape
			switch {
			case strings.HasPrefix(shape, "[("):
				nd.shape, text = "store", shape[2:len(shape)-2]
			case strings.HasPrefix(shape, "(("):
				nd.shape, text = "circle", shape[2:len(shape)-2]
			case strings.HasPrefix(shape, "{"):
				nd.shape, text = "decision", shape[1:len(shape)-1]
			case strings.HasPrefix(shape, "("):
				nd.shape, text = "round", shape[1:len(shape)-1]
			default:
				nd.shape, text = "box", shape[1:len(shape)-1]
			}
			nd.text = strings.Trim(strings.TrimSpace(text), `"`)
		}
	}
	for i, ln := range lines[1:] {
		pos := diag.Pos{Line: n.Pos.Line + 2 + i, Col: n.Pos.Col + 2}
		rest := ln
		prev := ""
		for rest != "" {
			nm := mmNode.FindStringSubmatch(rest)
			if nm == nil {
				r.diags.Errorf(pos, "e.g. `A[Client] --> B{Cache hit?}`", "cannot read %q", rest)
				break
			}
			addNode(nm[1], nm[2])
			if prev != "" {
				edges[len(edges)-1].to = nm[1]
			}
			prev = nm[1]
			rest = strings.TrimSpace(rest[len(nm[0]):])
			if rest == "" {
				break
			}
			lm := mmLink.FindStringSubmatch(rest)
			if lm == nil {
				r.diags.Errorf(pos, "links are -->, ---, -.->, ==>, -- text --> or -->|text|", "cannot read link in %q", rest)
				break
			}
			ed := mmEdge{from: nm[1], arrow: lm[1] != "---", dashed: strings.HasPrefix(lm[1], "-.")}
			ed.label = strings.TrimSpace(lm[2] + lm[3] + lm[4])
			edges = append(edges, ed)
			rest = strings.TrimSpace(rest[len(lm[0]):])
			if rest == "" {
				r.diags.Errorf(pos, "", "link has no target in %q", ln)
				edges = edges[:len(edges)-1]
			}
		}
	}

	// rank nodes by longest path from the sources, ignoring back links
	rank := map[string]int{}
	indeg := map[string]int{}
	for _, ed := range edges {
		indeg[ed.to]++
	}
	visiting := map[string]bool{}
	var visit func(id string, d int)
	visit = func(id string, d int) {
		if visiting[id] || rank[id] >= d && d > 0 {
			return
		}
		if d > rank[id] {
			rank[id] = d
		}
		visiting[id] = true
		for _, ed := range edges {
			if ed.from == id {
				visit(ed.to, d+1)
			}
		}
		visiting[id] = false
	}
	for _, id := range order {
		if indeg[id] == 0 {
			visit(id, 0)
		}
	}
	for _, id := range order { // nodes only reachable through cycles
		if _, ok := rank[id]; !ok {
			visit(id, 0)
		}
	}
	slot := map[int]int{}
	for _, id := range order {
		nd := nodes[id]
		kind := nd.shape
		el := c.NewElement(kind, n.Pos)
		if kind == "round" {
			el = c.NewElement("box", n.Pos)
			el.Inline = append(el.Inline, c.Decl{Key: "r", Value: "24", Pos: n.Pos})
		}
		el.SetPart(id)
		el.SetOwner(e)
		el.Text = nd.text
		el.Inline = append(el.Inline,
			c.Decl{Key: "rank", Value: strconv.Itoa(rank[id]), Pos: n.Pos},
			c.Decl{Key: "order", Value: strconv.Itoa(slot[rank[id]]), Pos: n.Pos})
		if kind == "circle" {
			el.Inline = append(el.Inline, c.Decl{Key: "r", Value: "40", Pos: n.Pos})
		}
		slot[rank[id]]++
		e.Append(el)
	}
	for _, ed := range edges {
		link := c.NewElement("edge", n.Pos)
		link.SetOwner(e)
		op := "->"
		switch {
		case ed.dashed:
			op = "..>"
		case !ed.arrow:
			op = "--"
		}
		link.Edge = &c.Edge{Op: op, From: partSel(ed.from), To: partSel(ed.to)}
		link.Text = ed.label
		e.Append(link)
	}
}

func partSel(id string) *selector.Selector {
	return &selector.Selector{Src: "#" + id, Group: []selector.Complex{{Parts: []selector.Compound{{Part: id}}}}}
}

var (
	seqParticipant = regexp.MustCompile(`^(participant|actor)\s+([\w-]+)(?:\s+as\s+(.+))?$`)
	seqMessage     = regexp.MustCompile(`^(\w+(?:-\w+)*)\s*(-->>|->>|-->|->|--x|-x|--\)|-\))\s*[+-]?(\w+(?:-\w+)*)\s*:\s*(.*)$`)
	seqNote        = regexp.MustCompile(`^(?i:note)\s+(over|left of|right of)\s+([\w-]+)(?:\s*,\s*([\w-]+))?\s*:\s*(.*)$`)
	seqFrame       = regexp.MustCompile(`^(alt|opt|loop|par|critical|break|else|and)\b\s*(.*)$`)
)

// sequenceDiagram reads the Mermaid sequence diagram subset reel draws.
func (r *resolver) sequenceDiagram(e *c.Element, n *script.Node, lines []string) {
	sb := &c.SequenceBlock{}
	index := map[string]int{}
	part := func(id, label string) int {
		if i, ok := index[id]; ok {
			if label != "" {
				sb.Participants[i] = label
			}
			return i
		}
		if label == "" {
			label = id
		}
		index[id] = len(sb.Participants)
		sb.Participants = append(sb.Participants, label)
		return index[id]
	}
	depth := 0
	for i, ln := range lines {
		pos := diag.Pos{Line: n.Pos.Line + 2 + i, Col: n.Pos.Col + 2}
		switch {
		case ln == "autonumber":
			sb.Numbered = true
		case ln == "end":
			if depth == 0 {
				r.diags.Errorf(pos, "", "`end` without an open alt, opt or loop")
				continue
			}
			depth--
			sb.Items = append(sb.Items, c.SeqItem{Kind: "close"})
		case strings.HasPrefix(ln, "activate ") || strings.HasPrefix(ln, "deactivate ") || strings.HasPrefix(ln, "rect "):
			// accepted and ignored: activation bars and coloured rects are not drawn
		default:
			if m := seqParticipant.FindStringSubmatch(ln); m != nil {
				part(m[2], strings.Trim(strings.TrimSpace(m[3]), `"`))
			} else if m := seqMessage.FindStringSubmatch(ln); m != nil {
				op := m[2]
				sb.Items = append(sb.Items, c.SeqItem{Kind: "msg", From: part(m[1], ""), To: part(m[3], ""), Text: strings.TrimSpace(m[4]),
					Dashed: strings.HasPrefix(op, "--"), Arrow: strings.Contains(op, ">"), Cross: strings.HasSuffix(op, "x")})
			} else if m := seqNote.FindStringSubmatch(ln); m != nil {
				a := part(m[2], "")
				b := a
				if m[3] != "" {
					b = part(m[3], "")
				}
				sb.Items = append(sb.Items, c.SeqItem{Kind: "note", From: min(a, b), To: max(a, b), Text: strings.TrimSpace(m[4])})
			} else if m := seqFrame.FindStringSubmatch(ln); m != nil {
				if m[1] == "else" || m[1] == "and" {
					if depth == 0 {
						r.diags.Errorf(pos, "", "`%s` outside an alt or par", m[1])
						continue
					}
					sb.Items = append(sb.Items, c.SeqItem{Kind: "else", Text: strings.TrimSpace(m[2])})
				} else {
					depth++
					sb.Items = append(sb.Items, c.SeqItem{Kind: "open", Text: strings.TrimSpace(m[1] + " " + m[2])})
				}
			} else {
				r.diags.Errorf(pos, "e.g. `A->>B: Request`, `B-->>A: Reply`, `Note over A: text`, `alt cached` … `end`", "cannot read %q", ln)
			}
		}
	}
	for ; depth > 0; depth-- {
		r.diags.Errorf(n.Pos, "close each alt, opt and loop with `end`", "sequence diagram has an unclosed frame")
		sb.Items = append(sb.Items, c.SeqItem{Kind: "close"})
	}
	if len(sb.Participants) == 0 {
		r.diags.Errorf(n.Pos, "e.g. `A->>B: Request`", "sequence diagram has no participants")
		return
	}
	e.Sequence = sb
}
