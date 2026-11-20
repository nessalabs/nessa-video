package resolve

import (
	"fmt"
	"os"
	"strings"
	"testing"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/selector"
)

func mustSel(t *testing.T, s string) *selector.Selector {
	t.Helper()
	sel, err := selector.ParseSelector(s)
	if err != nil {
		t.Fatal(err)
	}
	return sel
}

func TestExampleResolves(t *testing.T) {
	src, err := os.ReadFile("../../examples/cache-refresh.reel")
	if err != nil {
		t.Fatal(err)
	}
	res := Script(string(src))
	if len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
	v := res.Video
	if v.Title != "PR 482 · cache refresh" || v.Width != 1920 || v.FPS != 30 || len(v.Scenes) != 4 {
		t.Fatalf("video = %+v", v)
	}

	// components expand into parts owned by the instance
	est := v.Scenes[0]
	units := c.Select(est.Root, mustSel(t, "#api/units > box"))
	if len(units) != 3 || units[1].PartName() != "unit2" {
		t.Fatalf("server units = %d", len(units))
	}

	// sequence expands into a connector and two steps
	req := v.Scenes[1]
	if len(req.Steps) != 3 || req.Steps[0].Verb != "send" || req.Steps[1].Verb != "focus" || req.Steps[2].Verb != "say" {
		t.Fatalf("request steps = %+v", req.Steps)
	}
	edges := c.Select(req.Root, mustSel(t, "edge"))
	if len(edges) != 1 || edges[0].Text != "GET /v1/items" || edges[0].Edge.To.Src != "#api/unit1" {
		t.Fatalf("edges = %+v", edges)
	}

	// elements carry over into the next scene
	if len(c.Select(req.Root, mustSel(t, "#web"))) != 1 {
		t.Fatal("#web should carry into scene 2")
	}

	// children added through a part address land in the slot target
	inside := v.Scenes[2]
	procs := c.Select(inside.Root, mustSel(t, "#api/procs > process"))
	if len(procs) != 3 {
		t.Fatalf("processes in #api/procs = %d", len(procs))
	}
}

func TestCascade(t *testing.T) {
	src := `video "t"
style
  box              state=dim enter=fade
  .hot             state=focus
  row box          enter=rise
  #b2              state=active
scene s
  row enter=none
    box #b1 .hot
    box #b2 .hot
    box #b3 state=select
    label #l "x"
`
	res := Script(src)
	if len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
	root := res.Video.Scenes[0].Root
	get := func(id, key string) c.Value {
		e := c.Select(root, mustSel(t, "#"+id))[0]
		return e.Computed[key]
	}
	cases := []struct{ id, key, want string }{
		{"b1", "state", "focus"},  // class beats kind
		{"b2", "state", "active"}, // id beats class
		{"b3", "state", "select"}, // inline beats rules
		{"b1", "enter", "rise"},   // more specific script rule
		{"l", "enter", "fade"},    // theme default for every kind
	}
	for _, tc := range cases {
		if got := get(tc.id, tc.key).Value; got != tc.want {
			t.Errorf("#%s %s = %q, want %q", tc.id, tc.key, got, tc.want)
		}
	}
	if o := get("b1", "state").Origin; o.Layer != c.LayerScriptRule || o.Rule.Selector.Src != ".hot" {
		t.Errorf("origin = %+v", o)
	}
}

func TestInheritance(t *testing.T) {
	res := Script("video \"t\"\nstyle\n  row font=mono\nscene s\n  row\n    box #b\n")
	if len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
	b := c.Select(res.Video.Scenes[0].Root, mustSel(t, "#b"))[0]
	if v := b.Computed["font"]; v.Value != "mono" || !v.Origin.Inherited {
		t.Fatalf("font = %+v", v)
	}
}

