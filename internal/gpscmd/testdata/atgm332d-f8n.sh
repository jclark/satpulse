# ATGM332D-AT9880-F8N-76 dual-band navigation receiver, CASIC URANUS6 (V6)
dev=ttyUSB2
speed=115200
vendor=zhongke
# V6 firmware does not support --reload (CFG-CFG load-from-flash is a
# no-op), so reset to a clean baseline with --factory-reset. It restarts
# the receiver, so allow settle time.
reset_args="--factory-reset"
reload_secs=5
