// Package resolve translates parsed scripts into the composition model.
// It is the boundary between the script context and the composition domain:
// it decides which words are ids, kinds, steps or selectors, expands
// components and sequences, and reports everything that does not fit.
package resolve

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/diag"
	"github.com/nessalabs/nessa-video/internal/script"
	"github.com/nessalabs/nessa-video/internal/selector"
	"github.com/nessalabs/nessa-video/internal/source"
	"github.com/nessalabs/nessa-video/internal/source/syntax"
	"github.com/nessalabs/nessa-video/internal/theme"
)

// Result is a resolved video and every problem found on the way.
type Result struct {
	Video *c.Video
	Diags diag.List
}

// Options configure resolution.
type Options struct {
	// ReadFile reads files named by `file=`, relative to the script.
	// nil means files cannot be read, which is reported as an error.
	ReadFile func(path string) ([]byte, error)
}

// Script resolves a .reel source that does not read files.
func Script(src string) Result { return ScriptWith(src, Options{}) }

// ScriptWith resolves a .reel source.
func ScriptWith(src string, opt Options) Result {
	r := &resolver{defs: map[string]*def{}, seqs: map[string]*seq{}, opt: opt}
	nodes, pd := script.Parse(src)
	r.diags = append(r.diags, pd...)
	v := &c.Video{Width: 1920, Height: 1080, FPS: 30, Theme: theme.Default}

	var video *script.Node
	for _, n := range nodes {
		if n.Kind == "video" {
			if video != nil {
				r.diags.Errorf(n.Pos, "keep one video block at the top", "second video block")
				continue
			}
			video = n
		}
	}
	if video == nil {
		r.diags.Warnf(diag.Pos{Line: 1, Col: 1}, `start with: video "Title"`, "no video block; using 1920x1080, 30 fps, hairline theme")
	} else {
		r.video(v, video)
	}

	r.orientation = "landscape"
	if v.Height > v.Width {
		r.orientation = "portrait"
	}
	themePos := diag.Pos{Line: 1, Col: 1}
	if video != nil {
		themePos = video.Pos
		for _, ch := range video.Children {
			if ch.Kind == "theme" {
				themePos = ch.Pos
			}
		}
	}
	v.Tokens = map[string]string{}
	if !r.loadTheme(v.Theme, themePos, v.Tokens, nil) {
		r.loadTheme(theme.Default, themePos, v.Tokens, nil)
	}

	for _, n := range nodes { // definitions first, so style rules can name them
		switch n.Kind {
		case "define":
			r.define(n, false)
		case "sequence":
			r.sequence(n)
		}
	}
	for _, n := range nodes {
		switch n.Kind {
		case "video", "define", "sequence":
		case "tokens":
			r.tokens(n, v.Tokens)
		case "style":
			r.style(n, c.LayerScriptRule)
		case "import":
			r.diags.Warnf(n.Pos, "inline the definitions for now", "import is not supported yet")
		case "scene":
		default:
			r.diags.Errorf(n.Pos, hintFor(n.Kind, []string{"video", "tokens", "style", "define", "sequence", "scene", "import"}),
				"%q cannot be at the top level", n.Kind)
		}
	}
	v.Rules = r.rules

	var prev *c.Element
	for _, n := range nodes {
		if n.Kind == "scene" {
			s := r.scene(n, prev)
			v.Scenes = append(v.Scenes, s)
			prev = s.Root
		}
	}
	if len(v.Scenes) == 0 && video != nil {
		r.diags.Errorf(video.Pos, "add a block such as `scene intro`", "the script has no scenes")
	}

	v.Style()
	for _, s := range v.Scenes {
		r.windowCode(s)
		r.validate(s)
	}
	return Result{Video: v, Diags: r.diags.Sorted()}
}

type def struct {
	name   string
	params []script.Prop
	body   []*script.Node
	pos    diag.Pos
	theme  bool
}

type seq struct {
	name     string
	params   []string
	defaults []script.Prop
	body     []*script.Node
}

type resolver struct {
	diags       diag.List
	defs        map[string]*def
	seqs        map[string]*seq
	rules       []*c.Rule
	depth       int
	inTheme     bool
	opt         Options
	orientation string // "portrait" or "landscape"
}

func (r *resolver) video(v *c.Video, n *script.Node) {
	if len(n.Args) > 0 {
		v.Title = n.Args[0].Text
	}
	for _, ch := range n.Children {
		switch ch.Kind {
		case "theme":
			if len(ch.Args) > 0 {
				v.Theme = ch.Args[0].Text
			}
		case "size":
			for _, a := range ch.Args {
				if m := sizeRe.FindStringSubmatch(a.Text); m != nil {
					v.Width, _ = strconv.Atoi(m[1])
					v.Height, _ = strconv.Atoi(m[2])
				} else if a.Text == "fps" {
					continue
				} else if n, err := strconv.Atoi(a.Text); err == nil && n > 0 && n <= 120 {
					v.FPS = n
				} else {
					r.diags.Errorf(a.Pos, "e.g. size 1920x1080 or size 1080x1920 for phones", "bad size %q", a.Text)
				}
			}
		case "voice":
			r.voice(v, ch)
		case "fps":
			if len(ch.Args) > 0 {
				if n, err := strconv.Atoi(ch.Args[0].Text); err == nil && n > 0 && n <= 120 {
					v.FPS = n
				} else {
					r.diags.Errorf(ch.Args[0].Pos, "use 24, 30 or 60", "bad fps %q", ch.Args[0].Text)
				}
			}
		default:
			r.diags.Errorf(ch.Pos, "video takes theme, size, fps and voice", "unknown video setting %q", ch.Kind)
		}
	}
	if v.Width%2 != 0 || v.Height%2 != 0 {
		pos := n.Pos
		for _, ch := range n.Children {
			if ch.Kind == "size" {
				pos = ch.Pos
			}
		}
		r.diags.Errorf(pos, "use even numbers", "width and height must be even for H.264")
	}
}

