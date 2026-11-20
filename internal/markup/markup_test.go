package markup

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	got := Parse("one check {bad:is ==failing==} {x} {red:now}")
	want := []Span{
		{Text: "one check "},
		{Text: "is ", Tone: "bad"},
		{Text: "failing", Tone: "bad", Hi: true},
		{Text: " {x} "},
		{Text: "now", Tone: "bad"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestStrip(t *testing.T) {
	for in, want := range map[string]string{
		"{good:7 of 7} held":   "7 of 7 held",
		"a {b} c":              "a {b} c",
		"==x== __y__ {warn:z}": "x y z",
		"map{k: v}":            "map{k: v}",
		"{info:a} } stays":     "a } stays",
	} {
		if got := Strip(in); got != want {
			t.Errorf("Strip(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBalanceAndCut(t *testing.T) {
	got := Balance([]string{"one {bad:two", "three} four"})
	want := []string{"one {bad:two}", "{bad:three} four"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Balance = %q", got)
	}
	if s, n := Cut("ab {bad:cd} e", 4); s != "ab {bad:c}" || n != 4 {
		t.Fatalf("Cut = %q %d", s, n)
	}
}

func TestWordTones(t *testing.T) {
	got := WordTones("one check is {bad:failing}, two {warn:still running}.")
	want := []string{"", "", "", "bad", "", "warn", "warn"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}
