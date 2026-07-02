#!/usr/bin/env python3
"""Daemon smoke test runner (direct environment).

Runs real satpulsed binaries fed by realtime packet-log replay through a
FIFO, with no root and no GPS hardware. See plan/smoke-test.md.

Each scenario has an explicit ID in SCENARIOS. For scenario ID family/name:
  - scenarios/family/name.toml.in : config template using ${SATPULSE_TEST_*}
  - scenarios/family/name.py      : defines PACKET_LOG, FACTOR, and run(ctx)

The runner allocates a resource block (ports, paths) per scenario, renders
the config, starts the daemon, replays the packet log into the FIFO, then
calls the scenario's run(ctx) to perform its checks. Scenarios are
parallel-safe and run concurrently by default.
"""

from __future__ import annotations

import argparse
import concurrent.futures
import errno
import fcntl
import importlib.util
import os
import platform
import select
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import tomllib
import traceback
import tty
from typing import IO, Callable, Literal, Protocol, Sequence, cast

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)  # let run.py and scenarios import the shared checks module
REPO = os.path.dirname(HERE)

import common  # noqa: E402  (after sys.path is set up)
SCENARIOS_DIR = os.path.join(HERE, "scenarios")

SCENARIOS = [
    "basic/minimal",
    "logging/all",
    "http/full",
    "http/disabled",
    "http/multiple",
    "ntrip/basic",
    "ntrip/auth",
    "ntrip/anyuser",
    "ntrip/metadata",
    "ntrip/msm7to4",
    "ntrip/rtklib",
    "ntp/sock",
    "ntp/shm",
    "proxy/tcp",
    "proxy/socket",
    "stream/push-ntrip",
    "stream/push-udp",
    "stream/pull-ntrip",
    "stream/pull-tcp",
    "stream/pull-rtklib",
    "stream/nmea-send",
    "shutdown/serial-loss",
]


class ScenarioModule(Protocol):
    PACKET_LOG: str
    FACTOR: int

    def run(self, ctx: Context) -> None: ...

Status = Literal["PASS", "FAIL", "SKIP", "XFAIL", "XPASS"]

# Named resources mapped to offsets within each scenario's port block.
PORT_OFFSETS = {
    "SATPULSE_TEST_HTTP_PORT": 0,
    "SATPULSE_TEST_NTRIP_PORT": 1,
    "SATPULSE_TEST_PROXY_TCP_PORT": 2,
    "SATPULSE_TEST_PROXY_TCP_RTCM_PORT": 3,
    "SATPULSE_TEST_REMOTE_CASTER_PORT": 4,
    "SATPULSE_TEST_TOOL_PORT": 5,
    "SATPULSE_TEST_REMOTE_CASTER_PORT2": 6,
    "SATPULSE_TEST_REMOTE_UDP_PORT": 7,
    "SATPULSE_TEST_HTTP_PORT2": 8,
}
PORT_BLOCK = 16

# Exit codes the systemd unit treats as non-restartable (RestartPreventExitStatus
# in configs/satpulse@.service). A SELF_SHUTDOWN scenario expects a restartable
# failure: non-zero, and not one of these.
RESTART_PREVENT_CODES = {64, 77, 78}


def _ephemeral_floor() -> int:
    """Lowest port the kernel allocates for ephemeral (outbound) connections."""
    try:
        with open("/proc/sys/net/ipv4/ip_local_port_range") as f:
            return int(f.read().split()[0])
    except OSError:
        return 32768  # Linux default; fallback when the file is absent


# Scenario listener ports sit below the ephemeral range. A port inside that
# range can be handed out as an outbound source port between allocation and the
# daemon's listen(), which would then lose the bind race with EADDRINUSE.
PORT_BASE = 20000
PORT_CEIL = _ephemeral_floor()

_port_lock = threading.Lock()
_port_cursor = PORT_BASE


def _port_bindable(port: int) -> bool:
    """True if a fresh TCP listener can bind 127.0.0.1:port right now."""
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        s.bind(("127.0.0.1", port))
        return True
    except OSError:
        return False
    finally:
        s.close()


def alloc_port_block() -> int:
    """Reserve a free, contiguous block of PORT_BLOCK ports below the ephemeral range.

    Parallel scenarios share a cursor under a lock so their blocks are disjoint,
    and every port in a block is probed so a block already held by another
    process is skipped rather than colliding with the daemon's listen().
    """
    global _port_cursor
    with _port_lock:
        base = _port_cursor
        while base + PORT_BLOCK <= PORT_CEIL:
            if all(_port_bindable(base + off) for off in range(PORT_BLOCK)):
                _port_cursor = base + PORT_BLOCK
                return base
            base += PORT_BLOCK
        raise RuntimeError("no free port block available below the ephemeral range")


_out_lock = threading.Lock()


def emit(msg: str) -> None:
    """Print a progress line atomically across parallel worker threads."""
    with _out_lock:
        print(msg, flush=True)


def emit_failure_detail(name: str, detail: str) -> None:
    """Print a failure detail block atomically across parallel worker threads."""
    with _out_lock:
        print(f"\n--- {name} ---")
        print("    " + detail.replace("\n", "\n    ").rstrip(), flush=True)


def build_dir() -> str:
    """Name of the out/ subdir holding the binaries under test.

    Honour GOOS and GOARCH when set so a cross-built tree is exercised against
    its own binaries rather than the host's. Linux builds use out/<arch>;
    other Unix builds use out/<goos>_<arch>, matching unix-build.sh.
    """
    goos = os.environ.get("GOOS") or platform.system().lower()
    goarch = os.environ.get("GOARCH")
    if not goarch:
        m = platform.machine()
        goarch = {"x86_64": "amd64", "aarch64": "arm64"}.get(m, m)
    if goos == "linux":
        return goarch
    return f"{goos}_{goarch}"


def bin_path(name: str) -> str:
    return os.path.join(REPO, "out", build_dir(), name)


def make_pty() -> tuple[int, int, str]:
    """Create a pty whose slave stands in for a serial device.

    Returns (master_fd, slave_fd, slave_name). The daemon opens slave_name as
    its serial device; a pty slave is a real TTY, so it takes the term.Term
    path a USB serial receiver uses, not the FIFO replay path. Writing to the
    master feeds the daemon; closing every master fd makes the slave reads fail
    (EIO), which is how a pty -- unlike a FIFO -- can model the device being
    unplugged. The slave is put in raw mode so binary GPS packets pass through
    untranslated and the daemon's own writes are not echoed back to the master.
    """
    master, slave = os.openpty()
    tty.setraw(slave)
    return master, slave, os.ttyname(slave)


