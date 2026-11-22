// Package canvas is the production Rasterizer adapter, built on the pure-Go
// github.com/tdewolff/canvas library.
package canvas

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"sync"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"

	"github.com/nessalabs/nessa-video/assets"
	"github.com/nessalabs/nessa-video/internal/render"
)

// Canvas is the pure-Go backend built on github.com/tdewolff/canvas.
// Canvas implements render.Rasterizer.
type Canvas struct {
	math  sync.Map // tex -> mathResult
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
	// long paths are mostly curves traced anew each frame; caching them
	// would only hold every frame's copy in memory
	if len(d) < 4096 {
		c.paths.Store(d, p)
	}
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

type mathResult struct {
	d          string
	w, h, base float64
	err        error
}

// Typeset lays out a TeX formula with the built-in TeX engine and returns
// its outline as SVG path data in a box with the top-left corner at 0,0,
// sized so that 1em equals size. It implements layout.Typesetter.
func (c *Canvas) Typeset(tex string, size float64) (string, float64, float64, error) {
	key := fmt.Sprintf("%g|%s", size, tex)
	v, ok := c.math.Load(key)
	if !ok {
		var res mathResult
		func() {
			defer func() {
				if r := recover(); r != nil {
					res.err = fmt.Errorf("TeX failed: %v", r)
				}
			}()
			p, err := canvas.ParseLaTeX("$" + texCompat(tex) + "$")
			if err != nil {
				res.err = err
				return
			}
			k := size / (10 * mmPerPt) // the engine sets text at 10pt, in millimetres
			b := p.Bounds()
			p = p.TransformFunc(func(x, y float64) (float64, float64) { return (x - b.X0) * k, (b.Y1 - y) * k })
			res.d, res.w, res.h = p.ToSVG(), (b.X1-b.X0)*k, (b.Y1-b.Y0)*k
		}()
		v, _ = c.math.LoadOrStore(key, res)
	}
	r := v.(mathResult)
	if r.err != nil {
		return "", 0, 0, r.err
	}
	return r.d, r.w, r.h, nil
}

// texCompat maps common LaTeX commands to plain TeX ones the engine knows.
// Operators such as \log are plain TeX already and keep their spacing.
func texCompat(s string) string {
	return strings.NewReplacer(
		`\mathrm{`, `{\rm `, `\text{`, `{\rm `, `\mathbf{`, `{\bf `, `\mathit{`, `{\it `,
	).Replace(s)
}
