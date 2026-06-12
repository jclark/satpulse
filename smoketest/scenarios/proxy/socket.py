"""Read-only Unix-socket serial proxy, filtered to UNCB (Unicore binary).

Read with satpulsetool gps --socket --capture --packet-log, which also
exercises that tool's passive-capture path and the proxy's support for a
non-u-blox protocol filter. The check confirms only UNCB is forwarded and
every captured packet exists in the source packet log.
"""

import common
from scenarios import proxy

PACKET_LOG = "gps/testdata/packets/unicore/UM980/raw-cross-460800.jsonl"
FACTOR = 6


def run(ctx: common.SmokeContext) -> None:
    proxy.check_socket_capture(ctx, protocol="UNCB", capture_seconds=7.0)
    ctx.wait_replay()
