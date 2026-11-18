# reel

reel turns short text scripts into explainer videos for software, starting
with code-review videos. An agent writes a `.reel` script; reel checks it and
renders it. It is one binary with no runtime dependencies.

Status: `reel check`, `reel still` and `reel ref` work. Animation and video
output are being built.

```sh
go build -o bin/reel ./cmd/reel
./bin/reel check examples/cache-refresh.reel
./bin/reel check --explain '#p3' examples/cache-refresh.reel
./bin/reel still -o out examples/cache-refresh.reel
go test ./...
```

Documentation lives in [docs/state](docs/state/README.md) in the
[Nessa Docs](https://github.com/nessalabs/nessa-docs) format. The full design,
including parts not built yet, is in [docs/spec.html](docs/spec.html).

reel is licensed under the GNU General Public License, version 2 or later. It
will link x264, which uses the same licence. Bundled fonts and libraries are
listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
