"""Packet-provider seam for the smoke-test runner.

What plays the receiver behind SATPULSE_TEST_SERIAL is a scenario dimension,
orthogonal to the program under test: `replay` by default (a recorded packet
log streamed through a FIFO or pty), `ubxsim` when a scenario sets PROVIDER =
"ubxsim" (the u-blox receiver simulator behind a pty, which answers probes and
config, so the config path -- probe identification, ReadConfig, ApplyConfig,
and the daemon's startup config phase -- gets black-box coverage a read-only
replay cannot reach).

The Provider owns how receiver bytes are produced and consumed, and nothing
else: the serial endpoint, the feed lifecycle, and the source-side scenario
attributes (PACKET_LOG/FACTOR for replay, PERSONALITY/SIM_REPLAY for ubxsim).
run.py's replay lifecycle -- transport selection, the single pack replay, the
one-replay-per-lifetime invariant, and the wait-replay backstop -- lives behind
this seam so run.py stays free of PROVIDER == "..." branches, exactly as the
program seam keeps it free of PROGRAM branches.

This is the provider counterpart to program_api. Like a Program (and unlike the
one-per-process platform module), a Provider is chosen per scenario, so both
providers coexist in one process and share this Protocol; run.py holds one per
scenario and calls it polymorphically.

The four hooks mirror the run_scenario lifecycle:

  - create: before program.prepare -- make the transport or spawn the
    simulator, and point SATPULSE_TEST_SERIAL at the endpoint. run_scenario
    constructs the Context first so the provider can be handed it.
  - start: after program.wait_ready -- begin feeding packets (launch the pack
    replay; a no-op for ubxsim, whose simulator already runs).
  - finish: the post-run backstop -- wait for the feed to drain (a no-op for
    ubxsim).
  - close: the cleanup path -- release the transport, or SIGTERM the simulator.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Protocol

if TYPE_CHECKING:
    from run import Context, ScenarioModule


class Provider(Protocol):
    """The packet source and the per-provider steps around feeding it."""

    def create(self, ctx: Context, scen: ScenarioModule) -> None:
        """Set up the packet source and point SATPULSE_TEST_SERIAL at its endpoint.

        Runs before program.prepare (which renders input referencing the serial
        path). The replay provider selects and attaches the transport; the
        ubxsim provider spawns the simulator and waits for its symlink. A
        platform that cannot honour the scenario raises
        platform_api.TransportUnsupported, which the runner reports as SKIP.
        """
        ...

    def start(self, ctx: Context) -> None:
        """Begin feeding packets, after the program is observable (wait_ready)."""
        ...

    def finish(self, ctx: Context) -> None:
        """Post-run backstop: wait for the feed to drain cleanly."""
        ...

    def close(self, ctx: Context) -> None:
        """Release the provider's resources. Idempotent."""
        ...


def select(name: str) -> Provider:
    """Return the Provider for a scenario's PROVIDER name (default replay)."""
    if name == "replay":
        import provider_replay

        return provider_replay.Replay()
    if name == "ubxsim":
        import provider_ubxsim

        return provider_ubxsim.Ubxsim()
    raise RuntimeError(f"unknown PROVIDER {name!r}")
