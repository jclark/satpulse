#!/usr/bin/env python3
"""Receive and decode pipe-timeprov refclock samples (Windows only).

A standalone consumer for the pipe-timeprov wire format: the named-pipe
protocol that the pipe-timeprov w32time time provider DLL reads and that
satpulsed's [ntp.timeprov] sink writes. It creates the message-mode named
pipe a time source sends samples to -- the role the provider DLL normally
plays -- decodes each 32-byte record, and prints what it received and when.

This is the Windows counterpart of ntpsock.py (chrony SOCK). Use it to
inspect or test any pipe-timeprov sample source without touching the real
Windows Time service: point satpulsed at the pipe path and watch the
samples, or capture JSON for post-processing.

    timeprovpipe.py \\\\.\\pipe\\pipetimeprov          # human-readable, to stdout
    timeprovpipe.py -f json -o samples.jsonl \\\\.\\pipe\\pipetimeprov
    timeprovpipe.py -n 10 \\\\.\\pipe\\pipetimeprov     # stop after 10 records
    timeprovpipe.py -d 30 \\\\.\\pipe\\pipetimeprov     # stop after 30 seconds

Wire format version 1 (little-endian, one record per pipe message; see
pipe-timeprov's src/wire.h): magic u32 "TPV1", leap u8 (0 none, 1 insert,
2 delete, 3 unsynchronized), 3 reserved bytes, then three 64-bit fields in
100 ns units -- sysTime (system clock at measurement, since 1601), offset
(true time minus system clock, signed), dispersion (measurement error).
"""
from __future__ import annotations

import argparse
import ctypes
import json
import signal
import struct
import sys
import time
from types import FrameType
from typing import Callable, Sequence, TextIO, TypeAlias

SAMPLE_FMT = "<IB3sQqQ"
SAMPLE_SIZE = struct.calcsize(SAMPLE_FMT)
TPV_MAGIC = 0x31565054  # "TPV1"
# Unix epoch in 100 ns units since the NT epoch 1601-01-01 UTC.
FILETIME_EPOCH = 116444736000000000

PIPE_ACCESS_INBOUND = 0x1
FILE_FLAG_OVERLAPPED = 0x40000000
PIPE_TYPE_MESSAGE = 0x4
PIPE_READMODE_MESSAGE = 0x2
PIPE_REJECT_REMOTE_CLIENTS = 0x8
INVALID_HANDLE_VALUE = ctypes.c_void_p(-1).value
ERROR_IO_PENDING = 997
ERROR_PIPE_CONNECTED = 535
ERROR_BROKEN_PIPE = 109
ERROR_MORE_DATA = 234
WAIT_OBJECT_0 = 0
WAIT_TIMEOUT = 0x102

Record: TypeAlias = dict[str, object]
StopFn: TypeAlias = Callable[[], bool]


def decode(data: bytes, recv: float) -> Record:
    """Decode one pipe message into a record dict; non-sample sizes are flagged."""
    rec: Record = {"recv": recv, "len": len(data)}
    if len(data) == SAMPLE_SIZE:
        magic, leap, _res, sys_time, offset, dispersion = struct.unpack(SAMPLE_FMT, data)
        rec.update(magic=magic, leap=leap, sysTime=sys_time, offset=offset, dispersion=dispersion)
    else:
        rec["hex"] = data.hex()
    return rec


def iso(t: float) -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(t)) + f".{int((t % 1) * 1e6):06d}Z"


