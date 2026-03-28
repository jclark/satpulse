# Packet capture plan: u-blox ZED-X20P

Captures packet logs from the ZED-X20P on abondance.lan for use in [packet testing](../packet-testing.md) and [timemsg sync testing](../timemsg-sync-testing.md).

Based on the [ZED-F9P capture plan](./packet-capture-zed-f9p.md). The X20P is also HPG, so the same message mapping applies.

## Hardware

- Device: `/dev/ttyACM1` (USB)
- Receiver: u-blox ZED-X20P, HPG 2.02, protocol 50.10
- NVM state: factory defaults + 38400 baud saved
- USB port means CFG-VALSET MSGOUT keys use port offset 3
- Supported GNSS: GPS, GAL, BDS, QZSS, NAVIC, SBAS (no GLONASS)

## Differences from ZED-F9P

- Protocol 50.10 (vs 27.50) -- all modern messages available
- No GLONASS support -- skip time-glo capture, skip NavTimeGLO from per-constellation captures
- Default baud is 38400 -- factory capture at 38400, no baud rate suffix needed for 38400 captures
- Supports QZSS and NAVIC (not covered in this plan, to be discussed separately)

## Key principle: reload before every capture

Each capture must start from a known clean state. A `--reload` resets the receiver to NVM settings, ensuring no residual message configuration from a prior capture leaks into the next one.

After `--reload` on USB, add `sleep 2` as the device may briefly disconnect.

## How high-level config maps to messages on the X20P (HPG, protocol 50.10)

Same as F9P (see ZED-F9P plan for full details):

- `pos` on HPG => NavHPPosLLH (not NavPosLLH)
- `pos,ecef` on HPG => NavHPPosECEF (consumes ecef flag)
- `vel` => NavVelNED
- `vel,ecef` => NavVelECEF (but on HPG, `pos,vel,ecef` gives NavHPPosECEF + NavVelNED because pos consumes ecef first)
- `time` (without `tai`) => NavTimeUTC
- `time,tai` => NavTimeGPS
- `tp` => TimTP; `tp,after` adds `time` flag
- `qual` => NavDOP (also triggers NavPVT if combined with any of pos/vel/time)
- `epoch` => NavEOE
- `leap` => NavTimeLS
- `survey` + `--survey` => NavSvin
- When >= 2 of remaining {pos, vel, time} are set, or `qual` is set: NavPVT is enabled and subsumes them
- `off` turns off messages that would otherwise be left at their current rate

Messages not reachable via high-level config on HPG:
- NavTimeBDS, NavTimeGal, NavClock (no high-level flag)
- NavPosLLH, NavPosECEF (HPG always uses HP variants)
- NavTimeGLO not applicable (no GLONASS support)

## Design principles

- Every capture includes at least one time message (for timemsg sync testing).
- A capture of a single time message is fine -- testing a lone time message is valuable.
- Don't capture a single non-time message plus only EOE; combine it with something useful.
- Some captures should lack EOE -- testing epoch detection without it matters.

## Capture list

All captures use `-d /dev/ttyACM1 --vendor u-blox --packet-log <file> --capture 30` (60 for survey). Every capture is preceded by `--reload` + `sleep 2`.

### NMEA captures

These capture NMEA sentence types for testing NMEA decoding. None have EOE.

#### factory.jsonl

Default NMEA output at 38400 (default baud).

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --packet-log factory.jsonl --capture 30
```

**Expected:** GGA, GLL, GSA, GSV, RMC, VTG. Time messages: RMC.

#### nmea-rmc-gga.jsonl

RMC + GGA only.

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --nmea-out RMC,GGA --packet-log nmea-rmc-gga.jsonl --capture 30
```

Time messages: RMC.

#### nmea-rmc-gll.jsonl

