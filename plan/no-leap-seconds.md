# Simplify code now that leap seconds are abolished (#502)

## Motivation

As recorded in #502, no further leap second can occur: CGPM Resolution 5
(2026) ends leap seconds with effect from 20 May 2027, and the IERS has
announced that there is none at the end of December 2026. The last leap
second was therefore the one at the end of 2016, and TAI - UTC is fixed at
37 s from 2017-01-01T00:00:00 UTC onwards.

satpulse carries a good deal of machinery that exists only because a leap
second might still happen: a mutable `ptime.LeapSecond` value threaded
through the daemon, a `LeapSecondMsg` by which receivers announce one, a leap
indicator passed to NTP and PTP consumers, handling of 23:59:60, and a
`[leapSecond]` configuration table. This plan removes all of it.

## Scope

This plan is deliberately narrow. It covers the removal that follows directly
from "no more leap seconds", plus the one rule that makes a constant
conversion safe. It does not cover the further changes that the removal makes
possible or attractive.

### Included

- A time message whose time is before 2017 is refused.
- Conversion between TAI and UTC uses the constant 37 s. The
  `ptime.LeapSecond` type and everything built on it go.
- No component announces, schedules or applies a leap second.
- No code represents, skips or formats 23:59:60.
- `LeapSecondMsg`, its handlers, the converters that build it and the
  `PVTMsgLeapSecond` request flag go.
- The `[leapSecond]` configuration table goes.

### Unchanged

- The fields of `TimeMsg`, and the `UTCTime` type apart from its 23:59:60
  handling. `UTCTime` keeps its present representation, including the
  negative time of day used for GLONASS.
- Which time messages are requested from receivers. `PVTMsgTAI`,
  `PVTMsgTimingPTP` and `PVTMsgTimingSerialUTC` keep their meaning, minus the
  leap second flag.
- The preference for TAI-source messages in `compareTimeMsg`, and the
  stale-firmware detector `detectInvalidLeap`.
- `CorrectionParams`, and the computation of it in `unc` and `nov`.
- The packet decoders for leap second messages in the `gps/lib` packages.
- The wire-format libraries `sockrefclock`, `ntptime` and `pmc`, including
  their leap fields, and the shared-memory layout written by `ntpshm`. The
  Go API of `ntpshm` does change, because it takes a `ptime.LeapSecondKind`.
- Leap second fields in RINEX and RTCM, which are file and protocol content.

### Excluded

The following are consequences of this work, not part of it. Each would be a
separate issue with its own plan. If the implementation seems to need one of
them, stop and raise it instead of extending this plan.

- Changing `TimeMsg` to carry a single instant, and removing `UTCTime`.
- Handling a receiver's stale leap second count inside each driver, or using
  a reported `UTCOffset` other than 37 as evidence of one. A receiver with
  out-of-date firmware still reports a time that is wrong by whole seconds
  until it has decoded the current count; this plan changes nothing about
  that.
- Asking receivers for GNSS system time in place of UTC, and moving the
  serial timing path to those messages.
- A message carrying `CorrectionParams`, or any flag on `TimeMsg` describing
  its time base.

## Supported time range

The last leap second ended at 2017-01-01T00:00:00 UTC, which is
2017-01-01T00:00:37 TAI. `ptime` gets a constant for that instant,
`UTCContinuousStart`. Conversion to and from UTC is supported only at and
after it. `ptime.Time` itself is not restricted: arithmetic on a `Time` is
valid for any value.

### Which time messages are in range

`TimeMsg` gets a range predicate, separate from `ComputeTAITime`:

- A message with neither `TAITime` nor `UTCTime` set is in range. Converters
  use an empty `TimeMsg` to report that the receiver has no valid time, and
  `timemsg.Buffer` relies on receiving it.
- Otherwise every populated field must be in range. A converter can set both
  fields from independently validated parts of one packet, as the UBX-TIM-TOS
  converter does, so checking one is not enough.
