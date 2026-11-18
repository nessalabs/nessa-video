package syntax

import "testing"

func TestHighlight(t *testing.T) {
	if Lexer("", "cache.go") != "Go" || Lexer("python", "") != "Python" || Lexer("nope", "") != "" {
		t.Fatal("lexer lookup")
	}
	toks := Highlight("Go", []string{"func (c *Cache) Refresh(key string) {", `	s := "a" // note`})
	classes := map[string]Class{}
	for _, row := range toks {
		for _, tk := range row {
			classes[tk.Text] = tk.Class
		}
	}
	if classes["func"] != Keyword || classes["string"] != Type || classes["Refresh"] != Function ||
		classes[`"a"`] != String || classes["// note"] != Comment {
		t.Errorf("classes = %v", classes)
	}
	if len(toks) != 2 {
		t.Errorf("rows = %d", len(toks))
	}
}
