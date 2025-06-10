#!/bin/bash
# Test GNSS constellation configuration and reload functionality
# Tests changing GNSS constellations and the reload command

t --gnss GLO,GPS,BDS,GAL,QZSS
t --gnss GPS
t --reload
t