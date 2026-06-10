"""Derive the characterization from probe observations.

The characterization records the receiver's limitations relative to perfect
realization of the configuration model: properties realized exactly as
requested get no entry. Observations that fit no known pattern are carried
verbatim rather than dropped.
"""

import json
from typing import Any

from probes import (EmissionObservation, NMEA_VOCAB, Observation, ProbeRun,
                    PVT_CASES, PVT_EXTRA_KINDS, SATS_CASES, SignalObservation)

# RTCM MSM message numbers are <decade><level> with a fixed decade per
# constellation (RTCM 10403 standard numbering, not receiver-specific).
RTCM_DECADE = {"GPS": 107, "GLO": 108, "GAL": 109, "SBAS": 110, "QZSS": 111,
               "BDS": 112, "NAVIC": 113}



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
    if ok and all(o.readback is None for o in ok):
        entry["notReadable"] = True
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
    """Characterize constellation/band realization.

    The per-constellation signal sets achieved at full band are the
    receiver's signal vocabulary; refusals are recorded as the requested
    combination only (error wording is satpulsetool presentation, not
    receiver behavior); accepted band-limited combinations are carried
    verbatim until patterns earn their own vocabulary."""
    entry: dict[str, Any] = {}
    sets: dict[str, list[str]] = {}
    inconsistent = []
    coupled = []
    for o in obs:
        if o.error is not None or o.band is not None or o.achieved is None:
            continue
        for c, sigs in o.achieved.items():
            if c in sets and sets[c] != sigs:
                inconsistent.append({"gnss": o.gnss, "achieved": o.achieved})
            sets.setdefault(c, sigs)
        if sorted(o.achieved) != sorted(o.gnss):
            coupled.append({"gnss": o.gnss, "enabled": sorted(o.achieved)})
    if sets:
        entry["signalSet"] = sets
    if inconsistent:
        entry["inconsistent"] = inconsistent
    if coupled:
        entry["coupled"] = coupled
    refused = [{"gnss": o.gnss, "band": o.band} for o in obs if o.error is not None]
    if refused:
        entry["refused"] = refused
    banded = [{"gnss": o.gnss, "band": o.band, "achieved": o.achieved}
              for o in obs if o.error is None and o.band is not None]
    if banded:
        entry["bands"] = banded
    return entry if entry else None


def characterize_nmea(obs: list[EmissionObservation]) -> dict[str, Any] | None:
    """Characterize NMEA output selection: requests whose emitted sentence
    types differ from what was asked, within the model vocabulary."""
    entry: dict[str, Any] = {}
    refused = [o.requested for o in obs if o.error is not None]
    if refused:
        entry["refused"] = refused
    mismatched = []
    for o in obs:
        if o.error is not None:
            continue
        requested = [] if o.requested == ["none"] else sorted(o.requested)
        emitted = sorted(t for t in o.emitted if t in NMEA_VOCAB)
        if emitted != requested:
            mismatched.append({"requested": requested, "emitted": emitted})
    if mismatched:
        entry["mismatched"] = mismatched
    return entry if entry else None


def characterize_rtcm(obs: list[EmissionObservation],
                      enabled: list[str]) -> dict[str, Any] | None:
    """Characterize RTCM output: emitted message types compared against the
    types implied by the request for the enabled constellations."""
    entry: dict[str, Any] = {}
    refused = [o.requested for o in obs if o.error is not None]
    if refused:
        entry["refused"] = refused
    mismatched = []
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
        missing = sorted(expected - set(o.emitted))
        extra = sorted(set(o.emitted) - expected)
        if missing or extra:
            m: dict[str, Any] = {"requested": o.requested}
            if missing:
                m["missing"] = missing
            if extra:
                m["extra"] = extra
            mismatched.append(m)
    if mismatched:
        entry["mismatched"] = mismatched
    return entry if entry else None


def characterize_raw(obs: list[EmissionObservation]) -> dict[str, Any] | None:
    """Characterize raw output: each kind must produce some new emission,
    none must produce none, and the kinds' realizations must be distinct."""
    entry: dict[str, Any] = {}
    refused = [o.requested for o in obs if o.error is not None]
    if refused:
        entry["refused"] = refused
    mismatched = []
    by_kind: dict[str, set[str]] = {}
    for o in obs:
        if o.error is not None:
            continue
        if o.requested == ["none"]:
            if o.emitted:
                mismatched.append({"requested": [], "emitted": o.emitted})
        else:
            by_kind[o.requested[0]] = set(o.emitted)
            if not o.emitted:
                mismatched.append({"requested": o.requested, "emitted": []})
    if mismatched:
        entry["mismatched"] = mismatched
    overlap = sorted(by_kind.get("obs", set()) & by_kind.get("nav", set()))
    if overlap:
        entry["overlap"] = overlap
    return entry if entry else None


def characterize_expected(obs: list[EmissionObservation],
                          expect: dict[tuple[str, ...], set[str]]) -> dict[str, Any] | None:
    """Characterize information-level message output against per-case
    expected kinds: missing expected information, and unrequested
    information that was delivered anyway."""
    entry: dict[str, Any] = {}
    refused = [o.requested for o in obs if o.error is not None]
    if refused:
        entry["refused"] = refused
    mismatched = []
    for o in obs:
        if o.error is not None:
            continue
        want = expect.get(tuple(o.requested), set())
        got = set(o.emitted)
        missing = sorted(want - got)
        extra = sorted((got & PVT_EXTRA_KINDS) - want)
        if missing or extra:
            m: dict[str, Any] = {"requested": o.requested}
            if missing:
                m["missing"] = missing
            if extra:
                m["extra"] = extra
            mismatched.append(m)
    if mismatched:
        entry["mismatched"] = mismatched
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
