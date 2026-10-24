#!/bin/sh
# Tests install.sh against a local release: run as root it installs a
# root-owned binary even when the archive says uid 1001, and without root it
# warns when an older lfr comes first on PATH.
#
#   go build -o lfr ./cmd/lfr && sh scripts/install-test.sh ./lfr
#
# Needs GNU tar and passwordless sudo (as on GitHub's Linux runners).
set -eu

bin="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
repo="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'sudo rm -rf "$work"' EXIT INT TERM
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
*) arch=arm64 ;;
esac
archive="lfr_linux_${arch}.tar.gz"

mkdir -p "$work/release/download/v0.0.0" "$work/stage"
cp "$bin" "$work/stage/lfr"
tar --owner=1001 --group=1001 --numeric-owner -czf "$work/release/download/v0.0.0/$archive" -C "$work/stage" lfr
(cd "$work/release/download/v0.0.0" && sha256sum "$archive" > checksums.txt)
run() {
	env LFR_RELEASES_URL="file://$work/release" LFR_VERSION=v0.0.0 LFR_INSTALL_DIR="$1" sh "$repo/install.sh"
}

sudo env PATH="$PATH" LFR_RELEASES_URL="file://$work/release" LFR_VERSION=v0.0.0 LFR_INSTALL_DIR="$work/root" sh "$repo/install.sh"
owner="$(stat -c %u:%g "$work/root/lfr")"
[ "$owner" = 0:0 ] || { echo "root install left the binary owned by $owner"; exit 1; }

mkdir -p "$work/old"
printf '#!/bin/sh\necho "lfr 0.0.1"\n' > "$work/old/lfr"
chmod 755 "$work/old/lfr"
out="$(PATH="$work/old:$work/user:$PATH" run "$work/user" 2>&1)"
echo "$out"
echo "$out" | grep -q "warning: the shell runs $work/old/lfr (lfr 0.0.1)" || { echo "no warning about the older lfr on PATH"; exit 1; }
echo "$out" | grep -q "lfr setup" && { echo "a user install suggested lfr setup"; exit 1; }
echo "install.sh ok"
