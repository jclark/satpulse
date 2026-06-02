#!/usr/bin/env python3
"""Daemon smoke test runner (direct environment).

Runs real satpulsed binaries fed by realtime packet-log replay through a
FIFO, with no root and no GPS hardware. See plan/smoke-test.md.

Each scenario lives in scenarios/<name>/ as:
  - satpulse.toml.in : config template using ${SATPULSE_TEST_*} variables
  - scenario.py      : defines PACKET_LOG, FACTOR, and run(ctx)

The runner allocates a resource block (ports, paths) per scenario, renders
the config, starts the daemon, replays the packet log into the FIFO, then
calls the scenario's run(ctx) to perform its checks. Scenarios are
parallel-safe and run concurrently by default.
"""

import argparse
import concurrent.futures
import errno
import fcntl
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

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)  # let run.py and scenarios import the shared checks module
REPO = os.path.dirname(HERE)

import checks  # noqa: E402  (after sys.path is set up)
SCENARIOS_DIR = os.path.join(HERE, "scenarios")

# Named resources mapped to offsets within each scenario's port block.
PORT_OFFSETS = {
    "SATPULSE_TEST_HTTP_PORT": 0,
    "SATPULSE_TEST_NTRIP_PORT": 1,
    "SATPULSE_TEST_PROXY_TCP_PORT": 2,
    "SATPULSE_TEST_PROXY_TCP_RTCM_PORT": 3,
    "SATPULSE_TEST_REMOTE_CASTER_PORT": 4,
    "SATPULSE_TEST_TOOL_PORT": 5,
    "SATPULSE_TEST_REMOTE_CASTER_PORT2": 6,
}
PORT_BLOCK = 16


def _ephemeral_floor():
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


def _port_bindable(port):
    """True if a fresh TCP listener can bind 127.0.0.1:port right now."""
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        s.bind(("127.0.0.1", port))
        return True
    except OSError:
        return False
    finally:
        s.close()


def alloc_port_block():
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


def emit(msg):
    """Print a progress line atomically across parallel worker threads."""
    with _out_lock:
        print(msg, flush=True)


def arch():
    m = platform.machine()
    return {"x86_64": "amd64", "aarch64": "arm64"}.get(m, m)


def bin_path(name):
    return os.path.join(REPO, "out", arch(), name)


