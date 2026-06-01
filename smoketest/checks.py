"""Reusable smoke-test checks.

Checks are deliberately shallow black-box checks of the real daemon
process: they catch gross breakage (panics, failed wiring, missing
listeners, broken shutdown, missing logs, unusable endpoints), not
detailed packet/protocol behaviour, which belongs in package tests.

Observable-state checks poll until the condition holds or a deadline
expires, because the daemon can expose an intermediate state before
replay has produced the data a check needs.
"""

import datetime
import json
import os
import subprocess
import threading
import time
import urllib.error
import urllib.request

DEFAULT_TIMEOUT = 15.0


def poll(fn, timeout=DEFAULT_TIMEOUT, interval=0.1):
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


def http_get(url, timeout=2.0, auth=None):
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


def check_position(ctx):
    """/position eventually returns position JSON with a latitude."""

    def attempt():
        status, body = http_get(ctx.http_url("/position"))
        if status == 200 and body:
            return json.loads(body)
        return None

    pos = poll(attempt)
    assert pos, "/position never returned 200 with a body"
    assert "lat" in pos and "lon" in pos, f"/position missing lat/lon: {pos}"
    return pos


def check_metrics(ctx, expect=()):
    """/metrics returns Prometheus text containing the expected metric names."""

    def attempt():
        status, body = http_get(ctx.http_url("/metrics"))
        if status != 200:
            return None
        text = body.decode("utf-8", "replace")
        if all(name in text for name in expect):
            return text
        return None

    text = poll(attempt)
    assert text is not None, "/metrics did not return all expected names"
    return text


def check_html(ctx, path="/"):
    """An HTTP path returns 200 and looks like HTML."""

    def attempt():
        status, body = http_get(ctx.http_url(path))
        if status == 200 and b"<html" in body.lower():
            return body
        return None

    body = poll(attempt)
    assert body is not None, f"{path} did not return HTML"
    return body


def check_status(ctx, path, want):
    """An HTTP path returns the wanted status code."""
    status, _ = http_get(ctx.http_url(path))
    assert status == want, f"{path} expected {want}, got {status}"


def check_sse(ctx, expect=(), read_seconds=8.0):
    """SSE stream delivers the expected event names while replay is flowing.

    SSE events are live-only, so this reads the stream while the runner's
    single background replay is still feeding the daemon.
    """
    names = set()
    url = ctx.http_url("/sse")
    try:
        resp = urllib.request.urlopen(url, timeout=read_seconds + 2)
    except OSError as e:
        raise AssertionError(f"could not open SSE stream: {e}")

    def reader():
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
    while time.time() < deadline and not set(expect).issubset(names):
        time.sleep(0.1)
    resp.close()
    t.join(timeout=2)
    missing = set(expect) - names
    assert not missing, f"SSE missing events {sorted(missing)}; saw {sorted(names)}"
    return names


# --- Log file checks --------------------------------------------------------


def _log_path(ctx, kind):
    base = os.path.basename(ctx.fifo)
    return os.path.join(ctx.log_dir, f"{kind}.{base}.jsonl")


def check_event_log(ctx, expect_keys=()):
    """The event log exists and contains entries with the expected keys.

    Event-log entries carry observation data under top-level keys such as
    "time", "posGeo", "velGeo", "navEpoch".
    """
    path = _log_path(ctx, "event")

    def attempt():
        if not os.path.exists(path):
            return None
        seen = set()
        with open(path) as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                seen.update(json.loads(line).keys())
        if set(expect_keys).issubset(seen):
            return seen
        return None

    seen = poll(attempt)
    assert seen is not None, f"event log {path} missing keys {expect_keys}"
    return seen


def check_track_log(ctx):
    """The track log exists and contains at least one position."""
    path = _log_path(ctx, "track")

    def attempt():
        if not os.path.exists(path):
            return None
        with open(path) as f:
            for line in f:
                if line.strip():
                    e = json.loads(line)
                    if "lat" in e:
                        return e
        return None

    e = poll(attempt)
    assert e is not None, f"track log {path} has no position"
    return e


