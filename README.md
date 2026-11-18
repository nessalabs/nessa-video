# reel

reel turns short text scripts into explainer videos for software, starting
with code-review videos. An agent writes a `.reel` script; reel checks it and
renders it. It is one binary with no runtime dependencies.

Status: `reel check`, `reel still`, `reel render` and `reel ref` work.
Code and diff blocks have syntax colours and focus. Arrays, charts,
timelines, Mermaid and maths are drawn as placeholders for now.

```sh
make build                      # links x264 statically; needs x264 to build only
./bin/reel render examples/cache-refresh.reel
./bin/reel render examples/code-review.reel
./bin/reel check examples/cache-refresh.reel
./bin/reel check --explain '#p3' examples/cache-refresh.reel
./bin/reel still -o out examples/cache-refresh.reel
go test ./...
```

Documentation lives in [docs/state](docs/state/README.md) in the
[Nessa Docs](https://github.com/nessalabs/nessa-docs) format. The full design,
including parts not built yet, is in [docs/spec.html](docs/spec.html).

reel is licensed under the GNU General Public License, version 2 or later. It
links x264, which uses the same licence. Bundled fonts and libraries are
listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
