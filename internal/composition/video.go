package composition

import (
	"time"

	"github.com/nessalabs/nessa-video/internal/diag"
	"github.com/nessalabs/nessa-video/internal/selector"
)

// Video is the aggregate root: one script, its scenes and its style rules.
type Video struct {
	Title         string
	Width, Height int
	FPS           int
	Theme         string
	Tokens        map[string]string // theme tokens after extends and script overrides
	Rules         []*Rule           // theme and script rules, in cascade order
	Scenes        []*Scene
	Voice         *VoiceSpec // narration voice; nil means captions only
}

// VoiceSpec is the narration voice a script asks for: `voice ENGINE ID model=…`.
type VoiceSpec struct {
	Engine, ID, Model string
}

// Scene is one shot. Root holds what is on screen, including elements
// carried over from the previous scene. Steps say what happens, in order.
type Scene struct {
	Name     string
	Duration time.Duration // 0 means automatic
	Root     *Element
	Steps    []Step
	Pos      diag.Pos
}

// Step is one thing that happens during a scene: a state change, an exit,
// a camera move, a caption. The motion context gives steps their timing.
type Step struct {
	Verb   string
	Target *selector.Selector // nil for steps without a target, like wait or say
	Args   []string
	Text   string
	Decls  []Decl
	With   bool // starts together with the previous step
	Pos    diag.Pos

	// Speech is the narration length in seconds for a say step, filled in
	// once the voice has spoken it; 0 means it is estimated from the words.
	Speech float64
	// Words times each word of the caption as it is spoken, by caption word
	// index; nil when the spoken words differ from the caption.
	Words []WordTime
}

// WordTime is when one caption word is spoken, in seconds from the line's start.
type WordTime struct{ Start, End float64 }

// Spoken returns what a say step reads aloud: its speak= text, or its caption.
func (s Step) Spoken() string {
	if v, ok := s.Lookup("speak"); ok {
		return v
	}
	return s.Text
}

// Lookup returns a step declaration value.
func (s Step) Lookup(key string) (string, bool) {
	for i := len(s.Decls) - 1; i >= 0; i-- {
		if s.Decls[i].Key == key {
			return s.Decls[i].Value, true
		}
	}
	return "", false
}

// Style applies the cascade to every scene.
func (v *Video) Style() {
	for _, s := range v.Scenes {
		Cascade(s.Root, v.Rules)
	}
}
