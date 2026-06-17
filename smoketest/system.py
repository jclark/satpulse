#!/usr/bin/env python3
"""System smoke test for the installed satpulse systemd service.

This runner exercises the package-installed satpulse@.service and binaries in
PATH. It creates /dev/fakegps as a FIFO, installs a rich config at
/etc/satpulse.d/fakegps.toml, starts satpulse@fakegps.service, replays a packet
log through the FIFO, and then checks the externally visible services.
"""

from __future__ import annotations

import argparse
import errno
import fcntl
import json
import os
import re
import shutil
import socket
import stat
import subprocess
import sys
import tempfile
import threading
import time
import traceback
from typing import IO, Callable, Sequence, TypedDict, cast

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
REPO = os.path.dirname(HERE)

import common  # noqa: E402
from scenarios import ntrip, ntp, proxy, stream  # noqa: E402

DEFAULT_INSTANCE = "fakegps"
DEFAULT_PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon-msm4-115200.jsonl"
DEFAULT_PORT_BASE = 22000
PORT_BLOCK = 16
PORT_OFFSETS = {
    "SATPULSE_TEST_HTTP_PORT": 0,
    "SATPULSE_TEST_NTRIP_PORT": 1,
    "SATPULSE_TEST_PROXY_TCP_PORT": 2,
    "SATPULSE_TEST_PROXY_TCP_RTCM_PORT": 3,
    "SATPULSE_TEST_REMOTE_CASTER_PORT": 4,
}
NTP_SOCK_MAGIC = 0x534F434B
INSTANCE_RE = re.compile(r"^[A-Za-z0-9_.]+$")


class SystemState(TypedDict):
    service: dict[str, str]
    device: dict[str, str]
    jobs: str


class Root:
    """Runs commands that need root, optionally through sudo -n."""

    def __init__(self, use_sudo: bool) -> None:
        self.use_sudo = use_sudo

    def cmd(self, cmd: Sequence[str]) -> list[str]:
        if os.geteuid() == 0:
            return list(cmd)
        if self.use_sudo:
            return ["sudo", "-n", *cmd]
        raise RuntimeError("root command requested without root or sudo")

    def run(
        self,
        cmd: Sequence[str],
        *,
        timeout: float = 30,
        check: bool = True,
        input_data: bytes | None = None,
    ) -> subprocess.CompletedProcess[bytes]:
        p = subprocess.run(
            self.cmd(cmd),
            input=input_data,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=timeout,
        )
        if check and p.returncode != 0:
            raise RuntimeError(command_error(cmd, p))
        return p


class LiveCheck:
    """A packet-stream check that subscribes before replay starts."""

    def __init__(self, label: str, fn: Callable[[], object]) -> None:
        self.label = label
        self.fn = fn
        self.thread: threading.Thread | None = None
        self.error: BaseException | None = None

    def start(self, ctx: Context) -> None:
        ctx.progress(f"starting live check {self.label}")
        self.thread = threading.Thread(target=self.run, name=f"live-{self.label}", daemon=True)
        self.thread.start()

    def run(self) -> None:
        try:
            self.fn()
        except BaseException as e:
            self.error = e

    def wait(self, ctx: Context, timeout: float = 60) -> None:
        assert self.thread is not None
        self.thread.join(timeout=timeout)
        if self.thread.is_alive():
            raise RuntimeError(f"live check {self.label} did not finish within {timeout:g}s")
        if self.error is not None:
            raise self.error
        ctx.progress(f"ok live check {self.label}")