// voice reads `voice ENGINE ID model=…`.
func (r *resolver) voice(v *c.Video, n *script.Node) {
	if len(n.Args) == 0 {
		r.diags.Errorf(n.Pos, "e.g. voice elevenlabs JBFqnCBsd6RMkjVDRZzb", "voice needs an engine")
		return
	}
	eng := n.Args[0].Text
	if !r.oneOf(eng, n.Args[0].Pos, []string{"elevenlabs", "phonon", "silent"}, "voice engine") {
		return
	}
	spec := &c.VoiceSpec{Engine: eng}
	if len(n.Args) > 1 {
		spec.ID = n.Args[1].Text
	}
	if eng == "elevenlabs" && spec.ID == "" {
		r.diags.Errorf(n.Pos, "find voice ids in the ElevenLabs voice library, e.g. voice elevenlabs JBFqnCBsd6RMkjVDRZzb", "elevenlabs needs a voice id")
		return
	}
	for _, p := range n.Props {
		switch p.Key {
		case "model":
			spec.Model = p.Value
		case "stability", "similarity", "style", "speed", "temperature":
			f, err := strconv.ParseFloat(p.Value, 64)
			lo, hi := 0.0, 1.0
			switch p.Key {
			case "speed":
				lo, hi = 0.7, 1.2
			case "temperature":
				hi = 2
			}
			if err != nil || f < lo || f > hi {
				r.diags.Errorf(p.Pos, fmt.Sprintf("a number from %g to %g", lo, hi), "bad %s %q", p.Key, p.Value)
				continue
			}
			if spec.Tune == nil {
				spec.Tune = map[string]float64{}
			}
			spec.Tune[p.Key] = f
		default:
			r.diags.Errorf(p.Pos, "voice takes model= stability= similarity= style= speed= temperature=", "unknown voice setting %q", p.Key)
		}
	}
	v.Voice = spec
}

var sizeRe = regexp.MustCompile(`^(\d{2,5})x(\d{2,5})$`)

// loadTheme loads a theme pack, and the packs it extends first, into the
// resolver: its tokens into tokens, its components and its style rules.
// name is a built-in pack or a folder holding theme.reel, relative to the
// script. It reports whether the pack was found.
func (r *resolver) loadTheme(name string, pos diag.Pos, tokens map[string]string, seen []string) bool {
	for _, s := range seen {
		if s == name {
			r.diags.Errorf(pos, "", "theme %q extends itself through %s", name, strings.Join(seen, " -> "))
			return false
		}
	}
	src, ok := theme.Builtin(name)
	if !ok {
		var tried []string
		for _, path := range themePaths(name) {
			tried = append(tried, path)
			if r.opt.ReadFile == nil {
				break
			}
			if b, err := r.opt.ReadFile(path); err == nil {
				src, ok = string(b), true
				break
			}
		}
		if !ok {
			hint := "built-in themes: " + strings.Join(theme.Names(), ", ")
			if r.opt.ReadFile != nil {
				hint += "; also looked for " + strings.Join(tried, " and ")
			}
			r.diags.Errorf(pos, hint, "unknown theme %q", name)
			return false
		}
	}
	nodes, d := script.Parse(src)
	file := "theme " + name
	script.SetFile(nodes, file)
	for _, x := range d {
		x.Pos.File = file
		r.diags = append(r.diags, x)
	}
	for _, n := range nodes { // the base pack comes first, so this one overrides it
		if n.Kind == "extends" {
			if len(n.Args) != 1 {
				r.diags.Errorf(n.Pos, "e.g. extends hairline", "extends needs one theme")
				continue
			}
			r.loadTheme(n.Args[0].Text, n.Pos, tokens, append(seen, name))
		}
	}
	r.inTheme = true
	for _, n := range nodes {
		switch n.Kind {
		case "tokens":
			r.tokens(n, tokens)
		case "define":
			r.define(n, true)
		case "extends", "style":
		default:
			r.diags.Errorf(n.Pos, "a theme holds extends, tokens, style and define blocks", "%q cannot be in a theme", n.Kind)
		}
	}
	for _, n := range nodes {
		if n.Kind == "style" {
			r.style(n, c.LayerThemeDefault)
		}
	}
	r.inTheme = false
	return true
}

// themePaths lists where a theme named on disk may live, relative to the script.
func themePaths(name string) []string {
	name = strings.TrimSuffix(strings.TrimSuffix(name, "/"), "/theme.reel")
	if strings.ContainsAny(name, "/.") {
		return []string{name + "/theme.reel"}
	}
	return []string{"themes/" + name + "/theme.reel", name + "/theme.reel"}
}

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// tokens reads `name value` lines into tokens, later blocks overriding earlier ones.
func (r *resolver) tokens(n *script.Node, tokens map[string]string) {
	for _, ch := range n.Children {
		if len(ch.Args) != 1 || len(ch.Props) > 0 {
			r.diags.Errorf(ch.Pos, "one token per line: `line-2 #D4D7DE`", "bad token line")
			continue
		}
		val := ch.Args[0].Text
		if strings.HasPrefix(val, "#") && !hexColor.MatchString(val) {
			r.diags.Errorf(ch.Args[0].Pos, "colours are written #rrggbb", "bad colour %q for %s", val, ch.Kind)
			continue
		}
		tokens[ch.Kind] = val
	}
}

