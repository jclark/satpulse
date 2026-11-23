"""Checks owned by the serial proxy scenarios."""

from __future__ import annotations

import socket
import time

import common


def _check_stream(
    ctx: common.SmokeContext,
    conn: socket.socket,
    label: str,
    protocol: str,
    read_seconds: float,
) -> int:
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
    assert buf, f"{label} ({protocol}) delivered no data"
    concat = b"".join(d for (_, _, d) in common.log_packets(ctx.packet_log, protocol))
    assert buf in concat, f"{label} {protocol} bytes are not a slice of the source log"
    return len(buf)


def check_tcp(
    ctx: common.SmokeContext,
    port: int,
    protocol: str,
    read_seconds: float = 3.0,
    connect_timeout: float = 15.0,
) -> int:
    """A read-only TCP proxy forwards filtered packets from the source log.

    The proxy comes up after GPS detection, so poll-connect until it
    accepts; the single successful connection is the reading client (no
    throwaway probe that would leave a subscriber to error on disconnect).
    """
    deadline = time.time() + connect_timeout
    conn: socket.socket | None = None
    while time.time() < deadline:
        try:
            conn = socket.create_connection(("127.0.0.1", port), timeout=1)
            break
        except OSError:
            time.sleep(0.1)
    assert conn is not None, f"proxy TCP port {port} never accepted a connection"
    return _check_stream(ctx, conn, f"proxy TCP port {port}", protocol, read_seconds)


def check_socket(
    ctx: common.SmokeContext,
    protocol: str = "UBX",
    read_seconds: float = 3.0,
    connect_timeout: float = 15.0,
) -> int:
    """A read-only Unix-socket proxy forwards filtered source packets."""
    path = ctx.proxy_socket
    deadline = time.time() + connect_timeout
    conn: socket.socket | None = None
    while time.time() < deadline:
        candidate = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        candidate.settimeout(1)
        try:
            candidate.connect(path)
            conn = candidate
            break
        except OSError:
            candidate.close()
            time.sleep(0.1)
    assert conn is not None, f"proxy socket {path} never accepted a connection"
    return _check_stream(ctx, conn, "proxy socket", protocol, read_seconds)
