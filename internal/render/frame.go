// Package render is the rendering context. Its model is the display list:
// an ordered set of drawing operations for one frame, independent of any
// rasterizer. Rasterizers are adapters in subpackages behind the Rasterizer port.
package render

import (
	"image"
	"image/color"
	"math"
)

// Affine maps local coords to frame pixels: x' = a*x + c*y + e, y' = b*x + d*y + f.
type Affine [6]float64

func Identity() Affine { return Affine{1, 0, 0, 1, 0, 0} }

// Mul returns m∘n (n is applied first).
func (m Affine) Mul(n Affine) Affine {
	return Affine{
		m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5],
	}
}
func Translate(x, y float64) Affine { return Affine{1, 0, 0, 1, x, y} }
func ScaleAt(s, cx, cy float64) Affine {
	return Translate(cx, cy).Mul(Affine{s, 0, 0, s, 0, 0}).Mul(Translate(-cx, -cy))
}
func (m Affine) Scale() float64 { return math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2])) }

type Kind uint8

const (
	PathOp Kind = iota
	TextOp
)

type Anchor uint8

const (
	Start Anchor = iota
	Middle
	End
)

// Op is one drawing operation. Colors carry final opacity in alpha.
type Op struct {
	Kind    Kind
	M       Affine
	D       string // SVG path data, local coords
	Stroke  color.NRGBA
	StrokeW float64
	Fill    color.NRGBA
	Draw    float64 // stroke progress 0..1
	Dash    []float64

	Text   string
	X, Y   float64
	Size   float64
	Font   string // sans400, sans500, sans600, mono
	Anchor Anchor
	Color  color.NRGBA
}

type List struct {
	W, H int
	BG   color.NRGBA
	Ops  []Op
}

// Rasterizer is the port every drawing backend implements.
type Rasterizer interface {
	Name() string
	Rasterize(l *List, dst *image.RGBA) error
}
