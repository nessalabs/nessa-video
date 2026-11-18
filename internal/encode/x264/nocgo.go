//go:build !cgo

package x264

import (
	"errors"
	"io"

	"github.com/nessalabs/nessa-video/internal/encode"
)

// New reports that this binary was built without the encoder.
func New(out io.Writer, o encode.Options) (encode.Encoder, error) {
	return nil, errors.New("this reel binary was built without the video encoder (CGO_ENABLED=0); use `make build`")
}
