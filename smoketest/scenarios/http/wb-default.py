"""satpulsewb default launch: URL/token, auth, snapshots, SSE, priming, gating.

No -L, so the runner exercises the real default-port path and parses the printed
URL and generated token. -d auto-connects over the FIFO; the read-only FIFO
makes the session skip probing and fall to passive detection, so it reaches
connected and the monitor events flow. This is the workbench's meaty smoke
scenario -- wb-listen covers the token-disabled -L path.
"""

import common

PROGRAM = "satpulsewb"
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon-sats-pos-38400.jsonl"
FACTOR = 5


def run(ctx: common.SmokeContext) -> None:
    # Live checks first, while the background replay is still flowing: the
    # packet stream is gated (gps:packet is not primed, so it only flows live).
    common.check_wb_html(ctx)
    common.check_wb_auth_required(ctx)
    common.check_wb_snapshots(ctx)
    common.check_wb_packet_gating(ctx)
    common.wb_sse(ctx, expect=["gps:state", "gps:time", "gps:epochPVT", "gps:nmeaPosition"])
    ctx.wait_replay()
    # After replay the FIFO stays connected but idle, so a fresh SSE client can
    # only be seeing the primed sticky events.
    common.check_wb_priming(ctx)
    # Last: this supersedes the context's seat (newest window wins).
    common.check_wb_seat_takeover(ctx)
