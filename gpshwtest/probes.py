"""Probe driving: plan and execute invocations against the receiver.

This layer decides what to run - the probe cases, adaptive choices such
as signal combinations from the discovered supported set, and the restore
steps derived from the initial readback - and executes it, recording each
step's intent in model vocabulary. It renders no verdicts: failures and
the characterization are derived offline from the records by analyze.py,
for live runs and archived runs alike.
"""

import json
import re
import shutil
import sys
import time
import tomllib
from dataclasses import dataclass
from functools import partial
from pathlib import Path
from typing import Any, Callable

from model import (DEFAULT_SURVEY_ACC, DEFAULT_SURVEY_TIME, NMEA_VOCAB, PVT_MSG_JSON,
                   RAW_MSG_JSON, SATS_MSG_JSON, SIGNAL_UNIVERSE, SignalMap, Value,
                   config_value, emissions, has_fix, l1_signals, l2_signals,
                   l5_signals, mode_target, nmea_set, normalize_signal_map,
                   port_has_serial_speed, pps_props, raw_set, requested_signals,
                   rtcm_set, signal_map_union, signal_map_without, signal_request_valid,
                   survey_opts, target_arg, transient)
from tool import Invocation, Tool, ToolFailure, replay

# Settle time after a successful signal-set change: u-blox documents an
# internal GNSS-subsystem restart (wait for the ACK plus 0.5 s); 2 s has
# been reliable on real hardware.
SIGNAL_SETTLE = 2.0

# Settle time after a signal-set change made with --save --reload (the
# signalOnlyWithReset qualifier): the reset reboots the receiver, which
# answers again within ~15 s on the LG290P; 20 s adds margin.
SAVE_RESET_SETTLE = 20.0

# Seconds of packet capture used to observe what the receiver emits;
# the message kinds under test are all per-epoch (1 Hz), so this spans
# several epochs.
OBSERVE_SECONDS = 4

# Settle time after changing message output before observing, so the
# observation window does not straddle the change.
MSG_SETTLE = 1.0

# Speed a slow UART session is raised to for its duration (a ground rule
# in GOAL.md): faster runs, and raw output cannot saturate the line.
RAISED_SPEED = 115200

# Settle time after --reset before talking to the rebooting receiver
# (covers USB re-enumeration as well as the restart itself).
RESET_SETTLE = 5.0

# The fix interval (seconds) the preconditioned rate probe runs at: fast enough
# that fix-coupled message output shows. The fix-rate-5 message-file tag carries
# the value; the as-found rate is restored with the tag matching it.
FIXRATE_FAST = 0.2

# A longer capture for the two rate observations: OBSERVE_SECONDS at 1 Hz is
# only ~3 intervals, so a wider window keeps the median inter-arrival stable.
RATE_OBSERVE_SECONDS = 6


def signal_universe(gnss: list[str]) -> SignalMap:
    """The union of the full model signal set of each named constellation:
    what --gnss denotes before the backend intersects it with the receiver's
    supported set. A signalsEnabled target of this is the JSON spelling of
    --gnss, so a run discovers the supported signals by requesting it."""
    return normalize_signal_map({g: SIGNAL_UNIVERSE.get(g, []) for g in gnss})


def signals_target(sigs: SignalMap) -> dict[str, Any]:
    """The ConfigTarget that requests an enabled-signal set."""
    return {"Props": {"signalsEnabled": sigs}}


def wire_flags(case: list[str]) -> list[str]:
    """The JSON message flags for a wire-format (NMEA/RTCM) probe case:
    plain complete requests, exactly the shape production callers send, so
    the group's unmodeled messages are turned off like everything else.
    Out-of-vocabulary as-found output is therefore not restorable; the
    documented per-receiver starting state (setup/<receiver>.sh) must not
    include any. 'none' clears the group (an empty list)."""
    return [t for t in case if t != "none"]


def rtcm_restore_flags(initial: list[str], arp: bool = True) -> list[str]:
    """The RTCM request flags that reproduce an observed initial type set:
    MSM4/MSM7 from the observed MSM numbers, ARP from an observed 1005. ARP is
    dropped when arp is False: 1005 is the fixed position, so asking for it
    while the receiver has none is not a request the receiver can honor."""
    want = []
    if any(t.startswith("1") and t.endswith("4") for t in initial):
        want.append("MSM4")
    if any(t.startswith("1") and t.endswith("7") for t in initial):
        want.append("MSM7")
    if arp and "1005" in initial:
        want.append("ARP")
    return want


def fixrate_interval(log: Path) -> float | None:
    """The fix interval reported by a tagged low-level rate query."""
    try:
        entries = [json.loads(line) for line in log.read_text().splitlines()]
    except (OSError, ValueError):
        return None
    for e in entries:
        if e.get("out"):
            continue
        a = e.get("ascii")
        if isinstance(a, str):
            m = re.search(r"\bPQTMCFGFIXRATE,(?:(?:OK|R),)?(\d+)(?:[,*]|$)", a)
            if m:
                return int(m.group(1)) / 1000
        h = e.get("bin")
        if not isinstance(h, str):
            continue
        try:
            b = bytes.fromhex(h)
        except ValueError:
            continue
        if len(b) >= 10 and b[:2] == b"\xBA\xCE" and b[4:6] == b"\x06\x04":
            return int.from_bytes(b[6:8], "little") / 1000
    return None


def msg_flags(case: list[str], table: dict[str, str]) -> list[str]:
    """The JSON message flags for a semantic (PVT/sats/raw) probe case,
    translating the CLI content tokens the cases use to their configtarget.go
    flag names; 'none' clears the group (an empty list)."""
    return [table.get(t, t) for t in case if t != "none"]


def flip_mode_target(mode: Value) -> dict[str, Any]:
    """The mode target that flips static<->mobile from the current mode, used
    in a save-granularity experiment to move the positioning mode off its NVM
    value. Flipping into static uses the default survey settings."""
    if isinstance(mode, dict) and not mode.get("static"):
        return {"Props": {"mode": {"static": True}},
                "Opts": survey_opts(DEFAULT_SURVEY_TIME, DEFAULT_SURVEY_ACC)}
    return {"Props": {"mode": {"static": False}}}


@dataclass
class ScalarProp:
    """A property settable and readable at one config JSON path. props builds
    the Props fragment of the JSON target that sets a model value; the value
    is in model units (the units of the config JSON)."""

    name: str
    values: list[Value]
    path: tuple[str, ...]
    props: Callable[[Value], dict[str, Any]]


PROPS = [
    ScalarProp("antennaCableDelay", [1, 123, 32767], ("antennaCableDelay",),
               lambda v: {"antennaCableDelay": v}),
    ScalarProp("minElevation", [1, 7, 45], ("minElevation",),
               lambda v: {"minElevation": v}),
    ScalarProp("timeGNSS", ["GAL", "BDS", "GLO", "GPS"], ("timeGNSS",),
               lambda v: {"timeGNSS": v}),
    ScalarProp("timePulse.width", [0.25, 0.000123456, 0.1], ("timePulse", "width"),
               lambda v: {"timePulse": pps_props(v)}),
]

# The RTCM base station ID only means something with a fixed position (on
# the UM980 it is the optional ID of MODE BASE), so it is probed during the
# positioning-mode phase while a fixed position is set, not in the plain
# scalar sweep.
RTCM_BASE_ID = ScalarProp("rtcmBaseID", [1, 1234, 4095], ("rtcmBaseID",),
                          lambda v: {"rtcmBaseID": v})


@dataclass
class ModeCase:
    """One positioning-mode request: the JSON target plus the mode fields it
    implies.

    Request keys use the mode JSON vocabulary, flattened. Only properties
    are listed: survey duration and accuracy are parameters of the
    survey-in operation (ConfigOptions in gpsprot/configtarget.go), not
    configuration properties, so readback never applies to them. Values
    have more decimals than any receiver resolution so quantization shows.
    The fixed positions are an arbitrary plausible point; receivers store
    the position without checking it against the actual location.
    """

    name: str
    target: dict[str, Any]
    request: dict[str, Value]


