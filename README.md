# reel

reel turns short text scripts into explainer videos for software, starting
with code-review videos. An agent writes a `.reel` script; reel checks it and
renders it. It is one binary with no runtime dependencies.

Status: `reel check`, `reel still`, `reel render` and `reel ref` work.
Code, diffs, arrays, charts, timelines, Mermaid flowcharts and maths are all
drawn and can be animated.

## Install

On macOS (Apple silicon or Intel) or Linux (x86-64 or arm64):

```sh
curl -fsSL https://raw.githubusercontent.com/nessalabs/nessa-video/main/install.sh | sh
```

It downloads the latest [release](https://github.com/nessalabs/nessa-video/releases),
checks it against the release's checksums and installs `reel` into
`/usr/local/bin` (or `~/.local/bin`). `REEL_VERSION=v0.1.0` picks a version
and `REEL_DIR=…` a folder. Or download a `.tar.gz` from the releases page;
on macOS, a binary downloaded in a browser needs
`xattr -d com.apple.quarantine reel` before it runs. The binary carries a
local voice (`voice phonon`), so narration works offline with no account.

Releases are built by [the release workflow](.github/workflows/release.yml)
when a `v*` tag is pushed.

## Build from source

```sh
make build                      # links x264 statically; needs x264 to build only
./bin/reel render examples/cache-refresh.reel
./bin/reel render examples/code-review.reel
./bin/reel render examples/data-blocks.reel
./bin/reel check examples/cache-refresh.reel
./bin/reel check --explain '#p3' examples/cache-refresh.reel
./bin/reel still -o out examples/cache-refresh.reel
go test ./...
```

Narration: add `voice elevenlabs VOICE_ID` to a script and put
`ELEVENLABS_API_KEY=…` in a local `.env` (never committed). Every `say` line
is spoken and the MP4 gets an audio track; `--mute` skips it.

Agents can learn the format from the skill in [skills/reel](skills/reel/SKILL.md).
The same script renders for 16:9 and 9:16 by changing its `size`.

Documentation lives in [docs/state](docs/state/README.md) in the
[Nessa Docs](https://github.com/nessalabs/nessa-docs) format. The full design,
including parts not built yet, is in [docs/spec.html](docs/spec.html).

reel is licensed under the GNU General Public License, version 2 or later. It
links x264, which uses the same licence. Bundled fonts and libraries are
listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
