"""Full HTTP endpoint: position JSON, Prometheus metrics, GUI HTML, SSE."""

import checks

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 5


def run(ctx):
    # Live check first, while the background replay is still flowing.
    checks.check_sse(ctx, expect=["time", "posvel", "quality"])
    ctx.wait_replay()
    checks.check_position(ctx)
    checks.check_metrics(ctx, expect=["satpulse_num_satellites", "satpulse_dop"])
    checks.check_html(ctx, "/")
