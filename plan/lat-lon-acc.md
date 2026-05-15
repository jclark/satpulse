# Separate accuracy for latitude and longitude

Issue: #280

This looks like a clean fit for `NavEpochMsg.Acc`.

`gpsprot.Accuracy` already consists of optional scalar fields with simple
fill-if-unset merge semantics, so adding two more fields does not require a new
message type or any change to epoch accumulation.

The most obvious vendor-neutral meaning is latitude and longitude 1-sigma
position error in meters. That wording is used directly by standard NMEA and by
several binary protocols we implement.

Where this information is documented in implemented protocols:

- NMEA `GST`: `stdLat`, `stdLon`, `stdAlt` (but GST not parsed yet)
- Allystar `NAV-PVERR`: `stdlat`, `stdlon`, `stdalt`
- Quectel `NAV`: `LatStd`, `LonStd`, `AltStd`
- NovAtel/Bynav `BESTPOS`-style logs: `Lat σ`, `Lon σ`, `Hgt σ`
- SinoGNSS `BESTPOS`-style logs: `Lat σ`, `Lon σ`, `Hgt σ`
- Unicore `BESTNAV`/related logs: `lat σ`, `lon σ`, `hgt σ`

Related implemented binary messages that are not the same thing:

- CASIC `NAV-PV` / `NAV2-PVH`: only aggregate `hAcc` / `vAcc`
- u-blox `UBX-NAV-POSLLH` / `UBX-NAV-PVT`: only aggregate `hAcc` / `vAcc`
- u-blox `UBX-NAV-COV`: N/E/D covariance terms rather than direct lat/lon
  sigma fields

Comparison to current code:

- `gps/gpsprot/msg.go` already has `Accuracy` as an additive optional-field
  struct (`Pos`, `Hor`, `Vert`, `Speed`, `GroundSpeed`, `Course`).
- `gps/internal/quectel/handler.go` already gets axis-specific position sigma
  from `PQTMNAV` (`LatStd`, `LonStd`, `AltStd`), but collapses lat/lon into
  `Acc.Hor = sqrt(lat^2 + lon^2)` and stores only altitude as `Acc.Vert`.
- `gps/internal/nov/nav.go` does the same collapse for `LatSigma`/`LonSigma`
  from `BESTPOS`-style messages and stores `HgtSigma` in `Acc.Vert`.
- `gps/internal/ubx/ubxpv.go`, `gps/internal/as/aspv.go`,
  `gps/internal/casic/caspv.go`, `gps/internal/casic/caspv2.go`,
  and `gps/internal/unc/nav.go` already populate the existing aggregate
  accuracy fields, so adding `Lat`/`Lon` would be additive rather than a
  redesign.
- `gps/internal/nmea` currently consumes GSA DOP but does not consume `GST`, so
  plain NMEA would need new handler work before these fields could be filled
  from generic NMEA streams.

So the missing piece is mostly model plumbing: the data already exists in
multiple implemented protocol families, and in some cases we are already
parsing it but discarding the axis split by reducing it to `Acc.Hor`.
