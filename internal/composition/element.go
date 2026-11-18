// Package composition is reel's core domain: what is on screen in each scene
// and how it is styled. It knows nothing about script syntax, drawing or
// encoding. The resolve package translates scripts into this model.
package composition

import (
	"github.com/nessalabs/nessa-video/internal/diag"
	"github.com/nessalabs/nessa-video/internal/selector"
	"github.com/nessalabs/nessa-video/internal/source"
)

// Element is one thing in a scene: a primitive, a component instance, a
// part inside a component, a layout container or a connector.
type Element struct {
	kind    string
	id      string // unique within a scene; "" for parts and anonymous elements
	part    string // name within the owning component instance
	classes []string

	Text   string   // the quoted text on the element's line
	Args   []string // other positional arguments, e.g. a count
	Inline []Decl   // properties written on the element's own line
	Body   []string // verbatim body for terminal, code, math and mermaid
	Pos    diag.Pos

	// Edge is set when the element is a connector.
	Edge *Edge

	// Code is set for code and diff blocks.
	Code *CodeBlock

	parent   *Element
	owner    *Element // component instance that owns this part
	Children []*Element

	// Computed holds cascaded property values after Cascade runs.
	Computed map[string]Value

	// Carried is true for elements copied from the previous scene.
	Carried bool
	source  *Element // the element this one was copied from, if any
}

// Identity returns the original element this one descends from by copying
// across scenes. Copies of the same element share an identity.
func (e *Element) Identity() *Element {
	if e.source != nil {
		return e.source
	}
	return e
}

// CodeBlock is source code or a change to it, as numbered lines.
type CodeBlock struct {
	Lang   string // lexer name, "" for plain text
	Path   string // file path shown in the header
	Header string // hunk header, such as the enclosing function
	Diff   bool
	Lines  []source.Line
	Focus  source.Selection // lines to emphasise; nil means all
}

// Edge describes a connector between two sets of elements.
type Edge struct {
	Op       string // ->, -|, ~>, ..>, <->, --, ..
	From, To *selector.Selector
}

// NewElement creates an element of the given kind.
func NewElement(kind string, pos diag.Pos) *Element {
	return &Element{kind: kind, Pos: pos}
}

func (e *Element) SetID(id string)         { e.id = id }
func (e *Element) SetPart(name string)     { e.part = name }
func (e *Element) AddClass(c string)       { e.classes = append(e.classes, c) }
func (e *Element) Classes() []string       { return e.classes }
func (e *Element) SetOwner(o *Element)     { e.owner = o }
func (e *Element) OwnerElement() *Element  { return e.owner }
func (e *Element) ParentElement() *Element { return e.parent }

// Append adds child as the last child of e.
func (e *Element) Append(child *Element) {
	child.parent = e
	e.Children = append(e.Children, child)
}

// Walk visits e and its descendants depth first.
func (e *Element) Walk(fn func(*Element)) {
	fn(e)
	for _, c := range e.Children {
		c.Walk(fn)
	}
}

// Clone deep-copies the subtree. Owners inside the subtree are remapped to
// their copies; owners outside it are kept.
func (e *Element) Clone() *Element {
	m := map[*Element]*Element{}
	var cp func(*Element) *Element
	cp = func(x *Element) *Element {
		y := *x
		y.classes = append([]string(nil), x.classes...)
		y.Inline = append([]Decl(nil), x.Inline...)
		y.Children = nil
		y.Computed = nil
		y.Carried = true
		y.source = x.Identity()
		m[x] = &y
		for _, c := range x.Children {
			cc := cp(c)
			cc.parent = &y
			y.Children = append(y.Children, cc)
		}
		return &y
	}
	root := cp(e)
	root.parent = nil
	root.Walk(func(x *Element) {
		if o, ok := m[x.owner]; ok {
			x.owner = o
		}
	})
	return root
}

// Label returns a short human name for messages, such as `server#api` or `#api/unit1`.
func (e *Element) Label() string {
	switch {
	case e.id != "":
		return e.kind + "#" + e.id
	case e.part != "" && e.owner != nil:
		return e.owner.Label() + "/" + e.part
	default:
		return e.kind
	}
}

// selector.Element implementation.

func (e *Element) Kind() string     { return e.kind }
func (e *Element) ID() string       { return e.id }
func (e *Element) PartName() string { return e.part }
func (e *Element) HasClass(c string) bool {
	for _, x := range e.classes {
		if x == c {
			return true
		}
	}
	return false
}

// Attr reads inline declarations only, so attribute selectors never depend
// on the cascade they take part in.
func (e *Element) Attr(k string) (string, bool) {
	for i := len(e.Inline) - 1; i >= 0; i-- {
		if e.Inline[i].Key == k {
			return e.Inline[i].Value, true
		}
	}
	return "", false
}

func (e *Element) Parent() selector.Element {
	if e.parent == nil {
		return nil
	}
	return e.parent
}

func (e *Element) Owner() selector.Element {
	if e.owner == nil {
		return nil
	}
	return e.owner
}

func (e *Element) Index() int {
	if e.parent == nil {
		return 1
	}
	for i, c := range e.parent.Children {
		if c == e {
			return i + 1
		}
	}
	return 0
}

func (e *Element) SiblingCount() int {
	if e.parent == nil {
		return 1
	}
	return len(e.parent.Children)
}

// Select returns every element under root (root included) matching s, in document order.
func Select(root *Element, s *selector.Selector) []*Element {
	var out []*Element
	root.Walk(func(e *Element) {
		if s.Matches(e) {
			out = append(out, e)
		}
	})
	return out
}
