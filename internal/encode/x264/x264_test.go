//go:build cgo

package x264

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/nessalabs/nessa-video/internal/encode"
)

func TestEncodeMP4(t *testing.T) {
	const w, h, fps, n = 320, 180, 30, 45
	var out bytes.Buffer
	enc, err := New(&out, encode.Options{Width: w, Height: h, FPS: fps})
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < n; i++ {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetRGBA(x, y, color.RGBA{uint8(x + i*4), uint8(y), 128, 255})
			}
		}
		f := encode.NewFrame(w, h)
		encode.ToI420(img, f)
		if err := enc.Write(f); err != nil {
			t.Fatal(err)
		}
		encode.Release(f)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	b := out.Bytes()
	if len(b) < 1000 || string(b[4:8]) != "ftyp" || !bytes.Contains(b[:4096], []byte("moov")) {
		t.Fatalf("output does not look like a fast-start MP4 (%d bytes)", len(b))
	}

	// If ffprobe is installed, confirm an independent decoder accepts the file.
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed; structural checks passed")
	}
	f, _ := os.CreateTemp("", "reel-*.mp4")
	f.Write(b)
	f.Close()
	defer os.Remove(f.Name())
	probe, err := exec.Command("ffprobe", "-v", "error", "-count_frames", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name,width,height,nb_read_frames,r_frame_rate", "-of", "csv=p=0", f.Name()).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe: %v\n%s", err, probe)
	}
	got := strings.TrimSpace(string(probe))
	if got != "h264,320,180,30/1,45" {
		t.Fatalf("ffprobe = %q", got)
	}
}
