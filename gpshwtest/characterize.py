"""Derive the characterization from probe observations.

The characterization records the receiver's limitations relative to perfect
realization of the configuration model: properties realized exactly as
requested get no entry. Observations that fit no known pattern are carried
verbatim rather than dropped.
"""

import json
from typing import Any

from probes import (EmissionObservation, Observation, ProbeRun,
                    PVT_CASES, SATS_CASES, SignalObservation)

# RTCM MSM message numbers are <decade><level> with a fixed decade per
# constellation (RTCM 10403 standard numbering, not receiver-specific).
RTCM_DECADE = {"GPS": 107, "GLO": 108, "GAL": 109, "SBAS": 110, "QZSS": 111,
               "BDS": 112, "NAVIC": 113}

# Frequency band of a signal name, by prefix, ported from the model's
# signalIDBandTable (gps/gpsprot/signal.go). Order matters: longer or more
# specific prefixes come first.
SIGNAL_BAND_PREFIXES = [
    ("B2a", "L5"), ("E5b", "E5b"), ("E5a", "L5"), ("L5", "L5"),
    ("B2", "E5b"), ("L3", "E5b"), ("L1", "L1"), ("E1", "L1"), ("B1", "L1"),
    ("L2", "L2"), ("E6", "E6"), ("L6", "E6"), ("B3", "E6"),
]

# The signal-band keys denoted by each band name a request can use.
REQUEST_BANDS = {"L1": {"L1"}, "L2": {"L2"}, "L5": {"L5"}, "E5b": {"E5b"},
                 "E5": {"L5", "E5b"}, "E6": {"E6"}, "L6": {"E6"}}


def signal_band(name: str) -> str:
    for prefix, band in SIGNAL_BAND_PREFIXES:
        if name.startswith(prefix):
            return band
    return ""


def requested_signals(gnss: list[str], band: list[str] | None,
                      supported: dict[str, list[str]]) -> dict[str, list[str]] | None:
    """The signal set a constellation/band request denotes, intersected with
    the receiver's supported set - what satpulse actually requests. None when
    the supported set does not cover a named constellation (then the request
    cannot be expressed in discovered signals and is carried verbatim)."""
    if any(c not in supported for c in gnss):
        return None
    if band is None:
        return {c: supported[c] for c in gnss}
    keys = set().union(*(REQUEST_BANDS.get(b, set()) for b in band))
    return {c: [s for s in supported[c] if signal_band(s) in keys] for c in gnss}



def characterize(receiver: dict[str, Any], supports: list[str],
                 run: ProbeRun, enabled_gnss: list[str]) -> dict[str, Any]:
    """Build the characterization document for one receiver+firmware.
    enabled_gnss is the constellation set that was enabled while message
    output was probed; it scopes the expected RTCM MSM types."""
    limits: dict[str, Any] = {}
    for prop in sorted({o.prop for o in run.observations}):
        entry = characterize_prop([o for o in run.observations if o.prop == prop])
        if entry:
            limits[prop] = entry
    signals = characterize_signals(run.signal_observations)
    if signals:
        limits["signals"] = signals
    by_group = {g: [o for o in run.emission_observations if o.group == g]
                for g in ("nmeaOut", "rtcmOut", "rawOut")}
    nmea = characterize_nmea(by_group["nmeaOut"])
    if nmea:
        limits["nmeaOut"] = nmea
    rtcm = characterize_rtcm(by_group["rtcmOut"], enabled_gnss)
    if rtcm:
        limits["rtcmOut"] = rtcm
    raw = characterize_raw(by_group["rawOut"])
    if raw:
        limits["rawOut"] = raw
    pvt = characterize_expected(
        [o for o in run.emission_observations if o.group == "pvtOut"],
        {tuple(c.flags): c.expect for c in PVT_CASES})
    if pvt:
        limits["pvtOut"] = pvt
    sats = characterize_expected(
        [o for o in run.emission_observations if o.group == "satsOut"],
        {tuple(c): e for c, e in SATS_CASES})
    if sats:
        limits["satsOut"] = sats
    return {"receiver": receiver, "supports": sorted(supports), "limitations": limits}


def characterize_prop(obs: list[Observation]) -> dict[str, Any] | None:
    """Characterize one property; None when realization was perfect."""
    entry: dict[str, Any] = {}
    refused = [o for o in obs if o.error is not None]
    if refused:
        entry["refused"] = [o.requested for o in refused]
    ok = [o for o in obs if o.error is None]
    if ok and all(o.reported is None and o.readback is None for o in ok):
        # Setting reported nothing achieved and readback shows no such
        # property: the backend does not have this property. (A reported
        # value that cannot be read back is a guarantee violation, caught
        # as a failure by the probes, never a characterization category.)
        entry["unsupported"] = True
        return entry
    inexact = [o for o in ok if o.readback != o.requested]
    if inexact:
        dp = fit_quantum(ok)
        if dp is not None:
            entry["quantum"] = 10 ** -dp
        else:
            entry["observations"] = [
                {"requested": o.requested, "achieved": o.readback} for o in inexact]
    return entry if entry else None


