// Package elevenlabs is the Voice adapter for the ElevenLabs text-to-speech
// API. The API key comes from the environment, never from scripts.
package elevenlabs

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
func (v *Voice) Key() string { return "elevenlabs/" + v.ID + "/" + v.Model + "/pcm24k" }

// Speak asks the API for raw 24 kHz 16-bit mono PCM.
func (v *Voice) Speak(ctx context.Context, text string) (voice.Speech, error) {
	body, _ := json.Marshal(map[string]string{"text": text, "model_id": v.Model})
	u := fmt.Sprintf("%s/v1/text-to-speech/%s?output_format=pcm_24000", v.Base, url.PathEscape(v.ID))
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("xi-api-key", v.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "audio/pcm")
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
			if len(data)%2 != 0 {
				data = data[:len(data)-1]
			}
			s := make(voice.Speech, len(data)/2)
			for i := range s {
				s[i] = int16(binary.LittleEndian.Uint16(data[2*i:]))
			}
			return s, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			last = fmt.Errorf("elevenlabs: %s", resp.Status)
			continue
		default:
			return nil, fmt.Errorf("elevenlabs: %s: %s", resp.Status, apiMessage(data))
		}
	}
	return nil, last
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
