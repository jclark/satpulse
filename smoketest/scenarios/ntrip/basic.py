"""Ntrip caster: source table lists the mountpoint and streams RTCM."""

import common
from scenarios import ntrip

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    rec = ntrip.sourcetable_record(ctx, "RTCM")
    # Unset shared fields fall back to their documented defaults.
    assert rec[ntrip.STR_NETWORK] == "Misc", rec
    assert rec[ntrip.STR_COUNTRY] == "XXX", rec
    ntrip.check_stream(ctx, "RTCM")
    ctx.wait_replay()
