"""Support-aware property comparators.

Comparison decisions come from three places, in this order:

- ConfigSupportFlags: skip happens before a case runs (in the runner);
  here flags degrade comparisons (no band support: constellation-level
  only; no fixedPosAcc: set but do not compare) and bound them (expected
  constellations are the requested set intersected with the receiver's
  supported GNSS).
- Explicit tolerance rules, each tied to observed receiver behavior and
  the family it covers (see COUPLED_GNSS, TP_QUANT, POS_TOL).
- Everything else compares exactly. An unexplained mismatch is a FAIL,
  not a new tolerance; surfacing those disagreements is the point.
"""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Any

from cases import (
    AntCableDelayExpect,
    Case,
    MinElevExpect,
    ModeExpect,
    RTCMBaseIDExpect,
    SignalsExpect,
    TimeGNSSExpect,
    TimePulseExpect,
)
from runner import Baseline

# Tolerance rule (u-blox 6/7/M8): QZSS is coupled to GPS and may come
# back enabled when only GPS was requested. Keyed by vendor; the value
# maps a requested constellation to the additions it may drag in.
COUPLED_GNSS: dict[str, dict[str, frozenset[str]]] = {
    "u-blox": {"GPS": frozenset({"QZSS"})},
}

# Tolerance rule: time pulse width and period are quantized; accept the
# nearest representable value, one quantization step. u-blox CFG-TP
# expresses pulse length and period in microseconds.
TP_QUANT: dict[str, float] = {"u-blox": 1e-6}

# Tolerance rule: fixed positions are stored at cm resolution (u-blox:
# cm plus a 0.01 mm high-precision part; Unicore similar), so ECEF/LLH
# round trips compare within 2 cm rather than exactly.
POS_TOL = 0.02

# Antenna cable delay is quantized to whole nanoseconds on both families.
ANT_DELAY_TOL = 1e-9 + 1e-15

# Signal name to band mapping, ported from gpsprot's signalIDBandTable.
# Ordered prefix rules; first match wins. A signal may span several bands
# (Galileo E5 ALTBOC); it satisfies a band restriction only when all its
# bands were requested.
_SIGNAL_BANDS: list[tuple[str, bool, frozenset[str]]] = [
    # (name, is-prefix, bands)
    ("E5b", True, frozenset({"E5b"})),
    ("E5a", True, frozenset({"L5"})),
    ("E5", False, frozenset({"L5", "E5b"})),  # Galileo ALTBOC
    ("B2a+b", False, frozenset({"L5", "E5b"})),  # BeiDou B2a+b
    ("B2a", True, frozenset({"L5"})),
    ("B2", True, frozenset({"E5b"})),  # remaining B2: B2I, B2Q, B2b
    ("L1", True, frozenset({"L1"})),
    ("E1", True, frozenset({"L1"})),
    ("B1", True, frozenset({"L1"})),
    ("L2", True, frozenset({"L2"})),
    ("L5", True, frozenset({"L5"})),
    ("L3", True, frozenset({"E5b"})),  # GLONASS L3
    ("E6", True, frozenset({"E6"})),
    ("L6", True, frozenset({"E6"})),
    ("B3", True, frozenset({"E6"})),
]


def signal_bands(name: str) -> frozenset[str]:
    """Return the bands a signal name belongs to, empty when unknown."""
    for s, prefix, bands in _SIGNAL_BANDS:
        if (prefix and name.startswith(s)) or name == s:
            return bands
    return frozenset()


@dataclass
class Mismatch:
    prop: str
    expected: object
    actual: object
    note: str = ""

    def __str__(self) -> str:
        s = f"{self.prop}: expected {self.expected}, got {self.actual}"
        if self.note:
            s += f" ({self.note})"
        return s


def compare_case(case: Case, baseline: Baseline, config: dict[str, Any]) -> list[Mismatch]:
    """Compare the case's requested properties against the readback config."""
    e = case.expect
    if isinstance(e, SignalsExpect):
        return _compare_signals(e, baseline, config)
    if isinstance(e, TimeGNSSExpect):
        return _exact("timeGNSS", e.gnss, config.get("timeGNSS"))
    if isinstance(e, TimePulseExpect):
        return _compare_time_pulse(e, baseline, config)
    if isinstance(e, ModeExpect):
        return _compare_mode(e, baseline, config)
    if isinstance(e, AntCableDelayExpect):
        return _approx("antennaCableDelay", e.seconds, config.get("antennaCableDelay"), ANT_DELAY_TOL)
    if isinstance(e, MinElevExpect):
        return _approx("minElevation", e.degrees, config.get("minElevation"), 1e-6)
    if isinstance(e, RTCMBaseIDExpect):
        return _exact("rtcmBaseID", e.id, config.get("rtcmBaseID"))
    raise AssertionError(f"unhandled expectation {e!r}")


def _exact(prop: str, expected: object, actual: object) -> list[Mismatch]:
    if actual is None:
        return [Mismatch(prop, expected, None, "not reported by readback")]
    if actual != expected:
        return [Mismatch(prop, expected, actual)]
    return []


def _approx(prop: str, expected: float, actual: object, tol: float) -> list[Mismatch]:
    if not isinstance(actual, (int, float)) or isinstance(actual, bool):
        return [Mismatch(prop, expected, actual, "not reported by readback")]
    if abs(float(actual) - expected) > tol:
        return [Mismatch(prop, expected, actual, f"tolerance {tol:g}")]
    return []


