// Package motion gives steps their meaning over time. Timing, easing and
// enter and exit animation arrive in milestone 2. For now it computes how a
// scene looks once all of its steps have run, which is what a still shows.
package motion

import (
	c "github.com/nessalabs/nessa-video/internal/composition"
)

// End is a scene's final state.
type End struct {
	State   map[*c.Element]string // state after steps; falls back to the computed state
	Gone    map[*c.Element]bool   // elements that left during the scene
	Camera  *c.Element            // zoom or pan target, nil for the full stage
	Caption string                // the last caption said in the scene
}

// StateOf returns e's final state.
func (en End) StateOf(e *c.Element) string {
	if s, ok := en.State[e]; ok {
		return s
	}
	if v, ok := e.Computed["state"]; ok {
		return v.Value
	}
	return "rest"
}

// EndOf runs a scene's steps in order and returns where they leave it.
func EndOf(s *c.Scene) End {
	en := End{State: map[*c.Element]string{}, Gone: map[*c.Element]bool{}}
	for _, st := range s.Steps {
		var targets []*c.Element
		if st.Target != nil {
			targets = c.Select(s.Root, st.Target)
		}
		switch st.Verb {
		case "focus", "dim", "active", "select", "trace", "rest":
			for _, t := range targets {
				en.State[t] = st.Verb
			}
		case "leave":
			for _, t := range targets {
				t.Walk(func(x *c.Element) { en.Gone[x] = true })
			}
		case "camera":
			if len(st.Args) > 0 && st.Args[0] == "reset" {
				en.Camera = nil
			} else if len(targets) > 0 {
				en.Camera = targets[0]
			}
		case "say":
			en.Caption = st.Text
		}
	}
	return en
}
