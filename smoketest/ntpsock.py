#!/usr/bin/env python3
"""Receive and decode chrony SOCK refclock samples.

A standalone consumer for the Unix datagram "SOCK" refclock protocol that
chronyd reads via `refclock SOCK <path>`. It binds the socket a time source
sends samples to -- the role chronyd normally plays -- decodes each
struct sock_sample, and prints what it received and when.

(This is distinct from the NTP shared-memory protocol, chrony's `refclock SHM`
and ntpd's type 28 driver, which is not a socket at all.)

Useful for inspecting or testing any SOCK sample source (satpulsed, gpsd,
custom drivers): point it at a socket path and watch the samples, or capture
JSON for post-processing.

    ntpsock.py /run/chrony.ttyS0.sock              # human-readable, to stdout
    ntpsock.py -f json -o samples.jsonl /tmp/s     # JSON lines for analysis
    ntpsock.py -n 10 /tmp/s                         # stop after 10 samples
    ntpsock.py -d 30 /tmp/s                         # stop after 30 seconds

The sender must send to this path; this program creates and binds it. A stale
socket file at the path is replaced, and the path is removed on exit unless
--keep is given.
"""
import argparse
import json
import os
import signal
import socket
import stat
import struct
import sys
import time
from types import FrameType
from typing import Sequence, TextIO, TypeAlias


def _is_32bit() -> bool:
    """Whether the SOCK sender uses the 32-bit struct layout.

    Honour GOARCH, which the smoke-test runner sets when exercising a
    cross-built 32-bit daemon; otherwise fall back to this host's word size for
    standalone use against a local sender.
    """
    goarch = os.environ.get("GOARCH")
    if goarch:
        return goarch in ("386", "arm")
    return struct.calcsize("P") == 4


# struct sock_sample, as written by chrony's SOCK clients (little-endian on
# every arch we build for). struct timeval's two ints are 32-bit wide on 32-bit
# platforms and 64-bit on 64-bit ones; everything after it is arch-independent:
#   struct timeval tv  -> sec, usec : seconds and microseconds of the read
#   double offset                   : seconds, true_time - system_time
#   int pulse                       : non-zero if PPS-only (no seconds)
#   int leap                        : 0 normal, 1 insert, 2 delete
#   int _pad                        : ignored
#   int magic == 0x534f434b ("SOCK")
SAMPLE_FMT = "<iidiiii" if _is_32bit() else "<qqdiiii"
SAMPLE_SIZE = struct.calcsize(SAMPLE_FMT)
SOCK_MAGIC = 0x534F434B
Record: TypeAlias = dict[str, object]


def decode(data: bytes, recv: float) -> Record:
    """Decode one datagram into a record dict; non-sample sizes are flagged."""
    rec: Record = {"recv": recv, "len": len(data)}
    if len(data) == SAMPLE_SIZE:
        sec, usec, off, pulse, leap, _pad, magic = struct.unpack(SAMPLE_FMT, data)
        rec.update(sec=sec, usec=usec, offset=off, pulse=pulse, leap=leap, magic=magic)
    else:
        rec["hex"] = data.hex()
    return rec


def iso(t: float) -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(t)) + f".{int((t % 1) * 1e6):06d}Z"


def render_text(rec: Record) -> str:
    if "magic" not in rec:
        return (
            f"{iso(_float(rec['recv']))} recv  unexpected "
            f"{_int(rec['len'])}-byte datagram: {_str(rec['hex'])}"
        )
    tv = _int(rec["sec"]) + _int(rec["usec"]) * 1e-6
    magic_val = _int(rec["magic"])
    magic = "ok" if magic_val == SOCK_MAGIC else f"BAD({magic_val:#x})"
    kind = "pps" if _int(rec["pulse"]) else "sample"
    return (
        f"{iso(_float(rec['recv']))} recv  {kind} tv={iso(tv)} "
        f"offset={_float(rec['offset']):+.6f}s lag={(_float(rec['recv']) - tv) * 1e3:.1f}ms "
        f"leap={_int(rec['leap'])} magic={magic}"
    )


def _float(v: object) -> float:
    if isinstance(v, int | float):
        return float(v)
    raise TypeError(f"expected number, got {type(v).__name__}")


def _int(v: object) -> int:
    if isinstance(v, int):
        return v
    raise TypeError(f"expected int, got {type(v).__name__}")


def _str(v: object) -> str:
    if isinstance(v, str):
        return v
    raise TypeError(f"expected string, got {type(v).__name__}")


def bind(path: str, keep: bool) -> socket.socket:
    """Bind an AF_UNIX datagram socket at path, replacing a stale socket file."""
    if os.path.exists(path) and stat.S_ISSOCK(os.stat(path).st_mode):
        os.unlink(path)
    s = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
    s.bind(path)
    return s


def main(argv: Sequence[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("path", help="socket path to create and receive on")
    ap.add_argument("-o", "--output", help="write to this file instead of stdout")
    ap.add_argument("-f", "--format", choices=("text", "json"), default="text",
                    help="output format (default: text)")
    ap.add_argument("-n", "--count", type=int, default=0,
                    help="exit after this many datagrams (default: run until signalled)")
    ap.add_argument("-d", "--duration", type=float, default=0,
                    help="exit after this many seconds (default: run until signalled)")
    ap.add_argument("--keep", action="store_true",
                    help="leave the socket file in place on exit")
    args = ap.parse_args(argv)

    stop = False

    def handle(_signum: int, _frame: FrameType | None) -> None:
        nonlocal stop
        stop = True

    signal.signal(signal.SIGINT, handle)
    signal.signal(signal.SIGTERM, handle)

    s = bind(args.path, args.keep)
    s.settimeout(0.5)
    out: TextIO = open(args.output, "w") if args.output else sys.stdout
    n = 0
    deadline = time.time() + args.duration if args.duration else None
    try:
        while not stop:
            if deadline is not None and time.time() >= deadline:
                break
            try:
                data = s.recv(4096)
            except socket.timeout:
                continue
            rec = decode(data, time.time())
            out.write((json.dumps(rec) if args.format == "json" else render_text(rec)) + "\n")
            out.flush()
            n += 1
            if args.count and n >= args.count:
                break
    finally:
        s.close()
        if not args.keep:
            try:
                os.unlink(args.path)
            except OSError:
                pass
        if args.output:
            out.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
