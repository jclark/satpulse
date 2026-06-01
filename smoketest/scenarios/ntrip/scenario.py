"""Ntrip caster: source table lists the mountpoint and streams RTCM."""

import checks

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
FACTOR = 10


def run(ctx):
    checks.check_sourcetable(ctx, "RTCM")
    checks.check_ntrip_stream(ctx, "RTCM")
    ctx.wait_replay()
