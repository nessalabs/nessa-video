package composition

import (
	"fmt"
	"strconv"
	"strings"
)

// Content blocks carry their own data. The resolve package fills them from
// script text; the stage draws them.

// ArrayBlock is a row of cells with named pointers, such as two-pointer search.
type ArrayBlock struct {
	Values   []string
	Pointers []Pointer // starting positions
}

// Pointer is a named index into an array, shown above its cell.
type Pointer struct {
	Name  string
	Index int // 0-based
}

// ChartBlock is a bar or line chart over labelled rows.
type ChartBlock struct {
	Kind       string   // "bar" or "line"
	Series     []string // names of the value columns
	Rows       []ChartRow
	Horizontal bool   // bars run left to right, one row per line
	Unit       string // after each value label, e.g. "ms"
	Focus      string // the row drawn in the accent at rest
	// a dashed line across a line chart at Threshold, with its label; a
	// ring marks where a line first crosses it, labelled Cross ({x} is
	// where on the x axis)
	HasThreshold   bool
	Threshold      float64
	ThresholdLabel string
	Cross          string
}

// Row returns the index of the row labelled name, or -1.
func (c *ChartBlock) Row(name string) int {
	for i, r := range c.Rows {
		if r.Label == name {
			return i
		}
	}
	return -1
}

// ChartRow is one category with one value per series.
type ChartRow struct {
	Label  string
	Values []float64
}

// TimelineBlock is lanes of spans on a shared time axis.
type TimelineBlock struct {
	Unit  string // shown on the axis, e.g. "ms"
	Lanes []Lane
	// Readout leaves a line under the axis for what a `mark … span=` says
	Readout bool
}

// Picks reports whether name singles out span s of lane l: it is the
// span's label or its lane's.
func (t *TimelineBlock) Picks(name string, l, s int) bool {
	return t.Lanes[l].Spans[s].Label == name || t.Lanes[l].Label == name
}

// Lane is one row of a timeline.
type Lane struct {
	Label string
	Spans []Span
}

// Span is a labelled stretch of time. Strong spans use the bright line.
type Span struct {
	From, To float64
	Label    string
	Strong   bool
}

// End returns the latest time in the timeline.
func (t *TimelineBlock) End() float64 {
	var m float64
	for _, l := range t.Lanes {
		for _, s := range l.Spans {
			m = max(m, s.To)
		}
	}
	return m
}

// SequenceBlock is a Mermaid sequence diagram: participants with lifelines
// and the messages, notes and frames between them, top to bottom.
type SequenceBlock struct {
	Participants []string // labels, left to right
	Items        []SeqItem
	Numbered     bool // autonumber
}

// SeqItem is one row of a sequence diagram.
type SeqItem struct {
	Kind     string // "msg", "note", "open", "else", "close"
	From, To int    // participant indexes; for notes the span
	Text     string
	Dashed   bool // a reply, drawn dashed
	Arrow    bool // has an arrowhead
	Cross    bool // ends in a cross (-x)
}

// DataDecls returns the row=value pairs of a `set` on a chart: its
// declarations that are neither timing nor other known properties.
func DataDecls(ds []Decl) []Decl {
	var out []Decl
	for _, d := range ds {
		if !TimingProp(d.Key) && !IsProp(d.Key) {
			out = append(out, d)
		}
	}
	return out
}

// ChartValues reads the value side of a chart `set`: one number for the
// last series, or one per series separated by commas.
func ChartValues(v string, series int) ([]float64, error) {
	parts := strings.Split(v, ",")
	if len(parts) != 1 && len(parts) != series {
		return nil, fmt.Errorf("%q has %d values; give one, or %d", v, len(parts), series)
	}
	var out []float64
	for _, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, fmt.Errorf("bad value %q", p)
		}
		out = append(out, f)
	}
	return out, nil
}
