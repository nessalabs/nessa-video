// Package canvas is the production Rasterizer adapter, built on the pure-Go
// github.com/tdewolff/canvas library.
package canvas

import (
	"image"
	"image/color"
	"sync"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"

	"github.com/nessalabs/nessa-video/assets"
	"github.com/nessalabs/nessa-video/internal/render"
)

// Canvas is the pure-Go backend built on github.com/tdewolff/canvas.
// Canvas implements render.Rasterizer.
type Canvas struct {
	fonts map[string]*canvas.FontFamily
	paths sync.Map // d -> *canvas.Path
}

const mmPerPt = 25.4 / 72

// New loads the embedded fonts.
func New() (*Canvas, error) {
	c := &Canvas{fonts: map[string]*canvas.FontFamily{}}
	for k, data := range map[string][]byte{
		"sans400": assets.Inter400,
		"sans500": assets.Inter500,
		"sans600": assets.Inter600,
		"mono":    assets.Mono400,
	} {
		f := canvas.NewFontFamily(k)
		if err := f.LoadFont(data, 0, canvas.FontRegular); err != nil {
			return nil, err
		}
		c.fonts[k] = f
	}
	return c, nil
}

// Measure returns the advance width of s in pixels at the given size.
// It implements layout.Measurer.
func (c *Canvas) Measure(s, font string, size float64) float64 {
	f, ok := c.fonts[font]
	if !ok {
		f = c.fonts["sans400"]
	}
	return f.Face(size / mmPerPt).TextWidth(s)
}

func (c *Canvas) Name() string { return "canvas (pure Go)" }

func (c *Canvas) path(d string) *canvas.Path {
	if v, ok := c.paths.Load(d); ok {
		return v.(*canvas.Path)
	}
	p, err := canvas.ParseSVGPath(d)
	if err != nil {
		panic(err)
	}
	c.paths.Store(d, p)
	return p
}

func toMatrix(m render.Affine) canvas.Matrix {
	return canvas.Matrix{{m[0], m[2], m[4]}, {m[1], m[3], m[5]}}
}

func (c *Canvas) Rasterize(l *render.List, dst *image.RGBA) error {
	cv := canvas.New(float64(l.W), float64(l.H))
	ctx := canvas.NewContext(cv)
	ctx.SetCoordSystem(canvas.CartesianIV)
	ctx.SetFillColor(l.BG)
	ctx.SetStrokeColor(canvas.Transparent)
	ctx.DrawPath(0, 0, canvas.Rectangle(float64(l.W), float64(l.H)))
	ctx.SetStrokeCapper(canvas.RoundCap)
	ctx.SetStrokeJoiner(canvas.RoundJoin)

	for i := range l.Ops {
		op := &l.Ops[i]
		ctx.SetView(toMatrix(op.M))
		switch op.Kind {
		case render.PathOp:
			p := c.path(op.D)
			if op.Draw < 1 && op.Stroke.A > 0 {
				L := p.Length()
				parts := p.SplitAt(L * op.Draw)
				if len(parts) == 0 {
					continue
				}
				p = parts[0]
			}
			if op.Fill.A > 0 {
				ctx.SetFillColor(op.Fill)
			} else {
				ctx.SetFillColor(canvas.Transparent)
			}
			if op.Stroke.A > 0 {
				ctx.SetStrokeColor(op.Stroke)
				ctx.SetStrokeWidth(op.StrokeW)
			} else {
				ctx.SetStrokeColor(canvas.Transparent)
			}
			if len(op.Dash) > 0 {
				ctx.SetDashes(0, op.Dash...)
			} else {
				ctx.SetDashes(0)
			}
			ctx.DrawPath(0, 0, p)
		case render.TextOp:
			f := c.fonts[op.Font]
			face := f.Face(op.Size/mmPerPt, color.Color(op.Color))
			align := canvas.Left
			switch op.Anchor {
			case render.Middle:
				align = canvas.Center
			case render.End:
				align = canvas.Right
			}
			ctx.DrawText(op.X, op.Y, canvas.NewTextLine(face, op.Text, align))
		}
	}
	ctx.ResetView()
	ras := rasterizer.FromImage(dst, canvas.DPMM(1), canvas.LinearColorSpace{})
	cv.RenderTo(ras)
	ras.Close()
	return nil
}
