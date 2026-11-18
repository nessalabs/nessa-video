// Package app holds reel's use cases. Each one composes the contexts for a
// single job (check a script, explain an element, render a still) and is
// what the CLI, and later a hosted service, call.
package app

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/diag"
	"github.com/nessalabs/nessa-video/internal/motion"
	"github.com/nessalabs/nessa-video/internal/resolve"
	"github.com/nessalabs/nessa-video/internal/selector"
	"github.com/nessalabs/nessa-video/internal/voice"
)

// CheckReport is the result of checking a script.
type CheckReport struct {
	OK      bool        `json:"ok"`
	Title   string      `json:"title,omitempty"`
	Size    string      `json:"size,omitempty"`
	FPS     int         `json:"fps,omitempty"`
	Theme   string      `json:"theme,omitempty"`
	Voice   string      `json:"voice,omitempty"`
	Lines   int         `json:"narrated_lines,omitempty"` // say lines to speak
	Spoken  int         `json:"spoken_lines,omitempty"`   // of those, already spoken and cached
	Seconds float64     `json:"seconds"`                  // video length
	Scenes  []SceneInfo `json:"scenes"`
	Diags   diag.List   `json:"diagnostics"`

	video *c.Video
	plan  *motion.Plan
}

// SceneInfo summarizes one scene.
type SceneInfo struct {
	Name     string  `json:"name"`
	Elements int     `json:"elements"`
	Steps    int     `json:"steps"`
	Duration string  `json:"duration,omitempty"` // as written in the script
	Start    float64 `json:"start"`              // seconds into the video
	Seconds  float64 `json:"seconds"`            // how long it plays
}

// Check resolves a script that reads no files.
func Check(src string) CheckReport { return CheckIn(src, "") }

// CheckIn resolves a script whose `file=` paths are relative to dir.
// An empty dir means the script cannot read files.
func CheckIn(src, dir string) CheckReport { return CheckWith(src, dir, nil) }

// CheckWith is CheckIn with a voice used only for lines it can answer
// without speaking (an offline cache), so timings match the render.
func CheckWith(src, dir string, vo func(*c.VoiceSpec) voice.Voice) CheckReport {
	var opt resolve.Options
	if dir != "" {
		opt.ReadFile = func(p string) ([]byte, error) {
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			return os.ReadFile(p)
		}
	}
	res := resolve.ScriptWith(src, opt)
	v := res.Video
	rep := CheckReport{
		OK: !res.Diags.HasErrors(), Title: v.Title, Theme: v.Theme, FPS: v.FPS,
		Size: fmt.Sprintf("%dx%d", v.Width, v.Height), Diags: res.Diags, video: v,
	}
	if rep.Diags == nil {
		rep.Diags = diag.List{}
	}
	if v.Voice != nil {
		rep.Voice = strings.TrimSpace(v.Voice.Engine + " " + v.Voice.ID)
		rep.Lines = sayCount(v)
		if vo != nil && rep.OK {
			if inst := vo(v.Voice); inst != nil {
				speech, _ := speakAll(context.Background(), v, inst, nil) // misses are fine
				applySpeech(v, speech)
				rep.Spoken = len(speech)
			}
		}
	}
	plan := motion.New(v, motion.TimingsFrom(v.Tokens))
	rep.plan = plan
	rep.Seconds = round1(plan.Total)
	for i, s := range v.Scenes {
		n := 0
		s.Root.Walk(func(*c.Element) { n++ })
		info := SceneInfo{Name: s.Name, Elements: n - 1, Steps: len(s.Steps)}
		if s.Duration > 0 {
			info.Duration = s.Duration.String()
		}
		info.Start, info.Seconds = round1(plan.Scenes[i].Start), round1(plan.Scenes[i].Dur)
		rep.Scenes = append(rep.Scenes, info)
	}
	return rep
}

