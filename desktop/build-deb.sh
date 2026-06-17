#!/bin/sh
# build-deb.sh builds a .deb of the SatPulse desktop GUI for the Debian-based
# distribution and architecture it is run on (Debian trixie and derivatives).
# Unlike the main repo Makefile (which hand-assembles a tree and calls
# dpkg-deb), this uses the standard debhelper / dpkg-buildpackage flow, so
# runtime dependencies -- notably the WebKitGTK 4.1 stack -- are computed
# automatically by dpkg-shlibdeps.
#
# Usage: ./build-deb.sh [VERSION]
#   VERSION defaults to the same scheme as the main satpulse .deb (see below).
#   The .deb is written to build/.
set -eu

PKG=satpulse-gui
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$here"

# This is a Debian packaging script; refuse to run anywhere else.
command -v dpkg-buildpackage >/dev/null 2>&1 || {
	echo "build-deb.sh: needs a Debian-based system with dpkg-buildpackage." >&2
	echo "Install: sudo apt install build-essential debhelper" >&2
	exit 1
}

# Build the app if it has not been built yet (needs the wails toolchain on PATH).
if [ ! -x build/bin/SatPulse ]; then
	PATH="$PATH:$(go env GOPATH 2>/dev/null)/bin"
	export PATH
	[ -f ../go.work ] || make setup
	make
fi
[ -x build/bin/SatPulse ] || { echo "build-deb.sh: build/bin/SatPulse missing" >&2; exit 1; }

# Version: same convention as the main satpulse .deb (Makefile DEB_PKG_VERSION).
# Released (HEAD on the exact vVERSION tag, clean tree): VERSION-1.
# Otherwise a pre-release:                       VERSION~git<date>.<hash>[.dirty]-1.
# ghrel is the friendly GH_RELEASE name used for the symlink (empty if VERSION
# was overridden on the command line).
ver=${1:-}
ghrel=
if [ -z "$ver" ]; then
	root=$(git rev-parse --show-toplevel 2>/dev/null || echo "$here/..")
	VERSION=$(cat "$root/VERSION")
	DEB_REV=1
	if git diff-index --quiet HEAD 2>/dev/null; then dirty=; else dirty=.dirty; fi
	if [ "$(git describe --tags --exact-match 2>/dev/null || true)$dirty" = "v$VERSION" ]; then
		ver="$VERSION-$DEB_REV"
		ghrel="$VERSION"
	else
		date_ymd=$(env TZ=UTC git log -1 --format="%cd" --date=format-local:%Y%m%d)
		ver="$VERSION~git$date_ymd.$(git log -1 --format=%h)$dirty-$DEB_REV"
		ghrel="$VERSION-pre-$date_ymd"
	fi
fi

arch=$(dpkg --print-architecture)
dist=$(. /etc/os-release && echo "${VERSION_CODENAME:-stable}")
maint=$(git config user.name 2>/dev/null || echo SatPulse)
email=$(git config user.email 2>/dev/null || echo "satpulse@localhost")
date=$(date -R)

# Assemble a minimal source tree in a scratch dir and build it there. The files
# are staged under their installed command name so debian/install places them
# without any renaming.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
src="$work/$PKG-$ver"
mkdir -p "$src/debian/source"

cp build/bin/SatPulse "$src/$PKG"
cp build/appicon.png "$src/$PKG.png"

cat >"$src/$PKG.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=SatPulse
GenericName=GPS Receiver Tool
Comment=Configure and monitor GPS receivers
Exec=$PKG
Icon=$PKG
Terminal=false
Categories=Utility;Science;
EOF

cat >"$src/debian/control" <<EOF
Source: $PKG
Section: comm
Priority: optional
Maintainer: $maint <$email>
Build-Depends: debhelper-compat (= 13)
Standards-Version: 4.6.2
Rules-Requires-Root: no

Package: $PKG
Architecture: any
Depends: \${shlibs:Depends}, \${misc:Depends}
Description: SatPulse GPS receiver desktop GUI
 Desktop application for configuring and monitoring GPS receivers
 over a serial connection.
EOF

cat >"$src/debian/changelog" <<EOF
$PKG ($ver) $dist; urgency=medium

  * Build of the SatPulse desktop GUI.

 -- $maint <$email>  $date
EOF

cat >"$src/debian/install" <<EOF
$PKG usr/bin
$PKG.desktop usr/share/applications
$PKG.png usr/share/pixmaps
EOF

# Nothing to compile here -- the binary is prebuilt -- so the dh build/test
# steps are no-ops; dh_install places the staged files.
cat >"$src/debian/rules" <<'EOF'
#!/usr/bin/make -f
%:
	dh $@

override_dh_auto_build:
override_dh_auto_test:
EOF
chmod +x "$src/debian/rules"
echo '3.0 (quilt)' >"$src/debian/source/format"

( cd "$src" && dpkg-buildpackage -b -us -uc )

mkdir -p build
deb=$(ls "$work/${PKG}_${ver}_${arch}.deb")
cp "$deb" build/
echo
echo "Built build/$(basename "$deb")"

# Friendly symlink with the GH_RELEASE name, mirroring the main package.
if [ -n "$ghrel" ]; then
	pretty="${PKG}_${ghrel}_${arch}.deb"
	ln -sf "$(basename "$deb")" "build/$pretty"
	echo "Symlink build/$pretty -> $(basename "$deb")"
fi
