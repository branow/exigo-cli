#!/bin/sh
# Install the latest exigo release binary.
#   curl -fsSL https://raw.githubusercontent.com/branow/exigo-cli/main/scripts/install.sh | sh
# Environment:
#   EXIGO_INSTALL_DIR  target directory (default: /usr/local/bin, falls back
#                      to ~/.local/bin when /usr/local/bin is not writable)
#   EXIGO_VERSION      version to install, e.g. v0.1.0 (default: latest)
set -eu

REPO="branow/exigo-cli"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux | darwin) ;;
  *) echo "error: unsupported OS: $os (use the Windows zip from GitHub releases)" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "error: unsupported architecture: $arch" >&2; exit 1 ;;
esac

version="${EXIGO_VERSION:-}"
if [ -z "$version" ]; then
  version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    grep -m1 '"tag_name"' | cut -d '"' -f 4)
fi
[ -n "$version" ] || { echo "error: could not resolve the latest version" >&2; exit 1; }

archive="exigo_${version#v}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$version/$archive"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $url"
curl -fsSL "$url" -o "$tmp/$archive"

curl -fsSL "https://github.com/$REPO/releases/download/$version/checksums.txt" -o "$tmp/checksums.txt"
(cd "$tmp" && grep " $archive\$" checksums.txt | sha256sum -c - >/dev/null 2>&1) ||
  (cd "$tmp" && grep " $archive\$" checksums.txt | shasum -a 256 -c - >/dev/null) ||
  { echo "error: checksum verification failed" >&2; exit 1; }

tar -xzf "$tmp/$archive" -C "$tmp" exigo

dir="${EXIGO_INSTALL_DIR:-/usr/local/bin}"
if [ ! -w "$dir" ] && [ -z "${EXIGO_INSTALL_DIR:-}" ]; then
  dir="$HOME/.local/bin"
  mkdir -p "$dir"
fi

install -m 755 "$tmp/exigo" "$dir/exigo"
echo "Installed exigo $version to $dir/exigo"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "note: $dir is not on your PATH" ;;
esac