- `TAITime` is in range if it is at or after `UTCContinuousStart`.
- `UTCTime` is in range if its time of day is below 24 hours and it is at or
  after 2017-01-01T00:00:00 UTC. The first condition matters: 23:59:60 on
  2016-12-31 is represented with a time of day of 24 hours, and `SysTime`
  would otherwise normalize it to exactly the start of the range.

### Where time messages are checked

- `Dispatcher.Time` checks each message after `logMsg` and before
  `timeMsgBuffer.Time`, `sysPulseStartup.timeMsg`, `controller.TimeMessage`
  and `timeTicker.Time`. An out-of-range message is dropped. A warning is
  logged once, naming the message and the time it carried.
- `Replayer.replayEvent` calls `timeMsgBuffer.Time` directly, so it applies
  the same check. A refused message is still written to the event log by
  `logMsg`, so a replayed log can contain one.
- The `TimeTicker` in the workbench's `session` forwards an out-of-range
  message as received, with the derived fields left unfilled. The workbench
  therefore still shows what the receiver reported.

The dispatcher is one of the handlers on a `MultiHandler`, beside the
observers and the GGA synthesizer `nmeasyn.Synth`. Two effects of that are
known and are deliberately left as they are; changing either is outside this
plan.

- The observers do not implement `Time`. They receive time only through
  `Tick`, which the dispatcher's `TimeTicker` calls for accepted messages, so
  they never see a refused one. `TrackLogObserver` caches the UTC time from
  `Tick` and does not clear it at the end of an epoch, so a trackpoint that
  follows a refused time message carries the last accepted time. The same
  happens today in any epoch that has a position and no time message.
- `nmeasyn.Synth` is called directly by the `MultiHandler`, so it still
  receives a refused message. It converts nothing, and uses the UTC time of
  day from the message in the GGA sentence it synthesizes.

`syncsimcmd` needs no check: its start date is the fixed 2025-01-01, and the
`[sim]` `startTime` setting is a time of day only.

### Times derived from the PHC

Not every time that is converted comes from a time message. After reset mode
has stepped the PHC, the PHC is trusted for whole seconds:

- In converging and tracking mode, `Sample.Ref` is the PHC timestamp rounded
  to the nearest second. The clock log formats it as UTC.
- `Dispatcher.timestamp` passes a PHC reading to `sysSample`, which converts
  it to system time for the NTP reference clocks while in tracking mode.

These do not pass through the range check. Normally the PHC holds TAI by
then and the conversion is right. It is not if the PHC still holds an early
value: `doPHCAction` only logs a failed `AdjTime` step, and the controller
goes on to converging mode regardless. A PHC can also be set from outside.

Today such a value is converted anyway: the table applies 36 s to any time
before the end of 2016, giving a meaningless but harmless result. This plan
preserves that behaviour. The two sites convert with the constant and add no
check, so an early PHC value has 37 s subtracted where today it has 36 s.
Neither result means anything, and nothing fails.

After this change `gmUpdate` converts nothing, because the announced UTC
offset is a constant. Its existing guards on `lastSample` and a zero `Ref`
stay, so that the first grandmaster update is sent when it is today.

### What this gives up

The daemon currently updates its leap second from the receiver's
`LeapSecondMsg` when that is later than the one it has. With `[leapSecond]`
configured for an earlier leap second, it can therefore follow a GNSS
simulator scenario set at a past date, including one that plays a leap
second event. After this change such a scenario is refused with the warning.
This is an accepted limitation and is stated in the documentation.

## ptime

### Added

```go
// TAIMinusUTC is the number of seconds by which TAI is ahead of UTC
// at and after UTCContinuousStart.
const TAIMinusUTC = 37

// UTCContinuousStart is the earliest time for which conversion to and from UTC
// is correct. It is 2017-01-01T00:00:00 UTC, when the last leap second ended.
const UTCContinuousStart Time = 1483228837e9

// UTC converts a TAI time to UTC.
func (t Time) UTC() UTCTime

// TAI converts a UTC time to TAI.
func (ut UTCTime) TAI() Time

// Sys converts a TAI time to system time.
func (t Time) Sys() time.Time

// FromSys converts a system time to a TAI time.
func FromSys(sys time.Time) Time
```

