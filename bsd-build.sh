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
target="${outdir}/satpulsetool"

# Build info
build_date=$(date -u +"%Y-%m-%d %H:%M:%S UTC")
git_version=$(env TZ=UTC git log -1 --format="%cd.%h" --date=format-local:%Y%m%d)
if ! git diff-index --quiet HEAD 2>/dev/null; then
    git_version="${git_version}.dirty"
fi

# Create output directory
mkdir -p "$outdir"

# Build
echo "Building satpulsetool for $os/$goarch"
env GOOS=$os GOARCH=$goarch go build -tags "netgo,osusergo" \
    -o "$target" \
    -ldflags "-X \"github.com/jclark/satpulse/internal/cmd.gitVersion=$git_version\" -X \"github.com/jclark/satpulse/internal/cmd.buildDate=$build_date\"" \
    ./cmd/satpulsetool

echo "Built: $target"