func TestDiagnostics(t *testing.T) {
	cases := []struct{ src, want string }{
		{"video \"t\"\n  size 1921x1080\nscene s\n  box", "must be even"},
		{"video \"t\"\nscene s\n  servr #a", `did you mean "server"`},
		{"video \"t\"\nscene s\n  box #a enter=slide", `unknown enter mode "slide"`},
		{"video \"t\"\nscene s\n  box #a\n  focus #ab", "matches nothing"},
		{"video \"t\"\nscene s\n  box #a\n  #a -> #zz", "connector end #zz"},
		{"video \"t\"\nscene s\n  wait soon", "bad duration"},
		{"video \"t\"\nscene a\n  box #a\nscene b\n  box #a", "still on screen"},
		{"video \"t\"\nscene a\n  box #a\nscene b\n  clear\n  box #a", ""},
		{"video \"t\"\nstyle\n  procss state=dim\nscene s\n  box", "unknown kind \"procss\""},
		{"video \"t\"\ndefine q n=2\n  repeat n\n    box #s{i}\n  label \"{nope}\"\nscene s\n  q #q", "unknown parameter {nope}"},
		{"video \"t\"\ndefine q\n  box\nscene s\n  q #q\n    box", "no slot"},
		{"video \"t\"\nscene s\n  focus", "needs a target"},
		{"video \"t\"\n  theme neon\nscene s\n  box", "unknown theme"},
		{"video \"t\"", "no scenes"},
	}
	for _, tc := range cases {
		res := Script(tc.src)
		var msgs []string
		for _, d := range res.Diags {
			msgs = append(msgs, d.Error())
		}
		all := strings.Join(msgs, "\n")
		if tc.want == "" {
			if res.Diags.HasErrors() {
				t.Errorf("%q: unexpected errors:\n%s", tc.src, all)
			}
			continue
		}
		if !strings.Contains(all, tc.want) {
			t.Errorf("%q: want %q in:\n%s", tc.src, tc.want, all)
		}
	}
}

func TestDefineComposes(t *testing.T) {
	src := `video "t"
define queue items=3 label=""
  col gap=8
    repeat items
      box #slot
    label #label "{label}"

define service name="api"
  row
    server #host "server · {name}"
      slot
    queue #jobs items=4 label="jobs"
  #host -> #jobs "enqueue"

scene s
  service #billing name="billing"
    process "api" .hot
    process "worker"
  focus #billing/jobs/slot:nth(2)
`
	res := Script(src)
	if res.Diags.HasErrors() {
		t.Fatalf("diags: %v", res.Diags)
	}
	root := res.Video.Scenes[0].Root
	if n := len(c.Select(root, mustSel(t, "#billing/jobs/slot"))); n != 4 {
		t.Fatalf("queue slots = %d", n)
	}
	host := c.Select(root, mustSel(t, "#billing/host"))
	if len(host) != 1 || host[0].Text != "server · billing" {
		t.Fatalf("host = %+v", host)
	}
	if n := len(c.Select(root, mustSel(t, "#billing/host/procs > process"))); n != 2 {
		t.Fatalf("slotted processes = %d", n)
	}
}

func TestStyleCanNameLaterDefinitions(t *testing.T) {
	res := Script("video \"t\"\nstyle\n  widget state=dim\ndefine widget\n  box\nscene s\n  widget #w\n")
	if len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
}

func TestCodeAndDiffBlocks(t *testing.T) {
	diffText := "--- a/x.go\n+++ b/x.go\n@@ -3,3 +3,3 @@ func f() {\n a := 1\n-b := 2\n+b := 3\n c := 4\n"
	files := map[string]string{"change.diff": diffText, "main.py": "import os\n\ndef run():\n    return 1\n"}
	opt := Options{ReadFile: func(p string) ([]byte, error) {
		if s, ok := files[p]; ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	}}
	res := ScriptWith(`video "t"
scene s
  diff #d file=change.diff focus=+4
  code #c file=main.py lines=3..4
  code #i rust
    fn main() {}
`, opt)
	if res.Diags.HasErrors() || len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
	root := res.Video.Scenes[0].Root
	d := c.Select(root, mustSel(t, "#d"))[0].Code
	if !d.Diff || d.Path != "x.go" || len(d.Lines) != 4 || d.Lang != "Go" || d.Focus == nil {
		t.Fatalf("diff block = %+v", d)
	}
	cb := c.Select(root, mustSel(t, "#c"))[0].Code
	if len(cb.Lines) != 2 || cb.Lines[0].New != 3 || cb.Lang != "Python" {
		t.Fatalf("code block = %+v", cb)
	}
	if ib := c.Select(root, mustSel(t, "#i"))[0].Code; ib.Lang != "Rust" || len(ib.Lines) != 1 {
		t.Fatalf("inline block = %+v", ib)
	}

	bad := []struct{ src, want string }{
		{"video \"t\"\nscene s\n  diff file=nope.diff", "cannot read nope.diff"},
		{"video \"t\"\nscene s\n  diff\n    not a diff", "cannot read the diff"},
		{"video \"t\"\nscene s\n  diff file=change.diff hunk=4", "bad hunk"},
		{"video \"t\"\nscene s\n  diff file=change.diff path=y.go", `no file "y.go"`},
		{"video \"t\"\nscene s\n  code go focus=3..1\n    x", "bad focus"},
		{"video \"t\"\nscene s\n  code go focus=9\n    x", "matches no shown line"},
		{"video \"t\"\nscene s\n  code", "code is empty"},
		{"video \"t\"\nscene s\n  code #c go\n    x\n  mark #c", "mark needs lines="},
	}
	for _, b := range bad {
		r := ScriptWith(b.src, opt)
		var msgs []string
		for _, d := range r.Diags {
			msgs = append(msgs, d.Error())
		}
		if all := strings.Join(msgs, "\n"); !strings.Contains(all, b.want) {
			t.Errorf("%q: want %q in:\n%s", b.src, b.want, all)
		}
	}
	if r := Script("video \"t\"\nscene s\n  diff file=x.diff"); !strings.Contains(r.Diags[0].Error(), "cannot be read here") {
		t.Errorf("without a file reader: %v", r.Diags)
	}
}

