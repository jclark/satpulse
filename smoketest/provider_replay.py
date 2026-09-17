"""The replay packet provider (see provider_api).

The default provider: a recorded packet log streamed in real time through the
serial transport. It owns transport selection, the single `pack --realtime`
replay with its one-replay-per-lifetime invariant, the wait-replay backstop,
and the PACKET_LOG/FACTOR scenario attributes -- everything the runner used to
own inline, now behind the provider seam so run.py carries no PROVIDER branch.

The transport mechanics (feeding pack, draining and optionally capturing the
program's writes, disconnect, and the FIFO/pty distinction) stay on the runner
Context and the platform module; this provider decides which transport the
scenario's capabilities call for, when the replay runs, and when to wait for it.
"""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

if os.name == "posix":
    import platform_unix as plat
else:  # pragma: no cover - the runner already refuses a non-posix host
    raise RuntimeError(f"smoke tests do not yet support os.name={os.name!r}")

if TYPE_CHECKING:
    from run import Context, ScenarioModule

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


class Replay:
    """Realtime packet-log replay over the FIFO/pty transport."""

    def create(self, ctx: Context, scen: ScenarioModule) -> None:
        """Select and attach the transport, and record the replay attributes.

        The transport follows from the capabilities the scenario declares, not
        a named mechanism: CAPTURE_WRITES (a write-path scenario recording what
        the program sent back) needs a read-write transport; SELF_SHUTDOWN and
        DISCONNECTABLE both need a transport that can model the device vanishing.
        They are orthogonal -- serial-loss disconnects without capturing, the
        stream/pull-* scenarios capture without disconnecting -- and the platform
        maps them to a concrete transport (FIFO for neither, pty for either) or
        reports the request unsupported (a SKIP). PACKET_LOG/FACTOR are read
        here rather than in run.py, so the runner reaches them through the
        provider like any other source attribute.
        """
        capture_writes = bool(getattr(scen, "CAPTURE_WRITES", False))
        # DISCONNECTABLE requests the pty's disconnect capability without the
        # self-shutdown lifecycle: satpulsewb's device-loss scenario disconnects
        # and then asserts the program keeps running (SELF_SHUTDOWN's inverse),
        # so it needs the pty but not the no-signal exit check.
        disconnectable = bool(getattr(scen, "SELF_SHUTDOWN", False)) or bool(
            getattr(scen, "DISCONNECTABLE", False)
        )
        packet_log = scen.PACKET_LOG
        if not os.path.isabs(packet_log):
            packet_log = os.path.join(REPO, packet_log)
        ctx.packet_log = packet_log
        ctx.env["SATPULSE_TEST_PACKET_LOG"] = packet_log
        ctx.factor = scen.FACTOR
        # make_transport reads the FIFO path the runner allocated; on a pty it
        # ignores it and names the slave. Point SATPULSE_TEST_SERIAL at whatever
        # path the program must open before program.prepare renders it.
        ctx._transport = plat.make_transport(
            ctx.env["SATPULSE_TEST_SERIAL"], readwrite=capture_writes, disconnectable=disconnectable
        )
        ctx.env["SATPULSE_TEST_SERIAL"] = ctx._transport.path()
        # Enable capture before attaching so the transport captures from the
        # first byte; attach takes ownership (a pty starts its drain thread)
        # before the program starts.
        if capture_writes:
            ctx.start_write_capture()
        ctx._transport.attach()

    def start(self, ctx: Context) -> None:
        """Launch the single background replay, once the program is observable."""
        ctx.start_replay()

    def finish(self, ctx: Context) -> None:
        """Backstop the suite invariant that the replay finished cleanly."""
        ctx.wait_replay()

    def close(self, ctx: Context) -> None:
        """Stop the replay and release the transport. Idempotent."""
        if ctx.replay_proc is not None and ctx.replay_proc.poll() is None:
            ctx.replay_proc.kill()
        ctx._close_replay_files()
        if ctx._transport is not None:
            ctx._transport.close()
