"""Test GPS high-level configuration against real hardware.

See GOAL.md for what this program is for. It probes a receiver through
satpulsetool gps, recording every step with its intent, then derives the
verdicts offline from the records: tool-guarantee violations (failures)
and a characterization of how configuration is realized on the receiver
(data). The same offline analysis re-runs over any archived run directory
with --analyze, so improving the checks never requires re-running
hardware. Run in-repo as: python3 gpshwtest -d /dev/ttyACM0 -s 38400
Exit status: 0 clean; 1 the characterization differs from the baseline;
2 failures or errors.
"""

import argparse
import datetime
import difflib
import json
import shutil
import subprocess
import sys
import tomllib
from pathlib import Path
from typing import Any

from analyze import DISRUPTIVE_KEYS, analyze_run, load_steps
from characterize import to_json
from model import emissions, port_has_serial_speed
from probes import PROPS, ProbeRun
from tool import Invocation, Tool, ToolFailure


def main() -> int:
    ap = argparse.ArgumentParser(prog="gpshwtest", description=__doc__)
    ap.add_argument("-d", "--serial-device", help="serial device of the receiver")
    ap.add_argument("-s", "--device-speed", type=int, help="serial speed in bps")
    ap.add_argument("-f", "--config-file", help="read device and speed from a satpulse.toml")
    ap.add_argument("--analyze", type=Path, metavar="LOGDIR",
                    help="re-analyze a recorded log directory offline, "
                         "without touching hardware")
    ap.add_argument("--satpulsetool", type=Path, default=Path("satpulsetool"),
                    help="path to the satpulsetool binary (default: from PATH)")
    ap.add_argument("--logdir", type=Path,
                    help="parent directory for per-run log directories "
                         "(default: /tmp, with the directory removed on a clean exit)")
    ap.add_argument("--keep-logdir", action="store_true",
                    help="keep the log directory even on a clean exit "
                         "(implied when --logdir is given)")
    ap.add_argument("--baseline", type=Path,
                    help="characterization to compare against "
                         "(default: no comparison)")
    ap.add_argument("--restore-from", type=Path, metavar="LOGDIR",
                    help="run only the restore tail derived from a crashed "
                         "run's records, then verify the receiver state")
    ap.add_argument("--disruptive", action="store_true",
                    help="also run the probes that write NVM and reboot the "
                         "receiver (--save, --save-all, --reset), with recovery")
    ap.add_argument("--sudo", action="store_true",
                    help="use sudo -n for physical time pulse checks (needs root)")
    ap.add_argument("--phc", help="PHC pin the receiver's PPS is wired to, as "
                                  "iface:pin[:chan] (default: the [phc] table of the "
                                  "config file, or /etc/satpulse.toml)")
    ap.add_argument("--rtcm-fixed-pos-ecef", type=ecef_arg,
                    default="-1144697.93,6090335.51,1504171.28",
                    help="ECEF fixed position used while probing RTCM output "
                         "(default: %(default)s)")
    args = ap.parse_args()
    exe = args.satpulsetool
    if args.analyze:
        return report(args.analyze, exe, args.baseline)
    conn = conn_args(args)
    keep = args.keep_logdir or args.logdir is not None
    stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    log_dir = (args.logdir or Path("/tmp")) / f"gpshwtest-{stamp}-{device_slug(args)}"
    tool = Tool(exe, conn, log_dir)
    print(f"log directory: {log_dir}", file=sys.stderr)
    if args.restore_from:
        restore_from(tool, args.restore_from)
        a = analyze_run(log_dir, exe)
        for f in a.failures:
            print(f"FAILURE: {f}", file=sys.stderr)
        if not a.failures:
            print("receiver restored to the crashed run's as-found state",
                  file=sys.stderr)
        status = 2 if a.failures else 0
    else:
        status = 0
        try:
            drive(tool, resolve_phc(args), args.sudo, args.disruptive,
                  args.rtcm_fixed_pos_ecef)
        except ToolFailure as e:
            print(f"FAILURE: {e}", file=sys.stderr)
            status = 2
        status = max(status, report(log_dir, exe, args.baseline))
    if status == 0 and not keep:
        shutil.rmtree(log_dir, ignore_errors=True)
    return status


def device_slug(args: argparse.Namespace) -> str:
    """A run-directory name component identifying the receiver, so
    concurrent runs on different receivers cannot collide."""
    if args.serial_device:
        return Path(args.serial_device).name
    return Path(args.config_file).stem


def conn_args(args: argparse.Namespace) -> list[str]:
    if args.config_file:
        return ["-f", args.config_file]
    if not args.serial_device:
        raise SystemExit("specify -d/--serial-device (with -s) or -f/--config-file")
    conn = ["-d", args.serial_device]
    if args.device_speed:
        conn += ["-s", str(args.device_speed)]
    return conn


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