RMC + GLL. GLL parsing is not yet implemented but will be. RMC included for a time reference.

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --nmea-out RMC,GLL --packet-log nmea-rmc-gll.jsonl --capture 30
```

Time messages: RMC.

### UBX captures via high-level config

These use `--binary` to enable UBX output and `--pvt-out ... off` to disable unneeded messages.

#### daemon.jsonl

The base daemon timing set (no pos, no sats).

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --binary --pvt-out daemon --sats-out none --packet-log daemon.jsonl --capture 30
```

daemon = `tp,after,tai,leap,survey,qual,epoch,off`. On HPG X20P: TimTP, NavTimeGPS (after+tai), NavPVT (qual triggers it), NavTimeLS, NavDOP, NavEOE.

**Messages:** TimTP, NavPVT, NavTimeGPS, NavTimeLS, NavDOP, NavEOE.

#### daemon-sats-pos.jsonl

What satpulsed typically enables: daemon + pos + sats. At 38400 (default baud, no suffix).

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --binary --pvt-out daemon,pos --sats-out sat,sig --packet-log daemon-sats-pos.jsonl --capture 30
```

**Messages:** TimTP, NavPVT, NavTimeGPS, NavTimeLS, NavHPPosLLH, NavDOP, NavEOE, NavSat, NavSig.

#### ubx-tp-tai-vel.jsonl

TimTP + NavTimeGPS + NavVelNED. Covers NavVelNED alongside time messages. No EOE.

`tp,after,tai,vel,off`: TimTP (tp), NavTimeGPS (after+tai), NavVelNED (vel alone, nPVT=1, no qual).

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --binary --pvt-out tp,after,tai,vel,off --packet-log ubx-tp-tai-vel.jsonl --capture 30
```

**Messages:** TimTP, NavTimeGPS, NavVelNED. No EOE.

#### ecef.jsonl

ECEF position with NavTimeUTC. On HPG, `pos,ecef` consumes the ecef flag before vel can use it.

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --binary --pvt-out pos,ecef,time,epoch,off --packet-log ecef.jsonl --capture 30
```

HPG + pos + ecef => NavHPPosECEF. time alone (nPVT=1, no qual) => NavTimeUTC. epoch => NavEOE.

**Messages:** NavHPPosECEF, NavTimeUTC, NavEOE.

#### ubx-tp-tai-velecef.jsonl

TimTP + NavTimeGPS + NavVelECEF. Covers NavVelECEF alongside time messages. No EOE. Must not include `pos` because on HPG that would consume the ecef flag.

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --binary --pvt-out tp,after,tai,vel,ecef,off --packet-log ubx-tp-tai-velecef.jsonl --capture 30
```

**Messages:** TimTP, NavTimeGPS, NavVelECEF. No EOE.

#### ubx-tp-tai.jsonl

TimTP + NavTimeGPS only. Tests a minimal time-only UBX capture without EOE.

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --binary --pvt-out tp,after,tai,off --packet-log ubx-tp-tai.jsonl --capture 30
```

**Messages:** TimTP, NavTimeGPS. No EOE.

#### survey.jsonl

Survey-in with time messages.

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --binary --pvt-out tp,after,tai,survey,epoch,off --survey --survey-time 60 --survey-acc 50 --packet-log survey.jsonl --capture 60
```

**Messages:** TimTP, NavTimeGPS, NavSvin, NavEOE.

### Per-constellation TimTP captures

`--time-gnss` changes which GNSS system TimTP references. Each capture uses high-level config for the main setup, then message file tags to add extra NAV-TIME variants.

No GLONASS capture -- receiver does not support GLONASS.

#### time-gps.jsonl

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --binary --pvt-out tp,after,tai,leap,epoch,off --time-gnss gps --gnss gps
satpulsetool gps -d /dev/ttyACM1 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml -t ubx-nav-timegal-usb,ubx-nav-timeutc-usb,ubx-nav-timebds-usb \
  --packet-log time-gps.jsonl --capture 30
