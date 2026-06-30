"""Multiple [[http]] endpoints, each wired from its own table.

The first endpoint serves the full default (GUI, metrics, position); the
second serves position only (GUI and metrics disabled). Both listen and
serve concurrently, and each reflects its own config, proving repeated
[[http]] tables are parsed and applied per entry. The runner waits on
both listeners before replay and verifies both release on shutdown.
"""

import common

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 5


def run(ctx: common.SmokeContext) -> None:
    port2 = ctx.port("SATPULSE_TEST_HTTP_PORT2")
    # Live check on the full endpoint while the background replay flows.
    common.check_sse(ctx, expect=["time", "posvel", "quality"])
    ctx.wait_replay()
    # First endpoint: full default config.
    common.check_position(ctx)
    common.check_metrics(ctx, expect=["satpulse_num_satellites", "satpulse_dop"])
    common.check_html(ctx, "/")
    # Second endpoint: position only, GUI and metrics off.
    common.check_position(ctx, port=port2)
    common.check_status(ctx, "/", 404, port=port2)
    common.check_status(ctx, "/metrics", 404, port=port2)
