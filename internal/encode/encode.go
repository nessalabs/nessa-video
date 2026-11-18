// Package encode is the encoding context: it turns frames into a video file.
// The Encoder port hides the codec; the x264 adapter implements it.
package encode

import (
	"image"
	"sync"
)

// Options configure an encoder.
type Options struct {
	Width, Height int
	FPS           int
	CRF           float64 // quality; lower is better. 0 means the default (18)
	Preset        string  // x264 speed preset; "" means veryfast
}

// Encoder accepts frames in order and writes a video when closed.
type Encoder interface {
	// Write encodes one frame in I420 (see ToI420).
	Write(f *Frame) error
	// Close flushes delayed frames and finishes the file.
	Close() error
}

// Frame is one picture in 8-bit I420 (YUV 4:2:0, BT.709 limited range).
type Frame struct {
	W, H    int
	Y, U, V []byte
}

var framePool sync.Pool

// NewFrame returns a frame buffer for the size, reusing freed ones.
func NewFrame(w, h int) *Frame {
	if f, ok := framePool.Get().(*Frame); ok && f.W == w && f.H == h {
		return f
	}
	return &Frame{W: w, H: h, Y: make([]byte, w*h), U: make([]byte, w*h/4), V: make([]byte, w*h/4)}
}

// Release returns a frame to the pool once the encoder has copied it.
func Release(f *Frame) { framePool.Put(f) }

// ToI420 converts an RGBA image with even width and height into f using
// BT.709 limited-range coefficients, averaging chroma over 2x2 blocks.
func ToI420(img *image.RGBA, f *Frame) {
	w, h := f.W, f.H
	pix, stride := img.Pix, img.Stride
	for y := 0; y < h; y += 2 {
		r0 := pix[y*stride:]
		r1 := pix[(y+1)*stride:]
		y0 := f.Y[y*w:]
		y1 := f.Y[(y+1)*w:]
		cu := f.U[(y/2)*(w/2):]
		cv := f.V[(y/2)*(w/2):]
		for x := 0; x < w; x += 2 {
			var sr, sg, sb int
			for i, row := range [2][]byte{r0, r1} {
				for dx := 0; dx < 2; dx++ {
					o := (x + dx) * 4
					r, g, b := int(row[o]), int(row[o+1]), int(row[o+2])
					yy := (47*r + 157*g + 16*b + 128) >> 8
					if i == 0 {
						y0[x+dx] = byte(16 + yy)
					} else {
						y1[x+dx] = byte(16 + yy)
					}
					sr += r
					sg += g
					sb += b
				}
			}
			sr, sg, sb = sr/4, sg/4, sb/4
			cu[x/2] = byte(128 + ((-26*sr - 87*sg + 112*sb + 128) >> 8))
			cv[x/2] = byte(128 + ((112*sr - 102*sg - 10*sb + 128) >> 8))
		}
	}
}

// Audio is an encoded audio track: raw AAC frames plus the decoder config.
type Audio struct {
	Rate, Channels  int
	Config          []byte // AudioSpecificConfig
	Frames          [][]byte
	SamplesPerFrame int
}

// AudioSink is implemented by encoders that can add an audio track. It must
// be called before Close.
type AudioSink interface {
	SetAudio(a *Audio)
}
