package composition

import (
	"github.com/nessalabs/nessa-video/internal/diag"
	"github.com/nessalabs/nessa-video/internal/selector"
)

// Layer orders where a property value came from. Higher layers win.
type Layer int

const (
	LayerThemeDefault Layer = iota // theme defaults per kind
	LayerThemeRule                 // theme style rules
	LayerScriptRule                // the script's style block
	LayerInline                    // written on the element's own line
)

func (l Layer) String() string {
	return [...]string{"theme default", "theme rule", "script rule", "inline"}[l]
}

// Decl is one property assignment.
type Decl struct {
	Key, Value string
	Pos        diag.Pos
}

// Rule is a selector with declarations, from a theme or a script style block.
type Rule struct {
	Selector *selector.Selector
	Decls    []Decl
	Layer    Layer
	Order    int // position among all rules; later wins ties
	Pos      diag.Pos
}

// Origin explains why a computed value won.
type Origin struct {
	Layer       Layer
	Rule        *Rule // nil for inline values
	Specificity selector.Specificity
	Pos         diag.Pos
	Inherited   bool // copied from the parent because nothing set it here
}

// Value is a computed property value and where it came from.
type Value struct {
	Value  string
	Origin Origin
}

// Inherited lists properties that flow from a container to its children
// when nothing sets them on the child.
var Inherited = map[string]bool{"font": true, "color": true, "enter": true, "exit": true}

// Cascade computes every element's properties under root.
//
// For each property the winner is the declaration with the highest layer,
// then the highest selector specificity, then the latest rule. Inline
// declarations beat all rules. A few properties inherit from the parent.
func Cascade(root *Element, rules []*Rule) {
	var visit func(e *Element)
	visit = func(e *Element) {
		e.Computed = map[string]Value{}
		type cand struct {
			v     Value
			order int
		}
		best := map[string]cand{}
		better := func(a, b cand) bool { // does a beat b?
			if a.v.Origin.Layer != b.v.Origin.Layer {
				return a.v.Origin.Layer > b.v.Origin.Layer
			}
			if a.v.Origin.Specificity != b.v.Origin.Specificity {
				return b.v.Origin.Specificity.Less(a.v.Origin.Specificity)
			}
			return a.order > b.order
		}
		for _, r := range rules {
			spec, ok := r.Selector.Match(e)
			if !ok {
				continue
			}
			for _, d := range r.Decls {
				c := cand{Value{d.Value, Origin{Layer: r.Layer, Rule: r, Specificity: spec, Pos: d.Pos}}, r.Order}
				if old, seen := best[d.Key]; !seen || better(c, old) {
					best[d.Key] = c
				}
			}
		}
		for i, d := range e.Inline {
			c := cand{Value{d.Value, Origin{Layer: LayerInline, Pos: d.Pos}}, 1<<30 + i}
			best[d.Key] = c
		}
		for k, c := range best {
			e.Computed[k] = c.v
		}
		if e.parent != nil {
			for k := range Inherited {
				if _, set := e.Computed[k]; set {
					continue
				}
				if pv, ok := e.parent.Computed[k]; ok {
					pv.Origin.Inherited = true
					e.Computed[k] = pv
				}
			}
		}
		for _, c := range e.Children {
			visit(c)
		}
	}
	visit(root)
}
