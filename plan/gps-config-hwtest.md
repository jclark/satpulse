# Hardware tests for GPS high-level configuration (#310)

## Problem

High-level configuration in `satpulsetool gps` has device-independent semantics implemented over two receiver families (u-blox 6 through X20, Unicore Nebulas IV), with many per-receiver subtleties: signal sets clipped to what the receiver supports, coupled constellations (GPS/QZSS on some u-blox platforms), quantized pulse widths, properties that some backends cannot set or cannot read back.

Nothing today verifies against a live receiver that a requested configuration is actually applied, reads back consistently, and survives save/reload. The replay tests in `internal/gpscmd` verify that the tool reproduces previously recorded packet exchanges; they cannot detect that the receiver itself disagrees with what we believe we configured, and they only cover receivers and option combinations someone has captured.

casictool has a working precedent: `casic_hwtest.py` points at a receiver and runs a battery of set/readback tests per property group, message-output tests verified by watching the stream, and persistence tests using a save / alter-without-save / reload round trip. This plan adapts that model to satpulse.

## Overview

A stdlib-only Python harness that shells out to `satpulsetool gps` for all receiver I/O. Per test case it runs one invocation to set properties and a separate invocation to read them back, so the readback is a genuinely independent probe-and-query cycle; the configurator cannot leak its own intentions into the verification. `Configure` and the `Configurator` machinery are reused exactly as they are; all new Go code is additive.

The pieces:

1. A `--json` flag on `satpulsetool gps` for machine-readable output (receiver info, support flags, config properties, error).
2. The harness itself: table-driven test groups, support-aware comparators keyed on `ConfigSupportFlags`, persist tests, message-output tests.
3. Dewedging: when a step fails, automatically discover the receiver's actual baud rate and state, record it, and recover.
4. zipapp packaging for the harness and for the uncommitted `smoketest/system.py` system smoke, so both deploy to systest hosts as single files.
5. A systest playbook that runs the battery across the receiver fleet and fetches artifacts.

A free byproduct: every invocation's `--packet-log` output is already in the replay-test format (`env` / packets / `receiver` / `config` JSONL blocks, written by `internal/gpscmd/testlog.go`), and `replay_test.go`'s reader consumes concatenated blocks. Concatenating a run's per-step logs yields candidate golden testdata for that receiver. Running the battery across the systest fleet harvests replay-test coverage for every receiver family we own.

## Part 1: satpulsetool gps --json

New flag `--json`, modeled on `satpulsetool sdp`. When set, the command writes a single JSON object to stdout instead of the human-readable `print*` output:

```json
{
  "receiver": {"vendor": "u-blox", "hardware": "ZED-F9P", "firmware": "HPG 1.51 PROTVER 27.50", "supportedGNSS": ["GPS", "GAL", "BDS", "GLO", "QZSS", "SBAS"]},
  "supports": ["band", "speed", "survey", "surveyAcc", "fixedPos", "fixedPosAcc", "raw", "rtcmMSM4", "rtcmBaseID"],
  "packetFormats": ["UBX", "NMEA"],
  "config": {"signalsEnabled": {"GPS": ["L1C/A", "L2C"]}, "timePulse": {"width": 0.1, "period": 1, "alignToGNSS": true, "onlyWhenLocked": true, "polarityRising": true}, "mode": {"static": true, "fixedPosECEF": [-1144698.0, 6090335.4, 1504171.4], "fixedPosAcc": 10}, "...": "..."},
  "error": "..."
}
```

Notes:

- `ConfigProps`, `ConfigSupportFlags`, and `ReceiverInfo` already have JSON marshaling, so the `config`, `supports`, and `receiver` fields are nearly free. `ConfigProps.MarshalJSON` covers properties the packet-log `config` entry omits today (`minElevation`, `fixedPosLLH`), so the harness verifies through `--json` and the packet-log tail entry stays as-is.
- `config` carries whatever the result props contain: after a set run, the achieved values; with `--show-config`, the queried values; with `--show-port`, port and baud rate.
- `supports` is the piece that is currently human-only (the `Supports:` line printed by `--show-receiver`). The harness needs it machine-readable to drive skip/degrade decisions.
- On configuration error, emit the JSON object with the `error` field set and whatever partial results exist; the exit code behaves as today.
- `--json` applies to high-level configuration and the show options. Combining it with `--msg-file` is an error.
- Man page update (`docs/man/satpulsetool-gps.1.md`, per `internal/gpscmd/CLAUDE.md`) and a NEWS entry (user-facing feature).

