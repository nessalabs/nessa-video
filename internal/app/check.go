// Package app holds reel's use cases. Each one composes the contexts for a
// single job (check a script, explain an element, render a still) and is
// what the CLI, and later a hosted service, call.
package app

import (
	"fmt"
	"sort"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/diag"
	"github.com/nessalabs/nessa-video/internal/resolve"
	"github.com/nessalabs/nessa-video/internal/selector"
)

// CheckReport is the result of checking a script.
type CheckReport struct {
	OK     bool        `json:"ok"`
	Title  string      `json:"title,omitempty"`
	Size   string      `json:"size,omitempty"`
	FPS    int         `json:"fps,omitempty"`
	Theme  string      `json:"theme,omitempty"`
	Scenes []SceneInfo `json:"scenes"`
	Diags  diag.List   `json:"diagnostics"`

	video *c.Video
}

// SceneInfo summarizes one scene.
type SceneInfo struct {
	Name     string `json:"name"`
	Elements int    `json:"elements"`
	Steps    int    `json:"steps"`
	Duration string `json:"duration,omitempty"`
}

// Check resolves a script and reports its problems.
func Check(src string) CheckReport {
	res := resolve.Script(src)
	v := res.Video
	rep := CheckReport{
		OK: !res.Diags.HasErrors(), Title: v.Title, Theme: v.Theme, FPS: v.FPS,
		Size: fmt.Sprintf("%dx%d", v.Width, v.Height), Diags: res.Diags, video: v,
	}
	if rep.Diags == nil {
		rep.Diags = diag.List{}
	}
	for _, s := range v.Scenes {
		n := 0
		s.Root.Walk(func(*c.Element) { n++ })
		info := SceneInfo{Name: s.Name, Elements: n - 1, Steps: len(s.Steps)}
		if s.Duration > 0 {
			info.Duration = s.Duration.String()
		}
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
		fmt.Fprintf(&b, "ok: %d scenes, %s, %d fps, theme %s", len(r.Scenes), r.Size, r.FPS, r.Theme)
		if warns > 0 {
			fmt.Fprintf(&b, ", %d warnings", warns)
		}
		b.WriteString("\n")
	} else {
		fmt.Fprintf(&b, "%d errors, %d warnings\n", errs, warns)
	}
	return b.String()
}
