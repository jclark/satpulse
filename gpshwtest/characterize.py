"""Derive the characterization from probe observations.

The characterization records the receiver's limitations relative to perfect
realization of the configuration model: properties realized exactly as
requested get no entry. Observations that fit no known pattern are carried
verbatim rather than dropped.
"""

import json
from typing import Any

from probes import Observation

QUANTA = [1e-9, 1e-8, 1e-7, 1e-6, 1e-5, 1e-4, 1e-3, 1e-2, 1e-1, 1.0]


def characterize(receiver: dict[str, Any], supports: list[str],
                 observations: list[Observation]) -> dict[str, Any]:
    """Build the characterization document for one receiver+firmware."""
    limits: dict[str, Any] = {}
    for prop in sorted({o.prop for o in observations}):
        entry = characterize_prop([o for o in observations if o.prop == prop])
        if entry:
            limits[prop] = entry
    return {"receiver": receiver, "supports": sorted(supports), "limitations": limits}


def characterize_prop(obs: list[Observation]) -> dict[str, Any] | None:
    """Characterize one property; None when realization was perfect."""
    entry: dict[str, Any] = {}
    refused = [o for o in obs if o.error is not None]
    if refused:
        entry["refused"] = [{"requested": o.requested, "error": o.error} for o in refused]
    ok = [o for o in obs if o.error is None]
    inexact = [o for o in ok if o.readback != o.requested]
    if inexact:
        q = fit_quantum(ok)
        if q is not None:
            entry["quantum"] = q
        else:
            entry["observations"] = [
                {"requested": o.requested, "achieved": o.readback} for o in inexact]
    return entry if entry else None


def fit_quantum(obs: list[Observation]) -> float | None:
    """Find the smallest quantum that explains every achieved value, if any.

    A quantum fits when each achieved value is a multiple of it within one
    step of the request, which covers receivers that round and that truncate.
    """
    if not all(isinstance(o.requested, (int, float)) and
               isinstance(o.readback, (int, float)) for o in obs):
        return None
    for q in QUANTA:
        if all(quantum_fits(q, float(o.requested), float(o.readback)) for o in obs):
            return q
    return None


def quantum_fits(q: float, requested: float, achieved: float) -> bool:
    n = achieved / q
    return abs(n - round(n)) <= 1e-6 and abs(achieved - requested) < q


def to_json(doc: dict[str, Any]) -> str:
    """Render a characterization canonically so runs compare byte-for-byte."""
    return json.dumps(doc, indent=2, sort_keys=True) + "\n"
