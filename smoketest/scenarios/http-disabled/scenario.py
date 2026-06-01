"""HTTP with GUI and metrics disabled: position works, others 404.

The runner's shutdown check also exercises clean SIGINT shutdown for a
GUI-disabled HTTP endpoint.
"""

import checks

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 10


def run(ctx):
    ctx.wait_replay()
    checks.check_position(ctx)
    checks.check_status(ctx, "/metrics", 404)
    checks.check_status(ctx, "/", 404)
