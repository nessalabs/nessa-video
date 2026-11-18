package stage

import "testing"

func TestCaptionWords(t *testing.T) {
	got := captionWords("A client and ==one server==, really")
	want := []capWord{{"A", false}, {"client", false}, {"and", false}, {"one", true}, {"server,", true}, {"really", false}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("word %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