def check_packet_log(ctx):
    """The packet log exists and is non-empty."""
    path = _log_path(ctx, "packet")
    e = poll(lambda: os.path.exists(path) and os.path.getsize(path) > 0)
    assert e, f"packet log {path} missing or empty"


# --- Daemon output check ----------------------------------------------------

# Log lines expected under hardware-free FIFO replay: no PHC, and GPS
# detection times out until packets start flowing.
ALLOWED_WARNINGS = (
    "running without a PTP hardware clock",
    "GPS detection failed",
    "no output detected",
)


def check_no_unexpected_errors(ctx):
    """The daemon log has no panics or unexpected error/warn lines."""
    with open(ctx.daemon_log, "r", errors="replace") as f:
        lines = f.read().splitlines()
    bad = []
    for line in lines:
        low = line.lower()
        if "panic" in low or "goroutine" in low and "[running]" in low:
            bad.append(line)
            continue
        if "level=error" in low or "level=warn" in low or " err=" in low:
            if not any(a in line for a in ALLOWED_WARNINGS):
                bad.append(line)
    assert not bad, "unexpected daemon log lines:\n" + "\n".join(bad)


# --- NTP chrony SOCK check --------------------------------------------------

NTP_SOCK_MAGIC = 0x534F434B  # "SOCK"


def _ntp_samples(ctx):
    """Decoded sock_sample records logged by ntpsock.py, in receive order."""
    out = []
    if not os.path.exists(ctx.ntp_log):
        return out
    with open(ctx.ntp_log) as f:
        for line in f:
            line = line.strip()
            if line:
                out.append(json.loads(line))
    return out


def _rmc_utcs(path):
    """UTC instants (epoch seconds) asserted by the RMC sentences in a log."""
    out = []
    for tag, msg, data in _log_packets(path):
        if tag != "NMEA" or not (msg or "").endswith("RMC"):
            continue
        f = data.decode("latin1").split(",")
        if len(f) < 10 or not f[1] or not f[9]:
            continue
        hms, dmy = f[1], f[9]
        dt = datetime.datetime(
            2000 + int(dmy[4:6]), int(dmy[2:4]), int(dmy[0:2]),
            int(hms[0:2]), int(hms[2:4]), int(hms[4:6]),
            tzinfo=datetime.timezone.utc,
        )
        out.append(dt.timestamp() + (float(hms[6:]) if len(hms) > 6 else 0.0))
    return out


def check_ntp_sock(ctx, min_samples=10, max_spread=0.05, max_time_error=0.1):
    """The chrony SOCK refclock stream is well-formed, consistent, and correct.

    Under realtime replay of a 1 Hz UTC time-message log, each sample carries
    the read instant (tv) and offset = utc - tv. Capture time and replay time
    differ, so the offset's absolute value is meaningless, but the daemon's
    notion of true time must be right: tv + offset (both from the sample) must
    land on an actual RMC instant from the log, proving the GPS time was parsed
    and represented correctly. This uses the daemon's own measurement time, so
    it does not depend on when the consumer was scheduled. Separately, a tight
    offset spread shows consistent timestamping, the cadence is ~1 s, and the
    receive time trails the measurement by under half a second (a delivery-
    latency sanity check, which also catches tv not being the real system clock).
    """
    samples = poll(lambda: (lambda s: s if len(s) >= min_samples else None)(_ntp_samples(ctx)))
    assert samples is not None, f"got fewer than {min_samples} NTP samples"
    for s in samples:
        assert s["len"] == 40, f"NTP sample wrong size {s['len']} != 40"
        assert s["magic"] == NTP_SOCK_MAGIC, f"bad magic {s['magic']:#x}"
        assert s["pulse"] == 0, f"unexpected pulse flag {s['pulse']}"
        assert s["leap"] in (0, 1, 2), f"bad leap {s['leap']}"
    tv = [s["sec"] + s["usec"] * 1e-6 for s in samples]
    offs = [s["offset"] for s in samples]
    spread = max(offs) - min(offs)
    assert spread <= max_spread, f"offset spread {spread * 1000:.1f} ms exceeds {max_spread * 1000:.0f} ms"
    for i in range(1, len(tv)):
        step = tv[i] - tv[i - 1]
        assert 0.5 < step < 1.5, f"measurement time step {step:.3f}s not ~1s at sample {i}"
    for s, t in zip(samples, tv):
        lag = s["recv"] - t
        assert 0 <= lag < 0.5, f"measurement-to-receipt lag {lag * 1000:.1f} ms not in [0, 500)"
    utcs = _rmc_utcs(ctx.packet_log)
    assert utcs, f"no RMC times in {ctx.packet_log}"
    for s, t in zip(samples, tv):
        recon = t + s["offset"]
        err = recon - min(utcs, key=lambda u: abs(u - recon))
        assert abs(err) <= max_time_error, (
            f"reconstructed true time off by {err * 1000:.1f} ms from any RMC instant "
            f"(exceeds {max_time_error * 1000:.0f} ms): GPS time parsed or represented wrong"
        )
    return spread


