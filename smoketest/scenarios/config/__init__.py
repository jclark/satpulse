"""Checks for the config family: the startup and interactive config paths.

Both scenarios run against the u-blox simulator (PROVIDER ubxsim), the one
packet source that answers probes and config, so these checks assert on the
config wiring a read-only replay cannot reach: the receiver identity the program
reports (active detection carried the personality's MON-VER model and firmware)
and message enablement landing as live satellite data. The personality's Default
layer leaves NAV-SAT off, so satellite data appearing can only mean a VALSET
enabled it.
"""

from __future__ import annotations

import json
import threading
import time
import urllib.request
from typing import Callable, cast

import common

# The identity the simulator's MON-VER carries
# (gps/app/ubxsim/testdata/f9p/f9p-personality.ubx, a real ZED-F9P recording).
SIM_VENDOR = "u-blox"
SIM_MODEL = "ZED-F9P"
SIM_FIRMWARE = "HPG 1.51"


def read_sse(
    url: str, done: Callable[[dict[str, list[str]]], bool], read_seconds: float = 15.0
) -> dict[str, list[str]]:
    """Read an SSE stream into {event-name: [data-line, ...]} until done or timeout.

    A background reader accumulates each event's data payloads; the caller's
    done() predicate inspects the accumulator and stops the read as soon as its
    condition holds, else the read ends at read_seconds. Both config checks watch
    a live stream the simulator keeps feeding, so this replaces common.collect_sse
    (which returns only event names) where the data payload is what matters.
    """
    got: dict[str, list[str]] = {}
    try:
        resp = urllib.request.urlopen(url, timeout=read_seconds + 2)
    except OSError as e:
        raise AssertionError(f"could not open SSE stream {url}: {e}")
    name = ""

    def reader() -> None:
        nonlocal name
        try:
            for raw in resp:
                line = raw.decode("utf-8", "replace").rstrip("\n")
                if line.startswith("event: "):
                    name = line[len("event: ") :]
                elif line.startswith("data: ") and name:
                    got.setdefault(name, []).append(line[len("data: ") :])
        except Exception:
            pass

    t = threading.Thread(target=reader, daemon=True)
    t.start()
    deadline = time.time() + read_seconds
    while time.time() < deadline and not done(got):
        time.sleep(0.1)
    resp.close()
    t.join(timeout=2)
    return got


def _assert_identity(rcv: dict[str, object], where: str) -> None:
    assert rcv.get("vendor") == SIM_VENDOR, f"{where}: vendor {rcv.get('vendor')!r} != {SIM_VENDOR!r}"
    assert rcv.get("hardware") == SIM_MODEL, f"{where}: hardware {rcv.get('hardware')!r} != {SIM_MODEL!r}"
    fw = cast(str, rcv.get("firmware") or "")
    assert SIM_FIRMWARE in fw, f"{where}: firmware {fw!r} does not contain {SIM_FIRMWARE!r}"


def check_daemon_startup_config(ctx: common.SmokeContext) -> None:
    """Active detection identified the simulator, and the startup config landed.

    Reads the daemon's SSE while the simulator emits. The sticky `init` event
    (written to every new client) carries the receiver identity active detection
    learned: a read-only port would probe nothing and leave it empty, so a
    populated identity proves the probe ran and matched the simulated ZED-F9P.
    The live `satellites` event proves the daemon's startup VALSET enabled
    NAV-SAT, which the personality defaults leave off -- so the satellite data
    can only be the startup ConfigTarget landing on the receiver.
    """

    def seen(g: dict[str, list[str]]) -> bool:
        return bool(g.get("init")) and bool(g.get("satellites"))

    got = read_sse(ctx.http_url("/sse"), seen, read_seconds=20.0)
    inits = got.get("init", [])
    assert inits, "no SSE init event: active detection did not identify the receiver"
    rcv = None
    for d in inits:
        r = json.loads(d).get("receiver")
        if r:
            rcv = r
            break
    assert rcv, f"init event carried no receiver identity: {inits[:2]}"
    _assert_identity(rcv, "daemon init event")
    assert got.get("satellites"), (
        "no satellites SSE event: the startup config did not enable NAV-SAT "
        "(off in the personality defaults, so its absence means the VALSET did not land)"
    )


def check_wb_receiver_identity(ctx: common.SmokeContext, timeout: float = 15.0) -> None:
    """Poll /api/receiver until it identifies the simulated ZED-F9P.

    The interactive connect probes the pty and the simulator answers, so the
    session runs active detection and populates Info -- the first black-box
    exercise of an interactive connect whose probe is answered.
    """

    def attempt() -> dict[str, object] | None:
        st, b = common.wb_get(ctx, "/api/receiver")
        if st != 200:
            return None
        r = cast("dict[str, object]", json.loads(b))
        if not r.get("ok"):
            return None
        info = r.get("Info")
        return cast("dict[str, object]", info) if isinstance(info, dict) else None

    info = common.poll(attempt, timeout=timeout)
    assert info, "/api/receiver never identified a receiver"
    _assert_identity(info, "wb /api/receiver Info")


def check_wb_sats_live(ctx: common.SmokeContext, read_seconds: float = 15.0) -> None:
    """A live gps:msg of kind satellites appears once ApplyConfig enables it.

    The personality leaves NAV-SAT off, so the session can only surface
    satellites after the apply's Opts.SatsMsg VALSET enables them on the
    simulator; a live gps:msg kind=satellites on the workbench SSE stream is
    that enablement showing up as data.
    """
    marker = '"kind":"satellites"'

    def done(g: dict[str, list[str]]) -> bool:
        return any(marker in d for d in g.get("gps:msg", []))

    got = read_sse(ctx.wb_url("/sse"), done, read_seconds=read_seconds)
    assert any(marker in d for d in got.get("gps:msg", [])), (
        f"no live gps:msg kind=satellites after enabling Opts.SatsMsg; saw {got.get('gps:msg', [])[:3]}"
    )
