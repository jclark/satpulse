#!/bin/bash
# Establish the TAU951M-P200 characterization starting state (see
# HW/tau951m.md): the standard NMEA set at 1 Hz, saved to NVM.
# Everything else stays as found.
#
# The unit is 5 Hz-native, so its 1 Hz NMEA output needs CFG-MSG
# divisor 5 on every sentence; satpulsetool computes the divisor from
# the measured native rate, which also repairs any stray rate-1 value
# a vendor-tool session left in NVM (a bare rate 1 means 5 Hz here).
# Requires a satpulsetool with the Allystar rate estimator (the
# allystar-config branch).
#
# Usage: setup/tau951m.sh <device> [speed]
set -e
dev=${1:?usage: $0 <device> [speed]}
speed=${2:-115200}
satpulsetool gps -d "$dev" -s "$speed" --nmea-out GGA,GSA,GSV,RMC,ZDA --save
