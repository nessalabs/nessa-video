//go:build cgo

package app

import (
	"bytes"
	"context"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/encode"
	"github.com/nessalabs/nessa-video/internal/encode/aac"
	"github.com/nessalabs/nessa-video/internal/encode/x264"
	"github.com/nessalabs/nessa-video/internal/render/canvas"
	"github.com/nessalabs/nessa-video/internal/voice"
)

// tone speaks a 440 Hz tone lasting one second per word.
type tone struct{}

func (tone) Key() string { return "tone" }
func (tone) Speak(_ context.Context, text string) (voice.Speech, error) {
	n := len(strings.Fields(text)) * voice.Rate
	s := make(voice.Speech, n)
	for i := range s {
		s[i] = int16(8000 * math.Sin(2*math.Pi*440*float64(i)/voice.Rate))
	}
	return s, nil
}

func TestRenderWithNarration(t *testing.T) {
	src := "video \"t\"\n  size 320x180 fps 10\n  voice silent\nscene a\n  box #b\n  say \"one two three four five\"\nscene b\n  say \"six\" speak=\"six seven\"\n"
	cv, err := canvas.New()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	vs := Videos{Raster: cv, Measure: cv, EncodeAudio: aac.Encode,
		NewEncoder: func(w io.Writer, o encode.Options) (encode.Encoder, error) { return x264.New(w, o) },
		NewVoice:   func(*c.VoiceSpec) (voice.Voice, error) { return tone{}, nil }}
	stats, rep, err := vs.Render(src, &out, nil)
	if err != nil {
		t.Fatalf("%v\n%s", err, rep.FormatText("t"))
	}
	if stats.Spoken != 2 {
		t.Fatalf("spoken = %d", stats.Spoken)
	}
	// scene a must last at least its 5 s narration; scene b its 2 s (speak= text)
	if d := stats.Duration.Seconds(); d < 7 {
		t.Fatalf("video is %.1fs; narration needs at least 7s", d)
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	f, _ := os.CreateTemp("", "reel-*.mp4")
	f.Write(out.Bytes())
	f.Close()
	defer os.Remove(f.Name())
	probe, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type,codec_name,sample_rate,duration",
		"-of", "csv=p=0", f.Name()).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe: %v\n%s", err, probe)
	}
	got := string(probe)
	if !strings.Contains(got, "h264,video") || !strings.Contains(got, "aac,audio,24000") {
		t.Fatalf("streams:\n%s", got)
	}
	// the audio must actually contain sound
	vol, _ := exec.Command("ffmpeg", "-v", "info", "-i", f.Name(), "-af", "volumedetect", "-f", "null", "-").CombinedOutput()
	if !strings.Contains(string(vol), "max_volume") || strings.Contains(string(vol), "max_volume: -91") {
		t.Fatalf("audio looks silent:\n%s", vol)
	}

	// --mute renders without a voice
	vs.Mute = true
	out.Reset()
	if stats, _, err := vs.Render(src, &out, nil); err != nil || stats.Spoken != 0 {
		t.Fatalf("muted render: %v spoken=%d", err, stats.Spoken)
	}
}
