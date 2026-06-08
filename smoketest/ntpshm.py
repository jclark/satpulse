#!/usr/bin/env python3
"""Inspect and remove ntpd/NTPsec SHM refclock segments.

This is a smoke-test helper for the NTP SHM protocol. It attaches to the
System V shared-memory segment keyed the same way as ntpd/NTPsec/gpsd
(`0x4e545030 + segment`) and reads `struct shmTime` with the mode-1
count/valid protocol.

Python maps the memory with ctypes, but the interprocess synchronization uses
GCC libatomic functions so the consumer side has real atomic loads/stores and
fences. The helper prints one JSON object for `read` and silently succeeds for
`remove` when the segment is already absent.
"""

from __future__ import annotations

import argparse
import ctypes
import ctypes.util
import errno
import json
import os
import sys
from typing import Any, ClassVar, Sequence, cast

ATOMIC_ACQUIRE = 2
ATOMIC_RELEASE = 3
ATOMIC_SEQ_CST = 5
IPC_RMID = 0
SHM_KEY_BASE = 0x4E545030
SHM_SIZE = 96


class ShmTime(ctypes.Structure):
    """ctypes layout for ntpd/NTPsec `struct shmTime` on 64-bit Linux."""

    _fields_: ClassVar[list[tuple[str, Any]]] = [
        ("mode", ctypes.c_int),
        ("count", ctypes.c_int),
        ("clockTimeStampSec", ctypes.c_long),
        ("clockTimeStampUSec", ctypes.c_int),
        ("receiveTimeStampSec", ctypes.c_long),
        ("receiveTimeStampUSec", ctypes.c_int),
        ("leap", ctypes.c_int),
        ("precision", ctypes.c_int),
        ("nsamples", ctypes.c_int),
        ("valid", ctypes.c_int),
        ("clockTimeStampNSec", ctypes.c_int),
        ("receiveTimeStampNSec", ctypes.c_int),
        ("dummy", ctypes.c_int * 8),
    ]


if ctypes.sizeof(ShmTime) != SHM_SIZE:
    raise RuntimeError(f"unexpected shmTime size {ctypes.sizeof(ShmTime)} != {SHM_SIZE}")

libc = ctypes.CDLL(None, use_errno=True)
_shmget = cast(Any, libc.shmget)
_shmget.argtypes = [ctypes.c_int, ctypes.c_size_t, ctypes.c_int]
_shmget.restype = ctypes.c_int
_shmat = cast(Any, libc.shmat)
_shmat.argtypes = [ctypes.c_int, ctypes.c_void_p, ctypes.c_int]
_shmat.restype = ctypes.c_void_p
_shmdt = cast(Any, libc.shmdt)
_shmdt.argtypes = [ctypes.c_void_p]
_shmdt.restype = ctypes.c_int
_shmctl = cast(Any, libc.shmctl)
_shmctl.argtypes = [ctypes.c_int, ctypes.c_int, ctypes.c_void_p]
_shmctl.restype = ctypes.c_int

_atomic_lib: Any | None = None
_atomic_load_4: Any | None = None
_atomic_store_4: Any | None = None
_atomic_fence: Any | None = None


def load_atomic() -> None:
    """Load libatomic entry points used to mirror the C SHM reader protocol."""
    global _atomic_lib, _atomic_load_4, _atomic_store_4, _atomic_fence
    if _atomic_load_4 is not None:
        return
    name = ctypes.util.find_library("atomic")
    if name is None:
        raise RuntimeError("libatomic not found")
    _atomic_lib = ctypes.CDLL(name)
    load_4 = cast(Any, getattr(_atomic_lib, "__atomic_load_4"))
    load_4.argtypes = [ctypes.c_void_p, ctypes.c_int]
    load_4.restype = ctypes.c_uint32
    store_4 = cast(Any, getattr(_atomic_lib, "__atomic_store_4"))
    store_4.argtypes = [ctypes.c_void_p, ctypes.c_uint32, ctypes.c_int]
    store_4.restype = None
    fence = cast(Any, getattr(_atomic_lib, "atomic_thread_fence"))
    fence.argtypes = [ctypes.c_int]
    fence.restype = None
    _atomic_load_4 = load_4
    _atomic_store_4 = store_4
    _atomic_fence = fence


def ntp_key(segment: int) -> int:
    check_segment(segment)
    return SHM_KEY_BASE + segment


def check_segment(segment: int) -> None:
    if segment < 0 or segment > 255:
        raise ValueError(f"segment {segment} out of range 0..255")


