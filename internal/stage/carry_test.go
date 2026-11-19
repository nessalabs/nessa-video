package stage

import (
	"testing"

	"github.com/nessalabs/nessa-video/internal/motion"
	"github.com/nessalabs/nessa-video/internal/render"
	"github.com/nessalabs/nessa-video/internal/resolve"
)

type mono struct{}

func (mono) Measure(s, font string, size float64) float64 { return float64(len(s)) * size * 0.6 }

// A kept element is lifted out of its container, keeps its pointers, and
// glides from where the last scene left it.
func TestCarriedElementGlides(t *testing.T) {
	r := resolve.Script(`video "t"
scene one
  col
    label "log"
    array #log 1 2 3 4 ptr="lo=0"
  ptr #log lo=2
scene two
  clear keep="#log"
  box #b "new"
`)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	two := r.Video.Scenes[1]
	if n := len(two.Root.Children); n != 2 || two.Root.Children[0].Array == nil {
		t.Fatalf("scene two should hold the log itself and the box, got %d children", n)
	}
	p := motion.New(r.Video, motion.DefaultTimings())
	st := &Stage{Measure: mono{}, W: 1280, H: 720}
	lays := st.Layouts(p)
	log := two.Root.Children[0]
	was, ok := lays[two].Came[log]
	if !ok || was.W == 0 {
		t.Fatalf("no starting place for the carried log: %+v", lays[two].Came)
	}
	sp := p.Scenes[1]
	if g := sp.At(0).Glide(); g != 0 {
		t.Errorf("glide at the cut = %v, want 0", g)
	}
	if g := sp.At(motion.GlideTime).Glide(); g != 1 {
		t.Errorf("glide after %vs = %v, want 1", motion.GlideTime, g)
	}
	if ptrs := sp.At(0).Pointers(log); len(ptrs) != 1 || ptrs[0].Pos != 2 {
		t.Errorf("pointer should stay where scene one left it: %+v", ptrs)
	}
}

// When a diagram leaves, its connectors and their labels fade with it.
func TestConnectorsLeaveWithTheirDiagram(t *testing.T) {
	r := resolve.Script(`video "t"
scene one
  mermaid #f
    flowchart LR
      A[a] -- go --> B[b]
scene two
  clear
  box "next"
`)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	p := motion.New(r.Video, motion.DefaultTimings())
	st := &Stage{Measure: mono{}, W: 1280, H: 720}
	lays := st.Layouts(p)
	one := p.Scenes[0]
	full := st.Frame(one.Scene, lays[one.Scene], one.At(one.Dur-0.5))
	gone := st.Frame(one.Scene, lays[one.Scene], one.At(one.Dur-0.01))
	maxA := func(l *render.List) uint8 {
		var m uint8
		for _, op := range l.Ops {
			m = max(m, op.Stroke.A, op.Fill.A, op.Color.A)
		}
		return m
	}
	if maxA(full) < 100 {
		t.Fatalf("the diagram should be visible before it leaves")
	}
	if a := maxA(gone); a > 40 {
		t.Errorf("something is still drawn at alpha %d as the diagram leaves", a)
	}
}
