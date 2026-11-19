package app

import (
	"regexp"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/diag"
	"github.com/nessalabs/nessa-video/internal/voice"
)

// Pacing limits for narrated explainers. They come from research on
// learning from narrated diagrams (segmenting, redundancy, transient
// information, speech rate); skills/reel/references/explaining.md cites it
// and docs/state/check-a-script.md describes the warnings.
const (
	MaxCaptionWords = 8  // a caption is a key phrase, not a transcript
	MaxSceneWords   = 45 // narration per scene: about 15 s, one idea
	MinWPM, MaxWPM  = 130, 210
	MaxCodeRows     = 12 // rows of code on screen before it stops being a cue
	MaxSentence     = 25 // spoken words before a listener has to hold too much
)

// lintPacing warns where a script works against how people take in a
// narrated diagram. Warnings never stop a render.
func lintPacing(v *c.Video, ds *diag.List) {
	for _, sc := range v.Scenes {
		words := 0
		for _, st := range sc.Steps {
			if st.Verb != "say" {
				continue
			}
			if n := len(strings.Fields(voice.Plain(st.Text))); n > MaxCaptionWords {
				ds.Warnf(st.Pos, `keep the heading to the key phrase and put the rest in speak="…"`,
					"heading has %d words; viewers read at most %d while listening", n, MaxCaptionWords)
			}
			lintSpeech(st, ds)
			spoken := len(strings.Fields(voice.Plain(st.Spoken())))
			words += spoken
			if st.Speech > 0 && spoken >= 12 { // short lines time too noisily
				if wpm := float64(spoken) / st.Speech * 60; wpm < MinWPM || wpm > MaxWPM {
					ds.Warnf(st.Pos, "choose another voice or model, or reword the line",
						"narration runs at %.0f words a minute; %d to %d is easiest to follow", wpm, MinWPM, MaxWPM)
				}
			}
		}
		if words > MaxSceneWords {
			ds.Warnf(sc.Pos, "split the scene: one idea, then a new scene for the next",
				"scene %s narrates %d words; past about %d, listeners lose the start", sc.Name, words, MaxSceneWords)
		}
		sc.Root.Walk(func(e *c.Element) {
			if e.Code != nil && len(e.Code.Lines) > MaxCodeRows && !e.Carried {
				ds.Warnf(e.Pos, "show code only when the exact text is the point; draw the idea as a diagram, or narrow focus= and context=",
					"%d rows of code on screen; more than %d stop being a cue", len(e.Code.Lines), MaxCodeRows)
			}
		})
	}
}

// codeName matches words that are names from code rather than speech:
// camelCase, snake_case and dotted.names.
var codeName = regexp.MustCompile(`[a-z][A-Z]|[A-Za-z]_[A-Za-z]|[a-z]\.[a-z]`)

// lintSpeech warns about narration that is hard to follow by ear: names
// read out of the code, and sentences too long to hold in mind.
func lintSpeech(st c.Step, ds *diag.List) {
	text := voice.Plain(st.Spoken())
	for _, w := range strings.Fields(text) {
		if codeName.MatchString(strings.Trim(w, ".,;:!?\"'()")) {
			ds.Warnf(st.Pos, "say what it does in plain words; show the code name on screen once if it matters",
				"narration reads out the code name %q", strings.Trim(w, ".,;:!?\"'()"))
			break
		}
	}
	for _, sent := range regexp.MustCompile(`[.?!]+\s+`).Split(text, -1) {
		if n := len(strings.Fields(sent)); n > MaxSentence {
			ds.Warnf(st.Pos, "split it: one idea per sentence, subject first",
				"a spoken sentence of %d words; past %d, listeners lose its start", n, MaxSentence)
			break
		}
	}
}
