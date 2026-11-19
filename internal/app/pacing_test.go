package app

import (
	"strings"
	"testing"
)

func TestPacingWarnings(t *testing.T) {
	long := strings.Repeat("word ", 50)
	src := "video \"t\"\nscene a\n  box #b\n  say \"one two three four five six seven eight nine ten\"\n  say speak=\"" + long + "\"\n"
	rep := Check(src)
	if !rep.OK {
		t.Fatal(rep.Diags)
	}
	got := rep.FormatText("t")
	for _, want := range []string{"caption has 10 words", "scene a narrates"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing warning %q in:\n%s", want, got)
		}
	}
	clean := Check("video \"t\"\nscene a\n  box #b\n  say \"Short and ==clear==\"\n")
	if len(clean.Diags) != 0 {
		t.Errorf("a short scene should pass: %v", clean.Diags)
	}
}
