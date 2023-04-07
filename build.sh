#!/bin/bash
set -e
ARCH="arm64 amd64"
. ./vars.sh
XFLAGS="-X \"main.configFile=$CONFIG_FILE\" -X \"main.gitVersion=$(git describe --tags --always --dirty=-modified)\" -X \"main.buildDate=$(date -u --rfc-3339=seconds)\""
for arch in $ARCH; do
    env GOOS=linux GOARCH=$arch go build -o out/$arch/ -ldflags "$XFLAGS" ./... 
done
