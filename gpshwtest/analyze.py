"""Offline analysis of a recorded run.

A pure pass over a run directory's records (raw.jsonl plus the
per-invocation packet logs), answering two separate questions: were the
tool guarantees violated (failures), and what is the characterization.
It never touches hardware: live runs call it on their own run directory
after probing, and it re-runs over archived runs whenever the checks or
the characterization vocabulary improve. Replay of packet logs uses
satpulsetool offline.
"""

import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from characterize import characterize
from model import (NMEA_VOCAB, EmissionObservation, Observation, SignalObservation,
                   Value, config_value, emissions, event_kinds, flat_value,
                   mode_disagreements, nmea_set, raw_set, rtcm_set, transient)
from tool import replay


@dataclass
class Step:
    """One recorded invocation: intent plus result."""

    seq: int
    name: str
    intent: dict[str, Any]
    argv: list[str]
    exit_code: int
    out: dict[str, Any]
    stderr: str
    log: Path | None
    events: list[dict[str, Any]] | None
    timeout: float | None

    @property
    def error(self) -> str | None:
        """The reported configuration error, or None on success."""
        if self.timeout is not None:
            return f"no response within {self.timeout}s"
        err = self.out.get("error")
        if isinstance(err, str):
            return err
        if self.exit_code != 0:
            return self.stderr.strip() or f"exit code {self.exit_code}"
        return None

    def config(self) -> dict[str, Any]:
        """The config object from the JSON output, empty if absent."""
        cfg = self.out.get("config")
        return cfg if isinstance(cfg, dict) else {}


def load_steps(run_dir: Path) -> list[Step]:
    """Load the recorded steps of a run. A retry record supersedes the
    attempt it retried."""
    steps: list[Step] = []
    for line in (run_dir / "raw.jsonl").read_text().splitlines():
        e = json.loads(line)
        if "intent" not in e:
            raise SystemExit(f"{run_dir}: records lack intents "
                             "(run predates offline analysis); cannot analyze")
        out = e.get("json")
        log = e.get("log")
        s = Step(seq=e.get("seq", 0), name=e["name"], intent=e["intent"],
                 argv=e.get("argv", []), exit_code=e.get("exit", -1),
                 out=out if isinstance(out, dict) else {},
                 stderr=e.get("stderr", ""),
                 log=run_dir / log if isinstance(log, str) else None,
                 events=e.get("events"), timeout=e.get("timeout"))
        if e.get("retry") and steps and steps[-1].intent == s.intent:
            steps.pop()
        steps.append(s)
    return steps


@dataclass
class Analysis:
    """The derived verdicts and characterization of one run."""

    receiver: dict[str, Any]
    supports: list[str]
    failures: list[str]
    observation_count: int
    characterization: dict[str, Any]


def analyze_run(run_dir: Path, exe: Path) -> Analysis:
    """Analyze a run directory: load its records and derive failures and
    the characterization."""
    return Analyzer(load_steps(run_dir), exe).run()


