#!/bin/bash
# Establish the LG290P characterization starting state from factory
# defaults (see HW/lg290p.md): the time pulse in only-when-locked mode,
# saved to NVM. Everything else stays at factory defaults.
#
# The pulse mode matters because --pps requests only-when-locked by
# design and the CLI has no flag to set the mode back, so gpshwtest can
# restore what it finds only if the starting state is only-when-locked.
#
# Usage: setup/lg290p.sh <device> [speed]
set -e
dev=${1:?usage: $0 <device> [speed]}
speed=${2:-460800}
satpulsetool gps -d "$dev" -s "$speed" --pps 0.1 --save
