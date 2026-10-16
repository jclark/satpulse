#!/usr/bin/env python3
"""SatPulse PHC integration checks; run only inside a disposable KVM guest."""
from __future__ import annotations

import argparse
from decimal import Decimal
import fcntl
import json
import os
from pathlib import Path
import select
import signal
import struct
import subprocess
import tempfile
import time
from typing import Sequence

SAT = Path(__file__).resolve().parent.parent
TOOL = SAT / "out/amd64/satpulsetool"
DAEMON = SAT / "out/amd64/satpulsed"
PERSONALITY = SAT / "gps/app/ubxsim/testdata/f9p/f9p-personality.ubx"
MODULE = Path("/root/virt-phc-extts/module/virt_phc_extts.ko")
RESULTS = Path("/root/satpulse-results")
EDGE = Path("/sys/devices/platform/virt-phc-extts/extts_schedule")
DRIFT = 10000


def run(args: Sequence[str | Path], *, capture_output: bool = False) -> subprocess.CompletedProcess[str]:
    return subprocess.run([str(a) for a in args], check=True, text=True,
                          capture_output=capture_output)


def find_phc(name: str) -> Path:
    for p in Path("/sys/class/ptp").glob("ptp*"):
        if (p / "clock_name").read_text().strip() == name:
            return Path("/dev") / p.name
    raise RuntimeError(f"PHC {name!r} not found")


def load(no_cross: bool = False, drift: int = DRIFT) -> Path:
    run(["insmod", MODULE, f"freq_error_ppb={drift}",
         "initial_offset_ns=10000000000", f"no_crosststamp={int(no_cross)}"])
    try:
        run(["ip", "link", "set", "vphc0", "up"])
        return find_phc("virt-phc-extts")
    except BaseException:
        run(["rmmod", "virt_phc_extts"])
        raise


def cross(path: Path) -> tuple[int, int]:
    # Linux ptp_sys_offset_precise: three ptp_clock_time structures and
    # four reserved u32s. All fields are integer nanoseconds until differenced.
    with open(path, "rb", buffering=0) as f:
        b = bytearray(64)
        fcntl.ioctl(f, (3 << 30) | (64 << 16) | (ord("=") << 8) | 8, b)
    ts = [sec * 1_000_000_000 + ns for sec, ns, _ in
          struct.iter_unpack("=qII", b[:48])]
    return int(ts[0]), int(ts[2])


