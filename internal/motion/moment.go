package motion

import (
	"math"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/source"
)

// Appearance is how one element looks at one moment.
type Appearance struct {
	Opacity float64 // 0 hidden, 1 fully shown
	Draw    float64 // how much of the outline is traced, 0 to 1
	FillIn  float64 // fills and text inside a drawn shape, 0 to 1
	Rise    float64 // upward offset still to travel, in design units
	Reveal  float64 // share of text typed so far, 0 to 1
	State   string  // the state being moved to
	From    string  // the state being left
	Mix     float64 // 0 at From, 1 at State
}

// Moment is a scene at one time.
type Moment struct {
	plan *ScenePlan
	T    float64
}

// At returns the moment t seconds into the scene.
func (sp *ScenePlan) At(t float64) Moment { return Moment{plan: sp, T: t} }

// End returns the moment after all steps, before the exits that make room
// for the next scene.
func (sp *ScenePlan) End() Moment {
	t := sp.Dur
	if sp.closeAt > 0 {
		t = sp.closeAt
	}
	return Moment{plan: sp, T: t - 1e-6}
}

func applyMode(mode string, p float64, ap *Appearance) {
	switch mode {
	case "draw":
		ap.Draw = clamp(p * 1.4)
		ap.FillIn = clamp((p - 0.45) / 0.55)
		ap.Opacity = 1
		if p <= 0 {
			ap.Opacity = 0
		}
	case "rise":
		ap.Opacity = p
		ap.Rise = (1 - p) * 14
	case "none":
		ap.Opacity = 1
		if p <= 0 {
			ap.Opacity = 0
		}
	default: // fade, type
		ap.Opacity = p
	}
}

// Look returns e's appearance.
func (m Moment) Look(e *c.Element) Appearance {
	sp, t := m.plan, m.T
	ap := Appearance{Opacity: 1, Draw: 1, FillIn: 1, Reveal: 1, Mix: 1}
	if a, ok := sp.enter[e]; ok {
		if t < a.Start {
			ap.Opacity, ap.Draw, ap.FillIn = 0, 0, 0
		} else {
			applyMode(a.Mode, a.progress(t), &ap)
		}
	}
	if a, ok := sp.exit[e]; ok && t >= a.Start {
		var out Appearance
		out.Draw, out.FillIn = 1, 1
		applyMode(a.Mode, 1-a.progress(t), &out)
		ap.Opacity = math.Min(ap.Opacity, out.Opacity)
		ap.Draw = math.Min(ap.Draw, out.Draw)
		ap.FillIn = math.Min(ap.FillIn, out.FillIn)
		ap.Rise = math.Max(ap.Rise, out.Rise)
	}
	if a, ok := sp.types[e]; ok {
		ap.Reveal = 0
		if t >= a.Start {
			ap.Reveal = clamp((t - a.Start) / a.Dur)
		}
	}
	// state
	base := "rest"
	if s, ok := sp.initial[e]; ok {
		base = s
	} else if v, ok := e.Computed["state"]; ok {
		base = v.Value
	}
	ap.State, ap.From = base, base
	for _, ch := range sp.changes[e] {
		if t < ch.Start {
			break
		}
		ap.From, ap.State = ch.From, ch.To
		ap.Mix = 1
		if ch.Dur > 0 {
			ap.Mix = Ease((t - ch.Start) / ch.Dur)
		}
	}
	return ap
}

// Camera returns the camera move in effect: from and to targets (nil is
// the whole stage) and progress between them.
func (m Moment) Camera() (from, to *c.Element, mix float64) {
	sp := m.plan
	from, to, mix = sp.camInit, sp.camInit, 1
	for _, mv := range sp.camera {
		if m.T < mv.Start {
			break
		}
		from, to = mv.From, mv.To
		mix = 1
		if mv.Dur > 0 {
			mix = Ease((m.T - mv.Start) / mv.Dur)
		}
	}
	return
}

// Caption returns the caption text and its opacity.
func (m Moment) Caption() (string, float64) {
	sp := m.plan
	text, start := "", 0.0
	for _, cp := range sp.captions {
		if m.T >= cp.Start {
			text, start = cp.Text, cp.Start
		}
	}
	if text == "" {
		return "", 0
	}
	op := clamp((m.T - start) / 0.3)
	op = math.Min(op, clamp((sp.Dur-m.T)/0.3))
	return text, op
}

// Packet is a dot travelling along a connector.
type Packet struct {
	Edge     *c.Element
	Progress float64
}

// Packets returns the packets in flight.
func (m Moment) Packets() []Packet {
	var out []Packet
	for _, s := range m.plan.sends {
		if m.T >= s.Start && m.T <= s.Start+s.Dur {
			out = append(out, Packet{Edge: s.Edge, Progress: Ease((m.T - s.Start) / s.Dur)})
		}
	}
	return out
}

// Focus returns the emphasised lines of a code or diff block: the selection
// being left, the one being moved to (nil means all lines) and progress.
func (m Moment) Focus(e *c.Element) (from, to source.Selection, mix float64) {
	if e.Code != nil {
		from, to = e.Code.Focus, e.Code.Focus
	}
	mix = 1
	for _, mk := range m.plan.marks[e] {
		if m.T < mk.Start {
			break
		}
		from, to = to, mk.Lines
		mix = 1
		if mk.Dur > 0 {
			mix = Ease((m.T - mk.Start) / mk.Dur)
		}
	}
	return
}
