package composition

import "testing"

func gitOf(t *testing.T, lines ...string) *GitBlock {
	t.Helper()
	g := &GitBlock{}
	for _, l := range lines {
		c, err := ParseGitLine(l)
		if err != nil {
			t.Fatal(err)
		}
		g.Cmds = append(g.Cmds, c)
	}
	return g
}

func TestGitMerge(t *testing.T) {
	g := gitOf(t, "commit a1", "branch feature", "commit f1", "checkout main", "commit a2", "merge feature")
	end := g.States()[len(g.Cmds)]
	m := end.Find(end.Heads["main"])
	if m == nil || !m.Merge || len(m.Parents) != 2 || m.Parents[1] != "f1" {
		t.Fatalf("main should end on a merge of f1: %+v", m)
	}
	if f := end.Find("f1"); f.Lane != 1 || f.Col != 1 {
		t.Fatalf("f1 should sit on lane 1, column 1: %+v", f)
	}
}

func TestGitRebaseAndFastForward(t *testing.T) {
	g := gitOf(t, "commit a1", "switch -c feature", "commit f1", "commit f2", "checkout main", "commit a2",
		"checkout feature", "rebase main", "checkout main", "merge feature ff")
	st := g.States()
	rebased := st[8]
	if !rebased.Find("f1").Ghost || rebased.Find("f1'") == nil || rebased.Find("f1'").Parents[0] != "a2" {
		t.Fatal("rebase should copy f1 onto a2 and leave the old one as a ghost")
	}
	if rebased.Heads["feature"] != "f2'" {
		t.Fatalf("feature should point at f2', got %q", rebased.Heads["feature"])
	}
	end := st[len(g.Cmds)]
	if end.Find("f1") != nil {
		t.Fatal("ghosts leave on the next step")
	}
	if end.Heads["main"] != "f2'" || end.Find("f2'").Lane != 0 || end.Find("f1'").Lane != 0 {
		t.Fatal("a fast-forward moves main up and brings the commits onto its lane")
	}
}

func TestGitMermaidSyntax(t *testing.T) {
	c, err := ParseGitLine(`commit id: "fix"`)
	if err != nil || c.Op != "commit" || c.Arg != "fix" {
		t.Fatalf("got %+v %v", c, err)
	}
	if _, err := ParseGitLine("push origin"); err == nil {
		t.Fatal("unknown commands are reported")
	}
}