func TestDataBlocks(t *testing.T) {
	res := Script(`video "t"
scene s
  array #a 1 3 4 ptr="lo=0 hi=2"
  chart #c line
    burst, before, after
    1, 4, 1
    2, 7, 1
  timeline #tl unit=s
    request A | 0-18 wait | *18-22 200
  mermaid #flow
    flowchart LR
      C[Client] --> G[Gateway]
      G --> H{Cache hit?}
      H -- yes --> R[Respond]
      H -->|no| D[(Database)]
      D -.-> R
  math "x^2"
  ptr #a lo=1
  swap #a 0 2
  mark #a cells=0..1
  walk #flow C G H
  play #tl
`)
	if len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
	root := res.Video.Scenes[0].Root
	a := c.Select(root, mustSel(t, "#a"))[0].Array
	if len(a.Values) != 3 || len(a.Pointers) != 2 || a.Pointers[1].Index != 2 {
		t.Fatalf("array = %+v", a)
	}
	ch := c.Select(root, mustSel(t, "#c"))[0].Chart
	if ch.Kind != "line" || len(ch.Series) != 2 || len(ch.Rows) != 2 || ch.Rows[1].Values[0] != 7 {
		t.Fatalf("chart = %+v", ch)
	}
	tl := c.Select(root, mustSel(t, "#tl"))[0].Timeline
	if tl.Unit != "s" || len(tl.Lanes[0].Spans) != 2 || !tl.Lanes[0].Spans[1].Strong || tl.End() != 22 {
		t.Fatalf("timeline = %+v", tl)
	}
	h := c.Select(root, mustSel(t, "#flow/H"))
	if len(h) != 1 || h[0].Kind() != "decision" || h[0].Text != "Cache hit?" {
		t.Fatalf("H = %+v", h)
	}
	if d := c.Select(root, mustSel(t, "#flow/D"))[0]; d.Kind() != "store" {
		t.Fatalf("D kind = %s", d.Kind())
	}
	edges := c.Select(root, mustSel(t, "#flow edge"))
	if len(edges) != 5 {
		t.Fatalf("edges = %d", len(edges))
	}
	labels := map[string]string{}
	for _, e := range edges {
		labels[e.Edge.From.Src+e.Edge.To.Src] = e.Text + "|" + e.Edge.Op
	}
	if labels["#H#R"] != "yes|->" || labels["#H#D"] != "no|->" || labels["#D#R"] != "|..>" {
		t.Fatalf("edge labels = %v", labels)
	}
	rank := func(id string) string {
		v, _ := c.Select(root, mustSel(t, "#flow/"+id))[0].Attr("rank")
		return v
	}
	if rank("C") != "0" || rank("H") != "2" || rank("R") != "4" {
		t.Fatalf("ranks C=%s H=%s R=%s", rank("C"), rank("H"), rank("R"))
	}

	bad := []struct{ src, want string }{
		{"video \"t\"\nscene s\n  array #a", "array needs values"},
		{"video \"t\"\nscene s\n  array #a 1 2 ptr=\"lo=5\"", "outside the array"},
		{"video \"t\"\nscene s\n  chart pie\n    a, b\n    1, 2", "unknown chart kind"},
		{"video \"t\"\nscene s\n  chart\n    a, b\n    x, y", `bad value "y"`},
		{"video \"t\"\nscene s\n  timeline\n    lane | 5-2 back", "bad span"},
		{"video \"t\"\nscene s\n  mermaid\n    classDiagram\n    A <|-- B", "unsupported Mermaid diagram"},
		{"video \"t\"\nscene s\n  mermaid\n    sequenceDiagram\n    alt x\n    A->>B: hi", "unclosed frame"},
		{"video \"t\"\nscene s\n  mermaid\n    sequenceDiagram\n    A says hi", "cannot read"},
		{"video \"t\"\nscene s\n  mermaid\n    flowchart LR\n    A --> ", "link has no target"},
		{"video \"t\"\nscene s\n  array #a 1 2\n  swap #a 1", "swap needs"},
		{"video \"t\"\nscene s\n  array #a 1 2\n  ptr #a lo=x", "bad position"},
		{"video \"t\"\nscene s\n  math", "math needs a formula"},
	}
	for _, b := range bad {
		r := Script(b.src)
		var msgs []string
		for _, d := range r.Diags {
			msgs = append(msgs, d.Error())
		}
		if all := strings.Join(msgs, "\n"); !strings.Contains(all, b.want) {
			t.Errorf("%q: want %q in:\n%s", b.src, b.want, all)
		}
	}
}

