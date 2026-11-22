package motion

import (
	"math"
	"sort"

	c "github.com/nessalabs/nessa-video/internal/composition"
)

// Data steps change what charts, timelines and tables show: a `set` that
// eases a chart row to new values, a `sort` that slides bars into a new
// order, and the time a chart's bars or lines take to grow in.

// ChartSet is a `set` on a chart row: its values move to Values (one per
// series) from Start over Dur.
type ChartSet struct {
	Row        int
	Values     []float64
	Start, Dur float64
}

// ChartSort reorders a chart's rows by their last series, largest first
// unless Asc.
type ChartSort struct {
	Asc        bool
	Start, Dur float64
}

// dataPlan is what data steps do to one element in a scene.
type dataPlan struct {
	sets  []ChartSet
	sorts []ChartSort
	grow  *Anim // a chart's bars or lines growing in
	plot  *plotPlan
}

// Timings of data motion, in seconds.
const (
	ChartSetTime  = 1.2 // a bar easing to its new value
	SortTime      = 0.9 // bars sliding into their new order
	CellSetTime   = 0.7 // a table cell cross-fading to its new text
	MarkSpanTime  = 0.7 // a timeline span singled out
	MarkRowTime   = 0.6 // the accent moving to another chart row
	BarGrow       = 1.1 // one bar growing in
	BarStagger    = 0.09
	LineGrow      = 2.2 // lines drawing in, left to right
	CrossMarkTime = 0.45
)

func (sp *ScenePlan) dataOf(e *c.Element) *dataPlan {
	if sp.data == nil {
		sp.data = map[*c.Element]*dataPlan{}
	}
	d := sp.data[e]
	if d == nil {
		d = &dataPlan{}
		sp.data[e] = d
	}
	return d
}

// growTime is how long a chart's contents take to grow in once its frame
// has drawn, including the marker where a line crosses its threshold.
func growTime(e *c.Element) float64 {
	if e.Plot != nil {
		return PlotTrace
	}
	cb := e.Chart
	if cb == nil {
		return 0
	}
	if cb.Kind == "line" {
		d := LineGrow
		if cb.HasThreshold {
			d += CrossMarkTime
		}
		return d
	}
	return BarGrow + BarStagger*float64(max(len(cb.Rows)-1, 0))
}

// planGrow times a chart's contents growing in after its frame enters
// with a, and returns when the element is done entering.
func (sp *ScenePlan) planGrow(e *c.Element, a Anim) float64 {
	if e.Chart == nil && e.Plot == nil || math.IsInf(a.Start, 1) {
		return a.end()
	}
	if e.Plot != nil && (sp.played(e) || len(e.Plot.Curves) == 0) {
		return a.end() // a play step traces it
	}
	g := Anim{Mode: "grow", Start: a.Start + a.Dur*0.5, Dur: growTime(e)}
	sp.dataOf(e).grow = &g
	return math.Max(a.end(), g.end())
}

// noteShown records that a show step brought e in, so a table row can
// greet it with a soft highlight.
func (sp *ScenePlan) noteShown(e *c.Element) {
	if sp.late == nil {
		sp.late = map[*c.Element]bool{}
	}
	sp.late[e] = true
}

// carryData brings what data steps did to pe in the scene before over to
// e, settled, so a carried chart keeps its values and order.
func (sp *ScenePlan) carryData(prev *ScenePlan, pe, e *c.Element) {
	d := prev.data[pe]
	if d == nil {
		return
	}
	nd := sp.dataOf(e)
	for _, s := range d.sets {
		s.Start, s.Dur = -1, 0
		nd.sets = append(nd.sets, s)
	}
	for _, s := range d.sorts {
		s.Start, s.Dur = -1, 0
		nd.sorts = append(nd.sorts, s)
	}
}

