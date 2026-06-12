"""Checks owned by the serial proxy scenarios."""

from __future__ import annotations

import os
import socket
import subprocess
import time

import common


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
    concat = b"".join(d for (_, _, d) in common.log_packets(ctx.packet_log, protocol))
    assert buf in concat, f"proxy TCP {protocol} bytes are not a slice of the source log"
    return len(buf)


def check_socket_capture(
    ctx: common.SmokeContext,
    protocol: str = "UBX",
    capture_seconds: float = 3.0,
) -> None:
    """satpulsetool captures the proxy Unix-socket stream to a packet log.

    Exercises satpulsetool's --socket/--capture/--packet-log path (passive
    capture: it only reads, so it works against a read-only proxy).
    """
    sock = ctx.proxy_socket
    assert common.poll(lambda: os.path.exists(sock)), f"proxy socket {sock} not created"
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
    captured = common.log_packets(cap, protocol)
    assert captured, f"no {protocol} packets captured via proxy socket"
    assert len(captured) == len(common.log_packets(cap)), (
        f"proxy socket forwarded non-{protocol} packets despite filter"
    )
    src = {d for (_, _, d) in common.log_packets(ctx.packet_log, protocol)}
    missing = [m for (_, m, d) in captured if d not in src]
    assert not missing, f"captured packets not present in source log: {missing[:5]}"
