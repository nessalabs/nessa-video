// Package script parses .reel scripts into a generic line tree.
//
// The grammar is deliberately small: every line is
//
//	kind arg arg "text" key=value key="quoted value"
//
// nested by indentation. Meaning (which kinds exist, which args are ids or
// selectors) is decided later by the resolver, so new kinds from `define`
// blocks or plugins never need parser changes.
package script

import (
	"fmt"
	"strings"

	"reel/internal/diag"
)

// Pos is a 1-based source position.
type Pos = diag.Pos

// Arg is a positional token on a line.
type Arg struct {
	Pos
	Text   string
	Quoted bool
}

// Prop is a key=value token.
type Prop struct {
	Pos
	Key, Value string
	Quoted     bool
}

// Node is one line plus its indented children.
type Node struct {
	Pos
	Kind     string // first word; "edge" for connector lines
	With     bool   // line started with `with`
	Args     []Arg
	Props    []Prop
	Children []*Node
	Raw      []string // body lines for raw kinds (terminal, code, mermaid, fences)
	Lang     string   // fence language, e.g. "mermaid"
}

// Prop returns the value of key and whether it was set.
func (n *Node) Prop(key string) (string, bool) {
	for i := len(n.Props) - 1; i >= 0; i-- {
		if n.Props[i].Key == key {
			return n.Props[i].Value, true
		}
	}
	return "", false
}

// EdgeOps are the connector operators, longest first.
var EdgeOps = []string{"<->", "..>", "-->", "->", "-|", "~>", "--", ".."}

// RawKinds take their indented body verbatim instead of parsing it.
var RawKinds = map[string]bool{"terminal": true, "code": true, "math": true}

type line struct {
	no, indent int
	text       string
}

// Parse parses src into top-level nodes. It always returns as much of the
// tree as it could, plus every problem it found.
func Parse(src string) ([]*Node, diag.List) {
	var diags diag.List
	var lines []line
	for i, s := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimLeft(s, " \t")
		ind := len(s) - len(trimmed)
		if strings.ContainsRune(s[:ind], '\t') {
			diags.Errorf(Pos{Line: i + 1, Col: 1}, "indent with spaces", "tab used for indentation")
		}
		lines = append(lines, line{i + 1, ind, strings.TrimRight(trimmed, " \t")})
	}

	p := &parser{lines: lines}
	root := &Node{Kind: "root"}
	p.block(root, -1)
	return root.Children, append(diags, p.diags...)
}

type parser struct {
	lines []line
	i     int
	diags diag.List
}

func (p *parser) errf(pos Pos, hint, f string, a ...any) {
	p.diags.Errorf(pos, hint, f, a...)
}

func blank(l line) bool { return l.text == "" || strings.HasPrefix(l.text, "# ") || l.text == "#" }

// block parses lines indented deeper than parent into n.Children.
func (p *parser) block(n *Node, parent int) {
	child := -1
	for p.i < len(p.lines) {
		l := p.lines[p.i]
		if blank(l) {
			p.i++
			continue
		}
		if l.indent <= parent {
			return
		}
		if child == -1 {
			child = l.indent
		} else if l.indent != child {
			p.errf(Pos{Line: l.no, Col: l.indent + 1}, fmt.Sprintf("align with the line above (%d spaces)", child),
				"inconsistent indentation")
		}
		p.i++
		if strings.HasPrefix(l.text, "```") {
			n.Children = append(n.Children, p.fence(l))
			continue
		}
		c := p.parseLine(l)
		if c == nil {
			continue
		}
		n.Children = append(n.Children, c)
		if RawKinds[c.Kind] {
			c.Raw = p.raw(l.indent)
		} else {
			p.block(c, l.indent)
		}
	}
}

func (p *parser) fence(open line) *Node {
	n := &Node{Pos: Pos{Line: open.no, Col: open.indent + 1}, Kind: "fence", Lang: strings.TrimSpace(strings.TrimPrefix(open.text, "```"))}
	if n.Lang == "mermaid" {
		n.Kind = "mermaid"
	}
	for p.i < len(p.lines) {
		l := p.lines[p.i]
		p.i++
		if strings.HasPrefix(l.text, "```") {
			return n
		}
		n.Raw = append(n.Raw, strings.Repeat(" ", max(0, l.indent-open.indent))+l.text)
	}
	p.errf(n.Pos, "close it with a line containing ```", "unclosed ``` block")
	return n
}

