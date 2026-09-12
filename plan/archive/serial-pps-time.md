# Serial PPS edge time representation

## Scope

Design note for the timestamp fields carried by serial PPS edges through the
detection backends. It came out of the edge-level event log discussion: the
log envelope's `t` is defined as the time the daemon read the thing, serial
candidates carry no read time today, and tracing where one would come from
exposed a naming problem in the existing types. The event-log record itself
is designed in `serial-pps-event.md`, which depends on this note.

No GitHub issue has been filed yet; add the issue number to the heading when
one exists.

## Problem

`term.ModemControlPinChange` and `serialpps.Edge` each carry a pair of
`time.Time` fields named `Wall` and `Mono`. The names say the distinction is
clock-kind: a wall-clock reading and a monotonic reading of one instant. But
the backends have loaded the fields with two different distinctions:

- **Instant-kind (Linux kernel method).** `Wall` is the kernel's edge stamp
  from `PPS_FETCH` and `Mono` is the `time.Now` taken immediately after the
  fetch (`kernelpps_linux.go`). These are two different instants: the edge,
  and its delivery to the waiter. The `ModemControlPinChange` doc comment
  admits this as an escape clause rather than fixing it.
- **Clock-kind (Windows).** `now()` in `readtime_windows.go` takes two
  adjacent reads associated with the same observation:
  `GetSystemTimePreciseAsFileTime`
  (~100 ns, cannot carry a Go monotonic bit) and `time.Now` (~0.5 ms
  quantized, mono-carrying). Here the names mean what they say, but only
  because the platform forces one instant into two readings.
- **Degenerate (Linux wait and poll).** One `time.Now` reading serves as both
  fields (`readtime.go`, `ppswait_linux.go`); poll interpolates midpoints,
  which preserve both bits.

The conflation has a concrete consequence: `Generator.Sample` computes the
edge's position on the message's UTC timescale with `edge.Mono.Sub(g.msgRead)`,
treating `Mono` as a monotonic reading of the edge. For the kernel method it
is the delivery time, so the wakeup/fetch delay is silently folded into the
edge's UTC label and the inferred pulse-to-message delay. The causal-window
tolerance absorbs it today, but the arithmetic is wrong on its own terms.

It also blocks the event-log record: the envelope `t` of every log record
means "when the daemon read this" (`tRead` for messages, `TReadMono.Sys` for
`pulseEdge`), and a serial candidate has no field with that meaning.

## Solution

Name the fields by instant, not by clock:

- **`Timestamp`**: the timestamp assigned to the edge. Only the kernel backend
  timestamps the physical edge directly; polling yields a bracket-midpoint
  estimate, and the wait backends use the wakeup time as an edge proxy. Its
  wall reading is always meaningful. Go's monotonic-carrying convention
  expresses whether it also has a valid monotonic reading: a poll midpoint
  interpolated from `time.Now` readings keeps its mono bit, a kernel stamp
  constructed from the fetch's wall time never has one. No flag is needed.
- **`TRead`**: the reading captured when the backend's wait or poll
  completed, before any subsequent state or counter validation; always an
  ordinary `time.Now` reading with both bits valid.

Per backend:

| Backend | `Timestamp` | `TRead` |
|---|---|---|
| kernel | kernel edge stamp (wall only) | `time.Now` immediately after the fetch returns, before cancellation and error checks |
| poll (non-Windows) | bracket midpoint | the closing poll's end reading |
| wait (Linux) | the wakeup reading | same reading; `Timestamp == TRead` is the literal truth |
| wait (Windows) | precise wall read at the wakeup (no mono bit) | adjacent `time.Now` read |
| poll (Windows) | bracket midpoint of precise wall reads (no mono bit) | the closing poll's end `time.Now` reading |

The two Windows rows produce similar clock pairs but through different code
paths, and what `TRead - Timestamp` measures varies by backend: delivery latency for
kernel; half the bracket plus half the closing query duration for poll, on
Windows additionally the coarse/precise clock mismatch; zero for the Linux
wait; and, for the Windows wait, the separation between the two adjacent
reads (real elapsed time or preemption, plus the clock mismatch), which is
not edge-detection latency.

This restructures both layers that have the `Wall`/`Mono` pair,
`term.ModemControlPinChange` and `serialpps.Edge`, and the escape clause in
the `ModemControlPinChange` doc comment dissolves: the contract says what the
fields are on every platform and backend.

The field names and the contract deliberately match `phcsync.PulseEdge`
(`Timestamp` plus `TRead`), which has had this structure all along: the edge
timestamped in its own time domain, plus a read time legible in both that
domain and the message-read domain. `resetSampleGenerator.pulseTimes`
(`phcsync/reset.go`) consumes it with exactly the transfer-through-read-time
arithmetic proposed below: anchor at the read time, subtract the read-to-edge
correction measured in the timestamp's own domain. The arithmetic is the
same; only the clock domains and the required rate conversion differ.
The PHC correction interval is measured in PHC ticks and
needs `avgInterval` rate scaling, where the serial correction is on the same
clock and the scale factor is 1 to within ppm over microseconds. And the PHC
read time must be an explicitly paired `phctime.Sample`, where for serial a
single `time.Now` carries both readings, except on Windows (below).

## Consequences

### Generator arithmetic

