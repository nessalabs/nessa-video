// Package theme holds the built-in theme packs. A pack is a .reel file with
// tokens, style and define blocks, so the engine treats a theme like a
// script that runs before yours. Packs on disk are read by the resolver.
package theme

import (
	_ "embed"
	"sort"
)

// Default is the theme used when a script names none.
const Default = "hairline"

//go:embed hairline/theme.reel
var hairline string

var builtin = map[string]string{"hairline": hairline}

// Builtin returns the source of a built-in pack.
func Builtin(name string) (string, bool) {
	s, ok := builtin[name]
	return s, ok
}

// Names lists the built-in packs.
func Names() []string {
	var out []string
	for k := range builtin {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
