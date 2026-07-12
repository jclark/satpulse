"""Shared model vocabulary and pure helpers.

Types and functions used both when driving probes against hardware and
when analyzing recorded runs offline: observation records in
device-independent terms, config JSON access, and packet-log
interpretation. Nothing here touches hardware.
"""

import datetime
import json
import math
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

Value = Any
SignalMap = dict[str, list[str]]

GNSS_ORDER = ["GPS", "GAL", "BDS", "GLO", "QZSS", "NAVIC", "SBAS"]

# Frequency band of a signal name, by prefix, ported from the model's
# signalIDBandTable (gps/gpsprot/signal.go). Order matters: longer or more
# specific prefixes come first.
SIGNAL_BAND_PREFIXES = [
    ("B2a", "L5"), ("E5b", "E5b"), ("E5a", "L5"), ("L5", "L5"),
    ("B2", "E5b"), ("L3", "E5b"), ("L1", "L1"), ("E1", "L1"), ("B1", "L1"),
    ("L2", "L2"), ("E6", "E6"), ("L6", "E6"), ("B3", "E6"),
]

# satpulsetool refuses any request whose denoted set enables no
# non-augmentation signal from a major constellation.
MAJORS = {"GPS", "GLO", "GAL", "BDS"}
AUGMENT_SIGNALS = {"GAL": {"E6"}, "BDS": {"B2b"}}

# The NMEA sentence types in the model vocabulary. Types outside it (such
# as event-driven TXT diagnostics) are excluded from comparisons because
# their appearance in a short capture window is not deterministic; they
# remain visible in the packet-log artifacts.
NMEA_VOCAB = ["RMC", "GGA", "GSA", "GSV", "ZDA", "VTG", "GLL"]

# NMEA sentence types that appear exactly once per epoch, so their
# inter-arrival time reflects the output cadence. GSV (and GSA on some
# receivers) emits several sentences per epoch, so its gaps would misreport
# the rate; those types are excluded from rate measurement.
RATE_SAFE_NMEA = ["RMC", "GGA", "ZDA", "VTG", "GLL"]

# The full model signal set of each constellation, as the unqualified signal
# names of the readback JSON, mirroring the SigSet* constants in
# gps/gpsprot/signal.go. A --gnss request denotes a constellation's whole set;
# the backend intersects it with the receiver's supported set. Requesting this
# universe as a signalsEnabled target is the JSON spelling of that request, and
# is how a run discovers the receiver's supported signals.
SIGNAL_UNIVERSE: dict[str, list[str]] = {
    "GPS": ["L1", "L1C", "L2P", "L2C", "L5"],
    "GAL": ["E1", "E5a", "E5b", "E6"],
    "BDS": ["B1I", "B1C", "B2I", "B2b", "B2a", "B3I"],
    "GLO": ["L1", "L1OC", "L2", "L2OC", "L3"],
    "QZSS": ["L1", "L1C", "L1S", "L2C", "L5", "L5S", "L6"],
    "NAVIC": ["L1", "L5"],
    "SBAS": ["L1", "L5"],
}

# The message-output content tokens the probe cases and HW docs use (the CLI
# --pvt-out/--sats-out/--raw-out spellings) mapped to their configtarget.go
# flag names, where the two differ. Wire-format (NMEA, RTCM) tokens are their
# own flag names and need no table.
PVT_MSG_JSON = {"tp": "timePulse", "leap": "leapSecond", "qual": "quality",
                "after": "timePulseAfter"}
SATS_MSG_JSON = {"sig": "signal"}
RAW_MSG_JSON = {"nav": "navData"}


@dataclass
class Observation:
    """One probe outcome in device-independent terms."""

    prop: str
    requested: Value
    error: str | None
    reported: Value
    readback: Value


@dataclass
class SignalObservation:
    """Outcome of requesting an enabled-signal set. requested is in model
    vocabulary; syntax records which command-line spelling was used. achieved
    is the stored set from the readback; accepted carries the set response's
    own set only when it differs from the stored one. gnss preserves the
    command spelling for observations whose model request is intentionally
    unknown."""

    requested: SignalMap | None
    syntax: str
    error: str | None
    achieved: SignalMap | None
    accepted: SignalMap | None = None
    gnss: list[str] | None = None
    tags: list[str] = field(default_factory=list)


