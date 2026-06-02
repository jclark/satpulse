"""Read-only TCP serial proxies with per-listener protocol filters.

Two [[proxy.tcp]] listeners forward different protocols (UBX and RTCM)
from the same stream. Each check confirms its listener forwards that
protocol and that the bytes are a slice of the source packet log. Uses a
UBX+RTCM capture so both protocols flow throughout the replay.
"""

import common
from scenarios import proxy

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/raw-cross-115200.jsonl"
FACTOR = 3


def run(ctx: common.SmokeContext) -> None:
    proxy.check_tcp(ctx, ctx.port("SATPULSE_TEST_PROXY_TCP_PORT"), "UBX")
    proxy.check_tcp(ctx, ctx.port("SATPULSE_TEST_PROXY_TCP_RTCM_PORT"), "RTCM")
    ctx.wait_replay()
