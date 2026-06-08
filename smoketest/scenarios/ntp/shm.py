"""NTP SHM refclock: a mode-1 sample is published to shared memory.

A pure 1 Hz GNRMC log drives serial timing mode, which feeds one SHM sample
per second. Replay must be realtime (factor 1) for the same reason as the
SOCK scenario, but this smoke check only asserts the SHM interface contract:
the daemon attaches and publishes a valid sample with expected metadata.
"""

import common
from scenarios import ntp

PACKET_LOG = "gps/testdata/packets/unicore/UM980/nmea-rmc.jsonl"
FACTOR = 1
REQUIRES_ROOT = True


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()
    ntp.check_shm(ctx)