# --- Ntrip checks -----------------------------------------------------------


def ntrip_request(port, path, auth=None, timeout=2.0, read_bytes=65536):
    """Issue a raw Ntrip GET and return (status_code, body-bytes).

    The Ntrip caster replies with non-HTTP status lines such as
    "SOURCETABLE 200 OK" and "ICY 200 OK", which urllib cannot parse, so
    a raw socket is used. The status code is the first integer token of
    the response's first line.
    """
    import socket

    req = f"GET /{path} HTTP/1.0\r\nUser-Agent: NTRIP smoketest\r\n"
    if auth:
        import base64

        token = base64.b64encode(f"{auth[0]}:{auth[1]}".encode()).decode()
        req += f"Authorization: Basic {token}\r\n"
    req += "\r\n"
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=timeout) as s:
            s.sendall(req.encode())
            s.settimeout(timeout)
            chunks = []
            while len(b"".join(chunks)) < read_bytes:
                try:
                    b = s.recv(4096)
                except socket.timeout:
                    break
                if not b:
                    break
                chunks.append(b)
    except OSError:
        return None, b""
    body = b"".join(chunks)
    first = body.split(b"\r\n", 1)[0].decode("latin1")
    code = None
    for tok in first.split():
        if tok.isdigit():
            code = int(tok)
            break
    return code, body


def check_sourcetable(ctx, mount):
    """The Ntrip caster source table lists the expected mountpoint."""

    def attempt():
        status, body = ntrip_request(ctx.ntrip_port, "")
        if status == 200 and f"STR;{mount}".encode() in body:
            return body
        return None

    body = poll(attempt)
    assert body is not None, f"source table missing mountpoint {mount}"
    return body


def check_ntrip_stream(ctx, mount, auth=None, read_seconds=4.0):
    """An Ntrip client receives RTCM packets from the caster mountpoint.

    Uses satpulsetool ntrip as the client (also exercising that tool),
    reading the stream while the runner's background replay feeds RTCM
    through the daemon.
    """
    cmd = [ctx.satpulsetool, "ntrip"]
    if auth:
        cmd += ["--user", f"{auth[0]}:{auth[1]}"]
    cmd += [f"127.0.0.1:{ctx.ntrip_port}", mount]
    client = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    try:
        time.sleep(read_seconds)
    finally:
        client.terminate()
        try:
            out, _ = client.communicate(timeout=read_seconds)
        except subprocess.TimeoutExpired:
            client.kill()
            out, _ = client.communicate()
    rtcm = 0
    for line in out.decode("utf-8", "replace").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            e = json.loads(line)
        except json.JSONDecodeError:
            continue
        if e.get("tag") == "RTCM":
            rtcm += 1
    assert rtcm > 0, f"Ntrip client received no RTCM packets from {mount}"
    return rtcm


