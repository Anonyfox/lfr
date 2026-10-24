#!/bin/sh
# Installs the lfr binary for Linux or macOS (amd64, arm64).
#
#   curl -fsSL https://lfr.email/install.sh | sudo sh
#
# As root it installs to /usr/local/bin (a server); otherwise to ~/.local/bin.
# Settings, all optional:
#   LFR_VERSION       release tag such as v0.1.0 (default: the latest release)
#   LFR_INSTALL_DIR   where the binary goes
#   LFR_RELEASES_URL  base of the release downloads, for mirrors
#                     (default: https://github.com/Anonyfox/lfr/releases)
#
# The archive is checked against the release's checksums.txt. When no release
# can be downloaded and Go is installed, it falls back to go install.
set -eu

releases="${LFR_RELEASES_URL:-https://github.com/Anonyfox/lfr/releases}"
version="${LFR_VERSION:-latest}"
if [ "$(id -u)" = 0 ]; then
	default_dir=/usr/local/bin
else
	default_dir="$HOME/.local/bin"
fi
dir="${LFR_INSTALL_DIR:-$default_dir}"

say() { printf 'lfr install: %s\n' "$*" >&2; }
fail() { say "$*"; exit 1; }

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "no build for $(uname -s); lfr runs on Linux and macOS" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "no build for $(uname -m); lfr runs on amd64 and arm64" ;;
esac

if [ "$version" = latest ]; then
	base="$releases/latest/download"
else
	base="$releases/download/$version"
fi
archive="lfr_${os}_${arch}.tar.gz"

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$1" -O "$2"
	else
		fail "needs curl or wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

go_install() {
	command -v go >/dev/null 2>&1 || return 1
	say "no release download; building with go install instead"
	GOBIN="$dir" go install "github.com/Anonyfox/lfr/cmd/lfr@$version" || return 1
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM
mkdir -p "$dir"

if fetch "$base/$archive" "$tmp/$archive" && fetch "$base/checksums.txt" "$tmp/checksums.txt"; then
	want="$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
	[ -n "$want" ] || fail "checksums.txt has no entry for $archive"
	[ "$(sha256 "$tmp/$archive")" = "$want" ] || fail "checksum mismatch for $archive; nothing installed"
	tar -xzf "$tmp/$archive" -C "$tmp" lfr
	mv "$tmp/lfr" "$dir/lfr.new"
	chmod 755 "$dir/lfr.new"
	mv "$dir/lfr.new" "$dir/lfr"
elif ! go_install; then
	fail "could not download $base/$archive, and Go 1.26 or newer is not available to build it"
fi

say "installed $("$dir/lfr" version) to $dir/lfr"
case ":$PATH:" in
*":$dir:"*) ;;
*) say "$dir is not on PATH; add it, for example: export PATH=\"$dir:\$PATH\"" ;;
esac
if [ -f /etc/systemd/system/lfr.service ]; then
	say "next: sudo systemctl restart lfr (the server runs the old version until then)"
else
	say "next: sudo lfr setup --hostname mail.<your domain> --systemd"
fi
