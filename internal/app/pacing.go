package app

import (
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
	MaxCaptionWords = 8   // a caption is a key phrase, not a transcript
	MaxSceneWords   = 45  // narration per scene: about 15 s, one idea
	MinWPM, MaxWPM  = 130, 210
	MaxCodeRows     = 12 // rows of code on screen before it stops being a cue
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
				ds.Warnf(st.Pos, `keep the caption to the key phrase and put the rest in speak="…"`,
					"caption has %d words; viewers read at most %d while listening", n, MaxCaptionWords)
			}
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
