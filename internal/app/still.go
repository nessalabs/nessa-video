package app

import (
	"fmt"
	"image"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/voice"

	"github.com/nessalabs/nessa-video/internal/layout"
	"github.com/nessalabs/nessa-video/internal/motion"
	"github.com/nessalabs/nessa-video/internal/render"
	"github.com/nessalabs/nessa-video/internal/stage"
)

// Stills renders the final state of scenes to images. The rasterizer and
// text measurer are injected so the use case does not depend on a library.
type Stills struct {
	Raster  render.Rasterizer
	Measure layout.Measurer
	Dir     string // where `file=` paths are read from; "" disables files
	// Voice answers from the narration cache only, so stills match render timing.
	Voice func(*c.VoiceSpec) voice.Voice
}

// Still is one rendered scene.
type Still struct {
	Index int
	Scene string
	Image *image.RGBA
}

// Render checks src and renders every scene, or only the named one.
// It refuses to render a script with errors.
func (s Stills) Render(src, only string) ([]Still, CheckReport, error) {
	rep := CheckWith(src, s.Dir, s.Voice)
	if !rep.OK {
		return nil, rep, fmt.Errorf("the script has errors; run reel check")
	}
	v := rep.video
	st := &stage.Stage{Tokens: v.Tokens, Measure: s.Measure, W: v.Width, H: v.Height}
	plan := motion.New(v, motion.TimingsFrom(v.Tokens))
	var out []Still
	for i, sp := range plan.Scenes {
		sc := sp.Scene
		if only != "" && sc.Name != only {
			continue
		}
		img := image.NewRGBA(image.Rect(0, 0, v.Width, v.Height))
		if err := s.Raster.Rasterize(st.Still(sp), img); err != nil {
			return nil, rep, err
		}
		out = append(out, Still{Index: i + 1, Scene: sc.Name, Image: img})
	}
	if only != "" && len(out) == 0 {
		var names []string
		for _, sc := range v.Scenes {
			names = append(names, sc.Name)
		}
		return nil, rep, fmt.Errorf("no scene named %q; scenes: %v", only, names)
	}
	return out, rep, nil
}

// RenderAt draws the frame at t seconds into the video.
func (s Stills) RenderAt(src string, t float64) (*image.RGBA, CheckReport, error) {
	rep := CheckWith(src, s.Dir, s.Voice)
	if !rep.OK {
		return nil, rep, fmt.Errorf("the script has errors; run reel check")
	}
	v := rep.video
	if t < 0 || t > rep.plan.Total {
		return nil, rep, fmt.Errorf("--at %.2f is outside the video (0 to %.2fs)", t, rep.plan.Total)
	}
	st := &stage.Stage{Tokens: v.Tokens, Measure: s.Measure, W: v.Width, H: v.Height}
	sp, lt := rep.plan.At(t)
	img := image.NewRGBA(image.Rect(0, 0, v.Width, v.Height))
	err := s.Raster.Rasterize(st.Frame(sp.Scene, st.Layout(sp.Scene), sp.At(lt)), img)
	return img, rep, err
}
