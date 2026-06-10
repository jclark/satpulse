"""Test GPS high-level configuration against real hardware.

See GOAL.md for what this program is for. It probes a receiver through
satpulsetool gps, verifies the device-independent tool guarantees (failures),
and emits a characterization of how configuration is realized on the
receiver (data). Run in-repo as: python3 gpshwtest -d /dev/ttyACM0 -s 38400
"""

import argparse
import datetime
import difflib
import platform
import sys
from pathlib import Path
from typing import Any

from characterize import characterize, to_json
from probes import PROPS, ProbeRun
from tool import Tool, ToolFailure

HERE = Path(__file__).resolve().parent


def main() -> int:
    ap = argparse.ArgumentParser(prog="gpshwtest", description=__doc__)
    ap.add_argument("-d", "--serial-device", help="serial device of the receiver")
    ap.add_argument("-s", "--device-speed", type=int, help="serial speed in bps")
    ap.add_argument("-f", "--config-file", help="read device and speed from a satpulse.toml")
    ap.add_argument("--satpulsetool", type=Path, help="path to the satpulsetool binary")
    ap.add_argument("--runs", type=Path, default=HERE / "runs",
                    help="directory for run artifacts (default: gpshwtest/runs)")
    ap.add_argument("--baseline", type=Path,
                    help="characterization to compare against "
                         "(default: gpshwtest/baselines/<receiver>.json if present)")
    args = ap.parse_args()
    if running_satpulsed():
        print("satpulsed is running; stop it before touching the receiver", file=sys.stderr)
        return 1
    conn = conn_args(args)
    exe = args.satpulsetool if args.satpulsetool else find_satpulsetool()
    run_dir = args.runs / datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    tool = Tool(exe, conn, run_dir)
    print(f"run artifacts: {run_dir}", file=sys.stderr)
    try:
        return run(tool, args.baseline)
    except ToolFailure as e:
        print(f"FAILURE: {e}", file=sys.stderr)
        return 1


def conn_args(args: argparse.Namespace) -> list[str]:
    if args.config_file:
        return ["-f", args.config_file]
    if not args.serial_device:
        raise SystemExit("specify -d/--serial-device (with -s) or -f/--config-file")
    conn = ["-d", args.serial_device]
    if args.device_speed:
        conn += ["-s", str(args.device_speed)]
    return conn


def find_satpulsetool() -> Path:
    arch = {"x86_64": "amd64", "aarch64": "arm64"}.get(platform.machine(), platform.machine())
    p = HERE.parent / "out" / arch / "satpulsetool"
    if p.exists():
        return p
    raise SystemExit(f"satpulsetool not found at {p}; build with make or pass --satpulsetool")


def running_satpulsed() -> bool:
    for comm in Path("/proc").glob("[0-9]*/comm"):
        try:
            if comm.read_text().strip() == "satpulsed":
                return True
        except OSError:
            pass
    return False


def run(tool: Tool, baseline: Path | None) -> int:
    ident = tool.gps("show-receiver", ["--show-receiver"])
    if ident.error is not None:
        print(f"FAILURE: receiver detection failed: {ident.error}", file=sys.stderr)
        return 1
    receiver = ident.out.get("receiver", {})
    supports = ident.out.get("supports", [])
    print(f"receiver: {receiver.get('vendor')} {receiver.get('hardware')} "
          f"{receiver.get('firmware')}", file=sys.stderr)
    pr = ProbeRun(tool)
    initial = pr.show_config("initial-config")
    check_show_port(tool, pr)
    for p in PROPS:
        print(f"probing {p.name}", file=sys.stderr)
        pr.probe_scalar(p, initial)
    print("probing positioning mode", file=sys.stderr)
    pr.probe_modes(initial)
    final = pr.show_config("final-config")
    if final != initial:
        pr.failures.append(f"receiver not left as found: initial {initial!r}, final {final!r}")
    doc = characterize(receiver, supports, pr.observations)
    text = to_json(doc)
    (tool.run_dir / "characterization.json").write_text(text)
    sys.stdout.write(text)
    status = 0
    for f in pr.failures:
        print(f"FAILURE: {f}", file=sys.stderr)
        status = 1
    if not pr.failures:
        print(f"ok: {len(pr.observations)} observations, no failures", file=sys.stderr)
    return max(status, compare_baseline(receiver, baseline, text))


def check_show_port(tool: Tool, pr: ProbeRun) -> None:
    """Check that --show-port responds and reports a port. The port fields
    appear only with --show-port, so there is no readback to cross-check."""
    inv = tool.gps("show-port", ["--show-port"])
    if inv.error is not None:
        pr.failures.append(f"--show-port failed: {inv.error}")
    elif not inv.config().get("port"):
        pr.failures.append(f"--show-port reported no port: {inv.config()!r}")


def compare_baseline(receiver: dict[str, Any], baseline: Path | None, text: str) -> int:
    """Compare against the checked-in characterization; differences are
    regressions to investigate. Absence of a baseline is not a failure."""
    if baseline is None:
        slug = "-".join(str(receiver.get(k, "")) for k in ("hardware", "firmware"))
        baseline = HERE / "baselines" / (slug.replace(" ", "-").replace("/", "-") + ".json")
        if not baseline.exists():
            print(f"no baseline at {baseline}; vet and check in the characterization",
                  file=sys.stderr)
            return 0
    want = baseline.read_text()
    if want == text:
        print(f"matches baseline {baseline}", file=sys.stderr)
        return 0
    sys.stderr.writelines(difflib.unified_diff(
        want.splitlines(keepends=True), text.splitlines(keepends=True),
        fromfile=str(baseline), tofile="this run"))
    print("FAILURE: characterization differs from baseline", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