class Context:
    """Resolved resources and helpers passed to a scenario's run(ctx)."""

    def __init__(
        self,
        name: str,
        run_dir: str,
        env: dict[str, str],
        factor: int | float,
        packet_log: str,
        daemon_log: str,
        has_http: bool,
        has_http2: bool,
        has_ntrip: bool,
        has_ntp_sock: bool,
        has_push: bool,
        requires_root: bool,
        use_sudo: bool,
        input_kind: str = "fifo",
        has_pull: bool = False,
        pull_source_log: str = "",
        pull_peer: str = "fake",
    ) -> None:
        self.name = name
        self.run_dir = run_dir
        self.env = env
        # Serial input transport: "fifo" (read-only replay sink, the default) or
        # "pty" (full-duplex; can be written to and can be disconnected). See
        # attach_pty/disconnect.
        self.input_kind = input_kind
        self.factor = factor
        self.packet_log = packet_log
        self.daemon_log = daemon_log
        self.has_http = has_http
        self.has_http2 = has_http2
        self.has_ntrip = has_ntrip
        self.has_ntp_sock = has_ntp_sock
        self.has_push = has_push
        self.has_pull = has_pull
        # Absolute path to the RTCM log the fake correction source streams to the
        # daemon's stream.pull client (set only for pull scenarios).
        self.pull_source_log = pull_source_log
        # Correction-source implementation for stream.pull: "fake" or "str2str".
        self.pull_peer = pull_peer
        self.requires_root = requires_root
        self.use_sudo = use_sudo
        self.replay_err = os.path.join(run_dir, "replay.err")
        self.satpulsed = bin_path("satpulsed")
        self.satpulsetool = bin_path("satpulsetool")
        self.daemon_pid_file = os.path.join(run_dir, "satpulsed.pid")
        self.daemon: subprocess.Popen[bytes] | None = None
        self.replay_proc: subprocess.Popen[bytes] | None = None
        self._replay_fifo: IO[bytes] | None = None
        self._replay_err_file: IO[bytes] | None = None
        # pty transport state (input_kind == "pty"). The master is the write
        # side we feed and later close to disconnect; the slave is held open so
        # the master never sees a transient "no slave" hangup before the daemon
        # opens the device by name. A background thread drains the daemon's
        # upstream writes into _pty_capture (when set) or discards them.
        self._pty_master = -1
        self._pty_slave = -1
        self._pty_drain: threading.Thread | None = None
        self._pty_drain_stop = threading.Event()
        # When capture is enabled (start_write_capture), the drain thread writes
        # the daemon's upstream serial output to serial_writes instead of
        # discarding it, so a write-path scenario (stream.pull) can scan what the
        # daemon sent back to the receiver.
        self.serial_writes = os.path.join(run_dir, "serial-out.bin")
        self._pty_capture: IO[bytes] | None = None
        # Chrony SOCK consumer: a separate ntpsock.py process binds the socket
        # before the daemon starts and logs received samples to ntp_log.
        self.ntp_log = os.path.join(run_dir, "ntp.jsonl")
        self.ntp_proc: subprocess.Popen[bytes] | None = None
        self._ntp_log_file: IO[bytes] | None = None
        # Fake Ntrip casters: one scenarios/ntrip/fakecaster.py process per [[stream.push]]
        # entry, each listening before the daemon starts. The first entry's
        # caster accepts and appends the pushed payload to caster_capture; their
        # diagnostics share caster_log.
        self.caster_capture = os.path.join(run_dir, "pushed.bin")
        self.caster_log = os.path.join(run_dir, "caster.log")
        self.caster_procs: list[subprocess.Popen[bytes]] = []
        self._caster_log_file: IO[bytes] | None = None
        # Fake UDP push destinations: one scenarios/stream/fakeudp.py process
        # per UDP [[stream.push]] entry. The first entry writes packet bytes to
        # udp_capture; diagnostics go to udp_log.
        self.udp_capture = os.path.join(run_dir, "udp-pushed.bin")
        self.udp_log = os.path.join(run_dir, "udp.log")
        self.udp_procs: list[subprocess.Popen[bytes]] = []
        self._udp_log_file: IO[bytes] | None = None
        # Fake Ntrip correction source: one scenarios/stream/fakesource.py process
        # listening before the daemon, which the daemon's [stream.pull] client
        # connects out to and reads RTCM from. The daemon writes those corrections
        # back over the serial port, where the pty drain captures them to
        # serial_writes; the source's diagnostics go to source_log.
        self.source_log = os.path.join(run_dir, "source.log")
        self.source_proc: subprocess.Popen[bytes] | None = None
        # The pack process feeding a str2str correction source (pull_peer ==
        # "str2str"); None for the fake source, which runs pack itself.
        self.pack_proc: subprocess.Popen[bytes] | None = None
        self._source_log_file: IO[bytes] | None = None

    @property
    def serial(self) -> str:
        """The serial device path the daemon opens (FIFO path or pty slave)."""
        return self.env["SATPULSE_TEST_SERIAL"]

    @property
    def log_dir(self) -> str:
        return self.env["SATPULSE_TEST_LOG_DIR"]

    def port(self, key: str) -> int:
        return int(self.env[key])

    @property
    def http_port(self) -> int:
        return self.port("SATPULSE_TEST_HTTP_PORT")

    @property
    def http_port2(self) -> int:
        return self.port("SATPULSE_TEST_HTTP_PORT2")

    @property
    def ntrip_port(self) -> int:
        return self.port("SATPULSE_TEST_NTRIP_PORT")

    @property
    def proxy_socket(self) -> str:
        return self.env["SATPULSE_TEST_PROXY_SOCKET"]

    @property
    def ntp_sock(self) -> str:
        return self.env["SATPULSE_TEST_NTP_SOCK"]

    @property
    def ntp_shm_segment(self) -> int:
        return int(self.env["SATPULSE_TEST_NTP_SHM_SEGMENT"])

    def root_cmd(self, cmd: Sequence[str]) -> list[str]:
        if os.geteuid() == 0:
            return list(cmd)
        if not self.use_sudo:
            raise RuntimeError("root command requested without --sudo")
        # sudo scrubs the environment, so forward GOARCH explicitly: root
        # helpers (ntpshm.py) need it to pick the cross-built daemon's layout.
        goarch = os.environ.get("GOARCH")
        env = ["env", f"GOARCH={goarch}"] if goarch else []
        return ["sudo", "-n", *env, *cmd]

    def daemon_cmd(self, cmd: Sequence[str]) -> list[str]:
        if self.uses_darwin_sudo_daemon():
            return self.root_cmd([
                "sh", "-c", 'echo $$ > "$1"; shift; exec "$@"',
                "sh", self.daemon_pid_file, *cmd,
            ])
        if self.requires_root:
            return self.root_cmd(cmd)
        return list(cmd)

    def uses_darwin_sudo_daemon(self) -> bool:
        return (
            self.requires_root
            and sys.platform == "darwin"
            and os.geteuid() != 0
            and self.use_sudo
        )

    def wait_daemon_pid(self, timeout: float = 5) -> int:
        deadline = time.time() + timeout
        while True:
            try:
                return int(open(self.daemon_pid_file).read().strip())
            except OSError:
                pass
            except ValueError as e:
                raise RuntimeError(f"invalid daemon pid file {self.daemon_pid_file}") from e
            if self.daemon is not None and self.daemon.poll() is not None:
                raise RuntimeError(f"daemon exited before writing pid file (code {self.daemon.returncode})")
            if time.time() >= deadline:
                raise RuntimeError(f"daemon did not write pid file within {timeout:g}s")
            time.sleep(0.02)

    def daemon_signaler(self) -> Callable[[signal.Signals], None] | None:
        if not self.uses_darwin_sudo_daemon():
            return None

        def send(sig: signal.Signals) -> None:
            name = sig.name[3:] if sig.name.startswith("SIG") else str(int(sig))
            p = subprocess.run(
                self.root_cmd(["kill", f"-{name}", str(self.wait_daemon_pid())]),
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10,
            )
            if p.returncode != 0:
                err = p.stderr.decode("utf-8", "replace").strip()
                out = p.stdout.decode("utf-8", "replace").strip()
                raise RuntimeError(err or out or f"kill -{name} exited with code {p.returncode}")

        return send

    def daemon_final_grace(self) -> float | None:
        if self.uses_darwin_sudo_daemon():
            return 5.0
        return None

    def remove_ntp_shm(self) -> str | None:
        """Remove the test NTP SHM segment if it exists."""
        cmd = self.root_cmd([
            sys.executable,
            os.path.join(HERE, "ntpshm.py"),
            "remove",
            str(self.ntp_shm_segment),
        ])
        p = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
        if p.returncode == 0:
            return None
        err = p.stderr.decode("utf-8", "replace").strip()
        out = p.stdout.decode("utf-8", "replace").strip()
        return err or out or f"ntpshm.py remove exited with code {p.returncode}"

    def start_ntp_sock(self, timeout: float = 5) -> None:
        """Spawn the chrony SOCK consumer and wait for it to bind the socket.

        satpulsed plays the sender: it sends to this path and warns if no one
        is listening, so the consumer must be bound before the daemon starts.
        Binding an AF_UNIX datagram socket creates the path, so poll for it.
        """
        f = open(self.ntp_log, "wb")
        self._ntp_log_file = f
        self.ntp_proc = subprocess.Popen(
            [sys.executable, os.path.join(HERE, "ntpsock.py"), "-f", "json", self.ntp_sock],
            stdout=f, stderr=subprocess.STDOUT,
        )
        deadline = time.time() + timeout
        while not os.path.exists(self.ntp_sock):
            if self.ntp_proc.poll() is not None:
                raise RuntimeError("ntp consumer exited before binding the socket")
            if time.time() >= deadline:
                raise RuntimeError(f"ntp consumer did not bind {self.ntp_sock} within {timeout}s")
            time.sleep(0.02)

    def stop_ntp_sock(self) -> None:
        if self.ntp_proc is None:
            return
        if self.ntp_proc.poll() is None:
            self.ntp_proc.terminate()
            try:
                self.ntp_proc.wait(timeout=2)
            except subprocess.TimeoutExpired:
                self.ntp_proc.kill()
        if self._ntp_log_file is not None:
            self._ntp_log_file.close()
            self._ntp_log_file = None

    def start_push_peers(self, timeout: float = 5) -> None:
        """Start fake remote peers for configured stream.push entries."""
        self.start_caster(timeout)
        self.start_udp_push_server(timeout)

    def start_caster(self, timeout: float = 5) -> None:
        """Start one fake Ntrip caster per Ntrip [[stream.push]] entry.

        The daemon's push writers connect out at startup, so the casters must
        be listening first; otherwise a connect fails, logs an error (which
        check_no_unexpected_errors would flag), and backs off. A readiness
        probe that does not complete a SOURCE handshake is ignored, so polling
        the port is safe.

        Every caster requires the first entry's mountpoint and password (the
        good credentials, read back from the rendered config), so an entry that
        sends a wrong password is rejected with "Bad Password" -- a permanent
        failure the daemon must give up on. Each caster serves a single
        connection, so the long-lived good feed never blocks a reject.
        """
        with open(self.env["SATPULSE_TEST_CONFIG"], "rb") as cf:
            push = tomllib.load(cf)["stream"]["push"]
        entries = [(i, entry) for i, entry in enumerate(push) if "ntrip" in entry]
        if not entries:
            return
        good = entries[0][1]["ntrip"]
        self._caster_log_file = open(self.caster_log, "wb")
        ports = []
        for j, (i, entry) in enumerate(entries):
            port = int(entry["ntrip"]["address"].rsplit(":", 1)[1])
            ports.append(port)
            capture = self.caster_capture if j == 0 else os.path.join(self.run_dir, f"pushed-{i}.bin")
            self.caster_procs.append(subprocess.Popen(
                [sys.executable, os.path.join(SCENARIOS_DIR, "ntrip", "fakecaster.py"),
                 f"127.0.0.1:{port}", "-o", capture,
                 "--mountpoint", good["mountpoint"], "--password", good["password"]],
                stdout=self._caster_log_file, stderr=subprocess.STDOUT,
            ))
        deadline = time.time() + timeout
        for port in ports:
            while port_free(port):
                if any(p.poll() is not None for p in self.caster_procs):
                    raise RuntimeError("fake caster exited before listening")
                if time.time() >= deadline:
                    raise RuntimeError(f"fake caster did not listen on {port} within {timeout}s")
                time.sleep(0.02)

    def start_udp_push_server(self, timeout: float = 5) -> None:
        """Start one fake UDP receiver per UDP [[stream.push]] entry."""
        with open(self.env["SATPULSE_TEST_CONFIG"], "rb") as cf:
            push = tomllib.load(cf)["stream"]["push"]
        entries = [(i, entry) for i, entry in enumerate(push) if "udp" in entry]
        if not entries:
            return
        self._udp_log_file = open(self.udp_log, "wb")
        ports = []
        for j, (i, entry) in enumerate(entries):
            port = int(entry["udp"]["address"].rsplit(":", 1)[1])
            ports.append(port)
            capture = self.udp_capture if j == 0 else os.path.join(self.run_dir, f"udp-pushed-{i}.bin")
            self.udp_procs.append(subprocess.Popen(
                [sys.executable, os.path.join(SCENARIOS_DIR, "stream", "fakeudp.py"),
                 f"127.0.0.1:{port}", "-o", capture],
                stdout=self._udp_log_file, stderr=subprocess.STDOUT,
            ))
        deadline = time.time() + timeout
        for port in ports:
            want = f"listening 127.0.0.1:{port}"
            while True:
                if any(p.poll() is not None for p in self.udp_procs):
                    raise RuntimeError("fake UDP receiver exited before listening")
                try:
                    with open(self.udp_log, errors="replace") as f:
                        if want in f.read():
                            break
                except OSError:
                    pass
                if time.time() >= deadline:
                    raise RuntimeError(f"fake UDP receiver did not listen on {port} within {timeout}s")
                time.sleep(0.02)

    def wait_push(self, timeout: float = 15) -> None:
        """Wait until outbound stream.push senders are ready.

        Gates replay: push paths subscribe to the packet bcast during startup,
        so waiting for each configured remote path to connect guarantees the
        subscription exists before any packet flows. Without this, a fast replay
        could publish packets the push subscriber never sees, and the captured
        stream would be missing its leading packets.
        """
        with open(self.env["SATPULSE_TEST_CONFIG"], "rb") as cf:
            push = tomllib.load(cf)["stream"]["push"]
        want_caster = any("ntrip" in entry for entry in push)
        want_udp = any("udp" in entry for entry in push)
        deadline = time.time() + timeout
        while want_caster:
            try:
                with open(self.caster_log, errors="replace") as fh:
                    if "accepted SOURCE" in fh.read():
                        break
            except OSError:
                pass
            if self.daemon is not None and self.daemon.poll() is not None:
                raise RuntimeError("daemon exited before pushing to the caster")
            if time.time() >= deadline:
                raise RuntimeError(f"daemon did not push to the caster within {timeout}s")
            time.sleep(0.05)
        while want_udp:
            try:
                with open(self.daemon_log, errors="replace") as fh:
                    if "udp push connected" in fh.read():
                        break
            except OSError:
                pass
            if self.daemon is not None and self.daemon.poll() is not None:
                raise RuntimeError("daemon exited before starting UDP push")
            if time.time() >= deadline:
                raise RuntimeError(f"daemon did not start UDP push within {timeout}s")
            time.sleep(0.05)

    def stop_push_peers(self) -> None:
        self.stop_caster()
        self.stop_udp_push_server()

    def stop_caster(self) -> None:
        for p in self.caster_procs:
            if p.poll() is None:
                p.terminate()
                try:
                    p.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    p.kill()
        if self._caster_log_file is not None:
            self._caster_log_file.close()
            self._caster_log_file = None

    def stop_udp_push_server(self) -> None:
        for p in self.udp_procs:
            if p.poll() is None:
                p.terminate()
                try:
                    p.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    p.kill()
        if self._udp_log_file is not None:
            self._udp_log_file.close()
            self._udp_log_file = None

    def start_source(self, timeout: float = 5) -> None:
        """Start the correction source for [stream.pull], before the daemon.

        The daemon's pull client connects out to this source after GPS
        detection, so it must be listening first; otherwise the connect fails,
        the daemon logs a reconnect warning (which check_no_unexpected_errors
        would flag), and backs off. The default fake source (fakesource.py)
        speaks the pull protocol itself and streams the scenario's RTCM log
        (paced by pack) only once a client connects, so the daemon receives the
        whole log losslessly. The str2str peer (pull_peer == "str2str") is a
        real RTKLIB Ntrip caster fed by pack, which serves from the client's
        connect point on, so the daemon receives a contiguous window instead.
        """
        if not self.pull_source_log:
            raise RuntimeError("[stream.pull] configured but the scenario set no PULL_SOURCE_LOG")
        with open(self.env["SATPULSE_TEST_CONFIG"], "rb") as cf:
            pull = tomllib.load(cf)["stream"]["pull"]
        self._source_log_file = open(self.source_log, "wb")
        if self.pull_peer == "str2str":
            port = self._start_str2str_source(pull["ntrip"])
        else:
            port = self._start_fake_source(pull)
        proc = self.source_proc
        assert proc is not None
        deadline = time.time() + timeout
        while port_free(port):
            if proc.poll() is not None:
                raise RuntimeError("correction source exited before listening")
            if time.time() >= deadline:
                raise RuntimeError(f"correction source did not listen on {port} within {timeout}s")
            time.sleep(0.02)

    def _start_fake_source(self, pull: dict[str, object]) -> int:
        """Launch fakesource.py for the pull config; return its listen port."""
        src = os.path.join(SCENARIOS_DIR, "stream", "fakesource.py")
        if "tcp" in pull:
            tcp = cast("dict[str, str]", pull["tcp"])
            port = int(tcp["address"].rsplit(":", 1)[1])
            cmd = [
                sys.executable, src, f"127.0.0.1:{port}", "--tcp",
                "--pack", self.satpulsetool, "--factor", str(self.factor), self.pull_source_log,
            ]
        else:
            ntrip = cast("dict[str, str]", pull["ntrip"])
            port = int(ntrip["address"].rsplit(":", 1)[1])
            cmd = [
                sys.executable, src, f"127.0.0.1:{port}", "--mountpoint", ntrip["mountpoint"],
                "--pack", self.satpulsetool, "--factor", str(self.factor), self.pull_source_log,
            ]
            if ntrip.get("username"):
                cmd += ["--username", ntrip["username"]]
            if ntrip.get("password"):
                cmd += ["--password", ntrip["password"]]
            if ntrip.get("nmeaSend"):
                cmd += ["--require-gga"]
        self.source_proc = subprocess.Popen(cmd, stdout=self._source_log_file, stderr=subprocess.STDOUT)
        return port

    def _start_str2str_source(self, ntrip: dict[str, object]) -> int:
        """Launch a real RTKLIB str2str Ntrip caster fed by pack; return its port.

        pack paces and extracts the RTCM log; str2str serves it as an Ntrip
        caster, so the daemon's pull client talks to a real RTKLIB peer.
        """
        str2str = shutil.which("str2str")
        if str2str is None:
            raise RuntimeError("stream.pull str2str peer requires str2str on PATH")
        addr = cast(str, ntrip["address"])
        mountpoint = cast(str, ntrip["mountpoint"])
        port = int(addr.rsplit(":", 1)[1])
        self.pack_proc = subprocess.Popen(
            [self.satpulsetool, "pack", "--realtime", str(self.factor), self.pull_source_log],
            stdout=subprocess.PIPE,
        )
        assert self.pack_proc.stdout is not None
        self.source_proc = subprocess.Popen(
            [str2str, "-out", f"ntripc://:{port}/{mountpoint}"],
            stdin=self.pack_proc.stdout, stdout=self._source_log_file, stderr=subprocess.STDOUT,
        )
        self.pack_proc.stdout.close()
        return port

    def stop_source(self) -> None:
        for proc in (self.source_proc, self.pack_proc):
            if proc is not None and proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    proc.kill()
        if self._source_log_file is not None:
            self._source_log_file.close()
            self._source_log_file = None

    def http_url(self, path: str) -> str:
        return f"http://127.0.0.1:{self.http_port}{path}"

    def wait_listeners(self, timeout: float = 15) -> None:
        """Wait until the daemon's configured listeners accept connections.

        satpulsed brings up its HTTP and Ntrip listeners only after GPS
        detection, which times out (with no data flowing) about 2s after
        start. Waiting for the listeners before replay begins guarantees
        the SSE/Ntrip observers exist before any packet arrives, so live
        checks cannot race the startup path.
        """
        ports = []
        if self.has_http:
            ports.append(self.http_port)
        if self.has_http2:
            ports.append(self.http_port2)
        if self.has_ntrip:
            ports.append(self.ntrip_port)
        deadline = time.time() + timeout
        for p in ports:
            while port_free(p):
                if self.daemon is not None and self.daemon.poll() is not None:
                    raise RuntimeError(f"daemon exited before listening on port {p}")
                if time.time() >= deadline:
                    raise RuntimeError(f"daemon did not listen on port {p} within {timeout}s")
                time.sleep(0.05)

    def start_replay(self, open_timeout: float = 15) -> subprocess.Popen[bytes]:
        """Start the single packet-log replay into the FIFO in the background.

        Exactly one replay runs per daemon lifetime: concatenating replays
        corrupts the RTCM frame boundary at the junction, so live checks
        (SSE, Ntrip streaming) must observe this one replay while it flows
        rather than triggering their own bursts. pack's stderr is captured
        so wait_replay can surface a malformed-log or broken-replay failure.
        """
        cmd = [self.satpulsetool, "pack", "--realtime", str(self.factor), self.packet_log]
        errf = open(self.replay_err, "wb")
        if self.input_kind == "pty":
            # pack writes into its own dup of the pty master; the runner keeps
            # the original master for draining and for disconnect(), so pack
            # exiting (closing its dup) is not on its own a disconnect.
            wfd = os.dup(self._pty_master)
            try:
                p = subprocess.Popen(cmd, stdout=wfd, stderr=errf)
            finally:
                os.close(wfd)
        else:
            f = self._open_fifo_write(open_timeout)
            self._replay_fifo = f
            p = subprocess.Popen(cmd, stdout=f, stderr=errf)
        self._replay_err_file = errf
        self.replay_proc = p
        return p

    def _open_fifo_write(self, timeout: float) -> IO[bytes]:
        """Open the FIFO write end, waiting for the daemon to open the read end.

        A blocking open(O_WRONLY) would hang forever if the daemon crashes
        before opening the FIFO. Open non-blocking instead: O_WRONLY on a
        FIFO with no reader yet fails with ENXIO, so poll until the daemon
        opens it, the daemon dies, or the deadline expires. Clear O_NONBLOCK
        once open so the replay's writes block normally.
        """
        deadline = time.time() + timeout
        while True:
            try:
                fd = os.open(self.serial, os.O_WRONLY | os.O_NONBLOCK)
                break
            except OSError as e:
                if e.errno != errno.ENXIO:
                    raise
                if self.daemon is not None and self.daemon.poll() is not None:
                    raise RuntimeError(
                        f"daemon exited before opening FIFO (code {self.daemon.returncode})"
                    )
                if time.time() >= deadline:
                    raise RuntimeError(f"daemon did not open the FIFO within {timeout}s")
                time.sleep(0.05)
        flags = fcntl.fcntl(fd, fcntl.F_GETFL)
        fcntl.fcntl(fd, fcntl.F_SETFL, flags & ~os.O_NONBLOCK)
        return os.fdopen(fd, "wb")

    def wait_replay(self, timeout: float = 60) -> None:
        """Block until the background replay finishes, checking its exit status.

        A non-zero pack exit (malformed packet log, broken replay) is
        raised with its stderr so the run fails instead of silently
        passing. Idempotent: calling it again after the replay has already
        finished just re-reads the cached exit status. Scenarios that need
        the full replay present before their own checks (logs) call this
        themselves; the runner also calls it after the scenario as a
        backstop, so "the replay completed successfully" is a suite
        invariant rather than something each scenario must remember.
        """
        if self.replay_proc is None:
            return
        rc = self.replay_proc.wait(timeout=timeout)
        self._close_replay_files()
        if rc != 0:
            raise RuntimeError(f"replay (pack) exited with code {rc}: {self._replay_stderr()}")

    def _close_replay_files(self) -> None:
        if self._replay_fifo is not None:
            self._replay_fifo.close()
            self._replay_fifo = None
        if self._replay_err_file is not None:
            self._replay_err_file.close()
            self._replay_err_file = None

    def _replay_stderr(self) -> str:
        try:
            with open(self.replay_err, errors="replace") as fh:
                return fh.read().strip()[-2000:] or "(no stderr)"
        except OSError:
            return "(no stderr)"

    # --- pty transport ------------------------------------------------------
    #
    # A pty is the one transport that can model a serial device disappearing: a
    # FIFO cannot, because satpulsed opens it O_RDWR and holds its own write end
    # so an idle FIFO stays "connected" by design. The pty is also full-duplex,
    # so unlike the read-only FIFO it can carry the daemon's writes -- used by
    # the stream/pull-* write-path scenarios (see start_write_capture). disconnect()
    # requires a pty; using a pty does not require disconnect().

    def start_write_capture(self) -> None:
        """Record the daemon's serial writes to serial_writes (pty only).

        Call before attach_pty so the drain thread captures from the first byte.
        A write-path scenario (stream.pull) scans the result for the RTCM
        corrections the daemon wrote back to the receiver; the non-RTCM probe
        bytes the daemon emits during detection are filtered out by tag.
        """
        self._pty_capture = open(self.serial_writes, "wb")

    def attach_pty(self, master_fd: int, slave_fd: int) -> None:
        """Take ownership of the pty fds and start draining the daemon's writes.

        The slave fd is held open so the master never sees a transient "no
        slave" hangup before the daemon opens the device by name. The drain
        thread reads whatever the daemon writes upstream -- probe output during
        detection now, stream.pull corrections later -- into _pty_capture when
        set, otherwise discarding it. Draining also keeps the daemon's writes
        from blocking on a full pty buffer.
        """
        self._pty_master = master_fd
        self._pty_slave = slave_fd
        self._pty_drain_stop.clear()
        self._pty_drain = threading.Thread(target=self._drain_pty, daemon=True)
        self._pty_drain.start()

    def _drain_pty(self) -> None:
        # select() with a timeout lets the loop notice the stop flag, so the fd
        # is never read after _close_pty() is about to close it.
        while not self._pty_drain_stop.is_set():
            try:
                r, _, _ = select.select([self._pty_master], [], [], 0.1)
            except OSError:
                return
            if not r:
                continue
            try:
                data = os.read(self._pty_master, 4096)
            except OSError:
                return
            if not data:
                return
            if self._pty_capture is not None:
                self._pty_capture.write(data)
                self._pty_capture.flush()

    def disconnect(self) -> None:
        """Disconnect the serial input, as if the device were unplugged.

        pty-only: closing every master fd makes the daemon's slave reads fail
        (EIO), so the scan worker exits -- the same trigger as a vanished USB
        serial device. After this the daemon should shut down on its own;
        assert that with wait_exit().
        """
        if self.input_kind != "pty":
            raise RuntimeError("disconnect() is only meaningful for pty input")
        self._close_pty()

    def _close_pty(self) -> None:
        """Stop the drain thread and close the pty fds. Idempotent."""
        if self._pty_drain is not None:
            self._pty_drain_stop.set()
            self._pty_drain.join(timeout=2)
            self._pty_drain = None
        if self._pty_capture is not None:
            self._pty_capture.close()
            self._pty_capture = None
        for attr in ("_pty_master", "_pty_slave"):
            fd = getattr(self, attr)
            if fd >= 0:
                try:
                    os.close(fd)
                except OSError:
                    pass
                setattr(self, attr, -1)

    def wait_exit(self, timeout: float = 10) -> int:
        """Wait for the daemon to exit on its own, sending it no signal.

        For SELF_SHUTDOWN scenarios after disconnect(): the daemon must shut
        down because its serial input went away. A hang is exactly the bug we
        guard against, so on timeout escalate to SIGQUIT -- Go dumps every
        goroutine stack to the daemon log -- and fail, the same diagnostic
        stop_daemon gives for a stuck shutdown.
        """
        assert self.daemon is not None
        try:
            return self.daemon.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            self.daemon.send_signal(signal.SIGQUIT)
            self.daemon.wait()
            raise RuntimeError(
                f"daemon did not exit on its own within {timeout:g}s after the "
                "serial input disconnected; SIGQUIT dumped goroutines to "
                "satpulsed.log"
            )


