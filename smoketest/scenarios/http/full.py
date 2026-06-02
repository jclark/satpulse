"""Full HTTP endpoint: position JSON, Prometheus metrics, GUI HTML, SSE."""

import common

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 5


def run(ctx: common.SmokeContext) -> None:
    # Live check first, while the background replay is still flowing.
    common.check_sse(ctx, expect=["time", "posvel", "quality"])
    ctx.wait_replay()
    common.check_position(ctx)
    common.check_metrics(ctx, expect=["satpulse_num_satellites", "satpulse_dop"])
    common.check_html(ctx, "/")
