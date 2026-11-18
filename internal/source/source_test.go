package source

import "testing"

const sample = `diff --git a/internal/cache/cache.go b/internal/cache/cache.go
index 1111111..2222222 100644
--- a/internal/cache/cache.go
+++ b/internal/cache/cache.go
@@ -11,5 +11,5 @@ func (c *Cache) Get(key string) Item {
 func (c *Cache) Refresh(key string) {
-	if c.stale(key) {
-		c.load(key)
-	}
+	c.group.Do(key, func() {
+		if c.stale(key) { c.load(key) }
+	})
 }
`

func TestParseUnified(t *testing.T) {
	files, err := ParseUnified(sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "internal/cache/cache.go" || len(files[0].Hunks) != 1 {
		t.Fatalf("files = %+v", files)
	}
	h := files[0].Hunks[0]
	if h.Header != "func (c *Cache) Get(key string) Item {" {
		t.Errorf("header = %q", h.Header)
	}
	if len(h.Lines) != 8 {
		t.Fatalf("lines = %d: %+v", len(h.Lines), h.Lines)
	}
	first, del, add, last := h.Lines[0], h.Lines[1], h.Lines[4], h.Lines[7]
	if first.Kind != Context || first.Old != 11 || first.New != 11 {
		t.Errorf("first = %+v", first)
	}
	if del.Kind != Removed || del.Old != 12 || del.New != 0 || del.Text != "\tif c.stale(key) {" {
		t.Errorf("removed = %+v", del)
	}
	if add.Kind != Added || add.New != 12 || add.Old != 0 {
		t.Errorf("added = %+v", add)
	}
	if last.Old != 15 || last.New != 15 || last.Text != "}" {
		t.Errorf("last = %+v", last)
	}
	if _, err := ParseUnified("not a diff"); err == nil {
		t.Error("expected an error")
	}
}

func TestSelection(t *testing.T) {
	sel, err := ParseSelection("+12..+14, 20")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		l    Line
		want bool
	}{
		{Line{Kind: Added, New: 13}, true},
		{Line{Kind: Removed, Old: 13}, false},
		{Line{Kind: Context, Old: 20, New: 20}, true},
		{Line{Kind: Removed, Old: 20}, true}, // no side: removed lines match by old number
		{Line{Kind: Added, New: 15}, false},
	}
	for _, c := range cases {
		if got := sel.Has(c.l); got != c.want {
			t.Errorf("Has(%+v) = %v", c.l, got)
		}
	}
	for _, bad := range []string{"", "0", "3..1", "+3..-4", "x"} {
		if _, err := ParseSelection(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
