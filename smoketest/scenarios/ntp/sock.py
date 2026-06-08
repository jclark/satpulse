"""Chrony SOCK refclock: samples are well-formed and time-consistent.

A pure 1 Hz GNRMC log drives serial timing mode, which feeds one SOCK sample
per second. Replay must be realtime (factor 1): the offset stays constant only
because the message UTC and the read clock advance together, so a tight offset
spread is the real assertion. See scenarios.ntp.check_sock.
"""

import common
from scenarios import ntp

PACKET_LOG = "gps/testdata/packets/unicore/UM980/nmea-rmc.jsonl"
FACTOR = 1


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()
    ntp.check_sock(ctx)
