package composition

import (
	"fmt"
	"strings"
)

// GitBlock is a git history told command by command: commit, branch,
// checkout, merge and rebase. Each command is one step a play reveals.
type GitBlock struct {
	Cmds []GitCmd
}

// GitCmd is one command. Op is commit, branch, checkout, merge or rebase;
// Arg names a branch (or, for commit, the commit's label); FF asks a merge
// to fast-forward.
type GitCmd struct {
	Op, Arg string
	FF      bool
	Line    string // as written, shown under the graph while it plays
}

// GitCommit is one commit as drawn at some point of the history.
type GitCommit struct {
	ID      string
	Lane    int
	Col     int
	Parents []string
	Merge   bool // a merge commit, ringed in its second parent's colour
	Ghost   bool // the old copy of a rebased commit, fading out
}

// GitState is the history after some number of commands.
type GitState struct {
	Commits []*GitCommit
	Heads   map[string]string // branch -> commit id
	Lanes   []string          // branch per lane, top to bottom
	Current string
}

// Find returns the commit with id, or nil.
func (s GitState) Find(id string) *GitCommit {
	for _, c := range s.Commits {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// LaneOf returns a branch's lane.
func (s GitState) LaneOf(branch string) int {
	for i, b := range s.Lanes {
		if b == branch {
			return i
		}
	}
	return 0
}

// Simulate replays the commands: States()[k] is the history after k of
// them (0 is the empty repository on main).
func (g *GitBlock) States() []GitState {
	cur := GitState{Heads: map[string]string{}, Lanes: []string{"main"}, Current: "main"}
	out := []GitState{cur.clone()}
	n := 0
	newID := func(label string) string {
		n++
		if label != "" {
			return label
		}
		return fmt.Sprintf("c%d", n)
	}
	col := func(parents []string) int {
		c := 0
		for _, p := range parents {
			if pc := cur.Find(p); pc != nil {
				c = max(c, pc.Col+1)
			}
		}
		return c
	}
	for _, cmd := range g.Cmds {
		// ghosts of a rebase fade out on the next step
		kept := cur.Commits[:0]
		for _, c := range cur.Commits {
			if !c.Ghost {
				kept = append(kept, c)
			}
		}
		cur.Commits = kept
		switch cmd.Op {
		case "commit":
			var parents []string
			if h, ok := cur.Heads[cur.Current]; ok {
				parents = []string{h}
			}
			c := &GitCommit{ID: newID(cmd.Arg), Lane: cur.LaneOf(cur.Current), Col: col(parents), Parents: parents}
			cur.Commits = append(cur.Commits, c)
			cur.Heads[cur.Current] = c.ID
		case "branch":
			if _, ok := cur.Heads[cmd.Arg]; !ok {
				cur.Heads[cmd.Arg] = cur.Heads[cur.Current]
				if h := cur.Heads[cmd.Arg]; h == "" {
					delete(cur.Heads, cmd.Arg)
				}
				cur.Lanes = append(cur.Lanes, cmd.Arg)
			}
			cur.Current = cmd.Arg
		case "checkout":
			cur.Current = cmd.Arg
		case "merge":
			from := cur.Heads[cmd.Arg]
			into := cur.Heads[cur.Current]
			if from == "" {
				break
			}
			if cmd.FF || into == "" || cur.reaches(from, into) {
				// fast-forward: the branch moves up, and commits only the
				// merged branch had join this branch's lane
				lane := cur.LaneOf(cur.Current)
				for id := from; id != "" && id != into && !cur.onBranchBefore(id, into); {
					c := cur.Find(id)
					if c == nil {
						break
					}
					c.Lane = lane
					if len(c.Parents) == 0 {
						break
					}
					id = c.Parents[0]
				}
				cur.Heads[cur.Current] = from
				break
			}
			parents := []string{into, from}
			c := &GitCommit{ID: newID(cmd.Arg2()), Lane: cur.LaneOf(cur.Current), Col: col(parents), Parents: parents, Merge: true}
			cur.Commits = append(cur.Commits, c)
			cur.Heads[cur.Current] = c.ID
		case "rebase":
			onto := cur.Heads[cmd.Arg]
			head := cur.Heads[cur.Current]
			if onto == "" || head == "" {
				break
			}
			// the commits on this branch that onto does not have, oldest first
			var mine []*GitCommit
			for id := head; id != "" && !cur.reaches(onto, id); {
				c := cur.Find(id)
				if c == nil {
					break
				}
				mine = append([]*GitCommit{c}, mine...)
				if len(c.Parents) == 0 {
					break
				}
				id = c.Parents[0]
			}
			prev := onto
			for _, old := range mine {
				old.Ghost = true
				nc := &GitCommit{ID: old.ID + "'", Lane: old.Lane, Parents: []string{prev}}
				nc.Col = col(nc.Parents)
				cur.Commits = append(cur.Commits, nc)
				prev = nc.ID
			}
			cur.Heads[cur.Current] = prev
		}
		out = append(out, cur.clone())
	}
	return out
}

// Arg2 is the label for a merge commit.
func (c GitCmd) Arg2() string { return "" }

// reaches reports whether commit to is an ancestor of (or is) from.
func (s GitState) reaches(from, to string) bool {
	seen := map[string]bool{}
	var walk func(id string) bool
	walk = func(id string) bool {
		if id == to {
			return true
		}
		if id == "" || seen[id] {
			return false
		}
		seen[id] = true
		if c := s.Find(id); c != nil {
			for _, p := range c.Parents {
				if walk(p) {
					return true
				}
			}
		}
		return false
	}
	return walk(from)
}

// onBranchBefore reports whether id is an ancestor of head (so it was
// already on the branch before a fast-forward).
func (s GitState) onBranchBefore(id, head string) bool {
	return head != "" && s.reaches(head, id)
}

func (s GitState) clone() GitState {
	out := GitState{Heads: map[string]string{}, Lanes: append([]string(nil), s.Lanes...), Current: s.Current}
	for k, v := range s.Heads {
		out.Heads[k] = v
	}
	for _, c := range s.Commits {
		cc := *c
		cc.Parents = append([]string(nil), c.Parents...)
		out.Commits = append(out.Commits, &cc)
	}
	return out
}

// ParseGitLine reads one command; it accepts reel's short form (`commit
// a1`, `branch feature`, `merge feature ff`, `rebase main`) and Mermaid's
// gitGraph form (`commit id: "a1"`, `checkout main`).
func ParseGitLine(line string) (GitCmd, error) {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "git ")
	f := strings.Fields(t)
	if len(f) == 0 {
		return GitCmd{}, fmt.Errorf("empty line")
	}
	cmd := GitCmd{Op: f[0], Line: "git " + t}
	rest := f[1:]
	switch cmd.Op {
	case "switch":
		cmd.Op = "checkout"
		if len(rest) > 0 && rest[0] == "-c" {
			cmd.Op, rest = "branch", rest[1:]
		}
	}
	switch cmd.Op {
	case "commit":
		for i := 0; i < len(rest); i++ {
			w := rest[i]
			if w == "id:" && i+1 < len(rest) {
				cmd.Arg = strings.Trim(rest[i+1], `"'`)
				i++
			} else if strings.HasPrefix(w, "id:") {
				cmd.Arg = strings.Trim(strings.TrimPrefix(w, "id:"), `"'`)
			} else if cmd.Arg == "" && !strings.Contains(w, ":") && !strings.HasPrefix(w, "-") {
				cmd.Arg = strings.Trim(w, `"'`)
			}
		}
	case "branch", "checkout", "merge", "rebase":
		if len(rest) == 0 {
			return cmd, fmt.Errorf("%s needs a branch name", cmd.Op)
		}
		cmd.Arg = rest[0]
		for _, w := range rest[1:] {
			if w == "ff" || w == "--ff" || w == "--ff-only" {
				cmd.FF = true
			}
		}
	default:
		return cmd, fmt.Errorf("unknown git command %q; use commit, branch, checkout, merge or rebase", f[0])
	}
	return cmd, nil
}
