package app

import (
	"os"
	"testing"

	"github.com/nessalabs/nessa-video/internal/render/canvas"
)

func TestStillsRenderExample(t *testing.T) {
	src, err := os.ReadFile("../../examples/cache-refresh.reel")
	if err != nil {
		t.Fatal(err)
	}
	cv, err := canvas.New()
	if err != nil {
		t.Fatal(err)
	}
	stills, rep, err := Stills{Raster: cv, Measure: cv}.Render(string(src), "")
	if err != nil {
		t.Fatalf("%v\n%s", err, rep.FormatText("example"))
	}
	if len(stills) != 4 {
		t.Fatalf("stills = %d", len(stills))
	}
	img := stills[0].Image
	if b := img.Bounds(); b.Dx() != 1920 || b.Dy() != 1080 {
		t.Fatalf("size = %v", b)
	}
	// something other than the background was drawn
	bg := img.RGBAAt(0, 0)
	drawn := 0
	for y := 0; y < 1080; y += 4 {
		for x := 0; x < 1920; x += 4 {
			if img.RGBAAt(x, y) != bg {
				drawn++
			}
		}
	}
	if drawn < 500 {
		t.Fatalf("only %d sampled pixels differ from the background", drawn)
	}

	if _, _, err := (Stills{Raster: cv, Measure: cv}).Render(string(src), "nope"); err == nil {
		t.Fatal("expected an error for an unknown scene")
	}
	if _, _, err := (Stills{Raster: cv, Measure: cv}).Render("video \"t\"\nscene s\n  servr", ""); err == nil {
		t.Fatal("expected a refusal for a script with errors")
	}
}
