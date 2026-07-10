# CLAUDE.md -- smoketest

Guidance for working in `smoketest/`. For the user-facing overview, scenario
list, and design rationale, read `README.md` first; the design doc is
`plan/smoke-test.md` (repo root). This file covers what an agent needs to make
changes safely.

## What this is

Black-box smoke tests that run the real `satpulsed` and `satpulsewb` binaries,
fed by realtime packet-log replay through a FIFO or pty, with no root and no GPS
hardware. They check program-level behaviour -- config wiring, startup,
observability endpoints, logging, Ntrip, NTP, corrections, shutdown -- NOT packet
decoding. Decode correctness is covered by Go package tests and
`plan/packet-testing.md`; do not add packet/protocol assertions here.

The program under test is a scenario dimension (`PROGRAM`, default `satpulsed`;
`satpulsewb` is the SatPulse Workbench GUI server over `gps/app/session`), chosen
behind a per-program seam the same way the OS is chosen behind `platform_*`. See
Program under test below.

Checks are deliberately shallow: catch gross breakage (panics, failed wiring,
missing listeners, broken shutdown, missing/empty logs, unusable endpoints),
not fine-grained protocol behaviour.

This is Python (stdlib only at runtime), testing a Go daemon. It is not part of
`make test`.

## Running

The suite runs the binaries from `out/<arch>/` on Linux and
`out/<goos>_<arch>/` on macOS/FreeBSD, so build first. `make` does NOT rebuild
them as part of `make smoketest` -- the target only depends on the binaries
existing, so run the appropriate build yourself first (e.g. after any Go
change).

```sh
make            # from repo root: build satpulsed + satpulsetool + satpulsewb
make smoketest  # run all scenarios in parallel
```

On macOS and FreeBSD:

```sh
./unix-build.sh
python3 smoketest/run.py
```

The runner honours `GOOS` and `GOARCH` when set. Linux binaries live under
`out/<arch>/`; non-Linux Unix binaries live under `out/<goos>_<arch>/`, matching
`unix-build.sh`.

For selecting scenarios, listing, or serial debugging, call `run.py` directly
(the make target takes no args):

```sh
python3 smoketest/run.py            # all scenarios, in parallel
python3 smoketest/run.py http/full  # named scenarios (space-separated)
python3 smoketest/run.py --list     # list scenario IDs
python3 smoketest/run.py -j 1       # serial (use when debugging)
python3 smoketest/run.py --sudo     # use sudo -n for root-required scenarios
```

`PASS`/`FAIL` per scenario. On `FAIL` the traceback prints and the run
directory is **kept** (path printed to stderr) -- inspect the program log
(`satpulsed.log` or `satpulsewb.log`), `replay.err`, `caster.log`/`source.log`,
`ntp.jsonl`, the rendered `satpulse.toml`, and the `log/` dir there. On `PASS`
the run dir is deleted. Root-required scenarios
print `SKIP` when the runner is not root and `--sudo` was not specified.
`--sudo` uses `sudo -n`, so it is intended for passwordless sudo in CI and
fails before starting scenarios if sudo is unavailable.

No third-party Python packages are needed to run. Dev tooling (mypy) is scoped
to this dir and managed with `uv`:

```sh
cd smoketest
make dev-deps     # uv sync --extra dev --frozen
make typecheck    # strict mypy over run.py, ntpsock.py, common.py, scenarios/
make update-deps
```

Run `make typecheck` after any Python change here -- mypy is `strict = true`,
so missing types / loose casts will fail.

## Architecture (what the runner does per scenario)

`run.py` is the execution environment; read `run_scenario()` for the full
lifecycle. Per scenario `family/name`:

1. Allocate a disjoint block of ports (below the ephemeral range) and per-run
   paths under a temp dir -- this is what makes scenarios parallel-safe.
2. Ask the program (`program.prepare`) to render its input, substituting
   `${SATPULSE_TEST_*}` vars (see below). satpulsed renders `name.toml.in` and
   detects which listeners/peers the config configures (`[[http]]`, `[ntrip]`,
   `[ntp]`, `[[stream.push]]`, `[stream.pull]`); satpulsewb renders
   `name.args.in` into its flag list.
3. Start auxiliary peers that must exist *before* the program
   (`program.start_peers`): the chrony SOCK consumer (`ntpsock.py`), fake Ntrip
   casters, fake UDP receivers, and the fake correction source -- config-derived
   for satpulsed, the declared `CORRECTION_SOURCE` for satpulsewb.
