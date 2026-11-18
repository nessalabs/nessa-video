//go:build cgo

// Package x264 is the Encoder adapter for libx264. It is linked into the
// binary; see the Makefile for the static release build.
package x264

/*
#cgo darwin CFLAGS: -I/opt/homebrew/include -I/usr/local/include
#cgo darwin LDFLAGS: -L/opt/homebrew/lib -L/usr/local/lib
#cgo !x264static LDFLAGS: -lx264
#cgo LDFLAGS: -lm
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <x264.h>

static x264_t *reel_open(int w, int h, int fps, float crf, const char *preset, int threads) {
	x264_param_t p;
	if (x264_param_default_preset(&p, preset, "animation") < 0) return NULL;
	p.i_width = w;
	p.i_height = h;
	p.i_csp = X264_CSP_I420;
	p.i_fps_num = fps;
	p.i_fps_den = 1;
	p.i_timebase_num = 1;
	p.i_timebase_den = fps;
	p.i_bframe = 0;             // no reordering, so decode order is display order
	p.b_annexb = 0;             // 4-byte length prefixes, as MP4 needs
	p.b_repeat_headers = 0;     // SPS and PPS go in the avcC box
	p.i_keyint_max = fps * 4;
	p.rc.i_rc_method = X264_RC_CRF;
	p.rc.f_rf_constant = crf;
	p.vui.i_colorprim = 1;      // BT.709
	p.vui.i_transfer = 1;
	p.vui.i_colmatrix = 1;
	p.vui.b_fullrange = 0;
	p.i_threads = threads;
	p.i_log_level = X264_LOG_ERROR;
	if (x264_param_apply_profile(&p, "high") < 0) return NULL;
	return x264_encoder_open(&p);
}

// reel_headers copies SPS and PPS (without length prefixes) into the buffers.
static int reel_headers(x264_t *e, uint8_t *sps, int *sps_len, uint8_t *pps, int *pps_len) {
	x264_nal_t *nal; int n;
	if (x264_encoder_headers(e, &nal, &n) < 0) return -1;
	for (int i = 0; i < n; i++) {
		int len = nal[i].i_payload - 4;
		if (len <= 0 || len > 1024) continue;
		if (nal[i].i_type == NAL_SPS) { memcpy(sps, nal[i].p_payload + 4, len); *sps_len = len; }
		if (nal[i].i_type == NAL_PPS) { memcpy(pps, nal[i].p_payload + 4, len); *pps_len = len; }
	}
	return 0;
}

// reel_encode encodes one picture (or flushes when y is NULL). It returns the
// size of the output written to *out, which stays valid until the next call.
static int reel_encode(x264_t *e, x264_picture_t *pic, const uint8_t *y, const uint8_t *u, const uint8_t *v,
                       int w, int h, int64_t pts, uint8_t **out, int *key) {
	x264_picture_t pout; x264_nal_t *nal; int n;
	int size;
	if (y) {
		memcpy(pic->img.plane[0], y, (size_t)w * h);
		memcpy(pic->img.plane[1], u, (size_t)w * h / 4);
		memcpy(pic->img.plane[2], v, (size_t)w * h / 4);
		pic->i_pts = pts;
		size = x264_encoder_encode(e, &nal, &n, pic, &pout);
	} else {
		size = x264_encoder_encode(e, &nal, &n, NULL, &pout);
	}
	if (size <= 0) return size;
	*out = nal[0].p_payload; // payloads are contiguous
	*key = pout.b_keyframe;
	return size;
}

static int reel_delayed(x264_t *e) { return x264_encoder_delayed_frames(e); }
*/
import "C"

import (
	"errors"
	"fmt"
	"io"
	"unsafe"

	"github.com/nessalabs/nessa-video/internal/encode"
	"github.com/nessalabs/nessa-video/internal/encode/mp4"
)

// Encoder writes H.264 in MP4 using libx264.
type Encoder struct {
	enc  *C.x264_t
	pic  C.x264_picture_t
	mux  *mp4.Writer
	w, h int
	pts  int64
}

var _ encode.Encoder = (*Encoder)(nil)

// New opens an encoder that writes an MP4 to out when closed.
func New(out io.Writer, o encode.Options) (*Encoder, error) {
	if o.Width%2 != 0 || o.Height%2 != 0 {
		return nil, errors.New("x264: width and height must be even")
	}
	if o.CRF == 0 {
		o.CRF = 18
	}
	if o.Preset == "" {
		o.Preset = "veryfast"
	}
	preset := C.CString(o.Preset)
	defer C.free(unsafe.Pointer(preset))
	e := C.reel_open(C.int(o.Width), C.int(o.Height), C.int(o.FPS), C.float(o.CRF), preset, 0)
	if e == nil {
		return nil, fmt.Errorf("x264: could not open encoder for %dx%d at %d fps", o.Width, o.Height, o.FPS)
	}
	x := &Encoder{enc: e, w: o.Width, h: o.Height}
	if C.x264_picture_alloc(&x.pic, C.X264_CSP_I420, C.int(o.Width), C.int(o.Height)) < 0 {
		C.x264_encoder_close(e)
		return nil, errors.New("x264: could not allocate a picture")
	}
	var sps, pps [1024]byte
	var spsLen, ppsLen C.int
	if C.reel_headers(e, (*C.uint8_t)(&sps[0]), &spsLen, (*C.uint8_t)(&pps[0]), &ppsLen) < 0 || spsLen == 0 || ppsLen == 0 {
		x.free()
		return nil, errors.New("x264: could not read stream headers")
	}
	x.mux = mp4.New(out, o.Width, o.Height, o.FPS,
		append([]byte(nil), sps[:spsLen]...), append([]byte(nil), pps[:ppsLen]...))
	return x, nil
}

// Write encodes one frame.
func (x *Encoder) Write(f *encode.Frame) error {
	if f.W != x.w || f.H != x.h {
		return fmt.Errorf("x264: frame is %dx%d, encoder is %dx%d", f.W, f.H, x.w, x.h)
	}
	var out *C.uint8_t
	var key C.int
	n := C.reel_encode(x.enc, &x.pic, (*C.uint8_t)(&f.Y[0]), (*C.uint8_t)(&f.U[0]), (*C.uint8_t)(&f.V[0]),
		C.int(f.W), C.int(f.H), C.int64_t(x.pts), &out, &key)
	x.pts++
	return x.take(n, out, key)
}

func (x *Encoder) take(n C.int, out *C.uint8_t, key C.int) error {
	if n < 0 {
		return errors.New("x264: encoding failed")
	}
	if n > 0 {
		x.mux.Add(C.GoBytes(unsafe.Pointer(out), n), key != 0)
	}
	return nil
}

// Close flushes delayed frames and writes the MP4.
func (x *Encoder) Close() error {
	if x.enc == nil {
		return nil
	}
	for C.reel_delayed(x.enc) > 0 {
		var out *C.uint8_t
		var key C.int
		n := C.reel_encode(x.enc, &x.pic, nil, nil, nil, 0, 0, 0, &out, &key)
		if err := x.take(n, out, key); err != nil {
			x.free()
			return err
		}
	}
	x.free()
	return x.mux.Close()
}

func (x *Encoder) free() {
	C.x264_picture_clean(&x.pic)
	C.x264_encoder_close(x.enc)
	x.enc = nil
}
