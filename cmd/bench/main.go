// Command bench renders the M0 storyboard with each backend and reports timings.
//go:build resvg

package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/nessalabs/nessa-video/internal/render"
	"github.com/nessalabs/nessa-video/internal/render/canvas"
	"github.com/nessalabs/nessa-video/internal/render/resvg"
)

func main() {
	fonts := flag.String("fonts", "assets/fonts", "font directory")
	out := flag.String("out", "out", "output directory")
	which := flag.String("backend", "all", "canvas | resvg | all")
	workers := flag.Int("workers", runtime.NumCPU(), "parallel workers")
	encode := flag.Bool("encode", true, "also pipe frames to ffmpeg")
	flag.Parse()
	os.MkdirAll(*out, 0o755)

	var backends []render.Rasterizer
	if *which == "all" || *which == "canvas" {
		b, err := canvas.New()
		must(err)
		backends = append(backends, b)
	}
	if *which == "all" || *which == "resvg" {
		b, err := resvg.New(*fonts)
		must(err)
		backends = append(backends, b)
	}

	n := Frames()
	fmt.Printf("storyboard: %d frames, %dx%d @ %d fps, %.0fs of video, %d cores\n\n", n, W, H, FPS, Duration(), runtime.NumCPU())

	// display-list build cost (shared by all backends)
	t0 := time.Now()
	for i := 0; i < n; i++ {
		Build(float64(i) / FPS)
	}
	fmt.Printf("build display lists: %.3f ms/frame\n\n", ms(time.Since(t0))/float64(n))

	for _, b := range backends {
		fmt.Printf("== %s\n", b.Name())
		img := image.NewRGBA(image.Rect(0, 0, W, H))
		// warm-up and stills
		for s := 0; s < NumShots; s++ {
			t := float64(s)*ShotLen + 2.5
			must(b.Rasterize(Build(t), img))
			name := fmt.Sprintf("%s/%s-shot%d.png", *out, short(b), s+1)
			fh, _ := os.Create(name)
			png.Encode(fh, img)
			fh.Close()
		}
		// single thread: every 3rd frame
		var per []float64
		for i := 0; i < n; i += 3 {
			l := Build(float64(i) / FPS)
			st := time.Now()
			must(b.Rasterize(l, img))
			per = append(per, ms(time.Since(st)))
		}
		sort.Float64s(per)
		fmt.Printf("  single thread: median %.1f ms, p95 %.1f ms, max %.1f ms per frame\n", per[len(per)/2], per[len(per)*95/100], per[len(per)-1])

		// parallel, no encode
		st := time.Now()
		renderAll(b, n, *workers, nil)
		el := time.Since(st)
		fmt.Printf("  %d workers: %.0f fps (%.2fs for %.0fs of video, %.1fx real time)\n", *workers, float64(n)/el.Seconds(), el.Seconds(), Duration(), Duration()/el.Seconds())

		if *encode {
			for _, enc := range []struct {
				name string
				args []string
			}{
				{"h264_videotoolbox", []string{"-c:v", "h264_videotoolbox", "-b:v", "12M"}},
				{"libx264 veryfast", []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "18"}},
			} {
				file := fmt.Sprintf("%s/%s-%s.mp4", *out, short(b), enc.args[1])
				args := append([]string{"-y", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", W, H), "-r", fmt.Sprint(FPS), "-i", "-"}, enc.args...)
				args = append(args, "-pix_fmt", "yuv420p", file)
				cmd := exec.Command("ffmpeg", args...)
				cmd.Stderr = os.Stderr
				w, _ := cmd.StdinPipe()
				must(cmd.Start())
				st := time.Now()
				renderAll(b, n, *workers, w)
				w.Close()
				must(cmd.Wait())
				el := time.Since(st)
				fmt.Printf("  render + %s: %.2fs total, %.1fx real time\n", enc.name, el.Seconds(), Duration()/el.Seconds())
			}
		}
		fmt.Println()
	}
}

// render renders all frames in parallel; if w != nil frames are written in order.
func renderAll(b render.Rasterizer, n, workers int, w io.Writer) {
	type res struct {
		i   int
		img *image.RGBA
	}
	pool := sync.Pool{New: func() any { return image.NewRGBA(image.Rect(0, 0, W, H)) }}
	jobs := make(chan int)
	done := make(chan res, workers*2)
	var wg sync.WaitGroup
	for k := 0; k < workers; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				img := pool.Get().(*image.RGBA)
				must(b.Rasterize(Build(float64(i)/FPS), img))
				done <- res{i, img}
			}
		}()
	}
	go func() {
		for i := 0; i < n; i++ {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(done)
	}()
	pending := map[int]*image.RGBA{}
	next := 0
	for r := range done {
		if w == nil {
			pool.Put(r.img)
			continue
		}
		pending[r.i] = r.img
		for img, ok := pending[next]; ok; img, ok = pending[next] {
			_, err := w.Write(img.Pix)
			must(err)
			delete(pending, next)
			pool.Put(img)
			next++
		}
	}
}

func short(b render.Rasterizer) string {
	if _, ok := b.(*canvas.Canvas); ok {
		return "canvas"
	}
	return "resvg"
}
func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
func must(err error) {
	if err != nil {
		panic(err)
	}
}
