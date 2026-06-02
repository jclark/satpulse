# CLAUDE.md -- smoketest

Guidance for working in `smoketest/`. For the user-facing overview, scenario
list, and design rationale, read `README.md` first; the design doc is
`plan/smoke-test.md` (repo root). This file covers what an agent needs to make
changes safely.

## What this is

Black-box smoke tests that run the real `satpulsed` binary, fed by realtime
packet-log replay through a FIFO, with no root and no GPS hardware. They check
daemon-level behaviour -- config wiring, startup, observability endpoints,
logging, Ntrip, NTP, shutdown -- NOT packet decoding. Decode correctness is
covered by Go package tests and `plan/packet-testing.md`; do not add
packet/protocol assertions here.

Checks are deliberately shallow: catch gross breakage (panics, failed wiring,
missing listeners, broken shutdown, missing/empty logs, unusable endpoints),
not fine-grained protocol behaviour.

This is Python (stdlib only at runtime), testing a Go daemon. It is not part of
`make test`.

## Running

The suite runs the binaries from `out/<arch>/`, so build first. `make` does NOT
rebuild them as part of `make smoketest` -- the target only depends on the
binaries existing, so run `make` yourself first (e.g. after any Go change).

```sh
make            # from repo root: build satpulsed + satpulsetool
make smoketest  # run all scenarios in parallel
```

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
directory is **kept** (path printed to stderr) -- inspect `satpulsed.log`,
`replay.err`, `caster.log`, `ntp.jsonl`, the rendered `satpulse.toml`, and the
`log/` dir there. On `PASS` the run dir is deleted. Root-required scenarios
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
2. Render `scenarios/family/name.toml.in`, substituting `${SATPULSE_TEST_*}`
   vars (see below). From the rendered config the runner detects which
   listeners are configured (`[[http]]`, `[ntrip]`, `[ntp]`, `[[stream.push]]`)
   and sets up only the helpers each needs.
3. Start auxiliary consumers/peers that must exist *before* the daemon: the
   chrony SOCK consumer (`ntpsock.py`) for `[ntp]`, and a fake Ntrip caster
   (`scenarios/ntrip/fakecaster.py`) per `[[stream.push]]` entry.
4. Start `satpulsed -v -f <config>` (stdout+stderr -> `satpulsed.log`).
5. Wait for the daemon's listeners (`wait_listeners`) / outbound push
   (`wait_push`) before replay, so observers exist before any packet flows.
6. Start **exactly one** `satpulsetool pack --realtime <FACTOR> <log>` replay
   into the FIFO, in the background.
7. Call the scenario's `run(ctx)`.
8. Backstop `ctx.wait_replay()`, then SIGINT shutdown (`stop_daemon`), verify
   clean exit and released ports, then scan `satpulsed.log` for unexpected
   errors.

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
  - optional `ALLOWED_ERRORS` -- tuple of substrings for `level=error/warn`
    lines the scenario legitimately expects (e.g. a push it knows is rejected).
    These are added on top of `common.ALLOWED_WARNINGS`. Keep them as narrow as
    possible so real regressions still fail.
  - optional `REQUIRES_ROOT = True` -- the scenario is skipped unless the runner
    is already root or `--sudo` was passed; use `ctx.root_cmd(...)` for helper
    subprocesses that also need root.
- `scenarios/family/name.toml.in` -- config template using `${SATPULSE_TEST_*}`.
- An entry in the `SCENARIOS` list in `run.py` (IDs are explicit, not globbed).

Then update the scenario list in `README.md`.

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

- `SATPULSE_TEST_FIFO` -- serial device FIFO (every scenario sets
  `[serial] device` to this).
- `SATPULSE_TEST_LOG_DIR`, `SATPULSE_TEST_RUN_DIR`, `SATPULSE_TEST_CONFIG`.
- Ports (each scenario gets a private block): `SATPULSE_TEST_HTTP_PORT`,
  `SATPULSE_TEST_NTRIP_PORT`, `SATPULSE_TEST_PROXY_TCP_PORT`,
  `SATPULSE_TEST_PROXY_TCP_RTCM_PORT`, `SATPULSE_TEST_REMOTE_CASTER_PORT`,
  `SATPULSE_TEST_REMOTE_CASTER_PORT2`, `SATPULSE_TEST_TOOL_PORT`.
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

## Code style (Python here)

- Match the existing style: stdlib only at runtime, `from __future__ import
  annotations`, full type hints (mypy strict), module/function docstrings that
  explain *why* a step exists (the timing/ordering rationale matters more than
  the mechanics). ASCII only.
- Keep the repo-wide conventions from the root `CLAUDE.md` in mind (sentence-case
  headings in docs; reference issue numbers in commit/PR text; `git add -u` plus
  explicit new files, never `git add -A`).
