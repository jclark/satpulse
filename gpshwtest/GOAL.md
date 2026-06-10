# Goal: a program to test GPS high-level configuration against real hardware

Your goal is to produce a program that can usefully test the implementation of GPS high-level configuration against real hardware, and that fits into the workflow described in this file. The program is the deliverable; iterate on it against the real receivers until it serves that workflow well.

A pass/fail approach cannot work: real receivers are diverse, and high-level configuration has best-effort semantics, so "requested X, got Y" is not a verdict. Instead the program outputs a characterization of how device-independent configuration is realized on the receiver. Testing then works in two parts: the tool's device-independent guarantees are verified on every run (genuine failures), and the characterization is compared against a vetted, checked-in version (regressions in satpulsetool, in the program, or from a firmware change show up as differences).

This file defines the goal and how to measure progress toward it. It deliberately does not specify the internal design: implementation experience against the real receivers drives that, and where the original plan (`plan/gps-config-hwtest.md`) conflicts with this file, this file wins.

## Shape constraints

- A Python program in this directory, stdlib-only at runtime, runnable in-repo as `python3 gpshwtest`, mypy strict via uv (like `smoketest/`).
- All receiver I/O goes through `satpulsetool gps` invocations using `--json` and `--packet-log`. Captured packet logs are interpreted offline with `satpulsetool replay` (typed gpsprot events); physical time pulses are observed with `satpulsetool sdp`. The program contains no serial or GPS-protocol code; it tests exactly the behavior a satpulsetool user gets, at the level a satpulsetool user sees.
- Receiver-specific knowledge in the program is minimized. What a receiver does is discovered by probing it, not encoded in tables of expected per-model behavior.

## Coverage: all of high-level configuration

The program must cover the entire documented high-level configuration vocabulary of `satpulsetool gps` (see `docs/man/satpulsetool-gps.1.md`). Characterization that covers only some properties is not done. The vocabulary:

- Enabled constellations and bands (`--gnss`, `--band`)
- Timing constellation (`--time-gnss`)
- Time pulse (`--pps`)
- Antenna cable delay (`--ant-cable-delay`)
- Minimum elevation (`--min-elev`)
- Positioning mode: mobile, survey with time/accuracy, fixed ECEF, fixed LLH, position accuracy (`--mobile`, `--survey`, `--survey-time`, `--survey-acc`, `--fixed-pos-ecef`, `--fixed-pos-llh`, `--fixed-pos-acc`)
- RTCM base station ID (`--rtcm-base-id`)
- Receiver serial speed (`--speed`) [unsafe]
- Message output (`--pvt-out`, `--sats-out`, `--nmea-out`, `--rtcm-out`, `--raw-out`, `--nmea`, `--binary`)
- Non-volatile memory operations (`--save`, `--save-all`, `--reload`, `--reset`, `--factory-reset`) [unsafe]
- The query side: receiver identity and `supports` flags (`--show-receiver`), configuration readback (`--show-config`), active port and speed (`--show-port`)

Hidden/experimental flags (`--osnma`, `--static`, `--sys-time-trusted`) are out of scope.

## The semantics under test

High-level configuration has device-independent, best-effort semantics: a request expresses intent in a device-independent vocabulary, and satpulsetool realizes it on the receiver as best the receiver's capabilities allow.

Consequences that the tester must treat as normal, not as errors:

- What reads back may differ from what was requested. Signal sets are clipped to what the receiver supports, values are quantized to the receiver's resolution, some constellations are coupled, some properties cannot be set or cannot be read back on a given backend.
- A receiver may refuse a request outright (for example a signal combination outside its supported set). satpulsetool reports this as an error; the refusal must leave the receiver's configuration unchanged.
- Saving is best-effort in granularity: `--save-all` saves the entire running configuration; `--save` aims to persist exactly the changes specified in the invocation, but not all receivers can save that selectively - the realization is "what was specified, plus as little extra as possible". The model for characterizing this is config groups: saving a property persists everything in its group, so a receiver's save granularity is a partition of the properties into groups. Perfect realization is every property in its own group; the coarsest is a single group (`--save` indistinguishable from `--save-all`); in between is a partition like gen 8 u-blox's few config sections. The discovered partition is what the characterization records.
- `ConfigSupportFlags` (`supports` in the JSON output) declares which optional capabilities a backend offers; absent capabilities bound what can be expected of a request.

What the tool guarantees, and therefore what a tester can check without any receiver-specific knowledge:

- An invocation responds: it does not crash, hang, or silently produce nothing.
- The achieved values it reports are the truth: an independent readback (a separate invocation) agrees with them.
- A reported error is the truth: nothing changed when it says configuration failed.
- Persistence works as stated: what `--save` was asked to persist survives reload/reset (it may persist more, per the granularity limitation above); `--save-all` persists the whole running configuration; changes made after the last save do not survive reload.

### Message output

Message enablement (`--pvt-out`, `--sats-out`, `--nmea-out`, `--rtcm-out`, `--raw-out`) is defined at the information level: `pos` means "position information is delivered", not "message X is emitted". It cannot be verified by property readback; it is verified by what the receiver actually emits:

