FROM mcr.microsoft.com/devcontainers/go:1-1.22-bookworm
RUN apt-get update \
  && export DEBIAN_FRONTEND=noninteractive \
  && apt-get -y install gh rpm \
  && apt-get clean