// PropOrigin explains one computed property.
type PropOrigin struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	From   string `json:"from"`
	Where  string `json:"where"`
	Detail string `json:"detail,omitempty"`
}

// Explained lists where every property on one element came from.
type Explained struct {
	Scene   string       `json:"scene"`
	Element string       `json:"element"`
	Where   string       `json:"where"`
	Props   []PropOrigin `json:"props"`
}

// Explain reports the cascade for every element matching sel, scene by scene.
func (r CheckReport) Explain(sel string) ([]Explained, error) {
	s, err := selector.ParseSelector(sel)
	if err != nil {
		return nil, err
	}
	var out []Explained
	for _, sc := range r.video.Scenes {
		for _, e := range c.Select(sc.Root, s) {
			ex := Explained{Scene: sc.Name, Element: e.Label(), Where: e.Pos.String()}
			var keys []string
			for k := range e.Computed {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v := e.Computed[k]
				po := PropOrigin{Key: k, Value: v.Value, From: v.Origin.Layer.String(), Where: v.Origin.Pos.String()}
				if v.Origin.Rule != nil {
					sp := v.Origin.Specificity
					po.Detail = fmt.Sprintf("rule %q, specificity %d,%d,%d", v.Origin.Rule.Selector.Src, sp[0], sp[1], sp[2])
				}
				if v.Origin.Inherited {
					po.From = "inherited from parent, set by " + po.From
				}
				ex.Props = append(ex.Props, po)
			}
			out = append(out, ex)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s matches nothing in any scene", sel)
	}
	return out, nil
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// FormatTimeline lists when every step runs, scene by scene.
func (r CheckReport) FormatTimeline() string {
	if r.plan == nil {
		return ""
	}
	var b strings.Builder
	for _, sp := range r.plan.Scenes {
		fmt.Fprintf(&b, "%s  (%.1fs from %.1fs)\n", sp.Scene.Name, sp.Dur, sp.Start)
		for _, st := range sp.Timeline() {
			what := st.Verb
			if st.Target != "" {
				what += " " + st.Target
			}
			if st.Text != "" {
				what += fmt.Sprintf(" %q", st.Text)
			}
			fmt.Fprintf(&b, "  %6.2fs  %5.2fs  line %-4d %s\n", sp.Start+st.Start, st.Dur, st.Line, what)
		}
	}
	return b.String()
}

// FormatText renders diagnostics for people.
func (r CheckReport) FormatText(file string) string {
	var b strings.Builder
	for _, d := range r.Diags {
		fmt.Fprintf(&b, "%s:%s\n", file, d.Error())
	}
	errs, warns := 0, 0
	for _, d := range r.Diags {
		if d.Severity == diag.Warning {
			warns++
		} else {
			errs++
		}
	}
	if r.OK {
		for _, s := range r.Scenes {
			fmt.Fprintf(&b, "  %-16s %5.1fs  from %5.1fs\n", s.Name, s.Seconds, s.Start)
		}
		if r.Voice != "" {
			switch {
			case r.Lines > 0 && r.Spoken == r.Lines:
				fmt.Fprintf(&b, "  narration: %s; all %d lines spoken before, lengths are exact\n", r.Voice, r.Lines)
			case r.Spoken > 0:
				fmt.Fprintf(&b, "  narration: %s; %d of %d lines spoken before, the rest estimated\n", r.Voice, r.Spoken, r.Lines)
			default:
				fmt.Fprintf(&b, "  narration: %s; lengths are estimated until reel render speaks them\n", r.Voice)
			}
		}
		fmt.Fprintf(&b, "ok: %d scenes, %.1fs, %s, %d fps, theme %s", len(r.Scenes), r.Seconds, r.Size, r.FPS, r.Theme)
		if warns > 0 {
			fmt.Fprintf(&b, ", %d warnings", warns)
		}
		b.WriteString("\n")
	} else {
		fmt.Fprintf(&b, "%d errors, %d warnings\n", errs, warns)
	}
	return b.String()
}