4. Start the program (`program.command`, e.g. `satpulsed -v -f <config>` or
   `satpulsewb <flags>`); stdout+stderr -> `<program>.log`.
5. `program.wait_ready`: satpulsed waits for its listeners (`wait_listeners`)
   and outbound push (`wait_push`); satpulsewb parses the printed URL/token and
   waits for its HTTP port. This happens before replay, so observers exist
   before any packet flows.
6. Start **exactly one** `satpulsetool pack --realtime <FACTOR> <log>` replay
   into the FIFO/pty, in the background.
7. Call the scenario's `run(ctx)`.
8. Backstop `ctx.wait_replay()`, then SIGINT shutdown (`stop_daemon`), verify
   clean exit and `program.ports_to_free`, then scan `<program>.log` for
   unexpected errors (base allow-list from `program.allowed_errors`).

### Invariants you must not break

- **One replay per daemon lifetime.** Concatenating replays corrupts the RTCM
  frame boundary at the junction. Live checks (SSE, Ntrip stream, TCP proxy)
  must observe the single background replay *while it flows* -- they cannot
  trigger their own. Order live checks before `ctx.wait_replay()`; order
  after-the-fact checks (logs, captured push payload) after it.
- **Observers before packets.** The runner waits for listeners before replay so
  live checks don't race startup. Don't add a check that assumes data is
  present the instant the daemon starts.
- **Poll, don't sleep-and-hope.** The daemon exposes intermediate state; use
  `common.poll(...)` (or the `wait_*` ctx helpers) until a condition holds.
- **Choose `FACTOR` so the replay outlasts the live checks** but stays fast in
  CI. `ntp/sock` must use `FACTOR = 1` (realtime): its time-consistency
  assertion depends on message UTC and the read clock advancing together.

## Adding or changing a scenario

A scenario ID `family/name` maps to two files and one registry entry:

- `scenarios/family/name.py` -- the module. Must define:
  - `PACKET_LOG` -- path relative to repo root; reuse logs under
    `gps/testdata/packets/` (don't add new captures here).
  - `FACTOR` -- replay speedup (int).
  - `run(ctx)` -- the checks. Type the param as `common.SmokeContext`.
  - optional `PROGRAM` -- `"satpulsed"` (default) or `"satpulsewb"`, the program
    under test (see Program under test). A `"satpulsewb"` scenario pairs with a
    `name.args.in` flag list instead of `name.toml.in`, and uses the workbench
    checks in `common.py` (`check_wb_*`, `wb_get`/`wb_post`, `wb_sse`).
  - optional `ALLOWED_ERRORS` -- tuple of substrings for `level=error/warn`
    lines the scenario legitimately expects (e.g. a push it knows is rejected).
    These are added on top of the program's base allow-list
    (`common.ALLOWED_WARNINGS` for the daemon). Keep them as narrow as possible
    so real regressions still fail.
  - optional `REQUIRES_ROOT = True` -- the scenario is skipped unless the runner
    is already root or `--sudo` was passed; use `ctx.root_cmd(...)` for helper
    subprocesses that also need root.
  - optional `CAPTURE_WRITES = True` -- record the daemon's serial writes to
    `ctx.serial_writes`, so a write-path scenario (stream/pull-*) can scan what
    the daemon wrote back to the receiver. The daemon's non-RTCM detection
    probes are filtered out by tag. Implies a read-write transport (see
    Transports below); independent of `SELF_SHUTDOWN`.
  - optional `PULL_SOURCE_LOG` -- for a `[stream.pull]` scenario, the RTCM log
    the runner's fake correction source streams to the daemon (path relative to
    the repo root, like `PACKET_LOG`).
  - optional `PULL_PEER` -- the `[stream.pull]` correction-source implementation:
    `"fake"` (the default `fakesource.py`, which delivers the whole log
    losslessly) or `"str2str"` (a real RTKLIB Ntrip caster fed by `pack`, which
    serves only from the client's connect point on, so the daemon receives a
    contiguous window). Use `stream.check_pulled_rtcm_window` for the str2str case.
  - optional `REQUIRES` -- a tuple of external binary names that must be on PATH
    (e.g. `("str2str",)`); the scenario is skipped with `SKIP` when any is
    missing, so an optional real-peer interop test adds no hard dependency.
  - optional `SELF_SHUTDOWN = True` -- the daemon is expected to exit on its own
    when the input goes away, so the runner disconnects the transport, asserts a
    self-exit with a restartable non-zero code (not `0/64/77/78`), and reports a
    hang as a failure -- it does **not** send SIGINT. Implies a disconnectable
    transport (see Transports below); orthogonal to `CAPTURE_WRITES`. satpulsewb
    scenarios never set it (the workbench must survive device loss, not exit).
  - optional `DISCONNECTABLE = True` -- request the pty's disconnect capability
    *without* `SELF_SHUTDOWN`'s exit check, for a scenario that disconnects the
    input and then asserts the program keeps running (satpulsewb device loss).
  - optional `CORRECTION_SOURCE` (satpulsewb) -- a dict declaring a fake Ntrip
    correction source the runner starts before the program: `port_key` (the env
    key for its port), `log` (RTCM log to stream, repo-relative), and optional
    `mode`/`mountpoint`/`username`/`password`/`require_gga`. The runner points
    `ctx.pull_source_log` at `log` so `stream.check_pulled_rtcm` reuses cleanly.
  - optional `XFAIL = "<reason>"` -- the scenario is known to fail (e.g. a bug
    not yet fixed). It then reports `XFAIL` instead of `FAIL` and does not fail
    the suite; if it unexpectedly passes it reports `XPASS`, which *does* fail
    the suite, prompting removal of the marker once the fix lands.
