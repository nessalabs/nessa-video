package resolve

import (
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
		{"video \"t\"\nscene s\n  mermaid\n    sequenceDiagram\n    A->>B: hi", "only Mermaid flowcharts"},
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
