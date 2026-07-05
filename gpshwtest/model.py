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
    the semantic groups checked at the information level."""

    group: str
    requested: list[str]
    error: str | None
    emitted: list[str]
    expect: list[str] | None = None


def transient(err: str | None) -> bool:
    """Whether an error is a communication flake (detection failure or a
    request the receiver never answered) rather than a refusal of the
    requested configuration. Transient errors are retried, and recorded as
    failures rather than receiver limitations when they persist."""
    return err is not None and ("detection failed" in err or "no response" in err
                                or "abandoned after timeout" in err)


def fmt_value(v: Value) -> str:
    """Format a model value as a satpulsetool command-line argument."""
    if isinstance(v, float):
        s = f"{v:.9f}".rstrip("0").rstrip(".")
        return s if s else "0"
    return str(v)


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


def signal_map_cli_arg(m: SignalMap) -> str:
    """Render a signal map as a --signal/--except-signal argument."""
    parts = []
    for g, sigs in normalize_signal_map(m).items():
        parts += [g + s for s in sigs]
    return ",".join(parts)


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


def mode_args(mode: dict[str, Any]) -> list[str]:
    """Build the flags that reproduce a mode readback. Survey parameters are
    not readable, so a surveyed mode is restored with default survey settings."""
    if not mode.get("static"):
        return ["--mobile"]
    args: list[str]
    if "fixedPosECEF" in mode:
        x, y, z = mode["fixedPosECEF"]
        args = ["--fixed-pos-ecef", f"{x},{y},{z}"]
    elif "fixedPosLLH" in mode:
        lat, lon = mode["fixedPosLLH"]
        args = ["--fixed-pos-llh", f"{lat},{lon},{mode.get('height', 0)}"]
    else:
        args = ["--survey"]
    if "fixedPosAcc" in mode:
        args += ["--fixed-pos-acc", fmt_value(mode["fixedPosAcc"])]
    return args


# Replies to the session's own queries arrive well under this many seconds
# after the query; periodic output continues throughout the capture.
EMISSION_GRACE = datetime.timedelta(seconds=0.5)


def emissions(log: Path) -> dict[tuple[str, str], int]:
    """Counts of inbound (tag, msg) packets in the observation window of a
    packet log: strictly after the session's last outbound packet plus a
    grace period. The capture phase sends nothing, so this window contains
    exactly the receiver's unsolicited periodic output; replies to the
    session's own queries (which need not echo the query's message name,
    e.g. Unicore CONFIG dumps) fall before it."""
    entries = [json.loads(line) for line in log.read_text().splitlines()]
    last_out = max((parse_t(e["t"]) for e in entries if e.get("out")),
                   default=None)
    counts: dict[tuple[str, str], int] = {}
    for e in entries:
        tag, msg = e.get("tag"), e.get("msg")
        if e.get("out") or not isinstance(tag, str) or not isinstance(msg, str):
            continue
        if last_out is not None and parse_t(e["t"]) <= last_out + EMISSION_GRACE:
            continue
        k = (tag, msg)
        counts[k] = counts.get(k, 0) + 1
    return counts


def parse_t(s: str) -> datetime.datetime:
    """Parse a packet log timestamp (RFC 3339)."""
    return datetime.datetime.fromisoformat(s)


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