Out of scope but related: the TODO at `internal/gpscmd/gpsflags.go` about adding `PropIDBaudRate` to `showProps` so `--show-config` reports the configured speed. Not required for the harness (`--show-port` retrieves baud rate when it matters), and doing it forces regeneration of the `*-noop` replay traces, so it stays separate.

## Part 2: the harness

New top-level directory `gpstest/` containing a Python package, runnable in-repo as `python3 gpstest` and packageable with zipapp (part 4). Stdlib only at runtime; dev tooling (mypy strict via uv) mirrors `smoketest/`.

```
gpstest/
  __main__.py     # argparse, group selection, summary, exit code
  runner.py       # step execution: satpulsetool invocation, timeout, JSON + packet-log collection
  compare.py      # support-aware property comparators
  cases.py        # test tables per group
  dewedge.py      # baud discovery and recovery (part 3)
  events.py       # event-level message-output verification (drives satpulsetool replay)
  Makefile        # zipapp build, typecheck
```

### CLI

```
python3 gpstest -f /etc/satpulse.toml --all
python3 gpstest -d /dev/ttyACM0 -s 38400 --signals --tp
```

- Connection options `-d`/`-s`/`-f` pass through to `satpulsetool gps` unchanged. `-f /etc/satpulse.toml` is the systest path: each host's config supplies device and speed, so the playbook needs no per-host receiver knowledge.
- Group flags as in casic_hwtest: `--signals`, `--time-gnss`, `--tp`, `--mode`, `--ant-cable-delay`, `--min-elev`, `--rtcm-base-id`, `--msg-out`, `--persist`, `--all`. `--all` excludes the dangerous groups; `--speed-group` and `--reset-group` are explicit opt-ins (see "dangerous groups" below).
- `--satpulsetool PATH` (default: `satpulsetool` from PATH, falling back to install paths, like `system.py`); `--artifact-dir`; `--keep-artifacts`; `--vendor` passthrough to restrict probing once known.
- `--phc IFACE:PIN` and `--sudo` for physical PPS verification (see that section).

### Step execution

Each test case is a sequence of steps; each step is one `satpulsetool gps` invocation with `--json` and `--packet-log <artifact-dir>/step-NNN.jsonl`, run under a subprocess timeout (a stuck invocation is killed and treated as a wedge trigger). The runner records argv, exit code, parsed JSON, and the packet-log path for every step in `results.jsonl`.

The standard case shape:

1. Set: `satpulsetool gps <conn> <set flags> --json --packet-log ...`
2. Readback: `satpulsetool gps <conn> --show-config --json --packet-log ...`
3. Compare requested properties against readback with the comparators below.

The first action of a session is a plain probe (`--show-receiver --json`) that records the session baseline: receiver identity, support flags, the active port and its baud rate (`--show-port`), and a full config snapshot. The baseline drives skip decisions, the dewedge speed ordering, and end-of-run restoration.

### Verification model

`ConfigSupportFlags` drives three kinds of decision:

- Skip. Each case carries a required-support mask. A case requiring `survey`, `fixedPos`, `speed`, `rtcmBaseID`, `raw`, or `rtcmMSM4`/`rtcmMSM7` is reported SKIP (not FAIL) on a receiver lacking the flag.
- Degrade. Without `band`, compare signals at constellation level only (the receiver cannot be steered per band). Without `fixedPosAcc`/`surveyAcc`, set the value but do not compare it on readback.
- Bound. The expected signal set is requested intersected with `receiver.supportedGNSS`.

What flags do not cover stays as a short list of explicit tolerance rules in `compare.py`, each tied to observed receiver behavior:

- Coupled constellations: enabling GPS may force QZSS on some u-blox platforms; accept supersets that consist of known-coupled additions.
- Pulse width and period quantization: accept the nearest representable value (tolerance one quantization step; the step is per-family knowledge).
- Fixed position round-trip precision: ECEF/LLH coordinates compare within the receiver's storage resolution (cm-level), not exactly.