def check_ntrip_unauthorized(ctx, mount):
    """An unauthenticated request to a protected mountpoint is rejected."""
    status, _ = ntrip_request(ctx.ntrip_port, mount)
    assert status == 401, f"protected mount {mount} expected 401, got {status}"


# --- Serial proxy checks ----------------------------------------------------


def _log_packets(path, tag=None):
    """Input packets from a JSONL packet log as (tag, msg, raw-bytes).

    raw-bytes is the on-wire packet: decoded hex for binary protocols
    (UBX, RTCM) or the encoded string for ASCII protocols (NMEA). Skips
    metadata, sent (`out`), and non-tagged lines; optionally filters to a
    single tag.
    """
    out = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                e = json.loads(line)
            except json.JSONDecodeError:
                continue
            if e.get("out") or not e.get("tag"):
                continue
            if tag and e["tag"] != tag:
                continue
            if e.get("bin"):
                data = bytes.fromhex(e["bin"])
            elif e.get("ascii") is not None:
                data = e["ascii"].encode("latin1")
            else:
                continue
            out.append((e["tag"], e.get("msg"), data))
    return out


def check_proxy_tcp(ctx, port, protocol, read_seconds=3.0, connect_timeout=15.0):
    """A read-only TCP proxy on `port` filtered to `protocol` forwards that
    protocol's packets, and the bytes are a contiguous slice of the source log.

    The proxy comes up after GPS detection, so poll-connect until it
    accepts; the single successful connection is the reading client (no
    throwaway probe that would leave a subscriber to error on disconnect).
    """
    import socket

    deadline = time.time() + connect_timeout
    conn = None
    while time.time() < deadline:
        try:
            conn = socket.create_connection(("127.0.0.1", port), timeout=1)
            break
        except OSError:
            time.sleep(0.1)
    assert conn is not None, f"proxy TCP port {port} never accepted a connection"
    buf = b""
    conn.settimeout(read_seconds)
    stop = time.time() + read_seconds
    try:
        while time.time() < stop:
            try:
                chunk = conn.recv(4096)
            except socket.timeout:
                break
            if not chunk:
                break
            buf += chunk
    finally:
        conn.close()
    assert buf, f"proxy TCP port {port} ({protocol}) delivered no data"
    # The proxy forwards whole packets in scan order, so the received bytes
    # are a contiguous slice of the source log's same-protocol packets.
    concat = b"".join(d for (_, _, d) in _log_packets(ctx.packet_log, protocol))
    assert buf in concat, f"proxy TCP {protocol} bytes are not a slice of the source log"
    return len(buf)


def check_proxy_socket_capture(ctx, protocol="UBX", capture_seconds=3.0):
    """satpulsetool captures the proxy Unix-socket stream to a packet log,
    and every captured packet exists in the source log.

    Exercises satpulsetool's --socket/--capture/--packet-log path (passive
    capture: it only reads, so it works against a read-only proxy).
    """
    sock = ctx.proxy_socket
    assert poll(lambda: os.path.exists(sock)), f"proxy socket {sock} not created"
    cap = os.path.join(ctx.run_dir, "capture.jsonl")
    cmd = [
        ctx.satpulsetool, "gps", "--socket", sock,
        "--capture", str(capture_seconds), "--packet-log", cap,
    ]
    p = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                       timeout=capture_seconds + 15)
    assert p.returncode == 0, (
        f"satpulsetool capture failed ({p.returncode}): "
        f"{p.stderr.decode('utf-8', 'replace')[-500:]}"
    )
    captured = _log_packets(cap, protocol)
    assert captured, f"no {protocol} packets captured via proxy socket"
    assert len(captured) == len(_log_packets(cap)), (
        f"proxy socket forwarded non-{protocol} packets despite filter"
    )
    src = {d for (_, _, d) in _log_packets(ctx.packet_log, protocol)}
    missing = [m for (_, m, d) in captured if d not in src]
    assert not missing, f"captured packets not present in source log: {missing[:5]}"
    return len(captured)
