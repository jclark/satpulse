"""Common smoke-test checks.

Checks are deliberately shallow black-box checks of the real daemon
process: they catch gross breakage (panics, failed wiring, missing
listeners, broken shutdown, missing logs, unusable endpoints), not
detailed packet/protocol behaviour, which belongs in package tests.

Observable-state checks poll until the condition holds or a deadline
expires, because the daemon can expose an intermediate state before
replay has produced the data a check needs. Scenario-family-specific
checks live beside the scenarios that use them under scenarios/.
"""

from __future__ import annotations

import json
import os
import subprocess
import threading
import time
import urllib.error
import urllib.request
from typing import Callable, Iterable, Protocol, Sequence, TypeAlias, TypeVar, cast

DEFAULT_TIMEOUT = 15.0

Auth: TypeAlias = tuple[str, str]
JsonObject: TypeAlias = dict[str, object]
Packet: TypeAlias = tuple[str, str | None, bytes]
T = TypeVar("T")


class SmokeContext(Protocol):
    """Runner context shape used by reusable checks."""

    name: str
    run_dir: str
    packet_log: str
    daemon_log: str
    ntp_log: str
    caster_capture: str
    caster_log: str
    udp_capture: str
    udp_log: str
    satpulsetool: str
    serial_writes: str
    source_log: str
    pull_source_log: str

    @property
    def serial(self) -> str: ...

    @property
    def log_dir(self) -> str: ...

    @property
    def ntrip_port(self) -> int: ...

    @property
    def proxy_socket(self) -> str: ...

    def http_url(self, path: str) -> str: ...

    def port(self, key: str) -> int: ...

    @property
    def ntp_shm_segment(self) -> int: ...

    def root_cmd(self, cmd: Sequence[str]) -> list[str]: ...

    def wait_replay(self, timeout: float = 60) -> None: ...

    def disconnect(self) -> None: ...

    def wait_exit(self, timeout: float = 10) -> int: ...


def poll(fn: Callable[[], T | None], timeout: float = DEFAULT_TIMEOUT, interval: float = 0.1) -> T | None:
    """Call fn() until it returns a truthy value or the deadline passes.

    Returns fn()'s last value. fn should return None/False to keep
    waiting and any truthy value to stop.
    """
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        last = fn()
        if last:
            return last
        time.sleep(interval)
    return last


def http_get(url: str, timeout: float = 2.0, auth: Auth | None = None) -> tuple[int | None, bytes]:
    """GET url, returning (status, body-bytes). Returns (None, b"") on error."""
    req = urllib.request.Request(url)
    if auth:
        import base64

        token = base64.b64encode(f"{auth[0]}:{auth[1]}".encode()).decode()
        req.add_header("Authorization", "Basic " + token)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()
    except OSError:
        return None, b""


# --- HTTP endpoint checks ---------------------------------------------------


def _http_url(ctx: SmokeContext, path: str, port: int | None) -> str:
    """URL for an HTTP check: the primary endpoint, or an explicit port.

    A scenario with multiple [[http]] entries passes port to target a
    secondary endpoint; the default reuses the primary one.
    """
    if port is None:
        return ctx.http_url(path)
    return f"http://127.0.0.1:{port}{path}"


def check_position(ctx: SmokeContext, port: int | None = None) -> JsonObject:
    """/position eventually returns position JSON with a latitude."""

    def attempt() -> JsonObject | None:
        status, body = http_get(_http_url(ctx, "/position", port))
        if status == 200 and body:
            return cast(JsonObject, json.loads(body))
        return None

    pos = poll(attempt)
    assert pos, "/position never returned 200 with a body"
    assert "lat" in pos and "lon" in pos, f"/position missing lat/lon: {pos}"
    return pos


