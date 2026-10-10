#!/bin/bash
# Establish the TAU1302 characterization starting state from factory
# defaults (see HW/tau1302.md): the factory configuration with the RTCM
# broadcast-ephemeris output turned off, saved to NVM. Everything else
# stays at factory defaults (this unit's factory state includes MSM7 at
# rate 1 and its narrower signal selection).
#
# The ephemeris output matters because it is outside the model
# vocabulary: every RTCM request turns it off and no request can turn
# it back on, so gpshwtest can restore what it finds only if the
# starting state does not include it. Requires a satpulsetool with
# Allystar high-level configuration (the allystar-config branch).
#
# Usage: setup/tau1302.sh <device> [speed]
set -e
dev=${1:?usage: $0 <device> [speed]}
speed=${2:-115200}
satpulsetool gps -d "$dev" -s "$speed" --rtcm-out MSM7 --save
