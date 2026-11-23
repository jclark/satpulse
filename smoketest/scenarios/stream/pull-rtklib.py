"""stream.pull interop with a real RTKLIB str2str Ntrip caster.

Like stream/pull-ntrip, but the correction source is a real RTKLIB str2str
Ntrip caster (PULL_PEER = "str2str") instead of fakesource.py. str2str
serves from the daemon's connect point onward, so the daemon writes back a
contiguous window of the source RTCM (not the whole log); the check matches
that window against the source. Interop proof that the pull client speaks
the real caster's protocol and relays the bytes faithfully. Uses a pty as
the write path, stops via SIGINT, and is skipped when str2str is not on PATH.

A low FACTOR keeps the caster streaming until the daemon's pull client has
connected, so the received window is non-empty.
"""

import common
from scenarios import stream

REQUIRES = ("str2str",)
CAPTURE_WRITES = True
PULL_PEER = "str2str"
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
PULL_SOURCE_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon-msm4-115200.jsonl"
FACTOR = 2


def run(ctx: common.SmokeContext) -> None:
    stream.check_pull_connected(ctx)
    stream.check_pulled_rtcm_window(ctx)
