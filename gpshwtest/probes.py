"""Probes for scalar configuration properties.

Each probe sets a property, reads the achieved value from the set
invocation, and confirms it with an independent readback. Disagreement
between the two, or state changed by a reported error, is a failure
(a tool-guarantee violation); everything else - refusals, quantization,
clipping - is recorded as an observation for the characterization.
"""

import json
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable

from tool import Tool

Value = Any

# Settle time after a successful signal-set change: u-blox documents an
# internal GNSS-subsystem restart (wait for the ACK plus 0.5 s); 2 s has
# been reliable on real hardware.
SIGNAL_SETTLE = 2.0


@dataclass
class Observation:
    """One probe outcome in device-independent terms."""

    prop: str
    requested: Value
    error: str | None
    reported: Value
    readback: Value


def fmt_value(v: Value) -> str:
    """Format a model value as a satpulsetool command-line argument."""
    if isinstance(v, float):
        s = f"{v:.9f}".rstrip("0").rstrip(".")
        return s if s else "0"
    return str(v)


def fmt_nanos(v: Value) -> str:
    """Render a model value in seconds as integer nanoseconds for the CLI."""
    return str(round(float(v) * 1e9))


@dataclass
class ScalarProp:
    """A property settable by one flag and readable at one config JSON path.

    Probe values are in model units (the units of the config JSON); to_cli
    renders a model value as the flag's argument where the flag uses
    different units.
    """

    name: str
    flag: str
    values: list[Value]
    path: tuple[str, ...]
    to_cli: Callable[[Value], str] = fmt_value


PROPS = [
    ScalarProp("antennaCableDelay", "--ant-cable-delay", [1e-9, 1.23e-7, 3.2767e-5],
               ("antennaCableDelay",), to_cli=fmt_nanos),
    ScalarProp("minElevation", "--min-elev", [1, 7, 45], ("minElevation",)),
    ScalarProp("rtcmBaseID", "--rtcm-base-id", [1, 1234, 4095], ("rtcmBaseID",)),
    ScalarProp("timeGNSS", "--time-gnss", ["GAL", "BDS", "GLO", "GPS"], ("timeGNSS",)),
    ScalarProp("timePulse.width", "--pps", [0.25, 0.000123456, 0.1], ("timePulse", "width")),
]


@dataclass
class ModeCase:
    """One positioning-mode request: flags plus the mode fields it implies.

    Request keys use the mode JSON vocabulary, flattened (survey.* fields
    exist in the model but not in the readable mode object). Values have
    more decimals than any receiver resolution so quantization shows.
    The fixed positions are an arbitrary plausible point; receivers store
    the position without checking it against the actual location.
    """

    name: str
    args: list[str]
    request: dict[str, Value]


# The NMEA sentence types in the model vocabulary. Types outside it (such
# as event-driven TXT diagnostics) are excluded from comparisons because
# their appearance in a short capture window is not deterministic; they
# remain visible in the packet-log artifacts.
NMEA_VOCAB = ["RMC", "GGA", "GSA", "GSV", "ZDA", "VTG", "GLL"]

NMEA_CASES = [["RMC"], ["GGA", "ZDA"], ["none"]]

RTCM_CASES = [["MSM4", "ARP"], ["MSM7"], ["none"]]

# Seconds of packet capture used to observe what the receiver emits;
# the message kinds under test are all per-epoch (1 Hz), so this spans
# several epochs.
OBSERVE_SECONDS = 4

# Settle time after changing message output before observing, so the
# observation window does not straddle the change.
MSG_SETTLE = 1.0


@dataclass
class EmissionObservation:
    """Outcome of one message-output request, observed by packet capture."""

    group: str
    requested: list[str]
    error: str | None
    emitted: list[str]


def emissions(log: Path) -> dict[tuple[str, str], int]:
    """Counts of inbound (tag, msg) packets in a packet log. Replies to
    polls are excluded by dropping inbound messages whose name was also
    sent outbound."""
    counts: dict[tuple[str, str], int] = {}
    sent = set()
    for line in log.read_text().splitlines():
        e = json.loads(line)
        tag, msg = e.get("tag"), e.get("msg")
        if not isinstance(tag, str) or not isinstance(msg, str):
            continue
        if e.get("out"):
            sent.add(msg)
        else:
            k = (tag, msg)
            counts[k] = counts.get(k, 0) + 1
    return {k: n for k, n in counts.items() if k[1] not in sent}


def nmea_set(d: dict[tuple[str, str], int]) -> list[str]:
    """NMEA sentence types in an emission map."""
    return sorted({msg[2:] for tag, msg in d if tag == "NMEA" and len(msg) == 5})


