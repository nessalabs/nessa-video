package stage

import (
	"testing"

	c "github.com/nessalabs/nessa-video/internal/composition"
)

func TestSubtitleLinesBreakAtPauses(t *testing.T) {
	var ws []c.SaidWord
	for _, w := range []string{"So", "here's", "how", "it", "worked", "before.", "On", "a", "timer,", "the", "follower", "read", "the", "owner's", "whole", "catalogue", "and", "copied", "whatever", "matched."} {
		ws = append(ws, c.SaidWord{Text: w})
	}
	lines := subtitleLines(ws)
	if len(lines) < 2 || lines[0][len(lines[0])-1].Text != "before." {
		t.Fatalf("first line should end at the full stop: %v", lines)
	}
	for _, l := range lines {
		n := -1
		for _, w := range l {
			n += len(w.Text) + 1
		}
		if n > SubtitleMax {
			t.Errorf("line of %d characters: %v", n, l)
		}
	}
}