Once `Timestamp` is honest, `Generator.Sample` transfers the edge onto the message's
timescale through the read time, and uses the result for both the UTC
extrapolation and the age check:

```
edgeSinceMsg := edge.TRead.Sub(g.msgRead) - edge.TRead.Sub(edge.Timestamp)
```

This works uniformly across backends: the second subtraction uses monotonic
arithmetic when `Timestamp` carries a mono bit (Linux poll and wait) and wall
arithmetic for kernel and Windows timestamps. Step-immunity needs
qualifying: the first term is monotonic and immune to wall steps, but a
wall-clock step landing between `Timestamp` and `TRead` still corrupts the short
correction interval. The exposure shrinks from the whole message-to-edge
span to the `Timestamp`-to-`TRead` correction interval; it does not vanish.

### Event-log record

The edge-level event-log record consumes this representation: envelope `t`
comes from `TRead`, the payload `t` from `Timestamp`. Its design is in
`serial-pps-event.md`.

### Why Windows is different

For Linux wait and poll, one `time.Now` value supplies both a useful wall time
and a monotonic reading. Windows needs two calls instead:

- `GetSystemTimePreciseAsFileTime` supplies the precise wall time used for
  `Timestamp`, but the resulting `time.Time` has no monotonic reading.
- `time.Now` supplies the monotonic reading used for `TRead`, but its wall
  time is quantized at about 0.5 ms.

The difference is implemented below the exported edge representation. The
poller keeps a private pair for every clock observation, with fields named by
function rather than clock-kind:

```
type clockReading struct {
    stamp time.Time
    mono  time.Time
}
```

`stamp` is the measurement reading, taken from the most precise clock the
platform has; it feeds `Timestamp`, bracket widths, query durations, and gaps. `mono`
is the pacing reading, always an ordinary `time.Now`; the name is truthful at
this layer, and its monotonic bit is the reason it exists. On other platforms
`now()` puts one `time.Now` value in both fields; in `readtime_windows.go` it
pairs a precise FILETIME read (`stamp`) with an ordinary `time.Now` (`mono`).
`now()` is the only platform seam. `elapsedSince` is uniform:

```
func (r clockReading) elapsedSince(start clockReading) time.Duration {
    return r.stamp.Sub(start.stamp)
}
```

Go's arithmetic selects the right subtraction from the value itself: on Linux
both stamps carry monotonic readings, so `Sub` is monotonic; on Windows they
carry none, so it is wall arithmetic on the precise readings.

Each modem-state query is bracketed by a `clockReading` before and after the
query. The poller interpolates the `stamp` and `mono` coordinates separately.
It uses `elapsedSince` for query durations, gaps, and edge-bracket widths, so
those short measurements use the precise clock on Windows. Scheduling,
deadlines, and prediction use the `mono` coordinate directly, so a system-time
step does not move the polling window. This deliberately trades step-immunity
for resolution only in the short bracket measurements; a wall-clock step
during a bracket can corrupt that bracket.

Within each `now()` pair, the precise wall call and `time.Now` are adjacent,
not simultaneous. Comparing the two halves of a pair therefore includes the
`time.Now` quantization and any time spent or preempted between the calls.
This pairing error does not enter `elapsedSince`, which compares two precise
`stamp` fields; it matters when an edge is transferred between its `Timestamp` and
`TRead` coordinates.

For the Windows wait backend, both calls happen immediately after
`WaitCommEvent` completes. `Timestamp` is the precise wall timestamp assigned to that
wakeup, and `TRead` anchors the same wakeup on the monotonic timeline. Neither
is an independent timestamp of the physical edge. In particular,
`TRead.Sub(Timestamp)` does not measure edge-to-wakeup latency; it measures the
separation and clock-reading mismatch between the two post-wakeup calls.

For the Windows poll backend, `Timestamp` is instead the precise-wall midpoint of the
bracket containing the edge, and `TRead` is taken after the closing state
query. Here `TRead.Sub(Timestamp)` does include the real interval from the estimated
edge to the closing read, as well as the mismatch between the two clock
readings.

The Generator's transfer-through-read-time arithmetic handles both cases. It
measures the long message-to-`TRead` interval monotonically, then uses the wall
values for the short correction from `TRead` back to `Timestamp`. The remaining error
is normally on the scale of the `time.Now` quantum, comfortably below the
roughly 200 ms rejection margin used for second labelling.

The private pair remains useful after `Edge` changes to `Timestamp` and `TRead`. For a
polled edge, the loop continues scheduling from the interpolated `mono`
coordinate, publishes the interpolated `stamp` coordinate as `Timestamp`, and publishes
the closing poll's end `mono` reading as `TRead`. Thus the Windows-specific choice is
confined to `now()`, internal to polling; it is not part of the `Timestamp`/`TRead`
contract or the Generator arithmetic.

One plumbing consequence: `classify` returns the interpolated midpoint
`clockReading` rather than an `Edge`. The loop still needs the midpoint's
`mono` coordinate for `nextEdge` and its `stamp` coordinate for `Timestamp`,
while `Edge.TRead` comes separately from the closing poll's end reading, so
no conversion of a single reading can construct both `Edge` fields; the
`clockReading.edge()` conversion is deleted and the `Edge` is assembled where
the candidate is built.
