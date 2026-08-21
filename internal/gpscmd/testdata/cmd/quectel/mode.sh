# Test cases for positioning mode on the LG290P (PQTMCFGRCVRMODE/SVIN).
# Mode changes are stored-only (modeOnlyWithReset) and position modes
# exist only under base receiver mode: a request without --save
# --reload is skipped with a warning; an effective change writes
# RCVRMODE and SVIN, saves, and reboots via PQTMSRR. Base mode swaps
# to a per-mode message table (RTCM only, no NMEA).
t --survey --survey-time 60 --survey-acc 5
# Enter survey mode effectively
t --survey --survey-time 60 --survey-acc 5 --save --reload
sleep 20
t --show-config
# Restore rover (mobile) mode
t --mobile --save --reload
sleep 20
t --show-config
