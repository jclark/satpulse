"""Ntrip caster interop with a real RTKLIB str2str client.

str2str connects to satpulsed's caster as an Ntrip client and receives
RTCM. A real client joining the live stream mid-flight sees a contiguous
window of the caster's RTCM, relayed byte-for-byte -- interop proof that a
real RTKLIB client speaks our caster's protocol. Skipped when str2str is
not on PATH.
"""

import common
from scenarios import ntrip

REQUIRES = ("str2str",)
PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    ntrip.check_sourcetable(ctx, "RTCM")
    ntrip.check_stream_str2str(ctx, "RTCM")
    ctx.wait_replay()
