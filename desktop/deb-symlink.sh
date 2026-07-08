#!/bin/sh
# deb-symlink.sh creates the friendly GH_RELEASE-named symlink in build/,
# pointing at the versioned .deb that build-deb.sh produced. Mirrors the symlink
# the main satpulse package makes. Kept separate from build-deb.sh so a
# container build can export just the versioned file.
set -eu

PKG=satpulse-gui
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$here"

set -- $(./deb-version.sh)
ver=$1
ghrel=$2
arch=$(dpkg --print-architecture)

target="${PKG}_${ver}_${arch}.deb"
[ -f "build/$target" ] || { echo "deb-symlink.sh: build/$target not found (run build-deb.sh first)" >&2; exit 1; }

link="${PKG}_${ghrel}_${arch}.deb"
ln -sf "$target" "build/$link"
echo "Symlink build/$link -> $target"
