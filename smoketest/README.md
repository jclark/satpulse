# satpulse daemon and workbench smoke tests

Black-box smoke tests that run the real `satpulsed` and `satpulsewb`
binaries, fed by realtime packet-log replay through a FIFO or pty (or the
u-blox simulator for config scenarios), with no root and no GPS hardware.
They exercise program behaviour -- configuration wiring, startup,
observability endpoints, logging, Ntrip, corrections, and shutdown -- not
packet decoding (that is covered by package tests and
`plan/packet-testing.md`).

The program under test is a scenario dimension: `satpulsed` by default,
`satpulsewb` (SatPulse Workbench, the browser GUI over `gps/app/session`)
when a scenario sets `PROGRAM = "satpulsewb"`. What plays the receiver is
a second, orthogonal dimension: a packet-log replay by default, the u-blox
simulator when a scenario sets `PROVIDER = "ubxsim"` (see Program under
test and Packet provider below).

See `plan/smoke-test.md` for the design, and the Delivery section of
`plan/satpulseweb.md` for the workbench phase.

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

On macOS and FreeBSD, build with `./unix-build.sh` instead of `make`; the
runner uses `out/<goos>_<goarch>/`, matching the build script.

When `GOOS` or `GOARCH` is set, the runner uses those values to choose the
binary directory. For example, `GOOS=freebsd GOARCH=amd64 python3
smoketest/run.py --list` checks `out/freebsd_amd64/`.

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
2. asks the program (see Program under test) to prepare its input,
   substituting the `SATPULSE_TEST_*` resource variables: satpulsed
   renders `<name>.toml.in`, satpulsewb renders `<name>.args.in`;
3. sets up the packet source (see Packet provider) and points
   `SATPULSE_TEST_SERIAL` at it -- the serial input transport (a FIFO by
   default, or a pty for a scenario that needs to disconnect) for a replay
   scenario, or the u-blox simulator for a `PROVIDER = "ubxsim"` scenario -- and
   starts any required fake peers;
4. starts the program (`satpulsed` or `satpulsewb`);
5. starts the provider's feed: a replay scenario runs a single
   `satpulsetool pack --realtime <factor>` replay of its packet log into the
   input, in the background; a `ubxsim` scenario needs no separate replay (the
   simulator generates nav itself);
6. calls the scenario's `run(ctx)`, which performs its checks while the
   replay flows (live checks such as SSE and Ntrip) and after it
   finishes (`ctx.wait_replay()`, then log and error checks);
7. stops the program with `SIGINT` and verifies it exits and releases its
   ports -- except a self-shutdown scenario, which makes the daemon exit on
   its own (see Transports) and verifies that without sending a signal.

Only **one** replay runs per program lifetime: concatenating replays
corrupts the RTCM frame boundary at the junction, so live checks observe
the single replay rather than triggering their own.

`--realtime` takes a speedup factor: `1` reproduces the captured
inter-packet timing, larger values compress it. Scenarios use a factor
that keeps the replay flowing long enough for the live checks while
staying fast in CI.

## Transports

A scenario does not name a transport; it declares the capabilities it needs and
the runner picks one. The two capabilities are read-write (to capture the
daemon's own writes) and disconnectable (to model the input going away), and on
Unix they map to one of two transports:

- **FIFO** (the default, no capability requested) -- a read-only replay sink.
  satpulsed opens it `O_RDWR` and holds its own write end, so the daemon and the
  replayer can start and stop in any order and an idle FIFO looks like a silent-
  but-connected receiver. That convenience is also a limit: a FIFO can never
  look *disconnected*, so it cannot test what happens when the input goes away.
- **pty** (selected when a capability is requested) -- a real TTY (so satpulsed
  takes the same code path as a USB serial receiver) and full-duplex. Closing
  the master is a genuine disconnect: the slave reads fail and the scan worker
  exits. Being writable, it can also carry the daemon's own writes (the master
  is drained, and can be captured), which a read-only FIFO cannot -- this is
  what the `stream/pull-*` scenarios use.

The two capabilities are orthogonal. A scenario that captures the program's
writes sets `CAPTURE_WRITES = True` (read-write). A scenario whose daemon should
exit on its own when the input disappears sets `SELF_SHUTDOWN = True`
(disconnectable); the runner drops the input, expects the daemon to exit with no
signal and a restartable failure code, and reports a hang (goroutine dump via
SIGQUIT) as a failure. Neither implies the other: the `stream/pull-*` write-path
scenarios are read-write yet still stop via the normal `SIGINT` path.
`DISCONNECTABLE = True` requests the pty's disconnect capability *without* the
self-shutdown check, for `satpulsewb`'s device-loss scenario, which asserts the
program keeps running (the inverse of `SELF_SHUTDOWN`).

