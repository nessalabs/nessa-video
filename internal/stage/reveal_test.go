package stage

import (
	"strings"
	"testing"
)

func TestReveal(t *testing.T) {
	lines := []string{"$ curl", "x-load: ==shared=="}
	// 6 + 14 visible characters; 60% is 12: all of line 1 and "x-load" of line 2
	got := strings.Join(reveal(lines, 0.6), "|")
	if got != "$ curl|x-load" {
		t.Errorf("reveal 60%% = %q", got)
	}
	got = strings.Join(reveal(lines, 0.85), "|") // 17: three letters into the emphasis
	if got != "$ curl|x-load: ==sha==" {
		t.Errorf("reveal 85%% = %q", got)
	}
	if got := reveal(lines, 1); len(got) != 2 || got[1] != lines[1] {
		t.Errorf("full reveal changed text: %q", got)
	}
	if got := reveal(lines, 0); len(got) != 0 {
		t.Errorf("zero reveal = %q", got)
	}
}