@dataclass
class EmissionObservation:
    """Outcome of one message-output request, observed by packet capture.
    expect carries the information kinds the request should deliver, for
    the semantic groups checked at the information level. intervals is the
    observed inter-arrival per single-per-epoch type (model type name for
    the wire-format groups, event type for the semantic ones), for the rate
    check. fix_interval is the fix interval (seconds) the observation ran
    preconditioned to, set only by the preconditioned rate probe; None means
    the observation ran at the receiver's as-found fix rate."""

    group: str
    requested: list[str]
    error: str | None
    emitted: list[str]
    expect: list[str] | None = None
    intervals: dict[str, float] = field(default_factory=dict)
    fix_interval: float | None = None


def transient(err: str | None) -> bool:
    """Whether an error is a communication flake (detection failure or a
    request the receiver never answered) rather than a refusal of the
    requested configuration. Transient errors are retried, and recorded as
    failures rather than receiver limitations when they persist."""
    return err is not None and ("detection failed" in err or "no response" in err
                                or "abandoned after timeout" in err)


def normalize_signal_map(v: Any) -> SignalMap:
    """Return a canonical signal map: known GNSS order, sorted unique
    signal names, and no empty constellations."""
    if not isinstance(v, dict):
        return {}
    out: SignalMap = {}
    keys = [g for g in GNSS_ORDER if g in v] + sorted(
        str(g) for g in v if str(g) not in GNSS_ORDER)
    for g in keys:
        sigs = v.get(g)
        if isinstance(sigs, list):
            ss = sorted({str(s) for s in sigs})
            if ss:
                out[str(g)] = ss
    return out


def normalize_config(v: Any) -> dict[str, Any]:
    """Return a config JSON object in canonical form for comparisons."""
    if not isinstance(v, dict):
        return {}
    out: dict[str, Any] = {}
    for k, val in sorted(v.items()):
        if k == "signalsEnabled":
            sigs = normalize_signal_map(val)
            if sigs:
                out[k] = sigs
        elif isinstance(val, dict):
            out[k] = normalize_config(val)
        elif isinstance(val, list):
            out[k] = [normalize_config(x) if isinstance(x, dict) else x
                      for x in val]
        else:
            out[k] = val
    return out


def config_model_equal(want: Any, got: Any) -> bool:
    """Whether two configs are equal in the device-independent model.

    Some non-timing u-blox receivers default the time pulse grid to UTC.
    satpulsetool reports that as an absent timeGNSS because the model has
    only GNSS grids, and setting the represented time pulse bundle can move
    the receiver to GPS with no way to express a restore to UTC."""
    a, b = normalize_config(want), normalize_config(got)
    if a == b:
        return True
    if "timeGNSS" not in a and b.get("timeGNSS") == "GPS":
        b = dict(b)
        del b["timeGNSS"]
        return a == b
    return False


def signal_map_union(*maps: SignalMap) -> SignalMap:
    """Union signal maps in canonical form."""
    out: dict[str, set[str]] = {}
    for m in maps:
        for g, sigs in normalize_signal_map(m).items():
            out.setdefault(g, set()).update(sigs)
    return normalize_signal_map({g: sorted(sigs) for g, sigs in out.items()})


def signal_map_without(m: SignalMap, remove: SignalMap) -> SignalMap:
    """Return m with remove's signals subtracted."""
    rem = {g: set(sigs) for g, sigs in normalize_signal_map(remove).items()}
    out: SignalMap = {}
    for g, sigs in normalize_signal_map(m).items():
        keep = [s for s in sigs if s not in rem.get(g, set())]
        if keep:
            out[g] = keep
    return out


def signal_map_subset(m: SignalMap, gnss: str) -> SignalMap:
    """Return just one constellation's signals."""
    sigs = normalize_signal_map(m).get(gnss, [])
    return {gnss: sigs} if sigs else {}


