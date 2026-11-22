# reel for agents

Start here if you are an agent asked to install reel, make a video with it,
answer questions about it, or change its code. The first two sections cover
using reel; the rest covers working on it. The user-facing
[README](../../README.md) only covers installing and running it.

## Installing and using reel

```sh
brew install nessalabs/tap/reel     # macOS or Linux with Homebrew
# or, without Homebrew:
curl -fsSL https://raw.githubusercontent.com/nessalabs/nessa-video/main/install.sh | sh
```

Then everything is available from the CLI, with no repository needed:

1. `reel help`: what reel does, the workflow, every command and a minimal script.
2. `reel skill`: the full guide to planning and writing a script, with the
   language reference. Read it before writing one. `reel skill install`
   saves it to `~/.claude/skills/reel` (or a folder you give) so it loads
   as a skill in later sessions.
3. `reel ref`: every element kind, step, state and property this version knows.
4. Write the script, then `reel check FILE` until it exits 0. Each problem
   comes with a line number and a fix; `--json` gives the same as JSON.
5. `reel still -o DIR FILE` and look at each scene's PNG.
6. `reel render -o OUT.mp4 FILE`.

`voice phonon` narrates with the built-in local voice (Harper); nothing
needs to be set up. The first narrated render on a machine unpacks the
voice model into the user cache, about 63 MB.

## What reel is

A Go command-line tool that turns `.reel` scripts into explainer videos.
A script lists scenes; each scene holds elements (boxes, code, diffs,
diagrams, charts, components from a theme) and steps (`show`, `focus`,
`walk`, `play`, `send`, `set`, `say` …). reel checks the script, lays each
scene out, plans motion over time, draws frames, narrates the `say` lines
and encodes an H.264 MP4 with an AAC track. It ships as one binary.

- Repository: github.com/nessalabs/nessa-video (the module and the binary
  are called `reel`).
- License: GPL-2.0-or-later. It links x264, which uses the same licence.
  Third-party fonts and libraries are listed in
  [THIRD_PARTY_NOTICES.md](../../THIRD_PARTY_NOTICES.md); add any new one there.

## Writing scripts

The [reel skill](../../skills/reel/SKILL.md) teaches script writing; it is
embedded in the binary (`skills/reel/embed.go`), so `reel skill` always
matches the installed version. Its
[language reference](../../skills/reel/references/language.md) lists every
element and step, and [explaining.md](../../skills/reel/references/explaining.md)
covers how to explain things well. `reel ref` prints the built-in kinds,
steps and properties from the binary itself.

```sh
reel help                           # the workflow, every command, a minimal script
reel skill [install [DIR]]          # print the skill, or install it for an agent
reel ref                            # every kind, step and property
reel check FILE                     # problems with line numbers; --json, --timeline
reel check --explain '#id' FILE     # why an element looks the way it does
reel still -o DIR FILE              # one PNG per scene, after its steps
reel render -o OUT.mp4 FILE         # the video; --mute skips narration
reel themes / reel theme export NAME
reel version
```

To check how a moment looks, render and pull frames with ffmpeg
(`ffmpeg -ss 4.5 -i out.mp4 -frames:v 1 f.png`); a tile of frames
(`-vf fps=5,tile=4x6`) shows an animation's motion at a glance.

## Code map

| Path | What it holds |
| --- | --- |
| `cmd/reel` | the CLI, voice selection, the version flag |
| `cmd/bench` | renderer benchmark (needs the `resvg` tag; not part of the build) |
| `internal/script` | parses `.reel` text into a line tree |
| `internal/resolve` | turns the tree into the composition model; validation and diagnostics |
| `internal/composition` | the domain model: video, scenes, elements, edges; `catalog.go` lists kinds, steps and props |
| `internal/selector` | the selector language (`#id`, `.class`, `a>b`) |
| `internal/theme` | built-in theme packs; `hairline/theme.reel` holds tokens, styles and component `define`s |
| `internal/layout` | sizes and places elements: rows, columns, grids, tables, trees, asides, flowcharts, live layout states |
| `internal/motion` | plans steps over time: enters, exits, states, flow changes, `set`, narration timing |
| `internal/stage` | turns a laid-out scene at a moment into a display list (shapes, text, connectors, blocks) |
| `internal/plot` | the plot block's maths: expressions in complex numbers, gamma and zeta, adaptive sampling, the zoom view and ticks |
| `internal/render` | the display list and the canvas rasterizer |
| `internal/markup` | inline marks in text: `==bright==`, `__under__`, `{bad:tones}` |
| `internal/source` | reads diffs and source files that videos show |
| `internal/voice` | narration: caching, word timing; `elevenlabs/` and `phonon/` adapters |
| `internal/encode` | x264 video, AAC audio, MP4 muxing |
| `internal/app` | use cases (check, still, render) composing the above |
| `internal/diag` | source positions and diagnostics |
| `docs/state` | the documentation, one file per feature |
| `skills/reel` | the skill agents load to write scripts; embedded in the binary for `reel skill` |
| `examples` | sample scripts |

