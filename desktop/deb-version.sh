#!/bin/sh
# deb-version.sh prints two space-separated tokens: the .deb version and the
# friendly GH_RELEASE name, using the same convention as the main satpulse .deb.
#   Released (HEAD on the exact vVERSION tag, clean tree): "VERSION-1  VERSION"
#   Otherwise a pre-release: "VERSION~git<date>.<hash>[.dirty]-1  VERSION-pre-<date>"
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel 2>/dev/null || echo "$here/..")
VERSION=$(cat "$root/VERSION")
DEB_REV=1

if git -C "$here" diff-index --quiet HEAD 2>/dev/null; then dirty=; else dirty=.dirty; fi
if [ "$(git -C "$here" describe --tags --exact-match 2>/dev/null || true)$dirty" = "v$VERSION" ]; then
	echo "$VERSION-$DEB_REV $VERSION"
else
	d=$(env TZ=UTC git -C "$here" log -1 --format="%cd" --date=format-local:%Y%m%d)
	echo "$VERSION~git$d.$(git -C "$here" log -1 --format=%h)$dirty-$DEB_REV $VERSION-pre-$d"
fi