def rtcm_set(d: dict[tuple[str, str], int]) -> list[str]:
    """RTCM message types in an emission map."""
    return sorted({msg for tag, msg in d if tag == "RTCM"})


def raw_set(d: dict[tuple[str, str], int]) -> set[str]:
    """Non-NMEA, non-RTCM periodic messages in an emission map, as tag:msg."""
    return {f"{tag}:{msg}" for tag, msg in d if tag not in ("NMEA", "RTCM")}


@dataclass
class SignalObservation:
    """Outcome of requesting one constellation/band combination."""

    gnss: list[str]
    band: list[str] | None
    error: str | None
    achieved: dict[str, list[str]] | None


BANDS_ALL = [["L1"], ["L2"], ["L5"], ["E5"], ["L6"], ["L1", "L2"]]
BANDS_SINGLE = [["L1"], ["L2"], ["E5b"]]


def signal_cases(supported: list[str]) -> list[tuple[list[str], list[str] | None]]:
    """Build the signal probe set from the discovered constellation list:
    each constellation alone and with band subsets, augmentation systems
    paired with GPS (they are commonly coupled to it), all constellations
    together, and band subsets of all."""
    cases: list[tuple[list[str], list[str] | None]] = [([g], None) for g in supported]
    cases += [([g], b) for g in supported for b in BANDS_SINGLE]
    if "GPS" in supported:
        cases += [(["GPS", g], None) for g in ("QZSS", "SBAS") if g in supported]
    cases.append((list(supported), None))
    cases += [(list(supported), b) for b in BANDS_ALL]
    return cases


MODE_CASES = [
    ModeCase("survey", ["--survey", "--survey-time", "300", "--survey-acc", "2.345"],
             {"static": True, "survey.minDuration": 300, "survey.accLimit": 2.345}),
    ModeCase("fixed-llh",
             ["--fixed-pos-llh", "13.7318284567,100.6447407891,12.34567",
              "--fixed-pos-acc", "0.12345"],
             {"static": True, "fixedPosLLH[0]": 13.7318284567,
              "fixedPosLLH[1]": 100.6447407891, "height": 12.34567,
              "fixedPosAcc": 0.12345}),
    ModeCase("fixed-ecef",
             ["--fixed-pos-ecef", "-1132881.12345,6092270.56789,1504542.90123",
              "--fixed-pos-acc", "1.23456"],
             {"static": True, "fixedPosECEF[0]": -1132881.12345,
              "fixedPosECEF[1]": 6092270.56789, "fixedPosECEF[2]": 1504542.90123,
              "fixedPosAcc": 1.23456}),
    ModeCase("mobile", ["--mobile"], {"static": False}),
]


def config_value(cfg: dict[str, Any], path: tuple[str, ...]) -> Value:
    """Extract a value from the config JSON object, None if absent."""
    v: Value = cfg
    for k in path:
        if not isinstance(v, dict):
            return None
        v = v.get(k)
    return v


def flat_value(obj: Value, key: str) -> Value:
    """Extract a value by flattened key like "fixedPosLLH[0]" or "survey.minDuration"."""
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