```

**Messages:** TimTP (ref GPS), NavTimeGPS, NavTimeGAL, NavTimeUTC, NavTimeBDS, NavTimeLS, NavEOE.

#### time-gal.jsonl

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --binary --pvt-out tp,after,tai,leap,epoch,off --time-gnss gal --gnss gps,gal
satpulsetool gps -d /dev/ttyACM1 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml -t ubx-nav-timegal-usb,ubx-nav-timeutc-usb,ubx-nav-timebds-usb \
  --packet-log time-gal.jsonl --capture 30
```

**Messages:** TimTP (ref Galileo), NavTimeGPS, NavTimeGAL, NavTimeUTC, NavTimeBDS, NavTimeLS, NavEOE.

#### time-bds.jsonl

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --binary --pvt-out tp,after,tai,leap,epoch,off --time-gnss bds --gnss gps,bds
satpulsetool gps -d /dev/ttyACM1 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml -t ubx-nav-timegal-usb,ubx-nav-timeutc-usb,ubx-nav-timebds-usb \
  --packet-log time-bds.jsonl --capture 30
```

**Messages:** TimTP (ref BeiDou), NavTimeGPS, NavTimeGAL, NavTimeUTC, NavTimeBDS, NavTimeLS, NavEOE.

### UBX captures via message file tags

These enable messages that high-level config cannot reach on HPG.

#### ubx-nav-time-all.jsonl

All NAV-TIME variants (except GLO) + NAV-CLOCK + NAV-EOE. Default NMEA also present.

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml \
  -t ubx-nav-timegps-usb,ubx-nav-timeutc-usb,ubx-nav-timebds-usb,ubx-nav-timegal-usb,ubx-nav-clock-usb,ubx-nav-eoe-usb \
  --packet-log ubx-nav-time-all.jsonl --capture 30
```

**Messages:** NavTimeGPS, NavTimeUTC, NavTimeBDS, NavTimeGal, NavClock, NavEOE + default NMEA.

#### nmea-ubx-eoe.jsonl

Default NMEA + UBX NAV-EOE only. Tests cross-protocol epoch detection.

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml -t ubx-nav-eoe-usb \
  --packet-log nmea-ubx-eoe.jsonl --capture 30
```

**Messages:** Default NMEA (GGA, GLL, GSA, GSV, RMC, VTG) + UBX NavEOE. Time messages: RMC.

#### ubx-nav-pos.jsonl

NavPosLLH + NavPosECEF + NAV-TIMEGPS + NAV-EOE. Non-HP position variants with distinct decode paths. Default NMEA also present.

```
satpulsetool gps -d /dev/ttyACM1 -s 38400 --reload && sleep 2
satpulsetool gps -d /dev/ttyACM1 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml \
  -t ubx-nav-posllh-usb,ubx-nav-posecef-usb,ubx-nav-timegps-usb,ubx-nav-eoe-usb \
  --packet-log ubx-nav-pos.jsonl --capture 30
