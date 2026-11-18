// Package selector is the shared kernel for the selector language used by
// style rules, steps, connectors and the camera.
package selector

import (
	"fmt"
	"strconv"
	"strings"
)

// Selector is a comma-separated group of complex selectors.
type Selector struct {
	Src   string
	Group []Complex
}

// Complex is a chain of compounds joined by combinators, left to right.
type Complex struct {
	Parts []Compound
	// Comb[i] joins Parts[i] and Parts[i+1]: ' ' descendant, '>' child, '/' part.
	Comb []byte
}

// Compound matches one element: kind#id.class[k=v]:pseudo.
type Compound struct {
	Kind    string // "" or "*" matches any
	ID      string
	Part    string // used after '/' combinator: the part name
	Classes []string
	Attrs   []Attr
	Pseudos []Pseudo
}

type Attr struct{ Key, Value string }

type Pseudo struct {
	Name string // first, last, nth, not
	N    int
	Not  *Selector
}

// Specificity is (ids, classes+attrs+pseudos, kinds), compared left to right.
type Specificity [3]int

func (a Specificity) Less(b Specificity) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func (c Complex) Specificity() Specificity {
	var s Specificity
	for _, p := range c.Parts {
		if p.ID != "" {
			s[0]++
		}
		if p.Part != "" {
			s[1]++
		}
		s[1] += len(p.Classes) + len(p.Attrs)
		for _, ps := range p.Pseudos {
			if ps.Not != nil {
				best := Specificity{}
				for _, g := range ps.Not.Group {
					if sp := g.Specificity(); best.Less(sp) {
						best = sp
					}
				}
				s[0] += best[0]
				s[1] += best[1]
				s[2] += best[2]
			} else {
				s[1]++
			}
		}
		if p.Kind != "" && p.Kind != "*" {
			s[2]++
		}
	}
	return s
}

