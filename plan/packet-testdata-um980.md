# Packet capture plan: Unicore UM980

Captures packet logs from the UM980 on abondance.lan for use in [packet testing](./packet-testing.md).

The UM980 is the first non-u-blox receiver in the test data suite. As a NovAtel-family receiver, every data message has both an ASCII (`A`) and binary (`B`) variant carrying the same data. Captures include simultaneous ASCII+binary logs for all message types -- a new category of capture specific to NovAtel-family receivers. See `unicore-config.md` in the packet-testdata skill for details.

This plan serves as a template for similar captures on other Unicore receivers (UM981, UM982, UM960).

## Hardware

- Device: `/dev/ttyUSB0` (USB-serial)
- Receiver: Unicore UM980, Build17548
- Baud: 115200 (fixed -- no speed changes)

## Key principle: reload before every capture

Each capture must start from a known clean state. `--reload` resets the receiver to NVM settings. Unlike u-blox, the UM980 factory default has no message output, so `--reload` gives a clean slate with zero messages. Allow `sleep 5` after reload before the next command since the receiver needs time to restart.

## How high-level config maps to messages on the UM980

From `gps/internal/unc/cfgopts.go`:

- `time` or `tp` -> RECTIMEB
- `pos`/`vel`/`qual` -> BESTNAVB (or BESTNAVXYZB with `ecef`). BESTNAV carries both position and velocity.
- `qual` -> STADOPB
- `leap` -> GPSUTCB, BD3UTCB, GALUTCB (per enabled GNSS). Note: BDSUTCB is NOT enabled.
- `sat` -> SATSINFOB + BESTSATB
- NMEA -> GPRMC, GPGGA, GPGSA, GPGSV, GPZDA, GPVTG, GPGLL

High-level config only enables binary variants. ASCII variants must be added via message file tags or ephemeral `-m -`.

Messages not reachable via high-level config:
- PPPNAV (requires PPP configuration via message file)
- PPSSTATUS (not mapped to any high-level flag)
- BDSUTC (only BD3UTC is enabled by `leap`)
- All ASCII variants of any message
- NovAtel-format messages (BESTPOS, BESTXYZ)
- NMEA sentences (when `--binary` is used)

## Ephemeral `-m -` for ASCII variants

When using `-m -` to pipe TOML via stdin for dual captures, the TOML **must** include `[default.line]` with `responsePattern` and `delay`:

```
printf '[default.line]\nresponsePattern = "unicore"\ndelay = 0.1\n[[line]]\ntext = "BESTNAVA 1"\n' | \
  satpulsetool gps -d /dev/ttyUSB0 -s 115200 --vendor unicore -m - \
  --packet-log <file> --capture 30
```

Without `[default.line]`, commands are sent without waiting for acknowledgment and may not take effect.

## Capture list

All captures use `-d /dev/ttyUSB0 -s 115200`. Every capture is preceded by `--reload` + `sleep 5`. Output directory: `gps/testdata/packets/unicore/UM980/`.

### Binary-only captures

#### rectime.jsonl

RECTIMEB only. Minimal binary time-only capture.

```
satpulsetool gps ... --reload
satpulsetool gps ... --binary --pvt-out time,off --packet-log rectime.jsonl --capture 30
```

**Messages:** RECTIME (UNCB).

#### daemon.jsonl

Standard satpulsed binary message set.

```
satpulsetool gps ... --reload
satpulsetool gps ... --binary --pvt-out pos,time,qual,leap,off --sats-out sat \
  --packet-log daemon.jsonl --capture 30
```

**Messages:** RECTIME, BESTNAV, STADOP, SATSINFO, BESTSAT, GPSUTC, BD3UTC, GALUTC (all UNCB).

### NMEA captures

#### nmea-default.jsonl

Full standard NMEA set. Since UM980 has no default NMEA output, enable all sentences explicitly via message file tags.

