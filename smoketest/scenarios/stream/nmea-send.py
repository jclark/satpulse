"""Ntrip NMEA send pull: the daemon uploads GGA before corrections stream."""

import common
from scenarios import stream

CAPTURE_WRITES = True
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
PULL_SOURCE_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon-msm4-115200.jsonl"
# A modest FACTOR keeps the pull stream slow enough that the pty drain never
# backpressures the daemon into pruning a stale same-type correction, so the
# exact-match check is not flaky on slower CI hosts. nmeaSendInterval = 1 still
# yields many GGA keepalives over the longer replay, past the periodic check.
FACTOR = 3


def run(ctx: common.SmokeContext) -> None:
    stream.check_pull_connected(ctx)
    stream.check_pull_uploaded_gga(ctx)
    stream.check_pulled_rtcm(ctx)
    stream.check_pull_periodic_gga(ctx)