func (r *resolver) style(n *script.Node, layer c.Layer) {
	// `style portrait` and `style landscape` apply only in that orientation,
	// like a CSS media query.
	if len(n.Args) > 0 {
		want := n.Args[0].Text
		if want != "portrait" && want != "landscape" || len(n.Args) > 1 {
			r.diags.Errorf(n.Pos, "use `style`, `style portrait` or `style landscape`", "unknown style condition %q", want)
			return
		}
		if want != r.orientation {
			for _, ch := range n.Children { // still check them, so mistakes show in either orientation
				r.styleRule(ch, layer, true)
			}
			return
		}
	}
	for _, ch := range n.Children {
		r.styleRule(ch, layer, false)
	}
}

// styleRule reads one `selector key=value` line. checkOnly validates without adding the rule.
func (r *resolver) styleRule(ch *script.Node, layer c.Layer, checkOnly bool) {
	parts := []string{ch.Kind}
	for _, a := range ch.Args {
		if a.Quoted {
			r.diags.Errorf(a.Pos, "style rules hold a selector and key=value pairs", "unexpected text in a style rule")
			return
		}
		parts = append(parts, a.Text)
	}
	src := strings.Join(parts, " ")
	sel, err := selector.ParseSelector(src)
	if err != nil {
		r.diags.Errorf(ch.Pos, "e.g. `process.hot state=focus`", "bad selector %q: %v", src, err)
		return
	}
	if len(ch.Props) == 0 {
		r.diags.Warnf(ch.Pos, "add key=value pairs after the selector", "style rule %q sets nothing", src)
	}
	r.checkSelectorKinds(sel, ch.Pos)
	rule := &c.Rule{Selector: sel, Layer: layer, Order: len(r.rules), Pos: ch.Pos}
	for _, p := range ch.Props {
		r.checkProp(p.Key, p.Pos, nil)
		r.checkValue(p.Key, p.Value, p.Pos)
		rule.Decls = append(rule.Decls, c.Decl{Key: p.Key, Value: p.Value, Pos: p.Pos})
	}
	if !checkOnly {
		r.rules = append(r.rules, rule)
	}
}

func (r *resolver) define(n *script.Node, fromTheme bool) {
	if len(n.Args) == 0 || n.Args[0].Quoted {
		r.diags.Errorf(n.Pos, "e.g. `define queue items=3`", "define needs a name")
		return
	}
	name := n.Args[0].Text
	if c.IsBuiltinKind(name) || c.IsVerb(name) {
		r.diags.Errorf(n.Args[0].Pos, "pick another name", "%q is a built-in kind and cannot be redefined", name)
		return
	}
	if old, ok := r.defs[name]; ok && !old.theme {
		r.diags.Errorf(n.Pos, fmt.Sprintf("first defined at %s", old.pos), "%q is defined twice", name)
		return
	}
	r.defs[name] = &def{name: name, params: n.Props, body: n.Children, pos: n.Pos, theme: fromTheme}
}

func (r *resolver) sequence(n *script.Node) {
	if len(n.Args) == 0 || n.Args[0].Quoted {
		r.diags.Errorf(n.Pos, "e.g. `sequence request from to text=\"GET\"`", "sequence needs a name")
		return
	}
	s := &seq{name: n.Args[0].Text, defaults: n.Props, body: n.Children}
	for _, a := range n.Args[1:] {
		s.params = append(s.params, a.Text)
	}
	r.seqs[s.name] = s
}

func (r *resolver) scene(n *script.Node, prev *c.Element) *c.Scene {
	s := &c.Scene{Pos: n.Pos, Root: c.NewElement("scene", n.Pos)}
	if len(n.Args) == 0 || n.Args[0].Quoted {
		r.diags.Errorf(n.Pos, "e.g. `scene intro` or `scene intro 4s`", "scene needs a name")
	} else {
		s.Name = n.Args[0].Text
	}
	if len(n.Args) > 1 {
		if d, ok := r.duration(n.Args[1].Text, n.Args[1].Pos, false); ok {
			s.Duration = d
		}
	}

	// carry the previous scene unless it starts with clear
	if prev != nil {
		var keep *selector.Selector
		cleared := false
		if len(n.Children) > 0 && n.Children[0].Kind == "clear" {
			cleared = true
			if v, ok := n.Children[0].Prop("keep"); ok {
				keep = r.parseSel(v, n.Children[0].Pos)
			}
		}
		for _, ch := range prev.Children {
			switch {
			case !cleared:
				s.Root.Append(ch.Clone())
			case keep != nil:
				// keep exactly what matched, lifted out of its container
				for _, m := range outermost(c.Select(ch, keep)) {
					s.Root.Append(m.Clone())
				}
			}
		}
	}

	for _, ch := range n.Children {
		r.sceneChild(s, ch, nil)
	}
	return s
}

// sceneChild handles one top-level line of a scene: a step, a connector,
// a part address or an element.
func (r *resolver) sceneChild(s *c.Scene, n *script.Node, vars map[string]string) {
	if vars != nil {
		n = subst(n, vars, &r.diags)
	}
	switch {
	case c.IsVerb(n.Kind):
		s.Steps = append(s.Steps, r.step(n))
		if len(n.Children) > 0 {
			r.diags.Errorf(n.Children[0].Pos, "steps cannot have indented lines", "unexpected lines under %s", n.Kind)
		}
	case r.seqs[n.Kind] != nil:
		r.callSequence(s, r.seqs[n.Kind], n)
	case strings.HasPrefix(n.Kind, "#") || strings.HasPrefix(n.Kind, "."):
		// address an existing element or part and add children to it
		sel := r.parseSel(n.Kind, n.Pos)
		if sel == nil {
			return
		}
		targets := c.Select(s.Root, sel)
		if len(targets) == 0 {
			r.diags.Errorf(n.Pos, r.idHint(s.Root, n.Kind), "%s matches nothing on screen", n.Kind)
			return
		}
		for _, ch := range n.Children {
			r.element(targets[0], ch, nil)
		}
	default:
		r.element(s.Root, n, nil)
	}
}

