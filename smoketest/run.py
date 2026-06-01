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
import subprocess
import sys
import tempfile
import time
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
}
PORT_BLOCK = 16
PORT_BASE = 41000


def arch():
    m = platform.machine()
    return {"x86_64": "amd64", "aarch64": "arm64"}.get(m, m)


def bin_path(name):
    return os.path.join(REPO, "out", arch(), name)


class Context:
    """Resolved resources and helpers passed to a scenario's run(ctx)."""

    def __init__(self, name, run_dir, env, factor, packet_log, daemon_log, has_http, has_ntrip):
        self.name = name
        self.run_dir = run_dir
        self.env = env
        self.factor = factor
        self.packet_log = packet_log
        self.daemon_log = daemon_log
        self.has_http = has_http
        self.has_ntrip = has_ntrip
        self.replay_err = os.path.join(run_dir, "replay.err")
        self.satpulsed = bin_path("satpulsed")
        self.satpulsetool = bin_path("satpulsetool")
        self.daemon = None
        self.replay_proc = None

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


def allocate_env(name, index, run_dir):
    base = PORT_BASE + index * PORT_BLOCK
    log_dir = os.path.join(run_dir, "log")
    env = {
        "SATPULSE_TEST_RUN_DIR": run_dir,
        "SATPULSE_TEST_CONFIG": os.path.join(run_dir, "satpulse.toml"),
        "SATPULSE_TEST_FIFO": os.path.join(run_dir, "gps.fifo"),
        "SATPULSE_TEST_LOG_DIR": log_dir,
        "SATPULSE_TEST_PROXY_SOCKET": os.path.join(run_dir, "proxy.sock"),
    }
    for key, off in PORT_OFFSETS.items():
        env[key] = str(base + off)
    return env


def render_config(template_path, out_path, env):
    """Render the config template; report which listeners it configures.

    Returns (has_http, has_ntrip), detected from non-comment table headers
    so a comment that merely mentions a section is not mistaken for it.
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
    return "[[http]]" in headers, "[ntrip]" in headers


def port_free(port):
    import socket

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


def run_scenario(name, index):
    scen = load_scenario(name)
    factor = getattr(scen, "FACTOR", 5)
    packet_log = scen.PACKET_LOG
    if not os.path.isabs(packet_log):
        packet_log = os.path.join(REPO, packet_log)
    config_tmpl = os.path.join(SCENARIOS_DIR, name, "satpulse.toml.in")

    run_dir = tempfile.mkdtemp(prefix=f"satpulse-smoke-{name}-")
    env = allocate_env(name, index, run_dir)
    env["SATPULSE_TEST_PACKET_LOG"] = packet_log
    os.makedirs(env["SATPULSE_TEST_LOG_DIR"], exist_ok=True)
    os.mkfifo(env["SATPULSE_TEST_FIFO"])
    has_http, has_ntrip = render_config(config_tmpl, env["SATPULSE_TEST_CONFIG"], env)

    daemon_log = os.path.join(run_dir, "satpulsed.log")
    ctx = Context(name, run_dir, env, factor, packet_log, daemon_log, has_http, has_ntrip)

    daemon = None
    keep = False
    try:
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
        # (and any SIGQUIT goroutine dump) are included.
        checks.check_no_unexpected_errors(ctx)
        return (name, True, "")
    except Exception:
        keep = True
        return (name, False, traceback.format_exc())
    finally:
        if ctx.replay_proc is not None and ctx.replay_proc.poll() is None:
            ctx.replay_proc.kill()
        if daemon is not None and daemon.poll() is None:
            stop_daemon(daemon)
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
        futs = {ex.submit(run_scenario, n, i): n for i, n in enumerate(selected)}
        for fut in concurrent.futures.as_completed(futs):
            results.append(fut.result())

    results.sort()
    failed = 0
    for name, ok, detail in results:
        if ok:
            print(f"PASS {name}")
        else:
            failed += 1
            print(f"FAIL {name}")
            print("    " + detail.replace("\n", "\n    ").rstrip())
    print(f"\n{len(results) - failed}/{len(results)} scenarios passed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
