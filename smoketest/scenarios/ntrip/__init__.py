"""Checks owned by the Ntrip caster scenarios."""

from __future__ import annotations

import json
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


def check_stream(
    ctx: common.SmokeContext,
    mount: str,
    auth: common.Auth | None = None,
    read_seconds: float = 4.0,
) -> int:
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
            e = cast(common.JsonObject, json.loads(line))
        except json.JSONDecodeError:
            continue
        if e.get("tag") == "RTCM":
            rtcm += 1
    assert rtcm > 0, f"Ntrip client received no RTCM packets from {mount}"
    return rtcm


def check_unauthorized(ctx: common.SmokeContext, mount: str) -> None:
    """An unauthenticated request to a protected mountpoint is rejected."""
    status, _ = _request(ctx.ntrip_port, mount)
    assert status == 401, f"protected mount {mount} expected 401, got {status}"
