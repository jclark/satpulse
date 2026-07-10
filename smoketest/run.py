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
import importlib.util
import os
import platform
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
from typing import IO, Callable, Literal, Protocol, Sequence, cast

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)  # let run.py and scenarios import the shared checks module
REPO = os.path.dirname(HERE)

import common  # noqa: E402  (after sys.path is set up)
import platform_api  # noqa: E402
import program_api  # noqa: E402

# Platform seam: the per-OS module supplies the transport, shutdown, and
# privilege primitives (see platform_api). Select it by os.name so run.py
# itself imports cleanly on an OS whose module it does not use; a
# platform_windows module drops in under a further branch here later.
if os.name == "posix":
    import platform_unix as plat  # noqa: E402
else:
    raise RuntimeError(f"smoke tests do not yet support os.name={os.name!r}")

SCENARIOS_DIR = os.path.join(HERE, "scenarios")

SCENARIOS = [
    "basic/minimal",
    "logging/all",
    "http/full",
    "http/disabled",
    "http/multiple",
    "http/wb-default",
    "http/wb-listen",
    "http/wb-survey",
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
    "stream/wb-corrections",
    "shutdown/serial-loss",
    "shutdown/wb-serial-loss",
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
        requires_root: bool,
        use_sudo: bool,
        transport: platform_api.Transport,
    ) -> None:
        self.name = name
        self.run_dir = run_dir
        self.env = env
        # Serial input transport, chosen by the capabilities the scenario needs
        # (see run_scenario): a plain replay sink, or one that is read-write
        # (start_write_capture) and/or disconnectable (disconnect).
        self._transport = transport
        self.factor = factor
        self.packet_log = packet_log
        self.daemon_log = daemon_log
        # Which listeners/peers the program needs, set by program.prepare (from
        # the daemon's rendered config, or left at the defaults for satpulsewb,
        # which has one HTTP listener and no config-derived peers).
        self.has_http = False
        self.has_http2 = False
        self.has_ntrip = False
        self.has_ntp_sock = False
        self.has_push = False
        self.has_pull = False
        # Absolute path to the RTCM log the fake correction source streams (a
        # daemon stream.pull source, or a satpulsewb corrections source).
        self.pull_source_log = ""
        # Correction-source implementation for stream.pull: "fake" or "str2str".
        self.pull_peer = "fake"
        # satpulsewb: the flag list (program.prepare), the bound HTTP port and
        # access token (program.wait_ready, parsed from the printed URL), and
        # the program binary and scenario template base (set by run_scenario).
        self.wb_args: list[str] = []
        self.wb_port = 0
        self.token = ""
        self.seat = ""
        self.prog_bin = ""
        self.template_base = os.path.join(SCENARIOS_DIR, name)
        self.requires_root = requires_root
        self.use_sudo = use_sudo
        self.replay_err = os.path.join(run_dir, "replay.err")
        self.satpulsetool = bin_path("satpulsetool")
        self.daemon_pid_file = os.path.join(run_dir, "satpulsed.pid")
        self.daemon: subprocess.Popen[bytes] | None = None
        self.replay_proc: subprocess.Popen[bytes] | None = None
        self._replay_err_file: IO[bytes] | None = None
        # When capture is enabled (start_write_capture, read-write transport
        # only), the transport writes the daemon's upstream serial output to
        # serial_writes instead of discarding it, so a write-path scenario
        # (stream.pull) can scan what the daemon sent back to the receiver.
        self.serial_writes = os.path.join(run_dir, "serial-out.bin")
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
        return plat.root_cmd(cmd, self.use_sudo)

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
            and not plat.is_root()
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

    def start_correction_source(
        self,
        port: int,
        log: str,
        mode: str = "ntrip",
        mountpoint: str = "",
        username: str = "",
        password: str = "",
        require_gga: bool = False,
        timeout: float = 5,
    ) -> None:
        """Start a fake correction source for a satpulsewb corrections scenario.

        The daemon's start_source derives its fakesource.py from the rendered
        [stream.pull] config; satpulsewb has no such config, so the scenario
        declares the source (port, log, mode, credentials) and this starts the
        same fakesource.py from those parameters. The workbench connects to it
        only once the scenario POSTs corrections/start, but it listens from the
        outset so the connect never races startup.
        """
        self._source_log_file = open(self.source_log, "wb")
        src = os.path.join(SCENARIOS_DIR, "stream", "fakesource.py")
        cmd = [sys.executable, src, f"127.0.0.1:{port}", log,
               "--pack", self.satpulsetool, "--factor", str(self.factor)]
        if mode == "tcp":
            cmd.append("--tcp")
        else:
            if mountpoint:
                cmd += ["--mountpoint", mountpoint]
            if username:
                cmd += ["--username", username]
            if password:
                cmd += ["--password", password]
            if require_gga:
                cmd.append("--require-gga")
        self.source_proc = subprocess.Popen(cmd, stdout=self._source_log_file, stderr=subprocess.STDOUT)
        deadline = time.time() + timeout
        while port_free(port):
            if self.source_proc.poll() is not None:
                raise RuntimeError("correction source exited before listening")
            if time.time() >= deadline:
                raise RuntimeError(f"correction source did not listen on {port} within {timeout}s")
            time.sleep(0.02)

    def http_url(self, path: str) -> str:
        return f"http://127.0.0.1:{self.http_port}{path}"

    def wb_url(self, path: str) -> str:
        """URL for a satpulsewb request, carrying the access token and seat when set.

        The bound port comes from the printed URL (program.wait_ready), so this
        is correct for the no-`-L` default-port path too, where the port is
        chosen at runtime. Requests always use loopback; the all-interfaces bind
        includes it. POSTs and SSE opens require the current seat (claimed in
        wait_ready); GETs ignore it, so appending it unconditionally is safe.
        """
        url = f"http://127.0.0.1:{self.wb_port}{path}"
        if self.token:
            url += ("&" if "?" in path else "?") + "t=" + self.token
        if self.seat:
            url += ("&" if "?" in url else "?") + "seat=" + self.seat
        return url

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
        """Start the single packet-log replay into the transport in the background.

        Exactly one replay runs per daemon lifetime: concatenating replays
        corrupts the RTCM frame boundary at the junction, so live checks
        (SSE, Ntrip streaming) must observe this one replay while it flows
        rather than triggering their own bursts. pack's stderr is captured
        so wait_replay can surface a malformed-log or broken-replay failure.
        The transport decides where pack's output goes (a FIFO write fd, a dup
        of the pty master).
        """
        cmd = [self.satpulsetool, "pack", "--realtime", str(self.factor), self.packet_log]
        errf = open(self.replay_err, "wb")
        self._replay_err_file = errf
        self.replay_proc = self._transport.feed_replay(cmd, errf, open_timeout, self.daemon)
        return self.replay_proc

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
        self._transport.close_replay()
        if self._replay_err_file is not None:
            self._replay_err_file.close()
            self._replay_err_file = None

    def _replay_stderr(self) -> str:
        try:
            with open(self.replay_err, errors="replace") as fh:
                return fh.read().strip()[-2000:] or "(no stderr)"
        except OSError:
            return "(no stderr)"

    def start_write_capture(self) -> None:
        """Record the daemon's serial writes to serial_writes (read-write only).

        Call before the transport is attached so it captures from the first
        byte. A write-path scenario (stream.pull) scans the result for the
        RTCM corrections the daemon wrote back to the receiver; the non-RTCM
        probe bytes the daemon emits during detection are filtered out by tag.
        """
        self._transport.enable_write_capture(self.serial_writes)

    def disconnect(self) -> None:
        """Disconnect the serial input, as if the device were unplugged.

        Only a disconnectable transport supports this. It makes the daemon's
        serial reads fail, so the scan worker exits -- the same trigger as a
        vanished USB serial device. After this the daemon should shut down on
        its own; assert that with wait_exit().
        """
        self._transport.disconnect()

    def wait_exit(self, timeout: float = 10) -> int:
        """Wait for the daemon to exit on its own, sending it no signal.

        For SELF_SHUTDOWN scenarios after disconnect(): the daemon must shut
        down because its serial input went away. A hang is the bug we guard
        against, so the platform escalates to a forced diagnostic dump on a
        timeout and fails.
        """
        assert self.daemon is not None
        return plat.wait_exit(self.daemon, timeout)


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


