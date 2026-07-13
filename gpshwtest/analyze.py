"""Offline analysis of a recorded run.

A pure pass over a log directory's records (runs.jsonl plus the
per-invocation packet logs), answering two separate questions: were the
tool guarantees violated (failures), and what is the characterization.
It never touches hardware: live runs call it on their own log directory
after probing, and it re-runs over archived runs whenever the checks or
the characterization vocabulary improve. Replay of packet logs uses
satpulsetool offline.
"""

import json
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from characterize import characterize
from model import (NMEA_VOCAB, EmissionObservation, Observation, SignalObservation,
                   Value, config_model_equal, config_value, emission_intervals,
                   emissions, event_intervals, event_kinds, flat_value,
                   mode_disagreements, nmea_rate_intervals, nmea_set,
                   normalize_config, normalize_signal_map, observation_start, pvt_event_kinds,
                   raw_set, rtcm_rate_intervals, rtcm_set, stored_form, transient)
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
    nojson: bool

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


def load_steps(log_dir: Path) -> list[Step]:
    """Load the recorded steps of a run. A retry record supersedes the
    attempt it retried."""
    steps: list[Step] = []
    record = log_dir / "runs.jsonl"
    if not record.exists():
        record = log_dir / "raw.jsonl"  # the record's name before the rename
    for line in record.read_text().splitlines():
        e = json.loads(line)
        if "intent" not in e:
            print(f"{log_dir}: records lack intents "
                  "(run predates offline analysis); cannot analyze", file=sys.stderr)
            raise SystemExit(2)
        out = e.get("json")
        log = e.get("log")
        s = Step(seq=e.get("seq", 0), name=e["name"], intent=e["intent"],
                 argv=e.get("argv", []), exit_code=e.get("exit", -1),
                 out=out if isinstance(out, dict) else {},
                 stderr=e.get("stderr", ""),
                 log=log_dir / log if isinstance(log, str) else None,
                 events=e.get("events"), timeout=e.get("timeout"),
                 nojson=bool(e.get("nojson")))
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


