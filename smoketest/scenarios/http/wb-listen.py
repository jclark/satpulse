"""satpulsewb -L with the token disabled: open API, CSRF gate, explicit bind.

-L takes the bind address (the runner's allocated HTTP port) and disables the
access token, the ssh-tunnel workflow the man page documents. The API is then
reachable without a token, but the CSRF content-type gate must still reject a
cross-site simple POST, so the token being off does not hand the receiver to
any page that can reach the port.
"""

import common

PROGRAM = "satpulsewb"
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    common.check_wb_html(ctx)
    common.check_wb_open_no_token(ctx)
    common.check_wb_csrf(ctx)
    common.check_wb_snapshots(ctx)
    ctx.wait_replay()
