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
import subprocess
import sys
import tomllib
from pathlib import Path
from typing import Any

from characterize import characterize, to_json
from probes import PROPS, Observation, ProbeRun
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
    ap.add_argument("--sudo", action="store_true",
                    help="use sudo -n for physical time pulse checks (needs root)")
    ap.add_argument("--phc", help="PHC pin the receiver's PPS is wired to, as "
                                  "iface:pin[:chan] (default: the [phc] table of the "
                                  "config file, or /etc/satpulse.toml)")
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
        return run(tool, args.baseline, resolve_phc(args), args.sudo)
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


def resolve_phc(args: argparse.Namespace) -> tuple[str, int, int] | None:
    """Find where the receiver's PPS is wired: the --phc argument, or the
    [phc] table of the connection config file or /etc/satpulse.toml."""
    if args.phc:
        iface, _, rest = args.phc.partition(":")
        pin, _, chan = rest.partition(":")
        if not pin:
            raise SystemExit("--phc must be iface:pin[:chan]")
        return iface, int(pin), int(chan) if chan else 0
    for path in (args.config_file, "/etc/satpulse.toml"):
        if not path:
            continue
        try:
            with open(path, "rb") as f:
                phc = tomllib.load(f).get("phc")
        except (OSError, tomllib.TOMLDecodeError):
            continue
        if isinstance(phc, dict) and "interface" in phc and "pin" in phc:
            return str(phc["interface"]), int(phc["pin"]), int(phc.get("channel", 0))
    return None


def sudo_ok() -> bool:
    """Whether sudo -n works without a password right now."""
    try:
        return subprocess.run(["sudo", "-n", "true"], capture_output=True,
                              timeout=10).returncode == 0
    except (OSError, subprocess.TimeoutExpired):
        return False


def running_satpulsed() -> bool:
    for comm in Path("/proc").glob("[0-9]*/comm"):
        try:
            if comm.read_text().strip() == "satpulsed":
                return True
        except OSError:
            pass
    return False


def run(tool: Tool, baseline: Path | None, phc: tuple[str, int, int] | None,
        use_sudo: bool) -> int:
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
    if initial is None:
        print("FAILURE: cannot read initial configuration", file=sys.stderr)
        return 1
    check_show_port(tool, pr)
    for p in PROPS:
        print(f"probing {p.name}", file=sys.stderr)
        pr.probe_scalar(p, initial)
    print("probing positioning mode", file=sys.stderr)
    pr.probe_modes(initial)
    print("probing message output", file=sys.stderr)
    pr.probe_messages()
    if phc is not None and use_sudo and sudo_ok():
        print(f"checking physical time pulse on {phc[0]} pin {phc[1]}", file=sys.stderr)
        pr.probe_pulse_physical(initial, phc, True)
    else:
        print("skipping physical time pulse checks (need --sudo, passwordless "
              "sudo -n, and PHC wiring)", file=sys.stderr)
    supported = receiver.get("supportedGNSS")
    if isinstance(supported, list):
        print("probing signal combinations", file=sys.stderr)
        pr.probe_signals(initial, supported)
    final = pr.show_config("final-config")
    if final is not None and final != initial:
        pr.failures.append(f"receiver not left as found: initial {initial!r}, final {final!r}")
    enabled = sorted(initial.get("signalsEnabled") or {})
    doc = characterize(receiver, supports, pr, enabled)
    text = to_json(doc)
    (tool.run_dir / "characterization.json").write_text(text)
    sys.stdout.write(text)
    status = 0
    for f in pr.failures:
        print(f"FAILURE: {f}", file=sys.stderr)
        status = 1
    if not pr.failures:
        n = (len(pr.observations) + len(pr.signal_observations)
             + len(pr.emission_observations))
        print(f"ok: {n} observations, no failures", file=sys.stderr)
    return max(status, compare_baseline(receiver, baseline, text))


def check_show_port(tool: Tool, pr: ProbeRun) -> None:
    """Check that --show-port responds. The port fields appear only with
    --show-port, so there is no readback to cross-check. Port is a
    read-only property: a backend whose protocol cannot identify the
    active port reports no port at all, which is the property's
    nonexistence - a limitation, not a failure (CASIC receivers answer a
    port query with one entry per UART and nothing marks the one in
    use). On a UART connection with no speed given, a reported speed is
    locked in for the rest of the run, saving a baud scan per
    invocation."""
    inv = tool.gps("show-port", ["--show-port"])
    if inv.error is not None:
        pr.failures.append(f"--show-port failed: {inv.error}")
        return
    if not inv.config().get("port"):
        pr.observations.append(Observation(
            prop="port", requested=None, error=None, reported=None, readback=None))
    baud = inv.config().get("baudRate")
    if "-s" not in tool.conn and isinstance(baud, int) and baud > 0:
        tool.conn += ["-s", str(baud)]


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