func TestThemePacks(t *testing.T) {
	files := map[string]string{
		"themes/brand/theme.reel": "extends hairline\ntokens\n  line-2 #FF5A5F\nstyle\n  box state=focus\ndefine pill\n  box #b\n",
		"themes/loop/theme.reel":  "extends loop\n",
		"themes/bad/theme.reel":   "tokens\n  bg #12\nscene x\n",
	}
	opt := Options{ReadFile: func(p string) ([]byte, error) {
		if s, ok := files[p]; ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	}}

	res := ScriptWith("video \"t\"\n  theme brand\ntokens\n  hi #FFFFFF\nscene s\n  pill #x\n  laptop #l\n", opt)
	if len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
	v := res.Video
	if v.Tokens["line-2"] != "#FF5A5F" || v.Tokens["bg"] != "#08090A" || v.Tokens["hi"] != "#FFFFFF" {
		t.Fatalf("tokens: line-2=%s bg=%s hi=%s", v.Tokens["line-2"], v.Tokens["bg"], v.Tokens["hi"])
	}
	box := c.Select(v.Scenes[0].Root, mustSel(t, "#x/b"))[0]
	if box.Computed["state"].Value != "focus" {
		t.Errorf("pack style rule did not apply: %+v", box.Computed["state"])
	}
	if len(c.Select(v.Scenes[0].Root, mustSel(t, "#l/screen"))) != 1 {
		t.Error("components from the extended pack are missing")
	}

	if res := Script("video \"t\"\nscene s\n  box"); res.Video.Tokens["bg"] != "#08090A" || len(res.Diags) > 0 {
		t.Errorf("default theme tokens missing: %v", res.Diags)
	}
	cases := []struct{ theme, want string }{
		{"loop", "extends itself"},
		{"bad", "bad colour"},
		{"nope", "unknown theme"},
	}
	for _, tc := range cases {
		r := ScriptWith("video \"t\"\n  theme "+tc.theme+"\nscene s\n  box", opt)
		var msgs []string
		for _, d := range r.Diags {
			msgs = append(msgs, d.Error())
		}
		if all := strings.Join(msgs, "\n"); !strings.Contains(all, tc.want) {
			t.Errorf("theme %s: want %q in:\n%s", tc.theme, tc.want, all)
		}
	}
	if r := ScriptWith("video \"t\"\n  theme bad\nscene s\n  box", opt); !strings.Contains(r.Diags.Sorted()[len(r.Diags)-1].Error()+fmt.Sprint(r.Diags), "cannot be in a theme") {
		t.Errorf("scene in a theme should be refused: %v", r.Diags)
	}
}

func TestSequenceDiagram(t *testing.T) {
	res := Script(`video "t"
scene s
  mermaid #seq
    sequenceDiagram
      autonumber
      participant C as Client
      participant A as API
      C->>A: GET /items
      alt cache miss
        A->>A: load
      else hit
        Note over A: cached
      end
      A-->>C: 200 OK
      A-x C: drop
`)
	if len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
	sb := c.Select(res.Video.Scenes[0].Root, mustSel(t, "#seq"))[0].Sequence
	if sb == nil || !sb.Numbered || len(sb.Participants) != 2 || sb.Participants[1] != "API" {
		t.Fatalf("sequence = %+v", sb)
	}
	kinds := ""
	for _, it := range sb.Items {
		kinds += it.Kind[:1]
	}
	if kinds != "momencmm" {
		t.Fatalf("item kinds = %s", kinds)
	}
	reply, drop := sb.Items[6], sb.Items[7]
	if !reply.Dashed || !reply.Arrow || reply.From != 1 || reply.To != 0 || !drop.Cross {
		t.Fatalf("reply = %+v drop = %+v", reply, drop)
	}
}

