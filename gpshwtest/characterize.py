"""Derive the characterization from probe observations.

The characterization records the receiver's limitations relative to perfect
realization of the configuration model: properties realized exactly as
requested get no entry. Observations that fit no known pattern are carried
verbatim rather than dropped.
"""

import json
from itertools import combinations
from typing import Any

from model import (MAJORS, EmissionObservation, Observation, SignalMap,
                   SignalObservation, l1_signals, l2_signals, l5_signals,
                   normalize_signal_map, signal_request_valid)

# RTCM MSM message numbers are <decade><level> with a fixed decade per
# constellation (RTCM 10403 standard numbering, not receiver-specific).
RTCM_DECADE = {"GPS": 107, "GLO": 108, "GAL": 109, "SBAS": 110, "QZSS": 111,
               "BDS": 112, "NAVIC": 113}

def characterize(receiver: dict[str, Any], supports: list[str],
                 observations: list[Observation],
                 signal_observations: list[SignalObservation],
                 emission_observations: list[EmissionObservation],
                 enabled_gnss: list[str],
                 save_results: list[dict[str, Any]] | None = None,
                 stored_forms: dict[str, str] | None = None) -> dict[str, Any]:
    """Build the characterization document for one receiver+firmware.
    enabled_gnss is the constellation set that was enabled while message
    output was probed; it scopes the expected RTCM MSM types. stored_forms
    records properties the receiver stores in a different representation
    than it accepted (re-expressions of the same value, not changes)."""
    limits: dict[str, Any] = {}
    for prop, form in (stored_forms or {}).items():
        limits[prop] = {"storedAs": form}
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
    rtcm = characterize_rtcm(by_group["rtcmOut"], enabled_gnss, supports)
    if rtcm:
        limits["rtcmOut"] = rtcm
    raw = characterize_raw(by_group["rawOut"], supports)
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
    observed: list[dict[str, Any]] = [
        {"request": o.requested, "error": "refused"}
        for o in obs if o.error is not None]
    ok = [o for o in obs if o.error is None]
    if ok and all(o.reported is None and o.readback is None for o in ok):
        # Setting reported nothing achieved and readback shows no such
        # property: the backend does not have this property. (Accepted and
        # stored values that differ are surfaced below as observations -
        # both reports are truthful, each at its own stage.)
        entry["unsupported"] = True
        return entry
    inexact = [o for o in ok if o.readback != o.requested]
    if inexact:
        dp = fit_quantum(ok)
        if dp is not None:
            entry["quantum"] = 10 ** -dp
        else:
            observed += [{"request": o.requested,
                          **({} if o.readback is None
                             else {"result": o.readback})} for o in inexact]
    observed += [{"request": o.requested, "accepted": o.reported, "stored": o.readback}
                 for o in ok if o.reported != o.readback]
    if observed:
        entry["observed"] = observed
    return entry if entry else None


def characterize_signals(obs: list[SignalObservation]) -> dict[str, Any] | None:
    """Characterize signal-set realization, in signal-set vocabulary.

    The supported set comes from discovery cases. Requests are recorded as
    the signal sets they denote; discovery observations can leave the
    request unknown. Exact realization gets no entry. Error wording is
    satpulsetool presentation and is omitted."""
    entry: dict[str, Any] = {}
    supported, inconsistent = supported_signals(obs)
    refused = []
    adjusted = []
    for o in obs:
        req = observation_request(o, supported)
        if o.error is not None:
            refused.append({"request": req} if req is not None
                           else {"gnss": o.gnss, "band": o.band})
        elif o.achieved is not None:
            achieved = normalize_signal_map(o.achieved)
            if req is None:
                if o.gnss is not None and sorted(achieved) != sorted(o.gnss):
                    adjusted.append({"gnss": o.gnss, "band": o.band,
                                     "result": achieved})
            elif achieved != supported_part(req, supported):
                adjusted.append({"request": req, "result": achieved})
    if supported:
        entry["signalSet"] = supported
    if inconsistent:
        entry["inconsistent"] = inconsistent
    signal_patterns(entry, obs, supported, dedup(refused), dedup(adjusted))
    mismatched: list[dict[str, Any]] = []
    for o in obs:
        if o.accepted is None:
            continue
        req = observation_request(o, supported)
        mismatched.append({
            "request": req if req is not None else {"gnss": o.gnss, "band": o.band},
            "accepted": o.accepted, "stored": o.achieved})
    if mismatched:
        entry["observed"] = dedup(entry.get("observed", []) + mismatched)
    return entry if entry else None


