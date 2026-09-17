"""The u-blox simulator packet provider (see provider_api).

`satpulsetool ubxsim` hosts the u-blox receiver simulator (gps/app/ubxsim)
behind a pty: it answers UBX config and probe messages and emits nav messages
by replaying a bank, gated by its own MSGOUT configuration. That makes it the
one provider that can drive the config path -- probe identification, ReadConfig,
ApplyConfig, and the daemon's startup config phase -- which a read-only replay
cannot reach because gpscfg skips probing on a port nothing answers.

Lifecycle rationale: the simulator starts before the program (the pty must
exist when the program opens the device) and is SIGTERMed only after the
program has shut down. The simulator holds its own slave fd open, so program
restarts never EOF it; killing it early would instead inject read errors into
the program's shutdown. It is a test double, not a program under test, so its
stdout+stderr go to ubxsim.log in the run dir and are not error-scanned.

Capabilities: the simulator consumes and answers the program's writes, so write
capture is meaningless, and device-loss modelling stays with the replay provider
(a CFG-RST pty drop is a listed simulator extension, not this phase). The
provider therefore rejects CAPTURE_WRITES, SELF_SHUTDOWN and DISCONNECTABLE as
scenario errors. `satpulsetool ubxsim` builds on Linux and macOS only, so
elsewhere the provider reports the scenario unsupported -- the same SKIP as an
unsupported transport.
"""

from __future__ import annotations

import os
import signal
import subprocess
import sys
import time
from typing import IO, TYPE_CHECKING

from platform_api import TransportUnsupported

if TYPE_CHECKING:
    from run import Context, ScenarioModule

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# The simulator builds behind //go:build linux || darwin, so only these hosts
# can run it; anywhere else the scenario is unsupported (a SKIP), mirroring a
# transport the platform cannot supply.
_SUPPORTED = ("linux", "darwin")


class Ubxsim:
    """A pty-hosted u-blox receiver simulator as the packet source."""

    def __init__(self) -> None:
        self._proc: subprocess.Popen[bytes] | None = None
        self._log_file: IO[bytes] | None = None

    def create(self, ctx: Context, scen: ScenarioModule) -> None:
        """Spawn the simulator and point SATPULSE_TEST_SERIAL at its pty symlink.

        The symlink is the endpoint -- the /dev/serial/by-id shape gpsio already
        opens -- and its appearance is readiness: ubxsim creates it before it
        prints the slave path. The nav bank (SIM_REPLAY) is optional; without it
        the simulator is silent, like a receiver with no antenna, but the config
        scenarios always want nav content the startup config can enable.
        """
        for cap in ("CAPTURE_WRITES", "SELF_SHUTDOWN", "DISCONNECTABLE"):
            if getattr(scen, cap, False):
                raise RuntimeError(f"the ubxsim provider does not support {cap}")
        if sys.platform not in _SUPPORTED:
            raise TransportUnsupported(
                f"satpulsetool ubxsim is not built on {sys.platform!r} (Linux/macOS only)"
            )
        personality = getattr(scen, "PERSONALITY", "")
        if not personality:
            raise RuntimeError("a ubxsim scenario must declare PERSONALITY")
        if not os.path.isabs(personality):
            personality = os.path.join(REPO, personality)
        bank = getattr(scen, "SIM_REPLAY", "")
        if bank and not os.path.isabs(bank):
            bank = os.path.join(REPO, bank)
        # ctx.factor defaults to 1 for simulator scenarios (the NAV engine
        # consumes one epoch per second in real time; correction-source pacing
        # is factor's only other consumer). Record the bank as the packet log so
        # the SmokeContext still names a source, though nothing replays it here.
        ctx.factor = 1
        ctx.packet_log = bank

        link = os.path.join(ctx.run_dir, "gps.pty")
        cmd = [ctx.satpulsetool, "ubxsim", "--link", link]
        if bank:
            cmd += ["-r", bank]
        cmd.append(personality)
        self._log_file = open(os.path.join(ctx.run_dir, "ubxsim.log"), "wb")
        self._proc = subprocess.Popen(cmd, stdout=self._log_file, stderr=subprocess.STDOUT)
        self._wait_link(link)
        ctx.env["SATPULSE_TEST_SERIAL"] = link

    def _wait_link(self, link: str, timeout: float = 10) -> None:
        deadline = time.time() + timeout
        while not os.path.exists(link):
            if self._proc is not None and self._proc.poll() is not None:
                raise RuntimeError(
                    f"ubxsim exited before creating {link} (code {self._proc.returncode}); "
                    "see ubxsim.log"
                )
            if time.time() >= deadline:
                raise RuntimeError(f"ubxsim did not create {link} within {timeout:g}s")
            time.sleep(0.02)

    def start(self, ctx: Context) -> None:
        """No-op: the simulator generates nav itself, with no separate replay."""

    def finish(self, ctx: Context) -> None:
        """No-op: there is no replay to wait for."""

    def close(self, ctx: Context) -> None:
        """SIGTERM the simulator, after the program has already shut down."""
        if self._proc is not None and self._proc.poll() is None:
            self._proc.send_signal(signal.SIGTERM)
            try:
                self._proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self._proc.kill()
        self._proc = None
        if self._log_file is not None:
            self._log_file.close()
            self._log_file = None