def ecef_arg(s: str) -> str:
    parts = s.split(",")
    if len(parts) != 3:
        raise argparse.ArgumentTypeError("must be X,Y,Z")
    try:
        for p in parts:
            float(p)
    except ValueError as e:
        raise argparse.ArgumentTypeError("must contain numeric X,Y,Z") from e
    return s


def sudo_ok() -> bool:
    """Whether sudo -n works without a password right now."""
    try:
        return subprocess.run(["sudo", "-n", "true"], capture_output=True,
                              timeout=10).returncode == 0
    except (OSError, subprocess.TimeoutExpired):
        return False


def restore_from(tool: Tool, crashed: Path) -> None:
    """Run only the restore tail, driven by a crashed run's records: its
    initial readback defines the target state, its baseline observation the
    message output to restore. This is the recovery path for runs that died
    without their in-process tail (kill -9, power loss)."""
    steps = load_steps(crashed)
    initial = None
    base = None
    port_cfg: dict[str, Any] = {}
    as_found_speed = None
    for s in steps:
        op, role = s.intent.get("op"), s.intent.get("role")
        if s.error is not None:
            continue
        if op == "config" and role == "initial" and initial is None:
            initial = s.config()
        elif op == "observe" and role == "baseline" and s.log is not None:
            base = emissions(s.log)
        elif op == "show-port":
            port_cfg = s.config()
        elif op == "session-speed" and role == "raise":
            as_found_speed = s.intent.get("from")
    if initial is None:
        raise SystemExit(f"{crashed}: no initial configuration recorded; "
                         "nothing to restore from")
    pr = ProbeRun(tool)
    ident = tool.gps("show-receiver", ["--show-receiver"], {"op": "identify"})
    if ident.error is not None:
        if pr.rediscover_speed() is None:
            return
        tool.gps("show-receiver", ["--show-receiver"], {"op": "identify"})
    pr.emergency_restore(initial, base, port_has_serial_speed(port_cfg))
    if isinstance(as_found_speed, int):
        pr.session_speed_restore(as_found_speed)


def drive(tool: Tool, phc: tuple[str, int, int] | None, use_sudo: bool,
          disruptive: bool, rtcm_fixed_pos_ecef: str) -> None:
    """Execute the probe sequence, recording every step. No verdicts here:
    the records are analyzed offline afterwards (also on a live run)."""
    pr = ProbeRun(tool)
    ident = identify_receiver(tool, pr)
    if ident is None:
        return
    receiver = ident.out.get("receiver", {})
    supports = ident.out.get("supports") or []
    print(f"receiver: {receiver.get('vendor')} {receiver.get('hardware')} "
          f"{receiver.get('firmware')}", file=sys.stderr)
    initial = pr.show_config("initial-config", "initial")
    if initial is None:
        return
    port_cfg = check_show_port(tool, pr)
    as_found_speed = pr.session_speed_raise(port_cfg, supports, receiver)
    if as_found_speed is not None:
        print(f"session speed raised from {as_found_speed}", file=sys.stderr)
    base = None
    done = False
    try:
        for p in PROPS:
            print(f"probing {p.name}", file=sys.stderr)
            pr.probe_scalar(p, initial)
        print("probing positioning mode", file=sys.stderr)
        pr.probe_modes(initial)
        if phc is not None and use_sudo and sudo_ok():
            print(f"checking physical time pulse on {phc[0]} pin {phc[1]}", file=sys.stderr)
            pr.probe_pulse_physical(initial, phc, True)
        else:
            print("skipping physical time pulse checks (need --sudo, passwordless "
                  "sudo -n, and PHC wiring)", file=sys.stderr)
        supported = receiver.get("supportedGNSS")
        if not supported:
            # The backend deduced no supported set (empty on the UM980); the
            # constellations enabled in the as-found configuration are a
            # discovered lower bound to probe instead.
            supported = sorted(initial.get("signalsEnabled") or {})
        if isinstance(supported, list) and supported:
            print("probing signal combinations", file=sys.stderr)
            pr.probe_signals(initial, supported)
        # Message output last: raw output can saturate the link beyond
        # in-band recovery on some receivers (HW/um980.md), so the probes
        # that can wedge the session come after everything else.
        print("probing message output", file=sys.stderr)
        fixed = rtcm_fixed_pos_ecef if "fixedPos" in supports else None
        base = pr.probe_messages(initial, fixed, receiver)
        print("probing reload", file=sys.stderr)
        # Serial links get speed rediscovery after each reload, since NVM
        # may hold a different baud rate. Native USB has no baud rate.
        uart = port_has_serial_speed(port_cfg)
        raised = as_found_speed is not None
        nvm = pr.probe_reload(initial, base, uart=uart, raised=raised)
        if disruptive:
            if nvm is None:
                print("skipping disruptive probes: NVM state unknown "
                      "(reload readback failed)", file=sys.stderr)
            else:
                print("running disruptive NVM probes", file=sys.stderr)
                pr.probe_disruptive(initial, nvm, base, uart, as_found_speed,
                                    "speed" in supports)
        pr.show_config("final-config", "final")
        done = True
    finally:
        if not done:
            # The run is aborting (tool failure, interrupt, crash): restore
            # the receiver best-effort, recorded like everything else.
            print("run aborted; restoring the receiver", file=sys.stderr)
            pr.emergency_restore(initial, base, port_has_serial_speed(port_cfg))
        if as_found_speed is not None:
            pr.session_speed_restore(as_found_speed)


