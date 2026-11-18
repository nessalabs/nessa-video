//go:build cgo

// Package aac encodes narration to AAC-LC for the MP4 audio track, using
// the VisualOn encoder compiled into the binary.
package aac

import (
	"bytes"
	"errors"
	"sync"

	vo "github.com/gen2brain/aac-go"

	"github.com/nessalabs/nessa-video/internal/encode"
)

var mu sync.Mutex // the VisualOn encoder keeps global state

// Encode turns mono 16-bit PCM into raw AAC frames of 1024 samples each.
func Encode(pcm []int16, rate int) (*encode.Audio, error) {
	idx, ok := rateIndex[rate]
	if !ok {
		return nil, errors.New("aac: unsupported sample rate")
	}
	mu.Lock()
	defer mu.Unlock()
	var in bytes.Buffer
	in.Grow(len(pcm) * 2)
	for _, s := range pcm {
		in.WriteByte(byte(s))
		in.WriteByte(byte(uint16(s) >> 8))
	}
	var out bytes.Buffer
	enc, err := vo.NewEncoder(&out, &vo.Options{SampleRate: rate, NumChannels: 1, BitRate: 96000})
	if err != nil {
		return nil, err
	}
	if err := enc.Encode(&in); err != nil {
		enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	frames, err := splitADTS(out.Bytes())
	if err != nil {
		return nil, err
	}
	// AudioSpecificConfig: AAC LC (2), sample rate index, mono
	asc := []byte{byte(2<<3 | idx>>1), byte((idx&1)<<7 | 1<<3)}
	return &encode.Audio{Rate: rate, Channels: 1, Config: asc, Frames: frames, SamplesPerFrame: 1024}, nil
}

var rateIndex = map[int]int{96000: 0, 88200: 1, 64000: 2, 48000: 3, 44100: 4, 32000: 5, 24000: 6, 22050: 7, 16000: 8, 12000: 9, 11025: 10, 8000: 11}

// splitADTS strips ADTS headers and returns the raw frames.
func splitADTS(b []byte) ([][]byte, error) {
	var frames [][]byte
	for len(b) >= 7 {
		if b[0] != 0xFF || b[1]&0xF0 != 0xF0 {
			return nil, errors.New("aac: lost ADTS sync")
		}
		protAbsent := b[1]&1 == 1
		size := int(b[3]&3)<<11 | int(b[4])<<3 | int(b[5])>>5
		head := 7
		if !protAbsent {
			head = 9
		}
		if size < head || size > len(b) {
			return nil, errors.New("aac: bad ADTS frame length")
		}
		frames = append(frames, append([]byte(nil), b[head:size]...))
		b = b[size:]
	}
	return frames, nil
}
