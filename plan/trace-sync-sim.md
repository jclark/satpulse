# Trace-driven controller simulation

Issue: #285

Build a trace-driven simulator for `phcsync` that replays real PHC pulse traces through the converging and tracking modes, modeling the closed-loop effect of PHC adjustments on future timestamps.

A pulse trace is a JSONL log of PPS edges captured from the PHC's external timestamp channel (e.g. via `satpulsetool sdp -i -j`). Each line records the PHC value at the pulse edge, the system time it was read, and the PHC value sampled near the read time, with optional `qErr` sawtooth correction.

Critically, the pulse trace must be captured while the PHC is **free-running** -- not being disciplined by any controller. The simulator layers a disciplined overlay on top of the captured raw evolution, so the underlying trace data must represent the undisciplined oscillator behaviour. A trace captured while another controller was steering the PHC would already contain that controller's adjustments and could not be replayed counterfactually.

## Testing strategy

Three complementary tools test `phcsync` controller behavior:

- **`syncsim`** -- fully synthetic simulation. Controls every parameter: oscillator model, PPS jitter, sawtooth, faults, outages. Tests all modes including fault injection and recovery. The reference tool for controller tuning and regression.

- **timemsg-sync-testing** ([plan](./timemsg-sync-testing.md)) -- replays real pulse traces and real packet logs to test reset mode. Works because during reset the PHC is not adjusted, so captured timestamps can be replayed verbatim with synthetic phase/frequency transforms. Tests the real message-timing and pulse-alignment pipeline.

- **tracesim** (this plan) -- replays real pulse traces through converging and tracking modes with closed-loop PHC modeling. The controller adjusts the simulated PHC, so future timestamps are counterfactual relative to the capture. Does not need packet logs or time messages: converging mode ignores them, and tracking mode uses them only for `qErr` corrections which come from the pulse trace.

Each tool covers a different combination of realism and controllability. `syncsim` gives full control but synthetic data. Timemsg-sync-testing gives real data but only open-loop (reset mode). Tracesim gives real data with closed-loop behavior (converging/tracking) but without fault injection.

A key difference: `syncsim` knows ground truth (the synthetic GPS model defines true time), so it can measure the thing you actually care about — how close the disciplined clock is to UTC. Tracesim cannot do this. It can observe that the controller converges and tracks stably on real oscillator data, but not the absolute accuracy of the result. This makes the two genuinely complementary: `syncsim` validates accuracy (assuming the oscillator/GPS models are correct), tracesim validates that the controller handles real hardware characteristics (assuming the controller logic is correct).

## Goals

- Drive `phcsync.Controller` converging and tracking modes from real captured pulse data.
- Model the closed-loop effect of PHC adjustments on future timestamps.
- Support optional sawtooth correction data (`qErr`) when present in the phase trace.
- Run from a single pulse trace file plus options -- no packet log required.

## Non-goals

- Testing reset mode (handled by timemsg-sync-testing).
- Fault injection (handled by `syncsim`).
- Refactoring `time/clocksim` up front.
- Replacing the existing synthetic `syncsim`.
- Generalizing the design into a reusable library before the first working implementation exists.

## Why closed-loop replay is hard

A trace-driven controller simulator cannot simply replay raw traces unchanged, because the controller modifies the PHC during the run:

- `SetFreqOffset` changes how future PHC time evolves
- `AdjTime` steps the PHC and changes future timestamps
- therefore future pulse timestamps seen by the controller are counterfactual relative to the original capture

The simulator must separate:

- captured free-running behavior
- simulated disciplined behavior layered on top of the capture

## Relationship to existing code

The current split is:

- `time/clocksim`: synthetic clock and PPS models
- `time/internal/syncsim`: application-layer runner that feeds `phcsync`

That split works because `clocksim` owns the synthetic plant:

- raw oscillator model
- PPS model
- timestamp queueing

For trace-driven replay, the plant comes from recorded data, so the current `clocksim` abstraction boundary is not the right one.

## Package structure

Start with a single new internal package:

```text
time/internal/tracesim
```

Keep everything in that package initially:

- trace loading
- replay-time remapping
- disciplined PHC overlay
- merged event loop
- controller driving
- stats and diagnostics

Do not split out a new lower-level library at first.

### Why one package first

We do not yet know what the reusable lower-level abstraction is.

The likely shared ideas with `clocksim` are:

- disciplined PHC overlay logic
- era handling concepts
- common event-loop structure

But the trace-driven problem has a different modeling boundary:

- externally supplied pulse schedule
- trace-backed raw PHC evolution
- optional phase annotations such as `qErr`

Forcing an early split would likely make the abstractions worse. Build the first working version in one package, then extract common pieces later only if they become obvious.

## Input data

The primary input is a pulse trace JSONL file. The simplest source is `satpulsetool sdp -i -j` capturing a free-running PHC (see [timemsg-sync-testing.md](./timemsg-sync-testing.md) for capture procedure).

