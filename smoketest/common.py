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
import re
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
    # satpulsewb only: the bound HTTP port and access token (empty when
    # disabled), set at readiness from the printed URL.
    wb_port: int
    token: str

    @property
    def serial(self) -> str: ...

    @property
    def log_dir(self) -> str: ...

    @property
    def ntrip_port(self) -> int: ...

    @property
    def proxy_socket(self) -> str: ...

    def http_url(self, path: str) -> str: ...

    def wb_url(self, path: str) -> str: ...

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
    return collect_sse(ctx.http_url("/sse"), expect, read_seconds)


def collect_sse(url: str, expect: Iterable[str] = (), read_seconds: float = 8.0) -> set[str]:
    """Read an SSE stream, returning the event names seen up to a deadline.

    Returns as soon as every name in expect has appeared, else after
    read_seconds; asserts none of expect is missing. Shared by the daemon's
    /sse check and the satpulsewb workbench SSE checks (which pass a URL with
    the access token and, for the packets stream, ?stream=packets).
    """
    want = set(expect)
    names: set[str] = set()
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
    (such as "time", "posGeo", "navEpoch", "phcPulseEdge") and a "data" payload.
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


def check_no_unexpected_errors(
    ctx: SmokeContext, allowed: Iterable[str] = (), base: Iterable[str] = ALLOWED_WARNINGS
) -> None:
    """The program log has no panics or unexpected error/warn lines.

    base is the program's own allow-list (the daemon's clock/detection warnings
    by default, satpulsewb's for the workbench); allowed adds the
    scenario-specific substrings on top.
    """
    with open(ctx.daemon_log, "r", errors="replace") as f:
        lines = f.read().splitlines()
    allow = tuple(base) + tuple(allowed)
    bad = []
    for line in lines:
        low = line.lower()
        if "panic" in low or "goroutine" in low and "[running]" in low:
            bad.append(line)
            continue
        if "level=error" in low or "level=warn" in low or " err=" in low:
            if not any(a in line for a in allow):
                bad.append(line)
    assert not bad, "unexpected program log lines:\n" + "\n".join(bad)


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


def is_contiguous_sublist(whole: list[bytes], part: list[bytes]) -> bool:
    """True if the non-empty part appears as a contiguous run within whole.

    An empty part returns False: a real Ntrip peer joining a live stream
    mid-flight relays a contiguous window of the source, byte-for-byte. An
    interop check wants that non-empty window to match the source, not a
    vacuous empty match.
    """
    n = len(part)
    if n == 0:
        return False
    return any(whole[i:i + n] == part for i in range(len(whole) - n + 1))


# --- SatPulse Workbench (satpulsewb) checks ---------------------------------
#
# The workbench is an HTTP+SSE server over gps/app/session. These checks reach
# it through ctx.wb_url (which carries the access token when there is one) and
# span families (http, stream, shutdown), so they live here rather than in one
# family. The receiver monitor events they observe are the session's, driven by
# the same replay the daemon scenarios use.


def wb_get(ctx: SmokeContext, path: str, timeout: float = 2.0) -> tuple[int | None, bytes]:
    """GET a satpulsewb endpoint with the access token when there is one."""
    return http_get(ctx.wb_url(path), timeout=timeout)


def wb_post(ctx: SmokeContext, path: str, body: JsonObject, timeout: float = 4.0) -> tuple[int | None, bytes]:
    """POST JSON to a satpulsewb endpoint with the token and application/json.

    The explicit Content-Type both satisfies the server's CSRF gate and passes
    the JSON body the session methods expect.
    """
    req = urllib.request.Request(ctx.wb_url(path), data=json.dumps(body).encode(), method="POST")
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()
    except OSError:
        return None, b""


def wb_claim(ctx: SmokeContext) -> str:
    """Claim the workbench write seat, returning the secret seat value.

    Writer POSTs (the mutating session operations) carry the current seat as a
    query parameter; reader POSTs, GETs, and SSE opens are seat-free. Newest
    claim wins unconditionally, so a check that needs to write just claims.
    """
    status, body = wb_post(ctx, "/api/seat", {})
    assert status == 200, f"seat claim expected 200, got {status}"
    seat = cast(JsonObject, json.loads(body)).get("seat")
    assert isinstance(seat, str) and seat, f"seat claim returned no seat: {body!r}"
    return seat