def analyze_run(log_dir: Path, exe: Path) -> Analysis:
    """Analyze a log directory: load its records and derive failures and
    the characterization."""
    return Analyzer(load_steps(log_dir), exe).run()


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
    stored_forms: dict[str, str] = field(default_factory=dict)
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
    gran_msg: dict[str, Any] | None = None
    gran_msg_s: list[str] | None = None
    gran_msg_f: list[str] | None = None
    gran_msg_scfg: dict[str, Any] | None = None
    save_results: list[dict[str, Any]] = field(default_factory=list)
    save_reset: dict[str, Any] | None = None
    replay_cache: dict[Path, list[dict[str, Any]]] = field(default_factory=dict)
    defect_keys: set[str] = field(default_factory=set)
    defects: dict[str, dict[str, Any]] = field(default_factory=dict)
    pending_nvm: list[tuple[set[str], str]] = field(default_factory=list)

    def run(self) -> Analysis:
        while self.i < len(self.steps):
            s = self.steps[self.i]
            self.i += 1
            if s.timeout is not None:
                self.failures.append(
                    f"{s.name}: no response within {s.timeout}s: {' '.join(s.argv)}")
                continue
            if s.events is None and s.exit_code == 0 and not s.out and not s.nojson:
                self.failures.append(f"{s.name}: exit 0 but no JSON output")
                continue
            self.step(s)
        if self.ident_error is not None:
            self.failures.append(f"receiver detection failed: {self.ident_error}")
        self.check_values_move()
        self.resolve_nvm()
        self.check_signal_equivalence()
        enabled = sorted(self.initial.get("signalsEnabled") or {})
        doc = characterize(self.receiver, self.supports, self.observations,
                           self.signal_observations, self.emission_observations,
                           enabled, self.save_results, self.stored_forms)
        if self.defects:
            doc["defects"] = {p: self.defects[p] for p in sorted(self.defects)}
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
        elif op == "rtcm-fixed-mode":
            if s.error is not None:
                self.failures.append(f"{s.name}: {s.error}")
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
        elif op == "gran-save-msg":
            self.gran_save_msg(s)
        elif op == "save-all":
            if s.error is not None:
                self.failures.append(f"save-all: {s.error}")
        elif op in ("reset", "factory-reset"):
            pass  # the receiver reboots; the readback carries the verdict
        elif op == "save-reset":
            # Like reset, the invocation's own error proves nothing (the
            # receiver reboots mid-invocation); the readback carries the
            # verdict. Remember the intent for the verify-save-reset readback,
            # with the accepted value when the response survived the reboot:
            # persistence is judged against what the receiver accepted, not
            # what was requested (quantization is a limitation, not a broken
            # save).
            self.save_reset = dict(s.intent)
            accepted = config_value(s.config(), tuple(s.intent["path"]))
            if accepted is not None:
                self.save_reset["accepted"] = accepted
        elif op == "fixrate":
            self.fixrate(s)
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
            if want and not config_model_equal(want, s.config()):
                self.pending_nvm.append((self.delta_keys(want, s.config()),
                    f"receiver not left as found: "
                    f"initial {want!r}, final {s.config()!r}"))
        elif role == "reload":
            self.reload_readback(s)
        elif role == "gran-s":
            self.gran_s = s.config()
        elif role == "gran-f":
            self.gran_evaluate(s.config())
        elif role == "gran-msg-scfg":
            self.gran_msg_scfg = s.config()
        elif role == "gran-msg-fcfg":
            self.gran_msg_evaluate(s.config())
        elif role == "factory":
            pass  # factory state is receiver data, recorded, not compared
        elif role in ("save-all", "reset"):
            if self.reload_nvm is not None \
                    and not config_model_equal(self.reload_nvm, s.config()):
                what = "--save-all recovery" if role == "save-all" else "state after --reset"
                self.pending_nvm.append((self.delta_keys(self.reload_nvm, s.config()),
                    f"{what} does not match the NVM state: "
                    f"{self.reload_nvm!r} -> {s.config()!r}"))
        elif role == "save-reset":
            # The save+reset persistence check: the value set with --save
            # --reset in one invocation must survive the reset (the save
            # completes before the reset, and gates it). A mismatch is a
            # broken persistence guarantee, not a limitation.
            if self.save_reset is not None:
                path = tuple(self.save_reset["path"])
                v = self.save_reset.get("accepted", self.save_reset["value"])
                got = config_value(s.config(), path)
                if got != v:
                    self.pending_nvm.append(({path[0]},
                        f"save+reset: {self.save_reset['prop']} saved as {v!r} "
                        f"with --reset but reads {got!r} after"))

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
        start = s.intent.get("prev", config_value(self.initial, path))
        self.start_vals.setdefault(prop, start)
        prev = self.prev_vals.get(prop, start)
        obs = Observation(prop, v, s.error, config_value(s.config(), path), back)
        self.observations.append(obs)
        if s.error is not None:
            if back != prev:
                self.failures.append(
                    f"{prop}: refusal of {v!r} changed state: {prev!r} -> {back!r}")
        else:
            self.accepted.setdefault(prop, []).append(obs)
            self.prev_vals[prop] = back

    def check_values_move(self) -> None:
        """An accepted set that leaves a property's value unchanged can be
        legitimate only as range clamping. When the requests bracket the
        prior value and it still never moved, no range limit explains it:
        the receiver acknowledged writes it did not apply. That is an
        ACK-without-apply receiver defect (characterization), not a
        tool-guarantee violation, so it is recorded on the property."""
        for prop, accepted in self.accepted.items():
            start = self.start_vals.get(prop)
            if not isinstance(start, (int, float)):
                continue
            vals = [o.requested for o in accepted if isinstance(o.requested, (int, float))]
            if (all(o.readback == start for o in accepted)
                    and any(v < start for v in vals) and any(v > start for v in vals)):
                for o in accepted:
                    self.note_defect(prop, prop.split(".")[0], o.requested, start)

    def restore_scalar(self, s: Step) -> None:
        prop, path = s.intent["prop"], tuple(s.intent["path"])
        v = s.intent["value"]
        if s.error is not None:
            self.failures.append(f"{prop}: restore to {v!r} failed: {s.error}")
            return
        cfg = self.take_config("verify-restore", prop)
        if cfg is not None:
            back = config_value(cfg, path)
            if back != v:
                # Accepted without error but the receiver did not apply it: an
                # ACK-without-apply defect, characterization rather than a tool
                # failure (the tool truthfully sent the restore and read back).
                self.note_defect(prop, path[0], v, back)

    def note_defect(self, prop: str, key: str, request: Value, readback: Value) -> None:
        """Record an accepted-but-ineffective set or restore on a property:
        the receiver ACKed the write and did not apply it. That is receiver
        behavior, not a violation of a tool guarantee (the tool truthfully
        sent the write and read the stored value back), so it is
        characterization. key is the top-level config key it lives under,
        tainted so the NVM-consistency checks recognize the same defect
        cascading into them."""
        self.defect_keys.add(key)
        d = self.defects.setdefault(prop, {"acceptedButNotApplied": []})
        entry = {"request": request, "readback": readback}
        if entry not in d["acceptedButNotApplied"]:
            d["acceptedButNotApplied"].append(entry)

    def delta_keys(self, want: dict[str, Any], got: dict[str, Any]) -> set[str]:
        """Top-level config keys that differ between two configs in the model."""
        a, b = normalize_config(want), normalize_config(got)
        return {k for k in set(a) | set(b) if a.get(k) != b.get(k)}

    def defect_cascade(self, keys: set[str]) -> bool:
        """Whether an NVM-consistency mismatch is entirely the cascade of an
        accepted-but-ineffective defect: every top-level key that differs was
        tainted by such a defect this run. The save truthfully persisted what
        the receiver was actually running; the ACK-without-apply is the root
        cause, already recorded on the property. Any delta beyond the tainted
        keys keeps the mismatch a failure."""
        return bool(keys) and keys <= self.defect_keys

    def resolve_nvm(self) -> None:
        """Resolve the deferred NVM-consistency mismatches into failures,
        except those that are only a tainted defect cascading. Deferred to
        after the full walk and check_values_move so every defect taint is
        known before the verdict."""
        for keys, msg in self.pending_nvm:
            if not self.defect_cascade(keys):
                self.failures.append(msg)

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
        rb = back
        if mode_disagreements(reported, back):
            form = stored_form(reported, back)
            if form is not None:
                # The accepted position was stored in the other
                # representation but denotes the same point: the per-key
                # readback is confirmed through the converted stored value.
                self.stored_forms.setdefault("mode.fixedPos", form)
                rb = reported
        for k, v in request.items():
            self.observations.append(Observation(
                f"mode.{k}", v, None, flat_value(reported, k), flat_value(rb, k)))
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
        req = self.signal_request(s)
        syntax = str(s.intent.get("syntax", "gnss"))
        tags = s.intent.get("tags")
        name = s.name.removeprefix("set-signals-")
        if transient(s.error):
            self.failures.append(f"{s.name}: {s.error}")
            return
        cfg = self.take_config("readback", "signals")
        if cfg is None:
            return
        back = normalize_signal_map(config_value(cfg, ("signalsEnabled",)))
        prev = self.prev_vals.get("signals",
                                  normalize_signal_map(config_value(
                                      self.initial, ("signalsEnabled",))))
        if s.error is not None:
            self.signal_observations.append(SignalObservation(
                req, syntax, s.error, None,
                gnss=s.intent.get("gnss"),
                tags=tags if isinstance(tags, list) else []))
            if back != prev:
                self.failures.append(
                    f"signals {name}: refusal changed state: {prev!r} -> {back!r}")
            return
        reported = normalize_signal_map(config_value(s.config(), ("signalsEnabled",)))
        achieved = back
        self.signal_observations.append(SignalObservation(
            req, syntax, None, achieved, reported if reported != achieved else None,
            gnss=s.intent.get("gnss"),
            tags=tags if isinstance(tags, list) else []))
        self.prev_vals["signals"] = achieved

    def signal_request(self, s: Step) -> dict[str, list[str]] | None:
        """The explicitly recorded model signal set for this step."""
        req = normalize_signal_map(s.intent.get("request"))
        return req if req else None

    def discovered_signal_set(self) -> dict[str, list[str]]:
        """Best current supported signal set from discovery observations,
        falling back to the initial readback for truncated runs."""
        supported: dict[str, list[str]] = {}
        for o in self.signal_observations:
            if o.error is not None or o.achieved is None:
                continue
            if "discover" in o.tags or o.syntax == "gnss":
                for g, sigs in o.achieved.items():
                    supported.setdefault(g, sigs)
        if supported:
            return supported
        return normalize_signal_map(config_value(self.initial, ("signalsEnabled",)))

    def check_signal_equivalence(self) -> None:
        """Equivalent signal sets must behave the same regardless of CLI
        syntax; otherwise the bug is in satpulsetool, not the receiver."""
        groups: dict[str, list[SignalObservation]] = {}
        for o in self.signal_observations:
            if o.requested is None:
                continue
            groups.setdefault(json.dumps(o.requested, sort_keys=True), []).append(o)
        for rows in groups.values():
            if len({o.syntax for o in rows}) < 2:
                continue
            outcomes = {json.dumps({"error": o.error is not None,
                                    "achieved": o.achieved},
                                   sort_keys=True) for o in rows}
            if len(outcomes) > 1:
                req = rows[0].requested
                detail = [(o.syntax, o.error is not None, o.achieved) for o in rows]
                self.failures.append(
                    f"signals: equivalent request {req!r} differed by syntax: {detail!r}")

    def restore_signals(self, s: Step) -> None:
        want = normalize_signal_map(s.intent["want"])
        if s.error is not None:
            self.failures.append(f"signals: restore to {want!r} failed: {s.error}")
            return
        cfg = self.take_config("verify-restore", "signals")
        back = normalize_signal_map(config_value(cfg or {}, ("signalsEnabled",)))
        if cfg is not None and back != want:
            self.failures.append(
                f"signals: restore to {want!r} read back as {back!r}")

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
        elif role == "gran-msg-s" and s.log is not None:
            self.gran_msg_s = nmea_set(emissions(s.log))
        elif role == "gran-msg-f" and s.log is not None:
            self.gran_msg_f = nmea_set(emissions(s.log))
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
        # The configuring invocation now captures its own observation;
        # older recorded runs observed in a separate following step.
        o = self.take("observe", role="case", group=group, case=case) or s
        if o.error is not None:
            self.failures.append(f"{o.name}: capture failed: {o.error}")
            return
        if o.log is None:
            return
        fi = s.intent.get("rate")
        self.emission_observations.append(
            EmissionObservation(group, case, None, self.emitted(group, case, o),
                                o.intent.get("expect"), self.rate_intervals(group, o),
                                fi if isinstance(fi, (int, float)) else None))

    def replay_events(self, log: Path) -> list[dict[str, Any]]:
        """Replay a packet log once and cache it: the semantic groups need
        the event stream for both the information kinds and the rate."""
        if log not in self.replay_cache:
            self.replay_cache[log] = replay(self.exe, log)
        return self.replay_cache[log]

    def emitted(self, group: str, case: list[str], o: Step) -> list[str]:
        """What the receiver emitted for one message-output case, in the
        group's vocabulary: sentence/message types for the wire-format
        groups, replayed information kinds for the semantic ones."""
        assert o.log is not None
        if group == "pvtOut":
            return sorted(pvt_event_kinds(o.log, self.replay_events(o.log)))
        if group == "satsOut":
            return sorted(event_kinds(self.replay_events(o.log)))
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

    def rate_intervals(self, group: str, o: Step) -> dict[str, float]:
        """The observed inter-arrival per single-per-epoch type for one
        observation, feeding the rate check (SEMANTICS.md, Rate): NMEA/RTCM
        from the packet log's safe types, PVT/satellite information from the
        epoch cadence of the replayed event stream. Raw output is event
        output with no rate, so it gets none."""
        assert o.log is not None
        if group == "pvtOut":
            return event_intervals(self.replay_events(o.log), "navEpoch",
                                   observation_start(o.log))
        if group == "satsOut":
            return event_intervals(self.replay_events(o.log), "satellites",
                                   observation_start(o.log))
        if group == "nmeaOut":
            return nmea_rate_intervals(emission_intervals(o.log))
        if group == "rtcmOut":
            return rtcm_rate_intervals(emission_intervals(o.log))
        return {}

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
        if self.reload_nvm is not None and not config_model_equal(self.reload_nvm, cfg):
            self.pending_nvm.append((self.delta_keys(self.reload_nvm, cfg),
                f"reload: configuration after second reload differs from the "
                f"NVM state: {self.reload_nvm!r} -> {cfg!r}"))

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
        if s.error is not None and back != prev:
            self.failures.append(
                f"baudRate: refusal of {v!r} changed state: {prev!r} -> {back!r}")

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

    def gran_save_msg(self, s: Step) -> None:
        """The set-with---save of the message-output granularity experiment."""
        self.gran_msg = None
        self.gran_msg_s = None
        self.gran_msg_f = None
        self.gran_msg_scfg = None
        if transient(s.error):
            self.failures.append(f"{s.name}: {s.error}")
        elif s.error is not None:
            self.save_results.append({"prop": "messageOutput", "error": s.error})
        else:
            self.gran_msg = s.intent

    def gran_msg_evaluate(self, fcfg: dict[str, Any]) -> None:
        """Evaluate the message-output granularity experiment: the saved
        sentence set must still be emitted after the reload (persistence is
        observable only by emission), and the unsaved property canary
        classifies message output against the property save groups."""
        exp, s_nmea, f_nmea = self.gran_msg, self.gran_msg_s, self.gran_msg_f
        scfg, r = self.gran_msg_scfg, self.gran_r
        self.gran_msg = None
        self.gran_msg_s = None
        self.gran_msg_f = None
        self.gran_msg_scfg = None
        if exp is None or s_nmea is None or f_nmea is None or scfg is None \
                or r is None:
            self.gran_r = fcfg
            return
        target = exp["case"]
        if not set(target) <= set(s_nmea):
            self.gran_r = fcfg
            return  # the set never took effect; delivery is characterized elsewhere
        if not set(target) <= set(f_nmea):
            self.failures.append(
                f"save: message output saved as {target!r} but emits "
                f"{f_nmea!r} after reload")
        path = tuple(exp["path"])
        rv, sv, fv = (config_value(c, path) for c in (r, scfg, fcfg))
        result: dict[str, Any] = {"prop": "messageOutput", "saved": [],
                                  "independent": [], "indeterminate": []}
        if sv == rv:
            result["indeterminate"].append(exp["prop"])
        elif fv == sv:
            result["saved"].append(exp["prop"])
        elif fv == rv:
            result["independent"].append(exp["prop"])
        self.save_results.append(result)
        self.gran_r = fcfg

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
        elif role in ("raise-msgs", "raise-verify", "restore-msgs", "port-query"):
            # Message-file speed changes cannot be confirmed in-invocation
            # (the link switches mid-session); the following verify or
            # rediscovery carries the verdict.
            pass
        elif role == "verify-msgs":
            if s.error is not None:
                self.failures.append(
                    f"session speed: restored to {s.intent['want']} but the "
                    f"receiver does not answer: {s.error}")
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

    def fixrate(self, s: Step) -> None:
        """The fix-rate precondition of the message-rate probe. A transient
        error is link trouble (a failure). A refusal of the fast rate just
        voids the rate check (the cases then run at the default rate and the
        passive check records nothing); a refusal of the default restore is a
        failure - the receiver may be left fast for the next run."""
        if transient(s.error):
            self.failures.append(f"{s.name}: {s.error}")
        elif s.error is not None and s.intent.get("role") == "default":
            self.failures.append(f"fixrate: restore to the default rate failed: {s.error}")

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