@dataclass
class Analyzer:
    """Sequential walk over a run's steps, mirroring the probe driving
    order: each set-like step is paired with the readback or observation
    the driver issues right after it. A truncated run (crash mid-probe)
    simply ends the walk; the steps up to the truncation still get full
    analysis."""

    steps: list[Step]
    exe: Path
    i: int = 0
    failures: list[str] = field(default_factory=list)
    observations: list[Observation] = field(default_factory=list)
    signal_observations: list[SignalObservation] = field(default_factory=list)
    emission_observations: list[EmissionObservation] = field(default_factory=list)
    receiver: dict[str, Any] = field(default_factory=dict)
    supports: list[str] = field(default_factory=list)
    initial: dict[str, Any] = field(default_factory=dict)
    prev_vals: dict[str, Value] = field(default_factory=dict)
    start_vals: dict[str, Value] = field(default_factory=dict)
    accepted: dict[str, list[Observation]] = field(default_factory=dict)
    baseline: dict[tuple[str, str], int] = field(default_factory=dict)
    raw_found: dict[str, set[str]] = field(default_factory=dict)

    def run(self) -> Analysis:
        while self.i < len(self.steps):
            s = self.steps[self.i]
            self.i += 1
            if s.timeout is not None:
                self.failures.append(
                    f"{s.name}: no response within {s.timeout}s: {' '.join(s.argv)}")
                continue
            if s.events is None and s.exit_code == 0 and not s.out:
                self.failures.append(f"{s.name}: exit 0 but no JSON output")
                continue
            self.step(s)
        self.check_values_move()
        enabled = sorted(self.initial.get("signalsEnabled") or {})
        doc = characterize(self.receiver, self.supports, self.observations,
                           self.signal_observations, self.emission_observations,
                           enabled)
        n = (len(self.observations) + len(self.signal_observations)
             + len(self.emission_observations))
        return Analysis(self.receiver, self.supports, self.failures, n, doc)

    def step(self, s: Step) -> None:
        op = s.intent.get("op")
        if op == "identify":
            self.identify(s)
        elif op == "show-port":
            self.show_port(s)
        elif op == "config":
            self.config_step(s)
        elif op == "set":
            self.set_scalar(s)
        elif op == "restore":
            self.restore_scalar(s)
        elif op == "set-mode":
            self.set_mode(s)
        elif op == "restore-mode":
            self.restore_mode(s)
        elif op == "set-signals":
            self.set_signals(s)
        elif op == "restore-signals":
            self.restore_signals(s)
        elif op == "observe":
            self.observe_step(s)
        elif op == "set-msg":
            self.set_msg(s)
        elif op == "restore-msg":
            self.restore_msg(s)
        elif op == "restore-protocol":
            if s.error is not None:
                self.failures.append(f"{s.name}: {s.error}")
        elif op == "pulse-set":
            self.pulse_set(s)
        elif op == "sdp":
            self.sdp(s)
        else:
            self.failures.append(f"{s.name}: unknown step intent {s.intent!r}")

    def take(self, op: str, **match: Any) -> Step | None:
        """Consume the next step when it matches; None otherwise (the run
        was truncated or the driver skipped the dependent step)."""
        if self.i < len(self.steps):
            s = self.steps[self.i]
            if (s.intent.get("op") == op and s.timeout is None
                    and all(s.intent.get(k) == v for k, v in match.items())):
                self.i += 1
                return s
        return None

    def take_config(self, role: str, prop: str) -> dict[str, Any] | None:
        """Consume the readback or verify-restore --show-config that follows
        a set or restore, returning its config; None when it is absent or
        failed (failed readbacks are failures, and dependent comparisons
        are skipped rather than cascaded)."""
        s = self.take("config", role=role, prop=prop)
        if s is None:
            return None
        if s.error is not None:
            self.failures.append(f"{s.name}: --show-config failed: {s.error}")
            return None
        return s.config()

    def identify(self, s: Step) -> None:
        if s.error is not None:
            self.failures.append(f"receiver detection failed: {s.error}")
            return
        rec = s.out.get("receiver")
        self.receiver = rec if isinstance(rec, dict) else {}
        sup = s.out.get("supports")
        self.supports = sup if isinstance(sup, list) else []

    def show_port(self, s: Step) -> None:
        if s.error is not None:
            self.failures.append(f"--show-port failed: {s.error}")
        elif not s.config().get("port"):
            self.failures.append(f"--show-port reported no port: {s.config()!r}")

    def config_step(self, s: Step) -> None:
        role = s.intent.get("role")
        if s.error is not None:
            self.failures.append(f"{s.name}: --show-config failed: {s.error}")
            return
        if role == "initial":
            self.initial = s.config()
        elif role == "final" and s.config() != self.initial:
            self.failures.append(f"receiver not left as found: "
                                 f"initial {self.initial!r}, final {s.config()!r}")

    def set_scalar(self, s: Step) -> None:
        prop, path = s.intent["prop"], tuple(s.intent["path"])
        v = s.intent["requested"]
        if transient(s.error):
            self.failures.append(f"{s.name}: {s.error}")
            return
        cfg = self.take_config("readback", prop)
        if cfg is None:
            return
        back = config_value(cfg, path)
        self.start_vals.setdefault(prop, config_value(self.initial, path))
        prev = self.prev_vals.get(prop, config_value(self.initial, path))
        obs = Observation(prop, v, s.error, config_value(s.config(), path), back)
        self.observations.append(obs)
        if s.error is not None:
            if back != prev:
                self.failures.append(
                    f"{prop}: refusal of {v!r} changed state: {prev!r} -> {back!r}")
        else:
            if obs.reported != back:
                self.failures.append(
                    f"{prop}: reported {obs.reported!r} but readback says {back!r}")
            self.accepted.setdefault(prop, []).append(obs)
            self.prev_vals[prop] = back

    def check_values_move(self) -> None:
        """An accepted set that leaves a property's value unchanged can be
        legitimate only as range clamping. When the requests bracket the
        prior value and it still never moved, no range limit explains it:
        the set was silently ignored, which is a bug."""
        for prop, accepted in self.accepted.items():
            start = self.start_vals.get(prop)
            if not isinstance(start, (int, float)):
                continue
            vals = [o.requested for o in accepted if isinstance(o.requested, (int, float))]
            if (all(o.readback == start for o in accepted)
                    and any(v < start for v in vals) and any(v > start for v in vals)):
                self.failures.append(
                    f"{prop}: accepted sets never changed the value from {start!r}")

    def restore_scalar(self, s: Step) -> None:
        prop, path = s.intent["prop"], tuple(s.intent["path"])
        v = s.intent["value"]
        if s.error is not None:
            self.failures.append(f"{prop}: restore to {v!r} failed: {s.error}")
            return
        cfg = self.take_config("verify-restore", prop)
        if cfg is not None and config_value(cfg, path) != v:
            self.failures.append(
                f"{prop}: restore to {v!r} read back as {config_value(cfg, path)!r}")

    def set_mode(self, s: Step) -> None:
        case, request = s.intent["case"], s.intent["request"]
        if transient(s.error):
            self.failures.append(f"{s.name}: {s.error}")
            return
        cfg = self.take_config("readback", "mode")
        if cfg is None:
            return
        back = config_value(cfg, ("mode",))
        prev = self.prev_vals.get("mode", config_value(self.initial, ("mode",)))
        if s.error is not None:
            for k, v in request.items():
                self.observations.append(Observation(f"mode.{k}", v, s.error, None, None))
            if back != prev:
                self.failures.append(
                    f"mode {case}: refusal changed state: {prev!r} -> {back!r}")
            return
        reported = config_value(s.config(), ("mode",))
        for k in mode_disagreements(reported, back):
            self.failures.append(
                f"mode {case}: reported {k}={flat_value(reported, k)!r} "
                f"but readback says {flat_value(back, k)!r}")
        for k, v in request.items():
            self.observations.append(Observation(
                f"mode.{k}", v, None, flat_value(reported, k), flat_value(back, k)))
        self.prev_vals["mode"] = back

    def restore_mode(self, s: Step) -> None:
        mode = s.intent["mode"]
        if s.error is not None:
            self.failures.append(f"mode: restore to {mode!r} failed: {s.error}")
            return
        cfg = self.take_config("verify-restore", "mode")
        if cfg is not None and config_value(cfg, ("mode",)) != mode:
            self.failures.append(
                f"mode: restore to {mode!r} read back as "
                f"{config_value(cfg, ('mode',))!r}")

    def set_signals(self, s: Step) -> None:
        gnss, band = s.intent["gnss"], s.intent["band"]
        name = "-".join(gnss) + ("-" + "-".join(band) if band else "")
        if transient(s.error):
            self.failures.append(f"{s.name}: {s.error}")
            return
        cfg = self.take_config("readback", "signals")
        if cfg is None:
            return
        back = config_value(cfg, ("signalsEnabled",))
        prev = self.prev_vals.get("signals",
                                  config_value(self.initial, ("signalsEnabled",)))
        if s.error is not None:
            self.signal_observations.append(SignalObservation(gnss, band, s.error, None))
            if back != prev:
                self.failures.append(
                    f"signals {name}: refusal changed state: {prev!r} -> {back!r}")
            return
        reported = config_value(s.config(), ("signalsEnabled",))
        if reported != back:
            self.failures.append(
                f"signals {name}: reported {reported!r} but readback says {back!r}")
        self.signal_observations.append(SignalObservation(gnss, band, None, back))
        self.prev_vals["signals"] = back

    def restore_signals(self, s: Step) -> None:
        want = s.intent["want"]
        if s.error is not None:
            self.failures.append(f"signals: restore to {want!r} failed: {s.error}")
            return
        cfg = self.take_config("verify-restore", "signals")
        if cfg is not None and config_value(cfg, ("signalsEnabled",)) != want:
            self.failures.append(
                f"signals: restore to {want!r} read back as "
                f"{config_value(cfg, ('signalsEnabled',))!r}")

    def observe_step(self, s: Step) -> None:
        """An observe step reached directly (not consumed by a set-like
        step): the baseline, a verify after a message restore, or the
        pulse fix check. Failed captures are failures wherever they occur."""
        if s.error is not None:
            self.failures.append(f"{s.name}: capture failed: {s.error}")
            return
        role, group = s.intent.get("role"), s.intent.get("group")
        if role == "baseline" and s.log is not None:
            self.baseline = emissions(s.log)
        elif role == "verify" and s.log is not None:
            self.verify_restore_msg(s, group)

    def set_msg(self, s: Step) -> None:
        group, case = s.intent["group"], s.intent["case"]
        if transient(s.error):
            self.failures.append(f"{s.name}: {s.error}")
            return
        if s.error is not None:
            self.emission_observations.append(
                EmissionObservation(group, case, s.error, []))
            return
        o = self.take("observe", role="case", group=group, case=case)
        if o is None:
            return
        if o.error is not None:
            self.failures.append(f"{o.name}: capture failed: {o.error}")
            return
        if o.log is None:
            return
        self.emission_observations.append(
            EmissionObservation(group, case, None, self.emitted(group, case, o),
                                o.intent.get("expect")))

    def emitted(self, group: str, case: list[str], o: Step) -> list[str]:
        """What the receiver emitted for one message-output case, in the
        group's vocabulary: sentence/message types for the wire-format
        groups, replayed information kinds for the semantic ones."""
        assert o.log is not None
        if group in ("pvtOut", "satsOut"):
            return sorted(event_kinds(replay(self.exe, o.log)))
        d = emissions(o.log)
        if group == "nmeaOut":
            return nmea_set(d)
        if group == "rtcmOut":
            return rtcm_set(d)
        new = raw_set(d) - raw_set(self.baseline)
        if case != ["none"]:
            self.raw_found[case[0]] = new
        return sorted(new)

    def restore_msg(self, s: Step) -> None:
        group, want = s.intent["group"], s.intent["want"]
        kind = {"nmeaOut": "nmea", "rtcmOut": "rtcm", "rawOut": "raw"}[group]
        if s.error is not None:
            self.failures.append(f"{kind}: restore to {want!r} failed: {s.error}")

    def verify_restore_msg(self, s: Step, group: str | None) -> None:
        """Check a post-restore observation against the baseline."""
        assert s.log is not None
        d = emissions(s.log)
        if group == "nmeaOut":
            want = [t for t in nmea_set(self.baseline) if t in NMEA_VOCAB]
            back = [t for t in nmea_set(d) if t in NMEA_VOCAB]
            if back != sorted(want):
                self.failures.append(f"nmea: restore to {want!r} read back as {back!r}")
        elif group == "rtcmOut":
            initial = rtcm_set(self.baseline)
            if rtcm_set(d) != initial:
                self.failures.append(
                    f"rtcm: restore to {initial!r} reads back as {rtcm_set(d)!r}")
        elif group == "rawOut":
            want = self.restore_want(s)
            if not set().union(*(self.raw_found.get(k, set()) for k in want)) <= raw_set(d):
                self.failures.append(f"raw: restore to {want!r} not emitting as before")
        elif group == "protocol":
            base_nmea = [t for t in nmea_set(self.baseline) if t in NMEA_VOCAB]
            for what, got, wanted in [
                    ("NMEA types", [t for t in nmea_set(d) if t in NMEA_VOCAB], base_nmea),
                    ("RTCM types", rtcm_set(d), rtcm_set(self.baseline)),
                    ("messages", sorted(raw_set(d)), sorted(raw_set(self.baseline)))]:
                if got != wanted:
                    self.failures.append(
                        f"messages: {what} after restore {got!r} != initial {wanted!r}")

    def restore_want(self, verify: Step) -> list[str]:
        """The want list of the restore-msg step this verify follows."""
        for s in reversed(self.steps[:self.i - 1]):
            if s.intent.get("op") == "restore-msg":
                want = s.intent["want"]
                return want if isinstance(want, list) else []
        return []

    def pulse_set(self, s: Step) -> None:
        role = s.intent["role"]
        if s.error is None:
            return
        msg = {"on": "enabling for physical check",
               "off": "disabling for physical check",
               "restore": "restore failed"}[role]
        self.failures.append(f"pulse: {msg}: {s.error}")

    def sdp(self, s: Step) -> None:
        if s.exit_code != 0 or s.events is None:
            return
        role, iface, pin = s.intent["role"], s.intent["iface"], s.intent["pin"]
        n = len(s.events)
        if role == "enabled" and n < 2:
            self.failures.append(
                f"pulse enabled with fix but {n} timestamps on {iface} pin {pin}")
        elif role == "disabled" and n > 0:
            self.failures.append(f"pulse disabled but {n} timestamps on {iface} pin {pin}")