MODE_CASES = [
    ModeCase("survey",
             {"Props": {"mode": {"static": True}}, "Opts": survey_opts(300, 2.345)},
             {"static": True}),
    ModeCase("fixed-llh",
             {"Props": {"mode": {"static": True,
                                 "fixedPosLLH": [13.7318284567, 100.6447407891],
                                 "height": 12.34567, "fixedPosAcc": 0.12345}}},
             {"static": True, "fixedPosLLH[0]": 13.7318284567,
              "fixedPosLLH[1]": 100.6447407891, "height": 12.34567,
              "fixedPosAcc": 0.12345}),
    ModeCase("fixed-ecef",
             {"Props": {"mode": {"static": True,
                                 "fixedPosECEF": [-1132881.12345, 6092270.56789,
                                                  1504542.90123],
                                 "fixedPosAcc": 1.23456}}},
             {"static": True, "fixedPosECEF[0]": -1132881.12345,
              "fixedPosECEF[1]": 6092270.56789, "fixedPosECEF[2]": 1504542.90123,
              "fixedPosAcc": 1.23456}),
    ModeCase("mobile", {"Props": {"mode": {"static": False}}}, {"static": False}),
]

NMEA_CASES = [["RMC"], ["GGA", "ZDA"], ["none"]]

RTCM_CASES = [["MSM4", "ARP"], ["MSM7"], ["none"]]