Every tolerance rule must be written down with the receiver family it covers; an unexplained mismatch is a FAIL, not a new tolerance. The point of the tool is to surface exactly these disagreements.

### Test groups

Modeled on casic_hwtest's tables, each ordered so the last case leaves a sane state:

- Signals: constellation subsets (e.g. GPS+GAL+BDS, GPS+GAL, GPS-only), band restrictions where supported (`L1`, `L1+L5`), ending on the receiver's sensible default set. Note: changing the GNSS set causes an internal restart on some u-blox platforms; the readback step's fresh probe absorbs the dead time, but the runner allows a settle delay after signal cases.
- Time GNSS: cycle through supported timing constellations, end on GPS.
- Time pulse: disabled (width 0), a few width/period/polarity/onlyWhenLocked combinations, end on the standard PPS config (`SetPPS` equivalent: 100 ms, rising, aligned, locked-only). Where the host has the receiver's PPS wired to a PHC pin, the group adds physical verification (see below).
- Antenna cable delay: a couple of values, end on 0.
- Min elevation: 5, 15, end on receiver default.
- Mode: survey with explicit duration/accuracy (verify static=true on readback; do not wait for survey completion), fixed ECEF, fixed LLH where supported, end on mobile.
- RTCM base ID: where supported, set and read back, end on 0.
- Persist (`--persist`): the casictool recipe per group: set props + `--save`, set alternative props without saving, `--reload`, read back, expect the saved props. Uses the next case in the group's table as the alternative, wrapping around.
- Message output (`--msg-out`): see below.

### Message-output verification

Setting `--pvt-out` / `--sats-out` / `--nmea-out` / `--rtcm-out` / `--raw-out` cannot be verified by property readback. The message enablement concept is defined in terms of information at the event level - `PVTMsgPos` means "position information is delivered", not "message X is emitted" - so verification happens at the event level too:

1. Apply the flags.
2. Capture: `satpulsetool gps <conn> --packet-log capture.jsonl --capture N`.
3. Convert offline: `satpulsetool replay --vendor V capture.jsonl` turns the packet log into the typed gpsprot event stream (JSONL of `MsgEvent`: `time`, `posGeo`, `posECEF`, `velGeo`, `velECEF`, `leapSecond`, `survey`, `satellites`, `navEpoch`, ...).
4. Assert on the events: each enabled flag implies events of the corresponding type (and field content where the flag is about content: `tai` selects the time scale within `time` events, `ecef` selects `posECEF`/`velECEF` over the geodetic variants, `tp` vs `time` distinguishes pulse time from solution time within `time` events, `sig` implies per-signal entries within `satellites` events). With `off`, events of unrequested PVT types must be absent from the capture.

This needs no per-vendor knowledge in the harness at all - the packet processors behind `satpulsetool replay` already normalize every receiver's messages to gpsprot events - and it verifies the decode pipeline along with the configuration: a case fails both when the receiver was not configured to emit the information and when satpulse fails to decode what it emitted. The flag-to-event mapping lives once in `events.py`, written in gpsprot terms.

Two groups are inherently about the wire format rather than information content, and stay at the packet level (still device-independent):

- `--nmea-out`: the flags name sentences; sentence types come straight from the `ascii` field of the captured packet log.
- `--rtcm-out`: MSM4/MSM7/ARP message types come from the first 12 payload bits of RTCM packets.

`--raw-out` is partly covered by events (`navEpoch`) but raw observations do not surface as gpsprot events; a coarse packet-level presence check suffices.

### Physical PPS verification

Property readback proves the receiver accepted the time pulse configuration; it does not prove a pulse comes out of the pin. On hosts where the receiver's PPS output is wired to a PHC pin - exactly the satpulsed deployment shape, declared by the `[phc]` table (`interface`, `pin`) in satpulse.toml - the time pulse group adds electrical verification with `satpulsetool sdp --extts --pin P -t N --jsonl IFACE`:

- PPS enabled: timestamp events arrive, and their spacing matches the configured period.
- PPS disabled (width 0): no timestamp events over the observation window.

What is observable is presence/absence and period. Pulse width and polarity are not visible through single-edge external timestamping and remain readback-only.

