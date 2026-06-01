"""Event, track, and packet logs are written with expected content."""

import checks

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 10


def run(ctx):
    ctx.wait_replay()
    checks.check_event_log(ctx, expect_keys=["time", "posGeo", "navEpoch"])
    checks.check_track_log(ctx)
    checks.check_packet_log(ctx)
