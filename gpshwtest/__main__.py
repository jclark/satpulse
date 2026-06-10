"""Hardware tests for satpulsetool gps high-level configuration.

Points at a live receiver and verifies that requested configurations are
applied and read back consistently, shelling out to satpulsetool gps for
all receiver I/O. See plan/gps-config-hwtest.md (#310).
"""

from __future__ import annotations

import argparse
import os
import platform
import shutil
import sys
import tempfile
import time
from dataclasses import dataclass, field
from typing import Any

import cases
import compare
from runner import Baseline, Runner, StepResult

# Exit codes: 1 means one or more cases failed; 2 means the receiver
# could not be probed at all.
EXIT_FAIL = 1
EXIT_UNREACHABLE = 2


@dataclass
class CaseOutcome:
    case: cases.Case
    status: str  # PASS | FAIL | SKIP
    reason: str = ""
    mismatches: list[compare.Mismatch] = field(default_factory=list)
    steps: list[StepResult] = field(default_factory=list)


def find_satpulsetool() -> str | None:
    """Prefer the in-repo build, then PATH, then install paths."""
    here = os.path.dirname(os.path.abspath(__file__))
    m = platform.machine()
    arch = {"x86_64": "amd64", "aarch64": "arm64"}.get(m, m)
    repo_out = os.path.join(os.path.dirname(here), "out", arch, "satpulsetool")
    if os.access(repo_out, os.X_OK):
        return repo_out
    p = shutil.which("satpulsetool")
    if p:
        return p
    for d in ("/usr/local/bin", "/usr/bin"):
        c = os.path.join(d, "satpulsetool")
        if os.access(c, os.X_OK):
            return c
    return None


def parse_args(argv: list[str]) -> argparse.Namespace:
    ap = argparse.ArgumentParser(
        prog="gpshwtest",
        description="Hardware tests for satpulsetool gps high-level configuration",
    )
    ap.add_argument("-d", "--device", help="serial device connected to the GPS receiver")
    ap.add_argument("-s", "--speed", type=int, help="serial device speed in bps")
    ap.add_argument("-f", "--config-file", help="read device and speed from a satpulse TOML config file")
    ap.add_argument("--vendor", help="receiver vendor, passed through to satpulsetool to restrict probing")
    ap.add_argument("--satpulsetool", help="path to the satpulsetool binary")
    ap.add_argument("--artifact-dir", help="directory for packet logs and results.jsonl (default: temp dir)")
    ap.add_argument("--keep-artifacts", action="store_true", help="keep the artifact directory even when all cases pass")
    ap.add_argument("-v", "--verbose", action="store_true", help="print each satpulsetool invocation")
    groups = ap.add_argument_group("test groups")
    for key, _ in cases.GROUPS:
        groups.add_argument(f"--{key}", dest=f"group_{key.replace('-', '_')}", action="store_true",
                            help=f"run the {key} group")
    groups.add_argument("--all", action="store_true", help="run all test groups")
    args = ap.parse_args(argv)
    if args.config_file:
        if args.device or args.speed:
            ap.error("--config-file cannot be combined with --device or --speed")
    elif not (args.device and args.speed):
        ap.error("specify --device and --speed, or --config-file")
    if not args.all and not any(getattr(args, f"group_{k.replace('-', '_')}") for k, _ in cases.GROUPS):
        ap.error("no test groups specified; use group flags or --all")
    return args


def run_case(r: Runner, baseline: Baseline, case: cases.Case) -> CaseOutcome:
    missing = sorted(case.requires - baseline.supports)
    if missing:
        return CaseOutcome(case, "SKIP", reason="missing " + ",".join(missing))
    set_step = r.step(case.flags)
    steps = [set_step]
    err = set_step.error()
    if err is not None:
        return CaseOutcome(case, "FAIL", reason=f"set step: {err}", steps=steps)
    if case.settle > 0:
        time.sleep(case.settle)
    readback = r.step(["--show-config"])
    steps.append(readback)
    err = readback.error()
    if err is not None:
        return CaseOutcome(case, "FAIL", reason=f"readback step: {err}", steps=steps)
    config: dict[str, Any] = {}
    if readback.out_json is not None:
        v = readback.out_json.get("config")
        if isinstance(v, dict):
            config = v
    mismatches = compare.compare_case(case, baseline, config)
    return CaseOutcome(case, "FAIL" if mismatches else "PASS", mismatches=mismatches, steps=steps)


