# Pseudorange residual per signal (#250)

`PRResidual opt.Val[Length]` on `SignalInfo`.

The pseudorange residual is a signed range-domain error: the difference between
an observed pseudorange and the pseudorange predicted by the receiver's current
solution. It is naturally expressed in meters but is not a geometric distance to
the satellite. A value near zero means the observation fits the solution well; a
large positive or negative value means it does not.

The clean vendor-neutral representation is per satellite signal, not per
satellite. The underlying quantity is attached to one observation, and an
observation is for a specific satellite-signal pair. Protocols that report
residuals per signal populate `SignalInfo.PRResidual` directly. Protocols that
only report one residual per SV can still populate `SignalInfo.PRResidual` using
the same approximation already used for `CN0`: attach the residual to the single
default signal represented in that message.

This does not naturally belong on `SVInfo`. If a receiver reports multiple
signals for the same satellite, those signals can have different residuals.

Where this information is documented in implemented protocols:

- u-blox `UBX-NAV-SIG`: per-signal `prRes` (`0.1 m`) -- maps directly to
  `SignalInfo.PRResidual`
- u-blox `UBX-NAV-SAT`: one `prRes` per SV (`0.1 m`) -- attach to the default
  signal in `SignalInfo`
- u-blox `UBX-NAV-SVINFO`: one `prRes` per SV (`0.01 m`) -- attach to the
  default signal in `SignalInfo`
- CASIC V6 `NAV2-SIG`: per-signal `PRRes` (`0.1 m`) -- maps directly to
  `SignalInfo.PRResidual`
- CASIC V5 `NAV-GPSINFO` / `NAV-BDSINFO` / `NAV-GLNINFO`: one `PRRes` per SV
  (meters) -- attach to the default signal in `SignalInfo`
- Allystar `NAV-SVINFO`: one `PrRes` per SV (cm) -- attach to the default
  signal in `SignalInfo`
- SDBP `DAT-SAT`: one `PRResidual` per entry (cm), and the entry already has a
  `SignalID` -- maps directly to `SignalInfo.PRResidual`

Not available / not currently useful in the paths we use:

- NMEA `GRS`: range residuals in GSA order, with sentence-level `systemId` and
  `signalId` in NMEA 4.10+. Deriving per-signal residuals requires correlating
  `GRS` with the matching `GSA`; the current NMEA path does not parse `GRS`.
- Unicore `GPGRS` / `GNGRS`: same NMEA-style residual output, not available
  from the current `SATSINFO` / `BESTSAT` binary path
- Quectel `GRS`: same NMEA-style residual output, would require NMEA `GRS`
  support
- Unicore `SATSINFO` / `BESTSAT`: no residual field in the current binary path
- standard NMEA `GSV` / `GSA`: no residual field; `GRS` is separate

Comparison to current code:

- `gps/gpsprot/msg.go` already has `SignalInfo` as the right home for this.
  Adding `PRResidual opt.Val[Length]` is an additive change. `Length` is
  signed, so it can represent negative residuals.
- `gps/internal/ubx/ubxsats.go` already builds `SignalInfo` from `NAV-SAT`,
  `NAV-SIG`, and `NAV-SVINFO`, and already treats `NAV-SAT` / `NAV-SVINFO` as
  single-signal approximations for `CN0`. The `prRes` fields exposed by
  `gps/lib/ubxbin/nav.go` can be propagated using the same rule.
- `gps/internal/casic/cassats.go` already has both a single-signal path
  (`NAV-xxxINFO`) and a true per-signal path (`NAV2-SIG`). The residual fields
  are already exposed by `gps/lib/casbin/nav.go` and can be propagated.
- `gps/internal/as/assats.go` already builds one default `SignalInfo` per SV
  from `asbin.NavSVInfo`; `PrRes` can be propagated there.
- `gps/internal/sdbp/sdbpsats.go` already builds `SignalInfo` from `DAT-SAT`
  entries that include both `SignalID` and `PRResidual`; this is a clean direct
  mapping.
- `gps/internal/unc/sats.go` does not currently have any residual value to
  propagate from `SATSINFO` / `BESTSAT`.
- `gps/internal/nmea` does not currently parse `GRS`, so NMEA/Quectel/Unicore
  residual support via `GRS` would be a separate follow-up feature.