// ParseSelector parses selector source such as `#api process:not(.hot), row > box`.
func ParseSelector(src string) (*Selector, error) {
	sel := &Selector{Src: strings.TrimSpace(src)}
	for _, part := range splitTop(sel.Src, ',') {
		c, err := parseComplex(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		sel.Group = append(sel.Group, c)
	}
	if len(sel.Group) == 0 {
		return nil, fmt.Errorf("empty selector")
	}
	return sel, nil
}

func splitTop(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

func isName(b byte) bool {
	return b == '-' || b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func parseComplex(s string) (Complex, error) {
	var c Complex
	i := 0
	pendingPart := false
	for i < len(s) {
		comp, n, err := parseCompound(s[i:], pendingPart)
		if err != nil {
			return c, fmt.Errorf("%w in %q", err, s)
		}
		c.Parts = append(c.Parts, comp)
		i += n
		pendingPart = false
		// combinator
		j := i
		for j < len(s) && s[j] == ' ' {
			j++
		}
		if j >= len(s) {
			break
		}
		comb := byte(' ')
		if s[j] == '>' || s[j] == '/' {
			comb = s[j]
			j++
			for j < len(s) && s[j] == ' ' {
				j++
			}
		} else if j == i {
			return c, fmt.Errorf("unexpected %q in %q", s[j], s)
		}
		if j >= len(s) {
			return c, fmt.Errorf("selector ends with %q", comb)
		}
		c.Comb = append(c.Comb, comb)
		pendingPart = comb == '/'
		i = j
	}
	if len(c.Parts) == 0 {
		return c, fmt.Errorf("empty selector")
	}
	return c, nil
}

func parseCompound(s string, isPart bool) (Compound, int, error) {
	var c Compound
	i := 0
	name := func() string {
		j := i
		for j < len(s) && isName(s[j]) {
			j++
		}
		r := s[i:j]
		i = j
		return r
	}
	if isPart {
		c.Part = name()
		if c.Part == "" {
			return c, i, fmt.Errorf("missing part name after /")
		}
	} else if i < len(s) && (isName(s[i]) || s[i] == '*') {
		if s[i] == '*' {
			c.Kind = "*"
			i++
		} else {
			c.Kind = name()
		}
	}
	for i < len(s) {
		switch s[i] {
		case '#':
			i++
			if c.ID = name(); c.ID == "" {
				return c, i, fmt.Errorf("missing id after #")
			}
		case '.':
			i++
			cl := name()
			if cl == "" {
				return c, i, fmt.Errorf("missing class after .")
			}
			c.Classes = append(c.Classes, cl)
		case '[':
			end := strings.IndexByte(s[i:], ']')
			if end < 0 {
				return c, i, fmt.Errorf("missing ]")
			}
			kv := s[i+1 : i+end]
			k, v, _ := strings.Cut(kv, "=")
			c.Attrs = append(c.Attrs, Attr{strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`)})
			i += end + 1
		case ':':
			i++
			pn := name()
			ps := Pseudo{Name: pn}
			if i < len(s) && s[i] == '(' {
				depth, j := 0, i
				for ; j < len(s); j++ {
					if s[j] == '(' {
						depth++
					} else if s[j] == ')' {
						if depth--; depth == 0 {
							break
						}
					}
				}
				if j >= len(s) {
					return c, i, fmt.Errorf("missing ) after :%s", pn)
				}
				arg := s[i+1 : j]
				i = j + 1
				switch pn {
				case "not":
					inner, err := ParseSelector(arg)
					if err != nil {
						return c, i, err
					}
					ps.Not = inner
				case "nth":
					n, err := strconv.Atoi(strings.TrimSpace(arg))
					if err != nil || n < 1 {
						return c, i, fmt.Errorf(":nth needs a number from 1, got %q", arg)
					}
					ps.N = n
				default:
					return c, i, fmt.Errorf("unknown pseudo :%s", pn)
				}
			} else if pn != "first" && pn != "last" {
				return c, i, fmt.Errorf("unknown pseudo :%s (use :first, :last, :nth(n) or :not(…))", pn)
			}
			c.Pseudos = append(c.Pseudos, ps)
		default:
			return c, i, nil
		}
	}
	return c, i, nil
}

// Element is what selectors match against. The resolver's scene graph implements it.
type Element interface {
	Kind() string
	ID() string
	PartName() string // name within its component, "" if none
	HasClass(string) bool
	Attr(string) (string, bool)
	Parent() Element
	Owner() Element // component instance that owns this part, nil if none
	Index() int     // 1-based among siblings
	SiblingCount() int
}

// Matches reports whether e matches the selector.
func (s *Selector) Matches(e Element) bool {
	_, ok := s.Match(e)
	return ok
}

// Match reports whether e matches, and the highest specificity among the
// matching members of the group, which is what the cascade compares.
func (s *Selector) Match(e Element) (Specificity, bool) {
	var best Specificity
	found := false
	for _, c := range s.Group {
		if c.matches(e, len(c.Parts)-1) {
			if sp := c.Specificity(); !found || best.Less(sp) {
				best = sp
			}
			found = true
		}
	}
	return best, found
}

func (c Complex) matches(e Element, k int) bool {
	if e == nil || !c.Parts[k].matches(e) {
		return false
	}
	if k == 0 {
		return true
	}
	switch c.Comb[k-1] {
	case '>':
		return c.matches(e.Parent(), k-1)
	case '/':
		return c.matches(e.Owner(), k-1)
	default:
		for a := e.Parent(); a != nil; a = a.Parent() {
			if c.matches(a, k-1) {
				return true
			}
		}
		return false
	}
}

func (p Compound) matches(e Element) bool {
	if p.Kind != "" && p.Kind != "*" && p.Kind != e.Kind() {
		return false
	}
	if p.ID != "" && p.ID != e.ID() {
		return false
	}
	if p.Part != "" && p.Part != e.PartName() {
		return false
	}
	for _, cl := range p.Classes {
		if !e.HasClass(cl) {
			return false
		}
	}
	for _, a := range p.Attrs {
		v, ok := e.Attr(a.Key)
		if !ok || (a.Value != "" && v != a.Value) {
			return false
		}
	}
	for _, ps := range p.Pseudos {
		switch ps.Name {
		case "first":
			if e.Index() != 1 {
				return false
			}
		case "last":
			if e.Index() != e.SiblingCount() {
				return false
			}
		case "nth":
			if e.Index() != ps.N {
				return false
			}
		case "not":
			if ps.Not.Matches(e) {
				return false
			}
		}
	}
	return true
}
