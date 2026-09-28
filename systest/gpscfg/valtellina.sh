#!/bin/sh
# Ephemeral (RAM-only) configuration of the Techtotop T303-5D: the SDBP
# DAT messages satpulsed decodes, NMEA off, and PPS enabled.
# The persistent configuration stays factory default (the factory baud
# is already 115200); this is reapplied on every start, so a
# power-cycled receiver is reconfigured.
set -e
[ -c "$DEVICE" ] && [ -w "$DEVICE" ] || { echo "DEVICE=$DEVICE: not a writable character device" >&2; exit 1; }
[ "$SPEED" -gt 0 ] 2>/dev/null || { echo "SPEED=$SPEED: not a positive number" >&2; exit 1; }
: "${GPSMSG_DIR:=/usr/share/satpulse/gpsmsg}"
export SATPULSE_VENDORS=Techtotop
exec satpulsetool gps -d "$DEVICE" -s "$SPEED" -m "$GPSMSG_DIR/techtotop/techtotop.toml" \
    -t nmea-off,\
sdbp-dat-tpps,sdbp-dat-gpst,sdbp-dat-galt,sdbp-dat-bdst,sdbp-dat-utct2,\
sdbp-dat-gpsu,sdbp-dat-galu,sdbp-dat-bdsu,\
sdbp-dat-lla3,sdbp-dat-ecef2,sdbp-dat-ned3,sdbp-dat-dop,sdbp-dat-sat,sdbp-dat-tsurv,\
pps
