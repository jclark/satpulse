"""Chrony SOCK refclock: samples are well-formed and time-consistent.

A pure 1 Hz GNRMC log drives serial timing mode, which feeds one SOCK sample
per second. Replay must be realtime (factor 1): the offset stays constant only
because the message UTC and the read clock advance together, so a tight offset
spread is the real assertion. See checks.check_ntp_sock.
"""

import checks

PACKET_LOG = "gps/testdata/packets/unicore/UM980/nmea-rmc.jsonl"
FACTOR = 1


def run(ctx):
    ctx.wait_replay()
    checks.check_ntp_sock(ctx)