- `scenarios/family/name.toml.in` (satpulsed) -- config template, or
  `scenarios/family/name.args.in` (satpulsewb) -- flag list, one token per line;
  both use `${SATPULSE_TEST_*}` substitution and allow `#` comments.
- An entry in the `SCENARIOS` list in `run.py` (IDs are explicit, not globbed).

Then update the scenario list in `README.md`.

### Program under test

`program_api.py` is the program seam (a `Program` Protocol plus `select(name)`),
implemented by `program_satpulsed.py` and `program_satpulsewb.py`. It is the
program counterpart to the `platform_*` seam, but a `Program` is chosen per
scenario (both coexist in one process), so it is shaped like the `Transport`
Protocol, not the one-per-process platform module. The `Program` owns exactly
what differs between the two programs and nothing else:

- `prepare(ctx, scen)` -- render the input (satpulsed: `name.toml.in` + detect
  `ctx.has_http/ntrip/...`; satpulsewb: `name.args.in` -> `ctx.wb_args`);
- `command(ctx)` -- the argv (before root wrapping);
- `start_peers(ctx, scen)` -- start the fake peers (satpulsed: config-derived;
  satpulsewb: the declared `CORRECTION_SOURCE`);
- `wait_ready(ctx)` -- satpulsed waits for its configured listeners; satpulsewb
  parses the URL and token it prints (into `ctx.wb_port`/`ctx.token`) and waits
  for its HTTP port;
- `allowed_errors()` -- the base log allow-list;
- `ports_to_free(ctx)` -- ports that must be released after shutdown.

Everything else in `run.py` is shared. Keep it that way: a difference between the
two programs belongs behind this seam, not as a `PROGRAM == "satpulsewb"` branch
in `run.py` or a scenario. The program log is `ctx.daemon_log` (named
`<program>.log`); for satpulsewb it also carries the printed URL that
`wait_ready` parses.

### Transports

A scenario never names a transport. It declares the *capabilities* it needs --
`CAPTURE_WRITES` wants a read-write transport, `SELF_SHUTDOWN` a disconnectable
one -- and the platform (`plat.make_transport` in `platform_unix.py`) maps those
to a concrete transport, or reports the request unsupported (a SKIP). The two
capabilities are orthogonal, and on Unix the choice is either a FIFO (neither)
or a pty (both); the distinction is load-bearing, not cosmetic:

- **FIFO** (default) -- satpulsed opens it `O_RDWR` (`gps/lib/term/fallback_linux.go`)
  and holds its own write end, so it never reaches EOF: an idle FIFO reads as a
  silent-but-connected receiver, and the daemon and replayer can start/stop in
  any order. By the same token a FIFO can never look *disconnected*, so it
  cannot drive the input-loss / shutdown path. It is also read-only at the gpsio
  layer (`DevFIFO` -> `ReadOnly()`), so it cannot test write-path behaviour.
- **pty** -- a real TTY (term.Term, the path real serial hardware uses) and
  full-duplex. The runner holds the master, feeds replay into it, and a drain
  thread reads the daemon's upstream writes (discarded by default, or captured
  to `ctx.serial_writes` when `CAPTURE_WRITES` is set). Closing the master is a
  real disconnect (slave reads fail, scan worker exits). This is the only
  transport that can model a device going away, and the only one the
  `stream/pull-*` write-path scenarios can use.

