"""Ntrip pull: the daemon writes a correction source's RTCM back to the receiver.

run.py starts a fake Ntrip correction source (scenarios/stream/fakesource.py)
before the daemon. The daemon's [stream.pull] client connects out to it, reads
the RTCM MSM4 corrections it streams, and writes each one back over the serial
port. The serial device is a pty so that write path exists (a read-only FIFO
cannot carry the daemon's writes), and the runner captures the daemon's writes;
the check scans them back into RTCM and matches the source corrections.

This is the only scenario using the pty as a write path rather than to model a
disconnect (shutdown/serial-loss), so it still stops via the normal SIGINT path.
The pull source is a daemon+MSM4 capture; the pull client forwards only its RTCM,
ignoring the UBX/NMEA, which is the realistic shape of a receiver-fed base.
"""

import common
from scenarios import stream

INPUT = "pty"
CAPTURE_WRITES = True
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
PULL_SOURCE_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon-msm4-115200.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    stream.check_pull_connected(ctx)
    stream.check_pulled_rtcm(ctx)
