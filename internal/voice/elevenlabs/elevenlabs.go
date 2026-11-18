// Package elevenlabs is the Voice adapter for the ElevenLabs text-to-speech
// API. The API key comes from the environment, never from scripts.
package elevenlabs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nessalabs/nessa-video/internal/voice"
)

// KeyEnv is the environment variable holding the API key.
const KeyEnv = "ELEVENLABS_API_KEY"

// DefaultModel is used when a script names no model.
const DefaultModel = "eleven_multilingual_v2"

// Voice speaks with one ElevenLabs voice.
type Voice struct {
	ID, Model, key string
	Base           string // API base URL; tests point it at a local server
	Client         *http.Client
}

// New returns a voice for id, reading the key from the environment.
func New(id, model string) (*Voice, error) {
	key := os.Getenv(KeyEnv)
	if key == "" {
		return nil, voice.ErrNoKey{Env: KeyEnv}
	}
	if model == "" {
		model = DefaultModel
	}
	return &Voice{ID: id, Model: model, key: key, Base: "https://api.elevenlabs.io",
		Client: &http.Client{Timeout: 90 * time.Second}}, nil
}

// Key implements voice.Voice.
func (v *Voice) Key() string { return KeyFor(v.ID, v.Model) }

// KeyFor is the cache key for a voice and model, so cached lines can be
// found without an API key.
func KeyFor(id, model string) string {
	if model == "" {
		model = DefaultModel
	}
	return "elevenlabs/" + id + "/" + model + "/pcm24k+words"
}

// Speak asks the API for 24 kHz 16-bit mono PCM with character timings,
// and turns those into word timings.
func (v *Voice) Speak(ctx context.Context, text string) (voice.Spoken, error) {
	body, _ := json.Marshal(map[string]string{"text": text, "model_id": v.Model})
	u := fmt.Sprintf("%s/v1/text-to-speech/%s/with-timestamps?output_format=pcm_24000", v.Base, url.PathEscape(v.ID))
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return voice.Spoken{}, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return voice.Spoken{}, err
		}
		req.Header.Set("xi-api-key", v.key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := v.Client.Do(req)
		if err != nil {
			last = err
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return decode(data)
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			last = fmt.Errorf("elevenlabs: %s", resp.Status)
			continue
		default:
			return voice.Spoken{}, fmt.Errorf("elevenlabs: %s: %s", resp.Status, apiMessage(data))
		}
	}
	return voice.Spoken{}, last
}

type alignment struct {
	Characters []string  `json:"characters"`
	Starts     []float64 `json:"character_start_times_seconds"`
	Ends       []float64 `json:"character_end_times_seconds"`
}

// decode reads a with-timestamps response.
func decode(data []byte) (voice.Spoken, error) {
	var r struct {
		Audio     string     `json:"audio_base64"`
		Alignment *alignment `json:"alignment"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return voice.Spoken{}, fmt.Errorf("elevenlabs: unexpected response: %w", err)
	}
	pcm, err := base64.StdEncoding.DecodeString(r.Audio)
	if err != nil {
		return voice.Spoken{}, fmt.Errorf("elevenlabs: bad audio: %w", err)
	}
	if len(pcm)%2 != 0 {
		pcm = pcm[:len(pcm)-1]
	}
	s := voice.Spoken{Audio: make(voice.Speech, len(pcm)/2)}
	for i := range s.Audio {
		s.Audio[i] = int16(binary.LittleEndian.Uint16(pcm[2*i:]))
	}
	if a := r.Alignment; a != nil {
		s.Words = wordsFromChars(a.Characters, a.Starts, a.Ends)
	}
	return s, nil
}

// apiMessage pulls the readable message out of an error body.
func apiMessage(b []byte) string {
	var e struct {
		Detail any `json:"detail"`
	}
	if json.Unmarshal(b, &e) == nil && e.Detail != nil {
		if m, ok := e.Detail.(map[string]any); ok {
			if msg, ok := m["message"].(string); ok {
				return msg
			}
		}
		if s, ok := e.Detail.(string); ok {
			return s
		}
	}
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}

// wordsFromChars groups ElevenLabs' per-character alignment into words,
// splitting on whitespace. It is specific to this API's response shape.
func wordsFromChars(chars []string, start, end []float64) []voice.Word {
	var out []voice.Word
	var cur strings.Builder
	var ws, we float64
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, voice.Word{Text: cur.String(), Start: ws, End: we})
			cur.Reset()
		}
	}
	for i, ch := range chars {
		if i >= len(start) || i >= len(end) {
			break
		}
		if strings.TrimSpace(ch) == "" {
			flush()
			continue
		}
		if cur.Len() == 0 {
			ws = start[i]
		}
		cur.WriteString(ch)
		we = end[i]
	}
	flush()
	return out
}

// Spec is the voice a script asks for: `voice ENGINE ID model=…`.
type Spec struct {
	Engine string
	ID     string
	Model  string
}
