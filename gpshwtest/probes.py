"""Probes for scalar configuration properties.

Each probe sets a property, reads the achieved value from the set
invocation, and confirms it with an independent readback. Disagreement
between the two, or state changed by a reported error, is a failure
(a tool-guarantee violation); everything else - refusals, quantization,
clipping - is recorded as an observation for the characterization.
"""

import json
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable

from tool import Invocation, Tool, transient

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
    ScalarProp("antennaCableDelay", "--ant-cable-delay", [1, 123, 32767],
               ("antennaCableDelay",)),
    ScalarProp("minElevation", "--min-elev", [1, 7, 45], ("minElevation",)),
    ScalarProp("rtcmBaseID", "--rtcm-base-id", [1, 1234, 4095], ("rtcmBaseID",)),
    ScalarProp("timeGNSS", "--time-gnss", ["GAL", "BDS", "GLO", "GPS"], ("timeGNSS",)),
    ScalarProp("timePulse.width", "--pps", [0.25, 0.000123456, 0.1], ("timePulse", "width")),
]


@dataclass
class ModeCase:
    """One positioning-mode request: flags plus the mode fields it implies.

    Request keys use the mode JSON vocabulary, flattened. Only properties
    are listed: survey duration and accuracy are parameters of the
    survey-in operation (ConfigOptions in gpsprot/configtarget.go), not
    configuration properties, so readback never applies to them. Values
    have more decimals than any receiver resolution so quantization shows.
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


@dataclass
class PVTCase:
    """One PVT output request and the information kinds it should deliver.

    Kinds are derived from the replayed event stream: pos/vel from the
    position and velocity events, time from solution-time events, tp from
    pulse-time events (TimeRef PrePulse/PostPulse), leap from leapSecond,
    navEpoch from epoch markers (covers qual and epoch), tai/ecef when the
    corresponding content selector is honored. The survey flag is excluded:
    survey events only flow during a survey-in, so a mobile-mode probe
    cannot verify them.
    """

    flags: list[str]
    expect: set[str]


# ptp and ntp are exact abbreviations of flag sets (see the man page), so
# they are not probed; the underlying flags are. survey is excluded per the
# docstring above; qual and epoch information both appear as navEpoch events.
# after requires time information that follows the pulse: a pre-pulse
# message (UBX-TIM-TP) is not enough by itself, while a post-pulse message
# (UBX-TIM-TOS) is; solution-time messages also satisfy it. event_kinds
# derives the "after" kind accordingly.
PVT_CASES = [
    PVTCase(["pos", "vel", "time", "off"], {"pos", "vel", "time"}),
    PVTCase(["pos", "vel", "time", "ecef", "tai", "off"],
            {"pos", "vel", "time", "ecef", "tai"}),
    PVTCase(["tp", "after", "tai", "leap", "qual", "epoch", "off"],
            {"tp", "after", "tai", "leap", "navEpoch"}),
    PVTCase(["off"], set()),
]

SATS_CASES = [(["sat"], {"satellites"}),
              (["sig"], {"satellites", "perSignal"}),
              (["none"], set())]


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
            if any(len(sv.get("signals") or []) >= 2 for sv in info):
                kinds.add("perSignal")
        elif t == "navEpoch":
            kinds.add("navEpoch")
    if "tpPost" in kinds or "time" in kinds:
        kinds.add("after")
    return kinds

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
             {"static": True}),
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
        start = prev
        accepted: list[Observation] = []
        for v in p.values:
            inv = self.tool.gps(f"set-{p.name}", [p.flag, p.to_cli(v)])
            reported = config_value(inv.config(), p.path)
            if transient(inv.error):
                self.failures.append(f"set-{p.name}: {inv.error}")
                continue
            cfg = self.show_config(f"readback-{p.name}")
            if cfg is None:
                continue
            back = config_value(cfg, p.path)
            obs = Observation(p.name, v, inv.error, reported, back)
            self.observations.append(obs)
            if inv.error is not None:
                if back != prev:
                    self.failures.append(
                        f"{p.name}: refusal of {v!r} changed state: {prev!r} -> {back!r}")
            else:
                if reported != back:
                    self.failures.append(
                        f"{p.name}: reported {reported!r} but readback says {back!r}")
                accepted.append(obs)
                prev = back
        self.check_value_moves(p, start, accepted)
        self.restore(p, initial)

    def check_value_moves(self, p: ScalarProp, start: Value,
                          accepted: list[Observation]) -> None:
        """An accepted set that leaves a property's value unchanged can be
        legitimate only as range clamping. When the requests bracket the
        prior value and it still never moved, no range limit explains it:
        the set was silently ignored, which is a bug."""
        if not isinstance(start, (int, float)) or not accepted:
            return
        vals = [o.requested for o in accepted if isinstance(o.requested, (int, float))]
        if (all(o.readback == start for o in accepted)
                and any(v < start for v in vals) and any(v > start for v in vals)):
            self.failures.append(
                f"{p.name}: accepted sets never changed the value from {start!r}")

    def observe_capture(self, name: str) -> Invocation | None:
        """Capture for a few seconds; the packet log is what the receiver
        emits. Message output configuration is not readable back, so
        observation is both the verification and the restore baseline."""
        inv = self.tool.gps(name, ["--show-receiver", "--capture", str(OBSERVE_SECONDS)])
        if inv.error is not None:
            self.failures.append(f"{name}: capture failed: {inv.error}")
            return None
        return inv

    def observe_emissions(self, name: str) -> dict[tuple[str, str], int] | None:
        inv = self.observe_capture(name)
        return emissions(inv.packet_log) if inv is not None else None

    def set_and_observe(self, group: str, flag: str, case: list[str],
                        pre: list[str] | None = None) -> Invocation | None:
        """Apply one message-output case and observe the result; None when
        the request was refused (recorded as an observation) or the
        observation capture failed (recorded as a failure)."""
        name = "-".join(case)
        inv = self.tool.gps(f"set-{group}-{name}", (pre or []) + [flag, ",".join(case)])
        if inv.error is not None:
            if transient(inv.error):
                self.failures.append(f"set-{group}-{name}: {inv.error}")
            else:
                self.emission_observations.append(
                    EmissionObservation(group, case, inv.error, []))
            return None
        time.sleep(MSG_SETTLE)
        return self.observe_capture(f"observe-{group}-{name}")

    def probe_messages(self) -> None:
        """Probe NMEA, RTCM, and raw message output from one shared baseline
        observation, restoring each group afterwards."""
        base = self.observe_emissions("messages-initial")
        if base is None:
            return
        self.probe_nmea(nmea_set(base))
        self.probe_rtcm(rtcm_set(base))
        self.probe_raw(raw_set(base))
        self.probe_pvt()
        self.probe_sats()
        self.restore_protocol(base)

    def probe_nmea(self, initial: list[str]) -> None:
        """Probe NMEA output selection, then restore the initial sentence set."""
        for case in NMEA_CASES:
            inv = self.set_and_observe("nmeaOut", "--nmea-out", case)
            if inv is not None:
                self.emission_observations.append(
                    EmissionObservation("nmeaOut", case, None,
                                        nmea_set(emissions(inv.packet_log))))
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
            inv = self.set_and_observe("rtcmOut", "--rtcm-out", case)
            if inv is not None:
                self.emission_observations.append(
                    EmissionObservation("rtcmOut", case, None,
                                        rtcm_set(emissions(inv.packet_log))))
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
            inv = self.set_and_observe("rawOut", "--raw-out", [kind])
            if inv is None:
                continue
            found[kind] = raw_set(emissions(inv.packet_log)) - initial
            self.emission_observations.append(
                EmissionObservation("rawOut", [kind], None, sorted(found[kind])))
        inv = self.set_and_observe("rawOut", "--raw-out", ["none"])
        if inv is not None:
            self.emission_observations.append(
                EmissionObservation("rawOut", ["none"], None,
                                    sorted(raw_set(emissions(inv.packet_log)) - initial)))
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

    def probe_pvt(self) -> None:
        """Probe PVT message output at the information level: apply each
        case in binary mode, replay the capture, and record the information
        kinds delivered."""
        for case in PVT_CASES:
            inv = self.set_and_observe("pvtOut", "--pvt-out", case.flags, pre=["--binary"])
            if inv is not None:
                kinds = event_kinds(self.tool.replay(inv.packet_log))
                self.emission_observations.append(
                    EmissionObservation("pvtOut", case.flags, None, sorted(kinds)))

    def probe_sats(self) -> None:
        """Probe satellite information output at the information level."""
        for case, _ in SATS_CASES:
            inv = self.set_and_observe("satsOut", "--sats-out", case,
                                       pre=["--binary", "--pvt-out", "off"])
            if inv is not None:
                kinds = event_kinds(self.tool.replay(inv.packet_log))
                self.emission_observations.append(
                    EmissionObservation("satsOut", case, None, sorted(kinds)))

    def restore_protocol(self, base: dict[tuple[str, str], int]) -> None:
        """Return the receiver to its pre-probe output mode. --nmea resets
        the sentence set (to RMC only on u-blox) as well as switching
        protocol, so the observed initial sentence set is restored after it.
        A receiver found in binary mode is switched back with --binary, but
        its PVT message selection cannot be reconstructed from observation;
        the verification below reports that honestly as a restore failure."""
        base_nmea = [t for t in nmea_set(base) if t in NMEA_VOCAB]
        if not base_nmea and raw_set(base):
            steps = [("restore-binary-mode", ["--binary"])]
        else:
            steps = [("restore-nmea-mode", ["--nmea"]),
                     ("restore-nmea-types",
                      ["--nmea-out", ",".join(base_nmea) if base_nmea else "none"])]
        for name, args in steps:
            inv = self.tool.gps(name, args)
            if inv.error is not None:
                self.failures.append(f"{name}: {inv.error}")
                return
        d = self.observe_emissions("verify-restore-messages")
        if d is None:
            return
        for what, got, want in [
                ("NMEA types", [t for t in nmea_set(d) if t in NMEA_VOCAB], base_nmea),
                ("RTCM types", rtcm_set(d), rtcm_set(base)),
                ("messages", sorted(raw_set(d)), sorted(raw_set(base)))]:
            if got != want:
                self.failures.append(
                    f"messages: {what} after restore {got!r} != initial {want!r}")

    def probe_pulse_physical(self, initial: dict[str, Any],
                             phc: tuple[str, int, int], use_sudo: bool) -> None:
        """Verify the time pulse electrically on the wired PHC pin: pulses
        present when enabled, absent when disabled. The default pulse fires
        only with a fix, so without one the check is skipped (absence would
        prove nothing). Pulse width and polarity are not observable through
        external timestamps and stay readback-only."""
        iface, pin, chan = phc
        inv = self.observe_capture("pulse-fix-check")
        if inv is None:
            return
        if not has_fix(self.tool.replay(inv.packet_log)):
            print("skipping physical time pulse checks: no fix", file=sys.stderr)
            return
        width = config_value(initial, ("timePulse", "width"))
        if not width:
            inv2 = self.tool.gps("set-pulse-on", ["--pps", "0.1"])
            if inv2.error is not None:
                self.failures.append(f"pulse: enabling for physical check: {inv2.error}")
                return
        on = self.tool.sdp_extts("sdp-pulse-enabled", iface, pin, chan, 4.0, use_sudo)
        if on is not None and len(on) < 2:
            self.failures.append(
                f"pulse enabled with fix but {len(on)} timestamps on {iface} pin {pin}")
        inv2 = self.tool.gps("set-pulse-off", ["--pps", "0"])
        if inv2.error is not None:
            self.failures.append(f"pulse: disabling for physical check: {inv2.error}")
        else:
            time.sleep(MSG_SETTLE)
            off = self.tool.sdp_extts("sdp-pulse-disabled", iface, pin, chan, 4.0, use_sudo)
            if off is not None and len(off) > 0:
                self.failures.append(
                    f"pulse disabled but {len(off)} timestamps on {iface} pin {pin}")
        inv2 = self.tool.gps("restore-pulse", ["--pps", fmt_value(width if width else 0)])
        if inv2.error is not None:
            self.failures.append(f"pulse: restore failed: {inv2.error}")

    def probe_signals(self, initial: dict[str, Any], supported: list[str]) -> None:
        """Probe constellation/band combinations, then restore the initial set."""
        prev = config_value(initial, ("signalsEnabled",))
        for gnss, band in signal_cases(supported):
            name = "-".join(gnss) + ("-" + "-".join(band) if band else "")
            args = ["--gnss", ",".join(gnss)]
            if band:
                args += ["--band", ",".join(band)]
            inv = self.tool.gps(f"set-signals-{name}", args)
            if transient(inv.error):
                self.failures.append(f"set-signals-{name}: {inv.error}")
                continue
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
            if transient(inv.error):
                self.failures.append(f"set-mode-{case.name}: {inv.error}")
                continue
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
            for k in mode_disagreements(reported, back):
                self.failures.append(
                    f"mode {case.name}: reported {k}={flat_value(reported, k)!r} "
                    f"but readback says {flat_value(back, k)!r}")
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

    def probe_speed(self, initial: dict[str, Any]) -> None:
        """Probe the serial speed property [disruptive]. The speed is a
        property like any other, so --speed with --save in one invocation
        must make the new rate both the running and the persisted rate;
        --reload then restarts the configuration from NVM, proving
        persistence when the receiver is still reachable at the new rate.
        Recovery: the original rate is restored and saved at the end."""
        orig = config_value(initial, ("baudRate",))
        if not isinstance(orig, int) or orig <= 0:
            return  # no UART speed reported: the property does not exist
        alt = 38400 if orig != 38400 else 115200
        inv = self.tool.gps("set-speed", ["--speed", str(alt), "--save"])
        if inv.error is not None:
            if transient(inv.error):
                self.failures.append(f"set-speed: {inv.error}")
            else:
                self.observations.append(Observation("speed", alt, inv.error, None, None))
            return
        reported = config_value(inv.config(), ("baudRate",))
        self.tool.set_speed(alt)
        if reported != alt:
            self.failures.append(f"speed: reported {reported!r}, requested {alt}")
        back: Value = None
        rel = self.tool.gps("speed-reload", ["--reload"])
        if rel.error is not None:
            self.failures.append(f"speed: reload at {alt} failed: {rel.error}")
        else:
            cfg = self.show_config("readback-speed")
            if cfg is not None:
                back = config_value(cfg, ("baudRate",))
                if back != alt:
                    self.failures.append(
                        f"speed: saved {alt} but post-reload readback says {back!r}")
        self.observations.append(Observation("speed", alt, None, reported, back))
        inv = self.tool.gps("restore-speed", ["--speed", str(orig), "--save"])
        if inv.error is not None:
            self.failures.append(f"speed: restore to {orig!r} failed: {inv.error}")
            self.tool.set_speed(orig)  # the receiver may have reverted; rediscover
            if self.show_config("speed-recover") is None:
                self.failures.append(f"speed: receiver unreachable at {orig!r} after failed restore")
            return
        self.tool.set_speed(orig)
        cfg = self.show_config("verify-restore-speed")
        if cfg is not None and config_value(cfg, ("baudRate",)) != orig:
            self.failures.append(
                f"speed: restore to {orig!r} read back as "
                f"{config_value(cfg, ('baudRate',))!r}")

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
