#!/bin/bash
set -e
ARCH="arm64 amd64"
XFLAGS="-X main.gitVersion=$(git describe --tags --always --dirty=-modified) -X main.buildDate=$(date -u --iso-8601=seconds)"
for arch in $ARCH; do
    env GOOS=linux GOARCH=$arch go build -o out/$arch/ -ldflags "$XFLAGS"  ./... 
done
