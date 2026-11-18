//go:build resvg

// Package resvg is a benchmark-only Rasterizer adapter that serializes the
// display list to SVG and draws it with resvg (Rust) through cgo.
package resvg

/*
#cgo LDFLAGS: -lresvgshim -framework CoreFoundation -framework CoreText -framework CoreGraphics
#include <stdint.h>
#include <stdlib.h>
int rv_init(const char* dir);
int rv_render(const uint8_t* svg, size_t len, uint32_t w, uint32_t h, uint8_t* out);
*/
import "C"

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/tdewolff/canvas"

	"reel/internal/render"
)

// Resvg serializes the display list to SVG and rasterizes it with resvg (Rust) via cgo.
type Resvg struct{ lengths sync.Map }

func New(fontDir string) (*Resvg, error) {
	cs := C.CString(fontDir)
	defer C.free(unsafe.Pointer(cs))
	if n := C.rv_init(cs); n == 0 {
		return nil, fmt.Errorf("resvg: no fonts loaded from %s", fontDir)
	}
	return &Resvg{}, nil
}

func (r *Resvg) Name() string { return "resvg (Rust, via cgo)" }

func (r *Resvg) length(d string) float64 {
	if v, ok := r.lengths.Load(d); ok {
		return v.(float64)
	}
	p, _ := canvas.ParseSVGPath(d)
	L := p.Length()
	r.lengths.Store(d, L)
	return L
}

func rgb(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
func op(c color.NRGBA) string  { return strconv.FormatFloat(float64(c.A)/255, 'f', 3, 64) }
func f(v float64) string       { return strconv.FormatFloat(v, 'f', -1, 64) }

var fam = map[string][2]string{
	"sans400": {"Inter", "400"}, "sans500": {"Inter", "500"}, "sans600": {"Inter", "600"},
	"mono": {"JetBrains Mono", "400"},
}

// SVG converts a display list to an SVG document.
func (r *Resvg) SVG(l *render.List) []byte {
	var b strings.Builder
	b.Grow(16 << 10)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d"><rect width="100%%" height="100%%" fill="%s"/>`, l.W, l.H, rgb(l.BG))
	for i := range l.Ops {
		o := &l.Ops[i]
		m := o.M
		tr := fmt.Sprintf(`matrix(%s %s %s %s %s %s)`, f(m[0]), f(m[1]), f(m[2]), f(m[3]), f(m[4]), f(m[5]))
		switch o.Kind {
		case render.PathOp:
			fmt.Fprintf(&b, `<path transform="%s" d="%s"`, tr, o.D)
			if o.Fill.A > 0 {
				fmt.Fprintf(&b, ` fill="%s" fill-opacity="%s"`, rgb(o.Fill), op(o.Fill))
			} else {
				b.WriteString(` fill="none"`)
			}
			if o.Stroke.A > 0 {
				fmt.Fprintf(&b, ` stroke="%s" stroke-opacity="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round"`, rgb(o.Stroke), op(o.Stroke), f(o.StrokeW))
				if len(o.Dash) > 0 {
					fmt.Fprintf(&b, ` stroke-dasharray="%s %s"`, f(o.Dash[0]), f(o.Dash[1]))
				} else if o.Draw < 1 {
					L := r.length(o.D)
					fmt.Fprintf(&b, ` stroke-dasharray="%s %s"`, f(L*o.Draw), f(L+1))
				}
			}
			b.WriteString(`/>`)
		case render.TextOp:
			ff := fam[o.Font]
			an := "start"
			switch o.Anchor {
			case render.Middle:
				an = "middle"
			case render.End:
				an = "end"
			}
			s := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(o.Text)
			fmt.Fprintf(&b, `<text transform="%s" x="%s" y="%s" font-family="%s" font-weight="%s" font-size="%s" text-anchor="%s" fill="%s" fill-opacity="%s" xml:space="preserve">%s</text>`,
				tr, f(o.X), f(o.Y), ff[0], ff[1], f(o.Size), an, rgb(o.Color), op(o.Color), s)
		}
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

func (r *Resvg) Rasterize(l *render.List, dst *image.RGBA) error {
	svg := r.SVG(l)
	rc := C.rv_render((*C.uint8_t)(unsafe.Pointer(&svg[0])), C.size_t(len(svg)), C.uint32_t(l.W), C.uint32_t(l.H), (*C.uint8_t)(unsafe.Pointer(&dst.Pix[0])))
	if rc != 0 {
		return fmt.Errorf("resvg render failed: %d", rc)
	}
	return nil
}