def discovery_and_sdp() -> None:
    load()
    try:
        data = run([TOOL, "sdp", "-j"], capture_output=True).stdout
        interfaces = [json.loads(s) for s in data.splitlines() if s.strip()]
        iface = next(x for x in interfaces if x["name"] == "vphc0")
        assert iface["pins"] == ["SDP0"] and iface["n_extts_channels"] == 1, iface
        assert iface["n_perout_channels"] == 0, iface
        p = subprocess.Popen([str(TOOL), "sdp", "-i", "-j", "-t", "3", "vphc0"],
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            # Poll pin assignment, then submit far enough ahead for enablement.
            pin = Path(f"/sys/class/ptp/ptp{iface['clock_index']}/pins/SDP0")
            end = time.monotonic() + 2
            while time.monotonic() < end:
                if pin.read_text().split()[0] == "1":
                    break
                if p.poll() is not None:
                    raise RuntimeError(p.communicate())
                time.sleep(0.01)
            else:
                raise RuntimeError("sdp did not assign pin")
            edge = time.clock_gettime_ns(time.CLOCK_MONOTONIC_RAW) + 300_000_000
            EDGE.write_text(f"{edge} {edge + 1_000_000_000}\n")
            out, err = p.communicate(timeout=6)
            assert p.returncode == 0, (out, err)
            events = [json.loads(s) for s in out.splitlines() if s.strip()]
            assert len(events) == 2, (events, err)
            assert all(x["chan"] == 0 for x in events), events
            interval = (Decimal(events[1]["timestamp"]) -
                        Decimal(events[0]["timestamp"])) * 1_000_000_000
            assert abs(interval - (1_000_000_000 + DRIFT)) <= 1, interval
            print("SDP events:", json.dumps(events), flush=True)
        finally:
            if p.poll() is None:
                p.kill()
                p.wait()
    finally:
        run(["rmmod", "virt_phc_extts"])
    print("PASS discovery and sdp", flush=True)


def stop(p: subprocess.Popen[str] | None) -> None:
    if p is None:
        return
    if p.poll() is None:
        p.send_signal(signal.SIGINT)
        try:
            p.wait(timeout=8)
        except subprocess.TimeoutExpired:
            p.kill()
            p.wait()
            raise RuntimeError("SIGINT shutdown hung")
    assert p.returncode == 0, f"process exited {p.returncode}"


def sync_check(name: str, wrong: bool = False, no_cross: bool = False) -> None:
    ref = find_phc("KVM virtual PTP")
    work = Path(tempfile.mkdtemp(prefix=f"vphc-{name}-", dir=RESULTS))
    print(f"START {name}: logs {work}", flush=True)
    run(["timedatectl", "set-ntp", "false"])
    if wrong:
        time.clock_settime(time.CLOCK_REALTIME, 1577836800)
    sim = daemon = None
    loaded = False
    try:
        phc = load(no_cross)
        loaded = True
        (work / "satpulse.toml").write_text(f'''[serial]
device = "{work}/receiver"
speed = 115200
[gps]
config = true
vendor = "u-blox"
[phc]
interface = "vphc0"
[log]
clock = true
packet = true
dir = "{work}"
''')
        with open(work / "simulator.log", "w") as slog, open(work / "daemon.log", "w") as dlog:
            sim = subprocess.Popen([str(TOOL), "-v", "ubxsim", "--timing", "--phc",
                                    str(ref), "--edge", str(EDGE), "--link", str(work / "receiver"),
                                    str(PERSONALITY)], stdout=slog, stderr=slog, text=True)
            end = time.monotonic() + 10
            while not (work / "receiver").exists():
                assert sim.poll() is None, "simulator exited before PTY appeared"
                if time.monotonic() > end:
                    raise RuntimeError("simulator PTY did not appear")
                time.sleep(0.05)
            daemon = subprocess.Popen([str(DAEMON), "-v", "-f", str(work / "satpulse.toml")],
                                      stdout=dlog, stderr=dlog, text=True)
            start = cross(ref)
            end = time.monotonic() + 150
            samples: list[list[str]] = []
            clocklog = work / "clock.vphc0.log"
            while time.monotonic() < end:
                assert sim.poll() is None, "simulator exited"
                assert daemon.poll() is None, "satpulsed exited"
                if clocklog.exists():
                    samples = [s.split() for s in clocklog.read_text().splitlines()
                               if s and not s.startswith("#")]
                    tail = samples[-10:]
                    if len(tail) == 10 and all(s[6] == "1" and s[4] == "0" and
                                              abs(int(s[2])) <= 100 for s in tail):
                        break
                time.sleep(0.2)
            else:
                raise RuntimeError(f"did not sustain tracking: last samples {samples[-10:]}")
            finish = cross(ref)
            rate = ((finish[0] - start[0]) / (finish[1] - start[1]) - 1) * 1e9
            freq = sum(float(s[3]) for s in tail) / len(tail)
            expected = rate - DRIFT
            assert abs(freq - expected) < 100, (freq, expected, rate)
            stop(daemon)
            daemon = None
            # An independent reader must see no new event after the daemon's
            # shutdown, while the simulator continues scheduling pulses.
            fd = os.open(phc, os.O_RDONLY | os.O_NONBLOCK)
            try:
                poll = select.poll()
                poll.register(fd, select.POLLIN)
                EDGE.write_text(f"{time.clock_gettime_ns(time.CLOCK_MONOTONIC_RAW) + 200_000_000}\n")
                assert not poll.poll(1500), "EXTTS still enabled after shutdown"
            finally:
                os.close(fd)
            stop(sim)
            sim = None
            if wrong:
                assert time.time() < 1600000000, "guest clock was corrected unexpectedly"
            print(f"PASS {name}: max offset {max(abs(int(s[2])) for s in tail)} ns, "
                  f"frequency {freq:.1f} ppb, expected {expected:.1f} ppb "
                  f"(reference/raw {rate:.1f} ppb), clean shutdown", flush=True)
    finally:
        try:
            stop(daemon)
        finally:
            try:
                stop(sim)
            finally:
                if loaded:
                    run(["rmmod", "virt_phc_extts"])
                if wrong:
                    time.clock_settime(time.CLOCK_REALTIME, cross(ref)[0] / 1e9)


def main() -> None:
    global MODULE, RESULTS
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("scenario", nargs="?", default="all",
                   choices=["all", "sdp", "normal", "wrong-clock", "extended"])
    p.add_argument("--module", type=Path, default=MODULE,
                   help="virtual PHC module built for the running guest kernel")
    p.add_argument("--results", type=Path, default=RESULTS,
                   help="directory in which to preserve integration logs")
    args = p.parse_args()
    # This harness changes the guest clock and loads an experimental module.
    assert Path("/etc/hostname").read_text().strip() == "vphc-test", "expected disposable test guest"
    assert Path("/root/guest-ready").exists(), "guest provisioning did not finish"
    MODULE, RESULTS = args.module, args.results
    RESULTS.mkdir(parents=True, exist_ok=True)
    run(["modprobe", "ptp_kvm"])
    if args.scenario in ("all", "sdp"):
        discovery_and_sdp()
    for name, wrong, no_cross in [("normal", False, False), ("wrong-clock", True, False),
                                   ("extended", False, True)]:
        if args.scenario in ("all", name):
            sync_check(name, wrong, no_cross)
    print("RESULT: PASS", flush=True)


if __name__ == "__main__":
    main()
