# Test cases for --pps (time pulse) on the LG290P (PQTMCFGPPS).
# Widths up to 900 ms use the legacy PQTMCFGPPS write form. Wider pulses
# would switch to the PQTMCFGPPS2 form, which the module rejects at a
# 1 s period, so they are exercised offline only (see the report).
t --pps 0.1
t --pps 0.5
t --pps 0.9
# Disable the pulse
t --pps 0
# Re-enable
t --pps 0.1
