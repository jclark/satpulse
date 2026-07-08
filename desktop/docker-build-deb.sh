#!/bin/sh
# docker-build-deb.sh builds the desktop .deb inside a container for a given CPU
# architecture and extracts the single versioned .deb to $OUT/<arch>/.
#
# Usage: ./docker-build-deb.sh <amd64|arm64>
#
# Override behaviour with these environment variables (set them before the
# command, e.g. `NO_CACHE=1 ./docker-build-deb.sh arm64`):
#
#   value vars -- take a literal value:
#     BASE_IMAGE    Debian-based distro to build for      (default: debian:trixie)
#     GO_VERSION    Go release line, latest patch resolved (default: Dockerfile's)
#     NODE_VERSION  Node major, latest patch resolved      (default: Dockerfile's)
#     WAILS_VERSION Wails CLI version, e.g. v2.12.0     (default: from desktop/go.mod)
#     OUT           output directory                       (default: out)
#     PROGRESS      buildkit progress, e.g. plain          (default: auto)
#
#   flag vars -- set to 1/true/yes to enable; unset or 0/false to disable:
#     NO_CACHE      --no-cache (re-resolve toolchain, pick up fresh patches)
#     PULL          --pull (refresh the base image)
set -eu

# enabled returns true when its argument is 1/true/yes/on (case-insensitive).
enabled() { case "$(printf %s "${1:-}" | tr A-Z a-z)" in 1|true|yes|on) return 0 ;; *) return 1 ;; esac; }

arch=${1:?usage: docker-build-deb.sh <amd64|arm64>}
: "${BASE_IMAGE:=debian:trixie}"
: "${OUT:=out}"

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel)

# Compute the version here on the host, where git works, and pass it into the
# container. The build context is a git worktree, so its .git is a pointer file
# to a path outside the container; git inside the container therefore fails and
# deb-version.sh cannot derive a version on its own.
deb_version=$("$here/deb-version.sh" | cut -d' ' -f1)
cd "$root"

set -- -f desktop/Dockerfile \
	--platform "linux/$arch" \
	--build-arg "BASE_IMAGE=$BASE_IMAGE" \
	--build-arg "DEB_VERSION=$deb_version" \
	--target export \
	-o "type=local,dest=$OUT/$arch"
if [ -n "${GO_VERSION:-}" ];      then set -- "$@" --build-arg "GO_VERSION=$GO_VERSION"; fi
if [ -n "${NODE_VERSION:-}" ];    then set -- "$@" --build-arg "NODE_VERSION=$NODE_VERSION"; fi
if [ -n "${WAILS_VERSION:-}" ];   then set -- "$@" --build-arg "WAILS_VERSION=$WAILS_VERSION"; fi
if enabled "${NO_CACHE:-}";       then set -- "$@" --no-cache; fi
if enabled "${PULL:-}";           then set -- "$@" --pull; fi
if [ -n "${PROGRESS:-}" ];        then set -- "$@" "--progress=$PROGRESS"; fi

echo "docker build ($arch, $BASE_IMAGE) -> $OUT/$arch/"
docker build "$@" .
ls -l "$OUT/$arch"
