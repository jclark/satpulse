# Serial PPS event-log record

## Scope

Design note for an edge-level event-log record for serial PPS. It depends on
`serial-pps-time.md`: the record's timestamp mapping is defined in terms of
the `Timestamp`/`TRead` edge representation designed there.

No GitHub issue has been filed yet; add the issue number to the heading when
one exists.

## Problem

Serial PPS edges leave no trace in the event log. The log is the record of
what the daemon observed: every `gpsprot.Msg` is logged via `logMsg` whether
or not anything downstream uses it, and PHC external timestamps are logged as
pulse-edge records. A serial candidate edge is an observation of the same
standing as a packet or message, and events drive observability: the log,
replay, and post-hoc analysis, alongside the live outlets.

The existing pulse-edge record cannot carry a serial edge as-is. Its payload
is entirely PHC-domain: `t` (the latched PHC timestamp, a `ptime.Time`),
`era`, and `tRead` (the PHC reading paired with the read). A pulse edge
timestamped with the system clock has none of that and instead carries what
detection contributes (uncertainty, settled state). In the log's schema the
`type` field discriminates the shape of `data` (`LogEvent.UnmarshalJSON`),
so the two shapes must be reconciled one way or another. Both records are
pulse edges of the same physical pulse, so any name pair must be on the axis
that actually distinguishes them, which is the timestamping clock.

## Solution

Two record types, named by the timestamping clock:

- **The existing record is renamed `phcPulseEdge`** (Go type
  `PHCPulseEdge`). Its payload is unchanged: `t`, `era`, and `tRead`, with
  the `ptime.Time` fields in the `%d.%09d` text form.
- **A new `sysPulseEdge` record** (Go type `SysPulseEdge`) carries the
  serial edge: `t` is a `time.Time` marshalled as RFC3339, `uncertainty` is
  a `gpsprot.Duration` (decimal seconds), and `settled` is a bool.

An earlier draft instead reconciled both shapes under the single `pulseEdge`
type, with `data.t` becoming a domain-neutral `ntime.Time` and a `timescale`
field making the domain explicit data. That required moving `ntime` from
`time/lib` under `gps/lib` to satisfy the import layering, and it never had
a clean answer for what `timescale` should say on a PHC record: the latched
`t` is the raw era-qualified PHC reading, which is not unconditionally TAI.
Splitting by clock dissolves both problems. Each record keeps its natural
representation, and the type name carries the domain: a PHC reading has no
honest UTC calendar interpretation, which is what the `%d.%09d` form
expresses, while a system-clock reading is UTC and gets the same RFC3339
form as the envelope `t`, making the payload directly comparable to the
envelope by eye.

The timestamp mapping comes from `serial-pps-time.md`: envelope `t` is
`TRead`'s wall reading, envelope `mono` derives from `TRead`'s monotonic bit
through the existing `logEvent` computation, and `data.t` is `Timestamp`'s
wall reading. A serial record looks like:

```
{"type":"sysPulseEdge","t":"2026-08-17T04:12:01.000026Z","mono":86.400026,
 "data":{"t":"2026-08-17T04:12:01.000000012Z",
         "uncertainty":0.000026,"settled":true}}
```

The rename is a log-format change: `LogEvent.UnmarshalJSON` dispatches on
the type string, so logs recorded under the old name need the string
renamed (in-repo, `testdata/fast.jsonl`). `migrate_log.go` still reads the
legacy sparse `pulseEdge` data key but emits type `phcPulseEdge`, so
old-format logs migrate directly to the new name.

## Emission

The record is emitted where the `CandidateEdge` reaches the dispatcher
(`serialPPSCandidateEdge`), before the `Settled` filter: the log-everything
convention `logMsg` already follows. Unsettled acquisition then reads
directly from the log (the pre-settle halving sequence in `uncertainty`,
misses as gaps in the record stream). Volume is at most one record per pulse
period, the same as `phcPulseEdge`. Replay ignores `sysPulseEdge` records:
`ReplayFile` drives the phcsync controller, which consumes only PHC edges.

## Open decisions

- The backend must not be inferred from field presence or from `data.t`
  equalling the envelope `t`; if the method matters in the log, record it
  explicitly (a `method` field, or accept the daemon log line as the record
  of it).
- The live side of the same event: whether the workbench gets the edge
  stream via `sseobs`, or only the sample-level observability that issue
  #274 defines.
