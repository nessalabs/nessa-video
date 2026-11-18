package app

import (
	"errors"
	"image"
	"io"
	"runtime"
	"sync"
	"time"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/encode"
	"github.com/nessalabs/nessa-video/internal/layout"
	"github.com/nessalabs/nessa-video/internal/motion"
	"github.com/nessalabs/nessa-video/internal/render"
	"github.com/nessalabs/nessa-video/internal/stage"
)

// Videos renders scripts to video. Its adapters are injected: a rasterizer
// that is safe for concurrent use, a text measurer and an encoder factory.
type Videos struct {
	Raster     render.Rasterizer
	Measure    layout.Measurer
	NewEncoder func(w io.Writer, o encode.Options) (encode.Encoder, error)
	Workers    int
	CRF        float64
	Preset     string
	Dir        string // where `file=` paths are read from; "" disables files
}

// RenderStats describes a finished render.
type RenderStats struct {
	Frames   int
	Duration time.Duration // length of the video
	Took     time.Duration // wall time spent
}

// Render checks src and writes the encoded video to out. progress, if set,
// is called after each frame is encoded.
func (vs Videos) Render(src string, out io.Writer, progress func(done, total int)) (RenderStats, CheckReport, error) {
	start := time.Now()
	rep := CheckIn(src, vs.Dir)
	if !rep.OK {
		return RenderStats{}, rep, errors.New("the script has errors; run reel check")
	}
	v := rep.video
	st := &stage.Stage{Tokens: v.Tokens, Measure: vs.Measure, W: v.Width, H: v.Height}
	plan := motion.New(v, motion.TimingsFrom(v.Tokens))
	lays := map[*c.Scene]layout.Result{}
	for _, sp := range plan.Scenes {
		lays[sp.Scene] = st.Layout(sp.Scene)
	}

	n := int(plan.Total*float64(v.FPS) + 0.5)
	if n < 1 {
		n = 1
	}
	enc, err := vs.NewEncoder(out, encode.Options{Width: v.Width, Height: v.Height, FPS: v.FPS, CRF: vs.CRF, Preset: vs.Preset})
	if err != nil {
		return RenderStats{}, rep, err
	}

	workers := vs.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	type result struct {
		i   int
		f   *encode.Frame
		err error
	}
	jobs := make(chan int)
	results := make(chan result, workers)
	slots := make(chan struct{}, workers*3) // bounds frames held in memory
	imgs := sync.Pool{New: func() any { return image.NewRGBA(image.Rect(0, 0, v.Width, v.Height)) }}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				t := float64(i) / float64(v.FPS)
				sp, lt := plan.At(t)
				img := imgs.Get().(*image.RGBA)
				err := vs.Raster.Rasterize(st.Frame(sp.Scene, lays[sp.Scene], sp.At(lt)), img)
				f := encode.NewFrame(v.Width, v.Height)
				if err == nil {
					encode.ToI420(img, f)
				}
				imgs.Put(img)
				results <- result{i, f, err}
			}
		}()
	}
	go func() {
		for i := 0; i < n; i++ {
			slots <- struct{}{}
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	pending := map[int]*encode.Frame{}
	next := 0
	var firstErr error
	for r := range results {
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
		pending[r.i] = r.f
		for f, ok := pending[next]; ok; f, ok = pending[next] {
			delete(pending, next)
			if firstErr == nil {
				if err := enc.Write(f); err != nil {
					firstErr = err
				}
			}
			encode.Release(f)
			<-slots
			next++
			if progress != nil {
				progress(next, n)
			}
		}
	}
	if err := enc.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	stats := RenderStats{Frames: n, Duration: time.Duration(float64(n) / float64(v.FPS) * float64(time.Second)), Took: time.Since(start)}
	return stats, rep, firstErr
}
