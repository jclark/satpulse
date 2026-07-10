"""The satpulsed program under test (see program_api).

satpulsed is the default program. Its input is a rendered TOML config, its
peers and listeners are derived from that config, and readiness is the
configured listeners accepting connections. This module owns those
per-program steps; the shared run-scenario machinery lives in run.py.
"""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

import common
import program_api

if TYPE_CHECKING:
    from run import Context, ScenarioModule

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


class Satpulsed:
    """The satpulsed daemon: config-driven input, peers, and listeners."""

    name = "satpulsed"

    def prepare(self, ctx: Context, scen: ScenarioModule) -> None:
        """Render the TOML config and detect which listeners/peers it configures.

        Detection reads non-comment table headers so a section merely mentioned
        in a comment is not mistaken for a real one; has_http2 keys off a second
        [[http]] table and has_pull off the [stream.pull...] prefix.
        """
        with open(ctx.template_base + ".toml.in") as f:
            text = program_api.substitute(f.read(), ctx.env)
        with open(ctx.env["SATPULSE_TEST_CONFIG"], "w") as f:
            f.write(text)
        headers = [
            line.strip()
            for line in text.splitlines()
            if not line.lstrip().startswith("#")
        ]
        ctx.has_http = "[[http]]" in headers
        ctx.has_http2 = headers.count("[[http]]") >= 2
        ctx.has_ntrip = "[ntrip]" in headers
        ctx.has_ntp_sock = any(line.startswith("sock.path") for line in headers)
        ctx.has_push = "[[stream.push]]" in headers
        ctx.has_pull = any(line.startswith("[stream.pull") for line in headers)
        # The RTCM log a [stream.pull] correction source streams, and the
        # source implementation; both are scenario attributes here rather than
        # config, since the config only names the peer's address.
        pull_source_log = getattr(scen, "PULL_SOURCE_LOG", "")
        if pull_source_log and not os.path.isabs(pull_source_log):
            pull_source_log = os.path.join(REPO, pull_source_log)
        ctx.pull_source_log = pull_source_log
        ctx.pull_peer = getattr(scen, "PULL_PEER", "fake")

    def command(self, ctx: Context) -> list[str]:
        return [ctx.prog_bin, "-v", "-f", ctx.env["SATPULSE_TEST_CONFIG"]]

    def start_peers(self, ctx: Context, scen: ScenarioModule) -> None:
        """Start the peers the config asks for, each before the daemon starts.

        The chrony SOCK consumer, the fake push casters, and the fake pull
        correction source must all be listening before the daemon, so its first
        sample/connect lands rather than warning and backing off.
        """
        if ctx.has_ntp_sock:
            ctx.start_ntp_sock()
        if ctx.has_push:
            ctx.start_push_peers()
        if ctx.has_pull:
            ctx.start_source()

    def wait_ready(self, ctx: Context) -> None:
        """Wait for the configured listeners, and for outbound push if any.

        Both gate replay: the inbound SSE/Ntrip observers must exist and the
        push subscription must be in place before any packet flows, so live
        checks cannot race startup.
        """
        ctx.wait_listeners()
        if ctx.has_push:
            ctx.wait_push()

    def allowed_errors(self) -> tuple[str, ...]:
        return common.ALLOWED_WARNINGS

    def ports_to_free(self, ctx: Context) -> list[int]:
        ports = []
        if ctx.has_http:
            ports.append(ctx.http_port)
        if ctx.has_http2:
            ports.append(ctx.http_port2)
        return ports
