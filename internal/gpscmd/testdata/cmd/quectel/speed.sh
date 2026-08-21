# Test --speed baud change on the LG290P (PQTMCFGUART current-port write).
# Only the rates the module documents are used (9600/115200/230400/460800/
# 921600); the confirm read must succeed at the new rate.
saved_speed=$speed
t --speed 115200
speed=115200
t --speed 230400
speed=230400
t --speed $saved_speed
