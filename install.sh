#!/bin/bash
set -e
arch=$(uname -m)

case "$arch" in
    x86_64)
        goarch="amd64"
        ;;
    aarch64)
        goarch="arm64"
        ;;
    *)
        echo "Unknown architecture" >&2
        exit 1
        ;;
esac

cp out/$goarch/gps4ptp /usr/local/sbin/gps4ptp
cp gps4ptp.service /etc/systemd/system/
# Don't overrwrite existing config file
[ -f /usr/local/etc/gps4ptp.toml ] || cp cmd/gps4ptp/gps4ptp.toml /usr/local/etc/gps4ptp.toml
systemctl daemon-reload
