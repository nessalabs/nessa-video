package app

import (
	"image"
	"testing"

	"github.com/nessalabs/nessa-video/internal/render/canvas"
)

// reddish counts pixels that read as the bad tone: clearly more red than
// green or blue.
func reddish(img *image.RGBA) int {
	n := 0
	for i := 0; i+3 < len(img.Pix); i += 4 {
		r, g, b := int(img.Pix[i]), int(img.Pix[i+1]), int(img.Pix[i+2])
		if r > 120 && r > g+50 && r > b+50 {
			n++
		}
	}
	return n
}

func TestTonesColourWords(t *testing.T) {
	cv, err := canvas.New()
	if err != nil {
		t.Fatal(err)
	}
	s := Stills{Raster: cv, Measure: cv}
	render := func(src string) int {
		img, rep, err := s.RenderAt(src, 1.2)
		if err != nil {
			t.Fatalf("%v\n%s", err, rep.FormatText("t"))
		}
		return reddish(img)
	}
	head := "video \"t\"\n  size 640x360\n\nscene a\n"
	plain := render(head + "  p \"one check is failing\"\n")
	toned := render(head + "  p \"one check is {bad:failing}\"\n")
	box := render(head + "  box \"failing\" tone=bad\n")
	if plain != 0 {
		t.Fatalf("plain text has %d red pixels", plain)
	}
	if toned == 0 || box == 0 {
		t.Fatalf("toned text %d, toned box %d red pixels; want some", toned, box)
	}
}
