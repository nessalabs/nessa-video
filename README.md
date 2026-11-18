# reel

reel turns short text scripts into explainer videos for software, starting
with code-review videos. An agent writes a `.reel` script; reel checks it and
renders it. It is one binary with no runtime dependencies.

Status: `reel check`, `reel still`, `reel render` and `reel ref` work.
Code, diffs, arrays, charts, timelines, Mermaid flowcharts and maths are all
drawn and can be animated.

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
