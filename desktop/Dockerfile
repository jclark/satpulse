# syntax=docker/dockerfile:1

# Reproducible build environment for the SatPulse desktop .deb. Two knobs:
#
#   BASE_IMAGE  which Debian-based distro to build for. This decides which
#               distro's library versions dpkg-shlibdeps records as the deb's
#               runtime dependencies, e.g.
#                 --build-arg BASE_IMAGE=ubuntu:24.04
#                 --build-arg BASE_IMAGE=debian:trixie
#
#   --platform  which CPU architecture, e.g. linux/arm64 (Raspberry Pi, Apple
#               Silicon) or linux/amd64 (Intel). On an arm64 Mac, arm64 builds
#               run natively and amd64 builds run under emulation.
#
# GO_VERSION / NODE_VERSION name the release line (Go, e.g. 1.25) or major
# (Node, e.g. 22); the latest patch on it is resolved from the vendor index at
# build time. A full version (e.g. 1.25.4) pins exactly. Because Docker caches
# this layer, rebuild with --no-cache to pick up a newer patch on the same line.
# The toolchain is fetched from upstream so it is new enough and identical
# regardless of base image; only the linked system libraries come from BASE_IMAGE.
ARG BASE_IMAGE=debian:trixie

FROM ${BASE_IMAGE} AS build
ARG TARGETARCH
ARG GO_VERSION=1.25
ARG NODE_VERSION=22

# Build tools + GTK/WebKitGTK headers from the chosen base distro.
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      build-essential debhelper pkg-config git ca-certificates curl xz-utils jq \
      libgtk-3-dev libwebkit2gtk-4.1-dev \
 && rm -rf /var/lib/apt/lists/*

# Go: resolve the latest patch on the GO_VERSION line, then install it.
# TARGETARCH is amd64/arm64, matching Go's naming.
RUN gv=$(curl -fsSL 'https://go.dev/dl/?mode=json&include=all' \
        | jq -r --arg v "$GO_VERSION" \
            'map(select(.stable and ((.version=="go"+$v) or (.version|startswith("go"+$v+".")))))|.[0].version') \
 && test -n "$gv" || { echo "no Go release matching $GO_VERSION" >&2; exit 1; } \
 && echo "Using Go $gv" \
 && curl -fsSL "https://go.dev/dl/${gv}.linux-${TARGETARCH}.tar.gz" | tar -C /usr/local -xz

# Node: resolve the latest release on the NODE_VERSION major, then install it.
# Node names archs x64/arm64.
RUN na=$(case "$TARGETARCH" in amd64) echo x64 ;; arm64) echo arm64 ;; *) echo "$TARGETARCH" ;; esac) \
 && nv=$(curl -fsSL https://nodejs.org/dist/index.json \
        | jq -r --arg v "$NODE_VERSION" \
            'map(select((.version=="v"+$v) or (.version|startswith("v"+$v+"."))))|.[0].version') \
 && test -n "$nv" || { echo "no Node release matching $NODE_VERSION" >&2; exit 1; } \
 && echo "Using Node $nv" \
 && curl -fsSL "https://nodejs.org/dist/${nv}/node-${nv}-linux-${na}.tar.xz" | tar -C /usr/local --strip-components=1 -xJ

ENV PATH=/usr/local/go/bin:/root/go/bin:$PATH

# Wails CLI. By default install the exact version the project depends on, read
# from desktop/go.mod with Go's own parser, so the CLI and library stay in sync
# (a mismatch can break binding/runtime generation). Installing by @version (not
# from the module) avoids missing-go.sum-entry errors for the CLI's own deps.
# Override with --build-arg WAILS_VERSION=vX.Y.Z. Only go.mod is copied here, so
# this layer is cached until the dependency changes.
ARG WAILS_VERSION=
COPY desktop/go.mod /tmp/go.mod
RUN wv="${WAILS_VERSION:-$(go mod edit -json /tmp/go.mod | jq -r '.Require[]|select(.Path=="github.com/wailsapp/wails/v2").Version')}" \
 && test -n "$wv" || { echo "could not determine Wails version" >&2; exit 1; } \
 && echo "Using Wails $wv" \
 && go install github.com/wailsapp/wails/v2/cmd/wails@"$wv"

# The desktop module depends on the parent module (replace => ../), so the build
# context is the repo root. build-deb.sh builds the app and packages it; it does
# NOT create the friendly symlink, so the export stage holds a single .deb.
# DEB_VERSION is computed on the host (where git works) and passed in, because
# the build context is a git worktree whose .git points outside the container,
# so deb-version.sh cannot derive the version here. See docker-build-deb.sh.
ARG DEB_VERSION
WORKDIR /src
COPY . .
RUN test -n "$DEB_VERSION" || { echo "DEB_VERSION build-arg is required" >&2; exit 1; } \
 && cd desktop && ./build-deb.sh "$DEB_VERSION"

# Final stage holds only the .deb artifacts, so
#   --output type=local,dest=out
# drops them straight onto the host (under an arch subdir for multi-platform).
FROM scratch AS export
COPY --from=build /src/desktop/build/*.deb /