class Context:
    """Resolved resources and helpers passed to a scenario's run(ctx)."""

    def __init__(self, name, run_dir, env, factor, packet_log, daemon_log, has_http, has_ntrip, has_ntp, has_push):
        self.name = name
        self.run_dir = run_dir
        self.env = env
        self.factor = factor
        self.packet_log = packet_log
        self.daemon_log = daemon_log
        self.has_http = has_http
        self.has_ntrip = has_ntrip
        self.has_ntp = has_ntp
        self.has_push = has_push
        self.replay_err = os.path.join(run_dir, "replay.err")
        self.satpulsed = bin_path("satpulsed")
        self.satpulsetool = bin_path("satpulsetool")
        self.daemon = None
        self.replay_proc = None
        # Chrony SOCK consumer: a separate ntpsock.py process binds the socket
        # before the daemon starts and logs received samples to ntp_log.
        self.ntp_log = os.path.join(run_dir, "ntp.jsonl")
        self.ntp_proc = None
        # Fake Ntrip casters: one fakecaster.py process per [[stream.push]]
        # entry, each listening before the daemon starts. The first entry's
        # caster accepts and appends the pushed payload to caster_capture; their
        # diagnostics share caster_log.
        self.caster_capture = os.path.join(run_dir, "pushed.bin")
        self.caster_log = os.path.join(run_dir, "caster.log")
        self.caster_procs = []
        self._caster_log_file = None

    @property
    def fifo(self):
        return self.env["SATPULSE_TEST_FIFO"]

    @property
    def log_dir(self):
        return self.env["SATPULSE_TEST_LOG_DIR"]

    def port(self, key):
        return int(self.env[key])

    @property
    def http_port(self):
        return self.port("SATPULSE_TEST_HTTP_PORT")

    @property
    def ntrip_port(self):
        return self.port("SATPULSE_TEST_NTRIP_PORT")

    @property
    def proxy_socket(self):
        return self.env["SATPULSE_TEST_PROXY_SOCKET"]

    @property
    def ntp_sock(self):
        return self.env["SATPULSE_TEST_NTP_SOCK"]

    def start_ntp_sock(self, timeout=5):
        """Spawn the chrony SOCK consumer and wait for it to bind the socket.

        satpulsed plays the sender: it sends to this path and warns if no one
        is listening, so the consumer must be bound before the daemon starts.
        Binding an AF_UNIX datagram socket creates the path, so poll for it.
        """
        f = open(self.ntp_log, "wb")
        self.ntp_proc = subprocess.Popen(
            [sys.executable, os.path.join(HERE, "ntpsock.py"), "-f", "json", self.ntp_sock],
            stdout=f, stderr=subprocess.STDOUT,
        )
        self.ntp_proc._out = f
        deadline = time.time() + timeout
        while not os.path.exists(self.ntp_sock):
            if self.ntp_proc.poll() is not None:
                raise RuntimeError("ntp consumer exited before binding the socket")
            if time.time() >= deadline:
                raise RuntimeError(f"ntp consumer did not bind {self.ntp_sock} within {timeout}s")
            time.sleep(0.02)

    def stop_ntp_sock(self):
        if self.ntp_proc is None:
            return
        if self.ntp_proc.poll() is None:
            self.ntp_proc.terminate()
            try:
                self.ntp_proc.wait(timeout=2)
            except subprocess.TimeoutExpired:
                self.ntp_proc.kill()
        self.ntp_proc._out.close()

    def start_caster(self, timeout=5):
        """Start one fake Ntrip caster per [[stream.push]] entry, before the daemon.

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
        good = push[0]["ntrip"]
        self._caster_log_file = open(self.caster_log, "wb")
        ports = []
        for i, entry in enumerate(push):
            port = int(entry["ntrip"]["address"].rsplit(":", 1)[1])
            ports.append(port)
            capture = self.caster_capture if i == 0 else os.path.join(self.run_dir, f"pushed-{i}.bin")
            self.caster_procs.append(subprocess.Popen(
                [sys.executable, os.path.join(HERE, "fakecaster.py"),
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

    def wait_push(self, timeout=15):
        """Wait until the daemon has completed its SOURCE handshake to the caster.

        Gates replay: Push subscribes to the packet bcast as it connects, so
        waiting for the caster to accept the SOURCE feed guarantees the
        subscription exists before any packet flows. Without this, a fast
        replay could publish packets the push subscriber never sees, and the
        captured stream would be missing its leading packets.
        """
        deadline = time.time() + timeout
        while True:
            try:
                with open(self.caster_log, errors="replace") as fh:
                    if "accepted SOURCE" in fh.read():
                        return
            except OSError:
                pass
            if self.daemon is not None and self.daemon.poll() is not None:
                raise RuntimeError("daemon exited before pushing to the caster")
            if time.time() >= deadline:
                raise RuntimeError(f"daemon did not push to the caster within {timeout}s")
            time.sleep(0.05)

    def stop_caster(self):
        for p in self.caster_procs:
            if p.poll() is None:
                p.terminate()
                try:
                    p.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    p.kill()
        if self._caster_log_file is not None:
            self._caster_log_file.close()

    def http_url(self, path):
        return f"http://127.0.0.1:{self.http_port}{path}"

    def wait_listeners(self, timeout=15):
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

    def start_replay(self, open_timeout=15):
        """Start the single packet-log replay into the FIFO in the background.

        Exactly one replay runs per daemon lifetime: concatenating replays
        corrupts the RTCM frame boundary at the junction, so live checks
        (SSE, Ntrip streaming) must observe this one replay while it flows
        rather than triggering their own bursts. pack's stderr is captured
        so wait_replay can surface a malformed-log or broken-replay failure.
        """
        f = self._open_fifo_write(open_timeout)
        cmd = [self.satpulsetool, "pack", "--realtime", str(self.factor), self.packet_log]
        errf = open(self.replay_err, "wb")
        p = subprocess.Popen(cmd, stdout=f, stderr=errf)
        p._fifo = f  # keep the write fd alive for the process lifetime
        p._errf = errf
        self.replay_proc = p
        return p

    def _open_fifo_write(self, timeout):
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
                fd = os.open(self.fifo, os.O_WRONLY | os.O_NONBLOCK)
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

    def wait_replay(self, timeout=60):
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
        if rc != 0:
            raise RuntimeError(f"replay (pack) exited with code {rc}: {self._replay_stderr()}")

    def _replay_stderr(self):
        try:
            with open(self.replay_err, errors="replace") as fh:
                return fh.read().strip()[-2000:] or "(no stderr)"
        except OSError:
            return "(no stderr)"


def load_scenario(name):
    path = os.path.join(SCENARIOS_DIR, name, "scenario.py")
    spec = importlib.util.spec_from_file_location(f"scenario_{name}", path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def allocate_env(name, run_dir):
    base = alloc_port_block()
    log_dir = os.path.join(run_dir, "log")
    env = {
        "SATPULSE_TEST_RUN_DIR": run_dir,
        "SATPULSE_TEST_CONFIG": os.path.join(run_dir, "satpulse.toml"),
        "SATPULSE_TEST_FIFO": os.path.join(run_dir, "gps.fifo"),
        "SATPULSE_TEST_LOG_DIR": log_dir,
        "SATPULSE_TEST_PROXY_SOCKET": os.path.join(run_dir, "proxy.sock"),
        "SATPULSE_TEST_NTP_SOCK": os.path.join(run_dir, "chrony.sock"),
    }
    for key, off in PORT_OFFSETS.items():
        env[key] = str(base + off)
    return env


def render_config(template_path, out_path, env):
    """Render the config template; report which listeners it configures.

    Returns (has_http, has_ntrip, has_ntp, has_push), detected from non-comment
    table headers so a comment that merely mentions a section is not mistaken
    for it.
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
    return ("[[http]]" in headers, "[ntrip]" in headers, "[ntp]" in headers,
            "[[stream.push]]" in headers)