// joinGroups rejoins a selector group that spaces split, so that
// `focus #a, #b` targets both elements.
func joinGroups(args []script.Arg) []script.Arg {
	var out []script.Arg
	for _, a := range args {
		if n := len(out); n > 0 && !a.Quoted && !out[n-1].Quoted && strings.HasSuffix(out[n-1].Text, ",") {
			out[n-1].Text += " " + a.Text
			continue
		}
		out = append(out, a)
	}
	return out
}

func (r *resolver) callSequence(s *c.Scene, q *seq, n *script.Node) {
	vars := map[string]string{}
	for _, p := range q.defaults {
		vars[p.Key] = p.Value
	}
	unquoted := 0
	for _, a := range n.Args {
		if unquoted < len(q.params) {
			vars[q.params[unquoted]] = a.Text
			unquoted++
		}
	}
	if unquoted < len(q.params) {
		r.diags.Errorf(n.Pos, fmt.Sprintf("%s takes %s", q.name, strings.Join(q.params, " ")), "missing argument %q", q.params[unquoted])
		return
	}
	for _, p := range n.Props {
		vars[p.Key] = p.Value
	}
	r.depth++
	defer func() { r.depth-- }()
	if r.depth > 32 {
		r.diags.Errorf(n.Pos, "a sequence probably calls itself", "sequences nested too deeply")
		return
	}
	for _, b := range q.body {
		r.sceneChild(s, b, vars)
	}
}

func (r *resolver) step(n *script.Node) c.Step {
	st := c.Step{Verb: n.Kind, With: n.With, Pos: n.Pos}
	args := joinGroups(n.Args)
	for _, p := range n.Props {
		st.Decls = append(st.Decls, c.Decl{Key: p.Key, Value: p.Value, Pos: p.Pos})
		if n.Kind != "ptr" { // ptr takes pointer names as keys
			r.checkProp(p.Key, p.Pos, nil)
		}
		switch p.Key {
		case "for", "stagger":
			r.duration(p.Value, p.Pos, false)
		case "at":
			r.duration(p.Value, p.Pos, true)
		case "exit":
			r.oneOf(p.Value, p.Pos, c.EnterModes, "exit mode")
		}
	}
	targetAt := 0
	switch n.Kind {
	case "say":
		for _, a := range args {
			if a.Quoted {
				st.Text = a.Text
			} else {
				st.Args = append(st.Args, a.Text)
			}
		}
		if st.Text == "" {
			// say speak="…" with no caption shows the spoken words as the caption
			if v, ok := st.Lookup("speak"); ok {
				st.Text = v
				kept := st.Decls[:0]
				for _, d := range st.Decls {
					if d.Key != "speak" {
						kept = append(kept, d)
					}
				}
				st.Decls = kept
			} else {
				r.diags.Errorf(n.Pos, `e.g. say "The request travels to the server"`, "say needs quoted text")
			}
		}
		return st
	case "wait", "hold":
		if len(args) != 1 {
			r.diags.Errorf(n.Pos, "e.g. `wait 0.5s`", "%s needs one duration", n.Kind)
			return st
		}
		r.duration(args[0].Text, args[0].Pos, false)
		st.Args = []string{args[0].Text}
		return st
	case "clear":
		return st
	case "ptr":
		if len(args) == 0 || len(n.Props) == 0 {
			r.diags.Errorf(n.Pos, "e.g. `ptr #a lo=2 hi=5`", "ptr needs an array and pointer positions")
			return st
		}
		st.Target = r.parseSel(args[0].Text, args[0].Pos)
		st.Decls = nil
		for _, p := range n.Props {
			if _, err := strconv.Atoi(p.Value); err != nil && !c.TimingProp(p.Key) {
				r.diags.Errorf(p.Pos, "positions are 0-based numbers", "bad position %s=%s", p.Key, p.Value)
			}
			st.Decls = append(st.Decls, c.Decl{Key: p.Key, Value: p.Value, Pos: p.Pos})
		}
		return st
	case "swap":
		if len(args) != 3 {
			r.diags.Errorf(n.Pos, "e.g. `swap #a 1 4`", "swap needs an array and two positions")
			return st
		}
		for _, a := range args[1:] {
			if _, err := strconv.Atoi(a.Text); err != nil {
				r.diags.Errorf(a.Pos, "positions are 0-based numbers", "bad position %q", a.Text)
			}
		}
	case "walk":
		if len(args) < 2 {
			r.diags.Errorf(n.Pos, "e.g. `walk #flow C G H`", "walk needs a diagram and node ids")
			return st
		}
	case "mark":
		if v, ok := n.Prop("cells"); ok {
			if _, err := source.ParseCells(v); err != nil {
				r.diags.Errorf(n.Pos, "e.g. mark #a cells=2..4 (0-based)", "bad cells %q: %v", v, err)
			}
		} else if v, ok := n.Prop("lines"); ok {
			if _, err := source.ParseSelection(v); err != nil {
				r.diags.Errorf(n.Pos, "e.g. mark #diff lines=+12..+14", "bad lines %q: %v", v, err)
			}
		} else {
			r.diags.Errorf(n.Pos, "e.g. mark #diff lines=+12..+14 or mark #a cells=2..4", "mark needs lines= or cells=")
		}
	case "camera":
		if len(args) == 0 {
			r.diags.Errorf(n.Pos, "camera zoom #x, camera pan #x or camera reset", "camera needs an action")
			return st
		}
		act := args[0].Text
		r.oneOf(act, args[0].Pos, []string{"zoom", "pan", "reset"}, "camera action")
		st.Args = []string{act}
		if act == "reset" {
			return st
		}
		targetAt = 1
	}
	if len(args) <= targetAt {
		r.diags.Errorf(n.Pos, fmt.Sprintf("e.g. `%s #api`", n.Kind), "%s needs a target", n.Kind)
		return st
	}
	st.Target = r.parseSel(args[targetAt].Text, args[targetAt].Pos)
	for _, a := range args[targetAt+1:] {
		if a.Quoted {
			st.Text = a.Text
		} else {
			st.Args = append(st.Args, a.Text)
		}
	}
	return st
}

