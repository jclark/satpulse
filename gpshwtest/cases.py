"""Test tables for the readback groups.

Each group is a function from the session baseline to a list of cases,
ordered so the last case leaves the receiver in a sane state. A case is
one set invocation followed by an independent --show-config readback.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Callable

from runner import Baseline

# Changing the GNSS set causes an internal restart on some u-blox
# platforms; wait this long after a signal-set case before reading back.
SIGNALS_SETTLE = 2.0

# Test position: Bangkok (also used by casictool's hardware tests).
TEST_ECEF = (-1144698.0455, 6090335.4099, 1504171.3914)
TEST_LLH = (13.75, 100.5, 10.0)

# Receiver-default minimum elevation in degrees, keyed by vendor.
MIN_ELEV_DEFAULT = {"u-blox": 5, "Unicore": 5}


@dataclass(frozen=True)
class SignalsExpect:
    """Expected signal configuration.

    gnss is the requested constellation set; the comparator bounds it by
    the receiver's supported GNSS. bands, when set, requires every
    reported signal to lie in one of the requested bands (only checked
    on receivers with band support).
    """

    gnss: frozenset[str]
    bands: frozenset[str] | None = None


@dataclass(frozen=True)
class TimeGNSSExpect:
    gnss: str


@dataclass(frozen=True)
class TimePulseExpect:
    """Expected time pulse configuration in seconds.

    width 0 means disabled and the remaining fields are not compared.
    Width and period comparisons allow per-family quantization.
    """

    width: float
    period: float | None = None
    align_to_gnss: bool | None = None
    only_when_locked: bool | None = None
    polarity_rising: bool | None = None


@dataclass(frozen=True)
class ModeExpect:
    """Expected positioning mode.

    For survey cases only static is verified (the survey is not awaited).
    ecef/llh positions compare within the receiver's storage resolution.
    acc is only compared on receivers with fixedPosAcc support.
    """

    static: bool
    ecef: tuple[float, float, float] | None = None
    llh: tuple[float, float, float] | None = None
    acc: float | None = None


@dataclass(frozen=True)
class AntCableDelayExpect:
    seconds: float


@dataclass(frozen=True)
class MinElevExpect:
    degrees: float


@dataclass(frozen=True)
class RTCMBaseIDExpect:
    id: int


Expect = (
    SignalsExpect
    | TimeGNSSExpect
    | TimePulseExpect
    | ModeExpect
    | AntCableDelayExpect
    | MinElevExpect
    | RTCMBaseIDExpect
)


@dataclass(frozen=True)
class Case:
    desc: str
    flags: tuple[str, ...]
    expect: Expect
    requires: frozenset[str] = frozenset()
    settle: float = 0.0


def pps(width: float) -> TimePulseExpect:
    """The configuration satpulsetool requests for --pps width: 1 s period,
    rising edge, aligned to GNSS time, only when locked."""
    return TimePulseExpect(
        width=width,
        period=1.0,
        align_to_gnss=True,
        only_when_locked=True,
        polarity_rising=True,
    )


def signals_cases(baseline: Baseline) -> list[Case]:
    def c(desc: str, gnss: str, bands: str | None = None, requires: frozenset[str] = frozenset()) -> Case:
        flags = ("--gnss", gnss) + (("--band", bands) if bands else ())
        return Case(
            desc,
            flags,
            SignalsExpect(
                gnss=frozenset(gnss.split(",")),
                bands=frozenset(bands.split(",")) if bands else None,
            ),
            requires=requires,
            settle=SIGNALS_SETTLE,
        )

    band = frozenset({"band"})
    return [
        c("GPS+GAL+BDS", "GPS,GAL,BDS"),
        c("GPS+GAL", "GPS,GAL"),
        c("GPS only", "GPS"),
        c("GPS+GAL L1 only", "GPS,GAL", "L1", band),
        c("GPS+GAL L1+L2", "GPS,GAL", "L1,L2", band),
        c("GPS+GAL L1+L5", "GPS,GAL", "L1,L5", band),
        c("all majors", "GPS,GAL,BDS,GLO"),
    ]


def time_gnss_cases(baseline: Baseline) -> list[Case]:
    cycle = [g for g in ("GAL", "BDS", "GLO") if g in baseline.supported_gnss]
    cycle.append("GPS")
    return [
        Case(f"time GNSS {g}", ("--time-gnss", g), TimeGNSSExpect(g)) for g in cycle
    ]


def tp_cases(baseline: Baseline) -> list[Case]:
    return [
        Case("PPS disabled", ("--pps", "0"), TimePulseExpect(width=0)),
        Case("PPS 1 ms", ("--pps", "0.001"), pps(0.001)),
        Case("PPS 250 ms", ("--pps", "0.25"), pps(0.25)),
        Case("PPS 100 ms (standard)", ("--pps", "0.1"), pps(0.1)),
    ]


def ant_cable_delay_cases(baseline: Baseline) -> list[Case]:
    return [
        Case(f"antenna cable delay {ns} ns", ("--ant-cable-delay", str(ns)), AntCableDelayExpect(ns / 1e9))
        for ns in (100, 250, 0)
    ]


def min_elev_cases(baseline: Baseline) -> list[Case]:
    default = MIN_ELEV_DEFAULT.get(baseline.vendor, 5)
    return [
        Case(f"min elevation {deg} deg", ("--min-elev", str(deg)), MinElevExpect(float(deg)))
        for deg in (5, 15, default)
    ]


def mode_cases(baseline: Baseline) -> list[Case]:
    x, y, z = TEST_ECEF
    lat, lon, height = TEST_LLH
    return [
        Case(
            "survey 120 s / 50 m",
            ("--survey", "--survey-time", "120", "--survey-acc", "50"),
            ModeExpect(static=True),
            requires=frozenset({"survey"}),
        ),
        Case(
            "fixed position ECEF",
            ("--fixed-pos-ecef", f"{x},{y},{z}", "--fixed-pos-acc", "10"),
            ModeExpect(static=True, ecef=TEST_ECEF, acc=10.0),
            requires=frozenset({"fixedPos"}),
        ),
        Case(
            "fixed position LLH",
            ("--fixed-pos-llh", f"{lat},{lon},{height}", "--fixed-pos-acc", "10"),
            ModeExpect(static=True, llh=TEST_LLH, acc=10.0),
            requires=frozenset({"fixedPos"}),
        ),
        Case("mobile", ("--mobile",), ModeExpect(static=False)),
    ]


def rtcm_base_id_cases(baseline: Baseline) -> list[Case]:
    req = frozenset({"rtcmBaseID"})
    return [
        Case(f"RTCM base ID {i}", ("--rtcm-base-id", str(i)), RTCMBaseIDExpect(i), requires=req)
        for i in (99, 1023, 0)
    ]


# Groups in canonical run order. The key doubles as the CLI flag name.
GROUPS: list[tuple[str, Callable[[Baseline], list[Case]]]] = [
    ("signals", signals_cases),
    ("time-gnss", time_gnss_cases),
    ("tp", tp_cases),
    ("ant-cable-delay", ant_cable_delay_cases),
    ("min-elev", min_elev_cases),
    ("mode", mode_cases),
    ("rtcm-base-id", rtcm_base_id_cases),
]
