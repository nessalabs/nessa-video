//go:build !phonon || !cgo

package phonon

import (
	"context"

	"github.com/nessalabs/nessa-video/internal/voice"
)

// Available reports whether this build can speak with Phonon.
const Available = false

// Voice is unavailable in this build.
type Voice struct{}

// New always fails in this build.
func New(dir, name string, tune map[string]float64, key string) (*Voice, error) {
	return nil, ErrNotBuilt
}

func (*Voice) Key() string { return "" }

func (*Voice) Speak(context.Context, string) (voice.Spoken, error) {
	return voice.Spoken{}, ErrNotBuilt
}

func (*Voice) Close() {}