Each line must include:

- `timestamp` -- PHC value when kernel captured the PPS edge
- `tRead` -- system time when the event was read
- `tReadPHC` -- PHC value sampled near the read time
- `chan` -- channel index

Optional:

- `qErr` -- sawtooth correction in nanoseconds

Options (not from a file):

- edges per pulse (1 or 2, default 1)

No packet log or `HW.toml` is needed. Converging mode ignores time messages, and tracking mode uses them only for `qErr` corrections which are synthesized from the pulse trace.


## Sawtooth / qErr

For this simulator, `qErr` is a property of the pulse trace: it tells us where the true top-of-second lies relative to the observed pulse.

```text
true_second = pulse_time + qErr
```

This is consistent with `clock-model` usage.

At replay time, `qErr` is translated into `TimeMsg.PulseOffset` inputs and delivered to `timemsg.Buffer` at the correct epoch. The `qErr` and the pulse timing are physically coupled -- they come from the same measurement of the same pulse edge. Any `PulseOffset` values that might appear in packet-derived messages cannot be used, because they are tied to the receiver's own timing, not to the PHC pulse timestamps in the trace. The pulse trace is the sole source of pulse corrections.

## Skipping reset mode

The controller always starts in `ModeReset`, which requires time messages to establish the initial second. Since tracesim has no packet log, it must bypass reset mode.

The simulator pre-seeds the controller state that reset mode would have produced:

- `lastSample` with a valid reference time, offset, era, edge index, and system time. The edge index must be consistent with the first trace pulse being a leading edge (needed for dual-edge parity filtering in converging and tracking modes).
- `PulseType` with the configured edges-per-pulse
- estimated frequency from the pulse interval
- PHC phase set close to the correct time (within ~1ms)

This requires a new entry point on `Controller` or a shim that constructs the post-reset state from the first few trace samples.

Converging mode then starts normally: it uses only pulse edges, applies the PI servo, and transitions to tracking when converged.

## Core design

### 1. Trace loader

Loads the pulse trace JSONL and validates:

- monotone ordering of read times
- expected edge count behavior (consistent with edges-per-pulse option)
- required fields present

### 2. Local replay timeline

Go monotonic `time.Time` components cannot be serialized, so the simulator must synthesize a fresh local monotonic timeline:

- choose a local replay base time
- map all recorded wallclock read times to `base + (tRead - tRead[0])`

This gives a coherent time axis for:

- pulse `TRead.Sys`
- controller ticks

### 3. Trace-backed raw PHC model

The trace gives discrete `(tRead, tReadPHC)` pairs at each pulse event. After remapping `tRead` to the replay timeline, these become `(replayTime[i], rawPHCNs[i])` samples.

The raw PHC model is a linear interpolation over these samples: given any replay time, return the raw (undisciplined) PHC value. Linear interpolation between 1-second PPS samples is sufficient -- sub-ppm drift within a single second is negligible.

For pulse events, the trace also gives `timestamp` -- the hardware-captured PHC value at the PPS edge, which is slightly earlier than `tReadPHC`. Both values are needed:

- `timestamp[i]` is the raw PHC at the PPS edge -- the input to the disciplined overlay that produces `PulseEdge.Timestamp.T`
- `tReadPHC[i]` is the raw PHC at the read time -- the input to the overlay that produces `PulseEdge.TRead.PHC.T`

The delta between them is just the kernel delivery delay for that pulse event. It comes directly from the trace.

For non-pulse replay times (tick events), `Now()` needs a raw PHC value. The `(replayTime, tReadPHC)` interpolation table provides this.

**Boundary behavior:** Before the first sample, return the first sample's value. After the last sample, return the last sample's value. In practice the event loop should not generate events outside the trace span, but ticks near boundaries may land slightly outside.

**Difference from `clocksim.RawClock`:** `RawClock` integrates a stateful oscillator simulator and must be called with monotonically increasing times. The trace-backed model is a lookup table with interpolation -- stateless and random-access in principle, though in practice called monotonically.

### 4. Disciplined PHC overlay

The overlay maintains the simulated effect of controller adjustments on top of the raw trace-backed PHC model. The state is:

- `lastAdjTime` -- replay time of last adjustment
- `lastRawPhaseNs` -- raw PHC value at last adjustment
- `lastVirtPhaseNs` -- virtual (disciplined) PHC value at last adjustment
- `freqOffset` -- current frequency offset in PPB
- `era` -- current era value (must start at 1, an odd/certain era; even eras are uncertain and would cause converging mode to drop all pulses)

Given these, the virtual PHC at any replay time is:

```text
rawNow         = rawPHC(replayTime)
rawDelta       = rawNow - lastRawPhaseNs
correctedDelta = rawDelta * (1 + freqOffset / 1e9)
virtNow        = lastVirtPhaseNs + correctedDelta
```

