"""Ntrip caster MSM7-to-MSM4 conversion.

The receiver feed carries RTCM MSM7 observations (1077/1087/1097/1127).
The mountpoint sets msm7to4, so the client receives the MSM4 equivalents
(1074/1084/1094/1124) instead, with no MSM7 message passing through.
"""

import common
from scenarios import ntrip

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/raw-cross-115200.jsonl"
FACTOR = 5

MSM7 = {"1077", "1087", "1097", "1127"}
MSM4 = {"1074", "1084", "1094", "1124"}


def run(ctx: common.SmokeContext) -> None:
    ntrip.check_sourcetable(ctx, "RTCM")
    msgs = ntrip.stream_rtcm_msgs(ctx, "RTCM")
    assert msgs & MSM4, f"expected MSM4 messages, got {sorted(msgs)}"
    assert not (msgs & MSM7), f"MSM7 messages not converted: {sorted(msgs & MSM7)}"
    ctx.wait_replay()