def supported_signals(obs: list[SignalObservation]) -> tuple[SignalMap, list[dict[str, Any]]]:
    """The discovered full signal set plus inconsistent discovery results."""
    supported: SignalMap = {}
    inconsistent = []
    for o in obs:
        if o.error is not None or o.achieved is None:
            continue
        if "discover" not in o.tags and not (o.syntax == "gnss" and o.band is None):
            continue
        for c, sigs in normalize_signal_map(o.achieved).items():
            if c in supported and supported[c] != sigs:
                inconsistent.append({"request": o.requested, "result": o.achieved})
            supported.setdefault(c, sigs)
    for o in obs:
        if o.error is None and o.achieved is not None:
            supported = signal_union(supported, o.achieved)
    return normalize_signal_map(supported), inconsistent


def observation_request(o: SignalObservation, supported: SignalMap) -> SignalMap | None:
    """The model signal set requested by an observation."""
    req = normalize_signal_map(o.requested)
    return req if req else None


def signal_union(a: SignalMap, b: SignalMap) -> SignalMap:
    out = {g: list(sigs) for g, sigs in normalize_signal_map(a).items()}
    for g, sigs in normalize_signal_map(b).items():
        out[g] = sorted(set(out.get(g, [])) | set(sigs))
    return normalize_signal_map(out)


def signal_patterns(entry: dict[str, Any], obs: list[SignalObservation],
                    supported: dict[str, list[str]],
                    refused: list[dict[str, Any]],
                    adjusted: list[dict[str, Any]]) -> None:
    """Express refused and adjusted signal sets as conservative receiver
    validity patterns. Unexplained entries stay verbatim."""
    patterns = {
        "requiresGNSS": requires_gnss(obs, supported),
        "requiresSignalAnyOf": requires_signal_any_of(obs, supported),
        "mutuallyExclusiveSignals": mutually_exclusive_signals(obs, supported),
        "mutuallyExclusiveSignalGroups": mutually_exclusive_signal_groups(obs, supported),
    }
    for k, v in patterns.items():
        if v:
            entry[k] = v
    observed = []
    for r in refused:
        req = normalize_signal_map(r.get("request"))
        if not req:
            observed.append({**r, "error": "refused"})
        elif req == supported_part(req, supported) and signal_request_valid(req) \
                and not refusal_explained(req, patterns):
            observed.append({"request": req, "error": "refused"})
    for a in adjusted:
        req = normalize_signal_map(a.get("request"))
        if not req or req == supported_part(req, supported) and signal_request_valid(req):
            observed.append(a)
    if observed:
        entry["observed"] = dedup(observed)


def requires_gnss(obs: list[SignalObservation], supported: SignalMap) -> dict[str, list[str]]:
    """Augmentation-style constellations that require one of a set of
    major-constellation anchors."""
    result: dict[str, list[str]] = {}
    for target in ("QZSS", "SBAS", "NAVIC"):
        if target not in supported:
            continue
        ok: set[str] = set()
        refused: set[str] = set()
        for o in obs:
            req = observation_request(o, supported)
            if req is None or target not in req:
                continue
            if req != supported_part(req, supported):
                continue
            anchors = [g for g in req if g in MAJORS and g != target]
            if len(anchors) != 1:
                continue
            if o.error is None:
                ok.add(anchors[0])
            else:
                refused.add(anchors[0])
        if ok and refused:
            result[target] = sorted(ok)
    return result


def requires_signal_any_of(obs: list[SignalObservation],
                           supported: SignalMap) -> dict[str, list[list[str]]]:
    """Per-constellation signal groups where at least one signal from each
    group is required."""
    result: dict[str, list[list[str]]] = {}
    for g, sigs in supported.items():
        rows = signal_rows_for_gnss(obs, supported, g)
        ok = [r for r, err in rows if not err]
        bad = [r for r, err in rows if err]
        groups = []
        l1 = l1_signals(sigs)
        if l1 and any(not set(r) & set(l1) for r in bad) \
                and any(set(r) & set(l1) for r in ok):
            groups.append(l1)
        secondary = l2_signals(sigs) + l5_signals(sigs)
        if l1 and secondary and any(set(r) <= set(l1) for r in bad) \
                and any(set(r) & set(l1) and set(r) & set(secondary) for r in ok):
            alts = sorted({s for r in ok for s in r if s in secondary})
            if alts:
                groups.append(alts)
        if groups:
            result[g] = groups
    return result


def mutually_exclusive_signals(obs: list[SignalObservation],
                               supported: SignalMap) -> list[SignalMap]:
    """Two-signal requests that are refused while their individual signals
    are accepted."""
    singles = set()
    refused_pairs = []
    for o in obs:
        req = normalize_signal_map(o.requested)
        if not req:
            continue
        if req != supported_part(req, supported):
            continue
        atoms = signal_atoms(req)
        if len(atoms) == 1 and o.error is None:
            singles.add(atoms[0])
        elif len(atoms) == 2 and o.error is not None:
            refused_pairs.append((atoms, req))
    return dedup([req for atoms, req in refused_pairs
                  if all(atom in singles for atom in atoms)])


