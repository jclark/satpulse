#!/bin/sh
# Ephemeral (RAM-only) configuration of the Septentrio mosaic on USB1:
# the SBF blocks satpulsed decodes, NMEA off, and PPS enabled.
# The persistent configuration stays factory default; this is reapplied
# on every start, so a power-cycled receiver is reconfigured.
set -e
[ -c "$DEVICE" ] && [ -w "$DEVICE" ] || { echo "DEVICE=$DEVICE: not a writable character device" >&2; exit 1; }
[ "$SPEED" -gt 0 ] 2>/dev/null || { echo "SPEED=$SPEED: not a positive number" >&2; exit 1; }
: "${GPSMSG_DIR:=/usr/share/satpulse/gpsmsg}"
export SATPULSE_VENDORS=Septentrio
exec satpulsetool gps -d "$DEVICE" -s "$SPEED" -m "$GPSMSG_DIR/septentrio/mosaic.toml" \
    -t cmd-escape,nmea-off,sbf-off,\
sbf-pvtgeodetic-usb1,sbf-pvtcartesian-cur,sbf-dop-cur,sbf-endofpvt-cur,\
sbf-xppsoffset-cur,sbf-gpsutc-cur,sbf-galutc-cur,sbf-bdsutc-cur,\
sbf-diffcorrin-cur,sbf-basestation-cur,sbf-channelstatus-cur,sbf-measepoch-cur,\
pps