// element builds an element from n under parent. Inside a component body,
// owner is the instance and `#name` names a part rather than a global id.
func (r *resolver) element(parent *c.Element, n *script.Node, inst *instance) {
	if inst != nil {
		n = subst(n, inst.vars, &r.diags)
	}
	if n.Kind == "edge" {
		r.edge(parent, n, inst)
		return
	}
	if c.IsVerb(n.Kind) {
		r.diags.Errorf(n.Pos, "move it to the top level of the scene", "step %q inside an element", n.Kind)
		return
	}
	if inst != nil {
		switch n.Kind {
		case "slot":
			if inst.slot != nil {
				inst.slot(parent)
			}
			return
		case "repeat":
			r.repeat(parent, n, inst)
			return
		}
	}
	if d := r.defs[n.Kind]; d != nil {
		r.instantiate(parent, d, n, inst)
		return
	}
	if !c.IsBuiltinKind(n.Kind) {
		r.diags.Errorf(n.Pos, hintFor(n.Kind, r.kindNames()), "unknown kind %q", n.Kind)
		return
	}
	e := r.base(n, inst)
	switch n.Kind {
	case "code", "diff":
		r.codeBlock(e, n)
	case "array", "chart", "timeline", "mermaid", "math":
		r.block(e, n)
	}
	parent.Append(e)
	for _, ch := range n.Children {
		r.element(e, ch, inst)
	}
}

// codeBlock reads a code or diff block from its body or a file.
func (r *resolver) codeBlock(e *c.Element, n *script.Node) {
	file, _ := n.Prop("file")
	lang, _ := n.Prop("lang")
	pathFilter, _ := n.Prop("path")
	if len(e.Args) > 0 {
		if n.Kind == "code" && lang == "" {
			lang = e.Args[0]
		} else if n.Kind == "diff" && pathFilter == "" {
			pathFilter = e.Args[0]
		}
	}
	text := strings.Join(n.Raw, "\n")
	if file != "" {
		if len(n.Raw) > 0 {
			r.diags.Errorf(n.Pos, "use either file= or indented lines", "%s has both a file and inline lines", n.Kind)
		}
		if r.opt.ReadFile == nil {
			r.diags.Errorf(n.Pos, "inline the lines instead", "files cannot be read here")
			return
		}
		b, err := r.opt.ReadFile(file)
		if err != nil {
			r.diags.Errorf(n.Pos, "paths are relative to the script", "cannot read %s: %v", file, err)
			return
		}
		text = string(b)
	}
	if strings.TrimSpace(text) == "" {
		r.diags.Errorf(n.Pos, fmt.Sprintf("indent the %s under the line, or use file=", n.Kind), "%s is empty", n.Kind)
		return
	}
	cb := &c.CodeBlock{Path: file}
	if n.Kind == "diff" {
		cb.Diff = true
		files, err := source.ParseUnified(text)
		if err != nil {
			r.diags.Errorf(n.Pos, "paste `git diff` output or point file= at a .diff", "cannot read the diff: %v", err)
			return
		}
		fd := files[0]
		if pathFilter != "" {
			found := false
			var paths []string
			for _, f := range files {
				paths = append(paths, f.Path)
				if f.Path == pathFilter || strings.HasSuffix(f.Path, "/"+pathFilter) {
					fd, found = f, true
					break
				}
			}
			if !found {
				r.diags.Errorf(n.Pos, "files in the diff: "+strings.Join(paths, ", "), "the diff has no file %q", pathFilter)
				return
			}
		} else if len(files) > 1 {
			r.diags.Warnf(n.Pos, "add path= to pick one", "the diff changes %d files; showing %s", len(files), fd.Path)
		}
		hunk := 1
		if v, ok := n.Prop("hunk"); ok {
			h, err := strconv.Atoi(v)
			if err != nil || h < 1 || h > len(fd.Hunks) {
				r.diags.Errorf(n.Pos, fmt.Sprintf("%s has %d hunks", fd.Path, len(fd.Hunks)), "bad hunk %q", v)
				return
			}
			hunk = h
		}
		if len(fd.Hunks) == 0 {
			r.diags.Errorf(n.Pos, "", "%s has no changed lines", fd.Path)
			return
		}
		cb.Path, cb.Header, cb.Lines = fd.Path, fd.Hunks[hunk-1].Header, fd.Hunks[hunk-1].Lines
		for _, l := range cb.Lines {
			switch l.Kind {
			case source.Added:
				cb.Added++
			case source.Removed:
				cb.Removed++
			}
		}
	} else {
		from, to := 1, 0
		if v, ok := n.Prop("lines"); ok {
			sel, err := source.ParseSelection(v)
			if err != nil || len(sel) != 1 || sel[0].Side != 0 {
				r.diags.Errorf(n.Pos, "e.g. lines=11..15", "bad lines %q", v)
			} else {
				from, to = sel[0].From, sel[0].To
			}
		}
		cb.Lines = source.FileLines(text, from, to)
		if len(cb.Lines) == 0 {
			r.diags.Errorf(n.Pos, "", "lines %d..%d are outside the file", from, to)
			return
		}
	}
	cb.Lang = syntax.Lexer(lang, cb.Path)
	if lang != "" && cb.Lang == "" {
		r.diags.Warnf(n.Pos, "e.g. go, python, typescript, rust", "unknown language %q; showing plain text", lang)
	}
	if v, ok := n.Prop("focus"); ok {
		sel, err := source.ParseSelection(v)
		if err != nil {
			r.diags.Errorf(n.Pos, "e.g. focus=12..14 or focus=+12..+14 for added lines", "bad focus %q: %v", v, err)
		} else {
			cb.Focus = sel
			if !anyLine(cb.Lines, sel) {
				r.diags.Warnf(n.Pos, "line numbers are the file's own numbers, shown in the gutter of `reel still`", "focus %q matches no shown line", v)
			} else if missing := missingLines(cb.Lines, sel); missing != "" {
				r.diags.Warnf(n.Pos, "line numbers are the file's own numbers, shown in the gutter of `reel still`", "focus %q includes %s, which this block does not show", v, missing)
			}
		}
	}
	e.Code = cb
}

