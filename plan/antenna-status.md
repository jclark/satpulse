# Receiver antenna monitor status (#232)

This is not a bool.

The clean vendor-neutral representation is a receiver-reported antenna monitor
state, meaning the receiver's diagnosis of the electrical condition on the
antenna feed, not a statement of physical truth about the antenna itself.

The obvious enum is:

- `unknown`
- `normal`
- `open`
- `short`
- `fault`

where `fault` is the catch-all for vendor-specific abnormal states such as
SinoGNSS `crosstalk` that are neither cleanly `open` nor `short`.

This is not naturally part of `NavEpochMsg`. It is receiver diagnostics rather
than navigation-solution metadata. The better fit would be a separate receiver
status or receiver event message.

The interesting operational signal is often the transition from `normal` to a
non-`normal` state. Most protocols only expose current state, so that
transition would have to be derived from consecutive observations in
higher-level code.

Related but separate axis:

- antenna power should not be folded into the status enum
- when available it is a separate `off` / `on` / `unknown` value

Where this information is documented in implemented protocols:

- u-blox `UBX-MON-RF`: `antStatus` (`INIT`, `DONTKNOW`, `OK`, `SHORT`,
  `OPEN`) and `antPower` (`OFF`, `ON`, `DONTKNOW`)
- Quectel `PQTMANTENNASTATUS`: `Status` (`Unknown`, `Normal`, `Open circuit`,
  `Short circuit`) and `PowerInd` (`Off`, `On`, `Unknown`)
- CASIC vendor text `GPTXT`: `ANTENNA OK`, `ANTENNA OPEN`, `ANTENNA SHORT`
- SinoGNSS `SYSRTS`: `ANT1` / `ANT2` antenna status (`No load`, `Normal`,
  `Short circuit`, `Crosstalk`)

Not available / not currently useful in the paths we use:

- NMEA standard sentences: no standard antenna-status sentence
- Allystar docs in the currently implemented paths do not show a comparable
  clean current-state message
- Unicore docs in the currently implemented message paths do not show a
  comparable current-state output
- Bynav docs do not appear to document a current-state antenna monitor message
  comparable to u-blox `MON-RF` or Quectel `PQTMANTENNASTATUS`

Comparison to current code:

- `gps/gpsprot/msg.go` does not currently have a natural home for this. If we
  pursue it, a dedicated receiver-status or receiver-event message would be a
  cleaner fit than `NavEpochMsg`. Dual-antenna receivers also naturally have
  per-input state rather than one global value.
- `gps/lib/qtmmsg/periodic.go` already parses `PQTMANTENNASTATUS` as
  `AntennaStatus { Status, PowerInd }`, but `gps/internal/quectel/handler.go`
  currently ignores it.
- `gps/internal/nmea` does not consume `TXT`, so CASIC/u-blox text-based
  antenna status is not currently surfaced.
- support for `UBX-MON-RF` would need to be added before `gps/internal/ubx`
  could fill antenna monitor state or power.
- There is no current SinoGNSS-specific parser path for `SYSRTS`.

So this sits outside the easy `NavEpochMsg` additions. Quectel already
provides a clean current-state source, but the semantics belong to receiver
status/event reporting, not epoch quality metadata. The richer and more
interesting version -- transition from `normal` to non-`normal` -- is mostly
not explicit in the currently implemented protocol paths and would need
stateful edge-detection above `gpsprot`.
