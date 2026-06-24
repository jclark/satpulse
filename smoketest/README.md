# satpulsed daemon smoke tests

Black-box smoke tests that run the real `satpulsed` binary, fed by
realtime packet-log replay through a FIFO, with no root and no GPS
hardware. They exercise daemon behaviour -- configuration wiring,
startup, observability endpoints, logging, Ntrip, and shutdown -- not
packet decoding (that is covered by package tests and
`plan/packet-testing.md`).

See `plan/smoke-test.md` for the design.

## Running

Build the binaries first, then run the suite:

```sh
make
python3 smoketest/run.py            # run all scenarios in parallel
python3 smoketest/run.py http/full  # run named scenarios
python3 smoketest/run.py --list     # list scenarios
python3 smoketest/run.py -j 1        # run serially
python3 smoketest/run.py --sudo     # use sudo -n for root-required scenarios
```

No third-party Python packages are required (standard library only).

Output is one line per scenario:

- `PASS` -- all checks passed and the daemon shut down cleanly.
- `FAIL` -- a check failed; the traceback is printed and the run
  directory is kept for inspection.
- `SKIP` -- a scenario needs root and neither root nor `--sudo` was used.
- `XFAIL` -- a scenario that declares `XFAIL = "<reason>"` failed as
  expected (e.g. a bug not yet fixed); it does not fail the suite.
- `XPASS` -- an `XFAIL` scenario unexpectedly passed; this *does* fail the
  suite, as a prompt to remove the `XFAIL` marker once the fix lands.

Root-required scenarios are skipped by default when the runner is not root.
Use `--sudo` to run them through passwordless `sudo -n`, as in CI. If `--sudo`
is requested and `sudo -n true` fails, the runner fails before starting any
scenario.

## How it works

For each scenario the runner (`run.py`):

1. allocates a resource block (a port range and per-run paths) so
   scenarios are parallel-safe;
2. renders the scenario's `<name>.toml.in` template, substituting the
   `SATPULSE_TEST_*` resource variables;
3. creates the serial input transport (a FIFO by default, or a pty for a
   scenario that needs to disconnect) and starts any required fake peers;
4. starts `satpulsed`;
5. starts a single `satpulsetool pack --realtime <factor>` replay of the
   scenario's packet log into that input, in the background;
6. calls the scenario's `run(ctx)`, which performs its checks while the
   replay flows (live checks such as SSE and Ntrip) and after it
   finishes (`ctx.wait_replay()`, then log and error checks);
7. stops `satpulsed` with `SIGINT` and verifies it exits and releases its
   ports -- except a self-shutdown scenario, which makes the daemon exit on
   its own (see Transports) and verifies that without sending a signal.

Only **one** replay runs per daemon lifetime: concatenating replays
corrupts the RTCM frame boundary at the junction, so live checks observe
the single replay rather than triggering their own.

`--realtime` takes a speedup factor: `1` reproduces the captured
inter-packet timing, larger values compress it. Scenarios use a factor
that keeps the replay flowing long enough for the live checks while
staying fast in CI.

## Transports

The serial input is one of two transports, chosen per scenario with the
`INPUT` attribute:

- **FIFO** (`INPUT` unset, the default) -- a read-only replay sink. satpulsed
  opens it `O_RDWR` and holds its own write end, so the daemon and the replayer
  can start and stop in any order and an idle FIFO looks like a silent-but-
  connected receiver. That convenience is also a limit: a FIFO can never look
  *disconnected*, so it cannot test what happens when the input goes away.
- **pty** (`INPUT = "pty"`) -- a real TTY (so satpulsed takes the same code path
  as a USB serial receiver) and full-duplex. Closing the master is a genuine
  disconnect: the slave reads fail and the scan worker exits. Being writable,
  it can also carry the daemon's own writes (the master is drained, and can be
  captured), which a read-only FIFO cannot -- this is what the `stream/pull`
  scenario uses.

A scenario whose daemon should exit on its own when the input disappears sets
`SELF_SHUTDOWN = True`. That requires a pty (only a pty can disconnect); the
runner then closes the master, expects the daemon to exit with no signal and a
restartable failure code, and reports a hang (goroutine dump via SIGQUIT) as a
failure. Using a pty does **not** imply `SELF_SHUTDOWN`: the `stream/pull`
write-path scenario uses a pty and still stops via the normal `SIGINT` path.

## Layout

```
smoketest/
  run.py              execution environment: resources, replay, shutdown
  common.py           checks and helpers shared across scenario families
  ntpshm.py           NTP SHM read/remove helper for root-required scenarios
  Makefile            Python dev tasks (typecheck, dev-deps, update-deps)
  pyproject.toml      dev-only Python tooling config
  scenarios/
    basic/minimal.py
    basic/minimal.toml.in
    proxy/__init__.py
    http/full.py
    http/full.toml.in
```

