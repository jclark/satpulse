# Separate accuracy for latitude and longitude (#280)

This looks like a clean fit for `NavEpochMsg.Acc`.

`gpsprot.Accuracy` already consists of optional scalar fields with simple
fill-if-unset merge semantics, so adding two more fields does not require a new
message type or any change to epoch accumulation.

The most obvious vendor-neutral meaning is latitude and longitude 1-sigma
position error in meters. That wording is used directly by standard NMEA and by
several binary protocols we implement.

Concretely, add `Lat` and `Lon` as `opt.Val[Length]` fields. Like the existing
`Pos`, `Hor`, and `Vert` fields, these are linear distances represented by
`gpsprot.Length`; handlers construct them with `gpsprot.Meters`, and their JSON
representation is in metres. They are not angles, variances, or covariance
terms. Add both fields to `Accuracy.Fill` and the generated TypeScript type.

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

## Implementation

1. Add `Accuracy.Lat` and `Accuracy.Lon`, including fill behavior, JSON and
   TypeScript generation, and model tests.
2. Preserve the already-decoded values from Quectel, NovAtel/Bynav/SinoGNSS,
   and Unicore messages instead of retaining only their aggregate `Hor` value.
3. Parse NMEA GST latitude, longitude, and altitude standard deviations into
   `Acc.Lat`, `Acc.Lon`, and the existing `Acc.Vert`. Test missing fields,
   invalid input, talker variants, and epoch association.
4. Add `NMEAMsgGST`, `--nmea-out GST`, and receiver-specific NMEA
   enable/disable mappings. GST is enabled only in NMEA mode; proprietary mode
   continues to use the native messages selected by each backend.
5. Map `Acc.Lon` to gpsd `epx` and `Acc.Lat` to gpsd `epy` without rescaling.

Workbench can use the two values for north/south and east/west error bars. A
rotated error ellipse requires the additional information described below.

## Future work: horizontal error ellipse

A complete horizontal error ellipse is a natural extension of `Accuracy`, but
is not required for the latitude/longitude fields in this issue. Represent it
using the existing unit-bearing types rather than exposing covariance terms in
square metres:

```go
type ErrorEllipse struct {
	Major       Length // one-sigma semi-major axis
	Minor       Length // one-sigma semi-minor axis
	Orientation Angle  // clockwise from true north
}

type Accuracy struct {
	// existing scalar fields, including Lat and Lon
	ErrorEllipse opt.Val[ErrorEllipse]
}
```

This is equivalent to horizontal north/east covariance, but uses the existing
`Length` and `Angle` types. Keep the value atomic and one-sigma; require
non-negative axes with `Major >= Minor`, and normalize orientation modulo 180
degrees. `Lat` and `Lon` remain independently optional because many receivers
do not provide an ellipse.

Populate it directly from complete GST fields in NMEA mode. In proprietary
mode, convert u-blox `NAV-COV` and Septentrio `PosCovGeodetic` horizontal
covariance to this representation. Axis-only native messages such as NovAtel
`BESTPOS` and Unicore `BESTNAV` cannot populate it.

Workbench can render a labelled one-sigma ellipse, or a 95% Gaussian contour
by scaling both axes by approximately 2.4477. Clear it when the matching epoch
has no valid position or ellipse.
