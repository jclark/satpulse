#!/usr/bin/env python3
"""Minimal Ntrip v1 caster that accepts a SOURCE feed and captures its payload.

Plays the caster role for satpulsed's `[[stream.push]]` Ntrip destination: it
listens on a TCP port, accepts the Ntrip v1 SOURCE handshake a pushing client
sends, replies "ICY 200 OK", and appends every payload byte that follows to a
file. The captured bytes are the raw pushed stream (e.g. RTCM), suitable for
feeding to `satpulsetool scan`.

It implements only what a SOURCE client needs:
  - read the request headers up to the blank line
  - require the first line to be "SOURCE <password> /<mountpoint>"
  - optionally check the password and mountpoint against expected values
  - reply "ICY 200 OK\r\n" (or "ERROR - ...\r\n" on a mismatch)
  - append the rest of the connection to the capture file

    fakecaster.py 127.0.0.1:2101 -o pushed.bin
    fakecaster.py 127.0.0.1:2101 -o pushed.bin --mountpoint RTCM --password secret

Diagnostic lines go to stdout (one per connection); the captured payload goes
to the -o file. Runs until signalled. A connection that does not complete a
valid SOURCE handshake (e.g. a readiness probe) is closed and ignored.
"""
import argparse
import signal
import socket
import sys
import time

# Bound the handshake read so a connection that opens but never sends a
# complete request (a readiness probe, a stalled peer) cannot wedge the
# single-threaded accept loop.
HANDSHAKE_TIMEOUT = 10.0
MAX_HEADER = 8192


def log(msg):
    sys.stdout.write(msg + "\n")
    sys.stdout.flush()


def iso(t):
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(t)) + "Z"


def read_request(conn):
    """Read request bytes up to the blank line; return (headers, leftover)."""
    conn.settimeout(HANDSHAKE_TIMEOUT)
    buf = b""
    while b"\r\n\r\n" not in buf:
        if len(buf) > MAX_HEADER:
            return None, b""
        try:
            chunk = conn.recv(4096)
        except socket.timeout:
            return None, b""
        if not chunk:
            return None, b""
        buf += chunk
    head, rest = buf.split(b"\r\n\r\n", 1)
    return head, rest


def handle(conn, capture_path, want_mount, want_pw):
    """Run one SOURCE handshake and append the payload to capture_path."""
    head, rest = read_request(conn)
    if head is None:
        return  # probe or incomplete handshake; nothing to capture
    line = head.split(b"\r\n", 1)[0].decode("latin1")
    # Ntrip v1 SOURCE request: "SOURCE <password> /<mountpoint>". The
    # password is printable ASCII without spaces, so a 3-token split is safe.
    parts = line.split(" ")
    if len(parts) != 3 or parts[0] != "SOURCE":
        conn.sendall(b"ERROR - Bad Request\r\n")
        log(f"{iso(time.time())} rejected: {line!r}")
        return
    pw, mount = parts[1], parts[2].lstrip("/")
    if (want_mount and mount != want_mount) or (want_pw and pw != want_pw):
        conn.sendall(b"ERROR - Bad Password\r\n")
        log(f"{iso(time.time())} rejected SOURCE mount={mount}")
        return
    conn.sendall(b"ICY 200 OK\r\n")
    log(f"{iso(time.time())} accepted SOURCE mount={mount}")
    conn.settimeout(None)
    n = 0
    with open(capture_path, "ab") as f:
        if rest:
            f.write(rest)
            f.flush()
            n += len(rest)
        while True:
            try:
                chunk = conn.recv(65536)
            except OSError:
                break
            if not chunk:
                break
            f.write(chunk)
            f.flush()
            n += len(chunk)
    log(f"{iso(time.time())} closed mount={mount} bytes={n}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("listen", help="address to listen on, host:port")
    ap.add_argument("-o", "--output", required=True, help="append captured payload to this file")
    ap.add_argument("--mountpoint", default="", help="require this mountpoint (default: accept any)")
    ap.add_argument("--password", default="", help="require this SOURCE password (default: accept any)")
    args = ap.parse_args(argv)

    host, port = args.listen.rsplit(":", 1)
    stop = False

    def handle_signal(_signum, _frame):
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
                handle(conn, args.output, args.mountpoint, args.password)
            finally:
                conn.close()
    finally:
        s.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