```
satpulsetool gps ... --reload
satpulsetool gps ... --vendor unicore \
  -m configs/gpsmsg/um980.toml \
  -t nmea-rmc,nmea-gga,nmea-gsa,nmea-gsv,nmea-vtg,nmea-gll \
  --packet-log nmea-default.jsonl --capture 30
```

**Messages:** GNRMC, GNGGA, GNGSA, xGSV (multi-constellation), GNVTG, GNGLL.

#### nmea-rmc.jsonl

RMC only. Tests a single NMEA sentence type.

```
satpulsetool gps ... --reload
satpulsetool gps ... --vendor unicore \
  -m configs/gpsmsg/um980.toml -t nmea-rmc \
  --packet-log nmea-rmc.jsonl --capture 30
```

**Messages:** GNRMC.

#### nmea-sats-uncb.jsonl

Cross-protocol satellite capture: NMEA GSA/GSV alongside native UNCB satellite messages for cross-protocol validation.

```
satpulsetool gps ... --reload
satpulsetool gps ... --pvt-out time,off --sats-out sat --nmea-out RMC,GSA,GSV \
  --packet-log nmea-sats-uncb.jsonl --capture 30
```

**Messages:** GNRMC, GNGSA, xGSV (NMEA) + SATSINFO, BESTSAT, RECTIME (UNCB).

### Dual ASCII/binary captures

These enable both A and B variants simultaneously. For each:
1. Reload + sleep 5
2. High-level config enables binary variants
3. Ephemeral `-m -` (with `[default.line]` responsePattern) adds ASCII variants and captures

#### rectime-dual.jsonl

RECTIME in both formats.

```
satpulsetool gps ... --reload
satpulsetool gps ... --binary --pvt-out time,off
printf '[default.line]\n...\n[[line]]\ntext = "RECTIMEA 1"\n' | \
  satpulsetool gps ... --vendor unicore -m - --packet-log rectime-dual.jsonl --capture 30
```

**Messages:** RECTIME (UNCB + UNCA).

#### bestnav-dual.jsonl

BESTNAV + RECTIME. Covers PosGeo + VelGeo + TimeMsg.

```
satpulsetool gps ... --binary --pvt-out pos,vel,time,off
# -m - adds: BESTNAVA 1, RECTIMEA 1
```

**Messages:** BESTNAV (UNCB + UNCA), RECTIME (UNCB + UNCA).

#### bestnavxyz-dual.jsonl

BESTNAVXYZ + RECTIME. Covers PosECEF + VelECEF.

```
satpulsetool gps ... --binary --pvt-out pos,vel,ecef,time,off
# -m - adds: BESTNAVXYZA 1, RECTIMEA 1
```

**Messages:** BESTNAVXYZ (UNCB + UNCA), RECTIME (UNCB + UNCA).

#### sats-dual.jsonl

SATSINFO + BESTSAT + RECTIME.

```
satpulsetool gps ... --binary --pvt-out time,off --sats-out sat
# -m - adds: SATSINFOA 1, BESTSATA 1, RECTIMEA 1
```

**Messages:** SATSINFO (UNCB + UNCA), BESTSAT (UNCB + UNCA), RECTIME (UNCB + UNCA).

#### stadop-dual.jsonl

STADOP + BESTNAV + RECTIME. STADOP is not in um980.toml; use ephemeral `-m -`.

```
satpulsetool gps ... --binary --pvt-out pos,qual,time,off
# -m - adds: STADOPA 1, BESTNAVA 1, RECTIMEA 1
```

**Messages:** STADOP (UNCB + UNCA), BESTNAV (UNCB + UNCA), RECTIME (UNCB + UNCA).

#### ppsstatus-dual.jsonl

PPSSTATUS + RECTIME. PPSSTATUS has no gpsprot.Msg conversion but exercises the lib-layer decoder.

```
satpulsetool gps ... --binary --pvt-out time,off
# -m - adds: PPSSTATUSA 1, PPSSTATUSB 1, RECTIMEA 1
```