def signal_band(name: str) -> str:
    """Return the model band key of a signal name."""
    for prefix, band in SIGNAL_BAND_PREFIXES:
        if name.startswith(prefix):
            return band
    return ""


def signals_in_bands(sigs: list[str], bands: set[str]) -> list[str]:
    """Signals whose band is in bands."""
    return [s for s in sigs if signal_band(s) in bands]


def l1_signals(sigs: list[str]) -> list[str]:
    """Signals in the L1/E1/B1 family."""
    return signals_in_bands(sigs, {"L1"})


def l2_signals(sigs: list[str]) -> list[str]:
    """Signals in the L2/E5b/B2I low-band family."""
    return [s for s in sigs if signal_band(s) in {"L2", "E5b"} and s != "B2b"]


def l5_signals(sigs: list[str]) -> list[str]:
    """Signals in the L5/E5a/B2a/NavIC L5 family."""
    return signals_in_bands(sigs, {"L5"})


def signal_request_valid(req: SignalMap) -> bool:
    """Whether a signal-set request is valid under satpulsetool's own
    semantics: at least one non-augmentation signal from a major
    constellation enabled."""
    return any(set(s) - AUGMENT_SIGNALS.get(c, set())
               for c, s in normalize_signal_map(req).items() if c in MAJORS)


def requested_signals(gnss: list[str], supported: SignalMap) -> SignalMap | None:
    """The signal set a --gnss request denotes, intersected with the
    discovered supported set. None when the supported set does not cover a
    named constellation."""
    sup = normalize_signal_map(supported)
    if any(c not in sup for c in gnss):
        return None
    return normalize_signal_map({c: sup[c] for c in gnss})


def config_value(cfg: dict[str, Any], path: tuple[str, ...]) -> Value:
    """Extract a value from the config JSON object, None if absent."""
    v: Value = cfg
    for k in path:
        if not isinstance(v, dict):
            return None
        v = v.get(k)
    return v


def port_is_usb(cfg: dict[str, Any]) -> bool:
    """Whether a --show-port config describes native USB."""
    port = cfg.get("port")
    return isinstance(port, str) and port.upper() == "USB"


def port_has_serial_speed(cfg: dict[str, Any]) -> bool:
    """Whether a --show-port config describes a baud-rate-bearing port."""
    if port_is_usb(cfg):
        return False
    baud = cfg.get("baudRate")
    return isinstance(baud, int) and baud > 0


def flat_value(obj: Value, key: str) -> Value:
    """Extract a value by flattened key like "fixedPosLLH[0]"."""
    cur = obj
    for part in key.split("."):
        name, _, idx = part.partition("[")
        if not isinstance(cur, dict):
            return None
        cur = cur.get(name)
        if idx:
            i = int(idx.rstrip("]"))
            if not isinstance(cur, list) or i >= len(cur):
                return None
            cur = cur[i]
    return cur


def mode_disagreements(reported: Value, back: Value) -> list[str]:
    """Keys on which a mode set response and the independent readback
    disagree: the value the receiver accepted and the value it stores
    differ there. The difference is data about the receiver, not a
    failure; stored_form recognizes the case where it is only the fixed
    position stored in the other representation."""
    if not isinstance(reported, dict) or not isinstance(back, dict):
        return [] if reported == back else ["mode"]
    return sorted(k for k in reported if back.get(k) != reported[k])


def llh_to_ecef(lat: float, lon: float, height: float) -> tuple[float, float, float]:
    """Convert a WGS84 geodetic position to ECEF coordinates in meters."""
    a, f = 6378137.0, 1 / 298.257223563
    e2 = f * (2 - f)
    rlat, rlon = math.radians(lat), math.radians(lon)
    n = a / math.sqrt(1 - e2 * math.sin(rlat) ** 2)
    return ((n + height) * math.cos(rlat) * math.cos(rlon),
            (n + height) * math.cos(rlat) * math.sin(rlon),
            (n * (1 - e2) + height) * math.sin(rlat))


# Re-expressed positions count as the same point within this many meters:
# covers centimeter storage resolution plus conversion rounding, far below
# any genuinely different position.
POSITION_TOLERANCE = 0.05


