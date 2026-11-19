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
	"camera", "send", "type", "play", "walk", "swap", "mark", "ptr",
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
	"ptr", "cells", "unit", "dir", "rank", "order", "hidden", "fit", "context", "speak", "model", "on",
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

// KindProps lists the properties that mean something for each kind, beyond
// the ones every element takes (state, enter, exit, hidden, w, h). It is
// what `reel ref` prints.
var KindProps = map[string]string{
	"box":      `"text" r= (corner radius)`,
	"card":     `"text that wraps" title="header" w= (wrap width)`,
	"circle":   `"text" r=`,
	"decision": `"text"`,
	"store":    `"text"`,
	"stack":    `"text"`,
	"title":    `"text with ==emphasis=="`,
	"p":        `"text" (wraps)`,
	"label":    `"text" color= font= size=`,
	"metric":   `"1024" label="what it counts" note="where it comes from" size=`,
	"code":     `LANG, indented lines or file= lines=; focus= context= (lines kept around focus)`,
	"diff":     `indented git diff or file= path= hunk=; focus=+12..+14 context=`,
	"terminal": `indented lines; "$ " starts a command`,
	"array":    `values... ptr="lo=0 hi=7"`,
	"chart":    `bar|line, indented CSV with a header row`,
	"timeline": `unit=, indented lanes: label | 0-18 text | *18-22 strong`,
	"mermaid":  `indented flowchart LR|TD or sequenceDiagram (participant, ->>, -->>, -x, Note, alt/else/opt/loop/par/end, autonumber)`,
	"math":     `"TeX" size=`,
	"row":      `gap= align= justify= pad= dir= fit=`,
	"col":      `gap= align= justify= pad= dir= fit=`,
	"grid":     `cols= gap=`,
	"layer":    `(children stacked)`,
	"place":    `children take x= y=`,
}

// TimingProp reports whether a step property sets when the step runs
// rather than what it does.
func TimingProp(k string) bool { return k == "at" || k == "for" || k == "on" || k == "ease" }