@dataclass
class ProbeRun:
    """Drives probes against one receiver, collecting observations and failures."""

    tool: Tool
    observations: list[Observation] = field(default_factory=list)
    signal_observations: list[SignalObservation] = field(default_factory=list)
    emission_observations: list[EmissionObservation] = field(default_factory=list)
    failures: list[str] = field(default_factory=list)

    def show_config(self, name: str) -> dict[str, Any] | None:
        """Read the full current configuration in a separate invocation.
        Returns None when the invocation itself fails (recorded as a
        failure); callers must then skip comparisons rather than cascade."""
        inv = self.tool.gps(name, ["--show-config"])
        if inv.error is not None:
            self.failures.append(f"{name}: --show-config failed: {inv.error}")
            return None
        return inv.config()

    def probe_scalar(self, p: ScalarProp, initial: dict[str, Any]) -> None:
        """Probe each value of p, then restore its initial value."""
        prev = config_value(initial, p.path)
        for v in p.values:
            inv = self.tool.gps(f"set-{p.name}", [p.flag, p.to_cli(v)])
            reported = config_value(inv.config(), p.path)
            cfg = self.show_config(f"readback-{p.name}")
            if cfg is None:
                continue
            back = config_value(cfg, p.path)
            self.observations.append(Observation(p.name, v, inv.error, reported, back))
            if inv.error is not None:
                if back != prev:
                    self.failures.append(
                        f"{p.name}: refusal of {v!r} changed state: {prev!r} -> {back!r}")
            else:
                if reported != back:
                    self.failures.append(
                        f"{p.name}: reported {reported!r} but readback says {back!r}")
                prev = back
        self.restore(p, initial)

    def observe_emissions(self, name: str) -> dict[tuple[str, str], int] | None:
        """Capture for a few seconds and return what the receiver emits.
        Message output configuration is not readable back, so observation
        is both the verification and the restore baseline."""
        inv = self.tool.gps(name, ["--show-receiver", "--capture", str(OBSERVE_SECONDS)])
        if inv.error is not None:
            self.failures.append(f"{name}: capture failed: {inv.error}")
            return None
        return emissions(inv.packet_log)

    def set_and_observe(self, group: str, flag: str, case: list[str]) -> \
            dict[tuple[str, str], int] | None:
        """Apply one message-output case and observe the result; None when
        the request was refused (recorded as an observation) or the
        observation capture failed (recorded as a failure)."""
        name = "-".join(case)
        inv = self.tool.gps(f"set-{group}-{name}", [flag, ",".join(case)])
        if inv.error is not None:
            self.emission_observations.append(
                EmissionObservation(group, case, inv.error, []))
            return None
        time.sleep(MSG_SETTLE)
        return self.observe_emissions(f"observe-{group}-{name}")

    def probe_messages(self) -> None:
        """Probe NMEA, RTCM, and raw message output from one shared baseline
        observation, restoring each group afterwards."""
        base = self.observe_emissions("messages-initial")
        if base is None:
            return
        self.probe_nmea(nmea_set(base))
        self.probe_rtcm(rtcm_set(base))
        self.probe_raw(raw_set(base))

    def probe_nmea(self, initial: list[str]) -> None:
        """Probe NMEA output selection, then restore the initial sentence set."""
        for case in NMEA_CASES:
            d = self.set_and_observe("nmeaOut", "--nmea-out", case)
            if d is not None:
                self.emission_observations.append(
                    EmissionObservation("nmeaOut", case, None, nmea_set(d)))
        want = [t for t in initial if t in NMEA_VOCAB]
        inv = self.tool.gps("restore-nmea", ["--nmea-out", ",".join(want) if want else "none"])
        if inv.error is not None:
            self.failures.append(f"nmea: restore to {want!r} failed: {inv.error}")
            return
        d = self.observe_emissions("verify-restore-nmea")
        if d is not None:
            back = [t for t in nmea_set(d) if t in NMEA_VOCAB]
            if back != sorted(want):
                self.failures.append(f"nmea: restore to {want!r} read back as {back!r}")

    def probe_rtcm(self, initial: list[str]) -> None:
        """Probe RTCM output selection, then restore the initial emission."""
        for case in RTCM_CASES:
            d = self.set_and_observe("rtcmOut", "--rtcm-out", case)
            if d is not None:
                self.emission_observations.append(
                    EmissionObservation("rtcmOut", case, None, rtcm_set(d)))
        want = []
        if any(t.endswith("4") and t.startswith("1") for t in initial):
            want.append("MSM4")
        if any(t.endswith("7") and t.startswith("1") for t in initial):
            want.append("MSM7")
        if "1005" in initial:
            want.append("ARP")
        inv = self.tool.gps("restore-rtcm", ["--rtcm-out", ",".join(want) if want else "none"])
        if inv.error is not None:
            self.failures.append(f"rtcm: restore to {want!r} failed: {inv.error}")
            return
        d = self.observe_emissions("verify-restore-rtcm")
        if d is not None and rtcm_set(d) != initial:
            self.failures.append(
                f"rtcm: restore to {initial!r} reads back as {rtcm_set(d)!r}")

    def probe_raw(self, initial: set[str]) -> None:
        """Probe raw output kinds, then restore the initial emission. The
        messages realizing each kind are discovered from the probe itself
        (whatever appears beyond the baseline), so restoring needs no
        receiver-specific knowledge."""
        found: dict[str, set[str]] = {}
        for kind in ("obs", "nav"):
            d = self.set_and_observe("rawOut", "--raw-out", [kind])
            if d is None:
                continue
            found[kind] = raw_set(d) - initial
            self.emission_observations.append(
                EmissionObservation("rawOut", [kind], None, sorted(found[kind])))
        d = self.set_and_observe("rawOut", "--raw-out", ["none"])
        if d is not None:
            self.emission_observations.append(
                EmissionObservation("rawOut", ["none"], None, sorted(raw_set(d) - initial)))
        want = [k for k, msgs in found.items() if msgs and msgs <= initial]
        if not want:
            return
        inv = self.tool.gps("restore-raw", ["--raw-out", ",".join(want)])
        if inv.error is not None:
            self.failures.append(f"raw: restore to {want!r} failed: {inv.error}")
            return
        d = self.observe_emissions("verify-restore-raw")
        if d is not None and not set().union(*(found[k] for k in want)) <= raw_set(d):
            self.failures.append(f"raw: restore to {want!r} not emitting as before")

    def probe_signals(self, initial: dict[str, Any], supported: list[str]) -> None:
        """Probe constellation/band combinations, then restore the initial set."""
        prev = config_value(initial, ("signalsEnabled",))
        for gnss, band in signal_cases(supported):
            name = "-".join(gnss) + ("-" + "-".join(band) if band else "")
            args = ["--gnss", ",".join(gnss)]
            if band:
                args += ["--band", ",".join(band)]
            inv = self.tool.gps(f"set-signals-{name}", args)
            reported = config_value(inv.config(), ("signalsEnabled",))
            if inv.error is None:
                time.sleep(SIGNAL_SETTLE)
            cfg = self.show_config(f"readback-signals-{name}")
            if cfg is None:
                continue
            back = config_value(cfg, ("signalsEnabled",))
            if inv.error is not None:
                self.signal_observations.append(SignalObservation(gnss, band, inv.error, None))
                if back != prev:
                    self.failures.append(
                        f"signals {name}: refusal changed state: {prev!r} -> {back!r}")
                continue
            if reported != back:
                self.failures.append(
                    f"signals {name}: reported {reported!r} but readback says {back!r}")
            self.signal_observations.append(SignalObservation(gnss, band, None, back))
            prev = back
        self.restore_signals(initial)

    def restore_signals(self, initial: dict[str, Any]) -> None:
        """Re-enable the initial constellation set. Band subsetting within a
        constellation cannot be reproduced generically, so a band-limited
        initial set shows up as a restore failure rather than silently passing."""
        want = config_value(initial, ("signalsEnabled",))
        if not isinstance(want, dict):
            return
        inv = self.tool.gps("restore-signals", ["--gnss", ",".join(want)])
        if inv.error is not None:
            self.failures.append(f"signals: restore to {want!r} failed: {inv.error}")
            return
        time.sleep(SIGNAL_SETTLE)
        cfg = self.show_config("verify-restore-signals")
        if cfg is not None and config_value(cfg, ("signalsEnabled",)) != want:
            self.failures.append(
                f"signals: restore to {want!r} read back as "
                f"{config_value(cfg, ('signalsEnabled',))!r}")

    def probe_modes(self, initial: dict[str, Any]) -> None:
        """Probe each positioning-mode case, then restore the initial mode."""
        prev = config_value(initial, ("mode",))
        for case in MODE_CASES:
            inv = self.tool.gps(f"set-mode-{case.name}", case.args)
            reported = config_value(inv.config(), ("mode",))
            cfg = self.show_config(f"readback-mode-{case.name}")
            if cfg is None:
                continue
            back = config_value(cfg, ("mode",))
            if inv.error is not None:
                for k, v in case.request.items():
                    self.observations.append(Observation(f"mode.{k}", v, inv.error, None, None))
                if back != prev:
                    self.failures.append(
                        f"mode {case.name}: refusal changed state: {prev!r} -> {back!r}")
                continue
            if reported != back:
                self.failures.append(
                    f"mode {case.name}: reported {reported!r} but readback says {back!r}")
            for k, v in case.request.items():
                self.observations.append(Observation(
                    f"mode.{k}", v, None, flat_value(reported, k), flat_value(back, k)))
            prev = back
        self.restore_mode(initial)

    def restore_mode(self, initial: dict[str, Any]) -> None:
        """Set the positioning mode back to its initial readback."""
        mode = config_value(initial, ("mode",))
        if not isinstance(mode, dict):
            return
        inv = self.tool.gps("restore-mode", mode_args(mode))
        if inv.error is not None:
            self.failures.append(f"mode: restore to {mode!r} failed: {inv.error}")
            return
        cfg = self.show_config("verify-restore-mode")
        if cfg is not None and config_value(cfg, ("mode",)) != mode:
            self.failures.append(
                f"mode: restore to {mode!r} read back as {config_value(cfg, ('mode',))!r}")

    def restore(self, p: ScalarProp, initial: dict[str, Any]) -> None:
        """Set p back to its value in the initial configuration."""
        v = config_value(initial, p.path)
        if v is None:
            return
        inv = self.tool.gps(f"restore-{p.name}", [p.flag, p.to_cli(v)])
        if inv.error is not None:
            self.failures.append(f"{p.name}: restore to {v!r} failed: {inv.error}")
            return
        cfg = self.show_config(f"verify-restore-{p.name}")
        if cfg is not None and config_value(cfg, p.path) != v:
            self.failures.append(
                f"{p.name}: restore to {v!r} read back as {config_value(cfg, p.path)!r}")
