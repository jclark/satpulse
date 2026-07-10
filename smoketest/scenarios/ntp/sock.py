"""Chrony SOCK refclock: samples are well-formed and time-consistent.

A pure 1 Hz GNRMC log drives serial timing mode, which feeds one SOCK sample
per second. Replay must be realtime (factor 1): the offset stays constant only
because the message UTC and the read clock advance together, so a tight offset
spread is the real assertion. See scenarios.ntp.check_sock.
"""

import sys

import common
from scenarios import ntp

PACKET_LOG = "gps/testdata/packets/unicore/UM980/nmea-rmc.jsonl"
FACTOR = 1

# macOS CI runners suffer isolated scheduling stalls of up to ~200 ms that
# land in a single sample's read timestamp, so the consistency bound is wider
# there. The correctness assertion (max_time_error) is unchanged.
MAX_SPREAD = 0.2 if sys.platform == "darwin" else 0.05


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()
    ntp.check_sock(ctx, max_spread=MAX_SPREAD)
