# Analyzing what messages to capture

## Two layers to cover

For each receiver protocol, there are two layers of code that process messages:

### Lib layer (`gps/lib/*bin/` or `gps/lib/*msg/`)

This layer decodes raw binary or ASCII packets into Go structs. Each message type has a struct (e.g., `ubxbin.NavPVT`, `casbin.NavPV`). The lib layer is purely structural -- it parses bytes into fields.

Find the lib package for a protocol:
- u-blox: `gps/lib/ubxbin/`
- CASIC: `gps/lib/casbin/`
- Allystar: `gps/lib/asbin/`
- Techtotop/SDBP: `gps/lib/sdbpbin/`
- NovAtel/Bynav: `gps/lib/novmsg/`
- Unicore: `gps/lib/uncmsg/`
- NMEA: `gps/lib/nmeamsg/`

### Domain layer (`gps/internal/<protocol>/`)

This layer converts lib-layer structs into `gpsprot.Msg` types (`TimeMsg`, `PosGeoMsg`, `VelECEFMsg`, `SatellitesMsg`, `SurveyMsg`, `LeapSecondMsg`, `NavEpochMsg`). Not every decoded message produces a `gpsprot.Msg` -- some are configuration responses, diagnostics, or acknowledgements.

Find the domain package for a protocol:
- u-blox: `gps/internal/ubx/`
- CASIC: `gps/internal/casic/`
- Allystar: `gps/internal/as/`
- Techtotop/SDBP: `gps/internal/sdbp/`
- NovAtel/Bynav: `gps/internal/nov/`
- Unicore: `gps/internal/unc/`
- NMEA: `gps/internal/nmea/`

## How to build the coverage list

1. **Enumerate lib-layer messages**: Look at the non-test `.go` files in the lib package. Find all message struct types and their message IDs.

2. **Find which produce gpsprot.Msg**: In the domain package, look for conversion functions that take lib-layer structs and return `*gpsprot.TimeMsg`, `*gpsprot.PosGeoMsg`, etc. Messages that only produce diagnostic output or have no conversion function don't need dedicated captures (though they may appear incidentally).

3. **Classify by time relevance**: Identify which messages produce `TimeMsg`. These are the most important for sync testing. Classify as:
   - **Pre-pulse** (e.g., UBX TIM-TP): `Ref: gpsprot.PrePulse`
   - **Post-pulse** (e.g., UBX NAV-TIMEGPS): `Ref: gpsprot.PostPulse` or no ref
   - **Combined** (e.g., UBX NAV-PVT produces TimeMsg alongside PosGeo and VelGeo)

4. **Identify what high-level config enables**: Check the domain layer's config code to see which messages get enabled by `--pvt-out`, `--sats-out`, etc. Messages not reachable via high-level config need low-level message files.

5. **Check for mutual exclusivity**: Some messages are alternatives (e.g., NavPosLLH vs NavHPPosLLH on HPG receivers, NavVelNED vs NavVelECEF). These need separate captures.

6. **Check for legacy messages**: Some messages are only available on older protocol versions (e.g., NavSVInfo replaced by NavSat). If the receiver doesn't support them, note it and skip.

## Coverage matrix

Build a coverage matrix mapping each message type to the trace(s) that will contain it. Every message that produces a `gpsprot.Msg` should appear in at least one trace. Post-pulse time messages should appear in multiple traces (they are the primary sync testing input).

## NMEA considerations

NMEA sentences are shared across vendors. The default set varies by receiver. Capture the default set first (factory capture), then targeted subsets for specific testing needs (e.g., RMC+GGA for timing correlation, GLL for future parsing support).