// dataDur adjusts a step's length for data steps: a chart set counts its
// value over ChartSetTime, a sort slides, a cell cross-fades, a mark on a
// timeline or chart eases, and a chart that is shown grows in.
func dataDur(st c.Step, targets []*c.Element, dur float64) float64 {
	if d := plotStepDur(st, targets, -1); d >= 0 {
		return d
	}
	switch st.Verb {
	case "set":
		for _, e := range targets {
			if e.Chart != nil && len(c.DataDecls(st.Decls)) > 0 {
				return ChartSetTime
			}
			if inTable(e) {
				dur = math.Max(dur, CellSetTime)
			}
		}
	case "sort":
		return SortTime
	case "mark":
		if _, ok := st.Lookup("span"); ok {
			return MarkSpanTime
		}
		if _, ok := st.Lookup("row"); ok {
			return MarkRowTime
		}
	case "show":
		for _, e := range targets {
			if e.Chart != nil || e.Plot != nil {
				dur = math.Max(dur, dur*0.5+growTime(e))
			}
		}
	}
	return dur
}

// inTable reports whether e is a cell of a table (or inside one).
func inTable(e *c.Element) bool {
	for p := e.ParentElement(); p != nil; p = p.ParentElement() {
		if p.Kind() == "table" {
			return true
		}
	}
	return false
}

// planData records a set on chart rows or a sort, and reports whether the
// step was one.
func (sp *ScenePlan) planData(st c.Step, targets []*c.Element, start, dur float64) bool {
	if sp.planPlot(st, targets, start, dur) {
		return true
	}
	switch st.Verb {
	case "set":
		data := c.DataDecls(st.Decls)
		if len(data) == 0 {
			return false
		}
		for _, e := range targets {
			if e.Chart == nil {
				continue
			}
			for _, d := range data {
				row := e.Chart.Row(d.Key)
				vals, err := c.ChartValues(d.Value, len(e.Chart.Series))
				if row < 0 || err != nil {
					continue
				}
				sp.dataOf(e).sets = append(sp.dataOf(e).sets, ChartSet{Row: row, Values: vals, Start: start, Dur: dur})
			}
		}
		return true
	case "sort":
		asc := len(st.Args) > 0 && st.Args[0] == "asc"
		for _, e := range targets {
			if e.Chart != nil {
				sp.dataOf(e).sorts = append(sp.dataOf(e).sorts, ChartSort{Asc: asc, Start: start, Dur: dur})
			}
		}
		return true
	}
	return false
}

// markNames reads what a mark step singles out of a chart or timeline:
// row= or span=, and the readout note=.
func markNames(st c.Step) (names []string, note string) {
	if v, ok := st.Lookup("row"); ok {
		names = append(names, v)
	}
	if v, ok := st.Lookup("span"); ok {
		names = append(names, v)
	}
	note, _ = st.Lookup("note")
	return names, note
}

// Grow returns how long a chart's contents have been growing in and how
// long they take; ok is false when they are not growing in this scene
// (they show in full).
func (m Moment) Grow(e *c.Element) (since, dur float64, ok bool) {
	d := m.plan.data[e]
	if d == nil || d.grow == nil {
		return 0, 0, false
	}
	return m.T - d.grow.Start, d.grow.Dur, true
}

// ChartNow is a chart at one moment.
type ChartNow struct {
	Values [][]float64 // by row, then series; between old and new while a set runs
	Slot   []float64   // by row: its place in the order, fractional while a sort runs
	Accent []float64   // by row: 0 grey, 1 in the accent
	Top    float64     // the largest value the chart takes in the scene
}

