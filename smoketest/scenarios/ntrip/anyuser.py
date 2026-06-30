"""Ntrip caster mountpoint with auth.anyUser.

The mountpoint accepts any valid top-level user (auth.anyUser, as
opposed to a fixed auth.users list). Unauthenticated requests and wrong
credentials are still rejected; a valid top-level user streams RTCM.
"""

import common
from scenarios import ntrip

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    ntrip.check_sourcetable(ctx, "RTCM")
    ntrip.check_unauthorized(ctx, "RTCM")
    ntrip.check_credentials_rejected(ctx, "RTCM", ("rover", "wrong"))
    ntrip.check_stream(ctx, "RTCM", auth=("rover", "secret"))
    ctx.wait_replay()
