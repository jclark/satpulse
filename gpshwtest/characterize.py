"""Derive the characterization from probe observations.

The characterization records the receiver's limitations relative to perfect
realization of the configuration model: properties realized exactly as
requested get no entry. Observations that fit no known pattern are carried
verbatim rather than dropped.
"""

import json
from typing import Any

from model import EmissionObservation, Observation, SignalObservation

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
                 observations: list[Observation],
                 signal_observations: list[SignalObservation],
                 emission_observations: list[EmissionObservation],
                 enabled_gnss: list[str],
                 save_results: list[dict[str, Any]] | None = None) -> dict[str, Any]:
    """Build the characterization document for one receiver+firmware.
    enabled_gnss is the constellation set that was enabled while message
    output was probed; it scopes the expected RTCM MSM types."""
    limits: dict[str, Any] = {}
    for prop in sorted({o.prop for o in observations}):
        entry = characterize_prop([o for o in observations if o.prop == prop])
        if entry:
            limits[prop] = entry
    signals = characterize_signals(signal_observations)
    if signals:
        limits["signals"] = signals
    by_group = {g: [o for o in emission_observations if o.group == g]
                for g in ("nmeaOut", "rtcmOut", "rawOut", "pvtOut", "satsOut")}
    nmea = characterize_nmea(by_group["nmeaOut"])
    if nmea:
        limits["nmeaOut"] = nmea
    rtcm = characterize_rtcm(by_group["rtcmOut"], enabled_gnss)
    if rtcm:
        limits["rtcmOut"] = rtcm
    raw = characterize_raw(by_group["rawOut"])
    if raw:
        limits["rawOut"] = raw
    pvt = characterize_expected(by_group["pvtOut"])
    if pvt:
        limits["pvtOut"] = pvt
    sats = characterize_expected(by_group["satsOut"])
    if sats:
        limits["satsOut"] = sats
    save = characterize_save(save_results or [])
    if save:
        limits["saveGranularity"] = save
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
    signal_patterns(entry, obs, supported, dedup(refused), dedup(adjusted))
    return entry if entry else None


def signal_patterns(entry: dict[str, Any], obs: list[SignalObservation],
                    supported: dict[str, list[str]],
                    refused: list[dict[str, Any]],
                    adjusted: list[dict[str, Any]]) -> None:
    """Express the refused and adjusted signal sets as receiver validity
    patterns where the observations support them, in the spirit of "an
    enabled constellation must have all its signals enabled, except BDS".
    Entries no pattern explains stay verbatim - patterns reduce the noise,
    never the information. The patterns, each emitted only when observed
    and never contradicted:

    - cannotBeAlone: constellations refused as the only enabled ones but
      accepted in combination (augmentation systems needing GPS).
    - partialSelectionRefused: a request giving some constellation a
      nonempty strict subset of its signals is refused; "except" lists the
      subsets observed to be accepted exactly.
    - emptySelectionRefused: a request that would enable a constellation
      with no signals is refused.
    - emptyConstellationsDropped: constellations denoted with no signals
      are dropped from the request rather than refused."""
    full = {c: tuple(s) for c, s in supported.items()}
    enabled_in_combination = set()
    allowed_subsets: dict[str, list[list[str]]] = {}
    empty_accepted = False
    for o in obs:
        if o.error is not None or o.achieved is None:
            continue
        req = requested_signals(o.gnss, o.band, supported)
        if len([c for c, s in (req or {}).items() if s]) > 1:
            enabled_in_combination |= set(o.achieved)
        if req is not None and any(not s for s in req.values()):
            empty_accepted = True
        for c, sigs in o.achieved.items():
            t = sorted(sigs)
            if c in full and tuple(t) != full[c] and req is not None \
                    and {k: sorted(v) for k, v in o.achieved.items()} == req:
                if t not in allowed_subsets.setdefault(c, []):
                    allowed_subsets[c].append(t)
    cannot_alone = sorted(
        c for c in supported if c in enabled_in_combination
        and any(r.get("signals") == {c: list(full[c])} for r in refused))
    saw_partial = False
    saw_empty = False
    residual_refused = []
    for r in refused:
        req = r.get("signals")
        if not isinstance(req, dict):
            residual_refused.append(r)
            continue
        nonempty = {c for c, s in req.items() if s}
        if nonempty and nonempty <= set(cannot_alone):
            continue
        if any(s and tuple(sorted(s)) != full.get(c)
               and sorted(s) not in allowed_subsets.get(c, [])
               for c, s in req.items()):
            saw_partial = True
            continue
        if any(not s for s in req.values()) and not empty_accepted:
            saw_empty = True
            continue
        residual_refused.append(r)
    saw_dropped = False
    residual_adjusted = []
    for a in adjusted:
        req = a.get("requested")
        if isinstance(req, dict) and any(req.values()) \
                and a.get("achieved") == {c: s for c, s in req.items() if s}:
            saw_dropped = True
            continue
        residual_adjusted.append(a)
    if cannot_alone:
        entry["cannotBeAlone"] = cannot_alone
    if saw_partial:
        entry["partialSelectionRefused"] = \
            {"except": allowed_subsets} if allowed_subsets else True
    if saw_empty:
        entry["emptySelectionRefused"] = True
    if saw_dropped:
        entry["emptyConstellationsDropped"] = True
    if residual_refused:
        entry["refused"] = residual_refused
    if residual_adjusted:
        entry["adjusted"] = residual_adjusted