func TestOrientationStyles(t *testing.T) {
	src := func(size string) string {
		return "video \"t\"\n  size " + size + "\nstyle landscape\n  #main dir=row\nstyle portrait\n  #main dir=col\n  .wide hidden=true\nscene s\n  col #main\n    box #a\n    box #b .wide\n"
	}
	for size, want := range map[string][2]string{"1920x1080": {"row", ""}, "1080x1920": {"col", "true"}} {
		res := Script(src(size))
		if len(res.Diags) > 0 {
			t.Fatalf("%s diags: %v", size, res.Diags)
		}
		root := res.Video.Scenes[0].Root
		if got := c.Select(root, mustSel(t, "#main"))[0].Computed["dir"].Value; got != want[0] {
			t.Errorf("%s: dir = %q, want %q", size, got, want[0])
		}
		if got := c.Select(root, mustSel(t, "#b"))[0].Computed["hidden"].Value; got != want[1] {
			t.Errorf("%s: hidden = %q, want %q", size, got, want[1])
		}
	}
	r := Script("video \"t\"\nstyle sideways\n  box w=1\nscene s\n  box")
	if !strings.Contains(fmt.Sprint(r.Diags), "unknown style condition") {
		t.Errorf("diags = %v", r.Diags)
	}
	r = Script("video \"t\"\nstyle portrait\n  box dir=diagonal\nscene s\n  box")
	if !strings.Contains(fmt.Sprint(r.Diags), "unknown direction") {
		t.Errorf("rules for the other orientation should still be checked: %v", r.Diags)
	}
}

func TestLongDiffIsFoldedAroundFocusAndMarks(t *testing.T) {
	var b strings.Builder
	b.WriteString("--- a/x.go\n+++ b/x.go\n@@ -1,40 +1,40 @@\n")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&b, " line%d\n", i)
	}
	opt := Options{ReadFile: func(string) ([]byte, error) { return []byte(b.String()), nil }}
	res := ScriptWith("video \"t\"\nscene s\n  diff #d file=x.diff focus=+5\n  mark #d lines=+30\n", opt)
	if len(res.Diags) > 0 {
		t.Fatal(res.Diags)
	}
	cb := c.Select(res.Video.Scenes[0].Root, mustSel(t, "#d"))[0].Code
	shown := map[int]bool{}
	gaps := 0
	for _, l := range cb.Lines {
		if l.Kind == 3 {
			gaps++
		} else {
			shown[l.New] = true
		}
	}
	if !shown[5] || !shown[30] || shown[15] || gaps != 3 {
		t.Fatalf("folded lines: shown=%v gaps=%d", shown, gaps)
	}
	r := ScriptWith("video \"t\"\nscene s\n  diff file=x.diff focus=+39..+42\n", opt)
	if !strings.Contains(fmt.Sprint(r.Diags), "lines +41 to +42") {
		t.Errorf("partial focus should warn: %v", r.Diags)
	}
}

// Many marks tighten the context around each so the block stays readable.
func TestManyMarksTightenFolding(t *testing.T) {
	var b strings.Builder
	b.WriteString("--- a/x.go\n+++ b/x.go\n@@ -1,100 +1,100 @@\n")
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&b, " line%d\n", i)
	}
	opt := Options{ReadFile: func(string) ([]byte, error) { return []byte(b.String()), nil }}
	src := "video \"t\"\nscene s\n  diff #d file=x.diff focus=+5\n"
	marks := []int{20, 35, 50, 65, 80}
	for _, m := range marks {
		src += fmt.Sprintf("  mark #d lines=+%d\n", m)
	}
	res := ScriptWith(src, opt)
	if len(res.Diags) > 0 {
		t.Fatal(res.Diags)
	}
	cb := c.Select(res.Video.Scenes[0].Root, mustSel(t, "#d"))[0].Code
	if len(cb.Lines) > 18 {
		t.Fatalf("%d rows shown; want at most 18", len(cb.Lines))
	}
	shown := map[int]bool{}
	for _, l := range cb.Lines {
		if l.Kind != 3 {
			shown[l.New] = true
		}
	}
	for _, m := range append(marks, 5) {
		if !shown[m] {
			t.Errorf("marked line +%d was folded away", m)
		}
	}
}