**Messages:** PPSSTATUS (UNCB + UNCA), RECTIME (UNCB + UNCA).

#### utc-dual.jsonl

All four UTC message types + RECTIME. Note: high-level `leap` doesn't enable BDSUTCB, so it must be added via `-m -`.

```
satpulsetool gps ... --binary --pvt-out time,leap,off
# -m - adds: GPSUTCA 1, GALUTCA 1, BD3UTCA 1, BDSUTCA 1, BDSUTCB 1, RECTIMEA 1
```

**Messages:** GPSUTC, GALUTC, BD3UTC, BDSUTC (UNCB + UNCA), RECTIME (UNCB + UNCA).

### NovAtel format capture

#### novatel-dual.jsonl

NovAtel-format BESTPOS + BESTXYZ + RECTIME. Tests the NovAtel processor path (NOVB/NOVA tags).

```
satpulsetool gps ... --binary --pvt-out time,off
satpulsetool gps ... --vendor unicore \
  -m configs/gpsmsg/um980.toml \
  -t nov-bestposa,nov-bestposb,nov-bestxyza,nov-bestxyzb,unc-rectimea \
  --packet-log novatel-dual.jsonl --capture 30
```

**Messages:** BESTPOS (NOVB + NOVA), BESTXYZ (NOVB + NOVA), RECTIME (UNCB + UNCA).

### PPPNAV capture

#### pppnav-converged-dual.jsonl

Requires SIGNALGROUP 2 + E6-HAS PPP. Done last because SIGNALGROUP change causes a receiver reset and PPP convergence takes 10-30 minutes.

```
satpulsetool gps ... --reload
satpulsetool gps ... --vendor unicore -m configs/gpsmsg/um980.toml -t signalgroup-2
sleep 5
satpulsetool gps ... --vendor unicore -m configs/gpsmsg/um980.toml -t ppp-has
sleep 2
satpulsetool gps ... --binary --pvt-out pos,time,off
# Wait for PPP convergence -- monitor with BESTPOSA to check for SOL_COMPUTED,PPP
# Once converged, capture with -m - (with [default.line]):
#   PPPNAVA 1, PPPNAVB 1, BESTNAVA 1, RECTIMEA 1
```

**Messages:** PPPNAV (UNCB + UNCA), BESTNAV (UNCB + UNCA), RECTIME (UNCB + UNCA).

### Cold-start capture

#### coldstart.jsonl

Exercises incomplete data paths (missing UTC, no fix, sentinel DOP values). Requires NVM save + factory reset -- needs user approval.

```
satpulsetool gps ... --reload
satpulsetool gps ... --binary --pvt-out pos,time,qual,leap,off --sats-out sat --save
satpulsetool gps ... --reset
sleep 5
satpulsetool gps ... --packet-log coldstart.jsonl --capture 120
satpulsetool gps ... --factory-reset
```

**Messages:** Same as daemon set (UNCB), but includes pre-fix epochs with sentinel values.

## Capture sequence

1. **User stops satpulsed** (`sudo systemctl stop satpulsed`)
2. `--show-receiver` to get firmware info for HW.toml
3. rectime.jsonl
4. daemon.jsonl
5. nmea-default.jsonl
6. nmea-rmc.jsonl
7. nmea-sats-uncb.jsonl
8. rectime-dual.jsonl
9. bestnav-dual.jsonl
10. bestnavxyz-dual.jsonl
11. sats-dual.jsonl
12. stadop-dual.jsonl
13. ppsstatus-dual.jsonl
14. utc-dual.jsonl
15. novatel-dual.jsonl
16. coldstart.jsonl (needs user approval for NVM save + factory reset)
17. pppnav-converged-dual.jsonl (SIGNALGROUP 2 + E6-HAS PPP, wait for convergence)
18. Final reload
19. **User restarts satpulsed**

## Coverage matrix

