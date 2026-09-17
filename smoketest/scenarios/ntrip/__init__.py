"""Checks owned by the Ntrip caster scenarios."""

from __future__ import annotations

import json
import os
import shutil
import socket
import subprocess
import time
from typing import cast

import common


def _request(
    port: int,
    path: str,
    auth: common.Auth | None = None,
    timeout: float = 2.0,
    read_bytes: int = 65536,
) -> tuple[int | None, bytes]:
    """Issue a raw Ntrip GET and return (status_code, body-bytes).

    The Ntrip caster replies with non-HTTP status lines such as
    "SOURCETABLE 200 OK" and "ICY 200 OK", which urllib cannot parse, so
    a raw socket is used. The status code is the first integer token of
    the response's first line.
    """
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
            chunks: list[bytes] = []
            total = 0
            while total < read_bytes:
                try:
                    b = s.recv(4096)
                except socket.timeout:
                    break
                if not b:
                    break
                chunks.append(b)
                total += len(b)
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


def check_sourcetable(ctx: common.SmokeContext, mount: str) -> bytes:
    """The Ntrip caster source table lists the expected mountpoint."""

    def attempt() -> bytes | None:
        status, body = _request(ctx.ntrip_port, "")
        if status == 200 and f"STR;{mount}".encode() in body:
            return body
        return None

    body = common.poll(attempt)
    assert body is not None, f"source table missing mountpoint {mount}"
    return body


# STR-record field indices, counting from the first field after the
# "STR;<name>;" prefix (so identifier is 0). See gps/app/ntrip/strrec.go.
STR_IDENTIFIER = 0
STR_NETWORK = 5
STR_COUNTRY = 6
STR_LAT = 7
STR_LON = 8
STR_GENERATOR = 11
STR_BITRATE = 15


def sourcetable_record(ctx: common.SmokeContext, mount: str) -> list[str]:
    """Return mount's STR record as a field list, identifier first.

    Splits the mountpoint's STR line on ";" and drops the leading "STR"
    and name tokens, so the result is indexable by the STR_* constants.
    """
    body = check_sourcetable(ctx, mount)
    prefix = f"STR;{mount};".encode()
    for raw in body.split(b"\r\n"):
        if raw.startswith(prefix):
            return raw.decode("latin1").split(";")[2:]
    raise AssertionError(f"no STR record for mountpoint {mount}")


def _read_stream(
    ctx: common.SmokeContext,
    mount: str,
    auth: common.Auth | None,
    read_seconds: float,
) -> list[common.JsonObject]:
    """Read the mountpoint with satpulsetool ntrip and return its RTCM packets.

    Runs satpulsetool ntrip as the client (also exercising that tool),
    reading the stream while the runner's background replay feeds RTCM
    through the daemon, and returns the RTCM packet objects it logged.
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
    pkts: list[common.JsonObject] = []
    for line in out.decode("utf-8", "replace").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            e = cast(common.JsonObject, json.loads(line))
        except json.JSONDecodeError:
            continue
        if e.get("tag") == "RTCM":
            pkts.append(e)
    return pkts


def check_stream(
    ctx: common.SmokeContext,
    mount: str,
    auth: common.Auth | None = None,
    read_seconds: float = 4.0,
) -> int:
    """An Ntrip client receives RTCM packets from the caster mountpoint."""
    pkts = _read_stream(ctx, mount, auth, read_seconds)
    assert pkts, f"Ntrip client received no RTCM packets from {mount}"
    return len(pkts)


def stream_rtcm_msgs(
    ctx: common.SmokeContext,
    mount: str,
    auth: common.Auth | None = None,
    read_seconds: float = 4.0,
) -> set[str]:
    """Return the set of RTCM message numbers received from the mountpoint.

    Like check_stream, but reports which RTCM message types the caster
    delivers, so a scenario can assert on conversions such as MSM7->MSM4.
    """
    msgs: set[str] = set()
    for p in _read_stream(ctx, mount, auth, read_seconds):
        m = p.get("msg")
        if isinstance(m, str):
            msgs.add(m)
    assert msgs, f"Ntrip client received no RTCM messages from {mount}"
    return msgs


def check_stream_str2str(
    ctx: common.SmokeContext,
    mount: str,
    read_seconds: float = 4.0,
    min_packets: int = 5,
) -> int:
    """A real RTKLIB str2str Ntrip client receives the mountpoint's RTCM.

    Runs str2str as the Ntrip client against our caster, capturing the
    stream while the background replay flows, then checks the captured RTCM
    is a non-empty contiguous run of the caster's source-log RTCM. A real
    client joining a live stream mid-flight sees a window, not the whole
    log, relayed byte-for-byte -- interop proof that a real RTKLIB client
    speaks our caster's protocol and gets the bytes faithfully.
    """
    str2str = shutil.which("str2str")
    assert str2str, "str2str not on PATH"
    capture = os.path.join(ctx.run_dir, f"str2str-{mount}.rtcm")
    url = f"ntrip://127.0.0.1:{ctx.ntrip_port}/{mount}"
    client = subprocess.Popen(
        [str2str, "-in", url, "-out", capture],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    )
    try:
        time.sleep(read_seconds)
    finally:
        client.terminate()
        try:
            client.communicate(timeout=read_seconds)
        except subprocess.TimeoutExpired:
            client.kill()
            client.communicate()
    got = [d for (_, _, d) in common.scan_packets(ctx, capture, "RTCM")]
    want = [d for (_, _, d) in common.log_packets(ctx.packet_log, "RTCM")]
    assert len(got) >= min_packets, (
        f"str2str received {len(got)} RTCM packets from {mount}, want >= {min_packets}"
    )
    assert common.is_contiguous_sublist(want, got), (
        f"str2str RTCM from {mount} is not a contiguous run of the caster source log"
    )
    return len(got)


def check_unauthorized(ctx: common.SmokeContext, mount: str) -> None:
    """An unauthenticated request to a protected mountpoint is rejected."""
    status, _ = _request(ctx.ntrip_port, mount)
    assert status == 401, f"protected mount {mount} expected 401, got {status}"


def check_credentials_rejected(
    ctx: common.SmokeContext, mount: str, auth: common.Auth
) -> None:
    """Wrong credentials to a protected mountpoint are rejected with 401.

    For an anyUser mountpoint this confirms the password is still
    verified: any valid top-level user is allowed, but bad credentials
    are not.
    """
    status, _ = _request(ctx.ntrip_port, mount, auth=auth)
    assert status == 401, f"mount {mount} with bad credentials expected 401, got {status}"
