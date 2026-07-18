"""The satpulsewb program under test (see program_api).

satpulsewb serves SatPulse Workbench over HTTP. Its input is a rendered flag
list rather than a config file, its peers are declared by the scenario rather
than derived from a config, and readiness is the printed URL and access token
(parsed from the program's stdout) plus its one HTTP port accepting. This
module owns those per-program steps; the shared run-scenario machinery lives
in run.py.
"""

from __future__ import annotations

import os
import re
import socket
import time
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from run import Context, ScenarioModule

import program_api

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# The workbench URL line satpulsewb prints at startup, e.g.
#   http://127.0.0.1:15754/?t=RM3YmI3_Hl-G5EbdrlkmiA
# The bracketed alternative matches an IPv6 host (net.JoinHostPort form); the
# token group is absent when the token is disabled (-L without -t).
_URL_RE = re.compile(r"http://(?:\[[^\]]+\]|[^\s/]+?):(\d+)/(?:\?t=(\S+))?")

# satpulsewb runs with neither a PTP hardware clock nor NTP, so it never emits
# the daemon's clock/detection warnings. On a read-only FIFO it does warn that
# it will not configure the receiver; on a writable pty the probe is answered
# and it stays silent. A scenario adds its own ALLOWED_ERRORS on top (e.g. the
# read error a device-loss scenario provokes).
_ALLOWED = ("device is not writable, not configuring GPS",)


class Satpulsewb:
    """SatPulse Workbench: flag-list input, declared peers, URL/token readiness."""

    name = "satpulsewb"

    def prepare(self, ctx: Context, scen: ScenarioModule) -> None:
        """Render the flag-list template into ctx.wb_args.

        One flag token per line, ${SATPULSE_TEST_*} substituted; blank lines and
        # comments are ignored, so the template can be laid out and annotated
        like the daemon's .toml.in.
        """
        with open(ctx.template_base + ".args.in") as f:
            text = program_api.substitute(f.read(), ctx.env)
        ctx.wb_args = [
            line.strip()
            for line in text.splitlines()
            if line.strip() and not line.lstrip().startswith("#")
        ]

    def command(self, ctx: Context) -> list[str]:
        return [ctx.prog_bin, *ctx.wb_args]

    def start_peers(self, ctx: Context, scen: ScenarioModule) -> None:
        """Start the scenario's declared correction source, if any.

        satpulsewb starts corrections on demand from the UI, not at startup, so
        the source only has to exist before the scenario POSTs corrections/start;
        starting it before the program (like the daemon's pull source) is
        harmless and keeps the runner's peers-before-program order.
        """
        src = getattr(scen, "CORRECTION_SOURCE", None)
        if src is None:
            return
        log = src["log"]
        if not os.path.isabs(log):
            log = os.path.join(REPO, log)
        # The reused stream.check_pulled_rtcm scans the captured serial writes
        # against this source log.
        ctx.pull_source_log = log
        ctx.start_correction_source(
            port=ctx.port(src["port_key"]),
            log=log,
            mode=src.get("mode", "ntrip"),
            mountpoint=src.get("mountpoint", ""),
            username=src.get("username", ""),
            password=src.get("password", ""),
            require_gga=src.get("require_gga", False),
        )

    def wait_ready(self, ctx: Context) -> None:
        """Parse the printed URL and token, then wait for the HTTP port.

        satpulsewb prints the workbench URL (with the per-run token when there is
        one) to stdout as soon as it binds, before it serves. Parsing that line
        fixes the port and token the scenario's checks use -- important for the
        no-`-L` default-port path, where the bound port is chosen at runtime
        (the canonical port, or an OS-picked fallback when it is taken).
        """
        deadline = time.time() + 15
        while True:
            m = self._match_url(ctx.daemon_log)
            if m is not None:
                ctx.wb_port = int(m.group(1))
                ctx.token = m.group(2) or ""
                break
            if ctx.daemon is not None and ctx.daemon.poll() is not None:
                raise RuntimeError(
                    f"satpulsewb exited before printing its URL (code {ctx.daemon.returncode})"
                )
            if time.time() >= deadline:
                raise RuntimeError("satpulsewb did not print its URL within 15s")
            time.sleep(0.05)
        # The listener is bound (the URL is printed after net.Listen), so a
        # connect succeeds from the backlog even before Serve accepts; poll it
        # so the scenario's first request cannot race the bind.
        while _port_free(ctx.wb_port):
            if ctx.daemon is not None and ctx.daemon.poll() is not None:
                raise RuntimeError(
                    f"satpulsewb exited before listening (code {ctx.daemon.returncode})"
                )
            if time.time() >= deadline:
                raise RuntimeError(f"satpulsewb did not listen on {ctx.wb_port} within 15s")
            time.sleep(0.05)

    def _match_url(self, log_path: str) -> re.Match[str] | None:
        try:
            with open(log_path, errors="replace") as f:
                return _URL_RE.search(f.read())
        except OSError:
            return None

    def allowed_errors(self) -> tuple[str, ...]:
        return _ALLOWED

    def ports_to_free(self, ctx: Context) -> list[int]:
        return [ctx.wb_port]


def _port_free(port: int) -> bool:
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=0.3):
            return False
    except OSError:
        return True