The four conversions are plain arithmetic with the constant and are defined
for any value; they do not panic. For a time before `UTCContinuousStart` the
result is not a correct UTC time. This is deliberate: times derived from the
PHC are converted without a range check, as they are today. What keeps
receiver times before 2017 out of the conversions is the range check on time
messages.

### Replacements

| Today | After |
| --- | --- |
| `ls.UTCtoTime(ut)` | `ut.TAI()` |
| `ls.TimeToUTC(t)` | `t.UTC()` |
| `ls.TimeToSys(t)` | `t.Sys()` |
| `ls.SysToTime(sys)`, returning `(Time, bool)` | `FromSys(sys)`; the result is never ambiguous |
| `ls.FormatTime(t)` | `t.Sys().Format(time.RFC3339)` |
| `ls.StateAt(t).UTCOffset` | `TAIMinusUTC` |
| `ls.StateAt(t).LeapTonight`, `ls.UTCStateAt(ut).LeapTonight` | nothing |

### Deleted

- `LeapSecond` and its methods `UTCtoTime`, `SysToTime`, `TimeToSys`,
  `TimeToMJD`, `Compare`, `Date`, `TimeToUTC`, `FormatTime`, `StateAt` and
  `UTCStateAt`. `TimeToMJD` has no caller outside `ptime` and gets no
  replacement.
- `LeapSecondState`, `LeapSecondKind` and its three constants.
- `LeapSecondOnDate`, `LeapSecond2016` and the four `LeapSecond2016`
  constants.
- `GNSSLeapSecond`, `GPSLeapSecond`, `GalileoLeapSecond`, `BeiDouLeapSecond`,
  `gnssLeapSecond`, `guessPastLeapSecondOffset` and `isLastDayOfQuarter`.
- The 23:59:60 handling in `UTCTime.Sub`, `UTCTime.String` and
  `UTCTime.UnmarshalText`. A 23:59:60 value is then written as the following
  midnight and no longer parses; such a value only occurs in a refused
  message.

## gpsprot

### LeapSecondMsg

`LeapSecondMsg` and `UpdateLeapSecond` go, with `MsgHandler.LeapSecond`, the
message's `Dispatch` method and registry entry, and the `LeapSecond` methods
on `DefaultHandler`, `GenericHandler`, `MultiHandler` and `TimeTicker`.

`UnmarshalMsg` rejects unknown message types, so an event log that contains
`leapSecond` events no longer replays. There is no compatibility path. The
build-ignored `migrate_log.go` loses the `LeapSecond` field of `OldLogEvent`,
so old-format leap second entries are skipped on migration.

### TimeMsg and TimeTicker

`ComputeTAITime` loses its argument and becomes the guarded conversion for
receiver data:

```go
// ComputeTAITime returns the time of the message as a TAI time.
// The result is false if the message has no time or is out of range.
func (msg *TimeMsg) ComputeTAITime() (ptime.Time, bool)
```

Code that handles receiver data ahead of the range check uses it, so that an
out-of-range time is not converted by accident. It cannot serve as the range
check itself, because its false result does not distinguish a message with
no time from one that is out of range; the check uses the range predicate.

`TimeTicker` loses its `ls` field, `SetLeapSecond` and `LeapSecond`, and the
constructor becomes `NewTimeTicker(h)`. For an in-range message `fill`
supplies the missing one of `TAITime` and `UTCTime`, and a missing
`UTCOffset` as `TAIMinusUTC`. For an out-of-range message it fills nothing.
A `UTCOffset` set by a converter is still the receiver's own report and is
passed through unchanged.

### PVTMsgLeapSecond

