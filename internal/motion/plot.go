package motion

import (
	"math"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/plot"
)

// Plots in motion: play traces their curves, set eases a parameter so the
// curves morph, note pins an amber label to a place, and zoom rescales the
// axes toward a point.

// Timings of plot motion, in seconds.
const (
	PlotTrace   = 4.0 // tracing every curve from start to end
	PlotMinPlay = 1.2 // the shortest trace a play step takes
	PlotSetTime = 1.6 // a parameter easing to its new value
	NoteTime    = 0.6 // a note's ring and label coming in
	ZoomTime    = 1.8 // the axes rescaling
	DotFade     = 0.5 // the leading dot fading once the trace is done
)

// ParamSet eases parameter Index of a plot to Value.
type ParamSet struct {
	Index      int
	Value      float64
	Start, Dur float64
}

// PlotNote is an amber ring and label pinned to a place on a plot.
type PlotNote struct {
	X, Y       float64
	Text       string
	Start, Dur float64
}

// plotPlan is what steps do to one plot in a scene.
type plotPlan struct {
	sets  []ParamSet
	zooms []plot.Zoom
	notes []PlotNote
}

func (sp *ScenePlan) plotOf(e *c.Element) *plotPlan {
	d := sp.dataOf(e)
	if d.plot == nil {
		d.plot = &plotPlan{}
	}
	return d.plot
}

// played reports whether a play step in the scene names e.
func (sp *ScenePlan) played(e *c.Element) bool {
	for _, st := range sp.Scene.Steps {
		if st.Verb != "play" || st.Target == nil {
			continue
		}
		for _, x := range c.Select(sp.Scene.Root, st.Target) {
			if x == e {
				return true
			}
		}
	}
	return false
}

// plotPlayDur is how long a play step on a plot traces: the share of the
// curves it covers at PlotTrace for the whole, never under PlotMinPlay.
// A play that runs during a spoken line and covers the rest keeps the
// line's length.
func (sp *ScenePlan) plotPlayDur(st c.Step, targets []*c.Element, dur float64, inLine bool) float64 {
	v, stepwise := st.Lookup("to")
	for _, e := range targets {
		if e.Plot == nil {
			continue
		}
		from := 0.0
		if segs := sp.plays[e]; len(segs) > 0 {
			from = segs[len(segs)-1].To
		}
		to := 1.0
		if stepwise {
			to, _ = e.Plot.Fraction(v)
		}
		if inLine && !stepwise {
			return math.Max(dur, PlotMinPlay)
		}
		return math.Max(PlotMinPlay, PlotTrace*math.Abs(to-from))
	}
	return dur
}

// planPlot records a set, note or zoom on a plot and reports whether the
// step was one.
func (sp *ScenePlan) planPlot(st c.Step, targets []*c.Element, start, dur float64) bool {
	done := false
	for _, e := range targets {
		if e.Plot == nil {
			continue
		}
		pp := sp.plotOf(e)
		switch st.Verb {
		case "set":
			for _, d := range c.PlotDecls(st.Decls, e.Plot) {
				v, err := plot.Const(d.Value)
				if err != nil {
					continue
				}
				pp.sets = append(pp.sets, ParamSet{Index: e.Plot.Param(d.Key), Value: v, Start: start, Dur: dur})
				done = true
			}
		case "note":
			place := strings.Join(st.Args, " ")
			if v, ok := st.Lookup("at"); ok && strings.HasPrefix(v, "(") {
				place = v
			}
			x, y, err := c.PlotPlace(place)
			if err == nil {
				pp.notes = append(pp.notes, PlotNote{X: x, Y: y, Text: st.Text, Start: start, Dur: dur})
			}
			done = true
		case "zoom":
			z := plot.Zoom{Start: start, Dur: dur}
			xv, hasX := st.Lookup("x")
			yv, hasY := st.Lookup("y")
			if hasX || hasY {
				r := plot.Ranges{X0: e.Plot.X[0], X1: e.Plot.X[1], Y0: e.Plot.Y[0], Y1: e.Plot.Y[1]}
				if lo, hi, err := plot.Range(xv); hasX && err == nil {
					r.X0, r.X1 = lo, hi
				}
				if lo, hi, err := plot.Range(yv); hasY && err == nil {
					r.Y0, r.Y1 = lo, hi
				}
				z.To = &r
			}
			pp.zooms = append(pp.zooms, z)
			done = true
		}
	}
	return done
}

// plotStepDur is the default length of a plot step.
func plotStepDur(st c.Step, targets []*c.Element, dur float64) float64 {
	for _, e := range targets {
		if e.Plot == nil {
			continue
		}
		switch st.Verb {
		case "set":
			if len(c.PlotDecls(st.Decls, e.Plot)) > 0 {
				return PlotSetTime
			}
		case "note":
			return NoteTime
		case "zoom":
			return ZoomTime
		}
	}
	return dur
}

