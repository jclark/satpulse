"""Event, track, and packet logs are written with expected content."""

import common

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()
    common.check_event_log(ctx, expect_types=["time", "posGeo", "navEpoch"])
    common.check_track_log(ctx)
    common.check_packet_log(ctx)
