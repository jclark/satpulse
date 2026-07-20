#!/bin/sh

set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT HUP INT TERM

git -C "$repo/configs/gpsmsg" ls-files -- '*/*.toml' |
	(cd "$repo/gps/msgfile" && go run gen.go ../../configs/gpsmsg "$tmp")
if ! cmp -s "$tmp" "$repo/gps/msgfile/gpsmsg.zip"; then
	echo "gps/msgfile/gpsmsg.zip is stale; run: go generate ./gps/msgfile" >&2
	exit 1
fi