// Chart returns a chart's values, order and accent at this moment.
func (m Moment) Chart(e *c.Element) ChartNow {
	cb := e.Chart
	n := len(cb.Rows)
	out := ChartNow{Values: make([][]float64, n), Slot: make([]float64, n), Accent: make([]float64, n)}
	base := make([][]float64, n)
	for i, r := range cb.Rows {
		base[i] = append([]float64(nil), r.Values...)
		out.Values[i] = append([]float64(nil), r.Values...)
		out.Slot[i] = float64(i)
		for _, v := range r.Values {
			out.Top = math.Max(out.Top, v)
		}
	}
	d := m.plan.data[e]
	if d == nil {
		d = &dataPlan{}
	}
	type event struct {
		start, dur float64
		set        *ChartSet
		sort       *ChartSort
	}
	var evs []event
	for i := range d.sets {
		s := &d.sets[i]
		evs = append(evs, event{start: s.Start, dur: s.Dur, set: s})
		for _, v := range s.Values {
			out.Top = math.Max(out.Top, v)
		}
	}
	for i := range d.sorts {
		s := &d.sorts[i]
		evs = append(evs, event{start: s.Start, dur: s.Dur, sort: s})
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].start < evs[j].start })

	slot := make([]int, n)
	for i := range slot {
		slot[i] = i
	}
	// the accent: the focus= row at rest, then whichever rows a mark or a
	// set names last
	var accFrom, accTo []int
	if r := cb.Row(cb.Focus); cb.Focus != "" && r >= 0 {
		accFrom, accTo = []int{r}, []int{r}
	}
	accMix := 1.0
	type pick struct {
		start, dur float64
		rows       []int
	}
	var picks []pick
	for _, mk := range m.plan.marks[e] {
		var rows []int
		for _, name := range mk.Names {
			if r := cb.Row(name); r >= 0 {
				rows = append(rows, r)
			}
		}
		if rows != nil {
			picks = append(picks, pick{mk.Start, mk.Dur, rows})
		}
	}
	for _, s := range d.sets {
		picks = append(picks, pick{s.Start, math.Max(s.Dur*0.75, 0), []int{s.Row}})
	}
	sort.SliceStable(picks, func(i, j int) bool { return picks[i].start < picks[j].start })
	for _, pk := range picks {
		if m.T < pk.start {
			break
		}
		accFrom, accTo = accTo, pk.rows
		accMix = 1
		if pk.dur > 0 {
			accMix = Ease((m.T - pk.start) / pk.dur)
		}
	}
	has := func(set []int, i int) float64 {
		for _, x := range set {
			if x == i {
				return 1
			}
		}
		return 0
	}
	for i := range out.Accent {
		out.Accent[i] = has(accFrom, i) + (has(accTo, i)-has(accFrom, i))*accMix
	}

	for _, ev := range evs {
		if m.T < ev.start {
			break
		}
		f := 1.0
		if ev.dur > 0 {
			f = Ease((m.T - ev.start) / ev.dur)
		}
		if s := ev.set; s != nil {
			if s.Row >= n {
				continue
			}
			row := base[s.Row]
			for j := range row {
				to := row[j]
				if len(s.Values) == len(row) {
					to = s.Values[j]
				} else if j == len(row)-1 {
					to = s.Values[0]
				}
				out.Values[s.Row][j] = row[j] + (to-row[j])*f
				row[j] = to
			}
			continue
		}
		// a sort: order rows by their settled last value
		order := make([]int, n)
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool { return slot[order[a]] < slot[order[b]] })
		last := func(i int) float64 {
			if len(base[i]) == 0 {
				return 0
			}
			return base[i][len(base[i])-1]
		}
		sort.SliceStable(order, func(a, b int) bool {
			if ev.sort.Asc {
				return last(order[a]) < last(order[b])
			}
			return last(order[a]) > last(order[b])
		})
		for k, i := range order {
			out.Slot[i] = float64(slot[i]) + float64(k-slot[i])*f
			slot[i] = k
		}
	}
	return out
}

// Picked returns which spans or rows mark steps single out: the names
// being left, the ones being moved to (nil is none), the progress, and
// the readout note of the latest mark.
func (m Moment) Picked(e *c.Element) (from, to []string, mix float64, note string) {
	mix = 1
	for _, mk := range m.plan.marks[e] {
		if mk.Names == nil || m.T < mk.Start {
			continue
		}
		from, to, note = to, mk.Names, mk.Note
		mix = 1
		if mk.Dur > 0 {
			mix = Ease((m.T - mk.Start) / mk.Dur)
		}
	}
	return
}

// ShownFor returns how long ago a show step brought e in, and whether one
// did.
func (m Moment) ShownFor(e *c.Element) (float64, bool) {
	if !m.plan.late[e] {
		return 0, false
	}
	a, ok := m.plan.enter[e]
	if !ok || math.IsInf(a.Start, 1) {
		return 0, false
	}
	return m.T - a.Start, true
}

// PlayEnded returns how long ago a block's playhead reached the end of
// its axis, and whether it has.
func (m Moment) PlayEnded(e *c.Element) (float64, bool) {
	segs := m.plan.plays[e]
	if len(segs) == 0 {
		return 0, false
	}
	last := segs[len(segs)-1]
	end := last.Start + last.Dur
	if last.To < 1 || m.T < end {
		return 0, false
	}
	return m.T - end, true
}