def _compare_signals(e: SignalsExpect, baseline: Baseline, config: dict[str, Any]) -> list[Mismatch]:
    actual = config.get("signalsEnabled")
    if not isinstance(actual, dict):
        return [Mismatch("signalsEnabled", sorted(e.gnss), None, "not reported by readback")]
    expected_gnss = e.gnss & baseline.supported_gnss
    actual_gnss = frozenset(str(g) for g in actual)
    allowed_extra: frozenset[str] = frozenset()
    for g, extra in COUPLED_GNSS.get(baseline.vendor, {}).items():
        if g in expected_gnss:
            allowed_extra |= extra
    mm: list[Mismatch] = []
    missing = expected_gnss - actual_gnss
    extra = actual_gnss - expected_gnss - allowed_extra
    if missing or extra:
        mm.append(
            Mismatch(
                "signalsEnabled",
                ",".join(sorted(expected_gnss)),
                ",".join(sorted(actual_gnss)),
                "constellation set",
            )
        )
    # Band-level check only when bands were requested and the receiver
    # can be steered per band; without band support the constellation
    # check above is all we can ask for.
    if e.bands is not None and "band" in baseline.supports:
        for g in sorted(expected_gnss & actual_gnss):
            sigs = actual.get(g)
            if not isinstance(sigs, list):
                continue
            for sig in sigs:
                bands = signal_bands(str(sig))
                if not bands:
                    mm.append(Mismatch("signalsEnabled", sorted(e.bands), f"{g} {sig}", "unknown signal band"))
                elif not bands <= e.bands:
                    mm.append(
                        Mismatch(
                            "signalsEnabled",
                            f"{g} signals in bands " + ",".join(sorted(e.bands)),
                            f"{g} {sig} in bands " + ",".join(sorted(bands)),
                        )
                    )
    return mm


def _compare_time_pulse(e: TimePulseExpect, baseline: Baseline, config: dict[str, Any]) -> list[Mismatch]:
    tp = config.get("timePulse")
    if not isinstance(tp, dict):
        return [Mismatch("timePulse", e, None, "not reported by readback")]
    quant = TP_QUANT.get(baseline.vendor, 0.0)
    tol = quant + 1e-12
    mm = _approx("timePulse.width", e.width, tp.get("width"), tol)
    if e.width == 0:
        # Disabled: only the width matters.
        return mm
    if e.period is not None:
        mm += _approx("timePulse.period", e.period, tp.get("period"), tol)
    for key, expected in (
        ("alignToGNSS", e.align_to_gnss),
        ("onlyWhenLocked", e.only_when_locked),
        ("polarityRising", e.polarity_rising),
    ):
        if expected is not None:
            mm += _exact(f"timePulse.{key}", expected, tp.get(key))
    return mm


def _compare_mode(e: ModeExpect, baseline: Baseline, config: dict[str, Any]) -> list[Mismatch]:
    mode = config.get("mode")
    if not isinstance(mode, dict):
        return [Mismatch("mode", e, None, "not reported by readback")]
    mm = _exact("mode.static", e.static, mode.get("static"))
    expected_ecef = e.ecef if e.ecef is not None else (llh_to_ecef(*e.llh) if e.llh is not None else None)
    if expected_ecef is not None:
        actual_ecef = _readback_ecef(mode)
        if actual_ecef is None:
            mm.append(Mismatch("mode.fixedPos", expected_ecef, None, "no fixed position in readback"))
        else:
            for i, axis in enumerate("XYZ"):
                if abs(actual_ecef[i] - expected_ecef[i]) > POS_TOL:
                    mm.append(
                        Mismatch(
                            f"mode.fixedPosECEF.{axis}",
                            expected_ecef[i],
                            actual_ecef[i],
                            f"tolerance {POS_TOL:g} m",
                        )
                    )
    if e.acc is not None and "fixedPosAcc" in baseline.supports:
        mm += _approx("mode.fixedPosAcc", e.acc, mode.get("fixedPosAcc"), POS_TOL)
    return mm


def _readback_ecef(mode: dict[str, Any]) -> tuple[float, float, float] | None:
    """Fixed position from a readback mode, converting LLH when that is
    how the receiver reports it."""
    ecef = mode.get("fixedPosECEF")
    if isinstance(ecef, list) and len(ecef) == 3:
        return (float(ecef[0]), float(ecef[1]), float(ecef[2]))
    llh = mode.get("fixedPosLLH")
    if isinstance(llh, list) and len(llh) == 2:
        height = mode.get("height")
        h = float(height) if isinstance(height, (int, float)) else 0.0
        return llh_to_ecef(float(llh[0]), float(llh[1]), h)
    return None


def llh_to_ecef(lat: float, lon: float, height: float) -> tuple[float, float, float]:
    """Convert WGS84 geodetic coordinates (degrees, meters) to ECEF meters."""
    a = 6378137.0
    f = 1 / 298.257223563
    e2 = f * (2 - f)
    phi = math.radians(lat)
    lam = math.radians(lon)
    n = a / math.sqrt(1 - e2 * math.sin(phi) ** 2)
    x = (n + height) * math.cos(phi) * math.cos(lam)
    y = (n + height) * math.cos(phi) * math.sin(lam)
    z = (n * (1 - e2) + height) * math.sin(phi)
    return (x, y, z)
