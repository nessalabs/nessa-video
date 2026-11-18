//go:build !cgo

package aac

import (
	"errors"

	"github.com/nessalabs/nessa-video/internal/encode"
)

// Encode reports that this binary was built without the audio encoder.
func Encode(pcm []int16, rate int) (*encode.Audio, error) {
	return nil, errors.New("this reel binary was built without the audio encoder (CGO_ENABLED=0)")
}