This is the same formula as `clocksim.VirtualClock.computeVirtPhaseNs()`. The only difference is that `rawPHC()` comes from trace interpolation instead of oscillator integration.

**`SetFreqOffset(f)`:** Snapshot current raw and virtual phase at the current replay time, then update `freqOffset`. Identical logic to `clocksim.VirtualClock.SetFreqOffset()`.

**`AdjTime(d)`:** Step the virtual phase by `d`. For the first implementation, apply the step instantly without modeling kernel read-modify-write delay -- the trace already captures real-world timing. If the delay model matters, it can be added later. Increment era twice (once to mark uncertain, once to mark certain), matching `clocksim.TestClock.AdjTime()`.

**`Now()`:** Return `phctime.Time{T: virtNow, Era: era}` where `virtNow` is computed from the overlay at the current replay time.

**Initial virtual phase:** At the start of replay, the overlay has made no adjustments, so `lastVirtPhaseNs = lastRawPhaseNs = rawPHC(startTime)` and `freqOffset = 0`. The PHC starts at whatever value the trace captured.

### 5. How pulse timestamps become counterfactual

When the event loop processes a pulse event at replay time `t`:

1. Look up `timestamp[i]` from the trace -- the raw PHC at the PPS edge
2. Apply the disciplined overlay: `virtTimestamp = lastVirtPhase + (timestamp[i] - lastRawPhase) * (1 + freqOffset/1e9)`
3. That becomes `PulseEdge.Timestamp.T`

Similarly for `PulseEdge.TRead.PHC.T`:

1. Look up `tReadPHC[i]` from the trace
2. Apply the same overlay
3. That becomes `TRead.PHC.T`

`TRead.Sys` comes from the replay timeline remapping of `tRead[i]` and is unaffected by the overlay.

In the original capture, the PHC was free-running. In the simulation, the controller is actively steering it. So after the first `SetFreqOffset` or `AdjTime`, the timestamps diverge from the captured values -- which is exactly the point.

### 6. Event sources and merge loop

The event loop has two sources:

- pulse events from the pulse trace
- periodic tick events (every 250ms, matching `syncsim`)

No message events -- converging mode ignores time messages, and `qErr` corrections (when present) are delivered as synthetic `TimeMsg.PulseOffset` inputs at the pulse event, not as separate message events.

Process events in replay-time order. For each event:

**Tick:** Call `ctrl.Tick(replayTimeSys)`.

**Pulse:** Compute the counterfactual `PulseEdge` using the disciplined overlay (section 5 above). If `qErr` is present for this pulse, synthesize a `TimeMsg` with `Ref = PrePulse` and `PulseOffset = -qErr`, and deliver it to `timemsg.Buffer` before delivering the pulse edge to the controller. Using `PrePulse` (not `PostPulse`) ensures that `WaitForPulseCorrection` does not trigger unintended waits when `qErr` is absent on some pulses. If `qErr` is missing for a given pulse, no correction message is delivered. Then deliver `ctrl.PulseEdge(edge)`.

The merge can reuse the `iter.Seq[Event]` / `mergeEvents` pattern from `syncsim`. The pulse event generator iterates over the loaded trace data; the tick generator is synthetic (identical to `syncsim.generateTickEvents`).

## Interaction with clocksim

Do not try to use `time/clocksim` unchanged for the first implementation.

`clocksim.VirtualClock` currently combines:

- synthetic raw PHC evolution
- synthetic pulse generation
- disciplined clock state

That coupling is appropriate for synthetic simulation but not for trace-driven replay.

Instead:

- reuse ideas from `clocksim`
- possibly copy a small amount of adjustment logic initially
- defer any refactor of `clocksim` until the trace-driven design stabilizes

If a clean common layer later emerges, it can be extracted after the first working version.

## Development stages

### Stage 1: skeleton runner

- load pulse trace
- build local replay timeline
- merge pulse/tick events
- skip reset, start controller in converging mode
- feed controller with trace-derived inputs (without disciplined overlay)

### Stage 2: disciplined trace-backed PHC

- implement adjustment overlay
- ensure controller actions affect future simulated PHC values
- verify eras and step behavior

### Stage 3: sawtooth-aware replay

- support optional `qErr` on pulse trace
- synthesize `TimeMsg.PulseOffset` inputs from it
- compare behavior with and without correction

### Stage 4: stabilization

- identify any logic that truly belongs in a shared lower-level package
- decide whether to extract common code from `tracesim` and `clocksim`

## Verify

- A first working implementation exists entirely under `time/internal/tracesim`.
- It can load a pulse trace with `tReadPHC`.
- Replay uses a synthesized local monotonic timeline.
- Controller starts in converging mode, bypassing reset.
- Controller adjustments affect future simulated timestamps instead of merely replaying captured timestamps.
- Optional `qErr` can be consumed from the pulse trace and turned into appropriate `PulseOffset` corrections.
- No `clocksim` refactor is required to get the first working version.
