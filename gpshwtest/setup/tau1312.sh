#!/bin/bash
# Establish the TAU1312 characterization starting state (see
# HW/tau1312.md): the standard restorable NMEA set at 1 Hz, saved to NVM.
# This removes factory GST output, which is outside gpshwtest's NMEA
# vocabulary. Everything else stays as found.
#
# Usage: setup/tau1312.sh <device> [speed]
set -e
dev=${1:?usage: $0 <device> [speed]}
speed=${2:-115200}
satpulsetool gps -d "$dev" -s "$speed" --nmea-out GGA,GSA,GSV,RMC,ZDA --save