## Program under test

Like the transport, the program is a scenario dimension the runner dispatches to
polymorphically. `program_api.py` defines the interface; `program_satpulsed.py`
and `program_satpulsewb.py` implement it. This is the program counterpart to the
per-OS `platform_*` seam, but chosen per scenario (so both coexist in one
process) rather than per process. The Program owns exactly what differs between
the two programs:

- **input preparation** -- satpulsed renders a `<name>.toml.in` config and
  derives its peers/listeners from it; satpulsewb renders a `<name>.args.in`
  flag list (one token per line, same `${SATPULSE_TEST_*}` substitution) and
  declares its peers explicitly (`CORRECTION_SOURCE`);
- **the start command** and **readiness** -- satpulsed waits for its configured
  listeners; satpulsewb parses the URL and access token it prints to stdout and
  waits for its one HTTP port;
- the **base allow-list** for the log error scan, and the **ports to free**
  after shutdown.

Everything else -- port allocation, run dirs, the fake peers, and the parallel
executor -- stays shared in `run.py`. Scenario families are organised by feature,
not by program, and hold scenarios for both side by side: the workbench
corrections scenario lives beside the daemon's `stream/pull-*` ones and reuses
the family's captured-serial-writes RTCM check as-is.

## Packet provider

What plays the receiver behind `SATPULSE_TEST_SERIAL` is a second scenario
dimension, orthogonal to the program under test, chosen with `PROVIDER` (default
`replay`). It composes with the program: `satpulsed x ubxsim` smoke-tests the
daemon's startup config phase, which no replay can reach. `provider_api.py`
defines the seam (a `Provider` Protocol plus `select(name)`); `provider_replay.py`
and `provider_ubxsim.py` implement it, chosen per scenario like a Program. The
provider owns how receiver bytes are produced and consumed -- the serial
endpoint, the feed lifecycle, and its own source attributes -- and nothing else,
so `run.py` carries no `PROVIDER ==` branch:

