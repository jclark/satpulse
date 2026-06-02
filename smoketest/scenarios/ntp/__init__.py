"""Checks owned by the chrony SOCK NTP scenario."""

from __future__ import annotations

import datetime
import json
import os
from typing import TypedDict, cast

import common

NTP_SOCK_MAGIC = 0x534F434B  # "SOCK"


class NtpSample(TypedDict, total=False):
    """Decoded sock_sample record from ntpsock.py."""

    recv: float
    len: int
    sec: int
    usec: int
    offset: float
    pulse: int
    leap: int
    magic: int


def check_sock(
    ctx: common.SmokeContext,
    min_samples: int = 10,
    max_spread: float = 0.05,
    max_time_error: float = 0.1,
) -> float:
    """The chrony SOCK refclock stream is well-formed, consistent, and correct.

    Under realtime replay of a 1 Hz UTC time-message log, each sample carries
    the read instant (tv) and offset = utc - tv. Capture time and replay time
    differ, so the offset's absolute value is meaningless, but the daemon's
    notion of true time must be right: tv + offset (both from the sample) must
    land on an actual RMC instant from the log, proving the GPS time was parsed
    and represented correctly. This uses the daemon's own measurement time, so
    it does not depend on when the consumer was scheduled. Separately, a tight
    offset spread shows consistent timestamping, the cadence is ~1 s, and the
    receive time trails the measurement by under half a second (a delivery-
    latency sanity check, which also catches tv not being the real system clock).
    """
    samples = common.poll(lambda: (lambda s: s if len(s) >= min_samples else None)(_ntp_samples(ctx)))
    assert samples is not None, f"got fewer than {min_samples} NTP samples"
    for s in samples:
        assert s["len"] == 40, f"NTP sample wrong size {s['len']} != 40"
        assert s["magic"] == NTP_SOCK_MAGIC, f"bad magic {s['magic']:#x}"
        assert s["pulse"] == 0, f"unexpected pulse flag {s['pulse']}"
        assert s["leap"] in (0, 1, 2), f"bad leap {s['leap']}"
    tv = [s["sec"] + s["usec"] * 1e-6 for s in samples]
    offs = [s["offset"] for s in samples]
    spread = max(offs) - min(offs)
    assert spread <= max_spread, (
        f"offset spread {spread * 1000:.1f} ms exceeds {max_spread * 1000:.0f} ms"
    )
    for i in range(1, len(tv)):
        step = tv[i] - tv[i - 1]
        assert 0.5 < step < 1.5, f"measurement time step {step:.3f}s not ~1s at sample {i}"
    for s, t in zip(samples, tv):
        lag = s["recv"] - t
        assert 0 <= lag < 0.5, f"measurement-to-receipt lag {lag * 1000:.1f} ms not in [0, 500)"
    utcs = _rmc_utcs(ctx.packet_log)
    assert utcs, f"no RMC times in {ctx.packet_log}"
    for s, t in zip(samples, tv):
        recon = t + s["offset"]
        err = recon - min(utcs, key=lambda u: abs(u - recon))
        assert abs(err) <= max_time_error, (
            f"reconstructed true time off by {err * 1000:.1f} ms from any RMC instant "
            f"(exceeds {max_time_error * 1000:.0f} ms): GPS time parsed or represented wrong"
        )
    return spread


def _ntp_samples(ctx: common.SmokeContext) -> list[NtpSample]:
    """Decoded sock_sample records logged by ntpsock.py, in receive order."""
    out: list[NtpSample] = []
    if not os.path.exists(ctx.ntp_log):
        return out
    with open(ctx.ntp_log) as f:
        for line in f:
            line = line.strip()
            if line:
                out.append(cast(NtpSample, json.loads(line)))
    return out


def _rmc_utcs(path: str) -> list[float]:
    """UTC instants (epoch seconds) asserted by the RMC sentences in a log."""
    out = []
    for tag, msg, data in common.log_packets(path):
        if tag != "NMEA" or not (msg or "").endswith("RMC"):
            continue
        f = data.decode("latin1").split(",")
        if len(f) < 10 or not f[1] or not f[9]:
            continue
        hms, dmy = f[1], f[9]
        dt = datetime.datetime(
            2000 + int(dmy[4:6]), int(dmy[2:4]), int(dmy[0:2]),
            int(hms[0:2]), int(hms[2:4]), int(hms[4:6]),
            tzinfo=datetime.timezone.utc,
        )
        out.append(dt.timestamp() + (float(hms[6:]) if len(hms) > 6 else 0.0))
    return out
