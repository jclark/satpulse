# CASIC (Zhongke) specific capture details

## Firmware families

CASIC has two firmware families sharing packet framing but with
different message classes and several inverted enum encodings:

- **V5** (URANUS5, e.g. ATGM332D-5N71, AT6558D): NAV class (0x01)
  navigation messages, GPS/BDS/GLO only, constellation-level signal
  selection.
- **V6** (URANUS6, e.g. ATGM332D-...-F8N dual band, AT372-6P nav,
  AT362/AT632 timing): NAV2 class (0x11), adds GAL, per-signal
  selection via CFG-NAVBAND.

`--show-receiver` reports the family in the firmware string
("SW=URANUS5,V5.3.0.0" / "SW=URANUS6,V6.3.2.0"). The V5 has no
MON-VER; its identity comes from the PCAS06 text queries, and how
much they say is per-unit: the AT6558D answers the hardware query
with `AT6558D,0000000000000`, while the 5N71's hardware string was
empty and its HW.toml `model` had to be supplied from CLAUDE.local.md
instead of `--show-receiver`.

## Acknowledge-but-never-emit: plan captures per unit

CASIC firmware acknowledges enabling messages it never emits, so the
`Supports:` line OVERSTATES what is capturable. The per-unit truth
(from the gpshwtest characterizations in `gpshwtest/HW/`):

| Output | F8N (V6 nav) | AT372-6P (V6 nav) | AT632 (V6 timing) | 5N71/AT6558D (V5) |
|---|---|---|---|---|
| Time of pulse (TIM2-TPX / TIM-TP) | acked, never emits | acked, never emits | EMITS | EMITS (TIM-TP) |
| Leap second (TIM2-LS / MSG-GPSUTC) | acked, never emits | not delivered | EMITS (current leap, no pending event) | acked, never emits |
| Survey (TIM2-TIMEPOS) | acked, never emits | EMITS | EMITS | n/a |
| Raw (RXM2-MEASX/SFRBX) | acked, never emits | EMITS | EMITS | n/a |
| RTCM | acked, never emits | never emits | CFG NAKed | n/a |

So: pulse-time and leap captures come from the AT632; survey and raw
from the AT632 or the AT372-6P; the F8N contributes
PVT/satellite/NMEA/dual-band captures; the V5 contributes the whole
NAV-class side. No attached CASIC unit emits RTCM.

TIM2-LS on the AT632 is emitted (roughly once per second per monitored
constellation) when `leap` is enabled and at least one non-GPS
constellation is active; a GPS-only fix emits none. Both facts
re-measured 2026-07-22: 60 packets in 30 s with GPS+BDS active, zero
in 15 s with `--gnss GPS`. (The gpshwtest at632.md note recorded leap
as undelivered and has been corrected; its baseline keeps the
leap-missing entry until a re-characterization.) Its RaimType field
is 0 before the UTC almanac is decoded (zero leap fields) and 1 once
decoded (carrying the current leap, Dtls==Dtlsf, no pending change). It
does not reach RaimType 2 (a pending leap event) without an actual
scheduled leap, so a leap capture exercises the current-leap path but
not a pending-event one.

## How `--pvt-out` maps to CASIC messages

From `generatePVTReqs` in `gps/internal/casic/cascfgmsg.go`. Family
picks the class: V5 NAV-x, V6 NAV2-x.

- `time` without `tai` => NAV-TIMEUTC / NAV2-TIMEUTC
- `time,tai` => NAV-SOL / NAV2-SOL (also carries ECEF pos/vel)
- `pos`/`vel` => NAV-PV / NAV2-PVH; with `ecef` => NAV-SOL / NAV2-SOL
- `qual` => NAV-SOL + NAV-DOP (NAV2-SOL + NAV2-DOP)
- `tp` => TIM-TP (V5) / TIM2-TPX (V6; the V6 protocol does not
  document TIM-TP and the configurator does not attempt it)
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
  `casbin-tim2-time{gps,bds,gln,gal}` (at632.toml only;
  atgm332d-v6.toml has no TIM2-TIME* tags)
- TIM2-TPX directly: `casbin-tim2-tpx`
- Anything to a non-default rate; polls via `get-*` tags

## Per-constellation time captures

`--time-gnss` accepts GPS/BDS/GLO on V5, plus GAL on V6 (the AT632
also accepts GAL for the pulse source). Pattern per constellation:

1. `--binary --pvt-out tp,after,tai,leap,off --sats-out none
   --raw-out none --time-gnss <gnss> --gnss GPS,<gnss>` (no reload -
   see the V6 strategy below; on V5 use `--reload` first as normal)
2. (V6) `-m configs/gpsmsg/zhongke/<file>.toml -t <the three OTHER
   casbin-tim2-time*-off tags>,casbin-tim2-time<gnss>` - enable the
   target TIM2-TIME message AND disable the other three, since V6
   leaves prior message-file enables in place
