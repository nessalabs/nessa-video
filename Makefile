# reel builds as one binary. `make build` links x264 and the local Phonon
# voice statically, so the result needs nothing installed to run. Building
# needs x264 and a Rust toolchain (rustup.rs):
#   macOS:  brew install x264
#   Debian: apt install libx264-dev

X264_PREFIX ?= $(shell brew --prefix x264 2>/dev/null || echo /usr)
X264_LIB    ?= $(firstword $(wildcard $(X264_PREFIX)/lib/libx264.a $(X264_PREFIX)/lib/*/libx264.a))

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

PHONON_DIR := internal/voice/phonon/native
PHONON_LIB := $(CURDIR)/$(PHONON_DIR)/target/release/libreel_phonon.a

# BUNDLE=1 (the default) puts the Phonon checkpoint and the Harper voice
# inside the binary, so narration works offline from the first run.
BUNDLE        ?= 1
PHONON_BUNDLE := internal/voice/phonon/bundle
HF            := https://huggingface.co
MODEL_REV     := 92741c5b07d085ff840e26fea8ce43110b87852f
VOICES_REV    := da6c9ec0d47be8df5c87c730a3aa97187f3229c3
BUNDLE_FILES  := config.json tokenizer.json model.q8.gguf default-voice.safetensors
TAGS          := phonon$(if $(filter 1,$(BUNDLE)), phononbundle)

.PHONY: build dev test test-phonon check-static phonon bundle

# phonon builds the Rust wrapper around the Phonon runtime as a static library
phonon:
	@command -v cargo >/dev/null || (echo "cargo not found; install Rust from rustup.rs" && exit 1)
	cargo build --release --manifest-path $(PHONON_DIR)/Cargo.toml

# bundle fetches what the binary carries, once, from Hugging Face
bundle:
	@mkdir -p $(PHONON_BUNDLE)/voices
	@for f in $(BUNDLE_FILES); do test -s $(PHONON_BUNDLE)/$$f || \
	  curl -fsSL -o $(PHONON_BUNDLE)/$$f $(HF)/gradium/phonon-31m-en/resolve/$(MODEL_REV)/$$f || exit 1; done
	@test -s $(PHONON_BUNDLE)/voices/Harper.safetensors || \
	  curl -fsSL -o $(PHONON_BUNDLE)/voices/Harper.safetensors \
	  $(HF)/gradium/phonon-voices/resolve/$(VOICES_REV)/$$(curl -fsSL $(HF)/gradium/phonon-voices/resolve/$(VOICES_REV)/voices.jsonl | grep '"name": *"Harper"' | sed -n 's/.*"filename": *"\([^"]*\)".*/\1/p')

build: phonon $(if $(filter 1,$(BUNDLE)),bundle)
	@test -n "$(X264_LIB)" || (echo "libx264.a not found; install x264 or set X264_LIB" && exit 1)
	CGO_ENABLED=1 CGO_LDFLAGS="$(X264_LIB) $(PHONON_LIB)" go build -tags "x264static $(TAGS)" -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/reel ./cmd/reel
	@$(MAKE) --no-print-directory check-static

# dev links x264 dynamically for faster rebuilds
dev:
	go build -o bin/reel ./cmd/reel

test:
	go test ./...

# test-phonon also runs the local voice against the real model, if cached
test-phonon: phonon
	CGO_LDFLAGS="$(PHONON_LIB)" go test -tags phonon ./internal/voice/phonon/

check-static:
	@if command -v otool >/dev/null; then ! otool -L bin/reel | grep -q x264 || (echo "bin/reel links x264 dynamically" && exit 1); \
	elif command -v ldd >/dev/null; then ! ldd bin/reel | grep -q x264 || (echo "bin/reel links x264 dynamically" && exit 1); fi
	@echo "bin/reel: x264 linked statically ($$(du -h bin/reel | cut -f1))"
