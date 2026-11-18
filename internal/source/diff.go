// Package source reads the code that videos talk about: unified diffs and
// source files, split into numbered lines, plus syntax classes for colour.
package source

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// LineKind says how a line relates to a change.
type LineKind uint8

const (
	Context LineKind = iota
	Added
	Removed
)

// Line is one line of code. Old and New are 1-based line numbers on each
// side of a change; 0 means the line does not exist on that side.
type Line struct {
	Kind     LineKind
	Old, New int
	Text     string
}

// Hunk is one `@@` section of a diff.
type Hunk struct {
	Header string // text after the closing @@, often the enclosing function
	Lines  []Line
}

// FileDiff is the change to one file.
type FileDiff struct {
	Path  string
	Hunks []Hunk
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)

// ParseUnified reads `git diff` or `diff -u` output.
func ParseUnified(text string) ([]FileDiff, error) {
	var files []FileDiff
	var cur *FileDiff
	var h *Hunk
	oldN, newN := 0, 0
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "diff --git "):
			files = append(files, FileDiff{})
			cur, h = &files[len(files)-1], nil
			if parts := strings.Fields(ln); len(parts) >= 4 {
				cur.Path = strings.TrimPrefix(parts[3], "b/")
			}
		case strings.HasPrefix(ln, "+++ "):
			if cur == nil {
				files = append(files, FileDiff{})
				cur = &files[len(files)-1]
			}
			p := strings.TrimSpace(strings.TrimPrefix(ln, "+++ "))
			if p != "/dev/null" {
				cur.Path = strings.TrimPrefix(strings.SplitN(p, "\t", 2)[0], "b/")
			}
		case strings.HasPrefix(ln, "--- "), strings.HasPrefix(ln, "index "),
			strings.HasPrefix(ln, "new file"), strings.HasPrefix(ln, "deleted file"),
			strings.HasPrefix(ln, "similarity"), strings.HasPrefix(ln, "rename "),
			strings.HasPrefix(ln, `\ No newline`):
		case strings.HasPrefix(ln, "@@"):
			m := hunkRe.FindStringSubmatch(ln)
			if m == nil {
				return nil, fmt.Errorf("line %d: bad hunk header %q", i+1, ln)
			}
			if cur == nil {
				files = append(files, FileDiff{})
				cur = &files[len(files)-1]
			}
			cur.Hunks = append(cur.Hunks, Hunk{Header: strings.TrimSpace(m[5])})
			h = &cur.Hunks[len(cur.Hunks)-1]
			oldN, _ = strconv.Atoi(m[1])
			newN, _ = strconv.Atoi(m[3])
		case h != nil && strings.HasPrefix(ln, "+"):
			h.Lines = append(h.Lines, Line{Kind: Added, New: newN, Text: ln[1:]})
			newN++
		case h != nil && strings.HasPrefix(ln, "-"):
			h.Lines = append(h.Lines, Line{Kind: Removed, Old: oldN, Text: ln[1:]})
			oldN++
		case h != nil && (strings.HasPrefix(ln, " ") || ln == "" && i < len(lines)-1):
			t := ""
			if ln != "" {
				t = ln[1:]
			}
			h.Lines = append(h.Lines, Line{Kind: Context, Old: oldN, New: newN, Text: t})
			oldN++
			newN++
		}
	}
	// drop the trailing blank context line a final newline produces
	for fi := range files {
		for hi := range files[fi].Hunks {
			hk := &files[fi].Hunks[hi]
			for len(hk.Lines) > 0 && hk.Lines[len(hk.Lines)-1].Kind == Context && hk.Lines[len(hk.Lines)-1].Text == "" {
				hk.Lines = hk.Lines[:len(hk.Lines)-1]
			}
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no diff found; expected lines starting with @@")
	}
	return files, nil
}

// FileLines numbers the lines of a source file, optionally limited to [from, to].
func FileLines(text string, from, to int) []Line {
	all := strings.Split(strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n"), "\n")
	if from < 1 {
		from = 1
	}
	if to < 1 || to > len(all) {
		to = len(all)
	}
	var out []Line
	for n := from; n <= to; n++ {
		out = append(out, Line{Kind: Context, Old: n, New: n, Text: all[n-1]})
	}
	return out
}

// Range picks lines by number. Side is '+' for new numbers, '-' for old
// numbers, or 0 for either.
type Range struct {
	Side     byte
	From, To int
}

// Selection is a set of ranges, such as "12..14,+20" or "-3".
type Selection []Range

// ParseSelection reads ranges like "12", "12..14", "+12..+14" and "-7",
// separated by commas.
func ParseSelection(s string) (Selection, error) {
	var out Selection
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var r Range
		a, b, isRange := strings.Cut(part, "..")
		side := func(x string) (byte, string) {
			if strings.HasPrefix(x, "+") || strings.HasPrefix(x, "-") {
				return x[0], x[1:]
			}
			return 0, x
		}
		var as, bs string
		r.Side, as = side(a)
		from, err := strconv.Atoi(as)
		if err != nil || from < 1 {
			return nil, fmt.Errorf("bad line number %q", a)
		}
		r.From, r.To = from, from
		if isRange {
			var sb byte
			sb, bs = side(b)
			if sb != r.Side {
				return nil, fmt.Errorf("both ends of %q must use the same side", part)
			}
			to, err := strconv.Atoi(bs)
			if err != nil || to < from {
				return nil, fmt.Errorf("bad range %q", part)
			}
			r.To = to
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty line selection")
	}
	return out, nil
}

// Has reports whether the selection includes l.
func (s Selection) Has(l Line) bool {
	for _, r := range s {
		check := func(n int) bool { return n >= r.From && n <= r.To }
		switch r.Side {
		case '+':
			if l.New > 0 && check(l.New) {
				return true
			}
		case '-':
			if l.Old > 0 && check(l.Old) {
				return true
			}
		default:
			if l.New > 0 && check(l.New) || l.New == 0 && l.Old > 0 && check(l.Old) {
				return true
			}
		}
	}
	return false
}

// ParseCells reads 0-based cell positions like "2", "2..4" or "0,3..5".
func ParseCells(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		a, b, isRange := strings.Cut(part, "..")
		from, err := strconv.Atoi(a)
		if err != nil || from < 0 {
			return nil, fmt.Errorf("bad position %q", a)
		}
		to := from
		if isRange {
			if to, err = strconv.Atoi(b); err != nil || to < from {
				return nil, fmt.Errorf("bad range %q", part)
			}
		}
		for i := from; i <= to; i++ {
			out = append(out, i)
		}
	}
	return out, nil
}
