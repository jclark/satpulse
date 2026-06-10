"""Wrapper for satpulsetool gps invocations.

All receiver I/O goes through here: each invocation runs satpulsetool gps
with --json and a per-invocation packet log, and is recorded verbatim in
raw.jsonl in the run directory.
"""

import json
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any


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

    def gps(self, name: str, args: list[str], timeout: float = 90.0) -> Invocation:
        """Run satpulsetool gps with the given high-level args plus --json
        and a per-invocation packet log. Raises ToolFailure on timeout or
        on success without JSON output; a configuration error is not a
        failure and is reported through Invocation.error.

        Detection of a receiver whose periodic output is all disabled is
        intermittent (observed on a ZED-F9P after NMEA output was turned
        off), so a detection failure is retried once; the flake stays
        visible in raw.jsonl and the packet logs."""
        inv = self.gps_once(name, args, timeout)
        if inv.error is not None and "detection failed" in inv.error:
            time.sleep(2.0)
            inv = self.gps_once(f"{name}-retry", args, timeout)
        return inv

    def gps_once(self, name: str, args: list[str], timeout: float) -> Invocation:
        self.seq += 1
        log = self.run_dir / f"{self.seq:03d}-{name}.jsonl"
        argv = [str(self.exe), "gps", *self.conn, "--json", "--packet-log", str(log), *args]
        try:
            p = subprocess.run(argv, capture_output=True, text=True, timeout=timeout)
        except subprocess.TimeoutExpired:
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
        self.record({"seq": self.seq, "name": name, "argv": argv, "exit": p.returncode,
                     "json": out if out else p.stdout, "stderr": p.stderr})
        if p.returncode == 0 and not out:
            raise ToolFailure(f"{name}: exit 0 but no JSON output")
        return inv

    def record(self, entry: dict[str, Any]) -> None:
        """Append an entry to the raw observation log."""
        json.dump(entry, self.raw)
        self.raw.write("\n")
        self.raw.flush()
