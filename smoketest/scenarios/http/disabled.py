"""HTTP with GUI and metrics disabled: position works, others 404.

The runner's shutdown check also exercises clean SIGINT shutdown for a
GUI-disabled HTTP endpoint.
"""

import common

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()
    common.check_position(ctx)
    common.check_status(ctx, "/metrics", 404)
    common.check_status(ctx, "/", 404)