class Context:
    """Resolved system resources and helper subprocesses for one smoke run."""

    def __init__(
        self,
        *,
        instance: str,
        run_dir: str,
        root: Root,
        packet_log: str,
        factor: int,
        port_base: int,
        ntp_shm_segment: int,
    ) -> None:
        self.name = "system"
        self.instance = instance
        self.run_dir = run_dir
        self.root = root
        self.packet_log = packet_log
        self.factor = factor
        self.ports = {key: port_base + off for key, off in PORT_OFFSETS.items()}
        self.ntp_shm_segment_value = ntp_shm_segment
        self.device = f"/dev/{instance}"
        self.device_unit = systemd_escape_device(self.device)
        self.config_path = f"/etc/satpulse.d/{instance}.toml"
        self.service = f"satpulse@{instance}.service"
        self.run_system_dir = f"/run/satpulse-smoke/{instance}"
        self.udev_rule_path = "/run/udev/rules.d/99-satpulse-smoke.rules"
        self.ntp_sock = f"{self.run_system_dir}/chrony.sock"
        self.proxy_socket_path = f"{self.run_system_dir}/proxy.sock"
        self.daemon_log = os.path.join(run_dir, "journal.log")
        self.warning_log = os.path.join(run_dir, "journal-warnings.log")
        self.status_log = os.path.join(run_dir, "systemctl-status.txt")
        self.jobs_log = os.path.join(run_dir, "systemctl-jobs.txt")
        self.show_log = os.path.join(run_dir, "systemctl-show.txt")
        self.rendered_config = os.path.join(run_dir, "fakegps.toml")
        self.rendered_udev_rule = os.path.join(run_dir, "fakegps.udev.rules")
        self.replay_err = os.path.join(run_dir, "replay.err")
        self.ntp_log = os.path.join(run_dir, "ntp.jsonl")
        self.caster_capture = os.path.join(run_dir, "pushed.bin")
        self.caster_log = os.path.join(run_dir, "caster.log")
        self.udp_capture = os.path.join(run_dir, "udp-pushed.bin")
        self.udp_log = os.path.join(run_dir, "udp.log")
        self.serial_writes = os.path.join(run_dir, "serial-out.bin")
        self.pull_source_log = ""
        self.satpulsetool = find_program("satpulsetool", ["/usr/bin/satpulsetool", "/usr/local/bin/satpulsetool"])
        self.replay_proc: subprocess.Popen[bytes] | None = None
        self._replay_fifo: IO[bytes] | None = None
        self._replay_err_file: IO[bytes] | None = None
        self.ntp_proc: subprocess.Popen[bytes] | None = None
        self.caster_proc: subprocess.Popen[bytes] | None = None
        self._caster_log_file: IO[bytes] | None = None
        self.invocation_id = ""
        self.journal_since = time.time()
        self.t0 = time.monotonic()
        self.started_service = False
        self.own_device = False
        self.own_config = False
        self.own_logs = False
        self.own_run_system_dir = False
        self.own_udev_rule = False

    def progress(self, msg: str) -> None:
        print(f"[{time.monotonic() - self.t0:6.1f}s] {msg}", flush=True)

    @property
    def serial(self) -> str:
        return self.device

    @property
    def log_dir(self) -> str:
        return "/var/log/satpulse"

    @property
    def ntrip_port(self) -> int:
        return self.port("SATPULSE_TEST_NTRIP_PORT")

    @property
    def proxy_socket(self) -> str:
        return self.proxy_socket_path

    @property
    def ntp_shm_segment(self) -> int:
        return self.ntp_shm_segment_value

    def root_cmd(self, cmd: Sequence[str]) -> list[str]:
        return self.root.cmd(cmd)

    def port(self, key: str) -> int:
        return self.ports[key]

    def http_url(self, path: str) -> str:
        return f"http://127.0.0.1:{self.port('SATPULSE_TEST_HTTP_PORT')}{path}"

    def disconnect(self) -> None:
        raise RuntimeError("system smoke uses a FIFO and cannot disconnect it")

    def wait_exit(self, timeout: float = 10) -> int:
        raise RuntimeError("system smoke manages satpulsed through systemd")

    def setup(self, force: bool) -> None:
        self.progress(f"checking stale system paths for {self.instance}")
        require_clean_path(self.root, self.device, "device", force, allow_fifo=True)
        require_clean_path(self.root, self.config_path, "config", force)
        require_clean_path(self.root, self.udev_rule_path, "udev rule", force)
        for path in self.log_paths():
            require_clean_path(self.root, path, "log", force)
        self.own_logs = True
        require_clean_dir(self.root, self.run_system_dir, "run directory", force)
        self.install_device_alias()
        self.remove_ntp_shm()
        self.progress("reloading systemd and stopping any old test service")
        self.root.run(["systemctl", "daemon-reload"], timeout=15)
        self.root.run(["systemctl", "stop", self.service], timeout=20, check=False)
        self.root.run(["systemctl", "reset-failed", self.service], timeout=10, check=False)
        self.progress(f"creating FIFO {self.device} and system config")
        self.root.run(["install", "-d", "-m", "0755", "/etc/satpulse.d"], timeout=10)
        self.root.run(["install", "-d", "-m", "0755", self.log_dir], timeout=10)
        self.root.run(["install", "-d", "-m", "0777", self.run_system_dir], timeout=10)
        self.own_run_system_dir = True
        self.root.run(["mkfifo", "-m", "0666", self.device], timeout=10)
        self.own_device = True
        write_text(self.rendered_config, self.config_text())
        self.root.run(["install", "-D", "-m", "0644", self.rendered_config, self.config_path], timeout=10)
        self.own_config = True

    def install_device_alias(self) -> None:
        self.progress(f"installing runtime udev alias {self.device_unit} via /dev/null")
        write_text(self.rendered_udev_rule, self.udev_rule_text())
        self.root.run(["install", "-d", "-m", "0755", "/run/udev/rules.d"], timeout=10)
        self.root.run(["install", "-D", "-m", "0644", self.rendered_udev_rule, self.udev_rule_path], timeout=10)
        self.own_udev_rule = True
        self.reload_udev_alias()
        if not self.wait_device_unit(timeout=10):
            state = self.unit_props(self.device_unit, ["ActiveState", "SubState", "Job", "JobType", "JobResult"])
            raise RuntimeError(f"{self.device_unit} did not become active after installing udev alias: {format_props(state)}")

    def udev_rule_text(self) -> str:
        return (
            "# Created by smoketest/system.py; removed by test cleanup.\n"
            f'SUBSYSTEM=="mem", KERNEL=="null", TAG+="systemd", ENV{{SYSTEMD_ALIAS}}+="{self.device}"\n'
        )

    def reload_udev_alias(self) -> None:
        self.root.run(["udevadm", "control", "--reload-rules"], timeout=10)
        self.root.run(["udevadm", "trigger", "--action=change", "--name-match=/dev/null"], timeout=20)
        self.root.run(["udevadm", "settle"], timeout=20)

    def wait_device_unit(self, timeout: float) -> bool:
        deadline = time.time() + timeout
        next_log = 0.0
        while time.time() < deadline:
            state = self.unit_props(self.device_unit, ["ActiveState", "SubState", "Job", "JobType", "JobResult"])
            if state.get("ActiveState") == "active":
                self.progress(f"{self.device_unit} is active")
                return True
            now = time.time()
            if now >= next_log:
                self.progress(f"waiting for {self.device_unit}: {format_props(state)}")
                next_log = now + 2
            time.sleep(0.1)
        return False

    def config_text(self) -> str:
        return f"""[phc]
interface = ""

[serial]
device = "{self.device}"
speed = 115200

[gps]
config = false
vendor = "u-blox"

[log]
dir = "{self.log_dir}"
verbose = true
event = true
track = true
packet = true
interval = 1

[[user]]
name = "rover"
password = "secret"

[[http]]
listen = "127.0.0.1:{self.port('SATPULSE_TEST_HTTP_PORT')}"
pprof = true

[ntrip]
listen = "127.0.0.1:{self.port('SATPULSE_TEST_NTRIP_PORT')}"
country = "THA"
network = "SatPulseSmoke"
lat = 13.731839333333334
lon = 100.64473366666667
generator = "satpulse-system-smoke"
formatDetails = "1005(1),1074(1),1084(1),1094(1),1124(1),1230(1)"
bitrate = 9600

[[ntrip.mountpoint]]
name = "RTCM"
description = "Smoke authenticated RTCM"
auth.users = ["rover"]

[[ntrip.mountpoint]]
name = "OPEN"
description = "Smoke open RTCM"

[ntp]
sock.path = "{self.ntp_sock}"
shm.segment = {self.ntp_shm_segment}

[[proxy.tcp]]
listen = "127.0.0.1:{self.port('SATPULSE_TEST_PROXY_TCP_PORT')}"
protocol = "UBX"
readOnly = true

[[proxy.tcp]]
listen = "127.0.0.1:{self.port('SATPULSE_TEST_PROXY_TCP_RTCM_PORT')}"
protocol = "RTCM"
readOnly = true

[[proxy.socket]]
path = "{self.proxy_socket}"
protocol = "UBX"
readOnly = true

[[stream.push]]
protocol = "RTCM"

[stream.push.ntrip]
address = "127.0.0.1:{self.port('SATPULSE_TEST_REMOTE_CASTER_PORT')}"
mountpoint = "RTCM"
password = "smoketest"
description = "Smoke pushed RTCM"
bitrate = 9600
"""

    def start_ntp_sock(self, timeout: float = 5) -> None:
        self.progress(f"starting NTP SOCK capture at {self.ntp_sock}")
        self.ntp_proc = subprocess.Popen(
            [sys.executable, os.path.join(HERE, "ntpsock.py"), "-f", "json", "-o", self.ntp_log, self.ntp_sock],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        deadline = time.time() + timeout
        while not os.path.exists(self.ntp_sock):
            if self.ntp_proc.poll() is not None:
                _, err = self.ntp_proc.communicate()
                raise RuntimeError(f"ntp consumer exited before binding socket: {err.decode('utf-8', 'replace')}")
            if time.time() >= deadline:
                raise RuntimeError(f"ntp consumer did not bind {self.ntp_sock} within {timeout:g}s")
            time.sleep(0.02)

    def stop_ntp_sock(self) -> None:
        if self.ntp_proc is None:
            return
        stop_process(self.ntp_proc)
        self.ntp_proc = None

    def start_caster(self, timeout: float = 5) -> None:
        self.progress(f"starting fake Ntrip caster on port {self.port('SATPULSE_TEST_REMOTE_CASTER_PORT')}")
        self._caster_log_file = open(self.caster_log, "wb")
        self.caster_proc = subprocess.Popen(
            [
                sys.executable,
                os.path.join(HERE, "scenarios", "ntrip", "fakecaster.py"),
                f"127.0.0.1:{self.port('SATPULSE_TEST_REMOTE_CASTER_PORT')}",
                "-o",
                self.caster_capture,
                "--mountpoint",
                "RTCM",
                "--password",
                "smoketest",
            ],
            stdout=self._caster_log_file,
            stderr=subprocess.STDOUT,
        )
        deadline = time.time() + timeout
        while port_free(self.port("SATPULSE_TEST_REMOTE_CASTER_PORT")):
            if self.caster_proc.poll() is not None:
                raise RuntimeError("fake caster exited before listening")
            if time.time() >= deadline:
                raise RuntimeError(f"fake caster did not listen within {timeout:g}s")
            time.sleep(0.02)

    def wait_push(self, timeout: float = 15) -> None:
        self.progress("waiting for satpulsed to connect stream.push to fake caster")
        deadline = time.time() + timeout
        while True:
            if file_contains(self.caster_log, "accepted SOURCE"):
                return
            if not self.service_running():
                raise RuntimeError("systemd service stopped before pushing to the caster")
            if time.time() >= deadline:
                raise RuntimeError(f"daemon did not push to the caster within {timeout:g}s")
            time.sleep(0.05)

    def stop_caster(self) -> None:
        if self.caster_proc is not None:
            stop_process(self.caster_proc)
            self.caster_proc = None
        if self._caster_log_file is not None:
            self._caster_log_file.close()
            self._caster_log_file = None

    def start_service(self, timeout: float = 35) -> None:
        self.progress(f"queueing systemd start job for {self.service}")
        p = self.root.run(["systemctl", "--no-block", "start", self.service], timeout=10, check=False)
        if p.returncode != 0:
            self.collect_status()
            raise RuntimeError(command_error(["systemctl", "--no-block", "start", self.service], p))
        self.started_service = True
        last = self.wait_service_start(timeout)
        if last["service"].get("ActiveState") != "active":
            self.collect_status()
            self.collect_jobs()
            self.root.run(["systemctl", "cancel"], timeout=10, check=False)
            raise RuntimeError(
                f"{self.service} did not become active within {timeout:g}s; "
                f"service={format_props(last['service'])}; "
                f"{self.device_unit}={format_props(last['device'])}; "
                f"jobs={last['jobs'] or '(none)'}"
            )
        self.invocation_id = self.systemctl_show("InvocationID")
        self.progress(f"{self.service} is active")

    def wait_service_start(self, timeout: float) -> SystemState:
        deadline = time.time() + timeout
        next_log = 0.0
        last = self.system_state()
        while time.time() < deadline:
            last = self.system_state()
            service = last["service"]
            if service.get("ActiveState") == "active":
                return last
            if service.get("ActiveState") == "failed":
                raise RuntimeError(
                    f"{self.service} failed to start; "
                    f"service={format_props(service)}; jobs={last['jobs'] or '(none)'}"
                )
            now = time.time()
            if now >= next_log:
                device = last["device"]
                self.progress(
                    f"waiting for {self.service}: service={format_props(service)}; "
                    f"{self.device_unit}={format_props(device)}; jobs={last['jobs'] or '(none)'}"
                )
                next_log = now + 2
            time.sleep(0.2)
        return last

    def system_state(self) -> SystemState:
        service = self.unit_props(
            self.service,
            ["ActiveState", "SubState", "Result", "Job", "JobType", "JobResult", "ExecMainStatus"],
        )
        device = self.unit_props(self.device_unit, ["ActiveState", "SubState", "Job", "JobType", "JobResult"])
        return {"service": service, "device": device, "jobs": self.list_jobs()}

    def stop_service(self, check: bool = True) -> None:
        p = self.root.run(["systemctl", "stop", self.service], timeout=25, check=False)
        self.root.run(["systemctl", "reset-failed", self.service], timeout=10, check=False)
        if check and p.returncode != 0:
            raise RuntimeError(command_error(["systemctl", "stop", self.service], p))

    def systemctl_show(self, prop: str) -> str:
        p = self.root.run(["systemctl", "show", "--value", "-p", prop, self.service], timeout=10, check=False)
        if p.returncode != 0:
            return ""
        return p.stdout.decode("utf-8", "replace").strip()

    def unit_props(self, unit: str, props: Sequence[str]) -> dict[str, str]:
        cmd = ["systemctl", "show", unit]
        for prop in props:
            cmd += ["-p", prop]
        p = self.root.run(cmd, timeout=10, check=False)
        out: dict[str, str] = {}
        if p.returncode != 0:
            out["error"] = p.stderr.decode("utf-8", "replace").strip() or f"exit {p.returncode}"
            return out
        for line in p.stdout.decode("utf-8", "replace").splitlines():
            if "=" not in line:
                continue
            key, val = line.split("=", 1)
            if val:
                out[key] = val
        return out

    def list_jobs(self) -> str:
        p = self.root.run(["systemctl", "list-jobs", "--no-pager", "--full"], timeout=10, check=False)
        if p.returncode != 0:
            return p.stderr.decode("utf-8", "replace").strip() or f"exit {p.returncode}"
        lines = [
            line.strip()
            for line in p.stdout.decode("utf-8", "replace").splitlines()
            if line.strip() and not line.startswith("No jobs")
        ]
        return " | ".join(lines)

    def service_running(self) -> bool:
        return self.systemctl_show("ActiveState") == "active"

    def wait_listeners(self, timeout: float = 20) -> None:
        for key in ("SATPULSE_TEST_HTTP_PORT", "SATPULSE_TEST_NTRIP_PORT"):
            port = self.port(key)
            self.progress(f"waiting for listener on 127.0.0.1:{port}")
            deadline = time.time() + timeout
            next_log = time.time() + 2
            while port_free(port):
                if not self.service_running():
                    self.collect_status()
                    raise RuntimeError(f"{self.service} stopped before port {port} listened")
                if time.time() >= deadline:
                    self.collect_status()
                    raise RuntimeError(f"{self.service} did not listen on port {port} within {timeout:g}s")
                if time.time() >= next_log:
                    self.progress(f"still waiting for listener on 127.0.0.1:{port}")
                    next_log = time.time() + 2
                time.sleep(0.05)
            self.progress(f"listener is accepting on 127.0.0.1:{port}")

    def start_replay(self, open_timeout: float = 15) -> subprocess.Popen[bytes]:
        self.progress(f"starting packet replay into {self.serial}")
        cmd = [self.satpulsetool, "pack", "--realtime", str(self.factor), self.packet_log]
        errf = open(self.replay_err, "wb")
        fifo = self._open_fifo_write(open_timeout)
        self._replay_fifo = fifo
        self._replay_err_file = errf
        self.replay_proc = subprocess.Popen(cmd, stdout=fifo, stderr=errf)
        return self.replay_proc

    def _open_fifo_write(self, timeout: float) -> IO[bytes]:
        deadline = time.time() + timeout
        next_log = time.time() + 2
        while True:
            try:
                fd = os.open(self.serial, os.O_WRONLY | os.O_NONBLOCK)
                break
            except OSError as e:
                if e.errno != errno.ENXIO:
                    raise
                if not self.service_running():
                    raise RuntimeError(f"{self.service} stopped before opening {self.serial}")
                if time.time() >= deadline:
                    raise RuntimeError(f"{self.service} did not open {self.serial} within {timeout:g}s")
                if time.time() >= next_log:
                    self.progress(f"waiting for {self.service} to open {self.serial}")
                    next_log = time.time() + 2
                time.sleep(0.05)
        flags = fcntl.fcntl(fd, fcntl.F_GETFL)
        fcntl.fcntl(fd, fcntl.F_SETFL, flags & ~os.O_NONBLOCK)
        return os.fdopen(fd, "wb")

    def wait_replay(self, timeout: float = 90) -> None:
        if self.replay_proc is None:
            return
        self.progress("waiting for packet replay to finish")
        rc = self.replay_proc.wait(timeout=timeout)
        self._close_replay_files()
        if rc != 0:
            raise RuntimeError(f"replay exited with code {rc}: {self._replay_stderr()}")

    def _close_replay_files(self) -> None:
        if self._replay_fifo is not None:
            self._replay_fifo.close()
            self._replay_fifo = None
        if self._replay_err_file is not None:
            self._replay_err_file.close()
            self._replay_err_file = None

    def _replay_stderr(self) -> str:
        try:
            with open(self.replay_err, errors="replace") as f:
                return f.read().strip()[-2000:] or "(no stderr)"
        except OSError:
            return "(no stderr)"

    def collect_status(self) -> None:
        p = self.root.run(
            ["systemctl", "status", "--no-pager", "--full", self.service, self.device_unit],
            timeout=15,
            check=False,
        )
        write_bytes(self.status_log, p.stdout + p.stderr)

    def collect_jobs(self) -> None:
        p = self.root.run(["systemctl", "list-jobs", "--no-pager", "--full"], timeout=10, check=False)
        write_bytes(self.jobs_log, p.stdout + p.stderr)

    def collect_show(self) -> None:
        parts: list[bytes] = []
        for unit in (self.service, self.device_unit):
            parts.append(f"## {unit}\n".encode())
            p = self.root.run(["systemctl", "show", unit], timeout=10, check=False)
            parts.append(p.stdout + p.stderr + b"\n")
        write_bytes(self.show_log, b"".join(parts))

    def collect_journal(self) -> None:
        selector = self.journal_selector()
        all_cmd = ["journalctl", "-q", *selector, "--no-pager", "-o", "short-precise"]
        warn_cmd = ["journalctl", "-q", "-p", "0..4", *selector, "--no-pager", "-o", "cat"]
        all_log = self.root.run(all_cmd, timeout=20, check=False)
        warn_log = self.root.run(warn_cmd, timeout=20, check=False)
        write_bytes(self.daemon_log, all_log.stdout + all_log.stderr)
        write_bytes(self.warning_log, warn_log.stdout + warn_log.stderr)

    def journal_selector(self) -> list[str]:
        if self.invocation_id:
            return [f"_SYSTEMD_INVOCATION_ID={self.invocation_id}"]
        since = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(self.journal_since))
        return ["-u", self.service, "--since", since]

    def check_journal(self) -> None:
        self.collect_journal()
        with open(self.daemon_log, errors="replace") as f:
            all_lines = f.read().splitlines()
        with open(self.warning_log, errors="replace") as f:
            warning_lines = f.read().splitlines()
        bad: list[str] = []
        for line in all_lines:
            low = line.lower()
            if "panic" in low or ("goroutine" in low and "[running]" in low):
                bad.append(line)
        for line in warning_lines:
            if not line.strip():
                continue
            if not any(allowed in line for allowed in common.ALLOWED_WARNINGS):
                bad.append(line)
        if bad:
            raise AssertionError("unexpected journal lines:\n" + "\n".join(bad))

    def check_ports_released(self) -> None:
        for key in (
            "SATPULSE_TEST_HTTP_PORT",
            "SATPULSE_TEST_NTRIP_PORT",
            "SATPULSE_TEST_PROXY_TCP_PORT",
            "SATPULSE_TEST_PROXY_TCP_RTCM_PORT",
        ):
            port = self.port(key)
            if not port_free(port):
                raise RuntimeError(f"port {port} still in use after shutdown")

    def copy_logs(self) -> None:
        if not self.own_logs:
            return
        dst = os.path.join(self.run_dir, "log")
        os.makedirs(dst, exist_ok=True)
        for path in self.log_paths():
            if os.path.exists(path):
                self.root.run(["cp", "-a", path, dst], timeout=10, check=False)

    def make_artifacts_readable(self) -> None:
        self.root.run(["chmod", "-R", "u+rwX,go+rX", self.run_dir], timeout=10, check=False)
        owner = sudo_owner()
        if owner is not None:
            uid, gid = owner
            self.root.run(["chown", "-R", f"{uid}:{gid}", self.run_dir], timeout=10, check=False)

    def log_paths(self) -> list[str]:
        return [
            os.path.join(self.log_dir, f"event.{self.instance}.jsonl"),
            os.path.join(self.log_dir, f"track.{self.instance}.jsonl"),
            os.path.join(self.log_dir, f"packet.{self.instance}.jsonl"),
        ]

    def remove_ntp_shm(self) -> None:
        self.root.run(
            [sys.executable, os.path.join(HERE, "ntpshm.py"), "remove", str(self.ntp_shm_segment)],
            timeout=10,
            check=False,
        )

    def remove_device_alias(self) -> None:
        if not self.own_udev_rule:
            return
        self.progress(f"removing runtime udev alias {self.device_unit}")
        self.root.run(["rm", "-f", self.udev_rule_path], timeout=10, check=False)
        self.reload_udev_alias()
        self.own_udev_rule = False

    def cleanup(self) -> None:
        if self.replay_proc is not None and self.replay_proc.poll() is None:
            self.replay_proc.kill()
        self._close_replay_files()
        if self.started_service:
            self.stop_service(check=False)
        self.stop_ntp_sock()
        self.stop_caster()
        self.copy_logs()
        self.remove_ntp_shm()
        if self.own_device:
            self.root.run(["rm", "-f", self.device], timeout=10, check=False)
        if self.own_config:
            self.root.run(["rm", "-f", self.config_path], timeout=10, check=False)
        if self.own_logs:
            for path in self.log_paths():
                self.root.run(["rm", "-f", path], timeout=10, check=False)
        if self.own_run_system_dir:
            self.root.run(["rm", "-rf", self.run_system_dir], timeout=10, check=False)
        self.remove_device_alias()