`PVTMsgLeapSecond` goes, from the flag set and from `PVTMsgAny`,
`PVTMsgTimingPTP` and `PVTMsgTimingSerialUTC`. Its JSON name is `leapSecond`
and its `--pvt-out` name is `leap`; both go, and a configuration or command
line that uses either stops being accepted.

The configuration code keeps no knowledge of leap second messages; only the
decoders in `gps/lib` remain. `ubx`, `casic` and `unc` act on the flag today,
and all of that handling goes, including the side that turns the packets off
under the `off` option when the flag is absent: NAV-TIMELS in `ubx`, TIM2-LS
in `casic` and the UTC logs in `unc`. A receiver that was configured to send
these packets therefore keeps sending them, and no high-level option stops
it. They are ignored.

### TimeEstimate

`TimeEstimate.LeapSecond` goes, with the code in `gpscmd.EstimateSystemTime`
that fills it from the kernel's TAI offset and leap status.

`ubx.mgaTime` uses the field for `leapSecs` in UBX-MGA-INI-TIME_UTC, which
is the number of leap seconds since 1980, or 0x80 if unknown. It now sends
`TAIMinusUTC - TAIMinusGPS`, which is 18, when the estimated time is at or
after `UTCContinuousStart`, and unknown otherwise. This is the value already
sent today when the kernel's TAI offset is set; the change is that it no
longer depends on that. The code that adjusts the count when the estimate
crosses a leap second goes.

## Protocol packages

The converters that build `LeapSecondMsg`, and the dispatch that calls them,
go from six packages. The packet decoders in `gps/lib` stay.

| Package | Converters removed | Notes |
| --- | --- | --- |
| `ubx` | NAV-TIMELS (`ubxleap.go`) | Flag handling in `ubxcfgmsg.go` |
| `casic` | TIM2-LS and the leap part of the TIM2-TIME messages | Flag handling in `cascfgmsg.go`. `lastReceiverTime` exists only to date TIM2-LS, so it goes too, along with the `ComputeTAITime` call in `dispatchTime` |
| `unc` | The leap part of the UTC log converters in `time.go` | Flag handling in `cfgopts.go` |
| `nov` | The leap part of the IONUTC converter in `time.go` | |
| `septentrio` | GPSUtc, GALUtc, BDSUtc (`sbfutc.go`) | |
| `sdbp` | DAT-UTCT2, DAT-GNSSU | |

In `unc` and `nov` one function builds both halves: the
`utcConversionParamsFrom...` builders resolve the leap second first, return
it beside the `CorrectionParams`, and fail if it cannot be resolved. They
are reduced to building `CorrectionParams` alone. The test in
`gnssLeapSecondParams` that the raw fields are all zero, meaning the log
carries no data, is kept as the gate, without building a `GNSSLeapSecond`.
The builders no longer need the current time. Their tests keep the
assertions on the correction and lose those on the leap second. Nothing in
the packet processors calls the reduced builders; they are kept for the
correction message that is excluded from this plan.

## Daemon and timing

