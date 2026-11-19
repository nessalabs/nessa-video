# Third-party notices

reel includes or links the following software and fonts.

| Component | Use | Licence |
| --- | --- | --- |
| Inter (Latin subset, weights 400, 500, 600) | Embedded font for the hairline theme | SIL Open Font License 1.1 |
| JetBrains Mono (Latin subset, weight 400) | Embedded font for the hairline theme | SIL Open Font License 1.1 |
| github.com/tdewolff/canvas | Vector drawing, text and rasterizing | MIT |
| x264 | H.264 encoder, linked into the binary | GPL-2.0-or-later |
| github.com/gen2brain/aac-go (VisualOn AAC encoder) | AAC narration track, compiled into the binary | Apache-2.0 |
| github.com/alecthomas/chroma | Syntax highlighting lexers | MIT |
| star-tex.org/x/tex | TeX engine for maths | BSD-3-Clause |
| github.com/tdewolff/font (patched copy in third_party/) | Font parsing for text and maths | MIT |
| Latin Modern fonts (github.com/go-fonts/latin-modern) | Maths fonts | GUST Font License; Go package BSD-3-Clause |
| ptts, the Phonon runtime (github.com/gradium-ai/xn-ptts), with its Rust dependencies | Local narration, compiled into the binary through `internal/voice/phonon/native` | MIT or Apache-2.0; dependencies listed by `cargo tree` in that folder |
| Phonon 31M English model and voices (huggingface.co/gradium/phonon-31m-en, gradium/phonon-voices) | Downloaded into the cache on first use, not shipped in the binary | Apache-2.0 |
| resvg (benchmark only, not in the reel binary) | Comparison rasterizer in `cmd/bench` | Apache-2.0 or MIT |

The font files in `assets/fonts` are the Latin subsets published by
Fontsource. The SIL Open Font License allows bundling the fonts with
software. It does not allow selling the fonts on their own, and modified
versions may not use the reserved font names.

reel is GPL-2.0-or-later. The Apache-2.0 parts above are compatible with
version 3 of the GPL, so binaries that include them are distributed under
GPL-3.0-or-later terms.

Go module dependencies and their licences are listed in `go.sum` and can be
reviewed with `go list -m all`.
