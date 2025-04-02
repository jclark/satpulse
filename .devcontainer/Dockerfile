FROM mcr.microsoft.com/devcontainers/go:1-1.23-bookworm

# Install gh (for doing releases) and rpm (for building rpms)
RUN apt-get update \
  && export DEBIAN_FRONTEND=noninteractive \
  && apt-get -y install gh rpm \
  && apt-get clean

# Cache the go modules in the docker image
WORKDIR /go-mod
# This relies on context: ".."  in devcontainer.json
COPY go.mod go.sum ./
USER vscode
RUN go mod download
WORKDIR /workspace/satpulse
