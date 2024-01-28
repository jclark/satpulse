#!/bin/bash
set -e
ARCH="arm64 amd64"
. ./vars.sh
CMD="github.com/jclark/satpulse/internal/cmd"
# these tags produce a static binary
TAGS="netgo,osusergo" 
XFLAGS="-X \"$CMD/daemon.defaultConfigFile=$CONFIG_FILE\" -X \"$CMD.gitVersion=$(git describe --tags --always --dirty=-modified)\" -X \"$CMD.buildDate=$(date -u --rfc-3339=seconds)\""
for arch in $ARCH; do
    env GOOS=linux GOARCH=$arch go build -tags "$TAGS" -o out/$arch/ -ldflags "$XFLAGS" ./... 
done