def record_outcome(r: Runner, group: str, oc: CaseOutcome) -> None:
    entry: dict[str, Any] = {
        "type": "case",
        "group": group,
        "case": oc.case.desc,
        "status": oc.status,
        "steps": [s.record() for s in oc.steps],
    }
    if oc.reason:
        entry["reason"] = oc.reason
    if oc.mismatches:
        entry["mismatches"] = [
            {"prop": m.prop, "expected": str(m.expected), "actual": str(m.actual), "note": m.note}
            for m in oc.mismatches
        ]
    r.record(entry)


def print_outcome(oc: CaseOutcome) -> None:
    if oc.status == "SKIP":
        print(f"SKIP {oc.case.desc} ({oc.reason})")
    elif oc.status == "PASS":
        print(f"PASS {oc.case.desc}")
    else:
        reason = f": {oc.reason}" if oc.reason else ""
        print(f"FAIL {oc.case.desc}{reason}")
        for m in oc.mismatches:
            print(f"  {m}")


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    tool = args.satpulsetool or find_satpulsetool()
    if tool is None:
        print("satpulsetool not found; build with make or use --satpulsetool", file=sys.stderr)
        return EXIT_UNREACHABLE
    if args.artifact_dir:
        artifact_dir = args.artifact_dir
        os.makedirs(artifact_dir, exist_ok=True)
        auto_dir = False
    else:
        artifact_dir = tempfile.mkdtemp(prefix="gpshwtest-")
        auto_dir = True
    conn = ["-f", args.config_file] if args.config_file else ["-d", args.device, "-s", str(args.speed)]
    r = Runner(tool, conn, artifact_dir, args.vendor, args.verbose)
    try:
        rc = run_battery(r, args)
    finally:
        r.close()
    if rc == 0 and auto_dir and not args.keep_artifacts:
        shutil.rmtree(artifact_dir)
    else:
        print(f"artifacts: {artifact_dir}")
    return rc


def run_battery(r: Runner, args: argparse.Namespace) -> int:
    baseline, sr = r.probe_baseline()
    if baseline is None:
        print(f"cannot probe receiver: {sr.error()}", file=sys.stderr)
        r.record({"type": "baseline", "error": sr.error(), "steps": [sr.record()]})
        return EXIT_UNREACHABLE
    r.record(
        {
            "type": "baseline",
            "vendor": baseline.vendor,
            "hardware": baseline.hardware,
            "firmware": baseline.firmware,
            "supportedGNSS": sorted(baseline.supported_gnss),
            "supports": sorted(baseline.supports),
            "packetFormats": list(baseline.packet_formats),
            "config": baseline.config,
            "steps": [sr.record()],
        }
    )
    print(f"receiver: {baseline.vendor} {baseline.hardware} ({baseline.firmware})")
    print(f"supports: {', '.join(sorted(baseline.supports)) or '(none)'}")
    if baseline.port is not None:
        speed = f" at {baseline.baud} bps" if baseline.baud else ""
        print(f"port: {baseline.port}{speed}")
    counts: dict[str, dict[str, int]] = {}
    failed: list[tuple[str, str]] = []
    for key, case_fn in cases.GROUPS:
        if not args.all and not getattr(args, f"group_{key.replace('-', '_')}"):
            continue
        print(f"=== {key}")
        n = counts[key] = {"PASS": 0, "FAIL": 0, "SKIP": 0}
        for case in case_fn(baseline):
            oc = run_case(r, baseline, case)
            n[oc.status] += 1
            if oc.status == "FAIL":
                failed.append((key, case.desc))
            print_outcome(oc)
            record_outcome(r, key, oc)
    total = {s: sum(n[s] for n in counts.values()) for s in ("PASS", "FAIL", "SKIP")}
    r.record({"type": "summary", "groups": counts, "passed": total["PASS"], "failed": total["FAIL"], "skipped": total["SKIP"]})
    for key, n in counts.items():
        print(f"{key}: {n['PASS']} passed, {n['FAIL']} failed, {n['SKIP']} skipped")
    print(f"summary: {total['PASS']} passed, {total['FAIL']} failed, {total['SKIP']} skipped")
    if failed:
        print("failed cases:")
        for key, desc in failed:
            print(f"  {key}: {desc}")
        return EXIT_FAIL
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
