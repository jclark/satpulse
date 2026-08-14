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

CAPTURE_WRITES = True
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
PULL_SOURCE_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon-msm4-115200.jsonl"
# check_pulled_rtcm demands exact delivery, so FACTOR is bounded: the source
# emits each RTCM type once a second and a pty drain that stalls longer than
# 1/FACTOR seconds loses the stale packet to the daemon's same-type prune. At 10
# that window was 100 ms, under the ~200 ms stalls a macOS runner shows, and the
# scenario lost packets there. 5 gives the 200 ms of stream/wb-corrections, and
# is as low as this scenario can go for free: it runs in the shadow of
# stream/push-udp, so its 7 s fits the suite's slack where 3 would not.
FACTOR = 5


def run(ctx: common.SmokeContext) -> None:
    stream.check_pull_connected(ctx)
    stream.check_pulled_rtcm(ctx)
