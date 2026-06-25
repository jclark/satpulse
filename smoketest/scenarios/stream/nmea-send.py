"""Ntrip NMEA send pull: the daemon uploads GGA before corrections stream."""

import common
from scenarios import stream

INPUT = "pty"
CAPTURE_WRITES = True
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
PULL_SOURCE_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon-msm4-115200.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    stream.check_pull_connected(ctx)
    stream.check_pull_uploaded_gga(ctx)
    stream.check_pulled_rtcm(ctx)
    stream.check_pull_periodic_gga(ctx)