// missingLines names line numbers a selection asks for that no shown line has.
func missingLines(lines []source.Line, sel source.Selection) string {
	var miss []string
	for _, rg := range sel {
		for n := rg.From; n <= rg.To; n++ {
			found := false
			for _, l := range lines {
				if source.Selection([]source.Range{{Side: rg.Side, From: n, To: n}}).Has(l) {
					found = true
					break
				}
			}
			if !found {
				side := ""
				if rg.Side != 0 {
					side = string(rg.Side)
				}
				miss = append(miss, side+strconv.Itoa(n))
			}
		}
	}
	switch len(miss) {
	case 0:
		return ""
	case 1:
		return "line " + miss[0]
	}
	return fmt.Sprintf("lines %s to %s (%d lines)", miss[0], miss[len(miss)-1], len(miss))
}

func anyLine(lines []source.Line, sel source.Selection) bool {
	for _, l := range lines {
		if sel.Has(l) {
			return true
		}
	}
	return false
}

// base creates an element with id, classes, text, args and inline props.
func (r *resolver) base(n *script.Node, inst *instance) *c.Element {
	e := c.NewElement(n.Kind, n.Pos)
	for _, a := range n.Args {
		switch {
		case a.Quoted:
			if e.Text != "" {
				r.diags.Errorf(a.Pos, "an element takes one quoted text", "second text on one line")
			}
			e.Text = a.Text
		case strings.HasPrefix(a.Text, "#") && len(a.Text) > 1:
			name := a.Text[1:]
			if !validName(name) {
				r.diags.Errorf(a.Pos, "use letters, digits, - and _", "bad id %q", a.Text)
				continue
			}
			if inst != nil {
				e.SetPart(name)
				e.SetOwner(inst.el)
			} else if e.ID() != "" {
				r.diags.Errorf(a.Pos, "an element has one id; use .class for groups", "second id on one line")
			} else {
				e.SetID(name)
			}
		case strings.HasPrefix(a.Text, ".") && len(a.Text) > 1:
			e.AddClass(a.Text[1:])
		default:
			e.Args = append(e.Args, a.Text)
		}
	}
	if inst != nil && e.PartName() == "" {
		e.SetOwner(inst.el)
	}
	if len(n.Raw) > 0 {
		e.Body = n.Raw
	}
	for _, p := range n.Props {
		r.checkProp(p.Key, p.Pos, nil)
		r.checkValue(p.Key, p.Value, p.Pos)
		e.Inline = append(e.Inline, c.Decl{Key: p.Key, Value: p.Value, Pos: p.Pos})
	}
	return e
}

// checkValue validates values that have a fixed vocabulary.
func (r *resolver) checkValue(key, val string, pos diag.Pos) {
	switch key {
	case "enter", "exit":
		r.oneOf(val, pos, c.EnterModes, key+" mode")
	case "state":
		r.oneOf(val, pos, c.States, "state")
	case "tone":
		r.oneOf(val, pos, []string{"bad", "good", "warn", "info", "red", "green", "amber", "yellow", "blue"}, "tone")
	case "dir":
		r.oneOf(val, pos, []string{"row", "col", "LR", "RL", "TD", "TB"}, "direction")
	case "hidden", "fit":
		r.oneOf(val, pos, []string{"true", "false"}, key+" value")
	case "for", "stagger":
		r.duration(val, pos, false)
	}
}

// checkSelectorKinds warns when a selector names a kind that does not exist,
// since such a rule silently applies to nothing.
func (r *resolver) checkSelectorKinds(sel *selector.Selector, pos diag.Pos) {
	known := map[string]bool{"edge": true, "scene": true}
	for _, k := range r.kindNames() {
		known[k] = true
	}
	var walk func(*selector.Selector)
	walk = func(s *selector.Selector) {
		for _, cx := range s.Group {
			for _, p := range cx.Parts {
				if p.Kind != "" && p.Kind != "*" && !known[p.Kind] {
					r.diags.Warnf(pos, hintFor(p.Kind, r.kindNames()), "style rule names unknown kind %q, so it applies to nothing", p.Kind)
				}
				for _, ps := range p.Pseudos {
					if ps.Not != nil {
						walk(ps.Not)
					}
				}
			}
		}
	}
	walk(sel)
}

func validName(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if !(b == '-' || b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z') {
			return false
		}
	}
	return s != ""
}

