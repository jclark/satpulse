"""UDP push: the daemon forwards all packet data to a UDP destination."""

import common
from scenarios import stream

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()
    stream.check_udp_pushed_all(ctx)