// carryPlot brings a plot over from the scene before, settled: its trace,
// parameters, zooms and notes are where they ended.
func (sp *ScenePlan) carryPlot(prev *ScenePlan, pe, e *c.Element) {
	if e.Plot == nil {
		return
	}
	end := prev.At(prev.Dur)
	pos, _ := end.PlotTrace(pe)
	if prev.played(pe) || pos < 1 {
		sp.plays[e] = append(sp.plays[e], PlaySeg{Start: -1, Dur: 0, From: pos, To: pos})
	}
	d := prev.data[pe]
	if d == nil || d.plot == nil {
		return
	}
	pp := sp.plotOf(e)
	for _, s := range d.plot.sets {
		s.Start, s.Dur = -1, 0
		pp.sets = append(pp.sets, s)
	}
	for _, z := range d.plot.zooms {
		z.Start, z.Dur = -1, 0
		pp.zooms = append(pp.zooms, z)
	}
	for _, n := range d.plot.notes {
		n.Start, n.Dur = -1, 0
		pp.notes = append(pp.notes, n)
	}
}

// PlotTrace returns how much of a plot's curves are drawn (0 to 1) and how
// strongly the leading dot shows at the tip. Without a play step in the
// scene, curves trace in as the plot enters.
func (m Moment) PlotTrace(e *c.Element) (pos, dot float64) {
	segs := m.plan.plays[e]
	if len(segs) == 0 {
		if since, dur, ok := m.Grow(e); ok {
			pos = Ease(clamp(since / dur))
			if since >= 0 {
				dot = 1 - clamp((since-dur)/DotFade)
			}
			return pos, dot
		}
		return 1, 0
	}
	started := false
	var end float64
	for _, sg := range segs {
		if m.T < sg.Start {
			break
		}
		f := 1.0
		if sg.Dur > 0 {
			f = Ease((m.T - sg.Start) / sg.Dur)
		}
		pos = sg.From + (sg.To-sg.From)*f
		end = sg.Start + sg.Dur
		started = started || sg.Dur > 0 || sg.From > 0
	}
	if !started {
		return pos, 0
	}
	dot = 1
	if pos >= 1 {
		dot = 1 - clamp((m.T-end)/DotFade)
	}
	return pos, dot
}

// PlotReached returns how long ago the trace first reached share f of the
// curves, and whether it has.
func (m Moment) PlotReached(e *c.Element, f float64) (float64, bool) {
	segs := m.plan.plays[e]
	when := math.Inf(1)
	if len(segs) == 0 {
		since, dur, ok := m.Grow(e)
		if !ok {
			return math.Inf(1), true
		}
		when = m.T - since + dur*invEase(f)
	}
	for _, sg := range segs {
		if sg.Start < 0 && sg.Dur == 0 { // carried from the scene before
			if f <= sg.To {
				when = math.Inf(-1)
				break
			}
			continue
		}
		if f < sg.From || f > sg.To {
			continue
		}
		when = sg.Start
		if sg.Dur > 0 && sg.To > sg.From {
			when += sg.Dur * invEase((f-sg.From)/(sg.To-sg.From))
		}
		break
	}
	if m.T < when {
		return 0, false
	}
	return m.T - when, true
}

// invEase returns x with Ease(x) = y.
func invEase(y float64) float64 {
	lo, hi := 0.0, 1.0
	for i := 0; i < 40; i++ {
		mid := (lo + hi) / 2
		if Ease(mid) < y {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// PlotParams returns a plot's parameter values at this moment, and the
// readout of the parameter a set step changed last: its index (-1 for
// none) and how strongly it shows.
func (m Moment) PlotParams(e *c.Element) (vals []float64, readout int, show float64) {
	vals = e.Plot.Defaults()
	readout = -1
	d := m.plan.data[e]
	if d == nil || d.plot == nil {
		return vals, -1, 0
	}
	base := append([]float64(nil), vals...)
	for _, s := range d.plot.sets {
		if m.T < s.Start || s.Index < 0 || s.Index >= len(vals) {
			continue
		}
		f := 1.0
		if s.Dur > 0 {
			f = Ease((m.T - s.Start) / s.Dur)
		}
		vals[s.Index] = base[s.Index] + (s.Value-base[s.Index])*f
		if f >= 1 {
			base[s.Index] = s.Value
		}
		readout = s.Index
		show = 1
		if s.Start >= 0 {
			show = clamp((m.T - s.Start) / 0.3)
		}
	}
	return vals, readout, show
}

// PlotZooms returns the zoom steps on a plot, in time order.
func (m Moment) PlotZooms(e *c.Element) []plot.Zoom {
	d := m.plan.data[e]
	if d == nil || d.plot == nil {
		return nil
	}
	return d.plot.zooms
}

// NoteNow is a plot note at one moment: how far it has come in (0 to 1).
type NoteNow struct {
	PlotNote
	In float64
}

// PlotNotes returns a plot's notes that have started, with their progress.
func (m Moment) PlotNotes(e *c.Element) []NoteNow {
	d := m.plan.data[e]
	if d == nil || d.plot == nil {
		return nil
	}
	var out []NoteNow
	for _, n := range d.plot.notes {
		if m.T < n.Start {
			continue
		}
		in := 1.0
		if n.Dur > 0 {
			in = clamp((m.T - n.Start) / n.Dur)
		}
		out = append(out, NoteNow{n, in})
	}
	return out
}
