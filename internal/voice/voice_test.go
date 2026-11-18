package voice

import "testing"

func TestEstimateWords(t *testing.T) {
	w := EstimateWords("a client and one server", 5)
	if len(w) != 5 || w[0].Start != 0 || w[4].End > 5 || w[1].Start < w[0].End {
		t.Fatalf("words = %+v", w)
	}
	// longer words get more time
	if w[1].End-w[1].Start <= w[0].End-w[0].Start {
		t.Errorf("client should take longer than a: %+v", w[:2])
	}
	if EstimateWords("", 3) != nil || EstimateWords("x", 0) != nil {
		t.Error("empty input should give nil")
	}
}
