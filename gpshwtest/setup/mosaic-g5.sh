#!/bin/sh
# Establish the mosaic-G5 characterization starting state from factory
# defaults (see HW/mosaic-g5.md, session preconditions): SBF Stream1 =
# PVTGeodetic at 1 Hz, NMEA off, everything else at factory defaults.
# Start from factory defaults first (satpulsetool gps --factory-reset,
# or exeCopyConfigFile, RxDefault, Current on an untouched receiver).
# Usage: setup/mosaic-g5.sh /dev/ttyACM1
set -e
dev=${1:?usage: $0 <serial-device>}
satpulsetool gps -d "$dev" -s 115200 --nmea-out none --pvt-out pos,time
