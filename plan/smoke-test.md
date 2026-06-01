# Smoke tests using realtime packet log replay via FIFO (#303)

Build a daemon smoke test suite that runs real `satpulsed` binaries using
realtime packet log replay via FIFO.

The goal is to smoke test daemon behavior, not packet decoding. Packet logs
are test inputs. The behavior under test is the running `satpulsed` process:
configuration combinations, omitted config sections, logs, HTTP/SSE,
Prometheus metrics, Ntrip behavior, Web UI behavior, process shutdown, and
installed/systemd operation.

This is distinct from [packet-testing.md](./packet-testing.md), which tests
packet scanner and decoder behavior directly.

## Test shape

Keep scenarios simple. Each scenario has:

- one packet log
- one `satpulse.toml` config template
- a list of checks to run, possibly with check-specific arguments

The scenario owns the meaning of the test. Checks should be reusable where
appropriate, but there is no separate conceptual check engine. The execution
environment runs the scenario and passes concrete resources to it.

Checks that observe daemon state should be written as poll-until-condition with
a deadline, not sleep-then-assert. The daemon can legitimately expose an
intermediate state before replay has produced the data needed by a check. For
example, `/position` returns `204 No Content` until a fix exists.

## Execution environments

The execution environment is responsible for running things:

- allocate resource names and ports
- create the FIFO
- render the `satpulse.toml` template
- start `satpulsed`
- run `satpulsetool pack` into the FIFO with the scenario's replay timing
- stop `satpulsed`, normally with `SIGINT`
- provide paths for daemon output, logs, journal queries, and other artifacts
- invoke the scenario checks with resolved paths and URLs

The same scenario should be runnable in multiple environments.

### Direct environment

The direct environment is the default for local development and GitHub Actions.
It runs built binaries directly, without root:

- `out/$ARCH/satpulsed`
- `out/$ARCH/satpulsetool`
- per-run FIFO under a temporary directory
- per-run rendered config
- per-run log and artifact directory

This environment should support parallel execution in CI.

### Installed systemd environment

The installed systemd environment runs installed binaries as root under
systemd. It is intended mostly for owned remote machines driven through
`systest`, and can run serially. It does not need to be optimized for speed.

This mode catches issues that direct execution will miss:

- installed binary paths
- packaged systemd unit behavior
- root execution
- journald logging
- `/etc/satpulse.d` config paths
- `/var/log/satpulse` and other filesystem permissions
- service shutdown behavior

The intended FIFO instance is `/dev/gps`, with config at
`/etc/satpulse.d/gps.toml` and the packaged service as
`satpulse@gps.service`. systemd does not require a corresponding `dev-gps`
device unit to exist for this use.

Systemd mode can take much longer than CI mode. That is acceptable.

## Relation to `systest`

This suite should be a peer of `systest`, not a subdirectory of it.

`systest/` remains the Ansible-based system testing area for installed
machines, including real hardware tests. The smoke test suite provides
hardware-free daemon scenarios that `systest` can invoke remotely when useful.

The top-level directory should be:

```text
smoketest/
```

## Resource allocation

Scenarios must be parallel-safe in the direct environment. The execution
environment should allocate a complete resource block for every scenario
execution, regardless of whether the scenario uses every resource.

The resource block should include at least:

```text
SATPULSE_TEST_RUN_DIR
SATPULSE_TEST_CONFIG
SATPULSE_TEST_PACKET_LOG
SATPULSE_TEST_FIFO
SATPULSE_TEST_LOG_DIR
SATPULSE_TEST_HTTP_PORT
SATPULSE_TEST_NTRIP_PORT
SATPULSE_TEST_PROXY_TCP_PORT
SATPULSE_TEST_PROXY_TCP_RTCM_PORT
SATPULSE_TEST_REMOTE_CASTER_PORT
SATPULSE_TEST_TOOL_PORT
```

The runner should choose distinct ports for each scenario execution. A simple
block allocation scheme is enough for the first version: each parallel worker
or execution index gets a fixed-size port range, and named resources map to
offsets within that range.

Systemd mode can use the same environment variable names even if it runs only
one scenario at a time.

## Replay rate

Revise the `satpulsetool pack` realtime option so it carries a speedup factor
instead of being only a boolean. There should be one option for realtime replay
timing: factor `1.0` means current realtime inter-packet spacing, and larger
values compress the inter-packet delays.

Smoke scenarios whose checks are payload-derived can use faster-than-realtime
replay. This includes checks for position, event types, metrics, Web UI state,
and Ntrip data flow.

Scenarios that exercise serial-timing-mode time-message handling must use
realtime spacing. The time-message buffer and leap detection depend on
inter-packet read-time spacing.

NTP, SHM, and clock offset values are not meaningful under packet-log replay at
any speed, because capture time and replay time are different. Smoke tests
should not assert on those values.

## Config templates

Scenario configs should be templates. The execution environment renders the
template by substituting the resource variables before starting the daemon.

For example:

```toml
[serial]
device = "${SATPULSE_TEST_FIFO}"

[gps]
config = false
vendor = "u-blox"

[log]
dir = "${SATPULSE_TEST_LOG_DIR}"
event = true
track = true

[[http]]
listen = "127.0.0.1:${SATPULSE_TEST_HTTP_PORT}"

[ntrip]
listen = "127.0.0.1:${SATPULSE_TEST_NTRIP_PORT}"

[[ntrip.mountpoint]]
name = "RTCM"
```

The rendered config is an artifact of the test run.

The daemon writes logs to the paths specified by the rendered config. The
execution environment should make those paths scenario-specific and pass the
resolved locations to checks.

FIFO-driven daemon scenarios must set `gps.config = false`. FIFO serial
connections are read-only, so receiver configuration writes cannot work in this
test mode. Testing `gps.config = true` requires a different path, such as
`gpscmd` configuration replay, and is out of scope for this daemon FIFO replay
suite.

## Implementation language

Start with a low-tech Python implementation.

The first version should optimize for getting useful daemon coverage quickly,
not for building a general framework. Use Python scripts for scenarios and
checks, with shared helper functions only where they remove obvious
duplication.

A simple initial shape is:

```text
smoketest/
  run.py
  checks.py
  scenarios/
    minimal/
      packet.jsonl
      satpulse.toml.in
      check.py
```

`run.py` handles the common execution work: allocate resources, create the
FIFO, render config, start the daemon, run `satpulsetool pack` with the
scenario's replay timing, stop the daemon, and call the scenario's checks.

Avoid a declarative check engine initially. A scenario's `check.py` can import
shared helpers from `checks.py` and perform whatever checks that scenario
needs.

For parallel direct-mode execution, use simple Python concurrency first. The
work is mostly subprocess and file IO, so `concurrent.futures` is likely enough
for the first version. `asyncio` can wait until there is a concrete reason to
need it.

The Web UI check remains a TypeScript Playwright script under `web/`, invoked
as a subprocess when a scenario includes that check.

A small Go runner or deployable binary can be reconsidered later if Python
becomes a real limitation for CI or `systest` deployment.

## Checks

Checks are scenario-defined and reusable where appropriate.

The checks should be broad black-box checks of the real daemon process, not
detailed re-tests of lower-level package behavior. They should catch gross
breakage such as nil pointer panics, failed config wiring, missing listeners,
bad paths, process crashes, broken shutdown, missing logs, and unusable
externally visible endpoints. More detailed packet, protocol, formatting, and
handler behavior should stay in focused package tests where possible.

Keeping daemon checks shallow is intentional. It should keep the smoke test
implementation small while still giving broad coverage of real-process config,
startup, wiring, observability, and shutdown behavior. The aim is to add useful
coverage quickly, with minimal framework code.

Examples:

- event log contains expected event types
- track log contains at least one position
- packet log exists when enabled
- daemon output or journal has no unexpected warnings or errors
- `/position` returns position JSON when configured
- `/metrics` contains expected Prometheus metrics
- SSE receives expected event names
- Web UI shows expected cards
- Ntrip source table contains expected mountpoints
- Ntrip stream yields expected RTCM packets

Observable-state checks should poll until the expected condition is true or a
deadline expires. This applies to HTTP endpoints, log files, metrics, SSE, Web
UI state, Ntrip clients, and process shutdown checks.

## Ntrip scenarios

For SatPulse-as-caster scenarios, use `satpulsetool ntrip` as the client by
default. This checks the daemon caster and also exercises `satpulsetool ntrip`.
The check should verify that the real daemon process exposes the configured
caster and mountpoint and that an external client can use it. It does not need
to duplicate detailed source-table formatting checks that are already covered
by package tests.

For scenarios where SatPulse needs another Ntrip endpoint, such as
`stream.push` or `stream.pull`, use `str2str` where available as the external
peer. Those checks can be optional in CI and required on owned `systest`
machines if the dependency is installed there.

## Initial scenario ideas

Start with a small set of scenarios that target daemon integration bugs:

- minimal config with no optional sections
- event and track logging enabled
- HTTP default endpoint with position, metrics, and GUI enabled
- HTTP endpoints with some features disabled
- Ntrip caster serving RTCM from a packet log
- Ntrip caster authentication
- config combinations with omitted `[log]`, `[[http]]`, `[ntrip]`,
  `[stream]`, and `[proxy]` sections

Additional fixtures can be generated as needed. The fixture set should be
chosen for daemon behavior coverage, not scanner or decoder coverage.

## Verify

- Direct mode runs locally without root and without hardware.
- Direct mode runs in GitHub Actions and supports parallel scenario execution.
- Installed systemd mode runs at least one scenario through `satpulse@gps.service`.
- Scenarios use rendered config templates and runner-allocated resources.
- Reusable checks cover logs, HTTP, metrics, Ntrip, and Web UI behavior.
- Existing hardware `systest` workflows remain separate and continue to work.