def characterize_signals(obs: list[SignalObservation]) -> dict[str, Any] | None:
    """Characterize signal-set realization, in signal-set vocabulary.

    The supported set is what full requests achieve per constellation.
    Requests are expressed as the signal sets they denote (intersected
    with the supported set, which is what satpulse requests): refusals
    record the refused set, and accepted requests whose achieved set
    differs from the requested one record both. Exact realization gets
    no entry. Error wording is satpulsetool presentation and is omitted."""
    entry: dict[str, Any] = {}
    supported: dict[str, list[str]] = {}
    inconsistent = []
    for o in obs:
        if o.error is not None or o.band is not None or o.achieved is None:
            continue
        for c, sigs in o.achieved.items():
            if c in supported and supported[c] != sorted(sigs):
                inconsistent.append({"gnss": o.gnss, "achieved": o.achieved})
            supported.setdefault(c, sorted(sigs))
    refused = []
    adjusted = []
    for o in obs:
        req = requested_signals(o.gnss, o.band, supported)
        if o.error is not None:
            refused.append({"signals": req} if req is not None
                           else {"gnss": o.gnss, "band": o.band})
        elif o.achieved is not None:
            achieved = {c: sorted(s) for c, s in o.achieved.items()}
            if req is None:
                if sorted(achieved) != sorted(o.gnss):
                    adjusted.append({"gnss": o.gnss, "band": o.band,
                                     "achieved": achieved})
            elif achieved != req:
                adjusted.append({"requested": req, "achieved": achieved})
    if supported:
        entry["signalSet"] = supported
    if inconsistent:
        entry["inconsistent"] = inconsistent
    if refused:
        entry["refused"] = refused
    if adjusted:
        entry["adjusted"] = adjusted
    return entry if entry else None


def characterize_nmea(obs: list[EmissionObservation]) -> dict[str, Any] | None:
    """Characterize NMEA output selection: requested sentence types that
    were not emitted. Extra sentence types are normal best-effort behavior."""
    entry: dict[str, Any] = {}
    refused = [o.requested for o in obs if o.error is not None]
    if refused:
        entry["refused"] = refused
    missing = []
    for o in obs:
        if o.error is not None:
            continue
        requested = [] if o.requested == ["none"] else sorted(o.requested)
        lack = sorted(set(requested) - set(o.emitted))
        if lack:
            missing.append({"requested": requested, "missing": lack})
    if missing:
        entry["missing"] = missing
    return entry if entry else None


def characterize_rtcm(obs: list[EmissionObservation],
                      enabled: list[str]) -> dict[str, Any] | None:
    """Characterize RTCM output: message types implied by the request for
    the enabled constellations that were not emitted. Extra types are
    normal best-effort behavior."""
    entry: dict[str, Any] = {}
    refused = [o.requested for o in obs if o.error is not None]
    if refused:
        entry["refused"] = refused
    missing = []
    for o in obs:
        if o.error is not None:
            continue
        expected: set[str] = set()
        for f in o.requested:
            if f in ("MSM4", "MSM7"):
                expected |= {f"{RTCM_DECADE[c]}{f[-1]}" for c in enabled
                             if c in RTCM_DECADE}
            elif f == "ARP":
                expected.add("1005")
        lack = sorted(expected - set(o.emitted))
        if lack:
            missing.append({"requested": o.requested, "missing": lack})
    if missing:
        entry["missing"] = missing
    return entry if entry else None


def characterize_raw(obs: list[EmissionObservation]) -> dict[str, Any] | None:
    """Characterize raw output: a requested kind that produced no new
    emission is missing; anything beyond the request is normal."""
    entry: dict[str, Any] = {}
    refused = [o.requested for o in obs if o.error is not None]
    if refused:
        entry["refused"] = refused
    missing = [{"requested": o.requested, "missing": o.requested}
               for o in obs
               if o.error is None and o.requested != ["none"] and not o.emitted]
    if missing:
        entry["missing"] = missing
    return entry if entry else None


def characterize_expected(obs: list[EmissionObservation],
                          expect: dict[tuple[str, ...], set[str]]) -> dict[str, Any] | None:
    """Characterize information-level message output. The semantics are
    best-effort at the message level: delivering more than was asked for is
    normal (a needed message may carry extra information), so only missing
    requested information is a limitation."""
    entry: dict[str, Any] = {}
    refused = [o.requested for o in obs if o.error is not None]
    if refused:
        entry["refused"] = refused
    missing = []
    for o in obs:
        if o.error is not None:
            continue
        want = expect.get(tuple(o.requested), set())
        lack = sorted(want - set(o.emitted))
        if lack:
            missing.append({"requested": o.requested, "missing": lack})
    if missing:
        entry["missing"] = missing
    return entry if entry else None


def fit_quantum(obs: list[Observation]) -> int | None:
    """Find decimal places dp such that quantum 10^-dp explains every achieved
    value, returning the smallest fitting quantum. A quantum fits when the
    achieved value has at most dp decimals and is within one step of the
    request, which covers receivers that round and ones that truncate."""
    if not all(isinstance(o.requested, (int, float)) and
               isinstance(o.readback, (int, float)) for o in obs):
        return None
    for dp in range(9, -1, -1):
        if all(quantum_fits(dp, float(o.requested), float(o.readback)) for o in obs):
            return dp
    return None


def quantum_fits(dp: int, requested: float, achieved: float) -> bool:
    return abs(achieved - requested) < 10 ** -dp and round(achieved, dp) == achieved


def to_json(doc: dict[str, Any]) -> str:
    """Render a characterization canonically so runs compare byte-for-byte."""
    return json.dumps(doc, indent=2, sort_keys=True) + "\n"
