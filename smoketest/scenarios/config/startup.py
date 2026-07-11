"""satpulsed x u-blox simulator: the startup config path.

The daemon runs its startup config phase against the simulator over a pty -- the
write path a read-only FIFO replay cannot provide, so gpscfg actually probes and
configures. Asserts that detection was active (the receiver identity carries the
simulated ZED-F9P's MON-VER model and firmware), that the startup ConfigTarget
landed (NAV-SAT, off in the personality defaults, becomes live satellite data on
the daemon's SSE surface), and -- via the runner's shared shutdown/log-scan path
-- that configuration completed without errors and shutdown is graceful.

No FACTOR/PACKET_LOG: the simulator generates nav itself from SIM_REPLAY, one
epoch per second, so the bank need only outlast the run's checks (this F9P log
holds ~31 NAV-SAT epochs).
"""

import common
from scenarios import config

PROGRAM = "satpulsed"
PROVIDER = "ubxsim"
PERSONALITY = "gps/app/ubxsim/testdata/f9p/f9p-personality.ubx"
SIM_REPLAY = "gps/testdata/packets/u-blox/ZED-F9P/daemon-sats-pos-38400.jsonl"


def run(ctx: common.SmokeContext) -> None:
    config.check_daemon_startup_config(ctx)
