"""Derive the characterization from probe observations.

The characterization records the receiver's limitations relative to perfect
realization of the configuration model: properties realized exactly as
requested get no entry. Observations that fit no known pattern are carried
verbatim rather than dropped.
"""

import json
from typing import Any

from probes import Observation, SignalObservation



def characterize(receiver: dict[str, Any], supports: list[str],
                 observations: list[Observation],
                 signal_observations: list[SignalObservation]) -> dict[str, Any]:
    """Build the characterization document for one receiver+firmware."""
    limits: dict[str, Any] = {}
    for prop in sorted({o.prop for o in observations}):
        entry = characterize_prop([o for o in observations if o.prop == prop])
        if entry:
            limits[prop] = entry
    signals = characterize_signals(signal_observations)
    if signals:
        limits["signals"] = signals
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
