// Package assets embeds the files reel ships inside its binary.
package assets

import _ "embed"

// Fonts bundled with the hairline theme. Both are SIL Open Font License 1.1;
// see THIRD_PARTY_NOTICES.md.
var (
	//go:embed fonts/inter-latin-400-normal.ttf
	Inter400 []byte
	//go:embed fonts/inter-latin-500-normal.ttf
	Inter500 []byte
	//go:embed fonts/inter-latin-600-normal.ttf
	Inter600 []byte
	//go:embed fonts/jetbrains-mono-latin-400-normal.ttf
	Mono400 []byte
)
