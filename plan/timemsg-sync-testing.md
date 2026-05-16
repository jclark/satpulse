# Time message and sync testing with real captures (#286)

Test `phcsync` reset mode using real GPS time messages and real PHC pulse traces captured while the PHC is free-running.

## Why free-running captures

`syncsim` tests controller behavior with synthetic data. It does not exercise reset mode against real receiver message timing and real pulse read behavior.

Replaying pulse traces captured while `satpulsed` was disciplining the PHC is flawed: the captured timestamps contain closed-loop controller behavior, so they are not a clean fixture for validating reset mode or for comparing with a future port.

Instead, capture pulse traces while the PHC is free-running, then apply synthetic phase and frequency offsets to generate test scenarios.

## Test data layout

```text
time/testdata/phase/<nic>-<gps>/
  HW.toml
  pulse.jsonl
```

The packet log for time messages lives in `gps/testdata/packets/` (see [packet-testing.md](./packet-testing.md)). Both must be captured on the same host and during the same session so that message timing and pulse-to-message delay relationships are real.

### HW.toml

```toml
[gps]
vendor = "u-blox"
model = "ZED-F9P"
firmware = "1.32"
default-baud = 38400

[phc]
nic = "i225"
interface = "enp4s0"
edges-per-pulse = 2
```

## What reset mode consumes

Reset mode consumes `phcsync.PulseEdge`, which contains:

- pulse PHC timestamp (`Timestamp.T`)
- pulse era (`Timestamp.Era`)
- PHC time sampled near event read (`TRead.PHC.T`)
- system time corresponding to that sampled PHC read (`TRead.Sys`)

It uses these to estimate when the pulse occurred in system time by scaling the PHC-domain delay between the pulse timestamp and the read-time PHC sample. A raw pulse trace must include all three timestamps (pulse, PHC read, system read), not just the pulse timestamp and system read time.

## Change to `satpulsetool sdp -i -j`

Today `sdpcmd/extts.go` emits `timestamp`, `tRead`, `chan`, and `stale`. Add a `tReadPHC` field so the trace captures the PHC time sampled near the read:

```json
{"timestamp": "...", "tRead": "...", "tReadPHC": "...", "chan": 0}
```

This should use the same read-sampling strategy as `time/internal/ts.Clock.monoSample()`: external pulse timestamp from `ReadExtts()`, system time from a `time.Now()` sandwich, PHC time read in the middle.

## Replay-time reconstruction

Go's monotonic clock component cannot be serialized, so the replay harness synthesizes a fresh monotonic timeline: pick a local base time and map every recorded wallclock `tRead` to `base + (recorded_tRead - capture_start)`. This gives a coherent time axis for both packet/message read times and pulse-edge `TRead.Sys`. PHC-domain values come from the trace and the synthetic transform.

## Synthetic PHC transforms

Apply an affine transform to generate test scenarios from a single free-running capture:

```text
phc' = phase + scale * (phc - phc0)
scale = 1 + freq_ppb * 1e-9
```

Apply to both pulse `Timestamp` and `TReadPHC`. System read times and message times are unchanged (apart from the monotonic timeline remap).

This covers: PHC near zero, PHC close to correct phase, PHC off by seconds, and various frequency offsets.

## Verify

- `satpulsetool sdp -i -j` emits `tReadPHC`.
- Reset mode succeeds for plausible phase/frequency offsets.
- Reset mode stays in reset when delay or pulse interval constraints are violated.
- Correct edge list is chosen in dual-edge mode.
- Step magnitude is reasonable for the injected phase offset.
- Synthetic transforms are applied reproducibly.