def run_checks(ctx: Context) -> None:
    check_step(ctx, "Ntrip protected sourcetable", lambda: ntrip.check_sourcetable(ctx, "RTCM"))
    check_step(ctx, "Ntrip open sourcetable", lambda: ntrip.check_sourcetable(ctx, "OPEN"))
    check_step(ctx, "Ntrip unauthorized rejection", lambda: ntrip.check_unauthorized(ctx, "RTCM"))
    live = [
        LiveCheck("SSE events", lambda: common.check_sse(ctx, expect=["time", "posvel", "quality"])),
        LiveCheck("Ntrip authenticated stream", lambda: ntrip.check_stream(ctx, "RTCM", auth=("rover", "secret"))),
        LiveCheck("UBX TCP proxy", lambda: proxy.check_tcp(ctx, ctx.port("SATPULSE_TEST_PROXY_TCP_PORT"), "UBX")),
        LiveCheck("RTCM TCP proxy", lambda: proxy.check_tcp(ctx, ctx.port("SATPULSE_TEST_PROXY_TCP_RTCM_PORT"), "RTCM")),
        LiveCheck("UBX socket proxy capture", lambda: proxy.check_socket_capture(ctx, protocol="UBX")),
    ]
    for check in live:
        check.start(ctx)
    time.sleep(0.2)
    ctx.start_replay()
    for check in live:
        check.wait(ctx)
    ctx.wait_replay()
    check_step(ctx, "HTTP position", lambda: common.check_position(ctx))
    check_step(
        ctx,
        "Prometheus metrics",
        lambda: common.check_metrics(ctx, expect=["satpulse_num_satellites", "satpulse_dop"]),
    )
    check_step(ctx, "HTTP GUI", lambda: common.check_html(ctx, "/"))
    check_step(ctx, "HTTP pprof", lambda: common.check_html(ctx, "/debug/pprof/"))
    check_step(ctx, "event log", lambda: common.check_event_log(ctx, expect_types=["time", "posGeo", "navEpoch"]))
    check_step(ctx, "track log", lambda: common.check_track_log(ctx))
    check_step(ctx, "packet log", lambda: common.check_packet_log(ctx))
    check_step(ctx, "stream.push RTCM capture", lambda: stream.check_pushed_rtcm(ctx, mountpoint="RTCM"))
    check_step(ctx, "NTP SOCK samples", lambda: check_ntp_sock(ctx))
    check_step(ctx, "NTP SHM sample", lambda: ntp.check_shm(ctx))


