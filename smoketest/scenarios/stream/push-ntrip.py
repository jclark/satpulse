"""Ntrip push: the daemon forwards the log's RTCM to a remote caster.

run.py starts a fake Ntrip caster per [[stream.push]] entry before the daemon.
The first entry's credentials are correct, so its caster
accepts the feed and captures the scanned RTCM, which the check matches against
the source log. The second entry sends a wrong password, so its caster rejects
it with "Bad Password"; that is a permanent failure, so the daemon gives up on
it rather than reconnecting forever.
"""

import common
from scenarios import stream

PACKET_LOG = "gps/testdata/packets/unicore/UM982/rtcm-eph.jsonl"
FACTOR = 5

# The rejected second push entry logs these when it gives up: the writer's
# "giving up" and the daemon's "gave up" state change. A transient failure
# would instead log "ntrip push connect failed" (not allowlisted), so a
# regression to reconnect-forever still fails the scenario.
ALLOWED_ERRORS = ("ntrip push giving up", "ntrip push gave up")


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()
    stream.check_pushed_rtcm(ctx, mountpoint="RTCM")
    stream.check_push_gave_up(ctx)