def check_wb_auth_required(ctx: SmokeContext) -> None:
    """The token-protected API rejects a request with no token and accepts one with it."""
    assert ctx.token, "check_wb_auth_required needs a token-enabled launch"
    status, _ = http_get(f"http://127.0.0.1:{ctx.wb_port}/api/state")
    assert status == 401, f"/api/state without a token expected 401, got {status}"
    status, _ = wb_get(ctx, "/api/state")
    assert status == 200, f"/api/state with the token expected 200, got {status}"


def check_wb_open_no_token(ctx: SmokeContext) -> None:
    """With the token disabled (-L, no -t), the API is reachable without a token."""
    assert not ctx.token, "check_wb_open_no_token needs a token-disabled launch"
    status, _ = http_get(f"http://127.0.0.1:{ctx.wb_port}/api/state")
    assert status == 200, f"/api/state with the token disabled expected 200, got {status}"


def check_wb_csrf(ctx: SmokeContext) -> None:
    """A POST without application/json is rejected (the CSRF gate).

    This holds even when the token is disabled, where auth is a no-op. The
    target is a reader POST, which has no seat requirement: the content-type
    gate is all that stops a cross-site page issuing a simple POST at it
    (writer POSTs are additionally guarded by the secret seat). urllib
    defaults an un-typed POST body to x-www-form-urlencoded, exactly the
    simple-request content type the gate must reject.
    """
    req = urllib.request.Request(ctx.wb_url("/api/signals"), data=b"x", method="POST")
    try:
        with urllib.request.urlopen(req, timeout=2) as resp:
            status: int | None = resp.status
    except urllib.error.HTTPError as e:
        status = e.code
    except OSError:
        status = None
    assert status == 415, f"non-JSON POST expected 415, got {status}"


def check_wb_html(ctx: SmokeContext) -> None:
    """The workbench serves its single-page app, like the daemon's check_html.

    The mirror of common.check_html(ctx, '/'): the root returns HTML. It also
    confirms the referenced script bundle is actually served, so a missing or
    empty embed fails here rather than passing on the index shell alone. Static
    assets are not token-gated, so this needs no token -- the same request the
    browser makes before it has applied one.
    """
    base = f"http://127.0.0.1:{ctx.wb_port}"
    status, body = http_get(base + "/")
    assert status == 200, f"/ expected 200, got {status}"
    text = body.decode("utf-8", "replace")
    assert "<html" in text.lower(), f"/ did not return HTML: {text[:200]!r}"
    m = re.search(r'<script[^>]+src="([^"]+)"', text)
    assert m, f"served HTML references no script bundle: {text[:200]!r}"
    status, _ = http_get(base + m.group(1))
    assert status == 200, f"script bundle {m.group(1)} expected 200, got {status}"


def check_wb_state(ctx: SmokeContext, want: str = "connected", timeout: float = DEFAULT_TIMEOUT) -> None:
    """/api/state reaches the wanted connection state."""

    def attempt() -> bool | None:
        status, body = wb_get(ctx, "/api/state")
        return status == 200 and json.loads(body) == want or None

    assert poll(attempt, timeout=timeout), f"/api/state did not reach {want!r}"


def check_wb_snapshots(ctx: SmokeContext) -> None:
    """The snapshot endpoints populate as the replay flows: connection + receiver."""
    check_wb_state(ctx, "connected")
    status, body = wb_get(ctx, "/api/connection")
    assert status == 200, f"/api/connection expected 200, got {status}"
    conn = cast(JsonObject, json.loads(body))
    assert conn == {"state": "connected", "device": ctx.serial, "speed": 38400}, (
        f"/api/connection did not retain startup controls: {conn}"
    )
    status, body = wb_get(ctx, "/api/receiver")
    assert status == 200, f"/api/receiver expected 200, got {status}"
    rcv = cast(JsonObject, json.loads(body))
    assert rcv.get("ok"), f"/api/receiver reports no detected receiver: {rcv}"


def wb_sse(ctx: SmokeContext, packets: bool = False, expect: Iterable[str] = (), read_seconds: float = 8.0) -> set[str]:
    """Collect SSE event names from a workbench stream while replay flows.

    packets selects the gps:packet stream (?stream=packets); otherwise the
    regular event stream (everything else).
    """
    path = "/sse?stream=packets" if packets else "/sse"
    return collect_sse(ctx.wb_url(path), expect, read_seconds)


