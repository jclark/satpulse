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
. ./vars.sh
cp out/$goarch/gps4ptp /usr/local/sbin/gps4ptp
cp gps4ptp.service /etc/systemd/system/
# Don't overrwrite existing config file
if [ ! -f $CONFIG_FILE ]; then
    use=default
    # use config/cm4.toml if running on a Raspberry Pi Compute Module 4
    if [ -f /proc/device-tree/model ] && grep -q "Raspberry Pi Compute Module 4" /proc/device-tree/model; then
        use=cm4
    fi
    cp configs/$use.toml $CONFIG_FILE
fi
systemctl daemon-reload
