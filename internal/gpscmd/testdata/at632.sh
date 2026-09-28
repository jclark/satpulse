# AT362-AT6668-6T-30 (sold as AT632-6T-30) timing receiver, CASIC URANUS6 (V6)
dev=ttyUSB3
speed=115200
vendor=zhongke
# V6 firmware does not support --reload (CFG-CFG load-from-flash is a
# no-op), so reset to a clean baseline with --factory-reset. It restarts
# the receiver, so allow settle time.
reset_args="--factory-reset"
reload_secs=5
