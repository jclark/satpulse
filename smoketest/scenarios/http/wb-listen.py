"""satpulsewb -L with open API, CSRF gate, and message-file library wiring.

-L takes the bind address (the runner's allocated HTTP port) and disables the
access token, the ssh-tunnel workflow the man page documents. The API is then
reachable without a token, but the CSRF content-type gate must still reject a
cross-site simple POST, so the token being off does not hand the receiver to
any page that can reach the port. A fixture SATPULSE_GPSMSG_PATH also exercises
the message-file catalog and selection wiring through the real binary.
"""

import json
import os
from typing import cast

import common

PROGRAM = "satpulsewb"
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 10
ENV = {
    "SATPULSE_GPSMSG_PATH": os.path.join(os.path.dirname(__file__), "gpsmsg"),
}


def run(ctx: common.SmokeContext) -> None:
    common.check_wb_html(ctx)
    common.check_wb_open_no_token(ctx)
    common.check_wb_csrf(ctx)
    common.check_wb_snapshots(ctx)
    status, body = common.wb_get(ctx, "/api/msgfile/catalog")
    assert status == 200, f"message-file catalog expected 200, got {status}"
    cat = cast(common.JsonObject, json.loads(body))
    names = cast(list[common.JsonObject], cat.get("names"))
    assert any(e.get("vendor") == "u-blox" and e.get("file") == "smoke" for e in names), (
        f"message-file catalog missing u-blox/smoke: {cat}"
    )
    seat = common.wb_claim(ctx)  # msgfile/select is a writer POST
    status, body = common.wb_post(
        ctx, f"/api/msgfile/select?seat={seat}", {"vendor": "u-blox", "file": "smoke"}
    )
    assert status == 200, f"message-file select expected 200, got {status}: {body!r}"
    result = cast(common.JsonObject, json.loads(body))
    tags = cast(list[common.JsonObject], result.get("tags"))
    assert len(tags) == 1 and tags[0].get("tag") == "poll", (
        f"message-file select returned unexpected tags: {result}"
    )
    ctx.wait_replay()
