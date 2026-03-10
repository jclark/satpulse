# Separate North/East fields to DOP for latitude and longitude

This also fits cleanly into `NavEpochMsg.DOP`.

`gpsprot.DOP` already uses optional scalar fields with fill-if-unset merge
semantics. Adding two more fields is mechanically the same as the existing
`Geom`, `Pos`, `Hor`, `Vert`, and `Time` fields.

The main naming issue is that gpsd calls these `xdop` and `ydop`, while the
protocols we implement usually call them `nDOP` and `eDOP`. The implemented
protocol docs are much more consistent on north/east naming than on
latitudinal/longitudinal naming.

Where this information is documented:

- Quectel `PQTMDOP`: `GDOP`, `PDOP`, `TDOP`, `VDOP`, `HDOP`, `NDOP`, `EDOP`
- u-blox `UBX-NAV-DOP`: `nDOP`, `eDOP`
- Allystar binary `NAV-DOP`: `nDOP`, `eDOP`
- CASIC `NAV-DOP` / `NAV2-DOP`: `nDop`, `eDop`
- Unicore `ADRDOP`, `PPPDOP`, `PPPDOP2`, `RPPPDOP`, `SPPDOP`: `Ndop`, `Edop`

Not available:

- NMEA `GSA` only provides `PDOP`, `HDOP`, `VDOP`
- NovAtel/Bynav `PSRDOP`/`RTKDOP`/`PDPDOP` families appear to provide the
  common DOP set, not north/east split

Comparison to current code:

- `gps/gpsprot/msg.go` already has `DOP` as an additive optional-field struct.
- `gps/lib/qtmmsg/periodic.go` already parses Quectel `NDOP` and `EDOP`, but
  `gps/internal/quectel/handler.go` only copies `GDOP`, `PDOP`, `TDOP`, `VDOP`
  and `HDOP` into `epoch.DOP`.
- `gps/lib/ubxbin/nav.go` already exposes `NDOP` and `EDOP`, but
  `gps/internal/ubx/ubxpv.go` only copies the existing five common DOP fields.
- `gps/lib/asbin/nav.go` already exposes `NDOP` and `EDOP`, but
  `gps/internal/as/aspv.go` only copies the existing five common DOP fields.
- `gps/lib/casbin/nav.go` already exposes `NDOP` and `EDOP` for both `NAV-DOP`
  and `NAV2-DOP`, but `gps/internal/casic/caspv.go` and
  `gps/internal/casic/caspv2.go` only copy `PDOP`, `HDOP`, `VDOP`, `TDOP`.
- `gps/lib/uncmsg/dop.go` already exposes `NDOP` and `EDOP`, but
  `gps/internal/unc/nav.go` only copies `GDOP`, `PDOP`, `HDOP`, `VDOP`, `TDOP`.
- `gps/internal/nmea/nmeasats.go` can only populate `PDOP`, `HDOP`, `VDOP`
  because that is all `GSA` provides.
- `gps/internal/nov/nav.go` currently populates only the common DOP set because
  that is what the NovAtel/Bynav DOP logs appear to expose.

So this also looks like an easy additive extension. For several protocol
families the parser structs already have the north/east DOP fields; the
current code just has nowhere to put them in `gpsprot.DOP`.
