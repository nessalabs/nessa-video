// Package diag is the shared kernel for source positions and diagnostics.
// Every context reports problems in this one shape so that `reel check`
// can print them, and agents can act on them, without translation.
package diag

import (
	"fmt"
	"sort"
)

// Pos is a 1-based position. File is empty for the script being checked
// and names the source otherwise, such as a theme pack.
type Pos struct {
	Line int    `json:"line"`
	Col  int    `json:"col"`
	File string `json:"file,omitempty"`
}

func (p Pos) String() string {
	if p.File != "" {
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Col)
}

// Severity says whether a diagnostic blocks rendering.
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// Diag is one problem, with a hint written for an agent to act on.
type Diag struct {
	Pos      Pos      `json:"pos"`
	Severity Severity `json:"severity"`
	Msg      string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`
}

func (d Diag) Error() string {
	sev := d.Severity
	if sev == "" {
		sev = Error
	}
	if d.Hint != "" {
		return fmt.Sprintf("%s: %s: %s (hint: %s)", d.Pos, sev, d.Msg, d.Hint)
	}
	return fmt.Sprintf("%s: %s: %s", d.Pos, sev, d.Msg)
}

// List collects diagnostics.
type List []Diag

func (l *List) Errorf(p Pos, hint, f string, a ...any) {
	*l = append(*l, Diag{p, Error, fmt.Sprintf(f, a...), hint})
}
func (l *List) Warnf(p Pos, hint, f string, a ...any) {
	*l = append(*l, Diag{p, Warning, fmt.Sprintf(f, a...), hint})
}

// HasErrors reports whether any diagnostic is an error.
func (l List) HasErrors() bool {
	for _, d := range l {
		if d.Severity != Warning {
			return true
		}
	}
	return false
}

// Sorted returns the list ordered by position, without exact duplicates.
func (l List) Sorted() List {
	var out List
	seen := map[Diag]bool{}
	for _, d := range l {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pos.Line != out[j].Pos.Line {
			return out[i].Pos.Line < out[j].Pos.Line
		}
		return out[i].Pos.Col < out[j].Pos.Col
	})
	return out
}

// Closest returns the candidate nearest to word by edit distance, or "" if
// nothing is close enough to be a plausible typo.
func Closest(word string, candidates []string) string {
	best, bestD, bestP := "", max(1, len(word)/3)+1, -1
	for _, c := range candidates {
		d := lev(word, c)
		p := prefix(word, c)
		if d < bestD || d == bestD && p > bestP {
			best, bestD, bestP = c, d, p
		}
	}
	return best
}

func lev(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func prefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}
