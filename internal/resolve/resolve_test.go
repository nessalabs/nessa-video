package resolve

import (
	"os"
	"strings"
	"testing"

	c "reel/internal/composition"
	"reel/internal/selector"
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
