package elevenlabs

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nessalabs/nessa-video/internal/voice"
)

func TestSpeak(t *testing.T) {
	var gotKey, gotPath, gotQuery, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotPath, gotQuery = r.Header.Get("xi-api-key"), r.URL.Path, r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		pcm := make([]byte, 2*2400)
		for i := 0; i < 2400; i++ {
			binary.LittleEndian.PutUint16(pcm[2*i:], uint16(int16(i)))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"audio_base64": base64.StdEncoding.EncodeToString(pcm),
			"alignment": map[string]any{
				"characters":                    []string{"h", "i", " ", "y", "o"},
				"character_start_times_seconds": []float64{0, 0.01, 0.02, 0.05, 0.06},
				"character_end_times_seconds":   []float64{0.01, 0.02, 0.05, 0.06, 0.09},
			},
		})
	}))
	defer srv.Close()
	t.Setenv(KeyEnv, "test-key")
	v, err := New("voice123", "")
	if err != nil {
		t.Fatal(err)
	}
	v.Base = srv.URL
	s, err := v.Speak(context.Background(), "hello there")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Audio) != 2400 || s.Audio[100] != 100 || s.Audio.Seconds() != 0.1 {
		t.Fatalf("speech: %d samples, s[100]=%d", len(s.Audio), s.Audio[100])
	}
	if len(s.Words) != 2 || s.Words[0].Text != "hi" || s.Words[1].Text != "yo" || s.Words[1].Start != 0.05 || s.Words[1].End != 0.09 {
		t.Fatalf("words = %+v", s.Words)
	}
	if gotKey != "test-key" || gotPath != "/v1/text-to-speech/voice123/with-timestamps" || gotQuery != "output_format=pcm_24000" ||
		!strings.Contains(gotBody, `"text":"hello there"`) || !strings.Contains(gotBody, DefaultModel) {
		t.Fatalf("request: key=%q path=%q query=%q body=%s", gotKey, gotPath, gotQuery, gotBody)
	}
}

func TestErrors(t *testing.T) {
	t.Setenv(KeyEnv, "")
	if _, err := New("v", ""); err == nil || !strings.Contains(err.Error(), "--mute") {
		t.Fatalf("missing key error = %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"detail":{"status":"invalid_api_key","message":"Invalid API key"}}`))
	}))
	defer srv.Close()
	t.Setenv(KeyEnv, "bad")
	v, _ := New("v", "")
	v.Base = srv.URL
	if _, err := v.Speak(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "Invalid API key") {
		t.Fatalf("error = %v", err)
	}
}

func TestCache(t *testing.T) {
	calls := 0
	inner := fakeVoice{fn: func() { calls++ }}
	c := voice.Cached{Voice: inner, Dir: t.TempDir()}
	for i := 0; i < 3; i++ {
		s, err := c.Speak(context.Background(), "same line")
		if err != nil || len(s.Audio) != 3 || s.Audio[2] != -7 || len(s.Words) != 1 || s.Words[0].End != 0.5 {
			t.Fatalf("speak: %+v %v", s, err)
		}
	}
	if calls != 1 {
		t.Fatalf("voice called %d times, want 1", calls)
	}
}

type fakeVoice struct{ fn func() }

func (fakeVoice) Key() string { return "fake" }
func (f fakeVoice) Speak(context.Context, string) (voice.Spoken, error) {
	f.fn()
	return voice.Spoken{Audio: voice.Speech{1, 2, -7}, Words: []voice.Word{{Text: "same", Start: 0, End: 0.5}}}, nil
}
