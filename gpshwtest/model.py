"""Shared model vocabulary and pure helpers.

Types and functions used both when driving probes against hardware and
when analyzing recorded runs offline: observation records in
device-independent terms, config JSON access, and packet-log
interpretation. Nothing here touches hardware.
"""

import datetime
import json
from dataclasses import dataclass
from pathlib import Path
from typing import Any

Value = Any

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
    """Outcome of requesting one constellation/band combination."""

    gnss: list[str]
    band: list[str] | None
    error: str | None
    achieved: dict[str, list[str]] | None


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


def config_value(cfg: dict[str, Any], path: tuple[str, ...]) -> Value:
    """Extract a value from the config JSON object, None if absent."""
    v: Value = cfg
    for k in path:
        if not isinstance(v, dict):
            return None
        v = v.get(k)
    return v


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
    disagree. Every reported field must read back identically: mode is a
    property, so an achieved value the readback cannot confirm (such as a
    fixed position echoed as LLH but stored and read back as ECEF) is a
    guarantee violation, not a representation detail."""
    if not isinstance(reported, dict) or not isinstance(back, dict):
        return [] if reported == back else ["mode"]
    return sorted(k for k in reported if back.get(k) != reported[k])


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