- Capture a packet log after applying the flags, convert it offline with `satpulsetool replay` into the typed gpsprot event stream, and check the information against the request: each enabled flag implies events of the corresponding type, and content selectors are honored where realized (`tai` selects the time scale within time events, `ecef` selects ECEF position/velocity over geodetic, `tp` versus `time` distinguishes pulse time from solution time, `sig` implies per-signal entries in satellites events). With `off`, unrequested PVT information must be absent.
- This simultaneously tests the decode pipeline: information missing from the event stream means either the receiver was not configured to emit it or satpulse failed to decode what it emitted; the characterization and packet-log artifacts distinguish the two.
- Two groups are inherently about wire format and are checked at the packet level from the packet log: NMEA sentence types (from the log's `ascii` field) and RTCM message types (from the RTCM payload header). Raw observation output gets a packet-level presence check.

### Physical time pulse

Where the receiver's PPS output is wired to a PHC pin (declared by the `[phc]` table in a satpulse config file, or given explicitly), the time pulse configuration is verified electrically with `satpulsetool sdp --extts`: pulses present with the configured period when enabled, absent when disabled. Pulse width and polarity are not observable through external timestamps and stay readback-only. A pulse configured `onlyWhenLocked` fires only with a fix, so physical enable checks must first confirm the receiver has a fix (or use a configuration that pulses regardless). `sdp` on an interface requires root; without root or wiring, physical checks are skipped, never failed.

## Failures versus limitations

The single most important property of this program: **a receiver limitation is never reported as a test failure.** A tool that goes red because a receiver cannot do something is useless as a regression signal.

- **Failures** (errors, nonzero exit) are violations of the tool guarantees above: no response, timeout, crash; achieved values contradicted by readback; state changed by a reported failure; persistence guarantees broken.
- **Limitations** (data, the program's main output) are everything the receiver cannot do or does imprecisely: refused combinations, quantization, clipping, couplings, ranges, save granularity.

## The characterization

For each receiver+firmware the program produces a machine-readable characterization of how device-independent configuration works on that receiver. Its content is the receiver's limitations relative to perfect realization of the model - what cannot be done, what is coupled, what is quantized, how coarsely saving persists - because the perfectly-realized parts need no description. Requirements:

- Expressed in the device-independent vocabulary (model properties and values), never in terms of satpulsetool command-line syntax or the program's internal test cases.
- High level: patterns and parameters (for example "an enabled constellation must have all its signals enabled, except BDS", "pulse width quantized to 1 us", "fixed position stored to 1 cm"), not a list of probe outcomes.
- Stable: its content describes the receiver and the configuration semantics, so it must be substantially unchanged when the program's exact probe values or test set change, and when satpulsetool's implementation of configuration changes without changing behavior. Only genuine behavior differences may move it; that stability is what makes the checked-in characterization usable as a regression baseline.
- Honest: observations the program cannot express in its current vocabulary are carried verbatim rather than forced into a pattern or dropped.

## Workflow

1. Run the program against a receiver; it emits the characterization plus any failures.
2. First time for a receiver: the AI checks the plausibility of the characterization together with the human, against the vendor's documentation (for u-blox, the integration manual; see `../gps-protocol-docs/`).
3. The vetted characterization is checked in (per receiver model + firmware).
4. Subsequent runs regenerate it and compare automatically against the checked-in version; differences are regressions to investigate (in satpulsetool, in the program, or a firmware change). Identical output across repeated runs on the same receiver is required for this to work.

## How we measure success

- The characterization covers every property and operation in the coverage list above.
- Running against a supported receiver completes unattended with exit 0 and no spurious failures; a genuine tool bug or broken receiver session is reported as a failure.
- The characterization for a receiver we know well (ZED-F9P) states the things we know to be true of it, in a form a human can check against the integration manual in minutes.
- Pointing the program at a receiver family it has never seen requires no new code for the common path, and produces a characterization plus, at worst, verbatim unexplained observations.
- Two consecutive runs on the same receiver produce the same characterization.
- The receiver is left as it was found (running configuration restored; NVM untouched except by explicitly requested unsafe probes, which must restore NVM to a sane state).
- Per-invocation packet logs are kept as artifacts, usable as candidate replay-test data for `internal/gpscmd`.

## Ground rules for running against hardware

- Verify satpulsed is not running before touching a receiver (`ps ax | grep satpulsed`).
- Unsafe probes (serial speed, save/reload, reset, factory reset) run only when explicitly requested, and must include recovery: rediscovering a receiver whose speed changed, and restoring a sane configuration afterwards.
- Locally attached receivers: ZED-F9P on /dev/ttyACM0 at 38400 (USB; immune to baud wedging), u-blox M8T on /dev/ttyS0 (UART; baud discovered by scanning).
- On this host the F9P's PPS is wired to PHC pin 1 of enp4s0 (the `[phc]` table in /etc/satpulse.toml); physical time pulse checks need root (`sudo -n`, the smoketest convention).

## Working method

- Probe first, then derive vocabulary: do not invent characterization vocabulary for a property before looking at raw observations from real receivers. Keep the raw-observation output available permanently; it is how unfamiliar receivers get examined.
- Build the easiest properties first to establish the probe/characterize/vet/compare pipeline end to end; signal combinations and save granularity are the hardest parts.
- Keep `make typecheck` (mypy strict) passing; commit coherent increments.
- If these goals turn out to be wrong or ambiguous on contact with reality, flag it to the user rather than silently reinterpreting them.
