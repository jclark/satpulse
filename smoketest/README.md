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
python3 smoketest/run.py http-full  # run named scenarios
python3 smoketest/run.py --list     # list scenarios
python3 smoketest/run.py -j 1        # run serially
```

No third-party Python packages are required (standard library only).

Output is one line per scenario:

- `PASS` -- all checks passed and the daemon shut down cleanly.
- `FAIL` -- a check failed; the traceback is printed and the run
  directory is kept for inspection.

## How it works

For each scenario the runner (`run.py`):

1. allocates a resource block (a port range and per-run paths) so
   scenarios are parallel-safe;
2. renders the scenario's `satpulse.toml.in` template, substituting the
   `SATPULSE_TEST_*` resource variables;
3. creates the FIFO and starts `satpulsed`;
4. starts a single `satpulsetool pack --realtime <factor>` replay of the
   scenario's packet log into the FIFO, in the background;
5. calls the scenario's `run(ctx)`, which performs its checks while the
   replay flows (live checks such as SSE and Ntrip) and after it
   finishes (`ctx.wait_replay()`, then log and error checks);
6. stops `satpulsed` with `SIGINT` and verifies it exits and releases its
   ports.

Only **one** replay runs per daemon lifetime: concatenating replays
corrupts the RTCM frame boundary at the junction, so live checks observe
the single replay rather than triggering their own.

`--realtime` takes a speedup factor: `1` reproduces the captured
inter-packet timing, larger values compress it. Scenarios use a factor
that keeps the replay flowing long enough for the live checks while
staying fast in CI.

## Layout

```
smoketest/
  run.py            execution environment: resources, replay, shutdown
  checks.py         reusable checks (HTTP, metrics, SSE, logs, Ntrip)
  scenarios/<name>/
    satpulse.toml.in  config template
    scenario.py       PACKET_LOG, FACTOR, and run(ctx)
```

A scenario owns the meaning of its test. `scenario.py` declares:

- `PACKET_LOG` -- packet log to replay (path relative to the repo root;
  the scenarios reuse logs under `gps/testdata/packets/`);
- `FACTOR` -- replay speedup factor;
- `run(ctx)` -- the checks to perform, using helpers from `checks.py`.

## Scenarios

- `minimal` -- no optional sections; just start, replay, shut down.
- `logging` -- event, track, and packet logs written with expected content.
- `http-full` -- default HTTP endpoint: `/position`, `/metrics`, GUI HTML, SSE.
- `http-disabled` -- HTTP endpoint with GUI and metrics off, position
  only; also guards clean shutdown for GUI-disabled endpoints.
- `ntrip` -- Ntrip caster source table and RTCM streaming.
- `ntrip-auth` -- Ntrip caster with an authenticated mountpoint.
- `ntp` -- chrony SOCK refclock: a pure 1 Hz RMC stream drives serial timing
  mode, and the samples are well-formed, consistently timestamped, and carry
  the correct GPS time.

The Ntrip scenarios use `satpulsetool ntrip` as the client. Scenarios
that need an external Ntrip peer (e.g. `stream.push`/`stream.pull`) would
use `str2str` from RTKLIB; none are included yet.

## Installed systemd environment

The plan also describes an installed/systemd environment that runs the
same scenarios as root under `satpulse@gps.service`, driven from
`systest`. That environment is not implemented yet; the direct
environment here is the first version.