def check_step(ctx: Context, label: str, fn: Callable[[], object]) -> None:
    ctx.progress(f"checking {label}")
    fn()
    ctx.progress(f"ok {label}")


def check_ntp_sock(ctx: common.SmokeContext, min_samples: int = 1) -> None:
    def attempt() -> list[common.JsonObject] | None:
        samples = read_ntp_sock_samples(ctx.ntp_log)
        good = [
            s for s in samples
            if s.get("len") == 40 and s.get("magic") == NTP_SOCK_MAGIC and int_value(s, "sec") > 0
        ]
        if len(good) >= min_samples:
            return good
        return None

    samples = common.poll(attempt, timeout=15)
    assert samples is not None, f"got fewer than {min_samples} valid NTP SOCK samples"
    for sample in samples:
        leap = int_value(sample, "leap")
        assert leap in (0, 1, 2), f"bad NTP SOCK leap value {leap}"


def read_ntp_sock_samples(path: str) -> list[common.JsonObject]:
    if not os.path.exists(path):
        return []
    out: list[common.JsonObject] = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line:
                out.append(cast(common.JsonObject, json.loads(line)))
    return out


def int_value(obj: common.JsonObject, key: str) -> int:
    v = obj.get(key)
    assert isinstance(v, int), f"{key} missing or non-integer: {obj}"
    return v


