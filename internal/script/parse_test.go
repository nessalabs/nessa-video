package script

import (
	"strings"
	"testing"
)

const example = `video "PR 482 · cache refresh"
  theme hairline
  size 1920x1080  fps 30

style
  process          enter=fade
  .hot             state=focus
  #api process[state=dim]  exit=none

# a comment line
scene establish 3s
  row gap=240 align=center
    laptop #web "laptop · web"   enter=none
    server #api "server · api-1"  enter=fade
  say "A client and ==one server=="

scene request
  #web -> #api "GET /v1/items" for=1.5s   # arrow draws by default
  focus #api/unit1 at=2s
  with dim process:not(.hot, #p1)

scene proof
  terminal #t
    $ curl -i api.internal/v1/items
    HTTP/1.1 200 OK
      indented: kept
  ` + "```mermaid" + `
  flowchart LR
    C --> G
  ` + "```" + `
`

func TestParseExample(t *testing.T) {
	nodes, diags := Parse(example)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	kinds := []string{}
	for _, n := range nodes {
		kinds = append(kinds, n.Kind)
	}
	if got := strings.Join(kinds, ","); got != "video,style,scene,scene,scene" {
		t.Fatalf("top kinds = %s", got)
	}
	video := nodes[0]
	if video.Args[0].Text != "PR 482 · cache refresh" || !video.Args[0].Quoted {
		t.Fatalf("video title = %+v", video.Args)
	}
	if len(video.Children) != 2 || video.Children[1].Kind != "size" || len(video.Children[1].Args) != 3 {
		t.Fatalf("video children = %+v", video.Children[1])
	}

	style := nodes[1]
	if style.Children[2].Kind != "#api" || style.Children[2].Args[0].Text != "process[state=dim]" {
		t.Fatalf("style rule tokens = %+v", style.Children[2])
	}

	est := nodes[2]
	if est.Args[0].Text != "establish" || est.Args[1].Text != "3s" {
		t.Fatalf("scene args = %+v", est.Args)
	}
	laptop := est.Children[0].Children[0]
	if laptop.Kind != "laptop" || laptop.Args[0].Text != "#web" || laptop.Args[1].Text != "laptop · web" {
		t.Fatalf("laptop = %+v", laptop)
	}
	if v, _ := laptop.Prop("enter"); v != "none" {
		t.Fatalf("enter = %q", v)
	}

	req := nodes[3]
	edge := req.Children[0]
	if edge.Kind != "edge" || edge.Args[1].Text != "->" || edge.Args[2].Text != "#api" || edge.Args[3].Text != "GET /v1/items" {
		t.Fatalf("edge = %+v", edge)
	}
	if v, _ := edge.Prop("for"); v != "1.5s" {
		t.Fatalf("for = %q (trailing comment should be dropped)", v)
	}
	with := req.Children[2]
	if !with.With || with.Kind != "dim" || with.Args[0].Text != "process:not(.hot, #p1)" {
		t.Fatalf("with step = %+v", with)
	}

	proof := nodes[4]
	term := proof.Children[0]
	want := []string{"$ curl -i api.internal/v1/items", "HTTP/1.1 200 OK", "  indented: kept"}
	if strings.Join(term.Raw, "|") != strings.Join(want, "|") {
		t.Fatalf("terminal raw = %q", term.Raw)
	}
	mer := proof.Children[1]
	if mer.Kind != "mermaid" || len(mer.Raw) != 2 || mer.Raw[1] != "  C --> G" {
		t.Fatalf("mermaid = %+v", mer)
	}
}

func TestParseDiags(t *testing.T) {
	cases := map[string]string{
		"scene a\n  box \"open":      "unterminated string",
		"scene a\n  box\n   label":   "",
		"scene a\n    box\n  label":  "inconsistent indentation",
		"scene a\n\tbox":             "tab used",
		"scene a\n  #a -> for=1s":    "needs a target",
		"```mermaid\nflowchart LR\n": "unclosed",
		"scene a\n  with":            "needs a step",
	}
	for src, want := range cases {
		_, diags := Parse(src)
		var got []string
		for _, d := range diags {
			got = append(got, d.Error())
		}
		all := strings.Join(got, "; ")
		if want == "" {
			if all != "" {
				t.Errorf("%q: unexpected %s", src, all)
			}
			continue
		}
		if !strings.Contains(all, want) {
			t.Errorf("%q: want %q, got %q", src, want, all)
		}
	}
}

func TestQuotedEscapes(t *testing.T) {
	nodes, d := Parse("math \"a \\Rightarrow b \\nabla \\\"q\\\" c\\\\d\"\nlabel \"one\\ntwo \\\"x\\\"\"")
	if len(d) > 0 {
		t.Fatal(d)
	}
	if got := nodes[0].Args[0].Text; got != `a \Rightarrow b \nabla "q" c\d` {
		t.Errorf("math text = %q", got)
	}
	if got := nodes[1].Args[0].Text; got != "one\ntwo \"x\"" {
		t.Errorf("label text = %q", got)
	}
}
