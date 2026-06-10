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
    """The derived verdicts and characterization of one run. disruptive
    records whether the run included the flag-gated NVM/speed probes, whose
    characterization entries only a disruptive run can produce."""

    receiver: dict[str, Any]
    supports: list[str]
    failures: list[str]
    observation_count: int
    characterization: dict[str, Any]
    disruptive: bool


# The limitation entries only a disruptive run produces: a baseline from a
# disruptive run is compared in full, a default run with these stripped.
DISRUPTIVE_KEYS = ("baudRate", "saveGranularity")


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
    raw_base: set[str] | None = None
    raw_found: dict[str, set[str]] = field(default_factory=dict)
    ident_error: str | None = None
    reload_nvm: dict[str, Any] | None = None
    canary: tuple[tuple[str, ...], Value] | None = None
    gran_r: dict[str, Any] | None = None
    gran_exp: dict[str, Any] | None = None
    gran_s: dict[str, Any] | None = None
    save_results: list[dict[str, Any]] = field(default_factory=list)

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
        if self.ident_error is not None:
            self.failures.append(f"receiver detection failed: {self.ident_error}")
        self.check_values_move()
        enabled = sorted(self.initial.get("signalsEnabled") or {})
        doc = characterize(self.receiver, self.supports, self.observations,
                           self.signal_observations, self.emission_observations,
                           enabled, self.save_results)
        n = (len(self.observations) + len(self.signal_observations)
             + len(self.emission_observations))
        disruptive = any(s.intent.get("op") in ("gran-save", "set-speed",
                                                "factory-reset", "save-all")
                         for s in self.steps)
        return Analysis(self.receiver, self.supports, self.failures, n, doc,
                        disruptive)

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
        elif op == "session-speed":
            self.session_speed(s)
        elif op == "reload":
            self.reload(s)
        elif op == "canary-set":
            self.canary_set(s)
        elif op == "gran-set":
            if transient(s.error):
                self.failures.append(f"{s.name}: {s.error}")
        elif op == "gran-save":
            self.gran_save(s)
        elif op == "save-all":
            if s.error is not None:
                self.failures.append(f"save-all: {s.error}")
        elif op in ("reset", "factory-reset"):
            pass  # the receiver reboots; the readback carries the verdict
        elif op == "set-speed":
            self.set_speed_step(s)
        elif op == "speed-readback":
            pass  # consumed by set-speed; alone it carries no verdict
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
        """Identification may be attempted again after speed rediscovery,
        so a failed attempt counts only when no identify ever succeeded
        (checked at the end of the walk)."""
        if s.error is not None:
            self.ident_error = s.error
            return
        self.ident_error = None
        rec = s.out.get("receiver")
        self.receiver = rec if isinstance(rec, dict) else {}
        sup = s.out.get("supports")
        self.supports = sup if isinstance(sup, list) else []

    def show_port(self, s: Step) -> None:
        """--show-port must answer; a backend that does not implement port
        reporting yet (the Unicore backend) omits the fields, which is a
        limitation recorded in the characterization, not a failure."""
        if s.error is not None:
            self.failures.append(f"--show-port failed: {s.error}")
        elif not s.config().get("port"):
            self.observations.append(Observation("showPort", "port", None, None, None))

    def config_step(self, s: Step) -> None:
        role = s.intent.get("role")
        if s.error is not None:
            self.failures.append(f"{s.name}: --show-config failed: {s.error}")
            return
        if role == "initial":
            self.initial = s.config()
        elif role == "final":
            # A restore-tail run carries the crashed run's as-found state
            # in the intent; a normal run compares to its own initial.
            want = s.intent.get("want", self.initial)
            if want and s.config() != want:
                self.failures.append(f"receiver not left as found: "
                                     f"initial {want!r}, final {s.config()!r}")
        elif role == "reload":
            self.reload_readback(s)
        elif role == "gran-s":
            self.gran_s = s.config()
        elif role == "gran-f":
            self.gran_evaluate(s.config())
        elif role == "factory":
            pass  # factory state is receiver data, recorded, not compared
        elif role in ("save-all", "reset"):
            if self.reload_nvm is not None and s.config() != self.reload_nvm:
                what = "--save-all recovery" if role == "save-all" else "state after --reset"
                self.failures.append(
                    f"{what} does not match the NVM state: "
                    f"{self.reload_nvm!r} -> {s.config()!r}")

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
        elif role == "raw-baseline" and s.log is not None:
            self.raw_base = raw_set(emissions(s.log))
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
        base = self.raw_base if self.raw_base is not None else raw_set(self.baseline)
        new = raw_set(d) - base
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
            want_nmea = s.intent.get(
                "nmea", [t for t in nmea_set(self.baseline) if t in NMEA_VOCAB])
            want_rtcm = s.intent.get("rtcm", rtcm_set(self.baseline))
            want_raw = s.intent.get("raw", sorted(raw_set(self.baseline)))
            for what, got, wanted in [
                    ("NMEA types", [t for t in nmea_set(d) if t in NMEA_VOCAB], want_nmea),
                    ("RTCM types", rtcm_set(d), want_rtcm),
                    ("messages", sorted(raw_set(d)), want_raw)]:
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

    def reload(self, s: Step) -> None:
        """The reload invocation itself. On a UART satpulsetool may
        truthfully fail to confirm a reload it performed (the reload can
        change the link speed mid-invocation), so the verdict comes from
        the readback that follows rediscovery, not from this step's error.
        On a non-UART link there is no such excuse."""
        if s.error is not None and not s.intent.get("uart"):
            self.failures.append(f"{s.name}: {s.error}")

    def canary_set(self, s: Step) -> None:
        """The unsaved change whose survival the second reload tests."""
        if transient(s.error):
            self.failures.append(f"{s.name}: {s.error}")
        elif s.error is None:
            self.canary = tuple(s.intent["path"]), s.intent["value"]

    def reload_readback(self, s: Step) -> None:
        """The configuration read after a reload (and speed rediscovery).
        The first read is the NVM state; after the second, the canary must
        have reverted and the configuration must match the NVM state."""
        cfg = s.config()
        if s.intent.get("prop") == "reload-1":
            self.reload_nvm = cfg
            return
        if self.canary is not None:
            path, v = self.canary
            if config_value(cfg, path) == v:
                self.failures.append(
                    f"reload: unsaved {'.'.join(path)} change to {v!r} survived reload")
                return
        if self.reload_nvm is not None and cfg != self.reload_nvm:
            self.failures.append(
                f"reload: configuration after second reload differs from the "
                f"NVM state: {self.reload_nvm!r} -> {cfg!r}")

    def set_speed_step(self, s: Step) -> None:
        """The serial speed property, read back through --show-port (the
        only place the active port's speed appears). prev is the operating
        speed before the set, for the refusal-changed-nothing check."""
        v, prev = s.intent["requested"], s.intent.get("prev")
        if transient(s.error):
            self.failures.append(f"{s.name}: {s.error}")
            return
        rb = self.take("speed-readback")
        if rb is None:
            return
        if rb.error is not None:
            self.failures.append(f"{rb.name}: --show-port failed: {rb.error}")
            return
        back = rb.config().get("baudRate")
        obs = Observation("baudRate", v, s.error, s.config().get("baudRate"), back)
        self.observations.append(obs)
        if s.error is not None:
            if back != prev:
                self.failures.append(
                    f"baudRate: refusal of {v!r} changed state: {prev!r} -> {back!r}")
        elif obs.reported != back:
            self.failures.append(
                f"baudRate: reported {obs.reported!r} but readback says {back!r}")

    def gran_save(self, s: Step) -> None:
        """The set-with---save of one granularity experiment. A refusal
        voids the experiment (recorded verbatim); on success the experiment
        becomes current, to be evaluated by the gran-f readback."""
        self.gran_exp = None
        self.gran_s = None
        if transient(s.error):
            self.failures.append(f"{s.name}: {s.error}")
        elif s.error is not None:
            self.save_results.append({"prop": s.intent["exp"], "error": s.error})
        else:
            self.gran_exp = s.intent

    def gran_evaluate(self, f: dict[str, Any]) -> None:
        """Evaluate one granularity experiment from its three states: the
        baseline NVM state R, the running state at save time S, and the
        post-reload state F. The subject must persist (F == S at its path,
        the --save guarantee); every other property either kept its running
        value (same save group), reverted to R (independent), could not be
        distinguished (its running value never left R), or did something
        else entirely (carried verbatim as an anomaly)."""
        r, scfg, exp = self.gran_r if self.gran_r is not None else self.reload_nvm, \
            self.gran_s, self.gran_exp
        self.gran_r = f
        self.gran_exp = None
        self.gran_s = None
        if r is None or scfg is None or exp is None:
            return
        prop, path = exp["exp"], tuple(exp["path"])
        if config_value(f, path) != config_value(scfg, path):
            self.failures.append(
                f"save: {prop} saved as {config_value(scfg, path)!r} but reads "
                f"{config_value(f, path)!r} after reload")
        result: dict[str, Any] = {"prop": prop, "saved": [], "independent": [],
                                  "indeterminate": [], "anomalies": []}
        for q, qpath_list in sorted(exp["others"].items()):
            qpath = tuple(qpath_list)
            rv, sv, fv = (config_value(c, qpath) for c in (r, scfg, f))
            if sv == rv:
                result["indeterminate"].append(q)
            elif fv == sv:
                result["saved"].append(q)
            elif fv == rv:
                result["independent"].append(q)
            else:
                result["anomalies"].append(
                    {"prop": q, "nvm": rv, "running": sv, "afterReload": fv})
        self.save_results.append(result)

    def session_speed(self, s: Step) -> None:
        """Session speed management. A refused raise just leaves the session
        slow (the receiver may not accept the speed), and individual
        rediscovery attempts are expected to fail; everything else here must
        work, or the receiver risks being lost or left at the wrong speed
        for the next run."""
        role = s.intent["role"]
        if role == "raise":
            if transient(s.error):
                self.failures.append(f"{s.name}: {s.error}")
        elif role == "rediscover-try":
            if s.error is None:
                return
            nxt = self.steps[self.i].intent if self.i < len(self.steps) else {}
            if not (nxt.get("op") == "session-speed"
                    and nxt.get("role") == "rediscover-try"):
                self.failures.append(
                    f"session speed: receiver not rediscovered at any "
                    f"candidate speed: {s.error}")
        elif role == "rediscover":
            if s.error is not None:
                self.failures.append(
                    f"session speed: receiver not rediscovered after a failed "
                    f"speed change: {s.error}")
        elif role == "restore":
            if s.error is not None:
                self.failures.append(
                    f"session speed: restore to {s.intent['to']} failed: {s.error}")
        elif role == "verify":
            want = s.intent["want"]
            if s.error is not None:
                self.failures.append(f"{s.name}: --show-port failed: {s.error}")
            elif s.config().get("baudRate") != want:
                self.failures.append(
                    f"session speed: restored to {want} but the port reports "
                    f"{s.config().get('baudRate')!r}")

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