def mutually_exclusive_signal_groups(obs: list[SignalObservation],
                                     supported: SignalMap) -> list[dict[str, Any]]:
    """L2-family and L5-family requests that work separately but not
    together with L1."""
    result = []
    for g, sigs in supported.items():
        l1, l2, l5 = set(l1_signals(sigs)), set(l2_signals(sigs)), set(l5_signals(sigs))
        if not l1 or not l2 or not l5:
            continue
        rows = signal_rows_for_gnss(obs, supported, g)
        ok_l2 = any(not err and set(r) & l1 and set(r) & l2 and not set(r) & l5
                    for r, err in rows)
        ok_l5 = any(not err and set(r) & l1 and set(r) & l5 and not set(r) & l2
                    for r, err in rows)
        bad_both = any(err and set(r) & l1 and set(r) & l2 and set(r) & l5
                       for r, err in rows)
        if ok_l2 and ok_l5 and bad_both:
            result.append({"gnss": g, "groups": [sorted(l2), sorted(l5)]})
    return result


def signal_rows_for_gnss(obs: list[SignalObservation], supported: SignalMap,
                         gnss: str) -> list[tuple[list[str], bool]]:
    rows = []
    for o in obs:
        req = observation_request(o, supported)
        if req is None or gnss not in req:
            continue
        if req != supported_part(req, supported):
            continue
        if any(g in req and g != gnss and g in MAJORS for g in req):
            continue
        rows.append((req[gnss], o.error is not None))
    return rows


def signal_atoms(req: SignalMap) -> list[tuple[str, str]]:
    return [(g, s) for g, sigs in normalize_signal_map(req).items() for s in sigs]


def supported_part(req: SignalMap, supported: SignalMap) -> SignalMap:
    sup = normalize_signal_map(supported)
    return normalize_signal_map(
        {g: [s for s in sigs if s in sup.get(g, [])]
         for g, sigs in normalize_signal_map(req).items()})


def refusal_explained(req: SignalMap, patterns: dict[str, Any]) -> bool:
    req = normalize_signal_map(req)
    requires_g = patterns.get("requiresGNSS") or {}
    if isinstance(requires_g, dict):
        for g, anchors in requires_g.items():
            if g in req and not (set(req) & set(anchors)):
                return True
    requires_s = patterns.get("requiresSignalAnyOf") or {}
    if isinstance(requires_s, dict):
        for g, groups in requires_s.items():
            if g in req and any(not set(req[g]) & set(group) for group in groups):
                return True
    mutex = patterns.get("mutuallyExclusiveSignals") or []
    for m in mutex:
        mm = normalize_signal_map(m)
        if all(set(req.get(g, [])) >= set(sigs) for g, sigs in mm.items()):
            return True
    for m in patterns.get("mutuallyExclusiveSignalGroups") or []:
        if not isinstance(m, dict):
            continue
        g = m.get("gnss")
        groups = m.get("groups")
        if isinstance(g, str) and isinstance(groups, list) and g in req \
                and all(set(req[g]) & set(group) for group in groups):
            return True
    return False


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
    observed = [{"request": o.requested, "error": "refused"}
                for o in obs if o.error is not None]
    if observed:
        entry["observed"] = observed
    missing = []
    for o in obs:
        if o.error is not None:
            continue
        requested = [] if o.requested == ["none"] else sorted(o.requested)
        lack = sorted(set(requested) - set(o.emitted))
        if lack:
            missing.append({"request": requested, "missing": lack})
    if missing:
        entry["missing"] = missing
    return entry if entry else None


def characterize_rtcm(obs: list[EmissionObservation], enabled: list[str],
                      supports: list[str]) -> dict[str, Any] | None:
    """Characterize RTCM output: message types implied by the request for
    the enabled constellations that were not emitted. Extra types are
    normal best-effort behavior. A refusal that an absent capability flag
    already predicts (MSM4 without rtcmMSM4, MSM7 without rtcmMSM7) is
    excluded: the declared interface in supports carries that information."""
    entry: dict[str, Any] = {}
    predicted = {f for f, cap in (("MSM4", "rtcmMSM4"), ("MSM7", "rtcmMSM7"))
                 if cap not in supports}
    observed = [{"request": o.requested, "error": "refused"}
                for o in obs if o.error is not None
                and not (set(o.requested) & predicted)]
    if observed:
        entry["observed"] = observed
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
            missing.append({"request": o.requested, "missing": lack})
    if missing:
        entry["missing"] = missing
    return entry if entry else None


