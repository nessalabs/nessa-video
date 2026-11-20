// Package markup reads the inline marks reel allows in any text:
// ==bright==, __underlined__ and {tone:coloured words}. A tone is one of
// bad, good, warn or info (red, green, amber and blue are other names for
// them), and the theme sets its colour. Marks can sit inside each other;
// a tone's closing brace ends the tone, and a brace that closes nothing is
// ordinary text.
package markup

import "strings"

// Tones are the tone names a script may use, mapped to the tone they mean.
var Tones = map[string]string{
	"bad": "bad", "red": "bad",
	"good": "good", "green": "good",
	"warn": "warn", "amber": "warn", "yellow": "warn",
	"info": "info", "blue": "info",
}

// Span is a piece of text with one set of marks.
type Span struct {
	Text  string
	Hi    bool   // ==bright==
	Under bool   // __underlined__
	Tone  string // "" or a tone from Tones' values
}

// mark reads a mark starting at r[i]: its kind ("==", "__", "{" with the
// tone, or "}") and length. tone says whether a tone is open, since only
// then is "}" a mark.
func mark(r []rune, i int, tone bool) (kind, name string, n int) {
	if i+1 < len(r) && (r[i] == '=' && r[i+1] == '=' || r[i] == '_' && r[i+1] == '_') {
		return string(r[i : i+2]), "", 2
	}
	if r[i] == '}' && tone {
		return "}", "", 1
	}
	if r[i] == '{' {
		for j := i + 1; j < len(r) && j < i+9; j++ {
			if r[j] == ':' {
				if t, ok := Tones[strings.ToLower(string(r[i+1:j]))]; ok {
					return "{", t, j - i + 1
				}
				break
			}
		}
	}
	return "", "", 0
}

// Parse splits s into spans.
func Parse(s string) []Span {
	var out []Span
	var cur Span
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			cur.Text = b.String()
			out = append(out, cur)
			b.Reset()
		}
	}
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		kind, name, n := mark(r, i, cur.Tone != "")
		if n == 0 {
			b.WriteRune(r[i])
			continue
		}
		flush()
		switch kind {
		case "==":
			cur.Hi = !cur.Hi
		case "__":
			cur.Under = !cur.Under
		case "{":
			cur.Tone = name
		case "}":
			cur.Tone = ""
		}
		i += n - 1
	}
	flush()
	return out
}

// Strip returns s without marks.
func Strip(s string) string {
	if !strings.ContainsAny(s, "=_{}") {
		return s
	}
	var b strings.Builder
	for _, sp := range Parse(s) {
		b.WriteString(sp.Text)
	}
	return b.String()
}

// state is the marks open at some point in a text.
type state struct {
	hi, under bool
	tone      string
}

func (st state) open() string {
	s := ""
	if st.tone != "" {
		s += "{" + st.tone + ":"
	}
	if st.hi {
		s += "=="
	}
	if st.under {
		s += "__"
	}
	return s
}

func (st state) close() string {
	s := ""
	if st.under {
		s += "__"
	}
	if st.hi {
		s += "=="
	}
	if st.tone != "" {
		s += "}"
	}
	return s
}

// Cut returns the first n visible characters of s with its marks, every
// mark left open closed, and how many characters it used.
func Cut(s string, n int) (string, int) {
	var b strings.Builder
	var st state
	used := 0
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		kind, name, k := mark(r, i, st.tone != "")
		if k > 0 {
			switch kind {
			case "==":
				st.hi = !st.hi
			case "__":
				st.under = !st.under
			case "{":
				st.tone = name
			case "}":
				st.tone = ""
			}
			b.WriteString(string(r[i : i+k]))
			i += k - 1
			continue
		}
		if used >= n {
			break
		}
		b.WriteRune(r[i])
		used++
	}
	return b.String() + st.close(), used
}

// Balance makes each line stand alone: a mark open at the end of one line
// is closed there and opened again at the start of the next.
func Balance(lines []string) []string {
	out := make([]string, len(lines))
	var st state
	for i, l := range lines {
		start := st.open()
		r := []rune(l)
		for j := 0; j < len(r); j++ {
			kind, name, k := mark(r, j, st.tone != "")
			switch kind {
			case "==":
				st.hi = !st.hi
			case "__":
				st.under = !st.under
			case "{":
				st.tone = name
			case "}":
				st.tone = ""
			}
			if k > 0 {
				j += k - 1
			}
		}
		out[i] = start + l + st.close()
	}
	return out
}

// WordTones lists the tone of each word of s, split on spaces after marks
// are removed; a word with no tone has "".
func WordTones(s string) []string {
	var out []string
	inWord := false
	for _, sp := range Parse(s) {
		for _, r := range sp.Text {
			if r == ' ' || r == '\t' || r == '\n' {
				inWord = false
				continue
			}
			if !inWord {
				out = append(out, sp.Tone)
				inWord = true
			} else if sp.Tone != "" && out[len(out)-1] == "" {
				out[len(out)-1] = sp.Tone
			}
		}
	}
	return out
}
