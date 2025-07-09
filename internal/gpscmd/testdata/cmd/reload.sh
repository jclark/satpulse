#!/bin/bash
# Test GNSS constellation configuration and reload functionality
# Tests changing GNSS constellations and the reload command

t --gnss GPS --pps 0.01 --ant-cable-delay 100 --time-gnss GPS
t --show-config
t --reload
t --show-config
