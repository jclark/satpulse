# Health field per satellite signal

Health belongs on both `SignalInfo` and `SVInfo`, each as `opt.Val[bool]`
(true = healthy, false = unhealthy, absent = unknown).

The reality is that health is a property of individual signals -- a satellite
can broadcast a healthy L1 signal and an unhealthy L5 signal. Protocols that
report per-signal health (u-blox `NAV-SIG`) populate `SignalInfo.Healthy`.
Protocols that only report per-satellite health (u-blox `NAV-SAT`,
`NAV-SVINFO`, Allystar `NAV-SVINFO`) populate `SVInfo.Healthy`. The semantics
of `SVInfo.Healthy` are vendor-dependent -- it could mean primary signal (L1)
healthy, all signals healthy, or any signal healthy. For u-blox `NAV-SAT` it
is likely the primary signal. When only per-signal health is available,
`SVInfo.Healthy` is left unset -- do not synthesise it from signal-level data.

Where this information is documented in implemented satellite-message
protocols:

- u-blox `UBX-NAV-SIG`: per-signal health field in `sigFlags` (`0=unknown`,
  `1=healthy`, `2=unhealthy`) -- maps to `SignalInfo.Healthy`
- u-blox `UBX-NAV-SAT`: per-SV health field in the flags with the same
  values -- maps to `SVInfo.Healthy`
- u-blox `UBX-NAV-SVINFO`: `unhealthy` flag bit -- maps to `SVInfo.Healthy`
- Allystar `NAV-SVINFO`: parser already exposes an `unhealthy` flag bit; the
  related `NAV-SVSTATE` docs spell the same idea explicitly as `0=unknown`,
  `1=healthy`, `2=not healthy` -- maps to `SVInfo.Healthy`

Not available in the satellite-message families we currently use:

- NMEA `GSV` / `GSA`: no satellite health field
- CASIC `NAV-GPSINFO` / `NAV-BDSINFO` / `NAV-GLNINFO`: no per-satellite
  health field
- Unicore `SATSINFO` / `BESTSAT`: no per-satellite health field in the current
  parser path

Comparison to current code:

- `gps/gpsprot/msg.go` already has `SVInfo` and `SignalInfo` as the homes for
  this. Adding `Healthy opt.Val[bool]` to both is an additive change.
- `gps/internal/ubx/ubxsats.go` already builds `SVInfo` and `SignalInfo` from
  `NAV-SAT`, `NAV-SIG`, and `NAV-SVINFO`, but currently drops the health bits
  that are already exposed by `gps/lib/ubxbin/nav.go`.
- `gps/internal/as/assats.go` already builds `SVInfo` from `NAV-SVINFO`, but
  currently drops `asbin.NavSVInfoFlagUnhealthy`.
- `gps/internal/casic/cassats.go`, `gps/internal/unc/sats.go`, and
  `gps/internal/nmea` do not currently have any health value to propagate from
  the message families they use.