A scenario owns the meaning of its test. Each scenario ID is listed explicitly
in `run.py` as `family/name`, and maps to:

- `scenarios/family/name.py` -- scenario module
- `scenarios/family/name.toml.in` -- config template

The scenario module declares:

- `PACKET_LOG` -- packet log to replay (path relative to the repo root;
  the scenarios reuse logs under `gps/testdata/packets/`);
- `FACTOR` -- replay speedup factor;
- `run(ctx)` -- the checks to perform, using helpers from `common.py` and
  the owning scenario-family package.

Checks used only by one scenario family live in that family's package, e.g.
`scenarios/ntrip/__init__.py` or `scenarios/proxy/__init__.py`. Keep
top-level `common.py` for checks and helpers that are genuinely common across
scenario families.

## Development checks

Running smoke tests has no Python dependency beyond the standard library. Dev
tools are scoped to `smoketest/`:

```sh
cd smoketest
make dev-deps
make typecheck
make update-deps
```

`make typecheck` runs strict mypy over the explicit Python file list in
`smoketest/Makefile`.

## Scenarios

- `basic/minimal` -- minimal config: serial input only; just start, replay,
  shut down.
- `logging/all` -- event, track, and packet logs written with expected content.
- `http/full` -- default HTTP endpoint: `/position`, `/metrics`, GUI HTML, SSE.
- `http/disabled` -- HTTP endpoint with GUI and metrics off, position
  only; also guards clean shutdown for GUI-disabled endpoints.
- `ntrip/basic` -- Ntrip caster source table and RTCM streaming.
- `ntrip/auth` -- Ntrip caster with an authenticated mountpoint.
- `ntp/sock` -- chrony SOCK refclock: a pure 1 Hz RMC stream drives serial timing
  mode, and the samples are well-formed, consistently timestamped, and carry
  the correct GPS time.
- `ntp/shm` -- ntpd/NTPsec SHM refclock: a pure 1 Hz RMC stream drives serial
  timing mode, and the configured SHM segment receives a valid mode-1 sample.
  This scenario requires root and is skipped unless run as root or with
  `--sudo`; the SHM reader helper also needs the system `libatomic` library.
- `proxy/tcp` -- read-only TCP serial proxies with protocol filters.
- `proxy/socket` -- read-only Unix-socket serial proxy with a protocol filter.
- `stream/push-ntrip` -- Ntrip push: the daemon forwards the log's RTCM to a
  remote caster (the pushed stream matches the source log's RTCM), and a second
  push entry with a wrong password is permanently rejected, so the daemon gives
  up on it rather than reconnecting forever.
- `stream/push-udp` -- UDP push: the daemon forwards the log's packet bytes to
  a remote UDP receiver.
- `stream/pull` -- Ntrip pull: the daemon pulls RTCM MSM4 corrections from a
  fake correction source and writes them back to the receiver over the serial
  port; the captured serial writes match the source's RTCM. The only scenario
  that captures the daemon's serial writes (`CAPTURE_WRITES`) and uses the pty
  as a write path rather than to model a disconnect, so it stops via `SIGINT`.
- `stream/vrs` -- Ntrip VRS pull: the fake correction source waits for a
  post-handshake GGA before streaming RTCM corrections, and the daemon's serial
  writes still match the source RTCM.
- `shutdown/serial-loss` -- the serial input disappears (a pty whose master is
  closed mid-run) and the daemon must shut down on its own and exit with a
  restartable code, with an HTTP endpoint configured. Guards the scan-worker-
  exit -> daemon-shutdown path (issue #172); the only scenario using the pty
  transport and `SELF_SHUTDOWN`.

The Ntrip caster scenarios use `satpulsetool ntrip` as the client. The
`stream/push-ntrip` scenario uses the built-in Ntrip fake caster
(`scenarios/ntrip/fakecaster.py`) as the remote peer, so it needs no external
dependency: it accepts the daemon's Ntrip v1 SOURCE feed and captures the
payload, which the check scans back into RTCM. The `stream/push-udp` scenario
uses the built-in UDP receiver (`scenarios/stream/fakeudp.py`) as its remote
peer. The `stream/pull` scenario uses the matching fake correction source
(`scenarios/stream/fakesource.py`): it
answers the daemon's Ntrip v1 GET and streams an RTCM log, which the daemon
writes back to the receiver over the pty write path that a read-only FIFO
cannot provide. A real-peer variant using `str2str` from RTKLIB could be added
later for either.

## Installed systemd environment

The plan also describes an installed/systemd environment that runs the
same scenarios as root under `satpulse@gps.service`, driven from
`systest`. That environment is not implemented yet; the direct
environment here is the first version.
