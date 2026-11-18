package composition

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
	Kind   string   // "bar" or "line"
	Series []string // names of the value columns
	Rows   []ChartRow
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