def validate_instance(name: str) -> None:
    if not INSTANCE_RE.match(name):
        raise RuntimeError(f"instance {name!r} must match {INSTANCE_RE.pattern}")


def find_program(name: str, fallbacks: Sequence[str]) -> str:
    path = shutil.which(name)
    if path:
        return path
    for fallback in fallbacks:
        if os.path.exists(fallback):
            return fallback
    raise RuntimeError(f"{name} not found in PATH or expected install paths")


def systemd_escape_device(path: str) -> str:
    p = subprocess.run(
        ["systemd-escape", "--path", "--suffix=device", path],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=10,
    )
    if p.returncode != 0:
        err = p.stderr.decode("utf-8", "replace").strip()
        raise RuntimeError(err or f"systemd-escape exited with code {p.returncode}")
    return p.stdout.decode("utf-8", "replace").strip()


def sudo_owner() -> tuple[int, int] | None:
    uid = os.environ.get("SUDO_UID")
    gid = os.environ.get("SUDO_GID")
    if uid is None or gid is None:
        return None
    try:
        return int(uid), int(gid)
    except ValueError:
        return None


def format_props(props: dict[str, str]) -> str:
    if not props:
        return "{}"
    return "{" + ", ".join(f"{key}={props[key]}" for key in sorted(props)) + "}"


