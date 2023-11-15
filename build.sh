#!/bin/bash
set -e
ARCH="arm64 amd64"
. ./vars.sh
PKG="github.com/jclark/satpulse/internal/cmd"
XFLAGS="-X \"main.configFile=$CONFIG_FILE\" -X \"$PKG.gitVersion=$(git describe --tags --always --dirty=-modified)\" -X \"$PKG.buildDate=$(date -u --rfc-3339=seconds)\""
for arch in $ARCH; do
    env GOOS=linux GOARCH=$arch go build -o out/$arch/ -ldflags "$XFLAGS" ./... 
done
