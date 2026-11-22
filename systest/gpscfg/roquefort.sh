#!/bin/sh
# Ephemeral (RAM-only) configuration of the Quectel LG290P: the PQTM
# messages satpulsed decodes plus GSV, other NMEA off, and PPS enabled.
# The persistent configuration stays factory default (the factory baud
# is already 460800); this is reapplied on every start, so a
# power-cycled receiver is reconfigured.
set -e
[ -c "$DEVICE" ] && [ -w "$DEVICE" ] || { echo "DEVICE=$DEVICE: not a writable character device" >&2; exit 1; }
[ "$SPEED" -gt 0 ] 2>/dev/null || { echo "SPEED=$SPEED: not a positive number" >&2; exit 1; }
: "${GPSMSG_DIR:=/usr/share/satpulse/gpsmsg}"
export SATPULSE_VENDORS=Quectel
exec satpulsetool gps -d "$DEVICE" -s "$SPEED" -m "$GPSMSG_DIR/quectel/lg290p.toml" \
    -t nmea-off,nmea-gsv,pqtm-pvt,pqtm-epe,pqtm-dop,pqtm-eoe,pps