def require_root_access(root: Root) -> None:
    if os.geteuid() == 0:
        return
    if not root.use_sudo:
        raise RuntimeError("system smoke must run as root or use passwordless sudo")
    p = root.run(["true"], timeout=5, check=False)
    if p.returncode != 0:
        raise RuntimeError("sudo -n is not available; run as root or configure passwordless sudo")


def require_clean_path(root: Root, path: str, label: str, force: bool, allow_fifo: bool = False) -> None:
    if not os.path.lexists(path):
        return
    if not force:
        raise RuntimeError(f"{label} path {path} already exists; rerun with --force to replace test leftovers")
    if allow_fifo:
        mode = os.lstat(path).st_mode
        if stat.S_ISFIFO(mode):
            root.run(["rm", "-f", path], timeout=10)
            return
        raise RuntimeError(f"refusing to remove non-FIFO device path {path}")
    root.run(["rm", "-f", path], timeout=10)


def require_clean_dir(root: Root, path: str, label: str, force: bool) -> None:
    if not os.path.exists(path):
        return
    if not force:
        raise RuntimeError(f"{label} {path} already exists; rerun with --force to replace test leftovers")
    parent = os.path.realpath("/run/satpulse-smoke")
    real = os.path.realpath(path)
    if not real.startswith(parent + os.sep):
        raise RuntimeError(f"refusing to remove unexpected run directory {path}")
    root.run(["rm", "-rf", path], timeout=10)


