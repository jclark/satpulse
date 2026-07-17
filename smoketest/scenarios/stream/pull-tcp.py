"""Plain TCP pull: the daemon writes a raw TCP source's RTCM back to the receiver.

Like stream/pull-ntrip, but the daemon's [stream.pull.tcp] client connects to a
raw TCP correction source (scenarios/stream/fakesource.py --tcp) that streams
RTCM with no Ntrip handshake, exercising the non-Ntrip pull transport. The
daemon reads the corrections and writes each one back over the serial port; the
pty drain captures the writes and the check matches them against the source log.

The serial device is a pty so the write path exists (a read-only FIFO cannot
carry the daemon's writes), and the scenario still stops via the normal SIGINT
path rather than modelling a disconnect.
"""

import common
from scenarios import stream

CAPTURE_WRITES = True
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
PULL_SOURCE_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon-msm4-115200.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    stream.check_pull_connected(ctx)
    stream.check_pulled_rtcm(ctx)