Wiring discovery: with `-f`, the harness reads the `[phc]` table from the same config file via stdlib `tomllib`; a `--phc IFACE:PIN` flag overrides or supplies it for `-d`/`-s` use. No `[phc]` table and no flag means the physical checks are skipped (readback still runs).

Root: `satpulsetool sdp` with an interface argument requires root. The harness gains a `--sudo` flag (`sudo -n`, the smoketest convention) used only for the sdp invocations; without root or `--sudo`, physical checks report SKIP rather than FAIL.

Lock caveat: a pulse configured with `onlyWhenLocked` only fires when the receiver has a fix, so a physically-verified enable case must either use `onlyWhenLocked: false` or first confirm the receiver has a fix (short capture, `satpulsetool replay`, check `time`/`navEpoch` events report a valid solution). The disable case needs no such guard - absence is expected regardless of fix.

### Reporting

Per case: `PASS` / `FAIL` / `SKIP (missing <flag>)` with expected/actual diffs on FAIL, casic_hwtest style. Summary line with counts per group, exit code 0 only if no FAIL. Machine-readable `results.jsonl` in the artifact dir (one entry per case: group, case description, status, steps, mismatches) for the systest playbook to fetch and aggregate.

## Part 3: dewedging

The battery mutates real receiver state, and two failure shapes can strand the receiver somewhere the next step cannot reach:

- Baud wedge (UART-connected receivers only): a `--speed` case half-applied, or a reset/reload that restored a saved or default speed different from the session speed. USB CDC-ACM receivers (e.g. ZED-F9P on `/dev/ttyACM0`) ignore baud entirely and cannot wedge this way; the session baseline's `--show-port` result (port type, baud rate present or not) tells the harness which regime it is in.
- State wedge: receiver responsive but in an unexpected configuration (e.g. a persist test failed between save and reload).

Strategy: every step failure or timeout triggers a diagnosis pass before the run continues or aborts. The harness never retries blind.

### Baud discovery

On a step failure where the probe got no response (distinguishable from a config error in the `--json` output), scan candidate speeds, each attempt being `satpulsetool gps -d DEV -s SPEED --show-receiver --vendor V --json --packet-log dewedge-NNN.jsonl` with a short timeout. Order matters:

1. The session speed (transient glitch check).
2. The last speed the battery commanded, if a `--speed` case was in flight.
3. The receiver's default speed, since resets land there. This is model-dependent, not per-vendor: u-blox 6/7/M8 default to 9600 on UART1, F9/M9/M10 to 38400; Unicore Nebulas IV to 115200. `dewedge.py` keeps a small table keyed off the session baseline's receiver identity (hardware/firmware strings). The table only optimizes scan order - an unknown or wrong entry just means the full scan in step 4 finds the speed instead.
4. The remaining standard set: 9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600.

`--vendor` (known from the session baseline) keeps each probe attempt fast by restricting the protocols tried. First responding speed wins.

### Status capture

At the discovered speed, capture the wedge report before any recovery action: `--show-receiver --show-config --show-port --json` recorded to `wedge-NNN.json` in the artifact dir, plus the speeds tried and the failing step's identity. When things go wrong, the artifact answers "what state was the receiver actually in" without a desk visit and a manual hunt.

### Recovery

- Wrong speed discovered: command the receiver back to the session speed (`--speed <session>` issued at the discovered speed), confirm with a probe at the session speed, then resume the battery. The case that triggered the wedge is recorded as FAIL with the wedge report attached; subsequent cases still run.
- No speed responds: capture what is capturable (the dewedge packet logs show whether anything was received at each speed - garbage bytes vs silence is itself diagnostic), report the receiver unreachable, abort the run with a distinct exit code so the playbook flags the host.
- State wedge (responsive, wrong config): the end-of-run restoration handles it; no special-casing beyond the FAIL report.

### End-of-run restoration

The session baseline includes a full config snapshot. At the end of the battery (and on abort, best-effort), the harness re-applies the snapshot's properties via one set invocation. On systest hosts the playbook then restarts satpulsed, which reconfigures the receiver per its own config anyway, so restoration is defense in depth there - but it makes desk use non-destructive too. Restoration does not write NVM (no `--save`); a battery run without `--persist` leaves non-volatile state untouched. The persist group necessarily writes NVM - that is what it tests - so after a persist run, NVM holds the last props the group saved. Each persist table therefore ends on a sane configuration, and a `--persist-restore` option can re-apply the session snapshot with `--save` afterwards for hosts where NVM must match the baseline.