def stored_form(reported: Value, back: Value) -> str | None:
    """When a mode set response and its readback disagree only because the
    receiver stored the accepted fixed position in the other representation
    (the same point within POSITION_TOLERANCE), the stored representation
    ("ECEF" or "LLH"); None otherwise."""
    if not isinstance(reported, dict) or not isinstance(back, dict):
        return None
    if "fixedPosLLH" in reported and "fixedPosECEF" in back \
            and "fixedPosECEF" not in reported and "fixedPosLLH" not in back:
        llh, ecef, form = reported, back, "ECEF"
    elif "fixedPosECEF" in reported and "fixedPosLLH" in back \
            and "fixedPosLLH" not in reported and "fixedPosECEF" not in back:
        llh, ecef, form = back, reported, "LLH"
    else:
        return None
    pos_keys = {"fixedPosLLH", "fixedPosECEF", "height"}
    if {k: v for k, v in reported.items() if k not in pos_keys} != \
            {k: v for k, v in back.items() if k not in pos_keys}:
        return None
    lat, lon = llh["fixedPosLLH"]
    xyz = llh_to_ecef(lat, lon, llh.get("height", 0))
    if all(abs(a - b) <= POSITION_TOLERANCE
           for a, b in zip(xyz, ecef["fixedPosECEF"])):
        return form
    return None


# The satpulsetool defaults for a bare --survey request (--survey-time,
# --survey-acc): a surveyed mode's survey parameters are not readable, so it is
# restored with these, matching what mode_args used to spell as bare --survey.
DEFAULT_SURVEY_TIME = 2000
DEFAULT_SURVEY_ACC = 20.0


def target_arg(target: dict[str, Any], *extra: str) -> list[str]:
    """Render a ConfigTarget as satpulsetool --target-json args, plus any
    allowlisted output flags (--show-receiver/-config/-port, --capture). This
    is how every configuration and readback request is expressed: at the
    configtarget.go contract, below the CLI flag layer."""
    return ["--target-json", json.dumps(target, sort_keys=True), *extra]


def pps_props(width: float) -> dict[str, Any]:
    """The timePulse property bundle a --pps request sets: a 1 Hz pulse aligned
    to GNSS time, rising, only when locked, with the given width in seconds (0
    disables). Mirrors ConfigProps.SetPPS in configtarget.go."""
    return {"width": width, "period": 1, "alignToGNSS": True,
            "onlyWhenLocked": True, "polarityRising": True}


def survey_opts(min_dur_s: int, acc_m: float) -> dict[str, Any]:
    """The Opts.Survey a --survey request builds: it always forces a fresh
    survey (SurveyAgain), with the given minimum duration (seconds) and
    accuracy (meters). MinDur is a time.Duration, so nanoseconds on the wire."""
    return {"Survey": {"Flags": ["again"], "MinDur": min_dur_s * 1_000_000_000,
                       "AccLimit": acc_m}}


def mode_target(mode: dict[str, Any]) -> dict[str, Any]:
    """Build the ConfigTarget that reproduces a mode readback, mirroring the
    positioning-mode flags. Survey parameters are not readable, so a surveyed
    mode (static with no stored position) is restored with default survey
    settings; a fixed position is restored with its accuracy."""
    if not mode.get("static"):
        return {"Props": {"mode": {"static": False}}}
    m: dict[str, Any] = {"static": True}
    if "fixedPosECEF" in mode:
        m["fixedPosECEF"] = mode["fixedPosECEF"]
    elif "fixedPosLLH" in mode:
        m["fixedPosLLH"] = mode["fixedPosLLH"]
        m["height"] = mode.get("height", 0)
    else:
        return {"Props": {"mode": m}, "Opts": survey_opts(DEFAULT_SURVEY_TIME,
                                                          DEFAULT_SURVEY_ACC)}
    if "fixedPosAcc" in mode:
        m["fixedPosAcc"] = mode["fixedPosAcc"]
    return {"Props": {"mode": m}}


# Replies to the session's own queries arrive well under this many seconds
# after the query; periodic output continues throughout the capture.
EMISSION_GRACE = datetime.timedelta(seconds=0.5)