| Component | Change |
| --- | --- |
| `daemon` | `LeapSecondConfig`, `leapSecondDefault`, `configCapture.leapSecond`, `configCapture.LeapSecond`, `logLeapSecond` and the hand-over of the captured leap second to the dispatcher go. The configuration decoder disallows unknown fields, so a `[leapSecond]` table becomes an error |
| `gpsevent.Dispatcher` | Gains the range check. Loses its `ls` field and `LeapSecond` handler. `sysSample` uses `ref.Sys()` and passes no leap value. `MsgUTCTime` and `SHMWriter.Write` lose their leap parameter |
| `gpsevent` replay | `Replayer` loses its `LeapSecondMsg` case and gains the range check. The replay sampler loses `ls` |
| `timemsg.Buffer` | Loses its `ls` field and `LeapSecond` method. `msgUTCTime` loses the 23:59:60 skip, and `MsgUTCTimer.MsgUTCTime` loses its leap parameter. `GetPostTimeMessages` uses `UTCTime.TAI()` |
| `phcsync.Controller` | Loses its `leapSecond` field and `LeapSecond` method. `gmUpdate` passes only the sync state |
| `ptpgm` | `GrandmasterProps` stops embedding `LeapSecondState`, and `Grandmaster.Update` loses its leap argument. The announced UTC offset is `TAIMinusUTC`. `pmcLeapFlags` goes, so the leap flags are never set. The `leapTonight` log attribute in `ptp4l.go` goes |
| `refclock` | `Sample` loses its leap parameter and `RefClockSample` its `Leap` field; `sockLeap` goes. `LoggingSockRefClock` passes `sockrefclock.LeapNone` |
| `ntpshm` | `Writer.Write` and the platform `write` functions lose their leap parameter, and `shmLeap` goes. The `Leap` field of the segment is written as zero |
| `obs` | `Observer.NTPSample` loses its leap parameter, in the interface and every implementation |
| `pps.Generator` | `msgLeap`, `Sample.Leap` and the block that applies a leap at midnight go; `MsgUTCTime` loses its leap parameter |
| `sseobs`, `logobs.ClockLogObserver` | Lose their `ls` field and `LeapSecond` method |
| `syncsim`, `syncsimcmd` | The hand-built `LeapSecond` values go |
| `phc` | The local `utcOffset = 37` in the build-ignored `phc_cmp.go` becomes `ptime.TAIMinusUTC` |

## Tools and web interface

- `session`: `NewTimeTicker(te)`, and `msgHandler.LeapSecond` with the
  `leapSecond` event it emits to the workbench goes.
- `cmd/satpulsewb`: the `leapSecond` entry in `stickyMsgKinds` goes.
- `gpscmd`: the `leap` name of the PVT flag in `gpsflags.go` goes.
- `gps/ts`: `gpsprot.ts` and `configtarget.ts` are hand-written and are
  edited to match. In `generate.go` the `LeapSecondMsg` sample, its entry in
  the type list, `PVTMsgLeapSecond` in the `PVTMsg` sample and the
  `LeapSecondState` in the `TimeAssist` sample go. `validate.gen.ts` is
  regenerated.
- Workbench: `LeapSecondState`, the "Leap sec" column, the `leapSecond` event
  handling and the PVT flag's checkbox and presets go from `app.tsx`,
  `time-panel.tsx`, `pvt-panel.tsx`, `pvt-group.tsx` and `config-panel.tsx`.
- Workbench derived times: the time panel and the PVT time table both derive
  UTC from TAI, and TAI from UTC, using the receiver's `utcOffset` or, failing
  that, the leap second state. The PVT time table is fed raw time messages,
  which `TimeTicker` has not filled. Both use the constant 37 instead, so
  that the workbench agrees with the daemon. The receiver's `utcOffset` is
  still shown as reported.
- `formatUTCLocal` in the dashboard's and the workbench's `timefmt.ts`
  extracts the seconds separately so that 23:59:60 formats correctly; that
  special case goes.
- The built assets are regenerated and committed.

## Tests and test data

- `ptime_test.go`: the tests of leap second behaviour go. They are replaced
  by tests of the four conversions, including the boundary at
  `UTCContinuousStart`.
- Tests elsewhere that call the old API move to the new conversions:
  `gps/lib/rinex/obs_test.go`, `gps/internal/as/astime_test.go` and
  `time/internal/syncsim/sync_test.go`.
- The range predicate is tested with a message that has no time, each field
  alone, both fields with one out of range, and 2016-12-31T23:59:60.
- Fixtures that use a time message before 2017 are re-dated. A number of
  test files contain pre-2017 dates; not all of them are time message values.
- `time/internal/promobs/testdata/session.jsonl` contains `leapSecond`
  events, which are removed.
- `time/lib/ntpshm/shm_test.go` and `cmd/satpulsewb/sink_test.go` use the
  leap kinds and the `leapSecond` event.
