"""Ntrip push: the daemon forwards the log's RTCM to a remote caster.

run.py starts a fake Ntrip caster (fakecaster.py) before the daemon; the
daemon's [[stream.push]] feeds it the scanned RTCM, which the caster captures.
The check scans the capture and confirms it matches the source log's RTCM.
"""

import checks

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
FACTOR = 10


def run(ctx):
    ctx.wait_replay()
    checks.check_pushed_rtcm(ctx, mountpoint="RTCM")