def dedup(entries: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Collapse duplicates and order canonically. Distinct requests can
    denote the same signal set; the characterization records the sets, not
    the probe outcomes, so duplicates and probe order must not show."""
    seen: dict[str, dict[str, Any]] = {}
    for e in entries:
        seen.setdefault(json.dumps(e, sort_keys=True), e)
    return [seen[k] for k in sorted(seen)]


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


def characterize_expected(obs: list[EmissionObservation]) -> dict[str, Any] | None:
    """Characterize information-level message output against the information
    kinds the request should deliver (recorded with the observation). The
    semantics are best-effort at the message level: delivering more than was
    asked for is normal (a needed message may carry extra information), so
    only missing requested information is a limitation."""
    entry: dict[str, Any] = {}
    refused = [o.requested for o in obs if o.error is not None]
    if refused:
        entry["refused"] = refused
    missing = []
    for o in obs:
        if o.error is not None:
            continue
        lack = sorted(set(o.expect or []) - set(o.emitted))
        if lack:
            missing.append({"requested": o.requested, "missing": lack})
    if missing:
        entry["missing"] = missing
    return entry if entry else None


def characterize_save(results: list[dict[str, Any]]) -> dict[str, Any] | None:
    """Characterize save granularity from the per-property experiments.
    Perfect realization is one save group per property, which gets no
    entry: only groups of properties that persist together are limitations.
    The pairwise observations must be consistent (saving p persisted q iff
    saving q persisted p, and the together-relation is transitive); if they
    are not, the receiver's behavior is not a partition and the experiments
    are carried verbatim. Pairs no experiment could decide (a property's
    running value never left its NVM value) are listed as indeterminate."""
    if not results:
        return None
    entry: dict[str, Any] = {}
    errors = [r for r in results if "error" in r]
    anomalies = [a for r in results for a in r.get("anomalies", [])]
    together: dict[tuple[str, str], bool] = {}
    consistent = True
    props = set()
    for r in results:
        if "saved" not in r:
            continue
        props.add(r["prop"])
        for q, val in [(q, True) for q in r["saved"]] + \
                      [(q, False) for q in r["independent"]]:
            props.add(q)
            k = (min(r["prop"], q), max(r["prop"], q))
            if together.setdefault(k, val) != val:
                consistent = False
    groups = merge_groups(props, together)
    for g in groups:
        for a in g:
            for b in g:
                if a < b and together.get((a, b)) is False:
                    consistent = False
    if not consistent:
        entry["observations"] = [r for r in results if "saved" in r]
    else:
        grouped = sorted(sorted(g) for g in groups if len(g) > 1)
        if grouped:
            entry["groups"] = grouped
        undecided = sorted(f"{a}/{b}" for a in props for b in props
                           if a < b and (a, b) not in together)
        if undecided and grouped:
            entry["indeterminate"] = undecided
    if errors:
        entry["refused"] = [{"prop": r["prop"], "error": r["error"]} for r in errors]
    if anomalies:
        entry["anomalies"] = anomalies
    return entry if entry else None


def merge_groups(props: set[str], together: dict[tuple[str, str], bool]) -> list[set[str]]:
    """Connected components of the persists-together relation."""
    groups = [{p} for p in sorted(props)]
    for (a, b), v in together.items():
        if not v:
            continue
        ga = next(g for g in groups if a in g)
        gb = next(g for g in groups if b in g)
        if ga is not gb:
            ga |= gb
            groups.remove(gb)
    return groups


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
