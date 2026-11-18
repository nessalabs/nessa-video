package layout

import (
	"testing"

	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/resolve"
	"github.com/nessalabs/nessa-video/internal/selector"
)

// fixed measures every character as 10 units wide.
type fixed struct{}

func (fixed) Measure(s, font string, size float64) float64 { return float64(len([]rune(s))) * 10 }

func opts() Options {
	return Options{Type: map[string]Type{"box": {"sans500", 15}, "label": {"mono", 13}, "title": {"sans600", 40}, "p": {"sans400", 18}},
		Gap: 24, MaxTextW: 200, BlockSize: [2]float64{480, 200}}
}

func box(t *testing.T, root *c.Element, res Result, sel string) Rect {
	t.Helper()
	s, err := selector.ParseSelector(sel)
	if err != nil {
		t.Fatal(err)
	}
	els := c.Select(root, s)
	if len(els) == 0 {
		t.Fatalf("%s matches nothing", sel)
	}
	return res.Boxes[els[0]]
}

func TestRowAndCol(t *testing.T) {
	r := resolve.Script(`video "t"
scene s
  row #r gap=20 align=start
    box #a w=100 h=40
    box #b w=50 h=80
  col #c gap=10 align=end
    box #d w=30 h=30
    box #e w=60 h=30
`)
	if r.Diags.HasErrors() {
		t.Fatal(r.Diags)
	}
	root := r.Video.Scenes[0].Root
	res := Scene(root, Rect{0, 0, 1000, 1000}, fixed{}, opts())

	row, a, b := box(t, root, res, "#r"), box(t, root, res, "#a"), box(t, root, res, "#b")
	if row.W != 170 || row.H != 80 {
		t.Errorf("row = %+v, want 170x80", row)
	}
	if a.X != row.X || b.X != row.X+120 || a.Y != row.Y || b.Y != row.Y {
		t.Errorf("row children a=%+v b=%+v row=%+v", a, b, row)
	}
	col, d, e := box(t, root, res, "#c"), box(t, root, res, "#d"), box(t, root, res, "#e")
	if col.W != 60 || col.H != 70 || d.Right() != col.Right() || e.Y != col.Y+40 {
		t.Errorf("col=%+v d=%+v e=%+v", col, d, e)
	}
	// the scene centres its content in the area
	if top := min(row.Y, col.Y); top <= 0 {
		t.Errorf("content not centred: top=%v", top)
	}
}

func TestTextAndWrap(t *testing.T) {
	r := resolve.Script(`video "t"
scene s
  box #b "hello"
  p #p "one two three four five six seven"
`)
	root := r.Video.Scenes[0].Root
	res := Scene(root, Rect{0, 0, 1000, 1000}, fixed{}, opts())
	if b := box(t, root, res, "#b"); b.W != 96 || b.H != 48 {
		t.Errorf("box with short text = %+v, want minimum 96x48", b)
	}
	s, _ := selector.ParseSelector("#p")
	p := c.Select(root, s)[0]
	if n := len(res.Lines[p]); n < 2 {
		t.Errorf("paragraph wrapped into %d lines, want at least 2", n)
	}
}

func TestFitsTheFrame(t *testing.T) {
	r := resolve.Script(`video "t"
scene s
  row #r gap=20
    box #a w=400 h=100
    box #b w=400 h=100
    box #c w=400 h=100
`)
	root := r.Video.Scenes[0].Root
	// a narrow area turns the row into a column
	res := Scene(root, Rect{0, 0, 600, 1000}, fixed{}, opts())
	a, b := box(t, root, res, "#a"), box(t, root, res, "#b")
	if b.Y <= a.Y || b.X != a.X || res.Fit != 1 {
		t.Errorf("row should become a column: a=%+v b=%+v fit=%v", a, b, res.Fit)
	}
	// too small for either direction: scaled to fit
	res = Scene(root, Rect{0, 0, 300, 200}, fixed{}, opts())
	if res.Fit >= 1 || res.Fit <= 0 {
		t.Errorf("fit = %v, want between 0 and 1", res.Fit)
	}
	// an explicit dir is kept
	r2 := resolve.Script("video \"t\"\nscene s\n  row #r dir=row\n    box w=400 h=100\n    box w=400 h=100\n")
	root2 := r2.Video.Scenes[0].Root
	res = Scene(root2, Rect{0, 0, 600, 1000}, fixed{}, opts())
	if res.Fit >= 1 {
		t.Errorf("a row with dir=row must stay a row and be scaled instead; fit=%v", res.Fit)
	}
	// hidden elements take no space
	r3 := resolve.Script("video \"t\"\nscene s\n  row #r\n    box w=100 h=40\n    box w=100 h=40 hidden=true\n")
	root3 := r3.Video.Scenes[0].Root
	res = Scene(root3, Rect{0, 0, 1000, 1000}, fixed{}, opts())
	if w := box(t, root3, res, "#r").W; w != 100 {
		t.Errorf("row width with a hidden child = %v, want 100", w)
	}
}
