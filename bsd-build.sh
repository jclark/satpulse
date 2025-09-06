#!/bin/sh
# Build satpulsetool for BSD/Darwin systems

set -e

# Detect OS and architecture
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)

# Map architecture names to Go architecture names
case $arch in
    x86_64) goarch=amd64 ;;
    aarch64|arm64) goarch=arm64 ;;
    *) echo "Error: Unsupported architecture $arch"; exit 1 ;;
esac

# Output directory
outdir="out/${os}_${goarch}"
targets="./cmd/satpulsed ./cmd/satpulsetool ./cmd/ubxanno ./cmd/pollpps"

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
echo "Building $targets for $os/$goarch"
env GOOS=$os GOARCH=$goarch go build -tags "netgo,osusergo" \
    -o "$outdir" \
    -ldflags "-X \"github.com/jclark/satpulse/internal/cmd.version=$cmd_version\" -X \"github.com/jclark/satpulse/internal/cmd.buildDate=$build_date\"" \
    $targets

echo "Built: $targets"