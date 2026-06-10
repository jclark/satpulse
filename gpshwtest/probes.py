"""Probe driving: plan and execute invocations against the receiver.

This layer decides what to run - the probe cases, adaptive choices such
as signal combinations from the discovered supported set, and the restore
steps derived from the initial readback - and executes it, recording each
step's intent in model vocabulary. It renders no verdicts: failures and
the characterization are derived offline from the records by analyze.py,
for live runs and archived runs alike.
"""

import sys
import time
from dataclasses import dataclass
from typing import Any, Callable

from model import (NMEA_VOCAB, Value, config_value, emissions, fmt_value, has_fix,
                   mode_args, nmea_set, raw_set, rtcm_set, transient)
from tool import Invocation, Tool, replay

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
    recorded with its intent; verdicts come from offline analysis."""

    tool: Tool

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

    def session_speed_raise(self, port_cfg: dict[str, Any],
                            supports: list[str]) -> int | None:
        """Raise a slow UART link to RAISED_SPEED for the session. Returns
        the as-found speed to restore at session end, or None when there is
        nothing to do (not a UART, already fast, no speed capability, or
        the receiver refused - then the session just runs slow). After a
        transient failure the link speed is unknown (the change may have
        applied with its confirmation lost), so the speed is rediscovered
        by scanning; the as-found speed still gets restored at the end."""
        baud = port_cfg.get("baudRate")
        if "speed" not in supports or not isinstance(baud, int) \
                or not 0 < baud < RAISED_SPEED:
            return None
        return baud if self.raise_speed(baud) else None

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
        truth."""
        inv = self.tool.gps("session-speed-restore", ["--speed", str(as_found)],
                            {"op": "session-speed", "role": "restore", "to": as_found})
        if inv.error is None:
            self.tool.set_speed(as_found)
        elif transient(inv.error):
            self.rediscover_speed()
        self.tool.gps("verify-session-speed", ["--show-port"],
                      {"op": "session-speed", "role": "verify", "want": as_found})

    def rediscover_speed(self) -> int | None:
        """Find the receiver again when the link speed is unknown: drop the
        pinned speed so the invocation scans, then pin what it found."""
        self.tool.set_speed(None)
        inv = self.tool.gps("session-speed-rediscover", ["--show-port"],
                            {"op": "session-speed", "role": "rediscover"})
        baud = inv.config().get("baudRate")
        if isinstance(baud, int) and baud > 0:
            self.tool.set_speed(baud)
            return baud
        return None

    def probe_scalar(self, p: ScalarProp, initial: dict[str, Any]) -> None:
        """Probe each value of p, then restore its initial value. A readback
        follows every answered set - refusals included, so analysis can
        verify a refusal changed nothing."""
        for v in p.values:
            inv = self.tool.gps(f"set-{p.name}", [p.flag, p.to_cli(v)],
                                {"op": "set", "prop": p.name, "path": list(p.path),
                                 "requested": v})
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
            self.show_config(f"readback-mode-{case.name}", "readback", "mode")
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
        visible to analysis in the records)."""
        name = "-".join(case)
        inv = self.tool.gps(f"set-{group}-{name}", (pre or []) + [flag, ",".join(case)],
                            {"op": "set-msg", "group": group, "case": case})
        if inv.error is not None:
            return None
        time.sleep(MSG_SETTLE)
        intent: dict[str, Any] = {"op": "observe", "role": "case", "group": group,
                                  "case": case}
        if expect is not None:
            intent["expect"] = sorted(expect)
        return self.observe(f"observe-{group}-{name}", intent)

    def probe_messages(self) -> dict[tuple[str, str], int] | None:
        """Probe NMEA, RTCM, raw, PVT, and satellite output from one shared
        baseline observation, restoring each group afterwards. Returns the
        baseline emissions, which later restores (after --reload) reuse."""
        base_inv = self.observe("messages-initial", {"op": "observe", "role": "baseline"})
        if base_inv is None:
            return None
        base = emissions(base_inv.packet_log)
        self.probe_nmea(nmea_set(base))
        self.probe_rtcm(rtcm_set(base))
        self.probe_raw(raw_set(base))
        self.probe_pvt()
        self.probe_sats()
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
        messages realizing each kind are discovered from the probe itself
        (whatever appears beyond the baseline), so restoring needs no
        receiver-specific knowledge."""
        found: dict[str, set[str]] = {}
        for kind in ("obs", "nav"):
            inv = self.set_and_observe("rawOut", "--raw-out", [kind])
            if inv is not None:
                found[kind] = raw_set(emissions(inv.packet_log)) - initial
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
        self.observe("verify-restore-messages",
                     {"op": "observe", "role": "verify", "group": "protocol"})

    def probe_reload(self, initial: dict[str, Any],
                     base: dict[tuple[str, str], int] | None,
                     uart: bool, raised: bool) -> None:
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
        baud = self.rediscover_speed() if uart else None
        self.show_config("readback-reload-2", "reload", "reload-2")
        if raised and baud is not None and baud < RAISED_SPEED:
            self.raise_speed(baud)
        for p in PROPS:
            self.restore(p, initial)
        self.restore_mode(initial)
        self.restore_signals(initial)
        if base is not None:
            self.restore_protocol(base)

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
