"""satpulsewb corrections: forward a source's RTCM to the receiver, then VRS.

The satpulsewb counterpart to the daemon's stream/pull-ntrip, landing beside it.
The runner starts a fake Ntrip correction source (declared in CORRECTION_SOURCE)
before satpulsewb; the scenario connects, POSTs corrections/start from the API,
and checks that the workbench reports the state reaching connected, delivers the
correction packets as gps:corrpacket, decodes the base station's antenna
reference point as gps:basearp, and writes the source RTCM back over the serial
port. A pty backs the device so the write path exists and the runner captures
it; stream.check_pulled_rtcm scans it -- the same check the daemon pull scenario
uses. Finally, with a fix now held, a VRS (nmeaSend) restart is accepted.

The correction source is a purpose-recorded F9P base capture (base-arp.jsonl:
RTCM MSM7 plus 1005 station coordinates), so gps:basearp is exercised. A
deterministic pre-fix VRS refusal still wants a no-fix rover fixture (plan
phase 5) and is not exercised here.
"""

import common
from scenarios import stream

PROGRAM = "satpulsewb"
CAPTURE_WRITES = True
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 10

# The runner starts fakesource.py on this port before satpulsewb and points
# ctx.pull_source_log at this log, so stream.check_pulled_rtcm compares the
# captured serial writes against it. The scenario POSTs corrections/start
# pointing at the same port.
CORRECTION_SOURCE = {
    "port_key": "SATPULSE_TEST_REMOTE_CASTER_PORT",
    "log": "gps/testdata/packets/u-blox/ZED-F9P/base-arp.jsonl",
    "mountpoint": "RTCM",
    "username": "smoke",
    "password": "smoketest",
}


def run(ctx: common.SmokeContext) -> None:
    common.check_wb_state(ctx, "connected")
    src = {
        "mode": "ntrip",
        "host": "127.0.0.1",
        "port": ctx.port(CORRECTION_SOURCE["port_key"]),
        "mountpoint": "RTCM",
        "username": "smoke",
        "password": "smoketest",
        "nmeaSend": False,
    }
    status, body = common.wb_post(ctx, "/api/corrections/start", src)
    assert status == 200, f"corrections/start failed: {status} {body!r}"
    common.check_wb_corr_state(ctx, "connected")
    common.wb_sse(ctx, expect=["gps:corrpacket", "gps:basearp"])
    ctx.wait_replay()
    stream.check_pulled_rtcm(ctx)
    # VRS: with a position fix now held, a nmeaSend restart passes the fix gate.
    status, body = common.wb_post(ctx, "/api/corrections/start", {**src, "nmeaSend": True})
    assert status == 200, f"VRS corrections/start refused after a fix existed: {status} {body!r}"