def port_free(port: int) -> bool:
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=0.3):
            return False
    except OSError:
        return True


def scenario_requires_root(scen: ScenarioModule) -> bool:
    return bool(getattr(scen, "REQUIRES_ROOT", False))


def scenario_missing_binaries(scen: ScenarioModule) -> list[str]:
    """Names from the scenario's REQUIRES that are not on PATH (e.g. str2str)."""
    return [b for b in getattr(scen, "REQUIRES", ()) if shutil.which(b) is None]


def run_scenario(name: str, use_sudo: bool) -> tuple[str, Status, str]:
    scen = load_scenario(name)
    requires_root = scenario_requires_root(scen)
    if requires_root and not plat.is_root() and not use_sudo:
        return (name, "SKIP", "requires root; rerun with --sudo to use sudo -n")
    missing = scenario_missing_binaries(scen)
    if missing:
        return (name, "SKIP", f"requires {', '.join(missing)} on PATH")
    emit(f"START {name}")
    # The program under test is a scenario dimension (default satpulsed); the
    # Program owns what differs between the two (input prep, start command,
    # readiness, allowed log lines, ports to free). See program_api.
    program = program_api.select(getattr(scen, "PROGRAM", "satpulsed"))
    factor = scen.FACTOR
    packet_log = scen.PACKET_LOG
    if not os.path.isabs(packet_log):
        packet_log = os.path.join(REPO, packet_log)

    # Serial-input transport capabilities the scenario needs, computed from what
    # it does, not from a named mechanism. CAPTURE_WRITES (a write-path scenario
    # recording what the program sent back) needs a read-write transport;
    # SELF_SHUTDOWN and DISCONNECTABLE both need a transport that can model the
    # device vanishing. They are orthogonal to capture: serial-loss disconnects
    # without capturing, the stream/pull-* scenarios capture without
    # disconnecting. The platform maps them to a concrete transport (below).
    capture_writes = bool(getattr(scen, "CAPTURE_WRITES", False))
    self_shutdown = bool(getattr(scen, "SELF_SHUTDOWN", False))
    # DISCONNECTABLE requests a transport that can be unplugged without the
    # self-shutdown lifecycle: satpulsewb's device-loss scenario disconnects and
    # then asserts the program keeps running (SELF_SHUTDOWN's inverse), so it
    # needs the pty but not the no-signal exit check.
    disconnectable = self_shutdown or bool(getattr(scen, "DISCONNECTABLE", False))
    # INPUT (the old transport selector) is obsolete: transport is now chosen
    # from the capabilities above. Reject it loudly so a scenario merged from a
    # branch predating this change fails here rather than silently running over
    # the FIFO instead of the pty it asked for.
    if hasattr(scen, "INPUT"):
        return (name, "FAIL", "INPUT is obsolete; declare CAPTURE_WRITES and/or SELF_SHUTDOWN instead")
    # A scenario known to fail (e.g. a bug not yet fixed) declares XFAIL = reason.
    xfail = getattr(scen, "XFAIL", None)

    run_dir = tempfile.mkdtemp(prefix=f"satpulse-smoke-{name.replace('/', '-')}-")
    env = allocate_env(name, run_dir)
    env["SATPULSE_TEST_PACKET_LOG"] = packet_log
    os.makedirs(env["SATPULSE_TEST_LOG_DIR"], exist_ok=True)
    # Create the serial-input transport with the requested capabilities and
    # point the program's device path at it (the FIFO path the runner allocated,
    # or the pty slave name) before preparing the input that references
    # SATPULSE_TEST_SERIAL. A platform that cannot supply those capabilities
    # reports the scenario unsupported here, which is a SKIP.
    try:
        transport = plat.make_transport(
            env["SATPULSE_TEST_SERIAL"], readwrite=capture_writes, disconnectable=disconnectable)
    except platform_api.TransportUnsupported as e:
        shutil.rmtree(run_dir, ignore_errors=True)
        return (name, "SKIP", str(e))
    env["SATPULSE_TEST_SERIAL"] = transport.path()

    daemon_log = os.path.join(run_dir, f"{program.name}.log")
    ctx = Context(
        name, run_dir, env, factor, packet_log, daemon_log,
        requires_root, use_sudo, transport=transport,
    )
    ctx.prog_bin = bin_path(program.name)
    # Prepare the program's input (render the config or the flag list) and
    # record which listeners/peers it needs on ctx.
    program.prepare(ctx, scen)
    # Enable capture before attaching so the transport captures from the first
    # byte; attach takes ownership (a pty starts its drain thread) before the
    # program starts.
    if capture_writes:
        ctx.start_write_capture()
    transport.attach()

    daemon: subprocess.Popen[bytes] | None = None
    status: Status = "PASS"
    detail = ""
    try:
        # Start the fake peers the program needs, each before the program
        # starts so its first sample/connect lands rather than warning and
        # backing off (config-derived for satpulsed, scenario-declared for
        # satpulsewb).
        program.start_peers(ctx, scen)
        with open(daemon_log, "wb") as out:
            cmd = ctx.daemon_cmd(program.command(ctx))
            daemon = subprocess.Popen(
                cmd,
                stdout=out,
                stderr=subprocess.STDOUT,
                start_new_session=requires_root,
            )
        ctx.daemon = daemon
        if ctx.uses_darwin_sudo_daemon():
            ctx.wait_daemon_pid()

        # Wait until the program is observable before replaying, so live checks
        # cannot race startup: the daemon's configured listeners must accept and
        # its push subscription be in place; satpulsewb's URL and token must be
        # printed and its HTTP port up.
        if daemon.poll() is not None:
            raise RuntimeError(f"{program.name} exited at startup (code {daemon.returncode})")
        program.wait_ready(ctx)

        # A single replay then runs in the background while checks observe
        # the live program.
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
            err = plat.stop_daemon(
                daemon,
                process_group=requires_root,
                signaler=ctx.daemon_signaler(),
                final_grace=ctx.daemon_final_grace(),
            )
            if err is not None:
                raise RuntimeError(err)
            if daemon.returncode not in (0, -signal.SIGINT):
                raise RuntimeError(f"daemon exited with code {daemon.returncode} on SIGINT")
        for p in program.ports_to_free(ctx):
            if not port_free(p):
                raise RuntimeError(f"port {p} still in use after shutdown")
        # Scan the program log only now, so shutdown-time warnings/errors (and
        # any SIGQUIT goroutine dump) are included. The base allow-list is the
        # program's (the daemon's clock/detection warnings, or satpulsewb's);
        # a scenario adds its own ALLOWED_ERRORS on top (e.g. a push the caster
        # rejects, or the read error a device-loss scenario provokes).
        common.check_no_unexpected_errors(
            ctx, allowed=getattr(scen, "ALLOWED_ERRORS", ()), base=program.allowed_errors())
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
        ctx._transport.close()
        if daemon is not None and daemon.poll() is None:
            plat.stop_daemon(
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
        if scenario_requires_root(scen) and not plat.is_root() and not args.sudo
    ]
    runnable = [n for n in selected if n not in skipped_without_sudo]
    if args.sudo and not plat.is_root() and any(scenario_requires_root(s) for s in selected_mods.values()):
        if shutil.which("sudo") is None:
            print("--sudo requested, but sudo is not installed", file=sys.stderr)
            return 2
        p = subprocess.run(["sudo", "-n", "true"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
        if p.returncode != 0:
            err = p.stderr.decode("utf-8", "replace").strip()
            print("--sudo requested, but sudo -n true failed" + (f": {err}" if err else ""), file=sys.stderr)
            return 2

    if runnable:
        for b in ("satpulsed", "satpulsetool", "satpulsewb"):
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
