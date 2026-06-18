#!/usr/bin/env python3
"""Minimal Ntrip v1 source that serves an RTCM log to a stream.pull client.

Plays the caster role for satpulsed's `[stream.pull].ntrip` correction source:
it listens on a TCP port, accepts the Ntrip v1 GET request a pulling client
sends, replies "ICY 200 OK", and then streams the RTCM body by running
`satpulsetool pack` on a packet log so the corrections keep the log's
inter-packet timing.

It implements only what a pull client needs:
  - read the request headers up to the blank line
  - require the first line to be "GET /<mountpoint> HTTP/1.0"
  - optionally check the mountpoint and Basic-auth credentials
  - reply "ICY 200 OK\r\n" (or "ERROR - ...\r\n" on a mismatch)
  - stream the packed RTCM log to the connection

After streaming, the connection is held open until the client disconnects, so
the daemon does not see EOF, reconnect, and replay the corrections a second
time (which would make the captured serial output no longer match the source).

    scenarios/stream/fakesource.py 127.0.0.1:2101 --pack satpulsetool log.jsonl
    scenarios/stream/fakesource.py 127.0.0.1:2101 --pack satpulsetool --factor 10 \
        --mountpoint RTCM --username smoke --password secret log.jsonl

Diagnostic lines go to stdout (one per connection). Runs until signalled. A
connection that does not complete a valid GET handshake (e.g. a readiness
probe) is closed and ignored.
"""
from __future__ import annotations

import argparse
import base64
import signal
import socket
import subprocess
import sys
import time
from types import FrameType
from typing import Sequence

# Bound the handshake read so a connection that opens but never sends a
# complete request (a readiness probe, a stalled peer) cannot wedge the
# single-threaded accept loop.
HANDSHAKE_TIMEOUT = 10.0
MAX_HEADER = 8192


def log(msg: str) -> None:
    sys.stdout.write(msg + "\n")
    sys.stdout.flush()


def iso(t: float) -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(t)) + "Z"


def read_request(conn: socket.socket) -> bytes | None:
    """Read request bytes up to the blank line; return the headers or None."""
    conn.settimeout(HANDSHAKE_TIMEOUT)
    buf = b""
    while b"\r\n\r\n" not in buf:
        if len(buf) > MAX_HEADER:
            return None
        try:
            chunk = conn.recv(4096)
        except socket.timeout:
            return None
        if not chunk:
            return None
        buf += chunk
    return buf.split(b"\r\n\r\n", 1)[0]


def auth_ok(head: bytes, want_user: str, want_pw: str) -> bool:
    """Check the Basic Authorization header against expected credentials."""
    if not want_pw:
        return True
    for line in head.split(b"\r\n")[1:]:
        if line.lower().startswith(b"authorization: basic "):
            try:
                creds = base64.b64decode(line.split(b" ", 2)[2]).decode("latin1")
            except (ValueError, IndexError):
                return False
            user, _, pw = creds.partition(":")
            return pw == want_pw and (not want_user or user == want_user)
    return False


def handle(conn: socket.socket, pack: str, factor: str, log_path: str,
           want_mount: str, want_user: str, want_pw: str) -> None:
    """Run one GET handshake and stream the packed RTCM log to the client."""
    head = read_request(conn)
    if head is None:
        return  # probe or incomplete handshake; nothing to serve
    line = head.split(b"\r\n", 1)[0].decode("latin1")
    # Ntrip v1 pull request: "GET /<mountpoint> HTTP/1.0".
    parts = line.split(" ")
    if len(parts) < 2 or parts[0] != "GET":
        conn.sendall(b"ERROR - Bad Request\r\n")
        log(f"{iso(time.time())} rejected: {line!r}")
        return
    mount = parts[1].lstrip("/")
    if want_mount and mount != want_mount:
        conn.sendall(b"ERROR - Bad Mountpoint\r\n")
        log(f"{iso(time.time())} rejected GET mount={mount}")
        return
    if not auth_ok(head, want_user, want_pw):
        conn.sendall(b"ERROR - Bad Password\r\n")
        log(f"{iso(time.time())} rejected GET mount={mount} (auth)")
        return
    conn.sendall(b"ICY 200 OK\r\n")
    log(f"{iso(time.time())} accepted GET mount={mount}")
    # pack writes the on-wire packet stream straight to the socket, paced by the
    # log's inter-packet timing compressed by factor.
    conn.settimeout(None)
    p = subprocess.run([pack, "pack", "--realtime", factor, log_path], stdout=conn.fileno())
    log(f"{iso(time.time())} streamed mount={mount} (pack exit {p.returncode})")
    # Hold the connection open until the client disconnects, so it does not see
    # EOF and reconnect for a second copy of the corrections.
    try:
        while conn.recv(4096):
            pass
    except OSError:
        pass
    log(f"{iso(time.time())} closed mount={mount}")


def main(argv: Sequence[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("listen", help="address to listen on, host:port")
    ap.add_argument("log", help="packet log to stream as RTCM corrections")
    ap.add_argument("--pack", required=True, help="path to satpulsetool (for pack)")
    ap.add_argument("--factor", default="10", help="pack --realtime speedup factor")
    ap.add_argument("--mountpoint", default="", help="require this mountpoint (default: accept any)")
    ap.add_argument("--username", default="", help="require this Basic-auth username")
    ap.add_argument("--password", default="", help="require this Basic-auth password (default: accept any)")
    args = ap.parse_args(argv)

    host, port = args.listen.rsplit(":", 1)
    stop = False

    def handle_signal(_signum: int, _frame: FrameType | None) -> None:
        nonlocal stop
        stop = True

    signal.signal(signal.SIGINT, handle_signal)
    signal.signal(signal.SIGTERM, handle_signal)

    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind((host, int(port)))
    s.listen(8)
    s.settimeout(0.5)
    log(f"{iso(time.time())} listening {host}:{port}")
    try:
        while not stop:
            try:
                conn, _ = s.accept()
            except socket.timeout:
                continue
            try:
                handle(conn, args.pack, args.factor, args.log,
                       args.mountpoint, args.username, args.password)
            finally:
                conn.close()
    finally:
        s.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
