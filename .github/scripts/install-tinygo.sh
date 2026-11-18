#!/bin/sh
# Install the pinned TinyGo release into /usr/local and put it on PATH.
# Used by the Build (TinyGo) workflow, which sets TINYGO_VERSION.
set -e

url="https://github.com/tinygo-org/tinygo/releases/download/v$TINYGO_VERSION/tinygo$TINYGO_VERSION.linux-amd64.tar.gz"
curl -fsSL -o /tmp/tinygo.tar.gz "$url"
sudo tar -C /usr/local -xzf /tmp/tinygo.tar.gz
echo /usr/local/tinygo/bin >> "$GITHUB_PATH"
