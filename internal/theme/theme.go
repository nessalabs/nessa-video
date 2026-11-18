// Package theme holds theme packs: design tokens, default style rules and
// component definitions. Rules and components are written in the script
// language itself, so the engine treats a theme like a script that runs first.
package theme

import (
	_ "embed"
	"sort"
)

// Theme is one pack.
type Theme struct {
	Name   string
	Tokens map[string]string // color, stroke and type tokens by name
	Source string            // .reel source with style and define blocks
}

//go:embed hairline/theme.reel
var hairlineSource string

var builtin = map[string]*Theme{
	"hairline": {
		Name: "hairline",
		Tokens: map[string]string{
			"bg": "#08090A", "ink": "#EDEDEF", "hi": "#F4F5F7",
			"ink-2": "#9BA0AB", "ink-3": "#8A8F98",
			"line-0": "#34373F", "line-1": "#6A6E7A", "line-2": "#D4D7DE",
			"stroke": "1.2", "radius": "8",
			"sans": "Inter", "mono": "JetBrains Mono",
		},
		Source: hairlineSource,
	},
}

// Get returns a built-in theme by name.
func Get(name string) (*Theme, bool) {
	t, ok := builtin[name]
	return t, ok
}

// Names lists built-in themes.
func Names() []string {
	var out []string
	for k := range builtin {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
