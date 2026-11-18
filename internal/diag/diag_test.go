package diag

import "testing"

func TestClosest(t *testing.T) {
	cands := []string{"server", "process", "box", "color", "cols"}
	for in, want := range map[string]string{"servr": "server", "procss": "process", "bxo": "box", "zzzzzz": "", "colr": "color"} {
		if got := Closest(in, cands); got != want {
			t.Errorf("Closest(%q) = %q, want %q", in, got, want)
		}
	}
}
