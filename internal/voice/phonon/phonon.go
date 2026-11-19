//go:build phonon && cgo

package phonon

/*
#cgo CFLAGS: -I${SRCDIR}/native
// The static library itself comes in through CGO_LDFLAGS (see the Makefile):
// Go refuses #cgo flags naming archives with LLVM bitcode, which Rust's
// standard library carries.
#cgo LDFLAGS: -lm
#cgo darwin LDFLAGS: -liconv
#cgo linux LDFLAGS: -ldl -lpthread
#include <stdlib.h>
#include "phonon.h"
extern bool reelPhononAudio(float *pcm, size_t n, void *user);
*/
import "C"

import (
	"context"
	"errors"
	"hash/fnv"
	"math"
	"runtime/cgo"
	"sync"
	"unsafe"

	"github.com/nessalabs/nessa-video/internal/voice"
)

// Available reports whether this build can speak with Phonon.
const Available = true

// Voice speaks with one Phonon voice. The runtime takes one call at a time.
type Voice struct {
	mu   sync.Mutex
	h    *C.ReelPhonon
	name string
	temp float32
	key  string
}

// New loads the checkpoint in dir and picks voice (already in dir).
func New(dir, name string, tune map[string]float64, key string) (*Voice, error) {
	cdir, clang := C.CString(dir), C.CString("en")
	defer C.free(unsafe.Pointer(cdir))
	defer C.free(unsafe.Pointer(clang))
	h := C.reel_phonon_new(cdir, clang)
	if h == nil {
		return nil, errors.New("phonon: " + C.GoString(C.reel_phonon_error(nil)))
	}
	temp := float32(-1)
	if t, ok := tune["temperature"]; ok {
		temp = float32(t)
	}
	v := &Voice{h: h, name: name, temp: temp, key: key}
	if name == "default" {
		v.name = ""
	}
	return v, nil
}

// Key implements voice.Voice.
func (v *Voice) Key() string { return v.key }

type sink struct{ pcm voice.Speech }

//export reelPhononAudio
func reelPhononAudio(pcm *C.float, n C.size_t, user unsafe.Pointer) C.bool {
	s := (*(*cgo.Handle)(user)).Value().(*sink)
	for _, f := range unsafe.Slice((*float32)(unsafe.Pointer(pcm)), int(n)) {
		s.pcm = append(s.pcm, int16(math.Max(-1, math.Min(1, float64(f)))*32767))
	}
	return true
}

// Speak implements voice.Voice. Phonon reports no word timings, so the
// core estimates them from the audio's length.
func (v *Voice) Speak(ctx context.Context, text string) (voice.Spoken, error) {
	if err := ctx.Err(); err != nil {
		return voice.Spoken{}, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	s := &sink{}
	hd := cgo.NewHandle(s)
	defer hd.Delete()
	// the handle travels through C as a pointer to C memory holding it
	user := C.malloc(C.size_t(unsafe.Sizeof(hd)))
	defer C.free(user)
	*(*cgo.Handle)(user) = hd
	ct, cv := C.CString(text), C.CString(v.name)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cv))
	seed := fnv.New64a()
	seed.Write([]byte(text)) // the same line always comes out the same
	ok := C.reel_phonon_speak(v.h, ct, cv, C.float(v.temp), C.uint64_t(seed.Sum64()),
		C.ReelPhononAudio(C.reelPhononAudio), user)
	if !ok {
		return voice.Spoken{}, errors.New("phonon: " + C.GoString(C.reel_phonon_error(v.h)))
	}
	return voice.Spoken{Audio: s.pcm}, nil
}

// Close frees the model.
func (v *Voice) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.h != nil {
		C.reel_phonon_free(v.h)
		v.h = nil
	}
}
