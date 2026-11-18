# Third-party notices

reel includes or links the following software and fonts.

| Component | Use | Licence |
| --- | --- | --- |
| Inter (Latin subset, weights 400, 500, 600) | Embedded font for the hairline theme | SIL Open Font License 1.1 |
| JetBrains Mono (Latin subset, weight 400) | Embedded font for the hairline theme | SIL Open Font License 1.1 |
| github.com/tdewolff/canvas | Vector drawing, text and rasterizing | MIT |
| x264 | H.264 encoder, linked into the binary | GPL-2.0-or-later |
| github.com/alecthomas/chroma | Syntax highlighting lexers | MIT |
| star-tex.org/x/tex | TeX engine for maths | BSD-3-Clause |
| Latin Modern fonts (github.com/go-fonts/latin-modern) | Maths fonts | GUST Font License; Go package BSD-3-Clause |
| resvg (benchmark only, not in the reel binary) | Comparison rasterizer in `cmd/bench` | Apache-2.0 or MIT |

The font files in `assets/fonts` are the Latin subsets published by
Fontsource. The SIL Open Font License allows bundling the fonts with
software. It does not allow selling the fonts on their own, and modified
versions may not use the reserved font names.

Go module dependencies and their licences are listed in `go.sum` and can be
reviewed with `go list -m all`.
