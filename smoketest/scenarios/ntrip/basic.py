"""Ntrip caster: source table lists the mountpoint and streams RTCM."""

import common
from scenarios import ntrip

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    ntrip.check_sourcetable(ctx, "RTCM")
    ntrip.check_stream(ctx, "RTCM")
    ctx.wait_replay()
