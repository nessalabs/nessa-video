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
	for _, want := range []string{"heading has 10 words", "scene a narrates"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing warning %q in:\n%s", want, got)
		}
	}
	code := Check("video \"t\"\nscene a\n  box #b\n  say \"Subscribe\" speak=\"Then grants.subscribe returns a snapshot.\"\n")
	if got := code.FormatText("t"); !strings.Contains(got, `reads out the code name "grants.subscribe"`) {
		t.Errorf("a code name read aloud should warn:\n%s", got)
	}
	clean := Check("video \"t\"\nscene a\n  box #b\n  say \"Short and ==clear==\"\n")
	if len(clean.Diags) != 0 {
		t.Errorf("a short scene should pass: %v", clean.Diags)
	}
}