def port_free(port):
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=0.3):
            return False
    except OSError:
        return True


def stop_daemon(daemon, grace=5.0):
    """Stop the daemon, escalating the way the systemd unit does.

    satpulsed catches both SIGINT and SIGTERM and treats them as the same
    graceful-shutdown request, so SIGTERM cannot rescue a hung shutdown.
    The packaged unit uses TimeoutStopSec=5 with FinalKillSignal=SIGQUIT,
    so on a hang we escalate to SIGQUIT and stop there: the daemon does not
    catch it, and Go's default handler dumps all goroutine stacks (to the
    daemon log) and aborts the process -- both the diagnostic we want for a
    shutdown hang and a reliable terminator. SIGQUIT is the final backstop;
    no SIGKILL follows.

    Returns None on a clean SIGINT exit, otherwise an error string
    describing the escalation that was needed.
    """
    daemon.send_signal(signal.SIGINT)
    try:
        daemon.wait(timeout=grace)
        return None
    except subprocess.TimeoutExpired:
        pass
    daemon.send_signal(signal.SIGQUIT)  # dumps goroutines to the daemon log, then aborts
    daemon.wait()
    return (
        f"daemon did not exit within {grace:g}s of SIGINT; "
        "SIGQUIT dumped goroutines (see satpulsed.log) and aborted it"
    )


