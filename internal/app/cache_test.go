package app

import (
	"context"
	"testing"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/render/canvas"
	"github.com/nessalabs/nessa-video/internal/voice"
)

// twoSec speaks every line as two seconds of silence.
type twoSec struct{}

func (twoSec) Key() string { return "two" }
func (twoSec) Speak(context.Context, string) (voice.Spoken, error) {
	return voice.Spoken{Audio: make(voice.Speech, 2*voice.Rate)}, nil
}

const narrated = "video \"t\"\n  size 320x180 fps 10\n  voice silent\nscene a\n  box #b\n  say \"one\"\n  say \"two\"\n"

// check uses real lengths for lines already in the cache and never speaks.
func TestCheckUsesCachedLengths(t *testing.T) {
	dir := t.TempDir()
	offline := func(*c.VoiceSpec) voice.Voice {
		return voice.Cached{Voice: voice.Offline{K: "two"}, Dir: dir}
	}
	before := CheckWith(narrated, "", offline)
	if !before.OK || before.Lines != 2 || before.Spoken != 0 {
		t.Fatalf("empty cache: ok=%v lines=%d spoken=%d", before.OK, before.Lines, before.Spoken)
	}
	// speak one line into the cache
	if _, err := (voice.Cached{Voice: twoSec{}, Dir: dir}).Speak(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	after := CheckWith(narrated, "", offline)
	if after.Spoken != 1 {
		t.Fatalf("spoken = %d; want the cached line only", after.Spoken)
	}
	first := after.plan.Scenes[0].Timeline()[0]
	if first.Dur < 2.2-1e-6 {
		t.Fatalf("cached line lasts %.2fs; want its 2s of speech + 0.2", first.Dur)
	}
}

func TestRenderAt(t *testing.T) {
	cv, err := canvas.New()
	if err != nil {
		t.Fatal(err)
	}
	s := Stills{Raster: cv, Measure: cv}
	img, rep, err := s.RenderAt(narrated, 0.5)
	if err != nil {
		t.Fatalf("%v\n%s", err, rep.FormatText("t"))
	}
	if b := img.Bounds(); b.Dx() != 320 || b.Dy() != 180 {
		t.Fatalf("size = %v", b)
	}
	if _, _, err := s.RenderAt(narrated, 999); err == nil {
		t.Fatal("expected an error past the end of the video")
	}
}
