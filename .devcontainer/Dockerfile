FROM mcr.microsoft.com/devcontainers/go:2-1.25-trixie

# Install gh (for doing releases) and rpm (for building rpms) and pandoc (for manpages)
RUN apt-get update \
  && export DEBIAN_FRONTEND=noninteractive \
  && apt-get install -y --no-install-recommends gh rpm pandoc ripgrep \
  && apt-get clean \
  && rpm --initdb

# Cache the go modules in the docker image
WORKDIR /go-mod
# This relies on context: ".."  in devcontainer.json
COPY go.mod go.sum ./
USER vscode
RUN go mod download

# Cache the npm dependencies in the docker image, the same way as the go
# modules above. Copy just the workspace manifests and lockfile (plus the
# @satpulse/gps file: dependency target) and run npm ci.
WORKDIR /npm-mod
COPY --chown=vscode:vscode webui/package.json webui/package-lock.json webui/
COPY --chown=vscode:vscode webui/packages/dashboard/package.json webui/packages/dashboard/
COPY --chown=vscode:vscode gps/ts/package.json gps/ts/
RUN cd webui && npm ci

WORKDIR /workspace/satpulse
