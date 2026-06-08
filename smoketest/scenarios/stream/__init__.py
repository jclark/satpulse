"""Checks owned by stream scenarios."""

from __future__ import annotations

import common


def check_pushed_rtcm(ctx: common.SmokeContext, mountpoint: str | None = None) -> int:
    """The daemon's [[stream.push]] feed delivers the source log's RTCM intact.

    The fake caster captured the pushed payload; scanning it must yield
    exactly the same RTCM packets, in order, as the source packet log. With a
    prompt consumer the push path neither prunes nor transforms, and the
    packet bcast is lossless, so the streams are identical. Polls because the
    last pushed packets may still be in flight when the scenario calls this.
    """
    want = [d for (_, _, d) in common.log_packets(ctx.packet_log, "RTCM")]
    assert want, "source log has no RTCM packets to push"
    got = common.poll(lambda: _pushed_rtcm(ctx) == want or None, interval=0.25)
    if got is None:
        n = len(_pushed_rtcm(ctx))
        raise AssertionError(
            f"pushed RTCM does not match source log: captured {n} of {len(want)} packets"
        )
    if mountpoint is not None:
        with open(ctx.caster_log, errors="replace") as f:
            assert f"accepted SOURCE mount={mountpoint}" in f.read(), (
                f"caster recorded no SOURCE feed for mountpoint {mountpoint}"
            )
    return len(want)


def check_pull_connected(ctx: common.SmokeContext) -> None:
    """The daemon's [stream.pull] client connects to the correction source."""
    def attempt() -> bool:
        with open(ctx.daemon_log, errors="replace") as f:
            return "stream pull connected" in f.read()

    assert common.poll(attempt), "daemon did not connect to the pull correction source"


def check_pulled_rtcm(ctx: common.SmokeContext) -> int:
    """The pull source's RTCM is written back to the receiver over the serial port.

    The fake source streams the scenario's RTCM log to the daemon's pull client;
    the daemon writes each correction to the serial port, where the pty drain
    captures it to serial_writes. Scanning that capture must yield exactly the
    source log's RTCM packets, in order, the same way push matches its source:
    the pull pruning queue only drops a stale same-type packet under serial-write
    backpressure, which a prompt pty sink never applies. The daemon's non-RTCM
    detection probes are filtered out by tag. Polls because the last corrections
    may still be in flight when the scenario calls this.
    """
    want = [d for (_, _, d) in common.log_packets(ctx.pull_source_log, "RTCM")]
    assert want, "pull source log has no RTCM packets"
    got = common.poll(lambda: _pulled(ctx) == want or None, interval=0.25)
    if got is None:
        n = len(_pulled(ctx))
        raise AssertionError(
            f"pulled RTCM does not match source log: captured {n} of {len(want)} packets"
        )
    return len(want)


def _pulled(ctx: common.SmokeContext) -> list[bytes]:
    return [d for (_, _, d) in common.scan_packets(ctx, ctx.serial_writes, "RTCM")]


def check_push_gave_up(ctx: common.SmokeContext) -> None:
    """A push entry the caster permanently rejects makes the daemon give up.

    Guards the fix that a "Bad Password" rejection is fatal: the daemon logs
    "ntrip push gave up" and stops, rather than reconnecting forever (which
    logged "ntrip push connect failed", not allowlisted, so also caught).
    """
    def attempt() -> bool:
        with open(ctx.daemon_log, errors="replace") as f:
            return "ntrip push gave up" in f.read()

    assert common.poll(attempt), "daemon did not give up on the rejected push"


def _pushed_rtcm(ctx: common.SmokeContext) -> list[bytes]:
    return [d for (_, _, d) in common.scan_packets(ctx, ctx.caster_capture, "RTCM")]