- **replay** (the default) -- a recorded packet log streamed in real time. It
  owns transport selection (from the scenario's capabilities, see Transports),
  the single `pack --realtime` replay, the one-replay-per-lifetime invariant, and
  the `PACKET_LOG`/`FACTOR` attributes. Every existing scenario is a replay
  scenario and keeps `PACKET_LOG`/`FACTOR` unchanged.
- **ubxsim** (`PROVIDER = "ubxsim"`) -- the u-blox receiver simulator behind a
  pty (`satpulsetool ubxsim`), the one source that answers probes and config, so
  the config path (probe identification, `ReadConfig`, `ApplyConfig`, and the
  daemon's startup config phase) gets black-box coverage a read-only replay
  cannot. The scenario declares `PERSONALITY` (a recorded MON-VER + Default-layer
  dump) and an optional `SIM_REPLAY` nav bank; `ctx.factor` defaults to `1`. The
  provider spawns the simulator before the program (the pty must exist when the
  program opens the device) and SIGTERMs it only after the program has shut down
  (the simulator holds its own slave fd open, so program restarts never EOF it,
  while an early kill would inject read errors into the program's shutdown). It
  rejects `CAPTURE_WRITES`/`SELF_SHUTDOWN`/`DISCONNECTABLE` (write capture is
  meaningless when the simulator answers the writes; device-loss stays with the
  replay provider) and reports the scenario unsupported (a SKIP) off Linux/macOS,
  where `satpulsetool ubxsim` does not build. Its stdout+stderr land in
  `ubxsim.log` in the run dir, kept on failure and not error-scanned -- the
  simulator is a test double, not a program under test.

satpulsewb scenarios cannot use the no-argument default (it binds a fixed port
on all interfaces, which is neither parallel-safe nor in the allocated block),
so they pass `-L 127.0.0.1:${SATPULSE_TEST_HTTP_PORT}` to bind the allocated
port; the exception is `http/wb-default`, the one scenario that exercises the
real default-port path (and its OS-picked fallback) and parses whatever port the
program prints.

## Layout

```
smoketest/
  run.py                    execution environment: resources, lifecycle, shutdown
  common.py                 checks and helpers shared across scenario families
  program_api.py            the program-under-test seam (Protocol + select)
  program_satpulsed.py      satpulsed: config input, config-derived peers
  program_satpulsewb.py     satpulsewb: flag-list input, URL/token readiness
  provider_api.py           the packet-provider seam (Protocol + select)
  provider_replay.py        replay: transport selection + the pack replay
  provider_ubxsim.py        ubxsim: the u-blox simulator behind a pty
  platform_api.py           the serial-transport seam (Protocol)
  platform_unix.py          Unix transports (FIFO/pty), shutdown, privilege
  ntpshm.py                 NTP SHM read/remove helper for root-required scenarios
  Makefile                  Python dev tasks (typecheck, dev-deps, update-deps)
  pyproject.toml            dev-only Python tooling config
  scenarios/
    basic/minimal.py
    basic/minimal.toml.in
    http/full.py
    http/full.toml.in
    http/wb-default.py
    http/wb-default.args.in
```

A scenario owns the meaning of its test. Each scenario ID is listed explicitly
in `run.py` as `family/name`, and maps to a module and an input template:

- `scenarios/family/name.py` -- scenario module
- `scenarios/family/name.toml.in` -- config template (satpulsed), or
- `scenarios/family/name.args.in` -- flag list (satpulsewb)

The scenario module declares:

- `PACKET_LOG` -- packet log to replay (path relative to the repo root;
  the scenarios reuse logs under `gps/testdata/packets/`); a replay-provider
  attribute (omit it for a `ubxsim` scenario);
- `FACTOR` -- replay speedup factor; likewise replay-only;
- `run(ctx)` -- the checks to perform, using helpers from `common.py` and
  the owning scenario-family package;
- `ENV` -- optional environment variables for the program under test, with
  `${SATPULSE_TEST_*}` substitutions available in their values;
- `PROGRAM = "satpulsewb"` -- optional; selects the workbench program (default
  `satpulsed`). A workbench scenario may also declare `CORRECTION_SOURCE` (a fake
  correction source the runner starts, for a corrections scenario);
- `PROVIDER = "ubxsim"` -- optional; selects the u-blox simulator as the packet
  source (default `replay`, see Packet provider). A `ubxsim` scenario declares
  `PERSONALITY` (repo-relative) and an optional `SIM_REPLAY` nav bank instead of
  `PACKET_LOG`/`FACTOR`.

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
- `http/multiple` -- two `[[http]]` endpoints with independent config (a full
  GUI endpoint and a position-only one); both serve concurrently and each
  reflects its own table.
- `http/wb-default` (satpulsewb) -- the workbench with no `-L`: the real
  default-port bind (with OS-picked fallback), the printed URL and generated
  token, the browser auto-open vetoed by SSH_CONNECTION (asserting no launch
  line), the SPA served at
  `/` (HTML plus its script bundle, the counterpart to
  the daemon's `check_html`), token auth enforced, the snapshot endpoints
  populating, SSE monitor delivery, packet-stream gating (a `?stream=packets`
  client), late-joiner priming from the event cache, and the writer-seat
  takeover (a second seat claim delivers a fresh `writer` grant on the first,
  still-open stream and 410s a POST from the superseded seat).
- `http/wb-listen` (satpulsewb) -- the workbench with `-L` and the token
  disabled: the SPA is served, the API is open, but the CSRF content-type gate
  still rejects a cross-site simple POST. A fixture `SATPULSE_GPSMSG_PATH`
  verifies message-file catalog and selection wiring through the real binary.
- `http/wb-survey` (satpulsewb) -- survey-message priming: an F9P survey-in
  capture (`NAV-SVIN`) drives `gps:msg` kind `survey`, and after the replay ends
  a late SSE client is still primed with it from the hub's sticky-kind cache
  (the slow-changing state a reloaded browser must not have to wait for).
- `config/startup` (satpulsed x ubxsim) -- the daemon's startup config phase
  against the u-blox simulator: active detection carries the personality's
  MON-VER model and firmware, and the startup ConfigTarget landing shows up as
  NAV-SAT (off in the personality defaults) becoming live satellite data.
- `config/wb-apply` (satpulsewb x ubxsim) -- the interactive config path,
  UI-shaped: connect to the simulator's pty, identify the receiver, read the
  config, then apply ConfigTargets carrying a round-trippable property (the
  antenna cable delay) and every message flag group Opts has, and confirm they
  land -- a re-read shows the delay, and the packet stream shows the receiver
  sending what was enabled (NAV-TIMELS, NAV-SAT/NAV-SIG and RTCM MSM4, all off in
  the personality defaults) and dropping what was not (NMEA, which defaults on,
  is disabled with the empty array, shown gone, then re-enabled by name).
- `ntrip/basic` -- Ntrip caster source table and RTCM streaming; the source
  table's shared STR fields show their defaults.
- `ntrip/auth` -- Ntrip caster with an authenticated mountpoint.
- `ntrip/anyuser` -- Ntrip caster mountpoint with `auth.anyUser`: any valid
  top-level user streams, while unauthenticated and wrong-credential requests
  are rejected.
- `ntrip/metadata` -- Ntrip caster source-table metadata: shared STR-record
  overrides (network, country, generator, lat/lon, bitrate) apply to every
  mountpoint, and per-mountpoint description and bitrate override or fall back
  to them.
- `ntrip/msm7to4` -- Ntrip caster mountpoint with `msm7to4`: the receiver feed's
  RTCM MSM7 observations are delivered to the client as MSM4, with no MSM7
  message passing through.
- `ntrip/rtklib` -- Ntrip caster interop with a real RTKLIB `str2str` client: it
  connects as an Ntrip client and receives a contiguous window of the caster's
  RTCM, relayed byte-for-byte. Skipped when `str2str` is not on PATH.
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
- `stream/pull-ntrip` -- Ntrip pull: the daemon pulls RTCM MSM4 corrections from a
  fake correction source and writes them back to the receiver over the serial
  port; the captured serial writes match the source's RTCM. Captures the
  daemon's serial writes (`CAPTURE_WRITES`) and uses the pty as a write path
  rather than to model a disconnect, so it stops via `SIGINT`.
- `stream/pull-tcp` -- plain TCP pull: the same write path as `stream/pull-ntrip`,
  but the `[stream.pull.tcp]` client connects to a raw TCP source that streams
  RTCM with no Ntrip handshake, covering the non-Ntrip pull transport.
- `stream/pull-rtklib` -- the same write path as `stream/pull-ntrip`, but the
  correction source is a real RTKLIB `str2str` Ntrip caster instead of
  `fakesource.py`. The daemon writes back the contiguous window of source RTCM
  it pulls. Skipped when `str2str` is not on PATH.
- `stream/nmea-send` -- Ntrip NMEA send pull: the fake correction source waits for a
  post-handshake GGA before streaming RTCM corrections, the daemon re-sends GGA on
  the `nmeaSendInterval` (the source records more than one), and the daemon's serial
  writes still match the source RTCM.
- `stream/wb-corrections` (satpulsewb) -- the workbench forwards a correction
  source's RTCM to the receiver, beside the daemon's `stream/pull-*`. It connects
  over a pty, starts corrections from the API against the fake source (a
  purpose-recorded F9P base capture, `base-arp.jsonl`: MSM7 plus RTCM 1005),
  reports the state reaching connected, delivers `gps:corrpacket`, decodes the
  base station ARP as `gps:basearp`, and writes the source RTCM back over the
  serial port (the reused `stream.check_pulled_rtcm`); a VRS (`nmeaSend`) restart
  is then accepted once a fix is held.
- `shutdown/serial-loss` -- the serial input disappears (a pty whose master is
  closed mid-run) and the daemon must shut down on its own and exit with a
  restartable code, with an HTTP endpoint configured. Guards the scan-worker-
  exit -> daemon-shutdown path (issue #172); uses the pty transport and
  `SELF_SHUTDOWN`.
- `shutdown/wb-serial-loss` (satpulsewb) -- the inverse: when the serial input
  disappears, the workbench must *keep running* and report the receiver
  disconnected over its state endpoint, rather than exit. Uses the pty via
  `DISCONNECTABLE` and stops via the normal `SIGINT` path.

Most Ntrip caster scenarios use `satpulsetool ntrip` as the client. The
`stream/push-ntrip` scenario uses the built-in Ntrip fake caster
(`scenarios/ntrip/fakecaster.py`) as the remote peer, so it needs no external
dependency: it accepts the daemon's Ntrip v1 SOURCE feed and captures the
payload, which the check scans back into RTCM. The `stream/push-udp` scenario
uses the built-in UDP receiver (`scenarios/stream/fakeudp.py`) as its remote
peer. The `stream/pull-{ntrip,tcp,nmea-send}` scenarios use the matching fake
correction source (`scenarios/stream/fakesource.py`): for `pull-ntrip` it
answers the daemon's Ntrip v1 GET and streams an RTCM log; for `pull-tcp`
(`--tcp`) it skips the handshake and streams as soon as the daemon connects. The
daemon writes the corrections back to the receiver over the pty write path that
a read-only FIFO cannot provide.

The `ntrip/rtklib` and `stream/pull-rtklib` scenarios instead use a real RTKLIB
`str2str` as the peer (client and Ntrip caster respectively), for interop
coverage. They declare `REQUIRES = ("str2str",)` and are skipped when `str2str`
is not on PATH, so they add no hard dependency. Because a real Ntrip peer joins
a live stream mid-flight, it relays a contiguous window of the RTCM rather than
the whole log, so these checks match a non-empty contiguous run of the source
rather than the exact stream the fake peers guarantee.

## Installed systemd environment

The plan also describes an installed/systemd environment that runs the
same scenarios as root under `satpulse@gps.service`, driven from
`systest`. That environment is not implemented yet; the direct
environment here is the first version.