def check_metrics(ctx: SmokeContext, expect: Iterable[str] = (), port: int | None = None) -> str:
    """/metrics returns Prometheus text containing the expected metric names."""

    def attempt() -> str | None:
        status, body = http_get(_http_url(ctx, "/metrics", port))
        if status != 200:
            return None
        text = body.decode("utf-8", "replace")
        if all(name in text for name in expect):
            return text
        return None

    text = poll(attempt)
    assert text is not None, "/metrics did not return all expected names"
    return text


def check_html(ctx: SmokeContext, path: str = "/", port: int | None = None) -> bytes:
    """An HTTP path returns 200 and looks like HTML."""

    def attempt() -> bytes | None:
        status, body = http_get(_http_url(ctx, path, port))
        if status == 200 and b"<html" in body.lower():
            return body
        return None

    body = poll(attempt)
    assert body is not None, f"{path} did not return HTML"
    return body


def check_status(ctx: SmokeContext, path: str, want: int, port: int | None = None) -> None:
    """An HTTP path returns the wanted status code."""
    status, _ = http_get(_http_url(ctx, path, port))
    assert status == want, f"{path} expected {want}, got {status}"


def check_sse(ctx: SmokeContext, expect: Iterable[str] = (), read_seconds: float = 8.0) -> set[str]:
    """SSE stream delivers the expected event names while replay is flowing.

    SSE events are live-only, so this reads the stream while the runner's
    single background replay is still feeding the daemon.
    """
    want = set(expect)
    names: set[str] = set()
    url = ctx.http_url("/sse")
    try:
        resp = urllib.request.urlopen(url, timeout=read_seconds + 2)
    except OSError as e:
        raise AssertionError(f"could not open SSE stream: {e}")

    def reader() -> None:
        try:
            for raw in resp:
                line = raw.decode("utf-8", "replace").strip()
                if line.startswith("event: "):
                    names.add(line[len("event: ") :])
        except Exception:
            pass

    t = threading.Thread(target=reader, daemon=True)
    t.start()
    deadline = time.time() + read_seconds
    while time.time() < deadline and not want.issubset(names):
        time.sleep(0.1)
    resp.close()
    t.join(timeout=2)
    missing = want - names
    assert not missing, f"SSE missing events {sorted(missing)}; saw {sorted(names)}"
    return names


# --- Log file checks --------------------------------------------------------


def _log_path(ctx: SmokeContext, kind: str) -> str:
    base = os.path.basename(ctx.serial)
    return os.path.join(ctx.log_dir, f"{kind}.{base}.jsonl")


def check_event_log(ctx: SmokeContext, expect_types: Iterable[str] = ()) -> set[str]:
    """The event log exists and contains entries of the expected types.

    Each event-log entry is an envelope with a top-level "type" discriminator
    (such as "time", "posGeo", "navEpoch", "pulseEdge") and a "data" payload.
    """
    path = _log_path(ctx, "event")
    want = set(expect_types)

    def attempt() -> set[str] | None:
        if not os.path.exists(path):
            return None
        seen: set[str] = set()
        with open(path) as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                t = cast(JsonObject, json.loads(line)).get("type")
                if isinstance(t, str):
                    seen.add(t)
        if want.issubset(seen):
            return seen
        return None

    seen = poll(attempt)
    assert seen is not None, f"event log {path} missing types {sorted(want)}"
    return seen


def check_track_log(ctx: SmokeContext) -> JsonObject:
    """The track log exists and contains at least one position."""
    path = _log_path(ctx, "track")

    def attempt() -> JsonObject | None:
        if not os.path.exists(path):
            return None
        with open(path) as f:
            for line in f:
                if line.strip():
                    e = cast(JsonObject, json.loads(line))
                    if "lat" in e:
                        return e
        return None

    e = poll(attempt)
    assert e is not None, f"track log {path} has no position"
    return e


def check_packet_log(ctx: SmokeContext) -> None:
    """The packet log exists and is non-empty."""
    path = _log_path(ctx, "packet")
    ok = poll(lambda: os.path.exists(path) and os.path.getsize(path) > 0)
    assert ok, f"packet log {path} missing or empty"


