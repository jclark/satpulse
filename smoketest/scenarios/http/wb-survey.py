"""satpulsewb survey priming: a reloaded client sees the survey status at once.

Replays an F9P survey-in capture (UBX NAV-SVIN -> gps:msg kind "survey"), the
slow-changing message a base outputs while it settles its position. The event
flows live, and after the replay ends the FIFO stays connected but idle, so a
fresh SSE client that then still receives a gps:msg survey can only be seeing
the hub's sticky-kind cache -- the priming a reloaded browser relies on, rather
than waiting a full survey interval to see the state again.

survey-38400.jsonl has no NAV-TIMELS, so survey is its only sticky msg kind;
that is what makes the primed gps:msg unambiguous.
"""

import common

PROGRAM = "satpulsewb"
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/survey-38400.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    common.check_wb_state(ctx, "connected")
    common.wb_sse(ctx, expect=["gps:state", "gps:msg"])  # survey status flows live
    ctx.wait_replay()
    common.check_wb_msg_primed(ctx, "survey")