New building blocks go in as engine primitives that are generic
(`flow=live`, `set`, `table`, `tree`, `meter`, `badge`, `timer`, `sides=`, `arrow`, connector fan-out, `send … reply|drop`, `ping`, `fail`/`recover`, `isolate`/`return`, `show … from=`, `move`, `send … carry=`, `git`, `status`, dot meters, two-lane `send … reply=` with `spend=`, `set … tone=`, `move … first`, `plot` with `play`/`set`/`note`/`zoom`), with
everyday structures (lists, queues, stacks, balancers, breakers, limiters, caches) written as theme
`define`s on top of them, not as one-off layouts.

## Building

Needs Go (version in `go.mod`), a Rust toolchain (rustup.rs) and x264
(`brew install x264`, or `apt install libx264-dev`).

```sh
make build          # bin/reel: x264 and the Phonon voice linked statically,
                    # the voice model and the Harper voice embedded (~93 MB)
make build BUNDLE=0 # leave the model out; it downloads on first narrated render
make dev            # quick build, x264 linked dynamically, no local voice
make test           # go test ./...
make test-phonon    # also runs the local voice against the cached model
```

`make bundle` fetches the embedded model files into
`internal/voice/phonon/bundle/` (git-ignored) from pinned Hugging Face
revisions. `VERSION=` sets what `reel version` prints; it defaults to
`git describe`.

## Narration

- `voice phonon` (default voice Harper, or `voice phonon NAME`) runs
  Gradium's Phonon 31M model locally. Release builds carry it; the first
  narrated render copies it into `~/Library/Caches/reel/phonon` (macOS).
- `voice elevenlabs VOICE_ID` uses ElevenLabs. The key comes from
  `ELEVENLABS_API_KEY` or a `.env` file (`ELEVENLABS_API_KEY=…`) in the
  script's folder, its parents, or the current folder.
- `voice silent` times a script as if narrated, for tests.
- Spoken lines are cached by voice and text, so re-renders only speak what
  changed. Details: [narrate.md](../state/narrate.md).

## Testing

`go test ./...` must pass and `go vet ./internal/... ./cmd/reel` must be
clean before committing. Layout and motion tests live next to the code
(`internal/stage/live_test.go` shows how to lay out a scene in a test).
For anything visual, also render a short script and look at the frames;
tests don't catch an animation that looks wrong.

## Releasing

1. Commit and push to `main`.
2. Tag and push: `git tag -a v0.1.2 -m "reel v0.1.2" && git push origin v0.1.2`.
3. [The release workflow](../../.github/workflows/release.yml) builds on
   macOS arm64 and Intel and Linux x86-64 and arm64, runs the tests and a
   test render, and publishes `reel-VERSION-OS-ARCH.tar.gz` files with a
   `checksums.txt` as a GitHub release.
4. Its `tap` job writes the formula with `scripts/formula.sh` and pushes it
   to [nessalabs/homebrew-tap](https://github.com/nessalabs/homebrew-tap),
   using the `HOMEBREW_TAP_TOKEN` repository secret (a fine-grained token
   with contents write on that repository only, expiring 2027-11-19).
   Without the secret, the job skips.

Users install with `brew install nessalabs/tap/reel` or
[install.sh](../../install.sh), which picks the right archive, checks it
against `checksums.txt` and installs it into `/usr/local/bin` or
`~/.local/bin`.

## Documentation

`docs/state` follows the [Nessa Docs](https://github.com/nessalabs/nessa-docs)
format: one file per feature, with frontmatter listing its `sources`.
When behaviour changes, update the matching doc, the skill and the language
reference in the same commit. The full design, including parts not built
yet, is in [docs/spec.html](../spec.html).

## Rules

- When a command, element or step changes, update `reel help` (the
  `usage` text in `cmd/reel/main.go`), the skill and the language reference
  together, so the CLI and the docs never disagree.
- Never commit secrets. `.env` is git-ignored; never print a key or token.
- `out/` and `*.mp4` are git-ignored; keep renders there.
- Commit or push only when asked, and say what is and isn't committed.
- Keep the binary self-contained: new dependencies are linked statically
  and listed in THIRD_PARTY_NOTICES.md.
