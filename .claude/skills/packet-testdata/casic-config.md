# CASIC (Zhongke) specific capture details

## Firmware families

CASIC has two firmware families sharing packet framing but with
different message classes and several inverted enum encodings:

- **V5** (URANUS5, e.g. ATGM332D-5N71): NAV class (0x01) navigation
  messages, GPS/BDS/GLO only, constellation-level signal selection.
- **V6** (URANUS6, e.g. ATGM332D-...-F8N dual band, AT362/AT632
  timing): NAV2 class (0x11), adds GAL, per-signal selection via
  CFG-NAVBAND.

`--show-receiver` reports the family in the firmware string
("SW=URANUS5,V5.3.0.0" / "SW=URANUS6,V6.3.2.0"). The V5 reports NO
hardware identity (no MON-VER; only the PCAS06 firmware query
answers), so its HW.toml `model` must be supplied from
CLAUDE.local.md, not from `--show-receiver`.

## Acknowledge-but-never-emit: plan captures per unit

CASIC firmware acknowledges enabling messages it never emits, so the
`Supports:` line OVERSTATES what is capturable. The per-unit truth
(from the gpshwtest characterizations in `gpshwtest/HW/`):

| Output | F8N (V6 nav) | AT632 (V6 timing) | 5N71 (V5) |
|---|---|---|---|
| Time of pulse (TIM2-TPX / TIM-TP) | acked, never emits | EMITS | EMITS (TIM-TP) |
| Leap second (TIM2-LS / MSG-GPSUTC) | acked, never emits | EMITS (event-shaped) | acked, never emits |
| Survey (TIM2-TIMEPOS) | acked, never emits | EMITS | n/a |
| Raw (RXM2-MEASX/SFRBX) | acked, never emits | EMITS | n/a |
| RTCM | acked, never emits | CFG NAKed | n/a |

So: pulse-time, leap, survey, and raw captures come from the AT632;
the F8N contributes PVT/satellite/NMEA/dual-band captures; the V5
contributes the whole NAV-class side. No attached CASIC unit emits
RTCM. TIM2-LS is event-shaped (emitted when an event is announced),
so a leap capture may legitimately contain none; do not wait for one.

## How `--pvt-out` maps to CASIC messages

From `generatePVTReqs` in `gps/internal/casic/cascfgmsg.go`. Family
picks the class: V5 NAV-x, V6 NAV2-x.

- `time` without `tai` => NAV-TIMEUTC / NAV2-TIMEUTC
- `time,tai` => NAV-SOL / NAV2-SOL (also carries ECEF pos/vel)
- `pos`/`vel` => NAV-PV / NAV2-PVH; with `ecef` => NAV-SOL / NAV2-SOL
- `qual` => NAV-SOL + NAV-DOP (NAV2-SOL + NAV2-DOP)
- `tp` => TIM-TP; on V6 TIM-TP is NAKed by both known units and the
  configurator falls back to TIM2-TPX automatically
- `leap` => TIM2-LS (V6) / MSG-GPSUTC (V5, acked-never-emitted)
- `survey` => TIM2-TIMEPOS (V6 only)
- `--sats-out sat` => NAV-GPSINFO + NAV-BDSINFO + NAV-GLNINFO (V5) /
  NAV2-SIG (V6; `sig` also delivered by NAV2-SIG; V5 has no
  per-signal information)
- `--raw-out obs,nav` => RXM2-MEASX / RXM2-SFRBX (V6 only)

There is no epoch-marker message; `epoch` enables nothing (epochs are
inferred from the RunTime field shared by navigation messages).

## Messages not reachable via high-level config

Enable via the message files in `configs/gpsmsg/zhongke/`
(`atgm332d-v5.toml`, `atgm332d-v6.toml`, `at632.toml`), second
invocation with `-m` after the high-level one:

- TIM2-TIMEGPS/BDS/GLN/GAL (per-constellation time): tags
  `casbin-tim2-time{gps,bds,gln,gal}` (V6 files)
- TIM2-TPX directly: `casbin-tim2-tpx`
- Anything to a non-default rate; polls via `get-*` tags

## Per-constellation time captures

`--time-gnss` accepts GPS/BDS/GLO on V5, plus GAL on V6 (the AT632
also accepts GAL for the pulse source). Pattern per constellation:

1. `--reload` (see reload behavior below)
2. `--binary --pvt-out tp,after,tai,leap,off --time-gnss <gnss>
   --gnss GPS,<gnss>`
3. `-m configs/gpsmsg/zhongke/<file>.toml -t casbin-tim2-time<gnss>`
   (V6) for the matching TIM2-TIME message
4. Capture, then verify the time source: replay the file and check
   the `gnss` of the time events; TIM2-TPX carries it in TSrc.

## Reload behavior

`--reload` differs by family: V5 acknowledges in place; V6 RESTARTS
the receiver without acknowledging - allow 2-3 s before the next
invocation. Neither family re-applies the NVM-saved baud rate to the
live port on reload, so the link survives a reload at a changed rate.

CAUTION: on V6 units, reload does NOT reliably revert unsaved
configuration (observed: an unsaved minimum-elevation change survived
reload and even a cold reset on the F8N). Do not rely on reload to
clear configuration between captures on V6 - reconfigure explicitly
in each capture's setup and verify the capture contents. The V5
reverts unsaved changes on reload normally (except the live port
rate).

## V5 line budget

The 5N71's factory default is 9600, where the default NMEA load
saturates the line (~6 s response lag, spliced packets); the attached
unit is persistently saved at 115200 (CLAUDE.local.md). Capture at
115200. Record `default-baud = 115200` in HW.toml for this unit (the
NVM-saved rate) and note the 9600 factory default. If a 9600 capture
variant is ever needed, budget the message set against ~960 bytes/s
and expect configuration to need a quiet line (`nmea-quiet` tag)
first.

## Decode-path checklist

Lib layer (`gps/lib/casbin`) and domain layer
(`gps/internal/casic`) conversions to cover, by source:

- V5: NAV-SOL, NAV-PV, NAV-TIMEUTC, NAV-DOP, NAV-GPSINFO/BDSINFO/
  GLNINFO, TIM-TP
- V6: NAV2-SOL, NAV2-PVH, NAV2-TIMEUTC, NAV2-DOP, NAV2-SIG,
  TIM2-TPX, TIM2-TIMEGPS/BDS/GLN/GAL, and on the AT632 TIM2-TIMEPOS
  (survey) and RXM2-MEASX/SFRBX (raw)
- Both: NMEA alongside binary for the cross-protocol satellite
  capture (GSV/GSA vs native satellite info)
- Unobtainable on attached hardware (lib coverage only via synthetic
  tests, not captures): MSG-GPSUTC/BDSUTC, TIM2-LS event payloads,
  RTCM output

## Output directories

```
gps/testdata/packets/zhongke/atgm332d-f8n/   (V6 dual-band)
gps/testdata/packets/zhongke/at632/          (V6 timing)
gps/testdata/packets/zhongke/atgm332d-5n71/  (V5)
```

Device paths, verified identities, and speeds are in
CLAUDE.local.md. All three are UART-to-USB; reload restarts (V6) do
not drop the USB device, no sleep-for-reenumeration needed beyond
the V6 restart wait above.