```

**Messages:** NavPosLLH, NavPosECEF, NavTimeGPS, NavEOE + default NMEA.

## Capture sequence

Every capture starts with `--reload` + `sleep 2`. All at 38400 (default baud).

1. factory.jsonl
2. nmea-rmc-gga.jsonl
3. nmea-rmc-gll.jsonl
4. daemon.jsonl
5. daemon-sats-pos.jsonl
6. ubx-tp-tai-vel.jsonl
7. ecef.jsonl
8. ubx-tp-tai-velecef.jsonl
9. ubx-tp-tai.jsonl
10. time-gps.jsonl
11. time-gal.jsonl
12. time-bds.jsonl
13. ubx-nav-time-all.jsonl
14. nmea-ubx-eoe.jsonl
15. ubx-nav-pos.jsonl
16. survey.jsonl
17. **Reload receiver, user restarts satpulsed**

## Messages not captured (and why)

| Message | Reason |
|---------|--------|
| NavSol | Legacy, not on X20P |
| NavSVInfo | Legacy, X20P uses NavSat (protocol >= 15) |
| NavTimeGLO | No GLONASS support on X20P |
| TimSvin | Timing (TIM/FTS) receivers, not HPG; X20P uses NavSvin |
| TimTos | FTS product category only, X20P is HPG |
| NavTimeTrusted | No gpsprot.Msg conversion |
| SEC-OSNMA | Not useful at present |
| MGA | Outgoing (assistance data), not incoming |

## Coverage matrix

| ubxbin message | gpsprot.Msg | Trace(s) |
|----------------|-------------|----------|
| NavPVT | PosGeo+VelGeo+TimeMsg | daemon, daemon-sats-pos |
| NavHPPosLLH | PosGeo | daemon-sats-pos |
| NavHPPosECEF | PosECEF | ecef |
| NavPosLLH | PosGeoMsg | ubx-nav-pos |
| NavPosECEF | PosECEFMsg | ubx-nav-pos |
| NavVelNED | VelGeo | ubx-tp-tai-vel |
| NavVelECEF | VelECEF | ubx-tp-tai-velecef |
| NavTimeGPS | TimeMsg | daemon, daemon-sats-pos, ubx-tp-tai-vel, ubx-tp-tai, survey, ubx-nav-time-all, ubx-nav-pos, time-gps, time-gal, time-bds |
| NavTimeUTC | TimeMsg | ecef, ubx-nav-time-all, time-gps, time-gal, time-bds |
| NavTimeBDS | TimeMsg | ubx-nav-time-all, time-gps, time-gal, time-bds |
| NavTimeGal | TimeMsg | ubx-nav-time-all, time-gps, time-gal, time-bds |
| NavTimeLS | LeapSecondMsg | daemon, daemon-sats-pos, time-gps, time-gal, time-bds |
| NavDOP | NavEpochMsg | daemon, daemon-sats-pos |
| NavEOE | NavEpochMsg | daemon, daemon-sats-pos, ecef, survey, ubx-nav-time-all, nmea-ubx-eoe, ubx-nav-pos, time-gps, time-gal, time-bds |
| NavSat | SatellitesMsg | daemon-sats-pos |
| NavSig | SatellitesMsg | daemon-sats-pos |
| NavSvin | SurveyMsg | survey |
| NavClock | (no Msg) | ubx-nav-time-all |
| TimTP | TimeMsg (PrePulse) | daemon, daemon-sats-pos, ubx-tp-tai-vel, ubx-tp-tai, survey, time-gps, time-gal, time-bds |
| NMEA RMC | PosGeo+TimeMsg+VelGeo | factory, nmea-rmc-gga, nmea-rmc-gll |
| NMEA GGA | PosGeo | factory, nmea-rmc-gga |
| NMEA VTG | VelGeo | factory |
| NMEA GSA+GSV | SatellitesMsg | factory |
| NMEA GLL | (future) | factory, nmea-rmc-gll |

Captures without EOE: factory, nmea-rmc-gga, nmea-rmc-gll, ubx-tp-tai-vel, ubx-tp-tai-velecef, ubx-tp-tai.

## Verify

- Each `.jsonl` contains only the expected message types: `jq -r 'select(.out != true) | .msg' <file> | sort | uniq -c | sort -rn`
- `satpulsetool decode` parses every packet in each log
- Every `ubxbin` message that produces a `gpsprot.Msg` appears in at least one trace
- Post-pulse TimeMsg present in multiple traces
- At least one capture exercises each of: with EOE, without EOE, NMEA only, UBX only

After collecting captures, replay them with `out/amd64/satpulsetool replay <file>` and examine the event output for anomalies:

- Compare time message events across the per-constellation captures (time-gps, time-gal, time-bds). TAI times should be consistent.
- Check that TimTP events show the correct gnss for each `--time-gnss` setting.
- Verify that taiTime values from different NAV-TIME variants within the same epoch agree to within a few nanoseconds.
