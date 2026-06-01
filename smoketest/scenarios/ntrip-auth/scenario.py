"""Ntrip caster auth: unauthenticated rejected, valid credentials stream."""

import checks

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
FACTOR = 10


def run(ctx):
    checks.check_sourcetable(ctx, "RTCM")
    checks.check_ntrip_unauthorized(ctx, "RTCM")
    checks.check_ntrip_stream(ctx, "RTCM", auth=("rover", "secret"))
    ctx.wait_replay()