def identify_receiver(tool: Tool, pr: ProbeRun) -> Invocation | None:
    intent = {"op": "identify"}
    ident = tool.gps("show-receiver", ["--show-receiver"], intent)
    if ident.error is not None:
        # satpulsetool does not scan baud rates, so a UART resting at the
        # wrong speed (a crashed run, another program) looks like a dead
        # receiver. Rediscover the speed and identify again.
        if pr.rediscover_speed() is not None:
            ident = tool.gps("show-receiver", ["--show-receiver"], intent)
        if ident.error is not None:
            return None
    return ident


def check_show_port(tool: Tool, pr: ProbeRun) -> dict[str, Any]:
    """Probe --show-port and return its config. The port fields appear only
    with --show-port, so there is no readback to cross-check. On a UART
    connection with no speed given, the reported speed is locked in for
    the rest of the run, saving a baud scan per invocation."""
    inv = tool.gps("show-port", ["--show-port"], {"op": "show-port"})
    cfg = inv.config()
    baud = cfg.get("baudRate")
    if "-s" not in tool.conn and port_has_serial_speed(cfg):
        tool.set_speed(baud)
    return cfg


def report(log_dir: Path, exe: Path, baseline: Path | None) -> int:
    """Analyze a log directory and report: write and print the
    characterization, print the failures, compare against the baseline
    when one was given."""
    a = analyze_run(log_dir, exe)
    print(f"receiver: {a.receiver.get('vendor')} {a.receiver.get('hardware')} "
          f"{a.receiver.get('firmware')}", file=sys.stderr)
    text = to_json(a.characterization)
    (log_dir / "characterization.json").write_text(text)
    sys.stdout.write(text)
    status = 0
    for f in a.failures:
        print(f"FAILURE: {f}", file=sys.stderr)
        status = 2
    if not a.failures:
        print(f"ok: {a.observation_count} observations, no failures", file=sys.stderr)
    if baseline is None:
        return status
    return max(status, compare_baseline(baseline, text, a.disruptive))


def compare_baseline(baseline: Path, text: str, disruptive: bool) -> int:
    """Compare against the checked-in characterization; differences are
    regressions to investigate. The baseline holds the full characterization
    from a disruptive run; a default run is compared with the disruptive-only
    entries stripped.

    Defect entries are unstable by nature - a receiver's ACK-without-apply
    incidence drifts between sessions - so they are compared by content
    subset rather than exactly: a defect property absent from the run does not
    diff (drift down is allowed), and neither does a run whose observations
    are all recorded in the baseline. What diffs is novel receiver behavior -
    a defect property the baseline never recorded, or a stuck value or request
    shape the baseline's entry for that property does not contain. The rest of
    the characterization, the stable core, must match exactly."""
    want_doc = json.loads(baseline.read_text())
    run_doc = json.loads(text)
    if not disruptive:
        for k in DISRUPTIVE_KEYS:
            want_doc.get("limitations", {}).pop(k, None)
    want_defects = want_doc.pop("defects", {})
    run_defects = run_doc.pop("defects", {})
    new_defects = sorted(set(run_defects) - set(want_defects))
    novel = []
    for p in sorted(set(run_defects) & set(want_defects)):
        want_obs = want_defects[p].get("acceptedButNotApplied", [])
        extra = [o for o in run_defects[p].get("acceptedButNotApplied", [])
                 if o not in want_obs]
        if extra:
            novel.append((p, extra))
    want, core = to_json(want_doc), to_json(run_doc)
    if want == core and not new_defects and not novel:
        print(f"matches baseline {baseline}", file=sys.stderr)
        return 0
    if want != core:
        sys.stderr.writelines(difflib.unified_diff(
            want.splitlines(keepends=True), core.splitlines(keepends=True),
            fromfile=str(baseline), tofile="this run"))
    for p in new_defects:
        print(f"new receiver defect not in baseline: {p}: "
              f"{json.dumps(run_defects[p], sort_keys=True)}", file=sys.stderr)
    for p, extra in novel:
        print(f"receiver defect {p} has observations not in baseline: "
              f"{json.dumps(extra, sort_keys=True)}", file=sys.stderr)
    print("characterization differs from baseline", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