def run_scenario(name):
    emit(f"START {name}")
    scen = load_scenario(name)
    factor = getattr(scen, "FACTOR", 5)
    packet_log = scen.PACKET_LOG
    if not os.path.isabs(packet_log):
        packet_log = os.path.join(REPO, packet_log)
    config_tmpl = os.path.join(SCENARIOS_DIR, name, "satpulse.toml.in")

    run_dir = tempfile.mkdtemp(prefix=f"satpulse-smoke-{name}-")
    env = allocate_env(name, run_dir)
    env["SATPULSE_TEST_PACKET_LOG"] = packet_log
    os.makedirs(env["SATPULSE_TEST_LOG_DIR"], exist_ok=True)
    os.mkfifo(env["SATPULSE_TEST_FIFO"])
    has_http, has_ntrip, has_ntp, has_push = render_config(config_tmpl, env["SATPULSE_TEST_CONFIG"], env)

    daemon_log = os.path.join(run_dir, "satpulsed.log")
    ctx = Context(name, run_dir, env, factor, packet_log, daemon_log, has_http, has_ntrip, has_ntp, has_push)

    daemon = None
    keep = False
    try:
        # Bind the chrony SOCK consumer before the daemon starts, so its first
        # refclock sample lands in a listening socket rather than warning.
        if has_ntp:
            ctx.start_ntp_sock()
        # Start the fake Ntrip caster before the daemon, so the daemon's push
        # connect succeeds immediately instead of failing and backing off.
        if has_push:
            ctx.start_caster()
        with open(daemon_log, "wb") as out:
            daemon = subprocess.Popen(
                [ctx.satpulsed, "-v", "-f", env["SATPULSE_TEST_CONFIG"]],
                stdout=out,
                stderr=subprocess.STDOUT,
            )
        ctx.daemon = daemon

        # Wait for the daemon's listeners before replaying so the HTTP/SSE
        # and Ntrip observers exist before any packet arrives; otherwise a
        # fast replay could be consumed during the pre-listener detection
        # phase and live checks would race startup.
        if daemon.poll() is not None:
            raise RuntimeError(f"daemon exited at startup (code {daemon.returncode})")
        ctx.wait_listeners()
        # Push has no listener of its own; wait for its outbound SOURCE feed so
        # the bcast subscription is in place before replay, like wait_listeners
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

        # Graceful shutdown: SIGINT should terminate the daemon promptly
        # and release its ports. A hang escalates to SIGQUIT (goroutine
        # dump) and is reported as a failure.
        err = stop_daemon(daemon)
        if err is not None:
            raise RuntimeError(err)
        if daemon.returncode not in (0, -signal.SIGINT):
            raise RuntimeError(f"daemon exited with code {daemon.returncode} on SIGINT")
        if ctx.has_http and not port_free(ctx.http_port):
            raise RuntimeError(f"HTTP port {ctx.http_port} still in use after shutdown")
        # Scan the daemon log only now, so shutdown-time warnings/errors
        # (and any SIGQUIT goroutine dump) are included. A scenario may declare
        # ALLOWED_ERRORS for error lines it expects (e.g. a push it knows the
        # caster rejects).
        checks.check_no_unexpected_errors(ctx, allowed=getattr(scen, "ALLOWED_ERRORS", ()))
        return (name, True, "")
    except Exception:
        keep = True
        return (name, False, traceback.format_exc())
    finally:
        if ctx.replay_proc is not None and ctx.replay_proc.poll() is None:
            ctx.replay_proc.kill()
        if daemon is not None and daemon.poll() is None:
            stop_daemon(daemon)
        ctx.stop_ntp_sock()
        ctx.stop_caster()
        if keep:
            print(f"[{name}] artifacts kept in {run_dir}", file=sys.stderr)
        else:
            shutil.rmtree(run_dir, ignore_errors=True)


def discover_scenarios():
    names = []
    for entry in sorted(os.listdir(SCENARIOS_DIR)):
        if os.path.isfile(os.path.join(SCENARIOS_DIR, entry, "scenario.py")):
            names.append(entry)
    return names


def main():
    ap = argparse.ArgumentParser(description="satpulsed daemon smoke tests")
    ap.add_argument("scenarios", nargs="*", help="scenario names (default: all)")
    ap.add_argument("-j", "--jobs", type=int, default=os.cpu_count() or 4)
    ap.add_argument("-l", "--list", action="store_true", help="list scenarios and exit")
    args = ap.parse_args()

    available = discover_scenarios()
    if args.list:
        for n in available:
            print(n)
        return 0

    selected = args.scenarios or available
    unknown = [n for n in selected if n not in available]
    if unknown:
        print(f"unknown scenarios: {', '.join(unknown)}", file=sys.stderr)
        return 2

    for b in ("satpulsed", "satpulsetool"):
        if not os.path.exists(bin_path(b)):
            print(f"missing binary {bin_path(b)}; run make first", file=sys.stderr)
            return 2

    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.jobs) as ex:
        futs = {ex.submit(run_scenario, n): n for n in selected}
        for fut in concurrent.futures.as_completed(futs):
            name, ok, detail = fut.result()
            results.append((name, ok, detail))
            emit(f"PASS {name}" if ok else f"FAIL {name}")

    failed = sorted((name, detail) for name, ok, detail in results if not ok)
    for name, detail in failed:
        print(f"\n--- {name} ---")
        print("    " + detail.replace("\n", "\n    ").rstrip())
    print(f"\n{len(results) - len(failed)}/{len(results)} scenarios passed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
