package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/encode"
	"github.com/nessalabs/nessa-video/internal/layout"
	"github.com/nessalabs/nessa-video/internal/motion"
	"github.com/nessalabs/nessa-video/internal/render"
	"github.com/nessalabs/nessa-video/internal/stage"
	"github.com/nessalabs/nessa-video/internal/voice"
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

	// Narration. NewVoice builds the script's voice; EncodeAudio turns the
	// mixed narration into an audio track. Mute renders captions only.
	NewVoice    func(spec *c.VoiceSpec) (voice.Voice, error)
	EncodeAudio func(pcm []int16, rate int) (*encode.Audio, error)
	Mute        bool
	Log         func(msg string)
}

// RenderStats describes a finished render.
type RenderStats struct {
	Frames   int
	Duration time.Duration // length of the video
	Took     time.Duration // wall time spent
	Spoken   int           // narrated lines
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

	speech := map[[2]int]voice.Spoken{}
	if v.Voice != nil && !vs.Mute {
		if vs.NewVoice == nil || vs.EncodeAudio == nil {
			return RenderStats{}, rep, errors.New("this build cannot narrate; render with --mute")
		}
		vo, err := vs.NewVoice(v.Voice)
		if err != nil {
			return RenderStats{}, rep, err
		}
		if speech, err = speakAll(context.Background(), v, vo, vs.Log); err != nil {
			return RenderStats{}, rep, err
		}
		for k, s := range speech {
			st := &v.Scenes[k[0]].Steps[k[1]]
			st.Speech = s.Audio.Seconds()
			if _, own := st.Lookup("speak"); !own {
				timed := s.Words
				if len(timed) == 0 { // the engine gave no timings: estimate them
					timed = voice.EstimateWords(voice.Plain(st.Spoken()), s.Audio.Seconds())
				}
				st.Words = alignWords(st.Text, timed)
			}
		}
	}
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
	if len(speech) > 0 && firstErr == nil {
		if sink, ok := enc.(encode.AudioSink); ok {
			a, err := vs.EncodeAudio(mixNarration(plan, speech), voice.Rate)
			if err != nil {
				firstErr = err
			} else {
				sink.SetAudio(a)
			}
		}
	}
	if err := enc.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	stats := RenderStats{Frames: n, Duration: time.Duration(float64(n) / float64(v.FPS) * float64(time.Second)), Took: time.Since(start), Spoken: len(speech)}
	return stats, rep, firstErr
}

// speakAll narrates every say step, a few at a time.
func speakAll(ctx context.Context, v *c.Video, vo voice.Voice, log func(string)) (map[[2]int]voice.Spoken, error) {
	type job struct {
		key  [2]int
		text string
	}
	var jobs []job
	for i, sc := range v.Scenes {
		for j, st := range sc.Steps {
			if st.Verb == "say" {
				if t := strings.TrimSpace(voice.Plain(st.Spoken())); t != "" {
					jobs = append(jobs, job{[2]int{i, j}, t})
				}
			}
		}
	}
	if log != nil && len(jobs) > 0 {
		log(fmt.Sprintf("speaking %d lines", len(jobs)))
	}
	out := map[[2]int]voice.Spoken{}
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, jb := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(jb job) {
			defer wg.Done()
			defer func() { <-sem }()
			s, err := vo.Speak(ctx, jb.text)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("narrating %q: %w", jb.text, err)
				}
				return
			}
			out[jb.key] = s
		}(jb)
	}
	wg.Wait()
	return out, firstErr
}

// mixNarration places each spoken line at its cue on one track.
func mixNarration(p *motion.Plan, speech map[[2]int]voice.Spoken) []int16 {
	n := int(math.Ceil(p.Total * voice.Rate))
	mix := make([]int32, n)
	for _, cue := range p.Narration() {
		s := speech[[2]int{cue.Scene, cue.Step}].Audio
		at := int(cue.At * voice.Rate)
		for i, v := range s {
			if at+i >= n {
				break
			}
			mix[at+i] += int32(v)
		}
	}
	out := make([]int16, n)
	for i, v := range mix {
		out[i] = int16(max(-32768, min(32767, v)))
	}
	return out
}

// alignWords times each word of a caption from the voice's word timings.
// Words are matched in order ignoring case and punctuation; a caption word
// the voice did not say exactly (a number read out, say) takes the time
// between its neighbours. It returns nil if too little matches.
func alignWords(caption string, spoken []voice.Word) []c.WordTime {
	words := strings.Fields(voice.Plain(caption))
	if len(words) == 0 || len(spoken) == 0 {
		return nil
	}
	norm := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToLower(r)
			}
			return -1
		}, s)
	}
	out := make([]c.WordTime, len(words))
	found := make([]bool, len(words))
	j, matched := 0, 0
	for i, w := range words {
		nw := norm(w)
		for k := j; k < len(spoken) && k < j+4; k++ {
			if norm(spoken[k].Text) == nw {
				out[i] = c.WordTime{Start: spoken[k].Start, End: spoken[k].End}
				found[i] = true
				matched++
				j = k + 1
				break
			}
		}
	}
	if matched*2 < len(words) {
		return nil
	}
	// fill unmatched words between their matched neighbours
	end := spoken[len(spoken)-1].End
	for i := range out {
		if found[i] {
			continue
		}
		lo, hi := 0.0, end
		if i > 0 {
			lo = out[i-1].End
		}
		for k := i + 1; k < len(out); k++ {
			if found[k] {
				hi = out[k].Start
				break
			}
		}
		out[i] = c.WordTime{Start: lo, End: max(lo, hi)}
		found[i] = true
	}
	return out
}