def observation_start(log: Path) -> datetime.datetime | None:
    """The start of a packet log's observation window: the session's last
    outbound packet plus the grace period, None when nothing was sent. Output
    before it may still reflect a configuration mid-change (the rate
    estimator's deliberate enable-at-rate-1-then-correct transient, replies
    to the session's own queries), so measurements use only what follows."""
    entries = [json.loads(line) for line in log.read_text().splitlines()]
    last_out = max((parse_t(e["t"]) for e in entries if e.get("out")),
                   default=None)
    return None if last_out is None else last_out + EMISSION_GRACE


def emissions(log: Path) -> dict[tuple[str, str], int]:
    """Counts of inbound (tag, msg) packets in the observation window of a
    packet log: strictly after the session's last outbound packet plus a
    grace period. The capture phase sends nothing, so this window contains
    exactly the receiver's unsolicited periodic output; replies to the
    session's own queries (which need not echo the query's message name,
    e.g. Unicore CONFIG dumps) fall before it."""
    entries = [json.loads(line) for line in log.read_text().splitlines()]
    start = observation_start(log)
    counts: dict[tuple[str, str], int] = {}
    for e in entries:
        tag, msg = e.get("tag"), e.get("msg")
        if e.get("out") or not isinstance(tag, str) or not isinstance(msg, str):
            continue
        if start is not None and parse_t(e["t"]) <= start:
            continue
        k = (tag, msg)
        counts[k] = counts.get(k, 0) + 1
    return counts


def parse_t(s: str) -> datetime.datetime:
    """Parse a packet log timestamp (RFC 3339)."""
    return datetime.datetime.fromisoformat(s)


