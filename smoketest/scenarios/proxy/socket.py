"""Read-only Unix-socket serial proxy, filtered to UNCB (Unicore binary).

The check reads the socket directly and confirms that its bytes are a slice
of the source log's UNCB stream.
"""

import common
from scenarios import proxy

PACKET_LOG = "gps/testdata/packets/unicore/UM980/raw-cross-460800.jsonl"
FACTOR = 6


def run(ctx: common.SmokeContext) -> None:
    proxy.check_socket(ctx, protocol="UNCB", read_seconds=7.0)
    ctx.wait_replay()