def characterize_raw(obs: list[EmissionObservation],
                     supports: list[str]) -> dict[str, Any] | None:
    """Characterize raw output: a requested kind that produced no new
    emission is missing; anything beyond the request is normal. Refusals
    that the absent raw capability flag already predicts are excluded."""
    entry: dict[str, Any] = {}
    observed = [{"request": o.requested, "error": "refused"}
                for o in obs if o.error is not None and "raw" in supports]
    if observed:
        entry["observed"] = observed
    missing = [{"request": o.requested, "missing": o.requested}
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
    only missing requested information is a limitation, stated in the
    device-independent information model: the kinds the receiver never
    delivered (it has no message carrying them), independent of the probe
    cases. Per-request oddities worth prose belong in HW/<receiver>.md."""
    entry: dict[str, Any] = {}
    observed = [{"request": o.requested, "error": "refused"}
                for o in obs if o.error is not None]
    if observed:
        entry["observed"] = observed
    expected = set()
    emitted = set()
    for o in obs:
        if o.error is None:
            expected |= set(o.expect or [])
            emitted |= set(o.emitted)
    missing = sorted(expected - emitted)
    if missing:
        entry["missing"] = missing
    return entry if entry else None


def characterize_save(results: list[dict[str, Any]]) -> dict[str, Any] | None:
    """Characterize save granularity from the per-property experiments.
    Perfect realization is one save group per property, which gets no
    entry. Each property is classified: member of an equivalence class of
    properties that persist together (groups, or singleGroup when one
    class covers everything), anomalous (its observations contradict any
    partition - a property whose save realization spans several of the
    receiver's save sections), or unsaved (its own persistence was never
    observable because its value never moved off NVM)."""
    if not results:
        return None
    entry: dict[str, Any] = {}
    errors = [r for r in results if "error" in r]
    anomalies = [a for r in results for a in r.get("anomalies", [])]
    together: dict[tuple[str, str], bool] = {}
    determined = set()
    indeterminate = set()
    subjects = []
    for r in results:
        if "saved" not in r:
            continue
        subjects.append(r["prop"])
        for q, val in [(q, True) for q in r["saved"]] + \
                      [(q, False) for q in r["independent"]]:
            determined.add(q)
            together[(min(r["prop"], q), max(r["prop"], q))] = \
                together.get((min(r["prop"], q), max(r["prop"], q)), val) and val
        indeterminate |= set(r.get("indeterminate", []))
    unsaved = sorted(indeterminate - determined)
    classified = sorted(set(subjects) - set(unsaved))
    anomalous: list[str] = []
    for k in range(len(classified) + 1):
        done = False
        for combo in combinations(classified, k):
            remaining = set(classified) - set(combo)
            if partition_consistent(remaining, together):
                anomalous = sorted(combo)
                done = True
                break
        if done:
            break
    remaining = set(classified) - set(anomalous)
    groups = merge_groups(remaining, {k: v for k, v in together.items()
                                      if set(k) <= remaining})
    grouped = sorted(sorted(g) for g in groups if len(g) > 1)
    if not anomalous and not unsaved and len(grouped) == 1 \
            and set(grouped[0]) == remaining:
        entry["singleGroup"] = True
    elif grouped:
        entry["groups"] = grouped
    if anomalous:
        entry["anomalous"] = anomalous
    if unsaved:
        entry["unsaved"] = unsaved
    if errors:
        entry["refused"] = [{"prop": r["prop"], "error": r["error"]} for r in errors]
    if anomalies:
        entry["anomalies"] = anomalies
    return entry if entry else None


def partition_consistent(props: set[str],
                         together: dict[tuple[str, str], bool]) -> bool:
    """Whether the persists-together observations restricted to props are
    consistent with some partition: no two properties both joined (perhaps
    transitively) and observed apart."""
    rel = {k: v for k, v in together.items() if set(k) <= props}
    groups = merge_groups(props, rel)
    return not any(a != b and rel.get((min(a, b), max(a, b))) is False
                   for g in groups for a in g for b in g)


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
    """Render a characterization canonically so runs compare byte-for-byte:
    sorted keys, and containers on one line whenever they fit the width."""
    return fmt_json(doc, 0) + "\n"


def fmt_json(v: Any, indent: int) -> str:
    flat = json.dumps(v, sort_keys=True, separators=(", ", ": "))
    if indent + len(flat) <= 100:
        return flat
    pad = " " * (indent + 2)
    if isinstance(v, dict):
        items = [f"{pad}{json.dumps(k)}: {fmt_json(x, indent + 2)}"
                 for k, x in sorted(v.items())]
        return "{\n" + ",\n".join(items) + "\n" + " " * indent + "}"
    if isinstance(v, list):
        items = [f"{pad}{fmt_json(x, indent + 2)}" for x in v]
        return "[\n" + ",\n".join(items) + "\n" + " " * indent + "]"
    return flat
