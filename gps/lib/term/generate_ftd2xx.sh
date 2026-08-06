#!/bin/sh

set -eu

: "${FTD2XX_DIR:?set FTD2XX_DIR to the directory containing ftd2xx.h and WinTypes.h}"

output=zftd2xx_defs_darwin.go
tmp=$output.tmp
trap 'rm -f "$tmp" _cgo_2.o' EXIT

go tool cgo -godefs -- -I"$FTD2XX_DIR" ftd2xx_defs.go > "$tmp"
sed -i '' '2d' "$tmp"
gofmt -w "$tmp"
mv "$tmp" "$output"
