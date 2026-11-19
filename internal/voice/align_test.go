package voice

import (
	"math"
	"math/rand"
	"testing"
)

// speech of noise bursts separated by silences, in seconds
func bursts(parts ...float64) Speech {
	var s Speech
	r := rand.New(rand.NewSource(1))
	for i, d := range parts {
		n := int(d * Rate)
		for k := 0; k < n; k++ {
			if i%2 == 1 { // odd parts are speech
				s = append(s, int16(r.Intn(16000)-8000))
			} else {
				s = append(s, int16(r.Intn(40)-20))
			}
		}
	}
	return s
}

func TestWordsFollowThePauses(t *testing.T) {
	// 0.4 s of silence, a phrase, a pause, a longer phrase, silence
	pcm := bursts(0.4, 1.0, 0.35, 1.6, 0.5)
	words := EstimateWordsFromAudio("One two three, four five six seven.", pcm)
	if len(words) != 7 {
		t.Fatalf("%d words", len(words))
	}
	near := func(got, want float64) bool { return math.Abs(got-want) < 0.03 }
	if !near(words[0].Start, 0.4) {
		t.Errorf("first word at %.2f; speech starts at 0.40", words[0].Start)
	}
	if !near(words[3].Start, 1.75) {
		t.Errorf("\"four\" at %.2f; the second phrase starts at 1.75", words[3].Start)
	}
	if words[6].End > 3.35+0.03 {
		t.Errorf("last word ends at %.2f, after the speech ends at 3.35", words[6].End)
	}
	// a comma the speaker ran through gets no pause of its own
	run := EstimateWordsFromAudio("So, in short: one two three four.", bursts(0, 0.8, 0.3, 1.4, 0.2))
	if !near(run[3].Start, 1.1) {
		t.Errorf("after the only pause, \"one\" should start at 1.10, got %.2f", run[3].Start)
	}
}
