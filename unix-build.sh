#!/bin/sh
# Build satpulse on unix-like systems (BSD, Darwin, Linux).
# For 64-bit Linux (amd64/arm64), the Makefile is preferred.

set -e

# Detect or use provided GOOS
if [ -z "$GOOS" ]; then
    GOOS=$(uname -s | tr '[:upper:]' '[:lower:]')
    export GOOS
fi

# Validate GOOS
case $GOOS in
    darwin|freebsd|linux) ;;
    *) echo "Error: Unsupported OS $GOOS"; exit 1 ;;
esac


# Detect or use provided GOARCH
if [ -z "$GOARCH" ]; then
    GOARCH=$(uname -m)
fi

# Normalize / validate GOARCH
case $GOARCH in
    amd64|arm64|arm|386) ;;
    x86_64)  GOARCH=amd64 ;;
    aarch64) GOARCH=arm64 ;;
    armv6l|armv7l) GOARCH=arm ;;
    i386|i486|i586|i686) GOARCH=386 ;;
    *) echo "Error: Unsupported architecture $GOARCH"; exit 1 ;;
esac
export GOARCH

# Commands and output directory
cmddirs="cmd/satpulsed cmd/satpulsetool cmd/pollpps"
targets=""
cmds=""
for d in $cmddirs; do
    targets="$targets ./$d"
    cmds="$cmds $(basename $d)"
done
if [ "$GOOS" = linux ]; then
    outdir="out/${GOARCH}"
else
    outdir="out/${GOOS}_${GOARCH}"
fi

# Build info
version=$(cat VERSION)
build_date=$(date -u -Iseconds | tr 'T' ' ')
git_date=$(env TZ=UTC git log -1 --format="%cd" --date=format-local:%Y%m%d)
git_hash=$(git log -1 --format="%h")
cmd_version="${version}-pre.${git_date}.${git_hash}"
if ! git diff-index --quiet HEAD 2>/dev/null; then
    cmd_version="${cmd_version}.dirty"
elif [ "$(git describe --tags --exact-match 2>/dev/null)" = "v${version}" ]; then
    cmd_version="${version}"
fi

# Create output directory
mkdir -p "$outdir"

# Build
go build -tags "netgo,osusergo" \
    -o "$outdir" \
    -ldflags "-X \"github.com/jclark/satpulse/gps/app/cmd.version=$cmd_version\" -X \"github.com/jclark/satpulse/gps/app/cmd.buildDate=$build_date\"" \
    $targets

echo "Built $cmds in $outdir"