def alloc_port_block(base: int) -> int:
    ceil = ephemeral_floor()
    block = base
    while block + PORT_BLOCK <= ceil:
        if all(bindable(block + off) for off in range(PORT_BLOCK)):
            return block
        block += PORT_BLOCK
    raise RuntimeError(f"no free port block available from {base} below {ceil}")


def ephemeral_floor() -> int:
    try:
        with open("/proc/sys/net/ipv4/ip_local_port_range") as f:
            return int(f.read().split()[0])
    except OSError:
        return 32768


def bindable(port: int) -> bool:
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        s.bind(("127.0.0.1", port))
        return True
    except OSError:
        return False
    finally:
        s.close()


def port_free(port: int) -> bool:
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=0.3):
            return False
    except OSError:
        return True


def stop_process(proc: subprocess.Popen[bytes]) -> None:
    if proc.poll() is not None:
        return
    proc.terminate()
    try:
        proc.wait(timeout=2)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait()


def file_contains(path: str, needle: str) -> bool:
    try:
        with open(path, errors="replace") as f:
            return needle in f.read()
    except OSError:
        return False


def write_text(path: str, text: str) -> None:
    with open(path, "w") as f:
        f.write(text)


def write_bytes(path: str, data: bytes) -> None:
    with open(path, "wb") as f:
        f.write(data)