func (r *resolver) edge(parent *c.Element, n *script.Node, inst *instance) {
	e := c.NewElement("edge", n.Pos)
	if inst != nil {
		e.SetOwner(inst.el)
	}
	edge := &c.Edge{Op: n.Args[1].Text}
	edge.From = r.edgeSel(n.Args[0], inst)
	if len(n.Args) > 2 && !n.Args[2].Quoted {
		edge.To = r.edgeSel(n.Args[2], inst)
	}
	for _, a := range n.Args[3:] {
		if a.Quoted {
			e.Text = a.Text
		} else if strings.HasPrefix(a.Text, ".") {
			e.AddClass(a.Text[1:])
		} else {
			r.diags.Errorf(a.Pos, `connector lines take a quoted label and key=value pairs`, "unexpected %q", a.Text)
		}
	}
	for _, p := range n.Props {
		r.checkProp(p.Key, p.Pos, nil)
		e.Inline = append(e.Inline, c.Decl{Key: p.Key, Value: p.Value, Pos: p.Pos})
	}
	e.Edge = edge
	parent.Append(e)
}

// edgeSel parses a connector end. Inside a component, `#x` means a part of
// the instance, so it is rewritten to an owner-relative selector at validation.
func (r *resolver) edgeSel(a script.Arg, inst *instance) *selector.Selector {
	sel := r.parseSel(a.Text, a.Pos)
	if sel == nil || inst == nil {
		return sel
	}
	for gi := range sel.Group {
		p := &sel.Group[gi].Parts[0]
		if p.ID != "" {
			p.Part, p.ID = p.ID, ""
		}
	}
	return sel
}

type instance struct {
	el   *c.Element
	vars map[string]string
	slot func(parent *c.Element)
}

func (r *resolver) instantiate(parent *c.Element, d *def, call *script.Node, outer *instance) {
	r.depth++
	defer func() { r.depth-- }()
	if r.depth > 32 {
		r.diags.Errorf(call.Pos, "a component probably uses itself", "components nested too deeply")
		return
	}
	params := map[string]bool{}
	vars := map[string]string{}
	for _, p := range d.params {
		params[p.Key] = true
		vars[p.Key] = p.Value
	}
	// separate parameter values from ordinary inline props
	ordinary := *call
	ordinary.Props = nil
	for _, p := range call.Props {
		if params[p.Key] {
			vars[p.Key] = p.Value
		} else {
			ordinary.Props = append(ordinary.Props, p)
		}
	}
	ordinary.Children = nil
	el := r.base(&ordinary, outer)
	el.SetOwner(nil)
	if outer != nil && el.PartName() != "" {
		el.SetOwner(outer.el)
	}
	vars["text"] = el.Text
	parent.Append(el)

	inst := &instance{el: el, vars: vars}
	callerChildren := call.Children
	inst.slot = func(p *c.Element) {
		for _, ch := range callerChildren {
			r.element(p, ch, outer)
		}
	}
	for _, b := range d.body {
		r.element(el, b, inst)
	}
	if len(callerChildren) > 0 && !hasSlot(d.body) {
		r.diags.Errorf(callerChildren[0].Pos, fmt.Sprintf("%s has no slot for children", d.name), "unexpected lines under %s", d.name)
	}
}

func hasSlot(ns []*script.Node) bool {
	for _, n := range ns {
		if n.Kind == "slot" || hasSlot(n.Children) {
			return true
		}
	}
	return false
}

func (r *resolver) repeat(parent *c.Element, n *script.Node, inst *instance) {
	if len(n.Args) != 1 {
		r.diags.Errorf(n.Pos, "e.g. `repeat items` or `repeat 3`", "repeat needs a count")
		return
	}
	src := n.Args[0].Text
	if v, ok := inst.vars[src]; ok {
		src = v
	}
	count, err := strconv.Atoi(src)
	if err != nil || count < 0 || count > 1000 {
		r.diags.Errorf(n.Args[0].Pos, "use a number from 0 to 1000", "bad repeat count %q", src)
		return
	}
	for i := 1; i <= count; i++ {
		vars := map[string]string{"i": strconv.Itoa(i)}
		for k, v := range inst.vars {
			vars[k] = v
		}
		inner := &instance{el: inst.el, vars: vars, slot: inst.slot}
		for _, ch := range n.Children {
			r.element(parent, ch, inner)
		}
	}
}