def field_addr(s: ShmTime, name: str) -> ctypes.c_void_p:
    field = getattr(type(s), name)
    return ctypes.c_void_p(ctypes.addressof(s) + int(field.offset))


def atomic_i32_load(s: ShmTime, name: str, order: int = ATOMIC_ACQUIRE) -> int:
    load_atomic()
    assert _atomic_load_4 is not None
    u = int(cast(int, _atomic_load_4(field_addr(s, name), order)))
    return ctypes.c_int32(u).value


def atomic_i32_store(s: ShmTime, name: str, val: int, order: int = ATOMIC_RELEASE) -> None:
    load_atomic()
    assert _atomic_store_4 is not None
    _atomic_store_4(field_addr(s, name), ctypes.c_uint32(val & 0xFFFFFFFF), order)


def atomic_thread_fence(order: int = ATOMIC_SEQ_CST) -> None:
    load_atomic()
    assert _atomic_fence is not None
    _atomic_fence(order)


def shmget_existing(segment: int, size: int = SHM_SIZE) -> int:
    shmid = int(cast(int, _shmget(ntp_key(segment), size, 0)))
    if shmid >= 0:
        return shmid
    err = ctypes.get_errno()
    raise OSError(err, os.strerror(err))


def shmat(shmid: int) -> int:
    addr = cast(int | None, _shmat(shmid, None, 0))
    bad = ctypes.c_void_p(-1).value
    if addr is not None and addr != bad:
        return addr
    err = ctypes.get_errno()
    raise OSError(err, os.strerror(err))


def shmdt(addr: int) -> None:
    rc = int(cast(int, _shmdt(ctypes.c_void_p(addr))))
    if rc != 0:
        err = ctypes.get_errno()
        raise OSError(err, os.strerror(err))


def shmctl_remove(shmid: int) -> None:
    rc = int(cast(int, _shmctl(shmid, IPC_RMID, None)))
    if rc != 0:
        err = ctypes.get_errno()
        raise OSError(err, os.strerror(err))


def query(segment: int) -> dict[str, object]:
    """Return a JSON-serializable snapshot or a not-ready status."""
    try:
        shmid = shmget_existing(segment)
        addr = shmat(shmid)
    except OSError as e:
        if e.errno == errno.ENOENT:
            return {"ok": False, "status": "no_segment", "segment": segment}
        raise
    shm = ShmTime.from_address(addr)
    try:
        if atomic_i32_load(shm, "valid") == 0:
            return {"ok": False, "status": "not_ready", "segment": segment}
        count_before = atomic_i32_load(shm, "count")
        if count_before & 1:
            return {"ok": False, "status": "busy", "segment": segment}
        snap = ShmTime()
        atomic_thread_fence()
        ctypes.memmove(ctypes.byref(snap), ctypes.c_void_p(addr), SHM_SIZE)
        atomic_i32_store(shm, "valid", 0)
        atomic_thread_fence()
        count_after = atomic_i32_load(shm, "count")
        if snap.mode > 0 and (count_before != count_after or count_after & 1):
            return {"ok": False, "status": "clash", "segment": segment}
        return snapshot(segment, count_after, snap)
    finally:
        shmdt(addr)


def snapshot(segment: int, count: int, s: ShmTime) -> dict[str, object]:
    return {
        "ok": True,
        "status": "ok",
        "segment": segment,
        "mode": int(s.mode),
        "count": count,
        "clock_sec": int(s.clockTimeStampSec),
        "clock_usec": int(s.clockTimeStampUSec),
        "clock_nsec": int(s.clockTimeStampNSec),
        "receive_sec": int(s.receiveTimeStampSec),
        "receive_usec": int(s.receiveTimeStampUSec),
        "receive_nsec": int(s.receiveTimeStampNSec),
        "leap": int(s.leap),
        "precision": int(s.precision),
        "nsamples": int(s.nsamples),
        "valid": int(s.valid),
    }


def remove(segment: int) -> None:
    """Remove a segment if it exists; tolerate already-absent segments."""
    try:
        shmid = shmget_existing(segment, size=1)
    except OSError as e:
        if e.errno == errno.ENOENT:
            return
        raise
    shmctl_remove(shmid)


def main(argv: Sequence[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description="inspect or remove an NTP SHM segment")
    ap.add_argument("action", choices=("read", "remove"))
    ap.add_argument("segment", type=int)
    args = ap.parse_args(argv)
    try:
        if args.action == "read":
            print(json.dumps(query(args.segment), sort_keys=True))
        else:
            remove(args.segment)
        return 0
    except Exception as e:
        print(f"ntpshm.py: {e}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
