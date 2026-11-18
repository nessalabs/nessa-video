# reel builds as one binary. `make build` links x264 statically, so the
# result needs nothing installed to run. x264 itself is needed only to build:
#   macOS:  brew install x264
#   Debian: apt install libx264-dev

X264_PREFIX ?= $(shell brew --prefix x264 2>/dev/null || echo /usr)
X264_LIB    ?= $(firstword $(wildcard $(X264_PREFIX)/lib/libx264.a $(X264_PREFIX)/lib/*/libx264.a))

.PHONY: build dev test check-static

build:
	@test -n "$(X264_LIB)" || (echo "libx264.a not found; install x264 or set X264_LIB" && exit 1)
	CGO_ENABLED=1 CGO_LDFLAGS="$(X264_LIB)" go build -tags x264static -trimpath -ldflags="-s -w" -o bin/reel ./cmd/reel
	@$(MAKE) --no-print-directory check-static

# dev links x264 dynamically for faster rebuilds
dev:
	go build -o bin/reel ./cmd/reel

test:
	go test ./...

check-static:
	@if command -v otool >/dev/null; then ! otool -L bin/reel | grep -q x264 || (echo "bin/reel links x264 dynamically" && exit 1); \
	elif command -v ldd >/dev/null; then ! ldd bin/reel | grep -q x264 || (echo "bin/reel links x264 dynamically" && exit 1); fi
	@echo "bin/reel: x264 linked statically ($$(du -h bin/reel | cut -f1))"
