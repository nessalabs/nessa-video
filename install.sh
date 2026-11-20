#!/bin/sh
# Installs the latest reel release (or REEL_VERSION=v0.1.0) for this
# machine into REEL_DIR, default /usr/local/bin, or ~/.local/bin when that
# is not writable.
#   curl -fsSL https://raw.githubusercontent.com/nessalabs/nessa-video/main/install.sh | sh
set -eu

repo=nessalabs/nessa-video
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "reel: no build for $(uname -s); build from source with make build" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64) arch=amd64 ;;
  *) echo "reel: no build for $(uname -m)" >&2; exit 1 ;;
esac

version=${REEL_VERSION:-}
if [ -z "$version" ]; then
  version=$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" |
    sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
fi
[ -n "$version" ] || { echo "reel: could not find the latest release" >&2; exit 1; }

name="reel-$version-$os-$arch"
base="https://github.com/$repo/releases/download/$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "Downloading reel $version for $os-$arch"
curl -fsSL -o "$tmp/$name.tar.gz" "$base/$name.tar.gz"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"

want=$(grep " $name.tar.gz\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null; then got=$(sha256sum "$tmp/$name.tar.gz" | cut -d' ' -f1)
else got=$(shasum -a 256 "$tmp/$name.tar.gz" | cut -d' ' -f1); fi
[ -n "$want" ] && [ "$want" = "$got" ] || { echo "reel: checksum mismatch" >&2; exit 1; }

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
dir=${REEL_DIR:-/usr/local/bin}
if [ -z "${REEL_DIR:-}" ] && [ ! -w "$dir" ]; then dir="$HOME/.local/bin"; fi
mkdir -p "$dir"
install -m 755 "$tmp/$name/reel" "$dir/reel"
echo "Installed $dir/reel"
case ":$PATH:" in *":$dir:"*) ;; *) echo "Add $dir to your PATH to run reel." ;; esac