def median_interval(times: list[datetime.datetime]) -> float | None:
    """The median inter-arrival time in seconds of a set of timestamps, or
    None with fewer than two (a single arrival has no interval). Median, not
    mean, so one delayed packet does not skew the estimate."""
    if len(times) < 2:
        return None
    ts = sorted(times)
    gaps = sorted((b - a).total_seconds() for a, b in zip(ts, ts[1:]))
    n = len(gaps)
    return gaps[n // 2] if n % 2 else (gaps[n // 2 - 1] + gaps[n // 2]) / 2


def emission_intervals(log: Path) -> dict[tuple[str, str], float]:
    """Per (tag, msg) key, the median inter-arrival time in seconds over the
    same observation window emissions() uses (strictly after the last
    outbound packet plus EMISSION_GRACE). Absent for keys with fewer than
    two arrivals. Only single-per-epoch message types give a meaningful
    cadence (see RATE_SAFE_NMEA and the MSM numbers); a type emitted several
    times per epoch would show intra-epoch gaps, so callers select the safe
    types before drawing a rate verdict."""
    entries = [json.loads(line) for line in log.read_text().splitlines()]
    start = observation_start(log)
    times: dict[tuple[str, str], list[datetime.datetime]] = {}
    for e in entries:
        tag, msg = e.get("tag"), e.get("msg")
        if e.get("out") or not isinstance(tag, str) or not isinstance(msg, str):
            continue
        t = parse_t(e["t"])
        if start is not None and t <= start:
            continue
        times.setdefault((tag, msg), []).append(t)
    out: dict[tuple[str, str], float] = {}
    for k, ts in times.items():
        iv = median_interval(ts)
        if iv is not None:
            out[k] = iv
    return out


def nmea_rate_intervals(iv: dict[tuple[str, str], float]) -> dict[str, float]:
    """Observed inter-arrival per single-per-epoch NMEA sentence type, keyed
    by the model type name. On the rare receiver that emits one type under
    two talker IDs, the larger interval is kept: each talker's own cadence is
    per-epoch, and the conservative choice avoids a spurious fast reading."""
    out: dict[str, float] = {}
    for (tag, msg), t in iv.items():
        if tag == "NMEA" and len(msg) == 5 and msg[2:] in RATE_SAFE_NMEA:
            typ = msg[2:]
            out[typ] = max(out.get(typ, t), t)
    return out


def rtcm_rate_intervals(iv: dict[tuple[str, str], float]) -> dict[str, float]:
    """Observed inter-arrival per RTCM message number (each number is emitted
    once per epoch, so all are safe)."""
    return {msg: t for (tag, msg), t in iv.items() if tag == "RTCM"}


def event_intervals(events: list[dict[str, Any]], etype: str,
                    start: datetime.datetime | None) -> dict[str, float]:
    """The median inter-arrival of one replayed event type, keyed by the type
    name; empty when fewer than two events carry it. Used to measure the
    delivery cadence of the semantic groups (navEpoch for PVT, satellites for
    satellite information) at the information level. Restricted to events
    after start (the packet log's observation window): earlier events can
    reflect a configuration mid-change, e.g. the rate estimator's deliberate
    enable-at-rate-1-then-correct transient, which would skew the median."""
    times = [parse_t(e["t"]) for e in events
             if e.get("type") == etype and isinstance(e.get("t"), str)]
    if start is not None:
        times = [t for t in times if t > start]
    iv = median_interval(times)
    return {etype: iv} if iv is not None else {}


def nmea_set(d: dict[tuple[str, str], int]) -> list[str]:
    """NMEA sentence types in an emission map."""
    return sorted({msg[2:] for tag, msg in d if tag == "NMEA" and len(msg) == 5})


def rtcm_set(d: dict[tuple[str, str], int]) -> list[str]:
    """RTCM message types in an emission map."""
    return sorted({msg for tag, msg in d if tag == "RTCM"})


def raw_set(d: dict[tuple[str, str], int]) -> set[str]:
    """Non-NMEA, non-RTCM periodic messages in an emission map, as tag:msg."""
    return {f"{tag}:{msg}" for tag, msg in d if tag not in ("NMEA", "RTCM")}


def has_fix(events: list[dict[str, Any]]) -> bool:
    """Whether a replayed event stream shows a position fix."""
    return any(e.get("type") == "navEpoch" and (e.get("data") or {}).get("fixLevel")
               in ("code", "carrierFloat", "carrierFixed") for e in events)


def event_kinds(events: list[dict[str, Any]]) -> set[str]:
    """The information kinds present in a replayed event stream."""
    kinds = set()
    for e in events:
        t = e.get("type")
        d = e.get("data") or {}
        if t in ("posGeo", "posECEF"):
            kinds.add("pos")
            if t == "posECEF":
                kinds.add("ecef")
        elif t in ("velGeo", "velECEF"):
            kinds.add("vel")
        elif t == "time":
            ref = d.get("ref", 0)
            kinds.add("tp" if ref else "time")
            if ref == 2:
                kinds.add("tpPost")
            if "taiTime" in d:
                kinds.add("tai")
        elif t == "leapSecond":
            kinds.add("leap")
        elif t == "satellites":
            kinds.add("satellites")
            info = d.get("info") or []
            # Per-signal information means signal-level records are
            # present; on a single-band receiver that is one per satellite,
            # so counting records per satellite would be wrong.
            if any(sv.get("signals") for sv in info):
                kinds.add("perSignal")
        elif t == "navEpoch":
            kinds.add("navEpoch")
    if "tpPost" in kinds or "time" in kinds:
        kinds.add("after")
    return kinds


def pvt_event_kinds(log: Path, events: list[dict[str, Any]]) -> set[str]:
    """The PVT information kinds present in a packet log.

    Most kinds are semantic gpsprot events. For UBX leap-second output,
    the wire contract is UBX-NAV-TIMELS; older receivers can emit a
    TIMELS payload that is not convertible to the stronger LeapSecondMsg
    abstraction, but still satisfies --pvt-out leap."""
    kinds = event_kinds(events)
    if packet_present(log, "UBX", "NAV-TIMELS"):
        kinds.add("leap")
    return kinds


def packet_present(log: Path, tag: str, msg: str) -> bool:
    """Whether an inbound packet with tag/msg is present in a packet log."""
    for line in log.read_text().splitlines():
        try:
            e = json.loads(line)
        except ValueError:
            continue
        if not e.get("out") and e.get("tag") == tag and e.get("msg") == msg:
            return True
    return False
