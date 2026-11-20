// Package skill carries the reel skill (SKILL.md and its references) inside
// the binary, so `reel skill` can print or install it wherever reel runs.
package skill

import "embed"

// Files holds SKILL.md and references/*.md.
//
//go:embed SKILL.md references/*.md
var Files embed.FS