`SELF_SHUTDOWN` is a property of the *lifecycle* (does the daemon exit without a
signal?): it needs a disconnectable transport but does not follow from one --
the `stream/pull-*` scenarios take a disconnectable transport (via the pty they
get for read-write) yet stop via SIGINT. Keep the two attributes independent.

`ctx.disconnect()` (disconnectable transports only) drops the input;
`ctx.wait_exit()` waits for the no-signal exit and escalates to SIGQUIT on a
hang. Both live on the runner's `Context` and are mirrored in
`common.SmokeContext` for the scenario type.

### Where checks live

- `common.py` -- checks/helpers genuinely shared across families (HTTP
  endpoints, log files, `poll`, `http_get`, `log_packets`/`scan_packets`, the
  daemon-log error scan). Put something here only if more than one family uses
  it.
- `scenarios/<family>/__init__.py` -- checks owned by one family (e.g.
  `ntrip.check_stream`, `proxy.check_tcp`, `ntp.check_sock`,
  `stream.check_pushed_rtcm`). Import as `from scenarios import <family>`.

Don't widen a family-local check into `common.py` until a second family needs
it.

### Config template vars

Set per run by the runner; reference as `${NAME}`:

- `SATPULSE_TEST_SERIAL` -- the serial device path the daemon opens (every
  scenario sets `[serial] device` to this). The runner points it at the
  transport's path -- the FIFO path for a plain scenario, the pty slave name
  when a capability (`CAPTURE_WRITES`/`SELF_SHUTDOWN`) selects a pty.
- `SATPULSE_TEST_LOG_DIR`, `SATPULSE_TEST_RUN_DIR`, `SATPULSE_TEST_CONFIG`.
- Ports (each scenario gets a private block): `SATPULSE_TEST_HTTP_PORT`,
  `SATPULSE_TEST_HTTP_PORT2` (second `[[http]]` endpoint),
  `SATPULSE_TEST_NTRIP_PORT`, `SATPULSE_TEST_PROXY_TCP_PORT`,
  `SATPULSE_TEST_PROXY_TCP_RTCM_PORT`, `SATPULSE_TEST_REMOTE_CASTER_PORT`,
  `SATPULSE_TEST_REMOTE_CASTER_PORT2`, `SATPULSE_TEST_TOOL_PORT`,
  `SATPULSE_TEST_REMOTE_UDP_PORT`.
- `SATPULSE_TEST_PROXY_SOCKET`, `SATPULSE_TEST_NTP_SOCK` -- Unix socket paths.
- `SATPULSE_TEST_NTP_SHM_SEGMENT` -- high-numbered test SHM segment for
  root-required NTP SHM scenarios.

Adding a new resource means adding it to `PORT_OFFSETS` / `allocate_env()` in
`run.py`, not hardcoding a port. Listener detection in `render_config()` keys
off non-comment table headers, so a `[ntrip]` header in a *comment* won't be
mistaken for a real one.

Use dotted TOML key notation (`sock.path = "..."`), matching the rest of the
repo, not bracketed sub-tables where a dotted key reads cleanly.

## Standalone helper tools

These are also usable by hand for debugging, independent of the suite:

- `ntpsock.py` -- bind a chrony SOCK refclock path and decode/print the
  `sock_sample` datagrams a sender (satpulsed, gpsd) writes. Text or JSON.
- `ntpshm.py` -- attach to an ntpd/NTPsec SHM refclock segment, read one
  mode-1 sample using libatomic-backed loads/fences, or remove the segment.
- `scenarios/ntrip/fakecaster.py` -- minimal Ntrip v1 caster: accepts a SOURCE
  handshake, replies `ICY 200 OK`, and appends the pushed payload to a file
  (feed it to `satpulsetool scan`). Optional `--mountpoint`/`--password` checks.
- `scenarios/stream/fakesource.py` -- minimal Ntrip v1 correction source (the
  pull counterpart): answers a GET handshake, replies `ICY 200 OK`, and streams
  an RTCM packet log to the client via `satpulsetool pack`, then holds the
  connection open so the client does not reconnect and re-send. Optional
  `--mountpoint`/`--username`/`--password` checks.

## Code style (Python here)

- Match the existing style: stdlib only at runtime, `from __future__ import
  annotations`, full type hints (mypy strict), module/function docstrings that
  explain *why* a step exists (the timing/ordering rationale matters more than
  the mechanics). ASCII only.
- Keep the repo-wide conventions from the root `CLAUDE.md` in mind (sentence-case
  headings in docs; reference issue numbers in commit/PR text; `git add -u` plus
  explicit new files, never `git add -A`).
