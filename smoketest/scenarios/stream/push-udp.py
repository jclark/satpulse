"""UDP push: the daemon forwards all packet data to a UDP destination."""

import common
from scenarios import stream

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
# A modest FACTOR keeps the datagram rate low enough that the UDP receiver
# never drops bytes, so the lossless-delivery check is not flaky on slower
# CI hosts. The replay still finishes well under the suite's realtime pole.
FACTOR = 3


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()
    stream.check_udp_pushed_all(ctx)
