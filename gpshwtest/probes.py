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
import sys
import time
import tomllib
from dataclasses import dataclass
from functools import partial
from pathlib import Path
from typing import Any, Callable

from model import (NMEA_VOCAB, Value, config_value, emissions, fmt_value, has_fix,
                   mode_args, nmea_set, raw_set, rtcm_set, transient)
from tool import Invocation, Tool, ToolFailure, replay

# Settle time after a successful signal-set change: u-blox documents an
# internal GNSS-subsystem restart (wait for the ACK plus 0.5 s); 2 s has
# been reliable on real hardware.
SIGNAL_SETTLE = 2.0

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
    ScalarProp("timeGNSS", "--time-gnss", ["GAL", "BDS", "GLO", "GPS"], ("timeGNSS",)),
    ScalarProp("timePulse.width", "--pps", [0.25, 0.000123456, 0.1], ("timePulse", "width")),
]

# The RTCM base station ID only means something with a fixed position (on
# the UM980 it is the optional ID of MODE BASE), so it is probed during the
# positioning-mode phase while a fixed position is set, not in the plain
# scalar sweep.
RTCM_BASE_ID = ScalarProp("rtcmBaseID", "--rtcm-base-id", [1, 1234, 4095],
                          ("rtcmBaseID",))


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
        speed. A backend that also cannot report its port (no baudRate)
        is trusted to be at the pinned connection speed."""
        baud = port_cfg.get("baudRate")
        if baud == 0:
            return None
        if not isinstance(baud, int):
            baud = self.tool.speed()
        if not isinstance(baud, int) or baud <= 0:
            return None
        if "speed" in supports:
            if baud >= RAISED_SPEED:
                return None
            return baud if self.raise_speed(baud) else None
        return self.raise_speed_msgs(baud, receiver)

    # Speed used for sessions raised through low-level message files; the
    # shipped Unicore files carry tags for 115200/230400/460800, and raw
    # output (full-constellation 1 Hz ephemeris) needs more than 115200.
    MSGS_RAISED_SPEED = 460800

    def raise_speed_msgs(self, baud: int, receiver: dict[str, Any]) -> int | None:
        """Raise the link with the speed tags of the receiver's shipped
        low-level message file. Self-verifying: after sending the speed
        command, the receiver must answer at the new speed, else the speed
        is rediscovered and the session continues as found."""
        mf = self.speed_msg_file(receiver)
        target = self.MSGS_RAISED_SPEED
        if mf is None or not 0 < baud < target:
            return None
        port = self.active_port(mf)
        if port is None or not self.speed_tags_exist(
                mf, [f"speed-{baud}-{port}", f"speed-{target}-{port}"]):
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
        with speed tags exists (the receiver-specific knowledge lives in
        the shipped file, not here)."""
        vendor = str(receiver.get("vendor", "")).lower()
        hw = str(receiver.get("hardware", "")).lower()
        if not vendor or not hw:
            return None
        mf = self.tool.exe.resolve().parent.parent.parent / "configs" / "gpsmsg" \
            / vendor / f"{hw}.toml"
        return mf if mf.exists() else None

    def speed_tags_exist(self, mf: Path, wanted: list[str]) -> bool:
        """Whether the message file carries every wanted tag."""
        try:
            with open(mf, "rb") as f:
                lines = tomllib.load(f).get("line", [])
        except (OSError, tomllib.TOMLDecodeError):
            return False
        tags = {ln.get("tag") for ln in lines if isinstance(ln, dict)}
        return all(t in tags for t in wanted)

    def raise_speed(self, baud: int) -> bool:
        """Try to raise the link from baud to RAISED_SPEED. Returns whether
        the as-found speed now needs restoring: True on success, and after a
        transient failure (the change may have applied with its confirmation
        lost, so the speed is rediscovered by scanning and the restore must
        still run). A refusal returns False."""
        inv = self.tool.gps("session-speed-raise", ["--speed", str(RAISED_SPEED)],
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
        inv = self.tool.gps("session-speed-restore", ["--speed", str(as_found)],
                            {"op": "session-speed", "role": "restore", "to": as_found})
        if inv.error is None:
            self.tool.set_speed(as_found)
        elif transient(inv.error):
            self.rediscover_speed()
        self.tool.gps("verify-session-speed", ["--show-port"],
                      {"op": "session-speed", "role": "verify", "want": as_found})

    # Candidate link speeds for rediscovery, most likely first: the raised
    # session speeds, the near-universal default, then other common rates.
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
                    baud = inv.config().get("baudRate")
                    if isinstance(baud, int) and baud > 0 and baud != sp:
                        self.tool.set_speed(baud)
                        return baud
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
            inv = self.tool.gps(f"set-{p.name}", [p.flag, p.to_cli(v)], intent)
            if transient(inv.error):
                continue
            self.show_config(f"readback-{p.name}", "readback", p.name)
        self.restore(p, initial)

    def restore(self, p: ScalarProp, initial: dict[str, Any]) -> None:
        """Set p back to its value in the initial configuration."""
        v = config_value(initial, p.path)
        if v is None:
            return
        inv = self.tool.gps(f"restore-{p.name}", [p.flag, p.to_cli(v)],
                            {"op": "restore", "prop": p.name, "path": list(p.path),
                             "value": v})
        if inv.error is None:
            self.show_config(f"verify-restore-{p.name}", "verify-restore", p.name)

    def probe_modes(self, initial: dict[str, Any]) -> None:
        """Probe each positioning-mode case, then restore the initial mode."""
        for case in MODE_CASES:
            inv = self.tool.gps(f"set-mode-{case.name}", case.args,
                                {"op": "set-mode", "case": case.name,
                                 "request": case.request})
            if transient(inv.error):
                continue
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
        """Set the positioning mode back to its initial readback."""
        mode = config_value(initial, ("mode",))
        if not isinstance(mode, dict):
            return
        inv = self.tool.gps("restore-mode", mode_args(mode),
                            {"op": "restore-mode", "mode": mode})
        if inv.error is None:
            self.show_config("verify-restore-mode", "verify-restore", "mode")

    def probe_signals(self, initial: dict[str, Any], supported: list[str]) -> None:
        """Probe constellation/band combinations, then restore the initial set."""
        for gnss, band in signal_cases(supported):
            name = "-".join(gnss) + ("-" + "-".join(band) if band else "")
            args = ["--gnss", ",".join(gnss)]
            if band:
                args += ["--band", ",".join(band)]
            inv = self.tool.gps(f"set-signals-{name}", args,
                                {"op": "set-signals", "gnss": gnss, "band": band})
            if transient(inv.error):
                continue
            if inv.error is None:
                time.sleep(SIGNAL_SETTLE)
            self.show_config(f"readback-signals-{name}", "readback", "signals")
        self.restore_signals(initial)

    def restore_signals(self, initial: dict[str, Any]) -> None:
        """Re-enable the initial constellation set. Band subsetting within a
        constellation cannot be reproduced generically, so a band-limited
        initial set shows up in analysis as a restore failure rather than
        silently passing."""
        want = config_value(initial, ("signalsEnabled",))
        if not isinstance(want, dict):
            return
        inv = self.tool.gps("restore-signals", ["--gnss", ",".join(want)],
                            {"op": "restore-signals", "want": want})
        if inv.error is None:
            time.sleep(SIGNAL_SETTLE)
            self.show_config("verify-restore-signals", "verify-restore", "signals")

    def observe(self, name: str, intent: dict[str, Any]) -> Invocation | None:
        """Capture for a few seconds; the packet log is what the receiver
        emits. Message output configuration is not readable back, so
        observation is both the verification and the restore baseline."""
        inv = self.tool.gps(name, ["--show-receiver", "--capture", str(OBSERVE_SECONDS)],
                            intent)
        return None if inv.error is not None else inv

    def set_and_observe(self, group: str, flag: str, case: list[str],
                        pre: list[str] | None = None,
                        expect: set[str] | None = None) -> Invocation | None:
        """Apply one message-output case and observe the result; None when
        the request was refused or the observation capture failed (both
        visible to analysis in the records). A transient set failure means
        the link itself is in trouble (a flooding receiver answers
        nothing), so the message phase stops."""
        name = "-".join(case)
        inv = self.tool.gps(f"set-{group}-{name}", (pre or []) + [flag, ",".join(case)],
                            {"op": "set-msg", "group": group, "case": case})
        if transient(inv.error):
            self.line_dead = True
        if inv.error is not None:
            return None
        time.sleep(MSG_SETTLE)
        intent: dict[str, Any] = {"op": "observe", "role": "case", "group": group,
                                  "case": case}
        if expect is not None:
            intent["expect"] = sorted(expect)
        return self.observe(f"observe-{group}-{name}", intent)

    def probe_messages(self) -> dict[tuple[str, str], int] | None:
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
                      partial(self.probe_rtcm, rtcm_set(base)),
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
            self.set_and_observe("nmeaOut", "--nmea-out", case)
        want = [t for t in initial if t in NMEA_VOCAB]
        inv = self.tool.gps("restore-nmea", ["--nmea-out", ",".join(want) if want else "none"],
                            {"op": "restore-msg", "group": "nmeaOut", "want": want})
        if inv.error is None:
            self.observe("verify-restore-nmea",
                         {"op": "observe", "role": "verify", "group": "nmeaOut"})

    def probe_rtcm(self, initial: list[str]) -> None:
        """Probe RTCM output selection, then restore the initial emission."""
        for case in RTCM_CASES:
            self.set_and_observe("rtcmOut", "--rtcm-out", case)
        want = []
        if any(t.endswith("4") and t.startswith("1") for t in initial):
            want.append("MSM4")
        if any(t.endswith("7") and t.startswith("1") for t in initial):
            want.append("MSM7")
        if "1005" in initial:
            want.append("ARP")
        inv = self.tool.gps("restore-rtcm", ["--rtcm-out", ",".join(want) if want else "none"],
                            {"op": "restore-msg", "group": "rtcmOut", "want": want})
        if inv.error is None:
            self.observe("verify-restore-rtcm",
                         {"op": "observe", "role": "verify", "group": "rtcmOut"})

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
            inv = self.set_and_observe("rawOut", "--raw-out", [kind])
            if inv is not None:
                found[kind] = raw_set(emissions(inv.packet_log)) - pre
            if self.line_dead:
                return
        self.set_and_observe("rawOut", "--raw-out", ["none"])
        want = [k for k, msgs in found.items() if msgs and msgs <= initial]
        if not want:
            return
        inv = self.tool.gps("restore-raw", ["--raw-out", ",".join(want)],
                            {"op": "restore-msg", "group": "rawOut", "want": want})
        if inv.error is None:
            self.observe("verify-restore-raw",
                         {"op": "observe", "role": "verify", "group": "rawOut"})

    def probe_pvt(self) -> None:
        """Probe PVT message output at the information level: apply each
        case in binary mode and capture; analysis replays the capture and
        checks the information kinds delivered."""
        for case in PVT_CASES:
            self.set_and_observe("pvtOut", "--pvt-out", case.flags, pre=["--binary"],
                                 expect=case.expect)

    def probe_sats(self) -> None:
        """Probe satellite information output at the information level."""
        for flags, expect in SATS_CASES:
            self.set_and_observe("satsOut", "--sats-out", flags,
                                 pre=["--binary", "--pvt-out", "off"], expect=expect)

    def restore_protocol(self, base: dict[tuple[str, str], int]) -> None:
        """Return the receiver to its pre-probe output mode. --nmea resets
        the sentence set (to RMC only on u-blox) as well as switching
        protocol, so the observed initial sentence set is restored after it.
        A receiver found in binary mode is switched back with --binary, but
        its PVT message selection cannot be reconstructed from observation;
        analysis reports that honestly as a restore failure."""
        base_nmea = [t for t in nmea_set(base) if t in NMEA_VOCAB]
        if not base_nmea and raw_set(base):
            steps = [("restore-binary-mode", ["--binary"])]
        else:
            steps = [("restore-nmea-mode", ["--nmea"]),
                     ("restore-nmea-types",
                      ["--nmea-out", ",".join(base_nmea) if base_nmea else "none"])]
        for name, args in steps:
            inv = self.tool.gps(name, args, {"op": "restore-protocol"})
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
        self.tool.gps("reload-1", ["--reload"], {"op": "reload", "round": 1, "uart": uart})
        if uart:
            self.rediscover_speed()
        nvm = self.show_config("readback-reload-1", "reload", "reload-1")
        if nvm is not None:
            canary = next(p for p in PROPS if p.name == "minElevation")
            if config_value(nvm, canary.path) is not None:
                v = 7 if config_value(nvm, canary.path) != 7 else 12
                self.tool.gps("canary-set-minElevation", [canary.flag, canary.to_cli(v)],
                              {"op": "canary-set", "prop": canary.name,
                               "path": list(canary.path), "value": v})
        self.tool.gps("reload-2", ["--reload"], {"op": "reload", "round": 2, "uart": uart})
        self.resync_speed(uart, raised)
        nvm2 = self.show_config("readback-reload-2", "reload", "reload-2")
        for p in PROPS:
            self.restore(p, initial)
        self.restore_mode(initial)
        self.restore_signals(initial)
        if base is not None:
            self.restore_protocol(base)
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
            # session baseline (its persistence rides the save-all above on
            # the next disruptive run; the running state is what matters).
            self.restore_protocol(base)
        self.probe_reset(uart, raised)
        if speed_supported:
            self.probe_speed(self.tool.speed() if uart else None)
        print("probing factory reset", file=sys.stderr)
        self.probe_factory_reset(uart, raised)
        self.recover_nvm(nvm, subjects, uart, as_found_speed)
        for p in subjects:
            self.restore(p, initial)
        self.restore_mode(initial)
        self.restore_signals(initial)
        if base is not None:
            self.restore_protocol(base)

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
        inv = self.tool.gps("save-all", ["--save-all"], {"op": "save-all"})
        if inv.error is None:
            self.tool.gps("recovery-reload", ["--reload"],
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
        inv = self.tool.gps(name, ["--speed", str(bps)],
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
        speed on a UART (each accepted value moves the link, restored at
        the end); on a non-UART link cur is None - the achieved value is 0,
        nothing changes, and there is nothing to restore."""
        for v in self.SPEED_VALUES:
            inv = self.tool.gps(f"set-speed-{v}", ["--speed", str(v)],
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
        self.tool.gps("factory-reset", ["--factory-reset"], {"op": "factory-reset"})
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
        self.tool.gps(f"gran-set-{canary.name}", [canary.flag, canary.to_cli(v)],
                      {"op": "gran-set", "exp": "messageOutput",
                       "prop": canary.name})
        target = ["RMC"] if nmea_set(base) != ["RMC"] else ["GGA"]
        self.tool.gps("gran-save-messages",
                      ["--nmea-out", ",".join(target), "--save"],
                      {"op": "gran-save-msg", "case": target,
                       "prop": canary.name, "path": list(canary.path)})
        time.sleep(MSG_SETTLE)
        self.observe("gran-S-messages",
                     {"op": "observe", "role": "gran-msg-s", "case": target})
        self.show_config("gran-S-messages-cfg", "gran-msg-scfg", canary.name)
        self.tool.gps("gran-reload-messages", ["--reload"],
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
            self.tool.gps(f"gran-set-{q.name}", [q.flag, q.to_cli(v)],
                          {"op": "gran-set", "exp": name, "prop": q.name})
            others[q.name] = list(q.path)
        mode = config_value(r, ("mode",))
        flip = ["--survey"] if isinstance(mode, dict) and not mode.get("static") \
            else ["--mobile"]
        if p is not None:
            if isinstance(mode, dict):
                self.tool.gps("gran-set-mode", flip,
                              {"op": "gran-set", "exp": name, "prop": "mode"})
                others["mode"] = ["mode"]
            v = next(x for x in p.values if x != config_value(r, p.path))
            args = [p.flag, p.to_cli(v), "--save"]
            path = list(p.path)
        else:
            args = flip + ["--save"]
            path = ["mode"]
        self.tool.gps(f"gran-save-{name}", args,
                      {"op": "gran-save", "exp": name, "path": path, "others": others})
        self.show_config(f"gran-S-{name}", "gran-s", name)
        self.tool.gps(f"gran-reload-{name}", ["--reload"],
                      {"op": "reload", "round": 0, "uart": uart})
        self.resync_speed(uart, raised)
        return self.show_config(f"gran-F-{name}", "gran-f", name)

    def probe_reset(self, uart: bool, raised: bool) -> None:
        """Probe --reset: the receiver reboots (the link drops whatever its
        kind, so the invocation's own error proves nothing), reloads its
        configuration from NVM, and discards acquired position/time/orbit
        data. The readback after rediscovery must show the NVM state."""
        self.tool.gps("reset", ["--reset"], {"op": "reset"})
        time.sleep(RESET_SETTLE)
        self.resync_speed(True, raised)
        self.show_config("verify-reset", "reset")

    def resync_speed(self, uart: bool, raised: bool) -> None:
        """Re-pin the link speed after an operation that may have changed
        it, and raise it again for sessions that run raised."""
        if not uart:
            return
        baud = self.rediscover_speed()
        if not raised or baud is None:
            return
        if self.speed_msg_path is not None:
            if baud < self.MSGS_RAISED_SPEED:
                self.send_speed_msgs(baud, self.MSGS_RAISED_SPEED)
        elif baud < RAISED_SPEED:
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
        (timeouts land in raw.jsonl) but must not stop the tail."""
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
            inv2 = self.tool.gps("set-pulse-on", ["--pps", "0.1"],
                                 {"op": "pulse-set", "role": "on", "width": 0.1})
            if inv2.error is not None:
                return
        self.tool.sdp_extts("sdp-pulse-enabled", iface, pin, chan, 4.0, use_sudo,
                            {"op": "sdp", "role": "enabled", "iface": iface, "pin": pin})
        inv2 = self.tool.gps("set-pulse-off", ["--pps", "0"],
                             {"op": "pulse-set", "role": "off", "width": 0})
        if inv2.error is None:
            time.sleep(MSG_SETTLE)
            self.tool.sdp_extts("sdp-pulse-disabled", iface, pin, chan, 4.0, use_sudo,
                                {"op": "sdp", "role": "disabled", "iface": iface, "pin": pin})
        self.tool.gps("restore-pulse", ["--pps", fmt_value(width if width else 0)],
                      {"op": "pulse-set", "role": "restore", "width": width if width else 0})
