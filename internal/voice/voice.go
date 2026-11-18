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

// Voice speaks text.
type Voice interface {
	// Key identifies the voice for caching: engine, voice and model.
	Key() string
	Speak(ctx context.Context, text string) (Speech, error)
}

// Spec is the voice a script asks for: `voice ENGINE ID model=…`.
type Spec struct {
	Engine string
	ID     string
	Model  string
}

// Cached wraps a voice with an on-disk cache keyed by voice and text.
type Cached struct {
	Voice Voice
	Dir   string
}

// Speak returns cached speech or asks the wrapped voice.
func (c Cached) Speak(ctx context.Context, text string) (Speech, error) {
	sum := sha256.Sum256([]byte(c.Voice.Key() + "\x00" + text))
	path := filepath.Join(c.Dir, hex.EncodeToString(sum[:16])+".pcm")
	if b, err := os.ReadFile(path); err == nil && len(b)%2 == 0 {
		s := make(Speech, len(b)/2)
		for i := range s {
			s[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
		}
		return s, nil
	}
	s, err := c.Voice.Speak(ctx, text)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.Dir, 0o755); err == nil {
		b := make([]byte, 2*len(s))
		for i, v := range s {
			binary.LittleEndian.PutUint16(b[2*i:], uint16(v))
		}
		tmp := path + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			os.Rename(tmp, path)
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

func (v Silent) Speak(_ context.Context, text string) (Speech, error) {
	wps := v.WordsPerSecond
	if wps <= 0 {
		wps = 2.8
	}
	words := len(strings.Fields(text))
	return make(Speech, int(float64(words)/wps*Rate)), nil
}

// Engines lists the voice engines scripts can name.
var Engines = []string{"elevenlabs", "silent"}

// ErrNoKey is returned when an engine needs an API key that is not set.
type ErrNoKey struct{ Env string }

func (e ErrNoKey) Error() string {
	return fmt.Sprintf("%s is not set; export it, or render with --mute to skip narration", e.Env)
}