def render_text(rec: Record) -> str:
    if "magic" not in rec:
        return (
            f"{iso(_float(rec['recv']))} recv  unexpected "
            f"{_int(rec['len'])}-byte message: {_str(rec['hex'])}"
        )
    tv = (_int(rec["sysTime"]) - FILETIME_EPOCH) * 1e-7
    magic_val = _int(rec["magic"])
    magic = "ok" if magic_val == TPV_MAGIC else f"BAD({magic_val:#x})"
    return (
        f"{iso(_float(rec['recv']))} recv  sample tv={iso(tv)} "
        f"offset={_int(rec['offset']) * 1e-7:+.7f}s "
        f"disp={_int(rec['dispersion']) * 1e-4:.3f}ms "
        f"lag={(_float(rec['recv']) - tv) * 1e3:.1f}ms "
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


class _Overlapped(ctypes.Structure):
    _fields_ = (
        ("Internal", ctypes.c_void_p),
        ("InternalHigh", ctypes.c_void_p),
        ("Offset", ctypes.c_uint32),
        ("OffsetHigh", ctypes.c_uint32),
        ("hEvent", ctypes.c_void_p),
    )


def _winerr(what: str) -> OSError:
    err = ctypes.get_last_error()
    return OSError(f"{what} failed: WinError {err}")


class PipeServer:
    """Own the provider's end of the pipe: one inbound message-mode instance.

    All waits use a 500 ms timeout so the main thread regularly returns to
    Python and can observe signals and deadlines; a blocking ConnectNamedPipe
    or ReadFile would be uninterruptible.
    """

    def __init__(self, path: str) -> None:
        self.k32 = ctypes.WinDLL("kernel32", use_last_error=True)
        # Declare handle-bearing signatures: ctypes' default int restype
        # truncates 64-bit HANDLEs.
        u32, p = ctypes.c_uint32, ctypes.c_void_p
        self.k32.CreateEventW.restype = p
        self.k32.CreateEventW.argtypes = (p, ctypes.c_int, ctypes.c_int, ctypes.c_wchar_p)
        self.k32.CreateNamedPipeW.restype = p
        self.k32.CreateNamedPipeW.argtypes = (ctypes.c_wchar_p, u32, u32, u32, u32, u32, u32, p)
        self.k32.ConnectNamedPipe.argtypes = (p, p)
        self.k32.ReadFile.argtypes = (p, p, u32, p, p)
        self.k32.GetOverlappedResult.argtypes = (p, p, p, ctypes.c_int)
        self.k32.WaitForSingleObject.restype = u32
        self.k32.WaitForSingleObject.argtypes = (p, u32)
        for fn in (self.k32.CancelIo, self.k32.DisconnectNamedPipe,
                   self.k32.CloseHandle, self.k32.ResetEvent):
            fn.argtypes = (p,)
        self.path = path
        self.event = self.k32.CreateEventW(None, True, False, None)
        if not self.event:
            raise _winerr("CreateEvent")

    def serve(self, on_record: Callable[[bytes], None], should_stop: StopFn) -> None:
        while not should_stop():
            handle = self.k32.CreateNamedPipeW(
                self.path,
                PIPE_ACCESS_INBOUND | FILE_FLAG_OVERLAPPED,
                PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE | PIPE_REJECT_REMOTE_CLIENTS,
                1, 0, 4096, 0, None)
            if handle is None or handle == INVALID_HANDLE_VALUE:
                raise _winerr(f"CreateNamedPipe({self.path})")
            try:
                if self._connect(handle, should_stop):
                    self._read_loop(handle, on_record, should_stop)
            finally:
                self.k32.CloseHandle(handle)

    def _wait(self, handle: int, should_stop: StopFn) -> bool:
        """Wait for the pending overlapped operation; False when stopping."""
        while True:
            w = self.k32.WaitForSingleObject(self.event, 500)
            if w == WAIT_OBJECT_0:
                return True
            if w != WAIT_TIMEOUT:
                raise _winerr("WaitForSingleObject")
            if should_stop():
                self.k32.CancelIo(handle)
                return False

    def _start(self, handle: int) -> _Overlapped:
        ov = _Overlapped()
        ov.hEvent = self.event
        self.k32.ResetEvent(self.event)
        return ov

    def _connect(self, handle: int, should_stop: StopFn) -> bool:
        ov = self._start(handle)
        if not self.k32.ConnectNamedPipe(handle, ctypes.byref(ov)):
            err = ctypes.get_last_error()
            if err == ERROR_PIPE_CONNECTED:
                return True
            if err != ERROR_IO_PENDING:
                raise _winerr("ConnectNamedPipe")
            if not self._wait(handle, should_stop):
                return False
        return not should_stop()

    def _read_loop(self, handle: int, on_record: Callable[[bytes], None],
                   should_stop: StopFn) -> None:
        buf = ctypes.create_string_buffer(512)
        while not should_stop():
            ov = self._start(handle)
            if not self.k32.ReadFile(handle, buf, len(buf), None, ctypes.byref(ov)):
                err = ctypes.get_last_error()
                if err == ERROR_IO_PENDING:
                    if not self._wait(handle, should_stop):
                        return
                elif err not in (ERROR_MORE_DATA,):
                    return  # client disconnected or pipe error
            n = ctypes.c_uint32()
            if not self.k32.GetOverlappedResult(handle, ctypes.byref(ov), ctypes.byref(n), False):
                err = ctypes.get_last_error()
                if err == ERROR_MORE_DATA:
                    # Oversized message: the chunks are delivered and
                    # rejected by size, so just keep reading.
                    on_record(buf.raw[: n.value])
                    continue
                return
            on_record(buf.raw[: n.value])
        self.k32.DisconnectNamedPipe(handle)


def main(argv: Sequence[str] | None = None) -> int:
    if sys.platform != "win32":
        print("timeprovpipe.py requires Windows named pipes", file=sys.stderr)
        return 2
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("path", help=r"named pipe path to create and receive on, e.g. \\.\pipe\pipetimeprov")
    ap.add_argument("-o", "--output", help="write to this file instead of stdout")
    ap.add_argument("-f", "--format", choices=("text", "json"), default="text",
                    help="output format (default: text)")
    ap.add_argument("-n", "--count", type=int, default=0,
                    help="exit after this many records (default: run until signalled)")
    ap.add_argument("-d", "--duration", type=float, default=0,
                    help="exit after this many seconds (default: run until signalled)")
    args = ap.parse_args(argv)

    stop = False

    def handle(_signum: int, _frame: FrameType | None) -> None:
        nonlocal stop
        stop = True

    signal.signal(signal.SIGINT, handle)
    signal.signal(signal.SIGTERM, handle)

    out: TextIO = open(args.output, "w") if args.output else sys.stdout
    n = 0
    deadline = time.time() + args.duration if args.duration else None

    def should_stop() -> bool:
        if deadline is not None and time.time() >= deadline:
            return True
        if args.count and n >= args.count:
            return True
        return stop

    def on_record(data: bytes) -> None:
        nonlocal n
        rec = decode(data, time.time())
        out.write((json.dumps(rec) if args.format == "json" else render_text(rec)) + "\n")
        out.flush()
        n += 1

    try:
        PipeServer(args.path).serve(on_record, should_stop)
    finally:
        if args.output:
            out.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
