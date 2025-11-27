#!/bin/sh
# Build satpulsetool for BSD/Darwin systems

set -e

# Detect or use provided GOOS
if [ -z "$GOOS" ]; then
    GOOS=$(uname -s | tr '[:upper:]' '[:lower:]')
    export GOOS
fi

# Validate GOOS
case $GOOS in
    darwin|freebsd) ;;
    linux) echo Use Makefile on Linux 1>&2; exit 1;;
    *) echo "Error: Unsupported OS $GOOS"; exit 1 ;;
esac


# Detect or use provided GOARCH
if [ -z "$GOARCH" ]; then
    GOARCH=$(uname -m)
    export GOARCH
fi

# Validate GOARCH
case $GOARCH in
    amd64|arm64) ;;
    x86_64) GOARCH=amd64 ;;
    aarch64) GOARCH=arm64 ;;
    *) echo "Error: Unsupported architecture $GOARCH"; exit 1 ;;
esac

# Commands and output directory
cmddirs="cmd/satpulsed cmd/satpulsetool cmd/ubxanno cmd/pollpps internal/syncsim/cmd/syncsim internal/syncsim/cmd/tsgen"
targets=""
cmds=""
for d in $cmddirs; do
    targets="$targets ./$d"
    cmds="$cmds $(basename $d)"
done
outdir="out/${GOOS}_${GOARCH}"

# Build info
version=$(cat VERSION)
build_date=$(date -u -Iseconds | tr 'T' ' ')
git_date=$(env TZ=UTC git log -1 --format="%cd" --date=format-local:%Y%m%d)
git_hash=$(git log -1 --format="%h")
cmd_version="${version}-pre.${git_date}.${git_hash}"
if ! git diff-index --quiet HEAD 2>/dev/null; then
    cmd_version="${cmd_version}.dirty"
fi

# Create output directory
mkdir -p "$outdir"

# Build
go build -tags "netgo,osusergo" \
    -o "$outdir" \
    -ldflags "-X \"github.com/jclark/satpulse/internal/cmd.version=$cmd_version\" -X \"github.com/jclark/satpulse/internal/cmd.buildDate=$build_date\"" \
    $targets

echo "Built $cmds in $outdir"