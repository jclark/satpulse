#!/usr/bin/env python3
"""Minimal UDP receiver for stream.push smoke tests."""
from __future__ import annotations

import argparse
import signal
import socket
import sys
import time
from types import FrameType
from typing import BinaryIO, Sequence


def log(msg: str) -> None:
    sys.stdout.write(msg + "\n")
    sys.stdout.flush()


def iso(t: float) -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(t)) + "Z"


def run(sock: socket.socket, out: BinaryIO) -> None:
    stop = False

    def handle_signal(_signum: int, _frame: FrameType | None) -> None:
        nonlocal stop
        stop = True

    signal.signal(signal.SIGINT, handle_signal)
    signal.signal(signal.SIGTERM, handle_signal)
    sock.settimeout(0.5)
    n = 0
    while not stop:
        try:
            data, peer = sock.recvfrom(65535)
        except socket.timeout:
            continue
        n += 1
        out.write(data)
        out.flush()
        log(f"{iso(time.time())} received datagram {n} from {peer[0]}:{peer[1]} len={len(data)}")


def main(argv: Sequence[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("listen", help="address to listen on, host:port")
    ap.add_argument("-o", "--output", required=True, help="file for captured UDP payloads")
    args = ap.parse_args(argv)

    host, port = args.listen.rsplit(":", 1)
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    # Enlarge the kernel receive buffer (clamped to the OS max) so a brief
    # stall in the single-threaded recv/write loop -- a GC pause or a disk
    # flush -- cannot overflow the default buffer and silently drop datagrams,
    # which would make the lossless-delivery check flaky. macOS defaults this
    # buffer far smaller than Linux, so the loss only showed up there.
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4 * 1024 * 1024)
    sock.bind((host, int(port)))
    log(f"{iso(time.time())} listening {host}:{port}")
    try:
        with open(args.output, "ab") as out:
            run(sock, out)
    finally:
        sock.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
