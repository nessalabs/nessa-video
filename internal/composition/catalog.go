package composition

import "sort"

// The catalog is the ubiquitous language of a script: every built-in kind,
// verb and property. `reel ref` prints it and `reel check` validates against it.
// Kinds defined by themes or `define` blocks are added at resolve time.

// Primitives are drawn directly.
var Primitives = []string{
	"box", "card", "circle", "decision", "store", "stack",
	"title", "p", "code", "label", "metric",
}

// Layouts arrange their children.
var Layouts = []string{"row", "col", "grid", "layer", "place"}

// Blocks are content with their own input format.
var Blocks = []string{"diff", "terminal", "array", "chart", "timeline", "mermaid", "math"}

// Verbs are steps. They change what is on screen over time.
var Verbs = []string{
	"focus", "dim", "active", "select", "trace", "rest", // states
	"leave", "clear", // exits
	"camera", "send", "type", "play", "walk", "swap", "mark",
	"say", "wait", "hold",
}

// States an element can be in.
var States = []string{"rest", "focus", "dim", "active", "select", "trace"}

// EnterModes and ExitModes share the same names.
var EnterModes = []string{"draw", "fade", "rise", "type", "none"}

// Props lists every built-in property name.
var Props = []string{
	// layout
	"w", "h", "r", "gap", "align", "justify", "pad", "wrap", "cols", "span", "x", "y", "scale",
	// look and state
	"state", "font", "color", "size", "note", "label", "title", "route", "side",
	// motion
	"enter", "exit", "stagger", "for", "at", "ease", "keep",
	// content
	"file", "path", "lines", "hunk", "focus", "lang", "items", "n", "data", "kind",
}

// IsVerb reports whether k names a step.
func IsVerb(k string) bool { return contains(Verbs, k) }

// IsBuiltinKind reports whether k is a built-in element kind.
func IsBuiltinKind(k string) bool {
	return contains(Primitives, k) || contains(Layouts, k) || contains(Blocks, k) || k == "use"
}

// IsProp reports whether p is a built-in property.
func IsProp(p string) bool { return contains(Props, p) }

// AllKinds returns built-in kinds and verbs, for suggestions.
func AllKinds() []string {
	out := append(append(append(append([]string{}, Primitives...), Layouts...), Blocks...), Verbs...)
	sort.Strings(out)
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