def check_wb_packet_gating(ctx: SmokeContext) -> None:
    """The gps:packet stream is gated to the packets subscription (the Wants gate).

    A ?stream=packets client receives gps:packet, which the session emits only
    while such a client is connected; the regular event stream never carries it.
    """
    pkt = wb_sse(ctx, packets=True, expect=["gps:packet"])
    assert "gps:packet" in pkt, f"packets stream delivered no gps:packet; saw {sorted(pkt)}"
    main = wb_sse(ctx, packets=False, expect=["gps:state"], read_seconds=4.0)
    assert "gps:packet" not in main, "regular SSE stream carried gps:packet (gating failed)"


def check_wb_priming(ctx: SmokeContext, expect: Iterable[str] = ("gps:state", "gps:receiver", "gps:time", "gps:epochPVT")) -> None:
    """A late-joining SSE client is primed from the event cache.

    Call after wait_replay: on a FIFO the session stays connected but idle once
    the replay ends, so a fresh client can only be seeing primed sticky events,
    not live ones.
    """
    wb_sse(ctx, packets=False, expect=expect, read_seconds=5.0)


def check_wb_seat_takeover(ctx: SmokeContext) -> None:
    """A second seat claim supersedes the first without closing its stream.

    The current seat value is broadcast as the sticky `writer` event: opening a
    stream primes it, and claiming a second seat broadcasts the fresh value to
    the first, still-open stream (no stream is terminated). A writer POST still
    carrying the superseded seat is then refused with 410 (distinct from the
    409 used for session conflicts).
    """
    old = wb_claim(ctx)
    with urllib.request.urlopen(ctx.wb_url("/sse"), timeout=8) as resp:
        # urlopen returns once the headers are in, and the server subscribes
        # before sending them, so the second claim's seat cannot be in this
        # stream's prime: seeing it below proves live delivery. SSE data
        # buffers in the socket, so claiming before reading loses nothing.
        new_seat = wb_claim(ctx)
        pending = ""
        got = False
        try:
            for raw in resp:
                line = raw.decode("utf-8", "replace").strip()
                if line.startswith("event: "):
                    pending = line[len("event: ") :]
                elif line.startswith("data: ") and pending == "writer":
                    if json.loads(line[len("data: ") :]).get("seat") == new_seat:
                        got = True
                        break
                    pending = ""
        except TimeoutError:
            pass
        assert got, "new seat not delivered to the open stream"
    status, _ = wb_post(ctx, f"/api/disconnect?seat={old}", {})
    assert status == 410, f"POST with the superseded seat expected 410, got {status}"


def check_wb_msg_primed(ctx: SmokeContext, kind: str, read_seconds: float = 5.0) -> None:
    """A late SSE client is primed with a cached gps:msg of the given kind.

    Call after wait_replay: on an idle FIFO no live gps:msg flows, so a gps:msg
    the fresh client receives must come from the hub's sticky-kind cache
    (leapSecond/survey) -- the slow-changing state a reloaded browser relies on
    not having to wait for. Reads the event stream and matches the kind in the
    gps:msg data payload.
    """
    marker = f'"kind":"{kind}"'
    seen: list[str] = []
    try:
        resp = urllib.request.urlopen(ctx.wb_url("/sse"), timeout=read_seconds + 2)
    except OSError as e:
        raise AssertionError(f"could not open SSE stream: {e}")

    name = ""

    def reader() -> None:
        nonlocal name
        try:
            for raw in resp:
                line = raw.decode("utf-8", "replace").rstrip("\n")
                if line.startswith("event: "):
                    name = line[len("event: ") :]
                elif line.startswith("data: ") and name == "gps:msg":
                    seen.append(line[len("data: ") :])
        except Exception:
            pass

    t = threading.Thread(target=reader, daemon=True)
    t.start()
    deadline = time.time() + read_seconds
    while time.time() < deadline and not any(marker in d for d in seen):
        time.sleep(0.1)
    resp.close()
    t.join(timeout=2)
    assert any(marker in d for d in seen), (
        f"late SSE client was not primed with a gps:msg kind={kind!r}; saw {seen[:3]}"
    )


def check_wb_corr_state(ctx: SmokeContext, want: str = "connected", timeout: float = DEFAULT_TIMEOUT) -> None:
    """/api/corrections reaches the wanted corrections state (connecting -> connected)."""

    def attempt() -> bool | None:
        status, body = wb_get(ctx, "/api/corrections")
        if status != 200:
            return None
        return cast(JsonObject, json.loads(body)).get("state") == want or None

    assert poll(attempt, timeout=timeout), f"/api/corrections did not reach {want!r}"