### Dangerous groups

- `--speed-group`: exercises `--speed` round trips. Opt-in. The dewedge machinery exists precisely so this group is testable; it is still excluded from `--all` because a failure mid-group leaves the receiver at a non-config speed until dewedge runs.
- `--reset-group`: `--reset` and `--factory-reset`. Opt-in. Factory reset is the strongest wedge generator (default speed, default protocol, NVM gone) and also invalidates the session baseline as a restore target for NVM-backed settings; the group re-baselines after each reset. Excluded from `--all` and from the default systest run.

## Part 4: zipapp packaging

Both the new harness and the system smoke deploy to systest hosts as single-file zipapps built with stdlib `zipapp` (no dependencies to bundle):

- `gpstest/Makefile` target building `out/gpstest.pyz` from the `gpstest/` package (`python3 -m zipapp gpstest -o out/gpstest.pyz -p '/usr/bin/env python3'`).
- `smoketest/Makefile` target building `out/system-smoke.pyz` containing `system.py` (as `__main__.py` or via a shim), `common.py`, and `scenarios/`. `system.py`'s current `sys.path.insert(HERE)` is unnecessary inside a zipapp (the archive root is already on `sys.path`) but harmless; the only structural change needed is the `__main__.py` entry point.

Target hosts are Debian 12/13 (python3 >= 3.11), so the 3.10+ syntax already used in `smoketest/` is fine. The `.pyz` files are build artifacts under `out/`, not checked in.

## Part 5: systest integration

New playbook `systest/hwtest.yml` following the existing playbook patterns (`check.yml`, `stop.yml`, `start.yml`):

1. Build `out/gpstest.pyz` locally (or take it prebuilt).
2. Per host: stop the satpulse service(s), copy the `.pyz`, run `python3 gpstest.pyz -f /etc/satpulse.toml --all --artifact-dir /tmp/gpstest-<run>` (group selection overridable via an Ansible variable; dangerous groups never default on). The playbook runs as root, so physical PPS verification is active automatically on every host whose config has a `[phc]` table.
3. Fetch the artifact dir (results.jsonl, per-step packet logs, any wedge reports) back to the controller under a per-host directory.
4. Restart the satpulse service regardless of test outcome.
5. Aggregate: a controller-side summary across hosts from the fetched `results.jsonl` files.

A parallel playbook (or an extension of this one) runs `system-smoke.pyz` on hosts, covering the uncommitted `smoketest/system.py` work; the deployment mechanics (copy .pyz, run, fetch artifacts) are shared.

Golden-data harvest workflow: after a green fleet run, the fetched per-step packet logs for a receiver model can be concatenated and reviewed as candidate testdata for `internal/gpscmd` replay tests. This stays a manual, curated step (golden files are reviewed, not auto-committed), but the format already matches, including the trailing `config` entry the replayer verifies against.

## Phasing

1. `--json` on `satpulsetool gps` (+ man page, NEWS). Independently useful; everything else consumes it.
2. Harness skeleton: runner, session baseline, readback groups (signals, timeGNSS, timePulse, antCableDelay, minElev, mode, rtcmBaseID), comparators, reporting. Developed against the ZED-F9P on the desk.
3. Dewedging (baud discovery, status capture, recovery, restoration). Needs a UART-connected receiver to exercise for real.
4. Persist group.
5. Message-output groups: event-level verification via `satpulsetool replay` for PVT/sats, packet-level checks for NMEA/RTCM.
6. zipapp targets for gpstest and the system smoke.
7. `systest/hwtest.yml` playbook + artifact fetch/aggregation.
8. Opt-in dangerous groups (`--speed-group`, `--reset-group`).

## Open questions

- Directory and tool name: `gpstest/` is proposed; `hwtest/` is the casictool-aligned alternative. The name should not collide conceptually with `smoketest/` (no hardware) or `systest/` (Ansible).
- Whether `--json` should also be honored by the low-level `--msg-file` path eventually (error for now).
- Whether the persist group should also verify `--save-all` (saves running config wholesale), which is harder to make NVM-neutral.
