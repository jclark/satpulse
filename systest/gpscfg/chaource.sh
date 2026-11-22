#!/bin/sh
# Ephemeral (RAM-only) configuration of the Allystar TAU1201: the NAV
# messages satpulsed decodes, NMEA 4.10 GSA and GSV for satellite usage
# and per-signal C/N0 (NAV-SVINFO is one row per satellite), other NMEA
# off, and a survey started. PPS is left alone: the factory CFG-PPS
# already matches (1 s, 100 ms, rising, only with fix) and the pps tag
# would zero the unit's factory-calibrated offset.
# The persistent configuration stays factory default (the factory baud
# is already 115200); this is reapplied on every start, so a
# power-cycled receiver is reconfigured.
set -e
[ -c "$DEVICE" ] && [ -w "$DEVICE" ] || { echo "DEVICE=$DEVICE: not a writable character device" >&2; exit 1; }
[ "$SPEED" -gt 0 ] 2>/dev/null || { echo "SPEED=$SPEED: not a positive number" >&2; exit 1; }
: "${GPSMSG_DIR:=/usr/share/satpulse/gpsmsg}"
export SATPULSE_VENDORS=Allystar
exec satpulsetool gps -d "$DEVICE" -s "$SPEED" -m "$GPSMSG_DIR/allystar/tau1201.toml" \
    -t nmea-off,nmea-ver-410,nmea-gsa,nmea-gsv,\
asbin-nav-time,asbin-nav-posllh,asbin-nav-dop,asbin-nav-auto,asbin-nav-svin,\
survey
