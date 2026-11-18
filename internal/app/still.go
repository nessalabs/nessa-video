package app

import (
	"fmt"
	"image"

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
	rep := CheckIn(src, s.Dir)
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