def command_error(cmd: Sequence[str], p: subprocess.CompletedProcess[bytes]) -> str:
    out = p.stdout.decode("utf-8", "replace").strip()
    err = p.stderr.decode("utf-8", "replace").strip()
    detail = "\n".join(s for s in (out, err) if s)
    msg = f"{' '.join(cmd)} exited with code {p.returncode}"
    if detail:
        msg += f":\n{detail}"
    return msg


def parse_args(argv: Sequence[str]) -> argparse.Namespace:
    ap = argparse.ArgumentParser(description="system smoke test for installed satpulse")
    ap.add_argument("--instance", default=DEFAULT_INSTANCE, help="systemd instance and /dev name (default: fakegps)")
    ap.add_argument("--packet-log", default=os.path.join(REPO, DEFAULT_PACKET_LOG), help="packet JSONL to replay")
    ap.add_argument("--factor", type=int, default=1, help="satpulsetool pack realtime factor (default: 1)")
    ap.add_argument("--port-base", type=int, default=DEFAULT_PORT_BASE, help="first TCP port to try")
    ap.add_argument("--ntp-shm-segment", type=int, default=253, help="NTP SHM segment to use")
    ap.add_argument("--artifact-dir", help="directory for artifacts (default: temporary)")
    ap.add_argument("--force", action="store_true", help="replace stale fakegps config/device/log leftovers")
    ap.add_argument("--sudo", action="store_true", help="use sudo -n for privileged operations when not root")
    ap.add_argument("--keep-artifacts", action="store_true", help="keep artifacts even when the smoke passes")
    return ap.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    try:
        validate_instance(args.instance)
        packet_log = os.path.abspath(args.packet_log)
        if not os.path.exists(packet_log):
            raise RuntimeError(f"packet log {packet_log} does not exist")
        if args.factor <= 0:
            raise RuntimeError("--factor must be positive")
        if args.ntp_shm_segment < 0 or args.ntp_shm_segment > 255:
            raise RuntimeError("--ntp-shm-segment must be in 0..255")
        root = Root(args.sudo or os.geteuid() != 0)
        require_root_access(root)
        run_dir = os.path.abspath(args.artifact_dir) if args.artifact_dir else tempfile.mkdtemp(prefix="satpulse-system-smoke-")
        os.makedirs(run_dir, exist_ok=True)
        ctx = Context(
            instance=args.instance,
            run_dir=run_dir,
            root=root,
            packet_log=packet_log,
            factor=args.factor,
            port_base=alloc_port_block(args.port_base),
            ntp_shm_segment=args.ntp_shm_segment,
        )
    except Exception as e:
        print(f"FAIL setup: {e}", file=sys.stderr)
        return 1

    status = "PASS"
    detail = ""
    try:
        print(f"START system smoke artifacts={ctx.run_dir}", flush=True)
        ctx.setup(args.force)
        ctx.start_ntp_sock()
        ctx.start_caster()
        ctx.start_service()
        ctx.wait_listeners()
        ctx.wait_push()
        run_checks(ctx)
        ctx.stop_service()
        ctx.check_ports_released()
        ctx.check_journal()
    except Exception:
        status = "FAIL"
        detail = traceback.format_exc()
        write_text(os.path.join(ctx.run_dir, "failure.txt"), detail)
    finally:
        for action in (
            ctx.collect_status,
            ctx.collect_jobs,
            ctx.collect_show,
            ctx.collect_journal,
            ctx.cleanup,
            ctx.make_artifacts_readable,
        ):
            try:
                action()
            except Exception as e:
                msg = f"{type(e).__name__}: {e}\n"
                if status == "PASS":
                    status = "FAIL"
                    detail = msg
                else:
                    detail += "\ncleanup error: " + msg

    if status == "PASS":
        print("PASS system smoke", flush=True)
        if not args.keep_artifacts and args.artifact_dir is None:
            shutil.rmtree(ctx.run_dir, ignore_errors=True)
        elif not args.keep_artifacts:
            print(f"artifacts kept in {ctx.run_dir}", flush=True)
        return 0
    print(detail, file=sys.stderr)
    print(f"FAIL system smoke; artifacts kept in {ctx.run_dir}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
