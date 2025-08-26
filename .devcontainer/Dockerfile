FROM mcr.microsoft.com/devcontainers/go:1-1.23-bookworm

# Install gh (for doing releases) and rpm (for building rpms) and pandoc (for manpages)
RUN apt-get update \
  && export DEBIAN_FRONTEND=noninteractive \
  && apt-get install -y --no-install-recommends gh rpm pandoc ripgrep \
  && apt-get clean

# Cache the go modules in the docker image
WORKDIR /go-mod
# This relies on context: ".."  in devcontainer.json
COPY go.mod go.sum ./
USER vscode
RUN go mod download
WORKDIR /workspace/satpulse
