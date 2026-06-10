"""Wrapper for satpulsetool gps invocations.

All receiver I/O goes through here: each invocation runs satpulsetool gps
with --json and a per-invocation packet log, and is recorded verbatim in
raw.jsonl in the run directory together with its intent - what the step
requests, in model vocabulary. The records plus the packet logs are
everything offline analysis needs (see analyze.py).
"""

import json
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from model import transient


class ToolFailure(Exception):
    """A violation of the tool guarantees: no response or no parseable output."""


@dataclass
class Invocation:
    """One satpulsetool gps invocation and its machine-readable result."""

    name: str
    argv: list[str]
    exit_code: int
    out: dict[str, Any]
    stderr: str
    packet_log: Path

    @property
    def error(self) -> str | None:
        """The reported configuration error, or None on success."""
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


class Tool:
    """Runs satpulsetool gps against one receiver, archiving every invocation."""

    def __init__(self, exe: Path, conn: list[str], run_dir: Path) -> None:
        self.exe = exe
        self.conn = conn
        self.run_dir = run_dir
        self.seq = 0
        run_dir.mkdir(parents=True, exist_ok=True)
        self.raw = (run_dir / "raw.jsonl").open("a", encoding="utf-8")

    def gps(self, name: str, args: list[str], intent: dict[str, Any],
            timeout: float = 90.0) -> Invocation:
        """Run satpulsetool gps with the given high-level args plus --json
        and a per-invocation packet log. Raises ToolFailure on timeout or
        on success without JSON output; a configuration error is not a
        failure and is reported through Invocation.error.

        Communication flakes happen (intermittent detection of a silenced
        receiver on USB, unanswered requests on a slow UART), so a
        transient error is retried once; the flake stays visible in
        raw.jsonl and the packet logs."""
        inv = self.gps_once(name, args, intent, timeout, retry=False)
        if transient(inv.error):
            time.sleep(2.0)
            inv = self.gps_once(f"{name}-retry", args, intent, timeout, retry=True)
        return inv

    def gps_once(self, name: str, args: list[str], intent: dict[str, Any],
                 timeout: float, retry: bool) -> Invocation:
        self.seq += 1
        log = self.run_dir / f"{self.seq:03d}-{name}.jsonl"
        argv = [str(self.exe), "gps", *self.conn, "--json", "--packet-log", str(log), *args]
        entry = {"seq": self.seq, "name": name, "intent": intent, "argv": argv,
                 "log": log.name}
        if retry:
            entry["retry"] = True
        try:
            p = subprocess.run(argv, capture_output=True, text=True, timeout=timeout)
        except subprocess.TimeoutExpired:
            self.record({**entry, "timeout": timeout})
            raise ToolFailure(f"{name}: no response within {timeout}s: {' '.join(argv)}")
        out: dict[str, Any] = {}
        if p.stdout:
            try:
                v = json.loads(p.stdout)
                if isinstance(v, dict):
                    out = v
            except ValueError:
                pass
        inv = Invocation(name, argv, p.returncode, out, p.stderr, log)
        self.record({**entry, "exit": p.returncode, "json": out if out else p.stdout,
                     "stderr": p.stderr})
        if p.returncode == 0 and not out:
            raise ToolFailure(f"{name}: exit 0 but no JSON output")
        return inv

    def sdp_extts(self, name: str, iface: str, pin: int, chan: int, seconds: float,
                  use_sudo: bool, intent: dict[str, Any]) -> list[dict[str, Any]] | None:
        """Read external timestamp events from a PHC pin for a few seconds.
        Requires root; run with sudo -n when use_sudo is set. Returns None
        when sdp itself fails (the caller decides what that means)."""
        self.seq += 1
        argv = (["sudo", "-n"] if use_sudo else []) + \
            [str(self.exe), "sdp", "--extts", "--jsonl", "-p", str(pin),
             "--chan", str(chan), "-t", str(seconds), iface]
        try:
            p = subprocess.run(argv, capture_output=True, text=True, timeout=seconds + 30)
        except subprocess.TimeoutExpired:
            raise ToolFailure(f"{name}: sdp did not finish within {seconds + 30}s")
        events = []
        for line in p.stdout.splitlines():
            try:
                v = json.loads(line)
            except ValueError:
                continue
            if isinstance(v, dict):
                events.append(v)
        self.record({"seq": self.seq, "name": name, "intent": intent, "argv": argv,
                     "exit": p.returncode, "events": events, "stderr": p.stderr})
        return events if p.returncode == 0 else None

    def record(self, entry: dict[str, Any]) -> None:
        """Append an entry to the raw observation log."""
        json.dump(entry, self.raw)
        self.raw.write("\n")
        self.raw.flush()


def replay(exe: Path, log: Path, timeout: float = 60.0) -> list[dict[str, Any]]:
    """Convert a packet log offline into the typed gpsprot event stream."""
    argv = [str(exe), "replay", str(log)]
    try:
        p = subprocess.run(argv, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        raise ToolFailure(f"replay {log.name}: no response within {timeout}s")
    if p.returncode != 0:
        raise ToolFailure(f"replay {log.name}: exit {p.returncode}: {p.stderr.strip()}")
    events = []
    for line in p.stdout.splitlines():
        try:
            v = json.loads(line)
        except ValueError:
            continue
        if isinstance(v, dict):
            events.append(v)
    return events
