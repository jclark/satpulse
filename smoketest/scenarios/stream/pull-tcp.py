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
# check_pulled_rtcm demands exact delivery, so a stalled pty drain loses a stale
# packet to the daemon's same-type prune once it stalls longer than 1/FACTOR
# seconds (see stream/pull-ntrip). This scenario starts as stream/push-udp is
# ending, so 8 is as far as it can go without lengthening the suite; that is a
# 125 ms window, still short of the ~200 ms a macOS runner stalls for.
FACTOR = 8


def run(ctx: common.SmokeContext) -> None:
    stream.check_pull_connected(ctx)
    stream.check_pulled_rtcm(ctx)
