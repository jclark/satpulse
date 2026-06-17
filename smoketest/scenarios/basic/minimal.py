"""Minimal config: just start, replay, and shut down cleanly.

No scenario-specific checks: the runner verifies a clean replay, an
error-free daemon log, and a clean shutdown for every scenario.
"""

import common

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 10


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()