// raw collects every line indented deeper than parent, keeping relative indentation.
func (p *parser) raw(parent int) []string {
	var out []string
	base := -1
	for p.i < len(p.lines) {
		l := p.lines[p.i]
		if l.text != "" && l.indent <= parent {
			break
		}
		p.i++
		if l.text == "" {
			out = append(out, "")
			continue
		}
		if base == -1 {
			base = l.indent
		}
		out = append(out, strings.Repeat(" ", max(0, l.indent-base))+l.text)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

type token struct {
	pos    Pos
	text   string
	quoted bool
	key    string // set for key=value
	isKV   bool
}

func (p *parser) tokenize(l line) []token {
	var toks []token
	s := l.text
	i := 0
	for i < len(s) {
		if s[i] == ' ' {
			i++
			continue
		}
		col := l.indent + i + 1
		pos := Pos{Line: l.no, Col: col}
		if s[i] == '#' && i+1 < len(s) && s[i+1] == ' ' && len(toks) > 0 {
			break // trailing comment
		}
		if s[i] == '"' {
			str, n, ok := quoted(s[i:])
			if !ok {
				p.errf(pos, `add the closing "`, "unterminated string")
			}
			toks = append(toks, token{pos: pos, text: str, quoted: true})
			i += n
			continue
		}
		j := i
		for j < len(s) && s[j] != ' ' && s[j] != '=' && s[j] != '"' {
			if s[j] == '(' || s[j] == '[' { // keep :not(.a, .b) and [state=dim] together
				open, close := s[j], byte(')')
				if open == '[' {
					close = ']'
				}
				depth := 0
				for j < len(s) {
					if s[j] == open {
						depth++
					} else if s[j] == close {
						depth--
						if depth == 0 {
							break
						}
					}
					j++
				}
			}
			if j < len(s) {
				j++
			}
		}
		word := s[i:j]
		if j < len(s) && s[j] == '=' && word != "" && !strings.HasPrefix(word, "[") {
			key := word
			j++
			if j < len(s) && s[j] == '"' {
				str, n, ok := quoted(s[j:])
				if !ok {
					p.errf(Pos{Line: l.no, Col: l.indent + j + 1}, `add the closing "`, "unterminated string")
				}
				toks = append(toks, token{pos: pos, key: key, text: str, quoted: true, isKV: true})
				i = j + n
				continue
			}
			k := j
			for k < len(s) && s[k] != ' ' {
				k++
			}
			toks = append(toks, token{pos: pos, key: key, text: s[j:k], isKV: true})
			i = k
			continue
		}
		if word == "" { // stray '=' or '"' mid-word
			word = s[i : i+1]
			j = i + 1
		}
		toks = append(toks, token{pos: pos, text: word})
		i = j
	}
	return toks
}

func quoted(s string) (string, int, bool) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				i++
				switch s[i] {
				case 'n':
					b.WriteByte('\n')
				default:
					b.WriteByte(s[i])
				}
			}
		case '"':
			return b.String(), i + 1, true
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), len(s), false
}

func isEdgeOp(s string) bool {
	for _, op := range EdgeOps {
		if s == op {
			return true
		}
	}
	return false
}

func (p *parser) parseLine(l line) *Node {
	toks := p.tokenize(l)
	if len(toks) == 0 {
		return nil
	}
	n := &Node{Pos: Pos{Line: l.no, Col: l.indent + 1}}
	if !toks[0].isKV && !toks[0].quoted && toks[0].text == "with" {
		n.With = true
		toks = toks[1:]
		if len(toks) == 0 {
			p.errf(n.Pos, "write the step after it, e.g. `with type #t`", "`with` needs a step")
			return nil
		}
	}
	// connector line: <selector> <op> <selector> ...
	if len(toks) >= 3 && !toks[1].isKV && !toks[1].quoted && isEdgeOp(toks[1].text) {
		n.Kind = "edge"
		n.Pos = toks[0].pos
		for _, t := range toks {
			n.add(t)
		}
		if toks[2].isKV || toks[2].quoted {
			p.errf(toks[2].pos, "e.g. `#web -> #api`", "connector needs a target after %s", toks[1].text)
		}
		return n
	}
	if toks[0].isKV || toks[0].quoted {
		p.errf(toks[0].pos, "start the line with a kind such as box, label or focus", "line has no kind")
		return nil
	}
	n.Kind = toks[0].text
	n.Pos = toks[0].pos
	for _, t := range toks[1:] {
		n.add(t)
	}
	return n
}

func (n *Node) add(t token) {
	if t.isKV {
		n.Props = append(n.Props, Prop{t.pos, t.key, t.text, t.quoted})
		return
	}
	n.Args = append(n.Args, Arg{t.pos, t.text, t.quoted})
}

// SetFile stamps every position in the tree with a source name.
func SetFile(nodes []*Node, file string) {
	for _, n := range nodes {
		n.Pos.File = file
		for i := range n.Args {
			n.Args[i].Pos.File = file
		}
		for i := range n.Props {
			n.Props[i].Pos.File = file
		}
		SetFile(n.Children, file)
	}
}
