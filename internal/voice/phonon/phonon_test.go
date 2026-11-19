//go:build phonon && cgo

package phonon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Speaks with the real model when it is in the cache (a narrated render
// fetches it); run with `make test-phonon`.
func TestSpeaksWithCachedModel(t *testing.T) {
	dir := DefaultDir()
	if _, err := os.Stat(filepath.Join(dir, "model.q8.gguf")); err != nil {
		t.Skip("model not downloaded yet")
	}
	v, err := New(dir, "default", nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	s, err := v.Speak(context.Background(), "Your gateway sends its bookmark.")
	if err != nil {
		t.Fatal(err)
	}
	if sec := s.Audio.Seconds(); sec < 0.8 || sec > 6 {
		t.Fatalf("speech lasts %.2fs", sec)
	}
	loud := 0
	for _, x := range s.Audio {
		if x > 2000 || x < -2000 {
			loud++
		}
	}
	if loud < len(s.Audio)/50 {
		t.Fatalf("speech is nearly silent: %d of %d samples", loud, len(s.Audio))
	}
	again, _ := v.Speak(context.Background(), "Your gateway sends its bookmark.")
	if len(again.Audio) != len(s.Audio) {
		t.Errorf("the same line should come out the same: %d vs %d samples", len(again.Audio), len(s.Audio))
	}
}