@dataclass
class PVTCase:
    """One PVT output request and the information kinds it should deliver.

    Kinds are derived from the replayed event stream (see
    model.event_kinds): pos/vel from the position and velocity events,
    time from solution-time events, tp from pulse-time events (TimeRef
    PrePulse/PostPulse), leap from leapSecond, navEpoch from epoch markers
    (covers qual and epoch), tai/ecef when the corresponding content
    selector is honored. The survey flag is excluded: survey events only
    flow during a survey-in, so a mobile-mode probe cannot verify them.
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


@dataclass
class SignalCase:
    """One signal-set request. requested is the model signal set the flags
    denote when it is known before execution; discovery cases can leave it
    absent and let analysis derive it from the run."""

    name: str
    target: dict[str, Any]
    syntax: str
    requested: SignalMap | None
    tags: list[str]
    gnss: list[str] | None = None


SignalHypothesis = Callable[[SignalMap], list[SignalCase]]


def signal_discovery_cases(constellations: list[str],
                           known: SignalMap) -> list[SignalCase]:
    """Cases that discover the full signal set before the hypothesis
    generators run. Augmentation constellations are paired with an anchor
    because they cannot form a valid signal-set request on their own."""
    cases = []
    majors = [g for g in ("GPS", "GAL", "BDS", "GLO") if g in constellations]
    for g in majors:
        cases.append(gnss_signal_case(f"discover-{g}", [g], known, ["discover"]))
    anchor = "GPS" if "GPS" in constellations else majors[0] if majors else None
    if anchor is not None:
        for g in ("QZSS", "SBAS", "NAVIC"):
            if g in constellations:
                cases.append(gnss_signal_case(f"discover-{anchor}-{g}", [anchor, g],
                                              known, ["discover"]))
    if constellations:
        cases.append(gnss_signal_case("discover-all", constellations, known,
                                      ["discover", "all"]))
    for c in cases:
        c.requested = None
    return cases


def signal_cases(supported: SignalMap) -> list[SignalCase]:
    """Build a bounded, hypothesis-driven signal probe set."""
    cases: list[SignalCase] = []
    for hyp in SIGNAL_HYPOTHESES:
        cases += hyp(supported)
    return dedup_signal_cases(cases)


def syntax_hypothesis(supported: SignalMap) -> list[SignalCase]:
    cases = []
    gnss = list(supported)
    if gnss:
        cases.append(gnss_signal_case("syntax-gnss-all", gnss, supported, ["syntax"]))
        cases.append(direct_signal_case("syntax-signal-all", supported, ["syntax"]))
        first = first_signal(supported)
        if first:
            cases.append(except_signal_case("syntax-except-one", supported, first,
                                            ["syntax"]))
    return compact_cases(cases)


def anchor_hypothesis(supported: SignalMap) -> list[SignalCase]:
    cases = []
    for target in ("QZSS", "SBAS", "NAVIC"):
        if target not in supported:
            continue
        for anchor in ("GPS", "GAL", "BDS", "GLO"):
            if anchor in supported:
                req = signal_map_union(signal_subset(supported, anchor),
                                       signal_subset(supported, target))
                cases.append(direct_signal_case(
                    f"anchor-{target}-with-{anchor}", req,
                    [f"requires-gnss:{target}"]))
    return compact_cases(cases)


def l1_required_hypothesis(supported: SignalMap) -> list[SignalCase]:
    cases = []
    for g, sigs in supported.items():
        l1 = l1_signals(sigs)
        non_l1 = [s for s in sigs if s not in l1]
        cases.append(anchored_direct_case(
            f"l1-only-{g}", supported, g, l1, [f"l1-required:{g}"]))
        for s in l1:
            cases.append(anchored_direct_case(
                f"single-{g}-{s}", supported, g, [s], [f"l1-required:{g}"]))
        for s in non_l1:
            cases.append(anchored_direct_case(
                f"single-{g}-{s}", supported, g, [s], [f"l1-required:{g}"]))
        for s in l1:
            base = signal_subset(supported, g)
            if not signal_request_valid(base) and g != "GPS" and "GPS" in supported:
                base = signal_map_union(signal_subset(supported, "GPS"), base)
            cases.append(except_signal_case(
                f"without-{g}-{s}", base, {g: [s]}, [f"l1-required:{g}"]))
    return compact_cases(cases)


def l2_l5_hypothesis(supported: SignalMap) -> list[SignalCase]:
    cases = []
    for g, sigs in supported.items():
        l1, l2, l5 = l1_signals(sigs), l2_signals(sigs), l5_signals(sigs)
        if l1 and l2:
            cases.append(anchored_direct_case(
                f"l1-l2-{g}", supported, g, l1 + l2, [f"l2-l5:{g}"]))
        if l1 and l5:
            cases.append(anchored_direct_case(
                f"l1-l5-{g}", supported, g, l1 + l5, [f"l2-l5:{g}"]))
        if l1 and l2 and l5:
            cases.append(anchored_direct_case(
                f"l1-l2-l5-{g}", supported, g, l1 + l2 + l5, [f"l2-l5:{g}"]))
    l1_all = family_map(supported, l1_signals)
    l2_all = family_map(supported, l2_signals)
    l5_all = family_map(supported, l5_signals)
    cases.append(direct_signal_case("all-l1-l2", signal_map_union(l1_all, l2_all),
                                    ["l2-l5:all"]))
    cases.append(direct_signal_case("all-l1-l5", signal_map_union(l1_all, l5_all),
                                    ["l2-l5:all"]))
    cases.append(direct_signal_case("all-l1-l2-l5",
                                    signal_map_union(l1_all, l2_all, l5_all),
                                    ["l2-l5:all"]))
    return compact_cases(cases)


def ublox_conflict_hypothesis(supported: SignalMap) -> list[SignalCase]:
    cases = []
    bds = supported.get("BDS", [])
    glo = supported.get("GLO", [])
    gps = supported.get("GPS", [])
    gal = supported.get("GAL", [])
    if "B1I" in bds and "L1" in glo:
        base = {"BDS": ["B1I"], "GLO": ["L1"]}
        cases.append(direct_signal_case("conflict-bds-b1i-glo-l1", base,
                                        ["ublox-conflict:m8-m10"]))
        if gps:
            cases.append(direct_signal_case(
                "conflict-gps-bds-b1i-glo-l1",
                signal_map_union(signal_subset(supported, "GPS"), base),
                ["ublox-conflict:m8-m10"]))
        if gal:
            cases.append(direct_signal_case(
                "conflict-gal-bds-b1i-glo-l1",
                signal_map_union({"GAL": l1_signals(gal) or gal}, base),
                ["ublox-conflict:m8"]))
    if bds:
        cases.append(direct_signal_case("candidate-bds-b1i", {"BDS": ["B1I"]},
                                        ["ublox-conflict:m10"]))
        cases.append(direct_signal_case("candidate-bds-b1c", {"BDS": ["B1C"]},
                                        ["ublox-conflict:m10"]))
        cases.append(direct_signal_case("conflict-bds-b1i-b1c",
                                        {"BDS": ["B1I", "B1C"]},
                                        ["ublox-conflict:m10"]))
    l5_all = family_map(supported, l5_signals)
    if "L1" in glo and l5_all:
        cases.append(direct_signal_case(
            "conflict-glo-l1-with-l5",
            signal_map_union({"GLO": ["L1"]}, l5_all), ["ublox-conflict:f10"]))
    return compact_cases(cases)


SIGNAL_HYPOTHESES: list[SignalHypothesis] = [
    syntax_hypothesis,
    anchor_hypothesis,
    l1_required_hypothesis,
    l2_l5_hypothesis,
    ublox_conflict_hypothesis,
]


def gnss_signal_case(name: str, gnss: list[str], supported: SignalMap,
                     tags: list[str]) -> SignalCase:
    # The request sent is the constellations' whole model set; the backend
    # intersects it with what the receiver supports. requested records the
    # predicted result (the intersection with what is known so far), left None
    # for discovery cases that run before the supported set is known.
    return SignalCase(name, signals_target(signal_universe(gnss)), "gnss",
                      requested_signals(gnss, supported), tags, gnss)


def direct_signal_case(name: str, req: SignalMap,
                       tags: list[str]) -> SignalCase:
    r = normalize_signal_map(req)
    return SignalCase(name, signals_target(r), "signal", r, tags)


def except_signal_case(name: str, base: SignalMap, remove: SignalMap,
                       tags: list[str]) -> SignalCase:
    b = normalize_signal_map(base)
    r = normalize_signal_map(remove)
    req = signal_map_without(b, r)
    # base is always a subset of the supported set, so the denoted set (base
    # minus remove) equals what --gnss base --except-signal remove realizes.
    return SignalCase(name, signals_target(req), "except-signal", req, tags, list(b))


def anchored_direct_case(name: str, supported: SignalMap, gnss: str,
                         sigs: list[str], tags: list[str]) -> SignalCase:
    req = normalize_signal_map({gnss: sigs})
    if signal_request_valid(req) or gnss == "GPS" or "GPS" not in supported:
        return direct_signal_case(name, req, tags)
    return direct_signal_case(name, signal_map_union(signal_subset(supported, "GPS"), req),
                              tags)


def compact_cases(cases: list[SignalCase]) -> list[SignalCase]:
    return [c for c in cases if c.requested and signal_request_valid(c.requested)]


def dedup_signal_cases(cases: list[SignalCase]) -> list[SignalCase]:
    seen: set[str] = set()
    out = []
    for c in cases:
        k = json.dumps({"target": c.target, "request": c.requested, "syntax": c.syntax},
                       sort_keys=True)
        if k not in seen:
            seen.add(k)
            out.append(c)
    return out


def signal_subset(supported: SignalMap, gnss: str) -> SignalMap:
    sigs = supported.get(gnss, [])
    return {gnss: sigs} if sigs else {}


def family_map(supported: SignalMap,
               family: Callable[[list[str]], list[str]]) -> SignalMap:
    return normalize_signal_map({g: family(sigs) for g, sigs in supported.items()})


def with_save_reset(target: dict[str, Any], enabled: bool,
                    intent: dict[str, Any]) -> dict[str, Any]:
    """target with the Opts making a stored-only change effective (the
    onlyWithReset qualifiers), recorded in the step's intent. The Opts must
    ride in the target: a --target-json request takes its Opts from the JSON
    and ignores the --save/--reload flags entirely (createConfigTarget in
    gpscmd.go), so a flag would be dropped without an error."""
    if not enabled:
        return target
    intent["saveReset"] = True
    return {**target, "Opts": {**target.get("Opts", {}),
                               "Save": "minimal", "Reset": "reload"}}


def first_signal(m: SignalMap) -> SignalMap:
    for g, sigs in m.items():
        if sigs:
            return {g: [sigs[0]]}
    return {}


@dataclass
class ProbeRun:
    """Drives probes against one receiver. Pure execution: every step is
    recorded with its intent; verdicts come from offline analysis.
    line_dead is set when a message-output change stops getting through
    (a flooding receiver); the message phase stops there."""

    tool: Tool
    line_dead: bool = False
    speed_msg_path: Path | None = None
    speed_msg_port: str = "com1"
    # Signal (mode) changes need --save --reload to take effect (the
    # signalOnlyWithReset / modeOnlyWithReset qualifiers). Set only on
    # disruptive runs: they make every such request write NVM and
    # reboot the receiver.
    signal_save_reset: bool = False
    mode_save_reset: bool = False

    def show_config(self, name: str, role: str,
                    prop: str | None = None) -> dict[str, Any] | None:
        """Read the full current configuration in a separate invocation.
        Returns None when the invocation itself fails; callers then skip
        steps that depend on the result rather than cascade."""
        intent: dict[str, Any] = {"op": "config", "role": role}
        if prop is not None:
            intent["prop"] = prop
        inv = self.tool.gps(name, ["--show-config"], intent)
        return None if inv.error is not None else inv.config()

    def session_speed_raise(self, port_cfg: dict[str, Any], supports: list[str],
                            receiver: dict[str, Any]) -> int | None:
        """Raise a slow UART link for the session. Returns the as-found
        speed to restore at session end, or None when there is nothing to
        do (not a UART, already fast, no way to change the speed, or the
        receiver refused - then the session just runs slow). After a
        transient failure the link speed is unknown (the change may have
        applied with its confirmation lost), so the speed is rediscovered
        by scanning; the as-found speed still gets restored at the end.

        Backends without the speed capability fall back to the receiver's
        shipped low-level message file when it carries speed tags
        (configs/gpsmsg, currently the Unicore files): the link-speed
        command is sent with -m/-t and verified by talking at the new
        speed."""
        if not port_has_serial_speed(port_cfg):
            return None
        baud = port_cfg.get("baudRate")
        assert isinstance(baud, int)
        if "speed" in supports:
            if baud >= RAISED_SPEED:
                return None
            return baud if self.raise_speed(baud) else None
        return self.raise_speed_msgs(baud, receiver)

    def raise_speed_msgs(self, baud: int, receiver: dict[str, Any]) -> int | None:
        """Raise the link to RAISED_SPEED with the speed tags of the
        receiver's shipped low-level message file. Self-verifying: after
        sending the speed command, the receiver must answer at the new
        speed, else the speed is rediscovered and the session continues
        as found."""
        mf = self.speed_msg_file(receiver)
        target = RAISED_SPEED
        if mf is None or not 0 < baud < target:
            return None
        port = self.active_port(mf)
        if port is None or not {f"speed-{baud}-{port}", f"speed-{target}-{port}"} \
                <= self.msg_file_tags(mf):
            return None
        self.speed_msg_path = mf
        self.speed_msg_port = port
        if self.send_speed_msgs(baud, target):
            return baud
        found = self.rediscover_speed()
        if found == baud:
            self.speed_msg_path = None
            return None
        return baud

    def active_port(self, mf: Path) -> str | None:
        """Which receiver port this session is connected to, from the
        header of a long-format query response (the backend cannot report
        the port yet). The speed command must name the right port: the
        receiver happily reconfigures an unconnected one."""
        inv = self.tool.gps("query-active-port", ["-m", str(mf), "-t", "get-loglist"],
                            {"op": "session-speed", "role": "port-query"},
                            retry=False, json_out=False)
        try:
            for line in inv.packet_log.read_text().splitlines():
                e = json.loads(line)
                a = e.get("ascii", "")
                if not e.get("out") and a[:1] in "<#":
                    m = re.search(r"\b(COM\d)\b", a)
                    if m:
                        return m.group(1).lower()
        except OSError:
            pass
        return None

    def send_speed_msgs(self, baud: int, target: int) -> bool:
        """Send the message-file link-speed command and verify the receiver
        answers at the new speed."""
        assert self.speed_msg_path is not None
        self.tool.gps("session-speed-raise-msgs",
                      ["-m", str(self.speed_msg_path), "-t",
                       f"speed-{target}-{self.speed_msg_port}"],
                      {"op": "session-speed", "role": "raise-msgs",
                       "from": baud, "to": target}, retry=False, json_out=False)
        self.tool.set_speed(target)
        chk = self.tool.gps("verify-speed-raise-msgs", ["--show-receiver"],
                            {"op": "session-speed", "role": "raise-verify",
                             "to": target}, retry=False)
        return chk.error is None

    def speed_msg_file(self, receiver: dict[str, Any]) -> Path | None:
        """The shipped low-level message file for this receiver, when one
        exists (the receiver-specific knowledge lives there, not here)."""
        vendor = str(receiver.get("vendor", "")).lower()
        hw = str(receiver.get("hardware", "")).lower()
        if not vendor or not hw:
            return None
        found = shutil.which(str(self.tool.exe))
        exe = Path(found).resolve() if found is not None else self.tool.exe.resolve()
        roots = [exe.parent.parent / "share" / "satpulse" / "gpsmsg",
                 exe.parent.parent.parent / "configs" / "gpsmsg"]
        for root in roots:
            mf = root / vendor / f"{hw}.toml"
            if mf.exists():
                return mf
        return None

    def msg_file_tags(self, mf: Path) -> set[str]:
        """Every tag a message file makes available: across all message-type
        arrays (a file spells its messages as [[line]], [[nmea]], [[casbin]]
        and so on), and through [[include]], since a shared file carries the
        messages common to a family - the Zhongke fix-rate commands live in the
        included casic.toml, not in the per-model file."""
        seen: set[Path] = set()

        def collect(path: Path) -> set[str]:
            path = path.resolve()
            if path in seen:
                return set()
            seen.add(path)
            with open(path, "rb") as f:
                data = tomllib.load(f)
            tags: set[str] = set()
            for k, v in data.items():
                if k == "include" or not isinstance(v, list):
                    continue
                tags |= {ln["tag"] for ln in v
                         if isinstance(ln, dict) and isinstance(ln.get("tag"), str)}
            incs = data.get("include", [])
            if isinstance(incs, list):
                for inc in incs:
                    if isinstance(inc, dict) and isinstance(inc.get("src"), str):
                        tags |= collect(path.parent / inc["src"])
            return tags

        try:
            return collect(mf)
        except (OSError, tomllib.TOMLDecodeError):
            return set()

    def raise_speed(self, baud: int) -> bool:
        """Try to raise the link from baud to RAISED_SPEED. Returns whether
        the as-found speed now needs restoring: True on success, and after a
        transient failure (the change may have applied with its confirmation
        lost, so the speed is rediscovered by scanning and the restore must
        still run). A refusal returns False."""
        inv = self.tool.gps("session-speed-raise",
                            target_arg({"Props": {"baudRate": RAISED_SPEED}}),
                            {"op": "session-speed", "role": "raise",
                             "from": baud, "to": RAISED_SPEED})
        if inv.error is None:
            self.tool.set_speed(RAISED_SPEED)
            return True
        if transient(inv.error):
            self.rediscover_speed()
            return True
        return False

    def session_speed_restore(self, as_found: int) -> None:
        """Restore the as-found UART speed at session end. Runs whatever
        happened to the session, so the next run is not poisoned; a failed
        restore is rediscovered so the verification can still report the
        truth. A session raised through a message file restores the same
        way and verifies by answering at the restored speed (the backend
        cannot report its port)."""
        if self.speed_msg_path is not None:
            self.tool.gps("session-speed-restore-msgs",
                          ["-m", str(self.speed_msg_path), "-t",
                           f"speed-{as_found}-{self.speed_msg_port}"],
                          {"op": "session-speed", "role": "restore-msgs",
                           "to": as_found}, retry=False, json_out=False)
            self.tool.set_speed(as_found)
            self.tool.gps("verify-session-speed-msgs", ["--show-receiver"],
                          {"op": "session-speed", "role": "verify-msgs",
                           "want": as_found})
            return
        inv = self.tool.gps("session-speed-restore",
                            target_arg({"Props": {"baudRate": as_found}}),
                            {"op": "session-speed", "role": "restore", "to": as_found})
        if inv.error is None:
            self.tool.set_speed(as_found)
        elif transient(inv.error):
            self.rediscover_speed()
        self.tool.gps("verify-session-speed", ["--show-port"],
                      {"op": "session-speed", "role": "verify", "want": as_found})

    # Candidate link speeds for rediscovery, most likely first: the raised
    # session speed, the near-universal default, then other common rates
    # (460800 covers receivers left raised by runs predating ONCHANGED raw
    # output, when Unicore sessions ran at 460800).
    REDISCOVER_SPEEDS = [RAISED_SPEED, 460800, 9600, 38400, 57600, 19200, 230400]

    def rediscover_speed(self) -> int | None:
        """Find the receiver again when the link speed is unknown.
        satpulsetool does not scan baud rates (with no speed given it opens
        the port at its current termios state), so try the candidates
        explicitly and pin the first that answers. Individual attempts are
        expected to fail; analysis reports a failure only when the whole
        rediscovery does. A receiver that answers at no speed may simply be
        rebooting (the Unicore reload is realized as a receiver RESET), so
        a silent first sweep is repeated after a settle."""
        for attempt in range(2):
            for sp in self.REDISCOVER_SPEEDS:
                self.tool.set_speed(sp)
                inv = self.tool.gps(f"rediscover-at-{sp}", ["--show-port"],
                                    {"op": "session-speed", "role": "rediscover-try",
                                     "speed": sp}, retry=False)
                if inv.error is None:
                    # An answer at sp proves the line runs at sp. The
                    # readback's baudRate is stored configuration, which
                    # can lawfully differ from the live line (a V5 reload
                    # restores the stored baud without retuning the
                    # running UART), so it must not override the observed
                    # speed.
                    return sp
            if attempt == 0:
                time.sleep(RESET_SETTLE)
        self.tool.set_speed(None)
        return None

    def probe_scalar(self, p: ScalarProp, initial: dict[str, Any]) -> None:
        """Probe each value of p, then restore its value in initial. A
        readback follows every answered set - refusals included, so analysis
        can verify a refusal changed nothing. initial is whatever
        configuration is in effect when the probe starts: the run's initial
        readback for the plain sweep, or a context readback for properties
        that exist only in a particular state (rtcmBaseID with a fixed
        position); the first set carries the prior value so analysis does
        not assume the run-initial one."""
        first = True
        for v in p.values:
            intent: dict[str, Any] = {"op": "set", "prop": p.name,
                                      "path": list(p.path), "requested": v}
            if first:
                intent["prev"] = config_value(initial, p.path)
                first = False
            inv = self.tool.gps(f"set-{p.name}", target_arg({"Props": p.props(v)}), intent)
            if transient(inv.error):
                continue
            self.show_config(f"readback-{p.name}", "readback", p.name)
        self.restore(p, initial)

    def restore(self, p: ScalarProp, initial: dict[str, Any]) -> None:
        """Set p back to its value in the initial configuration."""
        v = config_value(initial, p.path)
        if v is None:
            return
        inv = self.tool.gps(f"restore-{p.name}", target_arg({"Props": p.props(v)}),
                            {"op": "restore", "prop": p.name, "path": list(p.path),
                             "value": v})
        if inv.error is None:
            self.show_config(f"verify-restore-{p.name}", "verify-restore", p.name)

    def probe_modes(self, initial: dict[str, Any]) -> None:
        """Probe each positioning-mode case, then restore the initial mode."""
        for case in MODE_CASES:
            intent: dict[str, Any] = {"op": "set-mode", "case": case.name,
                                      "request": case.request}
            inv = self.tool.gps(f"set-mode-{case.name}",
                                target_arg(self.mode_save_reset_target(case.target, intent)),
                                intent)
            if transient(inv.error):
                continue
            if inv.error is None and self.mode_save_reset:
                time.sleep(SAVE_RESET_SETTLE)
            cfg = self.show_config(f"readback-mode-{case.name}", "readback", "mode")
            if case.name == "fixed-ecef" and inv.error is None and cfg is not None:
                # The RTCM base station ID means something only with a fixed
                # position (it is the base ID of that mode), so probe it
                # while one is set; the mode restore below removes it again
                # on receivers where it exists only in that state.
                print("probing rtcmBaseID (with fixed position)", file=sys.stderr)
                self.probe_scalar(RTCM_BASE_ID, cfg)
        self.restore_mode(initial)

    def restore_mode(self, initial: dict[str, Any]) -> None:
        """Set the positioning mode back to its initial readback (and, when
        mode changes needed --save --reload, leave NVM holding it too)."""
        mode = config_value(initial, ("mode",))
        if not isinstance(mode, dict):
            return
        intent: dict[str, Any] = {"op": "restore-mode", "mode": mode}
        inv = self.tool.gps("restore-mode",
                            target_arg(self.mode_save_reset_target(mode_target(mode), intent)),
                            intent)
        if inv.error is None:
            if self.mode_save_reset:
                time.sleep(SAVE_RESET_SETTLE)
            self.show_config("verify-restore-mode", "verify-restore", "mode")

    def probe_signals(self, initial: dict[str, Any], supported: list[str]) -> None:
        """Probe signal-set hypotheses, then restore the initial set."""
        known = normalize_signal_map(config_value(initial, ("signalsEnabled",)))
        for case in signal_discovery_cases(supported, known):
            cfg = self.probe_signal_case(case)
            if cfg is not None:
                known = signal_map_union(known, normalize_signal_map(
                    config_value(cfg, ("signalsEnabled",))))
        for case in signal_cases(known):
            self.probe_signal_case(case)
        self.restore_signals(initial)

    def probe_signal_case(self, case: SignalCase) -> dict[str, Any] | None:
        """Run one signal case and return its readback config when present."""
        intent: dict[str, Any] = {"op": "set-signals", "syntax": case.syntax,
                                  "request": case.requested, "tags": case.tags}
        if case.gnss is not None:
            intent["gnss"] = case.gnss
        inv = self.tool.gps(f"set-signals-{case.name}",
                            target_arg(self.signal_save_reset_target(case.target, intent)),
                            intent)
        if transient(inv.error):
            return None
        if inv.error is None:
            time.sleep(SAVE_RESET_SETTLE if self.signal_save_reset else SIGNAL_SETTLE)
        return self.show_config(f"readback-signals-{case.name}", "readback", "signals")

    def restore_signals(self, initial: dict[str, Any]) -> None:
        """Re-enable the exact initial signal set (and, when signal changes
        needed --save --reload, leave NVM holding it too)."""
        want = normalize_signal_map(config_value(initial, ("signalsEnabled",)))
        if not want:
            return
        intent: dict[str, Any] = {"op": "restore-signals", "want": want}
        inv = self.tool.gps("restore-signals",
                            target_arg(self.signal_save_reset_target(
                                signals_target(want), intent)),
                            intent)
        if inv.error is None:
            time.sleep(SAVE_RESET_SETTLE if self.signal_save_reset else SIGNAL_SETTLE)
            self.show_config("verify-restore-signals", "verify-restore", "signals")

    def signal_save_reset_target(self, target: dict[str, Any],
                                 intent: dict[str, Any]) -> dict[str, Any]:
        """target made effective on a signalOnlyWithReset receiver,
        recorded in the step's intent."""
        return with_save_reset(target, self.signal_save_reset, intent)

    def mode_save_reset_target(self, target: dict[str, Any],
                               intent: dict[str, Any]) -> dict[str, Any]:
        """target made effective on a modeOnlyWithReset receiver,
        recorded in the step's intent."""
        return with_save_reset(target, self.mode_save_reset, intent)

    def observe(self, name: str, intent: dict[str, Any]) -> Invocation | None:
        """Capture for a few seconds; the packet log is what the receiver
        emits. Message output configuration is not readable back, so
        observation is both the verification and the restore baseline."""
        inv = self.tool.gps(name, ["--show-receiver", "--capture", str(OBSERVE_SECONDS)],
                            intent)
        return None if inv.error is not None else inv

    def set_and_observe(self, group: str, case: list[str], opts: dict[str, Any],
                        expect: set[str] | None = None,
                        seconds: int = OBSERVE_SECONDS,
                        rate: float | None = None) -> Invocation | None:
        """Apply one message-output case and observe the result with
        --capture in the same invocation: event output (raw navigation
        data per SEMANTICS.md) is delivered as a snapshot when the request
        is applied plus changes thereafter, so output emitted between a
        configuring invocation and a separate observing one would be lost
        with the port closed. opts is the ConfigOptions fragment the case
        sets. seconds lengthens the capture for the rate probe. rate marks an
        observation as running preconditioned to that fix interval (seconds),
        so the analyzer knows the rate check ran against a known fix rate.
        Returns None when the request was refused (visible to analysis in the
        records). A transient set failure means the link itself is in trouble
        (a flooding receiver answers nothing), so the message phase stops."""
        name = "-".join(case)
        intent: dict[str, Any] = {"op": "set-msg", "group": group, "case": case}
        if expect is not None:
            intent["expect"] = sorted(expect)
        if rate is not None:
            intent["rate"] = rate
        inv = self.tool.gps(f"set-{group}-{name}",
                            target_arg({"Opts": opts}, "--capture", str(seconds)),
                            intent)
        if transient(inv.error):
            self.line_dead = True
        return None if inv.error is not None else inv

    def probe_messages(self, initial_cfg: dict[str, Any], rtcm_fixed_pos_ecef: str | None,
                       receiver: dict[str, Any]) -> dict[tuple[str, str], int] | None:
        """Probe NMEA, RTCM, PVT, satellite, and raw output from one shared
        baseline observation, restoring each group afterwards. Raw runs
        last: it is the one group that can saturate the link beyond
        recovery (UM980 ephemeris output, see HW/um980.md), so a wedge
        poisons the least. A disable that cannot get through stops the
        phase rather than dragging every later probe down with it. Returns
        the baseline emissions, which later restores (after --reload) reuse."""
        base_inv = self.observe("messages-initial", {"op": "observe", "role": "baseline"})
        if base_inv is None:
            return None
        base = emissions(base_inv.packet_log)
        for probe in (partial(self.probe_nmea, nmea_set(base)),
                      partial(self.probe_rate, receiver),
                      partial(self.probe_rtcm, rtcm_set(base), initial_cfg,
                              rtcm_fixed_pos_ecef),
                      self.probe_pvt, self.probe_sats,
                      partial(self.probe_raw, raw_set(base))):
            if self.line_dead:
                break
            probe()
        self.restore_protocol(base)
        return base

    def probe_nmea(self, initial: list[str]) -> None:
        """Probe NMEA output selection, then restore the initial sentence set."""
        for case in NMEA_CASES:
            self.set_and_observe("nmeaOut", case, {"NMEAMsg": wire_flags(case)})
        want = [t for t in initial if t in NMEA_VOCAB]
        inv = self.tool.gps("restore-nmea",
                            target_arg({"Opts": {"NMEAMsg": want}}),
                            {"op": "restore-msg", "group": "nmeaOut", "want": want})
        if inv.error is None:
            self.observe("verify-restore-nmea",
                         {"op": "observe", "role": "verify", "group": "nmeaOut"})

    def probe_rate(self, receiver: dict[str, Any]) -> None:
        """Preconditioned message-rate probe: set a fast fix rate, then check
        NMEA and PVT output still arrive at 1 Hz (SEMANTICS.md, Rate). Enabled
        output is 1 Hz independent of the fix rate, so a fix-coupled bug is
        invisible while the receiver sits at its 1 Hz default fix rate; the
        precondition is the point. The fix rate has no device-independent
        knob, so it is set with the receiver's own message-file tags. The
        current interval is queried first and the matching fix-rate-N tag is
        used for restoration; the probe is skipped unless that exact restore
        is available. Running-state only - nothing saves it, so NVM is
        untouched and the observed rate is restored unconditionally.
        Assumes the as-found mode is not base mode: on the LG290P base mode
        forces 1 Hz and would mask the bug. A passing observation does not
        prove correct realization on a receiver already at 1 Hz; a fast
        observed rate, which only a fast fix rate can surface, is the finding."""
        mf = self.speed_msg_file(receiver)
        tags = self.msg_file_tags(mf) if mf is not None else set()
        if mf is None or not {"get-fix-rate", "fix-rate-5"} <= tags:
            print("skipping the message-rate probe: the receiver's message file "
                  "has no fix-rate tags", file=sys.stderr)
            return
        inv = self.tool.gps("fixrate-query", ["-m", str(mf), "-t", "get-fix-rate"],
                            {"op": "fixrate", "role": "query"}, retry=False,
                            json_out=False)
        interval = fixrate_interval(inv.packet_log) if inv.error is None else None
        if interval is None or interval <= 0:
            print("skipping the message-rate probe: the fix rate did not read back",
                  file=sys.stderr)
            return
        hz = round(1 / interval)
        restore_tag = f"fix-rate-{hz}"
        if hz <= 0 or abs(interval - 1 / hz) > 0.001 or restore_tag not in tags:
            print(f"skipping the message-rate probe: no tag restores the as-found "
                  f"fix interval of {interval}s", file=sys.stderr)
            return
        try:
            fast = self.send_fixrate(mf, "fix-rate-5", "fast", FIXRATE_FAST)
            if fast.error is not None:
                return
            self.set_and_observe("nmeaOut", ["RMC"], {"NMEAMsg": wire_flags(["RMC"])},
                                 seconds=RATE_OBSERVE_SECONDS, rate=FIXRATE_FAST)
            self.set_and_observe("pvtOut", ["pos", "time", "off"],
                                 {"NMEAMsg": [],
                                  "PVTMsg": msg_flags(["pos", "time", "off"], PVT_MSG_JSON)},
                                 expect={"pos", "time"},
                                 seconds=RATE_OBSERVE_SECONDS, rate=FIXRATE_FAST)
        finally:
            self.send_fixrate(mf, restore_tag, "restore", interval)

    def send_fixrate(self, mf: Path, tag: str, role: str,
                     interval: float) -> Invocation:
        """Send one fix-rate message-file tag. A message-file invocation
        cannot combine with --target-json, so it goes through -m/-t like the
        speed tags, with no JSON output; the rate change is confirmed by
        observing the emitted cadence, not read back."""
        return self.tool.gps(f"fixrate-{role}", ["-m", str(mf), "-t", tag],
                             {"op": "fixrate", "role": role,
                              "interval": interval}, retry=False, json_out=False)

    def probe_rtcm(self, initial: list[str], initial_cfg: dict[str, Any],
                   fixed_pos_ecef: str | None) -> None:
        """Probe RTCM output selection, then restore the initial emission."""
        fixed = False
        if fixed_pos_ecef is not None:
            xyz = [float(c) for c in fixed_pos_ecef.split(",")]
            inv = self.tool.gps("rtcm-fixed-mode",
                                target_arg({"Props": {"mode": {
                                    "static": True, "fixedPosECEF": xyz,
                                    "fixedPosAcc": 1}}}),
                                {"op": "rtcm-fixed-mode",
                                 "fixedPosECEF": fixed_pos_ecef})
            fixed = inv.error is None
        cases = RTCM_CASES if fixed else [c for c in RTCM_CASES if "ARP" not in c]
        try:
            for case in cases:
                self.set_and_observe("rtcmOut", case, {"RTCMMsg": wire_flags(case)})
            want = rtcm_restore_flags(initial, fixed)
            inv = self.tool.gps("restore-rtcm",
                                target_arg({"Opts": {"RTCMMsg": want}}),
                                {"op": "restore-msg", "group": "rtcmOut",
                                 "want": want})
            if inv.error is None:
                self.observe("verify-restore-rtcm",
                             {"op": "observe", "role": "verify", "group": "rtcmOut"})
        finally:
            if fixed:
                self.restore_mode(initial_cfg)

    def probe_raw(self, initial: set[str]) -> None:
        """Probe raw output kinds, then restore the initial emission. The
        messages realizing each kind are discovered from the probe itself,
        so restoring needs no receiver-specific knowledge. Raw runs after
        the binary-mode semantic probes, so what counts as new is diffed
        against a fresh observation rather than the session baseline; the
        restore decision still uses the session baseline (the kinds that
        were on as-found)."""
        pre_inv = self.observe("raw-baseline", {"op": "observe", "role": "raw-baseline"})
        if pre_inv is None:
            return
        pre = raw_set(emissions(pre_inv.packet_log))
        found: dict[str, set[str]] = {}
        for kind in ("obs", "nav"):
            inv = self.set_and_observe("rawOut", [kind],
                                       {"RawMsg": msg_flags([kind], RAW_MSG_JSON)})
            if inv is not None:
                found[kind] = raw_set(emissions(inv.packet_log)) - pre
            if self.line_dead:
                return
        self.set_and_observe("rawOut", ["none"], {"RawMsg": []})
        want = [k for k, msgs in found.items() if msgs and msgs <= initial]
        if not want:
            return
        inv = self.tool.gps("restore-raw",
                            target_arg({"Opts": {"RawMsg": msg_flags(want, RAW_MSG_JSON)}}),
                            {"op": "restore-msg", "group": "rawOut", "want": want})
        if inv.error is None:
            self.observe("verify-restore-raw",
                         {"op": "observe", "role": "verify", "group": "rawOut"})

    def probe_pvt(self) -> None:
        """Probe PVT message output at the information level: apply each
        case in binary mode and capture; analysis replays the capture and
        checks the information kinds delivered."""
        for case in PVT_CASES:
            # --binary: NMEA off, the case's PVT set.
            self.set_and_observe("pvtOut", case.flags,
                                 {"NMEAMsg": [],
                                  "PVTMsg": msg_flags(case.flags, PVT_MSG_JSON)},
                                 expect=case.expect)

    def probe_sats(self) -> None:
        """Probe satellite information output at the information level."""
        for flags, expect in SATS_CASES:
            # --binary --pvt-out off, plus the satellite case.
            self.set_and_observe("satsOut", flags,
                                 {"NMEAMsg": [], "PVTMsg": ["off"],
                                  "SatsMsg": msg_flags(flags, SATS_MSG_JSON)},
                                 expect=expect)

    def restore_protocol(self, base: dict[tuple[str, str], int],
                         save: bool = False) -> None:
        """Return the receiver to its pre-probe output mode. --nmea resets
        the sentence set (to RMC only on u-blox) as well as switching
        protocol, so the observed initial sentence set is restored after it.
        A receiver found in binary mode is switched back with --binary, but
        its PVT message selection cannot be reconstructed from observation;
        analysis reports that honestly as a restore failure. With save, each
        step also saves minimally: a disruptive run's NVM recovery persists
        whatever message rates the receiver was running at that moment, so
        the final restore must write the baseline rates back to NVM or a
        later reload re-arms the recovery-time rates (the rate-table time
        bomb HW/tau1302.md describes)."""
        base_nmea = [t for t in nmea_set(base) if t in NMEA_VOCAB]
        steps: list[tuple[str, dict[str, Any]]]
        if not base_nmea and raw_set(base):
            # --binary: turn NMEA off with a little binary PVT, as the flag
            # layer's --binary does.
            steps = [("restore-binary-mode",
                      {"NMEAMsg": [], "PVTMsg": ["pos", "time"]})]
        else:
            # --nmea then the exact sentence set - except that the initial
            # RTCM selection rides in the protocol switch where the flag
            # layer's --nmea unconditionally turns RTCM off: zeroing it here
            # would undo the restore probe_rtcm already verified, which is
            # exactly what happened on receivers found emitting RTCM. The
            # CLI cannot spell "NMEA mode with the RTCM kept"; the model can.
            steps = [("restore-nmea-mode",
                      {"NMEAMsg": ["RMC"], "RTCMMsg": rtcm_restore_flags(rtcm_set(base)),
                       "PVTMsg": ["off"], "RawMsg": [], "SatsMsg": []}),
                     ("restore-nmea-types", {"NMEAMsg": base_nmea})]
        for name, opts in steps:
            intent: dict[str, Any] = {"op": "restore-protocol"}
            if save:
                opts = {**opts, "Save": "minimal"}
                intent["save"] = True
            inv = self.tool.gps(name, target_arg({"Opts": opts}), intent)
            if inv.error is not None:
                return
        # The expectations ride in the intent: a restore-tail run's analyzer
        # has no baseline observation of its own to derive them from.
        self.observe("verify-restore-messages",
                     {"op": "observe", "role": "verify", "group": "protocol",
                      "nmea": base_nmea, "rtcm": rtcm_set(base),
                      "raw": sorted(raw_set(base))})

    def probe_reload(self, initial: dict[str, Any],
                     base: dict[tuple[str, str], int] | None,
                     uart: bool, raised: bool) -> dict[str, Any] | None:
        """Probe --reload: the first reload discovers the NVM state, then an
        unsaved canary change plus a second reload verify conclusively that
        unsaved changes do not survive a reload and that reloading is
        deterministic. A reload can change the link speed (NVM may hold a
        different baud rate, and it discards a raised session speed), so on
        a UART each reload is followed by rediscovery; satpulsetool may
        truthfully be unable to confirm the reload it performed, so the
        reload invocation's own error is judged by the analyzer against the
        readback, not taken at face value. Afterwards the as-found running
        configuration is restored: a reload replaces the running
        configuration with NVM contents, which need not match what was
        found running."""
        self.tool.gps("reload-1", target_arg({"Opts": {"Reset": "reload"}}),
                      {"op": "reload", "round": 1, "uart": uart})
        if uart:
            self.rediscover_speed()
        nvm = self.show_config("readback-reload-1", "reload", "reload-1")
        canary_v: int | None = None
        if nvm is not None:
            canary = next(p for p in PROPS if p.name == "minElevation")
            if config_value(nvm, canary.path) is not None:
                canary_v = 7 if config_value(nvm, canary.path) != 7 else 12
                self.tool.gps("canary-set-minElevation",
                              target_arg({"Props": canary.props(canary_v)}),
                              {"op": "canary-set", "prop": canary.name,
                               "path": list(canary.path), "value": canary_v})
        self.tool.gps("reload-2", target_arg({"Opts": {"Reset": "reload"}}),
                      {"op": "reload", "round": 2, "uart": uart})
        self.resync_speed(uart, raised)
        nvm2 = self.show_config("readback-reload-2", "reload", "reload-2")
        for p in PROPS:
            self.restore(p, initial)
        self.restore_mode(initial)
        self.restore_signals(initial)
        if base is not None:
            self.restore_protocol(base)
        # A surviving canary means the reload was ineffective (a receiver
        # without reload support): the second readback is then the running
        # configuration with the canary in it, not an NVM image, and using
        # it as the NVM reference would bake the canary into NVM when the
        # disruptive recovery saves that reference back. Fall back to the
        # pre-canary readback.
        if (nvm2 is not None and canary_v is not None
                and config_value(nvm2, canary.path) == canary_v):
            return nvm
        return nvm2 if nvm2 is not None else nvm

    def probe_disruptive(self, initial: dict[str, Any], nvm: dict[str, Any],
                         base: dict[tuple[str, str], int] | None,
                         uart: bool, as_found_speed: int | None,
                         speed_supported: bool) -> None:
        """Flag-gated NVM probes: save-granularity experiments (each uses
        --save, so they are also the --save probe), then recovery of the
        as-found NVM state through --save-all (which is thereby the
        --save-all probe), then --reset. nvm is the NVM state discovered by
        the reload probe; the experiments mutate NVM and the recovery puts
        it back, verified by a final reload readback."""
        raised = as_found_speed is not None
        subjects = [p for p in PROPS if config_value(nvm, p.path) is not None]
        r = nvm
        ok = True
        for p in subjects:
            print(f"save granularity: {p.name}", file=sys.stderr)
            nxt = self.gran_experiment(p, subjects, r, uart, raised)
            if nxt is None:
                ok = False
                break
            r = nxt
        if ok and isinstance(config_value(nvm, ("mode",)), dict):
            print("save granularity: mode", file=sys.stderr)
            r2 = self.gran_experiment(None, subjects, r, uart, raised)
            if r2 is not None and base is not None:
                print("save granularity: messageOutput", file=sys.stderr)
                self.gran_messages(r2, base, uart, raised)
        self.recover_nvm(nvm, subjects, uart, as_found_speed)
        if base is not None:
            # recover_nvm's --save-all persisted whatever message state the
            # experiments left; put the running message state back to the
            # session baseline (running-state only here - the reset and
            # factory probes that follow rewrite NVM anyway; the end-of-run
            # restore persists it).
            self.restore_protocol(base)
        self.probe_reset(uart, raised)
        self.probe_save_reset(nvm, uart, raised)
        if speed_supported and uart:
            self.probe_speed(self.tool.speed())
        print("probing factory reset", file=sys.stderr)
        self.probe_factory_reset(uart, raised)
        self.recover_nvm(nvm, subjects, uart, as_found_speed)
        for p in subjects:
            self.restore(p, initial)
        self.restore_mode(initial)
        self.restore_signals(initial)
        if base is not None:
            if as_found_speed is not None:
                self.set_link_speed(as_found_speed, "speed-for-message-save")
            self.restore_protocol(base, save=True)

    def recover_nvm(self, nvm: dict[str, Any], subjects: list[ScalarProp],
                    uart: bool, as_found_speed: int | None) -> None:
        """Restore the running configuration to the discovered NVM state and
        persist it with --save-all (which is thereby the --save-all probe,
        verified by the reload readback). --save-all persists the port
        configuration too, so the link is put back at the as-found speed
        before NVM is written."""
        for p in subjects:
            self.restore(p, nvm)
        self.restore_mode(nvm)
        self.restore_signals(nvm)
        if as_found_speed is not None:
            self.set_link_speed(as_found_speed, "speed-for-save-all")
        inv = self.tool.gps("save-all", target_arg({"Opts": {"Save": "all"}}),
                            {"op": "save-all"})
        if inv.error is None:
            self.tool.gps("recovery-reload", target_arg({"Opts": {"Reset": "reload"}}),
                          {"op": "reload", "round": 0, "uart": uart})
            self.resync_speed(uart, as_found_speed is not None)
            self.show_config("verify-save-all", "save-all")

    def set_link_speed(self, bps: int, name: str) -> None:
        """Move the link to bps; on a transient failure the actual speed is
        unknown, so rediscover. Sessions raised through a message file use
        the same mechanism (the backend has no speed capability), verified
        by the receiver answering at the new speed."""
        if self.speed_msg_path is not None:
            self.tool.gps(f"{name}-msgs",
                          ["-m", str(self.speed_msg_path), "-t",
                           f"speed-{bps}-{self.speed_msg_port}"],
                          {"op": "session-speed", "role": "restore-msgs", "to": bps},
                          retry=False, json_out=False)
            self.tool.set_speed(bps)
            chk = self.tool.gps(f"verify-{name}", ["--show-receiver"],
                                {"op": "session-speed", "role": "raise-verify",
                                 "to": bps}, retry=False)
            if chk.error is not None:
                self.rediscover_speed()
            return
        inv = self.tool.gps(name, target_arg({"Props": {"baudRate": bps}}),
                            {"op": "session-speed", "role": "restore", "to": bps})
        if inv.error is None:
            self.tool.set_speed(bps)
        elif transient(inv.error):
            self.rediscover_speed()

    # The serial speed value probed beyond the session speeds; one value
    # keeps the disruptive run short, and the session machinery already
    # exercises the raised and as-found speeds.
    SPEED_VALUES = [57600]

    def probe_speed(self, cur: int | None) -> None:
        """Probe the serial speed property. cur is the current operating
        speed on a UART; each accepted value moves the link, restored at
        the end."""
        for v in self.SPEED_VALUES:
            inv = self.tool.gps(f"set-speed-{v}", target_arg({"Props": {"baudRate": v}}),
                                {"op": "set-speed", "requested": v, "prev": cur or 0})
            if transient(inv.error):
                self.rediscover_speed()
                continue
            if inv.error is None and cur is not None:
                self.tool.set_speed(v)
            self.tool.gps("readback-speed", ["--show-port"], {"op": "speed-readback"})
        if cur is not None:
            self.set_link_speed(cur, "restore-speed-session")

    def probe_factory_reset(self, uart: bool, raised: bool) -> None:
        """Probe --factory-reset: NVM is replaced by factory defaults and
        the receiver reboots, so the link needs rediscovery and the verdict
        is that the readback responds. The factory state itself is receiver
        data, kept in the run artifacts rather than compared; the caller
        must recover NVM afterwards."""
        self.tool.gps("factory-reset", target_arg({"Opts": {"Reset": "factory"}}),
                      {"op": "factory-reset"})
        time.sleep(RESET_SETTLE)
        self.resync_speed(True, raised)
        self.show_config("readback-factory", "factory")

    def gran_messages(self, r: dict[str, Any], base: dict[tuple[str, str], int],
                      uart: bool, raised: bool) -> None:
        """One save-granularity experiment with message output as the
        subject: move a property off its NVM value unsaved, set a
        distinctive NMEA sentence set with --save, reload, and observe by
        capture whether the sentence set survived - message persistence is
        only observable by emission, never readback. The property canary
        classifies message output against the property groups. The NVM
        message state is left changed; the recovery save-all that follows
        persists the restored baseline."""
        canary = next(q for q in PROPS if q.name == "minElevation")
        v = next(x for x in canary.values if x != config_value(r, canary.path))
        self.tool.gps(f"gran-set-{canary.name}", target_arg({"Props": canary.props(v)}),
                      {"op": "gran-set", "exp": "messageOutput",
                       "prop": canary.name})
        target = ["RMC"] if nmea_set(base) != ["RMC"] else ["GGA"]
        self.tool.gps("gran-save-messages",
                      target_arg({"Opts": {"NMEAMsg": wire_flags(target),
                                           "Save": "minimal"}}),
                      {"op": "gran-save-msg", "case": target,
                       "prop": canary.name, "path": list(canary.path)})
        time.sleep(MSG_SETTLE)
        self.observe("gran-S-messages",
                     {"op": "observe", "role": "gran-msg-s", "case": target})
        self.show_config("gran-S-messages-cfg", "gran-msg-scfg", canary.name)
        self.tool.gps("gran-reload-messages", target_arg({"Opts": {"Reset": "reload"}}),
                      {"op": "reload", "round": 0, "uart": uart})
        self.resync_speed(uart, raised)
        time.sleep(MSG_SETTLE)
        self.observe("gran-F-messages",
                     {"op": "observe", "role": "gran-msg-f", "case": target})
        self.show_config("gran-F-messages-cfg", "gran-msg-fcfg", canary.name)

    def gran_experiment(self, p: ScalarProp | None, subjects: list[ScalarProp],
                        r: dict[str, Any], uart: bool,
                        raised: bool) -> dict[str, Any] | None:
        """One save-granularity experiment with subject p (None probes the
        positioning mode as the subject): move every other property off its
        NVM value unsaved, set the subject with --save in one invocation,
        snapshot the running state, reload, and read what survived. The
        post-reload readback is the NVM state the next experiment starts
        from, so NVM drift between experiments cannot corrupt the analysis."""
        name = p.name if p is not None else "mode"
        others: dict[str, list[str]] = {}
        for q in subjects:
            if p is not None and q.name == p.name:
                continue
            v = next(x for x in q.values if x != config_value(r, q.path))
            self.tool.gps(f"gran-set-{q.name}", target_arg({"Props": q.props(v)}),
                          {"op": "gran-set", "exp": name, "prop": q.name})
            others[q.name] = list(q.path)
        mode = config_value(r, ("mode",))
        if p is not None:
            if isinstance(mode, dict):
                self.tool.gps("gran-set-mode", target_arg(flip_mode_target(mode)),
                              {"op": "gran-set", "exp": name, "prop": "mode"})
                others["mode"] = ["mode"]
            v = next(x for x in p.values if x != config_value(r, p.path))
            target: dict[str, Any] = {"Props": p.props(v), "Opts": {"Save": "minimal"}}
            path = list(p.path)
        else:
            target = flip_mode_target(mode)
            target.setdefault("Opts", {})["Save"] = "minimal"
            path = ["mode"]
        self.tool.gps(f"gran-save-{name}", target_arg(target),
                      {"op": "gran-save", "exp": name, "path": path, "others": others})
        self.show_config(f"gran-S-{name}", "gran-s", name)
        self.tool.gps(f"gran-reload-{name}", target_arg({"Opts": {"Reset": "reload"}}),
                      {"op": "reload", "round": 0, "uart": uart})
        self.resync_speed(uart, raised)
        return self.show_config(f"gran-F-{name}", "gran-f", name)

    def probe_reset(self, uart: bool, raised: bool) -> None:
        """Probe --reset: the receiver reboots (the link drops whatever its
        kind, so the invocation's own error proves nothing), reloads its
        configuration from NVM, and discards acquired position/time/orbit
        data. The readback after rediscovery must show the NVM state."""
        self.tool.gps("reset", target_arg({"Opts": {"Reset": "cold"}}), {"op": "reset"})
        time.sleep(RESET_SETTLE)
        self.resync_speed(True, raised)
        self.show_config("verify-reset", "reset")

    def probe_save_reset(self, nvm: dict[str, Any], uart: bool, raised: bool) -> None:
        """Probe a combined save+reset in one invocation: <change> --save
        --reset is the CLI's own mandated shape (a reset that accompanies
        changes requires --save), and SEMANTICS.md gives the ordering - the
        save completes before the reset takes effect and gates it, so what the
        reset restores includes what was just saved. The established canary
        (minElevation, as in probe_reload and gran_messages) is set, saved,
        and reset together; after the reboot the readback must show the saved
        value. Timing-dependent: a passing probe does not prove the ordering,
        but a failing one is decisive. Skipped when the discovered NVM state
        lacks the canary property. The change is left in NVM; the
        recover_nvm(nvm, ...) call that already follows the factory-reset
        probe restores the discovered NVM state, so no local cleanup is
        needed."""
        canary = next(p for p in PROPS if p.name == "minElevation")
        cur = config_value(nvm, canary.path)
        if cur is None:
            return
        v = next(x for x in canary.values if x != cur)
        self.tool.gps("save-reset",
                      target_arg({"Props": canary.props(v),
                                  "Opts": {"Save": "minimal", "Reset": "cold"}}),
                      {"op": "save-reset", "prop": canary.name,
                       "path": list(canary.path), "value": v})
        time.sleep(RESET_SETTLE)
        self.resync_speed(True, raised)
        self.show_config("verify-save-reset", "save-reset")

    def resync_speed(self, uart: bool, raised: bool) -> None:
        """Re-pin the link speed after an operation that may have changed
        it, and raise it again for sessions that run raised."""
        if not uart:
            return
        baud = self.rediscover_speed()
        if not raised or baud is None:
            return
        if baud >= RAISED_SPEED:
            return
        if self.speed_msg_path is not None:
            self.send_speed_msgs(baud, RAISED_SPEED)
        else:
            self.raise_speed(baud)

    def emergency_restore(self, initial: dict[str, Any],
                          base: dict[tuple[str, str], int] | None,
                          uart: bool) -> None:
        """Best-effort restoration of the as-found running configuration
        when a run aborts. Every restore is attempted even when earlier
        ones fail (a ToolFailure normally aborts the run; here it must not
        cut the tail short), everything is recorded as usual, and the final
        readback lets analysis judge the result loudly."""
        if uart:
            self.attempt(self.rediscover_speed)
        for p in PROPS:
            self.attempt(partial(self.restore, p, initial))
        self.attempt(lambda: self.restore_mode(initial))
        self.attempt(lambda: self.restore_signals(initial))
        if base is not None:
            self.attempt(lambda: self.restore_protocol(base))
        self.attempt(lambda: self.tool.gps(
            "final-config", ["--show-config"],
            {"op": "config", "role": "final", "want": initial}))

    def attempt(self, fn: Callable[[], object]) -> None:
        """Run one step of the emergency tail; a tool failure is recorded
        (timeouts land in runs.jsonl) but must not stop the tail."""
        try:
            fn()
        except ToolFailure as e:
            print(f"emergency restore: {e}", file=sys.stderr)

    def probe_pulse_physical(self, initial: dict[str, Any],
                             phc: tuple[str, int, int], use_sudo: bool) -> None:
        """Verify the time pulse electrically on the wired PHC pin: pulses
        present when enabled, absent when disabled. The default pulse fires
        only with a fix, so without one the check is skipped (absence would
        prove nothing). Pulse width and polarity are not observable through
        external timestamps and stay readback-only."""
        iface, pin, chan = phc
        inv = self.observe("pulse-fix-check", {"op": "observe", "role": "fix-check"})
        if inv is None:
            return
        if not has_fix(replay(self.tool.exe, inv.packet_log)):
            print("skipping physical time pulse checks: no fix", file=sys.stderr)
            return
        width = config_value(initial, ("timePulse", "width"))
        if not width:
            inv2 = self.tool.gps("set-pulse-on",
                                 target_arg({"Props": {"timePulse": pps_props(0.1)}}),
                                 {"op": "pulse-set", "role": "on", "width": 0.1})
            if inv2.error is not None:
                return
        self.tool.sdp_extts("sdp-pulse-enabled", iface, pin, chan, 4.0, use_sudo,
                            {"op": "sdp", "role": "enabled", "iface": iface, "pin": pin})
        inv2 = self.tool.gps("set-pulse-off",
                             target_arg({"Props": {"timePulse": pps_props(0)}}),
                             {"op": "pulse-set", "role": "off", "width": 0})
        if inv2.error is None:
            time.sleep(MSG_SETTLE)
            self.tool.sdp_extts("sdp-pulse-disabled", iface, pin, chan, 4.0, use_sudo,
                                {"op": "sdp", "role": "disabled", "iface": iface, "pin": pin})
        self.tool.gps("restore-pulse",
                      target_arg({"Props": {"timePulse": pps_props(width if width else 0)}}),
                      {"op": "pulse-set", "role": "restore", "width": width if width else 0})
