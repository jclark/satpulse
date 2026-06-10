"""Step execution: satpulsetool gps invocation, timeout, JSON and packet-log collection."""

from __future__ import annotations

import json
import os
import subprocess
import time
from dataclasses import dataclass
from typing import IO, Any, Sequence

# A stuck invocation is killed after this many seconds and treated as a
# failure (and, in a later phase, as a wedge trigger).
STEP_TIMEOUT = 60.0


@dataclass
class StepResult:
    """One satpulsetool gps invocation: argv, exit code, parsed JSON, packet log."""

    argv: list[str]
    exit_code: int
    out_json: dict[str, Any] | None
    packet_log: str
    duration: float
    timed_out: bool
    stderr_tail: str

    def error(self) -> str | None:
        """The failure reason, or None when the step succeeded."""
        if self.timed_out:
            return f"timed out after {STEP_TIMEOUT:g}s"
        if self.out_json is None:
            return f"no JSON output (exit code {self.exit_code}): {self.stderr_tail}".strip()
        err = self.out_json.get("error")
        if isinstance(err, str) and err:
            return err
        if self.exit_code != 0:
            return f"exit code {self.exit_code}: {self.stderr_tail}".strip()
        return None

    def record(self) -> dict[str, Any]:
        """JSON-serializable form for results.jsonl."""
        return {
            "argv": self.argv,
            "exitCode": self.exit_code,
            "timedOut": self.timed_out,
            "durationSecs": round(self.duration, 3),
            "json": self.out_json,
            "stderrTail": self.stderr_tail,
            "packetLog": os.path.basename(self.packet_log),
        }


@dataclass(frozen=True)
class Baseline:
    """Session baseline recorded by the first probe of the run.

    Drives skip decisions (supports), comparison bounds (supported_gnss),
    and tolerance rules keyed on receiver family (vendor/hardware).
    """

    vendor: str
    hardware: str
    firmware: str
    supported_gnss: frozenset[str]
    supports: frozenset[str]
    packet_formats: tuple[str, ...]
    config: dict[str, Any]
    port: str | None
    baud: int | None


class Runner:
    """Runs satpulsetool gps invocations against one receiver.

    Every step gets --json and its own --packet-log file in the artifact
    directory, and is recorded in results.jsonl.
    """

    def __init__(
        self,
        satpulsetool: str,
        conn_args: Sequence[str],
        artifact_dir: str,
        vendor: str | None,
        verbose: bool,
    ) -> None:
        self.satpulsetool = satpulsetool
        self.conn_args = list(conn_args)
        self.artifact_dir = artifact_dir
        # Explicit --vendor, or the detected vendor after the baseline probe;
        # restricts which protocols later steps try.
        self.vendor = vendor
        self.verbose = verbose
        self.step_num = 0
        self.results: IO[str] = open(
            os.path.join(artifact_dir, "results.jsonl"), "w", encoding="utf-8"
        )

    def close(self) -> None:
        self.results.close()

    def record(self, entry: dict[str, Any]) -> None:
        json.dump(entry, self.results)
        self.results.write("\n")
        self.results.flush()

    def step(self, args: Sequence[str]) -> StepResult:
        """Run one satpulsetool gps invocation with --json and a fresh packet log."""
        self.step_num += 1
        packet_log = os.path.join(self.artifact_dir, f"step-{self.step_num:03d}.jsonl")
        argv = [self.satpulsetool, "gps", *self.conn_args]
        if self.vendor:
            argv += ["--vendor", self.vendor]
        argv += [*args, "--json", "--packet-log", packet_log]
        if self.verbose:
            print("+ " + " ".join(argv))
        start = time.monotonic()
        timed_out = False
        try:
            proc = subprocess.run(
                argv, capture_output=True, text=True, timeout=STEP_TIMEOUT
            )
            exit_code, stdout, stderr = proc.returncode, proc.stdout, proc.stderr
        except subprocess.TimeoutExpired as e:
            timed_out = True
            exit_code = -1
            stdout = e.stdout.decode(errors="replace") if isinstance(e.stdout, bytes) else (e.stdout or "")
            stderr = e.stderr.decode(errors="replace") if isinstance(e.stderr, bytes) else (e.stderr or "")
        duration = time.monotonic() - start
        out_json: dict[str, Any] | None = None
        try:
            v = json.loads(stdout)
            if isinstance(v, dict):
                out_json = v
        except ValueError:
            pass
        return StepResult(
            argv=argv,
            exit_code=exit_code,
            out_json=out_json,
            packet_log=packet_log,
            duration=duration,
            timed_out=timed_out,
            stderr_tail=stderr.strip().splitlines()[-1] if stderr.strip() else "",
        )

    def probe_baseline(self) -> tuple[Baseline | None, StepResult]:
        """Probe the receiver and record the session baseline."""
        sr = self.step(["--show-receiver", "--show-config", "--show-port"])
        if sr.error() is not None or sr.out_json is None:
            return None, sr
        j = sr.out_json
        rcvr = j.get("receiver") or {}
        config: dict[str, Any] = j.get("config") or {}
        bl = Baseline(
            vendor=str(rcvr.get("vendor", "")),
            hardware=str(rcvr.get("hardware", "")),
            firmware=str(rcvr.get("firmware", "")),
            supported_gnss=frozenset(str(g) for g in rcvr.get("supportedGNSS", [])),
            supports=frozenset(str(s) for s in j.get("supports", [])),
            packet_formats=tuple(str(f) for f in j.get("packetFormats", [])),
            config=config,
            port=str(config["port"]) if "port" in config else None,
            baud=int(config["baudRate"]) if "baudRate" in config else None,
        )
        if self.vendor is None and bl.vendor:
            self.vendor = bl.vendor
        return bl, sr