var varRe = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_-]*)\}`)

// subst returns a copy of n with {name} replaced in args and prop values.
// Children are substituted when they are expanded, so a repeat can add {i}.
func subst(n *script.Node, vars map[string]string, diags *diag.List) *script.Node {
	cp := *n
	rep := func(s string, pos diag.Pos) string {
		return varRe.ReplaceAllStringFunc(s, func(m string) string {
			k := m[1 : len(m)-1]
			if v, ok := vars[k]; ok {
				return v
			}
			if k == "i" { // filled in by an enclosing repeat
				return m
			}
			diags.Errorf(pos, "parameters: "+strings.Join(keys(vars), ", "), "unknown parameter {%s}", k)
			return m
		})
	}
	cp.Args = make([]script.Arg, len(n.Args))
	for i, a := range n.Args {
		a.Text = rep(a.Text, a.Pos)
		cp.Args[i] = a
	}
	cp.Props = make([]script.Prop, len(n.Props))
	for i, p := range n.Props {
		p.Value = rep(p.Value, p.Pos)
		cp.Props[i] = p
	}
	if strings.Contains(cp.Kind, "{") {
		cp.Kind = rep(cp.Kind, n.Pos)
	}
	return &cp
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (r *resolver) parseSel(src string, pos diag.Pos) *selector.Selector {
	sel, err := selector.ParseSelector(src)
	if err != nil {
		r.diags.Errorf(pos, "e.g. #api, .hot, process:first or #api/unit1", "bad selector %q: %v", src, err)
		return nil
	}
	return sel
}

func (r *resolver) checkProp(key string, pos diag.Pos, extra map[string]bool) {
	if c.IsProp(key) || extra[key] {
		return
	}
	r.diags.Warnf(pos, hintFor(key, c.Props), "unknown property %q", key)
}

func (r *resolver) duration(s string, pos diag.Pos, relative bool) (time.Duration, bool) {
	if relative {
		s = strings.TrimPrefix(s, "+")
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		r.diags.Errorf(pos, "e.g. 1.5s or 300ms", "bad duration %q", s)
		return 0, false
	}
	return d, true
}

func (r *resolver) oneOf(v string, pos diag.Pos, allowed []string, what string) bool {
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	r.diags.Errorf(pos, "use one of: "+strings.Join(allowed, ", "), "unknown %s %q", what, v)
	return false
}

func (r *resolver) kindNames() []string {
	out := c.AllKinds()
	for k := range r.defs {
		out = append(out, k)
	}
	for k := range r.seqs {
		out = append(out, k)
	}
	return out
}

func hintFor(word string, candidates []string) string {
	if s := diag.Closest(word, candidates); s != "" {
		return fmt.Sprintf("did you mean %q?", s)
	}
	return ""
}

// idHint suggests the nearest id or part on screen for a selector that matched nothing.
func (r *resolver) idHint(root *c.Element, src string) string {
	var names []string
	root.Walk(func(e *c.Element) {
		if e.ID() != "" {
			names = append(names, "#"+e.ID())
		}
		if o := e.OwnerElement(); o != nil && e.PartName() != "" && o.ID() != "" {
			names = append(names, "#"+o.ID()+"/"+e.PartName())
		}
	})
	names = uniq(names)
	if s := diag.Closest(src, names); s != "" {
		return fmt.Sprintf("did you mean %s?", s)
	}
	if len(names) == 0 {
		return "nothing on screen has an id yet"
	}
	if len(names) > 6 {
		names = names[:6]
	}
	return "on screen: " + strings.Join(names, ", ")
}

// validate checks a styled scene: ids, targets, connector ends and values.
// windowCode folds long code and diff blocks down to the lines around
// their focus and every line a mark step in the scene points at, so they
// stay readable instead of being scaled down. `lines` or `context=` control it.
func (r *resolver) windowCode(s *c.Scene) {
	keep := map[*c.CodeBlock]source.Selection{}
	s.Root.Walk(func(e *c.Element) {
		if e.Code != nil && e.Code.Focus != nil {
			keep[e.Code] = append(keep[e.Code], e.Code.Focus...)
		}
	})
	for _, st := range s.Steps {
		v, ok := st.Lookup("lines")
		if st.Verb != "mark" || !ok || st.Target == nil {
			continue
		}
		sel, err := source.ParseSelection(v)
		if err != nil {
			continue
		}
		for _, e := range c.Select(s.Root, st.Target) {
			if e.Code != nil {
				keep[e.Code] = append(keep[e.Code], sel...)
			}
		}
	}
	s.Root.Walk(func(e *c.Element) {
		if e.Code == nil || e.Code.Windowed || e.Carried {
			return
		}
		context := 3
		if v, ok := e.Computed["context"]; ok {
			if n, err := strconv.Atoi(v.Value); err == nil && n >= 0 {
				context = n
			}
		}
		lines := source.Window(e.Code.Lines, keep[e.Code], context, 16)
		for context > 0 && len(lines) > 18 { // many marks: keep fewer lines around each
			context--
			lines = source.Window(e.Code.Lines, keep[e.Code], context, 16)
		}
		e.Code.Lines = lines
		e.Code.Windowed = true
	})
}

func (r *resolver) validate(s *c.Scene) {
	r.checkOn(s)
	seen := map[string]*c.Element{}
	s.Root.Walk(func(e *c.Element) {
		if id := e.ID(); id != "" {
			if old, dup := seen[id]; dup {
				hint := fmt.Sprintf("first used at %s", old.Pos)
				if old.Pos.Line < s.Pos.Line {
					hint = "it is still on screen from an earlier scene; start this scene with `clear` or pick a new id"
				}
				r.diags.Errorf(e.Pos, hint, "id #%s is used twice in scene %s", id, s.Name)
			} else {
				seen[id] = e
			}
		}
		if e.Edge != nil {
			for _, end := range []*selector.Selector{e.Edge.From, e.Edge.To} {
				if end == nil {
					continue
				}
				scope := s.Root
				if o := e.OwnerElement(); o != nil {
					scope = o
				}
				if len(c.Select(scope, end)) == 0 {
					r.diags.Errorf(e.Pos, r.idHint(s.Root, end.Src), "connector end %s matches nothing", end.Src)
				}
			}
		}
	})
	for _, st := range s.Steps {
		if st.Target != nil && len(c.Select(s.Root, st.Target)) == 0 {
			r.diags.Errorf(st.Pos, r.idHint(s.Root, st.Target.Src), "%s target %s matches nothing in scene %s", st.Verb, st.Target.Src, s.Name)
		}
	}
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// checkOn makes sure every on= names words the last say line before it says.
func (r *resolver) checkOn(s *c.Scene) {
	var said []string
	have := false
	for _, st := range s.Steps {
		if st.Verb == "say" {
			said, have = c.SpokenWords(st.Spoken()), true
			continue
		}
		phrase, ok := st.Lookup("on")
		if !ok {
			continue
		}
		switch {
		case !have:
			r.diags.Errorf(st.Pos, "put the step after the say line that names it", "on=%q has no say line before it", phrase)
		case c.FindWords(said, phrase) < 0:
			r.diags.Errorf(st.Pos, fmt.Sprintf("the line says: %s", strings.Join(said, " ")), "the narration before this step never says %q", phrase)
		}
	}
}

// outermost drops elements nested inside another element of the list.
func outermost(els []*c.Element) []*c.Element {
	in := map[*c.Element]bool{}
	for _, e := range els {
		in[e] = true
	}
	var out []*c.Element
	for _, e := range els {
		nested := false
		for p := e.ParentElement(); p != nil; p = p.ParentElement() {
			if in[p] {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, e)
		}
	}
	return out
}