- `internal/gpscmd` replay tests: the `pvt-out` logs under `testdata`, and
  the capture script `testdata/cmd/pvt-out.sh`, record `--pvt-out` values
  that include `leap`, together with the configuration packets sent for
  them. The command lines no longer parse, and the recorded packets for `off`
  no longer include the ones that turn the leap second packets off.
- `gpshwtest`: the leap second message goes from `model.py`, `probes.py`,
  `SEMANTICS.md`, the hardware notes and the five CASIC baselines.
- `smoketest`: `scenarios/config/wb-apply.py` sets
  `PVT_MSG = ["leapSecond"]` and needs another value. The NTP helpers and
  scenarios refer to the leap indicator, which is now always "none".
- The `packet-testdata` skill's `verify-replay.py` checks `leapSecond`
  events, and the `hardware-test-gps-msgs` skill mentions them.

## Documentation

Each document changes in the same commit as the behaviour it describes.

- `docs/man/satpulse.toml.5.md` and `configs/config-schema.json`: the
  `[leapSecond]` table, and the mention of a leap indication on the NTP
  reference clock.
- `docs/man/satpulsetool-gps.1.md` and `docs/gps-config/high-level.md`: the
  `leap` value of `--pvt-out`, including the expansions of the `ptp` and
  `ntp` presets.
- `docs/intro/timing.md`: leap second handling, and the 2017 limit.
- The pages under `docs/gps-module-support/` and
  `docs/internals/vendor-support.md`, where they list leap second message
  support.
- `docs/_includes/NEWS.md`: the removed `[leapSecond]` table, the removed
  `leapSecond` and `leap` flag values, and the 2017 limit, under the
  unreleased version.
- `plan/backlog.md`: its "Leap seconds" section.

Published blog posts are left as they are.

## Sequencing

Seven commits, each leaving `make test` green.

1. `ptime` additions: the two constants and four conversions, with tests.
2. The range check: the range predicate, `ComputeTAITime` without its
   argument and its `casic` caller, the check in `Dispatcher.Time` and
   `Replayer`, and the guard in `TimeTicker.fill`. Fixtures are re-dated.
3. Leap indicator: the leap value leaves `obs`, `refclock`, `ntpshm`,
   `MsgUTCTimer`, `pps.Generator` and `ptpgm`, and the announced UTC offset
   becomes the constant.
4. `LeapSecondMsg`: the message and every `LeapSecond` handler, the
   converters, the reduced `unc` and `nov` builders, `PVTMsgLeapSecond`, the
   `gps/ts` samples and types, the workbench, and the test data that records
   the message or the flag.
5. Constant conversion: every caller moves to the new conversions; the `ls`
   fields, `NewTimeTicker`'s argument and the `[leapSecond]` configuration
   go.
6. `TimeEstimate.LeapSecond`: the field, `mgaTime`, `EstimateSystemTime`,
   the `TimeAssist` sample in `gps/ts` and the hand-written TypeScript type.
7. Old `ptime` API: `LeapSecond` and everything else listed as deleted, with
   their tests.

Step 5 follows steps 3 and 4: the leap indicator and the `LeapSecond`
handlers are the uses of `ls` that are not conversions. Step 6 is
independent of steps 3 to 5. Step 7 is last.

## Points to check during implementation

- **Receivers without time yet.** A receiver may report a default date
  before it has a fix. Check that the warning for a dropped message is
  logged once and does not hide a later, different problem.
- **Packet-log fixtures.** The packet logs used by the smoke tests and the
  packet tests are binary; check whether any carries receiver times before
  2017.
- **The `gpscmd` replay logs.** Find out whether they can be regenerated
  with the test's update option or need new captures from hardware.
- **The leap indicator on the wire.** chrony's SOCK and SHM interfaces and
  the PTP grandmaster settings now always carry "no leap". Confirm that the
  smoke tests which read those fields still describe the intended behaviour.