| Message | gpsprot.Msg | Trace(s) |
|---------|-------------|----------|
| RECTIME | TimeMsg | rectime, daemon, rectime-dual, bestnav-dual, bestnavxyz-dual, sats-dual, stadop-dual, ppsstatus-dual, utc-dual, novatel-dual, pppnav-converged-dual, nmea-sats-uncb |
| BESTNAV | PosGeo + VelGeo | daemon, bestnav-dual, stadop-dual, pppnav-converged-dual |
| BESTNAVXYZ | PosECEF + VelECEF | bestnavxyz-dual |
| PPPNAV | PosGeo (high pri) | pppnav-converged-dual |
| SATSINFO | SatellitesMsg | daemon, sats-dual, nmea-sats-uncb |
| BESTSAT | merged w/ SATSINFO | daemon, sats-dual, nmea-sats-uncb |
| STADOP | DOP fields | daemon, stadop-dual |
| PPSSTATUS | native only | ppsstatus-dual |
| GPSUTC | LeapSecondMsg | daemon, utc-dual |
| GALUTC | LeapSecondMsg | daemon, utc-dual |
| BD3UTC | LeapSecondMsg | daemon, utc-dual |
| BDSUTC | LeapSecondMsg | utc-dual |
| BESTPOS (nov) | PosGeo | novatel-dual |
| BESTXYZ (nov) | PosECEF + VelECEF | novatel-dual |
| NMEA RMC | PosGeo + TimeMsg | nmea-default, nmea-rmc, nmea-sats-uncb |
| NMEA GGA | PosGeo | nmea-default |
| NMEA GSA+GSV | SatellitesMsg | nmea-default, nmea-sats-uncb |
| NMEA VTG | VelGeo | nmea-default |
| NMEA GLL | (future) | nmea-default |

Dual ASCII/binary coverage: every message with a gpsprot.Msg conversion appears in at least one `-dual` capture with both UNCA+UNCB (or NOVA+NOVB) tags.

## Messages not captured (and why)

| Message | Reason |
|---------|--------|
| NovAtel TIME (101) | Not in um980.toml; Unicore-native RECTIME covers TimeMsg |
| NovAtel BESTVEL (99) | Not in um980.toml; BESTNAV already carries velocity |
| NovAtel PSRDOP (174) | Not in um980.toml; STADOP is the Unicore equivalent |
| NovAtel IONUTC (6) | Not in um980.toml; covered by GPSUTC/GALUTC/BD3UTC/BDSUTC |
| RANGECMP (140) | Raw observation, no gpsprot.Msg conversion |

## Verify

After all captures:

```
make
python3 .claude/skills/packet-testdata/verify-replay.py \
  --ecef="-1144697.9633,6090335.4599,1504171.3041" \
  out/amd64/satpulsetool \
  gps/testdata/packets/unicore/UM980/
```

Check for:
- Each `.jsonl` contains only expected message types: `jq -r 'select(.out != true) | .msg' <file> | sort | uniq -c | sort -rn`
- Dual captures have both UNCA/UNCB (or NOVA/NOVB): `jq -r 'select(.out != true) | .tag + " " + .msg' <file> | sort | uniq -c`
- `satpulsetool replay` reports no parsing errors (parsing errors are bugs that need fixing)
- Cross-protocol satellite consistency (NMEA vs native) -- NavIC satellites may be in NMEA but not in SATSINFO (known firmware issue, Build17548)
- DOP sentinel values (9999) in coldstart.jsonl are expected

## Adapting for other Unicore receivers

When capturing from a different Unicore receiver (UM981, UM982, UM960):

- Check firmware version -- older builds may lack PPP support or have different signal groups
- Check which constellations and signals are available (varies by model and signal group)
- UM981 is dual-antenna (heading) -- may have additional messages (UNIHEADING)
- Check if the receiver has NovAtel-format messages available (BESTPOS, BESTXYZ)
- PPP availability depends on firmware build number and signal group
- The same message file (um980.toml) should work for all Nebulas IV receivers