3. Settle until time is valid (the `--gnss` change briefly drops the
   fix), then capture
4. Verify the time source: replay the file and check the `gnss` of the
   time events; TIM2-TPX carries it in TSrc, TIM2-TIME<gnss> in its own
   field. Note: on V6 a GPS-only capture (`--gnss GPS`) emits no
   TIM2-LS (see the leap note above).

## Reset/reload behavior and the V6 capture strategy

V5: `--reload` works - it reverts unsaved RAM config to the NVM-saved
state in place, without restarting. Reset between V5 captures with
`--reload` as normal. (Neither family re-applies the NVM-saved baud to
the live port on reload, so the link survives a reload at a changed
rate.)

V6: there is NO working "load config from flash". `--reload` sends
CFG-CFG (0x06 0x05) opMode 2 ("load FLASH config to current"), but the
firmware ACKs it and does NOT apply it - it merely RESTARTS the GNSS
engine. `--reset` (CFG-RST) likewise restarts without loading config.
So a V6 reload/reset is the worst of both: it does NOT restore a clean
config (prior RAM config - enabled messages, the sats/raw axes, the
message-file TIM2-TIME enables - all persist), AND it drops the time
lock, producing a re-acquisition transient at the start of the next
capture where NAV2-TIMEUTC/NMEA time is not-yet-valid (TFlags lacks
TOWValid|Reliable) and the decoder correctly emits empty TimeMsgs.
(Firmware limitation, per the gpshwtest HW characterizations in
`gpshwtest/HW/`. CFG-RST has only a resetMode field
{hot,warm,cold,factory} - no BBR mask to force a flash reload; PCAS10
restart modes preserve the live config; only a factory reset or a power
cycle replaces the working config wholesale.)

Therefore DO NOT reload or reset between V6 captures. Keep the receiver
continuously LOCKED and make every capture self-describing:

1. Disable everything not wanted. High-level `--pvt-out off
   --sats-out none --raw-out none` clears the PVT/sat/raw axes. The
   message-file-only items are NOT cleared by that, so also disable
   them via `-m` `-off` tags: the four
   `casbin-tim2-time{gps,bds,gln,gal}-off` (plus
   `casbin-tim2-timeirn-off`), `survey-off`, `fixed-pos-off`, and
   `casbin-rxm2-{measx,sfrbx}-off` if raw was on.
2. Enable exactly what the capture wants (high-level flags, plus `-m`
   enables for TIM2-TIME*/TIM2-TPX).
3. Settle to valid time before capturing. Changing `--gnss` (the
   per-constellation captures) briefly drops the fix; a plain message
   reconfig does not (the receiver stays locked). Gate the capture on a
   short probe: capture ~5 s, replay, confirm the latest time events
   carry taiTime/utcTime; retry until valid.
4. Capture, then verify the message set matches (no leakage) and the
   FIRST time event already has valid time (no leading empty-time
   transient). Re-capture if either fails.

This is deterministic regardless of prior state and yields clean config
plus valid time throughout. A true `factory.jsonl` cannot be reached
without a reset on V6, so capture the default NMEA set explicitly
(`--pvt-out off --sats-out none --raw-out none --nmea-out
GGA,GLL,GSA,GSV,RMC,VTG,ZDA`) rather than relying on reload to expose
it.

V6 `--reload` IS still useful in one spot: it is the only way to force
the engine restart for a deliberate cold-start capture - save the
desired set to NVM first (the restart keeps it), then capture the
re-acquisition.

## V5 line budget

The V5 factory default is 9600, where the default NMEA load saturates
the line (~6 s response lag, spliced packets). The detached 5N71 was
persistently saved at 115200; the attached AT6558D rests at 9600
(running and NVM) and sessions raise the running speed to 115200
without saving (see gpshwtest/HW/at6558d.md). Capture at 115200 after
raising the speed, and record the unit's NVM-saved rate as
`default-baud` in HW.toml. If a 9600 capture variant is ever needed,
budget the message set against ~960 bytes/s and expect configuration
to need a quiet line (`nmea-quiet` tag) first.

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
  tests, not captures): MSG-GPSUTC/BDSUTC, a TIM2-LS with a pending
  leap event (RaimType 2), RTCM output

## Output directories

```
gps/testdata/packets/zhongke/atgm332d-f8n/   (V6 dual-band)
gps/testdata/packets/zhongke/at632/          (V6 timing)
gps/testdata/packets/zhongke/atgm332d-5n71/  (V5)
```

Device paths, verified identities, and speeds are in
CLAUDE.local.md. All three are UART-to-USB; a V6 engine restart does
not drop the USB device, so no sleep-for-reenumeration is needed.
