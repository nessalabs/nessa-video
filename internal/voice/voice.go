// Package voice is the narration context: turning caption text into
// speech. The Voice port hides the engine; adapters call a hosted service
// (ElevenLabs) or, later, a local model. Results are cached on disk so
// re-rendering a script does not pay for the same line twice.
package voice

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Rate is the sample rate narration is mixed at, in Hz.
const Rate = 24000

// Speech is mono 16-bit PCM at Rate.
type Speech []int16

// Seconds returns the length of the speech.
func (s Speech) Seconds() float64 { return float64(len(s)) / Rate }

// Word is one spoken word and when it is said, in seconds from the start.
type Word struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Spoken is speech plus the timing of each word, when the engine knows it.
type Spoken struct {
	Audio Speech
	Words []Word
}

// Voice speaks text. Adapters return word timings when their engine reports
// them and leave Words nil otherwise; the core fills the gap with
// EstimateWords, so every voice gets captions that follow it.
type Voice interface {
	// Key identifies the voice for caching: engine, voice and model.
	Key() string
	Speak(ctx context.Context, text string) (Spoken, error)
}

// EstimateWords spreads words over a length of speech, weighting each by
// its number of letters. The core uses it for voices that do not report
// word timings, so captions can follow any voice.
func EstimateWords(text string, seconds float64) []Word {
	fields := strings.Fields(text)
	if len(fields) == 0 || seconds <= 0 {
		return nil
	}
	weight := func(w string) float64 { return float64(len([]rune(w))) + 2 } // a little time per word for the gap
	total := 0.0
	for _, w := range fields {
		total += weight(w)
	}
	out := make([]Word, len(fields))
	t := 0.0
	for i, w := range fields {
		d := seconds * weight(w) / total
		out[i] = Word{Text: w, Start: t, End: t + d*0.85}
		t += d
	}
	return out
}

// Cached wraps a voice with an on-disk cache keyed by voice and text.
type Cached struct {
	Voice Voice
	Dir   string
}

// Speak returns cached speech or asks the wrapped voice. Audio is stored as
// raw PCM and word timings next to it as JSON.
func (c Cached) Speak(ctx context.Context, text string) (Spoken, error) {
	sum := sha256.Sum256([]byte(c.Voice.Key() + "\x00" + text))
	base := filepath.Join(c.Dir, hex.EncodeToString(sum[:16]))
	if b, err := os.ReadFile(base + ".pcm"); err == nil && len(b)%2 == 0 {
		s := Spoken{Audio: make(Speech, len(b)/2)}
		for i := range s.Audio {
			s.Audio[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
		}
		if j, err := os.ReadFile(base + ".words.json"); err == nil {
			json.Unmarshal(j, &s.Words)
		}
		return s, nil
	}
	s, err := c.Voice.Speak(ctx, text)
	if err != nil {
		return Spoken{}, err
	}
	if err := os.MkdirAll(c.Dir, 0o755); err == nil {
		b := make([]byte, 2*len(s.Audio))
		for i, v := range s.Audio {
			binary.LittleEndian.PutUint16(b[2*i:], uint16(v))
		}
		if j, err := json.Marshal(s.Words); err == nil {
			os.WriteFile(base+".words.json", j, 0o644)
		}
		tmp := base + ".pcm.tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			os.Rename(tmp, base+".pcm")
		}
	}
	return s, nil
}

// Key implements Voice.
func (c Cached) Key() string { return c.Voice.Key() }

// CacheDir is where narration is cached: $XDG_CACHE_HOME/reel/voice or the
// OS user cache directory.
func CacheDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "reel", "voice")
}

// Plain removes inline marks such as ==this== from text before speaking it.
func Plain(s string) string {
	return strings.NewReplacer("==", "", "__", "", "`", "").Replace(s)
}

// Silent is a voice that produces silence as long as reading the text takes.
// It lets scripts be timed and tested without a speech service.
type Silent struct{ WordsPerSecond float64 }

func (Silent) Key() string { return "silent" }

func (v Silent) Speak(_ context.Context, text string) (Spoken, error) {
	wps := v.WordsPerSecond
	if wps <= 0 {
		wps = 2.8
	}
	n := len(strings.Fields(text))
	return Spoken{Audio: make(Speech, int(float64(n)/wps*Rate))}, nil // timings left to the core
}

// ErrNotCached is returned by Offline for a line that has not been spoken yet.
var ErrNotCached = fmt.Errorf("not spoken yet")

// Offline stands in for a voice without contacting it. Wrapped in Cached,
// it answers only from the cache, which lets `reel check` and `reel still`
// use real narration lengths without network access.
type Offline struct{ K string }

func (o Offline) Key() string { return o.K }

func (Offline) Speak(context.Context, string) (Spoken, error) { return Spoken{}, ErrNotCached }

// Engines lists the voice engines scripts can name.
var Engines = []string{"elevenlabs", "silent"}

// ErrNoKey is returned when an engine needs an API key that is not set.
type ErrNoKey struct{ Env string }

func (e ErrNoKey) Error() string {
	return fmt.Sprintf("%s is not set; export it, or render with --mute to skip narration", e.Env)
}
