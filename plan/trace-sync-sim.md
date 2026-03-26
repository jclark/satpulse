# Trace-driven controller simulation

Build a trace-driven simulator for `phcsync` that uses real packet timing and real PHC pulse traces instead of synthetic GPS/PHC models.

This is separate from the current synthetic `syncsim` work. The aim is not to replace `syncsim`, but to add a new controller-validation path based on recorded traces.

## Goals

- Drive `phcsync.Controller` from real captured timing data.
- Preserve real packet/message timing and real pulse-read behavior from captures.
- Model the closed-loop effect of PHC adjustments on future timestamps.
- Support optional sawtooth correction data (`qErr`) when present in the phase trace.

## Non-goals

- Refactoring `time/clocksim` up front.
- Replacing the existing synthetic `syncsim`.
- Introducing synthetic perturbations beyond what is needed to replay the captured traces.
- Generalizing the design into a reusable library before the first working implementation exists.

## Why this is harder than packet replay or reset replay

Packet replay and reset replay can treat captured data mostly as fixed input.

A full trace-driven controller simulator cannot simply replay raw traces unchanged, because the controller modifies the PHC during the run:

- `SetFreqOffset` changes how future PHC time evolves
- `AdjTime` steps the PHC and changes future timestamps
- therefore future pulse timestamps seen by the controller are counterfactual relative to the original capture

The simulator must therefore separate:

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
- phase-trace interpretation
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
- externally supplied message timing
- trace-backed raw PHC evolution
- optional phase annotations such as `qErr`

Forcing an early split would likely make the abstractions worse. Build the first working version in one package, then extract common pieces later only if they become obvious.

## Input data

The simulator consumes captured data described by two companion plans:

- [packet-testing.md](./packet-testing.md) -- packet log collection and replay (`gps/testdata/packets/`)
- [timemsg-sync-testing.md](./timemsg-sync-testing.md) -- pulse trace capture with `tReadPHC` and synthetic PHC transforms (`time/testdata/phase/`)

Both must come from the same capture session so that message timing and pulse-to-message delay relationships are real.

Concretely:

- packet log or event log for GPS message timing (from `gps/testdata/packets/`)
- pulse trace captured from `satpulsetool sdp -i -j` (from `time/testdata/phase/`)
- `HW.toml` describing the GPS and PHC hardware

The pulse trace should include at least:

- `timestamp`
- `tRead`
- `tReadPHC`
- `chan`

Optional:

- `qErr`

## Sawtooth / qErr

For this simulator, `qErr` should be treated as optional phase-trace data.

From the measurement/modeling perspective, `qErr` is a property of the pulse trace:

- it tells us where the true top-of-second lies relative to the observed pulse
- equivalently: `true_second = pulse_time + qErr`

This is consistent with `clock-model` usage.

At replay time, `qErr` should still be translated into the satpulse runtime shape by synthesizing the appropriate `TimeMsg.PulseOffset` inputs at the correct epoch. But the canonical trace representation may store it with the PHC pulse trace.

## Core design

The simulator should have these conceptual pieces.

### 1. Trace loader

Loads:

- pulse trace JSONL
- packet log or event log
- `HW.toml`

Also validates:

- monotone ordering of read times
- expected edge count behavior
- required fields

### 2. Local replay timeline

Go monotonic `time.Time` components cannot be serialized, so the simulator must synthesize a fresh local monotonic timeline:

- choose a local replay base time
- map all recorded wallclock read times to `base + delta_from_capture_start`

This gives a coherent local time axis for:

- pulse `TRead.Sys`
- packet/message read times
- controller ticks

### 3. Trace-backed raw PHC model

Create a representation of the free-running PHC as a function of replay time, derived from the pulse trace.

This is the hard part of the design.

The model must support at least:

- PHC value at pulse timestamps
- PHC value at pulse read times (`tReadPHC`)
- PHC value queried between pulse events for controller operations such as `Now()`

The initial implementation can use interpolation over the captured PHC samples. Exact modeling can evolve later.

### 4. Disciplined PHC overlay

On top of the raw trace-backed PHC model, maintain the simulated controller adjustments:

- frequency offset changes
- time steps
- era transitions

This is conceptually similar to `clocksim.VirtualClock`, but should be implemented locally in `tracesim` first rather than by modifying `clocksim` up front.

### 5. Event sources and merge loop

Build a merged event loop similar in spirit to `syncsim`:

- pulse events from the pulse trace
- message events from the packet/event source
- periodic tick events

Process them in timestamp order and feed:

- `phcsync.PulseEdge`
- `timemsg.Buffer`
- `Controller.Tick`

## Message source choice

Two possible message sources:

1. Packet log replay
- decode packets during simulation
- closer to real runtime pipeline
- more moving parts

2. Unified event replay
- replay already-decoded timing messages
- simpler for controller experiments
- depends on unified event format existing first

Either is acceptable. The first implementation should choose whichever gets a working end-to-end path faster.

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

- load traces
- build local replay timeline
- merge pulse/message/tick events
- feed controller with trace-derived inputs
- no attempt to share code with `clocksim`

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
- It can load a pulse trace with `tReadPHC` and a matching message source.
- Replay uses a synthesized local monotonic timeline.
- Controller adjustments affect future simulated timestamps instead of merely replaying captured timestamps.
- Optional `qErr` can be consumed from the pulse trace and turned into appropriate time-message corrections.
- No `clocksim` refactor is required to get the first working version.