# --- Daemon output check ----------------------------------------------------

# Log lines expected under hardware-free FIFO replay: no PHC, and GPS
# detection times out until packets start flowing.
ALLOWED_WARNINGS = (
    "running without a PTP hardware clock",
    "GPS detection failed",
    "no output detected",
)


def check_no_unexpected_errors(ctx: SmokeContext, allowed: Iterable[str] = ()) -> None:
    """The daemon log has no panics or unexpected error/warn lines.

    allowed adds scenario-specific substrings (on top of ALLOWED_WARNINGS) for
    error/warn lines a scenario expects.
    """
    with open(ctx.daemon_log, "r", errors="replace") as f:
        lines = f.read().splitlines()
    allow = tuple(allowed)
    bad = []
    for line in lines:
        low = line.lower()
        if "panic" in low or "goroutine" in low and "[running]" in low:
            bad.append(line)
            continue
        if "level=error" in low or "level=warn" in low or " err=" in low:
            if not any(a in line for a in ALLOWED_WARNINGS) and not any(a in line for a in allow):
                bad.append(line)
    assert not bad, "unexpected daemon log lines:\n" + "\n".join(bad)


# --- Packet helpers ---------------------------------------------------------


def log_packets(path: str, tag: str | None = None) -> list[Packet]:
    """Input packets from a JSONL packet log as (tag, msg, raw-bytes).

    raw-bytes is the on-wire packet: decoded hex for binary protocols
    (UBX, RTCM) or the encoded string for ASCII protocols (NMEA). Skips
    metadata, sent (`out`), and non-tagged lines; optionally filters to a
    single tag.
    """
    out: list[Packet] = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                e = cast(JsonObject, json.loads(line))
            except json.JSONDecodeError:
                continue
            ptag = e.get("tag")
            if e.get("out") or not isinstance(ptag, str):
                continue
            if tag and ptag != tag:
                continue
            msg = e.get("msg")
            if msg is not None and not isinstance(msg, str):
                msg = None
            data = packet_data(e)
            if data is None:
                continue
            out.append((ptag, msg, data))
    return out


def log_packet_data(path: str) -> list[bytes]:
    """Input packet data from a JSONL packet log, without protocol filtering."""
    out: list[bytes] = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                e = cast(JsonObject, json.loads(line))
            except json.JSONDecodeError:
                continue
            if e.get("out"):
                continue
            data = packet_data(e)
            if data is not None:
                out.append(data)
    return out


def log_packet_bytes(path: str) -> bytes:
    """Input packet bytes from a JSONL packet log, without protocol filtering."""
    return b"".join(log_packet_data(path))


def scan_packets(ctx: SmokeContext, path: str, tag: str | None = None) -> list[Packet]:
    """Packets that `satpulsetool scan` finds in a raw byte-stream file.

    Returns (tag, msg, raw-bytes) tuples like log_packets, but the input is a
    raw on-wire byte stream (e.g. a captured push feed) rather than a JSONL
    packet log; scan turns it back into packets. Empty if the file is absent.
    """
    if not os.path.exists(path):
        return []
    p = subprocess.run(
        [ctx.satpulsetool, "scan", path],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30,
    )
    out: list[Packet] = []
    for line in p.stdout.decode("utf-8", "replace").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            e = cast(JsonObject, json.loads(line))
        except json.JSONDecodeError:
            continue
        ptag = e.get("tag")
        if e.get("out") or not isinstance(ptag, str):
            continue
        if tag and ptag != tag:
            continue
        msg = e.get("msg")
        if msg is not None and not isinstance(msg, str):
            msg = None
        data = packet_data(e)
        if data is None:
            continue
        out.append((ptag, msg, data))
    return out


def packet_data(e: JsonObject) -> bytes | None:
    b = e.get("bin")
    if isinstance(b, str):
        return bytes.fromhex(b)
    s = e.get("ascii")
    if isinstance(s, str):
        return s.encode("latin1")
    return None
