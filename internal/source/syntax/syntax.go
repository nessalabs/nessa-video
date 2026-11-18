// Package syntax classes code tokens for colour, using Chroma lexers.
package syntax

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Class is a syntax colour class. Themes map each class to a colour token.
type Class string

const (
	Plain       Class = "plain"
	Keyword     Class = "keyword"
	Type        Class = "type"
	Function    Class = "function"
	String      Class = "string"
	Number      Class = "number"
	Comment     Class = "comment"
	Punctuation Class = "punctuation"
)

// Token is a run of text with one class.
type Token struct {
	Text  string
	Class Class
}

// Lexer names a language from an explicit name or a file name; "" if unknown.
func Lexer(lang, file string) string {
	if lang != "" {
		if l := lexers.Get(lang); l != nil {
			return l.Config().Name
		}
	}
	if file != "" {
		if l := lexers.Match(filepath.Base(file)); l != nil {
			return l.Config().Name
		}
	}
	return ""
}

// Highlight splits each line into classed tokens. The lines are lexed
// together so that multi-line strings and comments are classed correctly.
func Highlight(lang string, lines []string) [][]Token {
	out := make([][]Token, len(lines))
	lx := lexers.Get(lang)
	if lx == nil {
		for i, l := range lines {
			out[i] = []Token{{l, Plain}}
		}
		return out
	}
	lx = chroma.Coalesce(lx)
	it, err := lx.Tokenise(nil, strings.Join(lines, "\n")+"\n")
	if err != nil {
		for i, l := range lines {
			out[i] = []Token{{l, Plain}}
		}
		return out
	}
	row := 0
	for _, tk := range it.Tokens() {
		cls := classify(tk.Type)
		parts := strings.Split(tk.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				row++
			}
			if row >= len(out) {
				break
			}
			if p != "" {
				out[row] = append(out[row], Token{p, cls})
			}
		}
	}
	return out
}

func classify(t chroma.TokenType) Class {
	switch {
	case t == chroma.KeywordType || t == chroma.NameClass || t == chroma.NameBuiltin:
		return Type
	case t.InCategory(chroma.Keyword):
		return Keyword
	case t == chroma.NameFunction || t == chroma.NameFunctionMagic:
		return Function
	case t.InCategory(chroma.LiteralString):
		return String
	case t.InCategory(chroma.LiteralNumber):
		return Number
	case t.InCategory(chroma.Comment):
		return Comment
	case t.InCategory(chroma.Operator) || t == chroma.Punctuation:
		return Punctuation
	}
	return Plain
}
