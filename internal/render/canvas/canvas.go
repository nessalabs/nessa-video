// Package canvas is the production Rasterizer adapter, built on the pure-Go
// github.com/tdewolff/canvas library.
package canvas

import (
	"image"
	"image/color"
	"sync"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"

	"reel/internal/render"
)

// Canvas is the pure-Go backend built on github.com/tdewolff/canvas.
// Canvas implements render.Rasterizer.
type Canvas struct {
	fonts map[string]*canvas.FontFamily
	paths sync.Map // d -> *canvas.Path
}

const mmPerPt = 25.4 / 72

func New(fontDir string) (*Canvas, error) {
	c := &Canvas{fonts: map[string]*canvas.FontFamily{}}
	load := func(key, file string) error {
		f := canvas.NewFontFamily(key)
		if err := f.LoadFontFile(fontDir+"/"+file, canvas.FontRegular); err != nil {
			return err
		}
		c.fonts[key] = f
		return nil
	}
	for k, f := range map[string]string{
		"sans400": "inter-latin-400-normal.ttf",
		"sans500": "inter-latin-500-normal.ttf",
		"sans600": "inter-latin-600-normal.ttf",
		"mono":    "jetbrains-mono-latin-400-normal.ttf",
	} {
		if err := load(k, f); err != nil {
			return nil, err
		}
	}
	return c, nil
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
