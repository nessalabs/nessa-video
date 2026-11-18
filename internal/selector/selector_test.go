package selector

import "testing"

// tree is a tiny Element implementation for selector tests.
type tree struct {
	kind, id, part string
	classes        []string
	attrs          map[string]string
	parent, owner  *tree
	kids           []*tree
}

func (t *tree) Kind() string     { return t.kind }
func (t *tree) ID() string       { return t.id }
func (t *tree) PartName() string { return t.part }
func (t *tree) HasClass(c string) bool {
	for _, x := range t.classes {
		if x == c {
			return true
		}
	}
	return false
}
func (t *tree) Attr(k string) (string, bool) { v, ok := t.attrs[k]; return v, ok }
func (t *tree) Parent() Element {
	if t.parent == nil {
		return nil
	}
	return t.parent
}
func (t *tree) Owner() Element {
	if t.owner == nil {
		return nil
	}
	return t.owner
}
func (t *tree) Index() int {
	if t.parent == nil {
		return 1
	}
	for i, k := range t.parent.kids {
		if k == t {
			return i + 1
		}
	}
	return 0
}
func (t *tree) SiblingCount() int {
	if t.parent == nil {
		return 1
	}
	return len(t.parent.kids)
}

func add(parent *tree, k *tree) *tree {
	k.parent = parent
	parent.kids = append(parent.kids, k)
	return k
}

func TestSelectors(t *testing.T) {
	scene := &tree{kind: "scene"}
	row := add(scene, &tree{kind: "row"})
	api := add(row, &tree{kind: "server", id: "api"})
	procs := add(api, &tree{kind: "col", part: "procs", owner: api})
	p1 := add(procs, &tree{kind: "process", id: "p1", classes: []string{"hot"}, attrs: map[string]string{"state": "focus"}})
	p2 := add(procs, &tree{kind: "process", id: "p2", attrs: map[string]string{"state": "rest"}})
	p3 := add(procs, &tree{kind: "process", id: "p3", classes: []string{"idle"}, attrs: map[string]string{"state": "dim"}})
	box := add(row, &tree{kind: "box", id: "b"})

	cases := []struct {
		sel  string
		want []*tree
	}{
		{"process", []*tree{p1, p2, p3}},
		{"#api process", []*tree{p1, p2, p3}},
		{"#api/procs", []*tree{procs}},
		{"#api/procs > process:first", []*tree{p1}},
		{"process:last", []*tree{p3}},
		{"process:nth(2)", []*tree{p2}},
		{"process:not(.hot)", []*tree{p2, p3}},
		{"process:not(.hot, #p3)", []*tree{p2}},
		{"[state=dim]", []*tree{p3}},
		{"row > box", []*tree{box}},
		{"row > process", nil},
		{".hot, #b", []*tree{p1, box}},
		{"*", []*tree{scene, row, api, procs, p1, p2, p3, box}},
	}
	all := []*tree{scene, row, api, procs, p1, p2, p3, box}
	for _, c := range cases {
		s, err := ParseSelector(c.sel)
		if err != nil {
			t.Fatalf("%s: %v", c.sel, err)
		}
		var got []*tree
		for _, e := range all {
			if s.Matches(e) {
				got = append(got, e)
			}
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: matched %d, want %d", c.sel, len(got), len(c.want))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: match %d is %s#%s", c.sel, i, got[i].kind, got[i].id)
			}
		}
	}

	spec := func(s string) Specificity {
		sel, err := ParseSelector(s)
		if err != nil {
			t.Fatal(err)
		}
		return sel.Group[0].Specificity()
	}
	if !spec("process").Less(spec(".hot")) || !spec(".hot").Less(spec("#p1")) || !spec("#api process").Less(spec("#api process.hot")) {
		t.Error("specificity order wrong")
	}

	for _, bad := range []string{"", "#", "process:nope", "process:nth(0)", "a >", "#api/", "[x=1"} {
		if _, err := ParseSelector(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