def load_scenario(name: str) -> ScenarioModule:
    path = os.path.join(SCENARIOS_DIR, f"{name}.py")
    spec = importlib.util.spec_from_file_location(f"scenario_{name.replace('/', '_')}", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"could not load scenario {name} from {path}")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return cast(ScenarioModule, mod)


def allocate_env(name: str, run_dir: str) -> dict[str, str]:
    base = alloc_port_block()
    log_dir = os.path.join(run_dir, "log")
    env = {
        "SATPULSE_TEST_RUN_DIR": run_dir,
        "SATPULSE_TEST_CONFIG": os.path.join(run_dir, "satpulse.toml"),
        "SATPULSE_TEST_SERIAL": os.path.join(run_dir, "gps.fifo"),
        "SATPULSE_TEST_LOG_DIR": log_dir,
        "SATPULSE_TEST_PROXY_SOCKET": os.path.join(run_dir, "proxy.sock"),
        "SATPULSE_TEST_NTP_SOCK": os.path.join(run_dir, "chrony.sock"),
        "SATPULSE_TEST_NTP_SHM_SEGMENT": str(240 + ((base - PORT_BASE) // PORT_BLOCK) % 13),
    }
    for key, off in PORT_OFFSETS.items():
        env[key] = str(base + off)
    return env


def render_config(template_path: str, out_path: str, env: dict[str, str]) -> tuple[bool, bool, bool, bool, bool, bool]:
    """Render the config template; report which listeners/peers it configures.

    Returns (has_http, has_http2, has_ntrip, has_ntp_sock, has_push, has_pull),
    detected from non-comment lines so a comment that merely mentions a section
    is not mistaken for it. has_http2 is set when a second [[http]] table is
    present, so the runner waits on and verifies the extra listener. has_pull
    keys off the [stream.pull...] table prefix so the runner knows to start a
    fake correction source for it.
    """
    with open(template_path) as f:
        text = f.read()
    for key, val in env.items():
        text = text.replace("${" + key + "}", val)
    with open(out_path, "w") as f:
        f.write(text)
    headers = [
        line.strip()
        for line in text.splitlines()
        if not line.lstrip().startswith("#")
    ]
    return ("[[http]]" in headers, headers.count("[[http]]") >= 2,
            "[ntrip]" in headers,
            any(line.startswith("sock.path") for line in headers),
            "[[stream.push]]" in headers,
            any(line.startswith("[stream.pull") for line in headers))


def port_free(port: int) -> bool:
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=0.3):
            return False
    except OSError:
        return True


def stop_daemon(
    daemon: subprocess.Popen[bytes],
    grace: float = 5.0,
    process_group: bool = False,
    signaler: Callable[[signal.Signals], None] | None = None,
    final_grace: float | None = None,
) -> str | None:
    """Stop the daemon, escalating the way the systemd unit does.

    satpulsed catches both SIGINT and SIGTERM and treats them as the same
    graceful-shutdown request, so SIGTERM cannot rescue a hung shutdown.
    The packaged unit uses TimeoutStopSec=5 with FinalKillSignal=SIGQUIT,
    so on a hang we escalate to SIGQUIT and normally stop there: the daemon
    does not catch it, and Go's default handler dumps all goroutine stacks
    (to the daemon log) and aborts the process -- both the diagnostic we want
    for a shutdown hang and a reliable terminator. SIGQUIT is the final
    backstop unless the caller sets final_grace for a platform wrapper that
    needs its own bounded wait.

    Returns None on a clean SIGINT exit, otherwise an error string
    describing the escalation that was needed.
    """
    send_daemon_signal(daemon, signal.SIGINT, process_group, signaler)
    try:
        daemon.wait(timeout=grace)
        return None
    except subprocess.TimeoutExpired:
        pass
    send_daemon_signal(daemon, signal.SIGQUIT, process_group, signaler)
    if final_grace is None:
        daemon.wait()
    else:
        try:
            daemon.wait(timeout=final_grace)
        except subprocess.TimeoutExpired:
            send_daemon_signal(daemon, signal.SIGKILL, process_group, signaler)
            try:
                daemon.wait(timeout=final_grace)
            except subprocess.TimeoutExpired:
                return (
                    f"daemon did not exit within {grace:g}s of SIGINT, "
                    f"{final_grace:g}s of SIGQUIT, or {final_grace:g}s of SIGKILL"
                )
            return (
                f"daemon did not exit within {grace:g}s of SIGINT or "
                f"{final_grace:g}s of SIGQUIT; SIGKILL stopped it"
            )
    return (
        f"daemon did not exit within {grace:g}s of SIGINT; "
        "SIGQUIT dumped goroutines (see satpulsed.log) and aborted it"
    )


def send_daemon_signal(
    daemon: subprocess.Popen[bytes],
    sig: signal.Signals,
    process_group: bool,
    signaler: Callable[[signal.Signals], None] | None = None,
) -> None:
    """Send sig to the daemon, or to its process group when launched through sudo."""
    if signaler is not None:
        signaler(sig)
        return
    if process_group:
        os.killpg(daemon.pid, sig)
        return
    daemon.send_signal(sig)


def scenario_requires_root(scen: ScenarioModule) -> bool:
    return bool(getattr(scen, "REQUIRES_ROOT", False))


def scenario_missing_binaries(scen: ScenarioModule) -> list[str]:
    """Names from the scenario's REQUIRES that are not on PATH (e.g. str2str)."""
    return [b for b in getattr(scen, "REQUIRES", ()) if shutil.which(b) is None]


def run_scenario(name: str, use_sudo: bool) -> tuple[str, Status, str]:
    scen = load_scenario(name)
    requires_root = scenario_requires_root(scen)
    if requires_root and os.geteuid() != 0 and not use_sudo:
        return (name, "SKIP", "requires root; rerun with --sudo to use sudo -n")
    missing = scenario_missing_binaries(scen)
    if missing:
        return (name, "SKIP", f"requires {', '.join(missing)} on PATH")
    emit(f"START {name}")
    factor = scen.FACTOR
    packet_log = scen.PACKET_LOG
    if not os.path.isabs(packet_log):
        packet_log = os.path.join(REPO, packet_log)
    config_tmpl = os.path.join(SCENARIOS_DIR, f"{name}.toml.in")

    # Serial input transport. "fifo" (default) is a read-only replay sink;
    # "pty" is full-duplex and can be disconnected. SELF_SHUTDOWN -- the daemon
    # is expected to exit on its own when its input goes away -- only makes
    # sense over a pty, since only a pty can disconnect. A pty does not imply
    # SELF_SHUTDOWN: the stream/pull-* write-path scenarios use a pty and still
    # stop via SIGINT.
    input_kind = getattr(scen, "INPUT", "fifo")
    self_shutdown = bool(getattr(scen, "SELF_SHUTDOWN", False))
    if self_shutdown and input_kind != "pty":
        return (name, "FAIL", "SELF_SHUTDOWN requires INPUT='pty' (only a pty can disconnect)")
    # A write-path scenario (stream.pull) sets CAPTURE_WRITES to record what the
    # daemon writes back to the receiver; only a pty carries those writes.
    capture_writes = bool(getattr(scen, "CAPTURE_WRITES", False))
    if capture_writes and input_kind != "pty":
        return (name, "FAIL", "CAPTURE_WRITES requires INPUT='pty' (only a pty carries the daemon's writes)")
    # The RTCM log a [stream.pull] correction source streams to the daemon.
    pull_source_log = getattr(scen, "PULL_SOURCE_LOG", "")
    if pull_source_log and not os.path.isabs(pull_source_log):
        pull_source_log = os.path.join(REPO, pull_source_log)
    # The correction-source implementation: "fake" (scenarios/stream/fakesource.py,
    # the default) or "str2str" (a real RTKLIB Ntrip caster, for interop).
    pull_peer = getattr(scen, "PULL_PEER", "fake")
    # A scenario known to fail (e.g. a bug not yet fixed) declares XFAIL = reason.
    xfail = getattr(scen, "XFAIL", None)

    run_dir = tempfile.mkdtemp(prefix=f"satpulse-smoke-{name.replace('/', '-')}-")
    env = allocate_env(name, run_dir)
    env["SATPULSE_TEST_PACKET_LOG"] = packet_log
    os.makedirs(env["SATPULSE_TEST_LOG_DIR"], exist_ok=True)
    pty_fds: tuple[int, int] | None = None
    if input_kind == "pty":
        master, slave, slave_name = make_pty()
        pty_fds = (master, slave)
        env["SATPULSE_TEST_SERIAL"] = slave_name
    else:
        os.mkfifo(env["SATPULSE_TEST_SERIAL"])
    has_http, has_http2, has_ntrip, has_ntp_sock, has_push, has_pull = render_config(
        config_tmpl, env["SATPULSE_TEST_CONFIG"], env)

    daemon_log = os.path.join(run_dir, "satpulsed.log")
    ctx = Context(
        name, run_dir, env, factor, packet_log, daemon_log,
        has_http, has_http2, has_ntrip, has_ntp_sock, has_push, requires_root, use_sudo,
        input_kind=input_kind, has_pull=has_pull, pull_source_log=pull_source_log,
        pull_peer=pull_peer,
    )
    if pty_fds is not None:
        # Enable capture before draining starts so no early write is missed.
        if capture_writes:
            ctx.start_write_capture()
        ctx.attach_pty(*pty_fds)

    daemon: subprocess.Popen[bytes] | None = None
    status: Status = "PASS"
    detail = ""
    try:
        # Bind the chrony SOCK consumer before the daemon starts, so its first
        # refclock sample lands in a listening socket rather than warning.
        if has_ntp_sock:
            ctx.start_ntp_sock()
        # Start fake remote push peers before the daemon, so outbound push
        # setup succeeds immediately instead of failing and backing off.
        if has_push:
            ctx.start_push_peers()
        # Likewise start the fake correction source before the daemon, so the
        # daemon's pull connect succeeds at once rather than warning and backing
        # off. Pull has no listener of its own and does not gate replay.
        if has_pull:
            ctx.start_source()
        with open(daemon_log, "wb") as out:
            cmd = [ctx.satpulsed, "-v", "-f", env["SATPULSE_TEST_CONFIG"]]
            cmd = ctx.daemon_cmd(cmd)
            daemon = subprocess.Popen(
                cmd,
                stdout=out,
                stderr=subprocess.STDOUT,
                start_new_session=requires_root,
            )
        ctx.daemon = daemon
        if ctx.uses_darwin_sudo_daemon():
            ctx.wait_daemon_pid()

        # Wait for the daemon's listeners before replaying so the HTTP/SSE
        # and Ntrip observers exist before any packet arrives; otherwise a
        # fast replay could be consumed during the pre-listener detection
        # phase and live checks would race startup.
        if daemon.poll() is not None:
            raise RuntimeError(f"daemon exited at startup (code {daemon.returncode})")
        ctx.wait_listeners()
        # Push has no listener of its own; wait for outbound push setup so the
        # bcast subscription is in place before replay, like wait_listeners
        # does for the inbound observers.
        if has_push:
            ctx.wait_push()

        # A single replay then runs in the background while checks observe
        # the live daemon.
        ctx.start_replay()

        scen.run(ctx)

        # Backstop the suite invariant that the replay finished cleanly,
        # even if the scenario did not call wait_replay itself. Idempotent
        # when the scenario already waited. Done before shutdown so pack
        # finishes against a live daemon rather than dying of SIGPIPE.
        ctx.wait_replay()

        if self_shutdown:
            # No signal: losing the serial input must be enough to make the
            # daemon exit. The scenario already disconnected and waited via
            # ctx.wait_exit(), so the daemon should be gone, with a restartable
            # failure code (non-zero, not one systemd treats as terminal) so
            # the unit restarts it.
            if daemon.poll() is None:
                raise RuntimeError("self-shutdown scenario: daemon still running after run()")
            rc = daemon.returncode
            if rc <= 0 or rc in RESTART_PREVENT_CODES:
                raise RuntimeError(
                    "self-shutdown scenario: expected a restartable non-zero exit "
                    f"(not 0 or {sorted(RESTART_PREVENT_CODES)}), got {rc}"
                )
        else:
            # Graceful shutdown: SIGINT should terminate the daemon promptly
            # and release its ports. A hang escalates to SIGQUIT (goroutine
            # dump) and is reported as a failure.
            err = stop_daemon(
                daemon,
                process_group=requires_root,
                signaler=ctx.daemon_signaler(),
                final_grace=ctx.daemon_final_grace(),
            )
            if err is not None:
                raise RuntimeError(err)
            if daemon.returncode not in (0, -signal.SIGINT):
                raise RuntimeError(f"daemon exited with code {daemon.returncode} on SIGINT")
        if ctx.has_http and not port_free(ctx.http_port):
            raise RuntimeError(f"HTTP port {ctx.http_port} still in use after shutdown")
        if ctx.has_http2 and not port_free(ctx.http_port2):
            raise RuntimeError(f"HTTP port {ctx.http_port2} still in use after shutdown")
        # Scan the daemon log only now, so shutdown-time warnings/errors
        # (and any SIGQUIT goroutine dump) are included. A scenario may declare
        # ALLOWED_ERRORS for error lines it expects (e.g. a push it knows the
        # caster rejects).
        common.check_no_unexpected_errors(ctx, allowed=getattr(scen, "ALLOWED_ERRORS", ()))
        if requires_root:
            err = ctx.remove_ntp_shm()
            if err is not None:
                raise RuntimeError(f"failed to remove NTP SHM segment {ctx.ntp_shm_segment}: {err}")
    except Exception:
        status, detail = "FAIL", traceback.format_exc()
    finally:
        if ctx.replay_proc is not None and ctx.replay_proc.poll() is None:
            ctx.replay_proc.kill()
        ctx._close_replay_files()
        ctx._close_pty()
        if daemon is not None and daemon.poll() is None:
            stop_daemon(
                daemon,
                process_group=requires_root,
                signaler=ctx.daemon_signaler(),
                final_grace=ctx.daemon_final_grace(),
            )
        ctx.stop_ntp_sock()
        ctx.stop_push_peers()
        ctx.stop_source()
        if requires_root:
            err = ctx.remove_ntp_shm()
            if err is not None:
                print(f"[{name}] failed to remove NTP SHM segment: {err}", file=sys.stderr)

    # Map an expected failure: an XFAIL scenario reports XFAIL when it fails as
    # expected, and XPASS -- counted as a failure -- when it unexpectedly
    # passes, prompting removal of the marker once the bug is fixed.
    if xfail:
        if status == "FAIL":
            status, detail = "XFAIL", xfail
        elif status == "PASS":
            status, detail = "XPASS", f"expected to fail ({xfail}) but passed; remove XFAIL"
    # Keep the run dir for anything worth investigating -- a real failure or an
    # unexpected pass. A clean pass or an expected failure leaves nothing behind.
    if status in ("FAIL", "XPASS"):
        print(f"[{name}] artifacts kept in {run_dir}", file=sys.stderr)
    else:
        shutil.rmtree(run_dir, ignore_errors=True)
    return (name, status, detail)


def main() -> int:
    ap = argparse.ArgumentParser(description="satpulsed daemon smoke tests")
    ap.add_argument("scenarios", nargs="*", help="scenario names (default: all)")
    ap.add_argument("-j", "--jobs", type=int, default=os.cpu_count() or 4)
    ap.add_argument("-l", "--list", action="store_true", help="list scenarios and exit")
    ap.add_argument("--sudo", action="store_true",
                    help="run root-required scenarios through sudo -n instead of skipping them")
    args = ap.parse_args()

    available = SCENARIOS
    if args.list:
        for n in available:
            print(n)
        return 0

    selected = cast(list[str], args.scenarios) or list(available)
    unknown = [n for n in selected if n not in available]
    if unknown:
        print(f"unknown scenarios: {', '.join(unknown)}", file=sys.stderr)
        return 2

    selected_mods = {n: load_scenario(n) for n in selected}
    skipped_without_sudo = [
        n for n, scen in selected_mods.items()
        if scenario_requires_root(scen) and os.geteuid() != 0 and not args.sudo
    ]
    runnable = [n for n in selected if n not in skipped_without_sudo]
    if args.sudo and os.geteuid() != 0 and any(scenario_requires_root(s) for s in selected_mods.values()):
        if shutil.which("sudo") is None:
            print("--sudo requested, but sudo is not installed", file=sys.stderr)
            return 2
        p = subprocess.run(["sudo", "-n", "true"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
        if p.returncode != 0:
            err = p.stderr.decode("utf-8", "replace").strip()
            print("--sudo requested, but sudo -n true failed" + (f": {err}" if err else ""), file=sys.stderr)
            return 2

    if runnable:
        for b in ("satpulsed", "satpulsetool"):
            if not os.path.exists(bin_path(b)):
                print(f"missing binary {bin_path(b)}; run make first", file=sys.stderr)
                return 2

    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.jobs) as ex:
        futs = {ex.submit(run_scenario, n, args.sudo): n for n in selected}
        for fut in concurrent.futures.as_completed(futs):
            name, status, detail = fut.result()
            results.append((name, status, detail))
            emit(f"{status} {name}" + (f" ({detail})" if status in ("SKIP", "XFAIL", "XPASS") else ""))
            if status in ("FAIL", "XPASS"):
                emit_failure_detail(name, detail)

    # FAIL and XPASS (expected-to-fail but passed) both fail the suite.
    failures = sorted((name, detail) for name, status, detail in results if status in ("FAIL", "XPASS"))
    counts = {s: sum(1 for _, st, _ in results if st == s) for s in ("PASS", "FAIL", "XFAIL", "XPASS", "SKIP")}
    parts = [f"{counts['PASS']} passed"]
    if counts["FAIL"]:
        parts.append(f"{counts['FAIL']} failed")
    if counts["XPASS"]:
        parts.append(f"{counts['XPASS']} unexpectedly passed")
    if counts["XFAIL"]:
        parts.append(f"{counts['XFAIL']} xfail")
    if counts["SKIP"]:
        parts.append(f"{counts['SKIP']} skipped")
    print("\n" + ", ".join(parts))
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
