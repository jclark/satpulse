"""UDP push: the daemon forwards all packet data to a UDP destination."""

import common
from scenarios import stream

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
# The source emits each RTCM type once a second, so a stalled sink has 1/FACTOR
# seconds before the next packet of that type prunes it from the daemon's queue
# (stream.pruningQueue) and the exact-delivery check reads the prune as loss.
# FACTOR 3 left 333 ms and still lost packets on a macOS runner, whose isolated
# scheduling stalls reach ~200 ms (see scenarios/ntp/sock.py); 2 leaves 500 ms.
# A modest FACTOR also keeps the datagram rate low enough that the UDP receiver
# does not drop bytes. The replay still finishes well under the realtime pole.
FACTOR = 2


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()
    stream.check_udp_pushed_all(ctx)
