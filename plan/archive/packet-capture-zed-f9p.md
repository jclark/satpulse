# Packet capture plan: u-blox ZED-F9P

Captures packet logs from the ZED-F9P on abondance.lan for use in [packet testing](./packet-testing.md) and [timemsg sync testing](./timemsg-sync-testing.md).

This plan serves as a template for similar captures on other u-blox receivers.

## Hardware

- Device: `/dev/ttyACM0` (USB)
- Receiver: u-blox ZED-F9P, HPG 1.51, protocol 27.50
- NVM state: factory defaults + 38400 baud saved
- USB port means CFG-VALSET MSGOUT keys use port offset 3

## Key principle: reload before every capture

Each capture must start from a known clean state. A `--reload` resets the receiver to NVM settings, ensuring no residual message configuration from a prior capture leaks into the next one.

For 9600 captures: `--reload`, then `--speed 9600` (RAM only), then the capture command. For 38400 captures: `--reload`, then the capture command.

## How high-level config maps to messages on the F9P (HPG, protocol 27.50)

Understanding this is essential for planning captures. The `pvt()` function in `gps/internal/ubx/ubxcfgmsg.go` determines which UBX messages get enabled:

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
- `--binary` disables NMEA output protocol, enables UBX
- `--binary` cannot be combined with `--nmea-out`
- `-m` (message file) cannot be combined with high-level config flags in a single invocation, but a second invocation with `-m` can add messages on top of a prior high-level config (it does not probe or reset)

Messages not reachable via high-level config on HPG:
- NavTimeBDS, NavTimeGLO, NavTimeGal, NavClock (no high-level flag)
- NavPosLLH, NavPosECEF (HPG always uses HP variants)
- NavSVInfo (legacy, replaced by NavSat on protocol >= 15)
- TimSvin (timing receivers, not HPG)
- TimTos (FTS only)

## Design principles

- Every capture includes at least one time message (for timemsg sync testing).
- A capture of a single time message is fine -- testing a lone time message is valuable.
- Don't capture a single non-time message plus only EOE; combine it with something useful.
- Some captures should lack EOE -- testing epoch detection without it matters.

## Capture list

All captures use `-d /dev/ttyACM0 --vendor u-blox --packet-log <file> --capture 30` (60 for survey). Every capture is preceded by `--reload`.

### NMEA captures at 9600

These capture NMEA sentence types for testing NMEA decoding. None have EOE.

#### factory.jsonl

Default NMEA output at 9600.

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --speed 9600
satpulsetool gps -d /dev/ttyACM0 -s 9600 --packet-log factory.jsonl --capture 30
```

**Expected:** GGA, GLL, GSA, GSV, RMC, VTG (confirmed). Time messages: RMC.

#### nmea-rmc-gga.jsonl

RMC + GGA only. For issue #195 (GGA timing to improve RMC/ZDA correlation).

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --speed 9600
satpulsetool gps -d /dev/ttyACM0 -s 9600 --nmea-out RMC,GGA --packet-log nmea-rmc-gga.jsonl --capture 30
```

Time messages: RMC.

#### nmea-rmc-gll.jsonl

RMC + GLL. GLL parsing is not yet implemented but will be. RMC included for a time reference.

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --speed 9600
satpulsetool gps -d /dev/ttyACM0 -s 9600 --nmea-out RMC,GLL --packet-log nmea-rmc-gll.jsonl --capture 30
```

Time messages: RMC.

### UBX captures via high-level config

These use `--binary` to enable UBX output and `--pvt-out ... off` to disable unneeded messages.

#### daemon.jsonl

The base daemon timing set (no pos, no sats). Safe at 9600 baud.

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --speed 9600
satpulsetool gps -d /dev/ttyACM0 -s 9600 --binary --pvt-out daemon --sats-out none --packet-log daemon.jsonl --capture 30
```

daemon = `tp,after,tai,leap,survey,qual,epoch,off`. On HPG F9P: TimTP, NavTimeGPS (after+tai), NavPVT (qual triggers it), NavTimeLS, NavDOP, NavEOE.

**Messages:** TimTP, NavPVT, NavTimeGPS, NavTimeLS, NavDOP, NavEOE.

#### daemon-sats-pos-38400.jsonl

What satpulsed typically enables: daemon + pos + sats. Needs 38400 for bandwidth.

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out daemon,pos --sats-out sat,sig --packet-log daemon-sats-pos-38400.jsonl --capture 30
```

**Messages:** TimTP, NavPVT, NavTimeGPS, NavTimeLS, NavHPPosLLH, NavDOP, NavEOE, NavSat, NavSig.

#### ubx-tp-tai-vel-38400.jsonl

TimTP + NavTimeGPS + NavVelNED. Covers NavVelNED alongside time messages. No EOE.

`tp,after,tai,vel,off`: TimTP (tp), NavTimeGPS (after+tai), NavVelNED (vel alone, nPVT=1, no qual).

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out tp,after,tai,vel,off --packet-log ubx-tp-tai-vel-38400.jsonl --capture 30
```

**Messages:** TimTP, NavTimeGPS, NavVelNED. No EOE.

#### ecef-38400.jsonl

ECEF position with NavTimeUTC. On HPG, `pos,ecef` consumes the ecef flag before vel can use it, so NavVelECEF must be captured separately.

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out pos,ecef,time,epoch,off --packet-log ecef-38400.jsonl --capture 30
```

HPG + pos + ecef => NavHPPosECEF. time alone (nPVT=1, no qual) => NavTimeUTC. epoch => NavEOE.

**Messages:** NavHPPosECEF, NavTimeUTC, NavEOE.

#### ubx-tp-tai-velecef-38400.jsonl

TimTP + NavTimeGPS + NavVelECEF. Covers NavVelECEF alongside time messages. No EOE. Must not include `pos` because on HPG that would consume the ecef flag.

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out tp,after,tai,vel,ecef,off --packet-log ubx-tp-tai-velecef-38400.jsonl --capture 30
```

**Messages:** TimTP, NavTimeGPS, NavVelECEF. No EOE.

#### ubx-tp-tai-38400.jsonl

TimTP + NavTimeGPS only. Tests a minimal time-only UBX capture without EOE.

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out tp,after,tai,off --packet-log ubx-tp-tai-38400.jsonl --capture 30
```

**Messages:** TimTP, NavTimeGPS. No EOE.

#### survey-38400.jsonl

Survey-in with time messages.

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out tp,after,tai,survey,epoch,off --survey --survey-time 60 --survey-acc 50 --packet-log survey-38400.jsonl --capture 60
```

**Messages:** TimTP, NavTimeGPS, NavSvin, NavEOE.

### Per-constellation TimTP captures at 38400

`--time-gnss` changes which GNSS system TimTP references. Each capture uses high-level config for the main setup, then message file tags to add the extra NAV-TIME variants that high-level config cannot enable.

Each capture: reload, high-level config (`--binary --pvt-out tp,after,tai,leap,epoch,off --time-gnss <gnss> --gnss gps,<gnss>`), then message file to add NAV-TIMEGAL/GLO/BDS/UTC.

#### time-gal-38400.jsonl

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out tp,after,tai,leap,epoch,off --time-gnss gal --gnss gps,gal
satpulsetool gps -d /dev/ttyACM0 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml -t ubx-nav-timegal-usb,ubx-nav-timeutc-usb,ubx-nav-timebds-usb,ubx-nav-timeglo-usb \
  --packet-log time-gal-38400.jsonl --capture 30
```

**Messages:** TimTP (ref Galileo), NavTimeGPS, NavTimeGAL, NavTimeUTC, NavTimeBDS, NavTimeGLO, NavTimeLS, NavEOE.

#### time-bds-38400.jsonl

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out tp,after,tai,leap,epoch,off --time-gnss bds --gnss gps,bds
satpulsetool gps -d /dev/ttyACM0 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml -t ubx-nav-timegal-usb,ubx-nav-timeutc-usb,ubx-nav-timebds-usb,ubx-nav-timeglo-usb \
  --packet-log time-bds-38400.jsonl --capture 30
```

**Messages:** TimTP (ref BeiDou), NavTimeGPS, NavTimeGAL, NavTimeUTC, NavTimeBDS, NavTimeGLO, NavTimeLS, NavEOE.

#### time-glo-38400.jsonl

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out tp,after,tai,leap,epoch,off --time-gnss glo --gnss gps,glo
satpulsetool gps -d /dev/ttyACM0 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml -t ubx-nav-timegal-usb,ubx-nav-timeutc-usb,ubx-nav-timebds-usb,ubx-nav-timeglo-usb \
  --packet-log time-glo-38400.jsonl --capture 30
```

**Messages:** TimTP (ref GLONASS), NavTimeGPS, NavTimeGAL, NavTimeUTC, NavTimeBDS, NavTimeGLO, NavTimeLS, NavEOE.

#### time-gps-38400.jsonl

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out tp,after,tai,leap,epoch,off --time-gnss gps --gnss gps
satpulsetool gps -d /dev/ttyACM0 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml -t ubx-nav-timegal-usb,ubx-nav-timeutc-usb,ubx-nav-timebds-usb,ubx-nav-timeglo-usb \
  --packet-log time-gps-38400.jsonl --capture 30
```

**Messages:** TimTP (ref GPS), NavTimeGPS, NavTimeGAL, NavTimeUTC, NavTimeBDS, NavTimeGLO, NavTimeLS, NavEOE.

### UBX captures via message file tags

These enable messages that high-level config cannot reach on HPG. Tags are defined in `configs/gpsmsg/ubx.toml` and composed with `-t`. Each tag sends a CFG-VALSET to RAM enabling one MSGOUT key on USB.

After `--reload`, the receiver outputs default NMEA + UBX protocols are both enabled on USB. The message file tags add UBX messages on top.

#### ubx-nav-time-all-38400.jsonl

All five NAV-TIME variants + NAV-CLOCK + NAV-EOE. Enables them all together so event output can be compared across time systems. Default NMEA also present.

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml \
  -t ubx-nav-timegps-usb,ubx-nav-timeutc-usb,ubx-nav-timebds-usb,ubx-nav-timeglo-usb,ubx-nav-timegal-usb,ubx-nav-clock-usb,ubx-nav-eoe-usb \
  --packet-log ubx-nav-time-all-38400.jsonl --capture 30
```

**Messages:** NavTimeGPS, NavTimeUTC, NavTimeBDS, NavTimeGLO, NavTimeGal, NavClock, NavEOE + default NMEA.

#### nmea-ubx-eoe-38400.jsonl

Default NMEA + UBX NAV-EOE only. Tests cross-protocol epoch detection (NavEpochManager receiving NMEA sentences with UBX EOE marker).

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml -t ubx-nav-eoe-usb \
  --packet-log nmea-ubx-eoe-38400.jsonl --capture 30
```

**Messages:** Default NMEA (GGA, GLL, GSA, GSV, RMC, VTG) + UBX NavEOE. Time messages: RMC.

#### ubx-nav-pos-38400.jsonl

NavPosLLH + NavPosECEF + NAV-TIMEGPS + NAV-EOE. Non-HP position variants have distinct decode paths from HP variants. NAV-TIMEGPS included as time message. Default NMEA also present.

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --vendor u-blox \
  -m configs/gpsmsg/ubx9.toml \
  -t ubx-nav-posllh-usb,ubx-nav-posecef-usb,ubx-nav-timegps-usb,ubx-nav-eoe-usb \
  --packet-log ubx-nav-pos-38400.jsonl --capture 30
```

**Messages:** NavPosLLH, NavPosECEF, NavTimeGPS, NavEOE + default NMEA.

## Capture sequence

Every capture starts with `--reload`. Ordered to minimize speed changes.

1. **User stops satpulsed** (needs sudo)
2. factory.jsonl (reload, speed 9600, capture)
3. nmea-rmc-gga.jsonl (reload, speed 9600, capture)
4. nmea-rmc-gll.jsonl (reload, speed 9600, capture)
5. daemon.jsonl (reload, speed 9600, capture)
6. daemon-sats-pos-38400.jsonl (reload, capture at 38400)
7. ubx-tp-tai-vel-38400.jsonl (reload, capture at 38400)
8. ecef-38400.jsonl (reload, capture at 38400)
9. ubx-tp-tai-velecef-38400.jsonl (reload, capture at 38400)
10. ubx-tp-tai-38400.jsonl (reload, capture at 38400)
11. time-gal-38400.jsonl (reload, high-level + message file, capture at 38400)
12. time-bds-38400.jsonl (reload, high-level + message file, capture at 38400)
13. time-glo-38400.jsonl (reload, high-level + message file, capture at 38400)
14. time-gps-38400.jsonl (reload, high-level + message file, capture at 38400)
15. ubx-nav-time-all-38400.jsonl (reload, message file, capture at 38400)
16. nmea-ubx-eoe-38400.jsonl (reload, message file, capture at 38400)
17. ubx-nav-pos-38400.jsonl (reload, message file, capture at 38400)
18. survey-38400.jsonl (reload, capture at 38400, 60s)
19. **User restarts satpulsed**

## Messages not captured (and why)

| Message | Reason |
|---------|--------|
| NavSol | Legacy, not on F9P (protocol >= 14) |
| NavSVInfo | Legacy, F9P uses NavSat (protocol >= 15) |
| TimSvin | Timing (TIM/FTS) receivers, not HPG; F9P uses NavSvin |
| TimTos | FTS product category only, F9P is HPG |
| NavTimeTrusted | No gpsprot.Msg conversion |
| SEC-OSNMA | Not useful at present |
| MGA | Outgoing (assistance data), not incoming |

## Coverage matrix

| ubxbin message | gpsprot.Msg | Trace(s) |
|----------------|-------------|----------|
| NavPVT | PosGeo+VelGeo+TimeMsg | daemon, daemon-sats-pos-38400 |
| NavHPPosLLH | PosGeo | daemon-sats-pos-38400 |
| NavHPPosECEF | PosECEF | ecef-38400 |
| NavPosLLH | PosGeoMsg | ubx-nav-pos-38400 |
| NavPosECEF | PosECEFMsg | ubx-nav-pos-38400 |
| NavVelNED | VelGeo | ubx-tp-tai-vel-38400 |
| NavVelECEF | VelECEF | ubx-tp-tai-velecef-38400 |
| NavTimeGPS | TimeMsg | daemon, daemon-sats-pos-38400, ubx-tp-tai-vel-38400, ubx-tp-tai-38400, survey-38400, ubx-nav-time-all-38400, ubx-nav-pos-38400, time-gal-38400, time-bds-38400, time-glo-38400, time-gps-38400 |
| NavTimeUTC | TimeMsg | ecef-38400, ubx-nav-time-all-38400, time-gal-38400, time-bds-38400, time-glo-38400, time-gps-38400 |
| NavTimeBDS | TimeMsg | ubx-nav-time-all-38400, time-gal-38400, time-bds-38400, time-glo-38400, time-gps-38400 |
| NavTimeGLO | TimeMsg | ubx-nav-time-all-38400, time-gal-38400, time-bds-38400, time-glo-38400, time-gps-38400 |
| NavTimeGal | TimeMsg | ubx-nav-time-all-38400, time-gal-38400, time-bds-38400, time-glo-38400, time-gps-38400 |
| NavTimeLS | LeapSecondMsg | daemon, daemon-sats-pos-38400, time-gal-38400, time-bds-38400, time-glo-38400, time-gps-38400 |
| NavDOP | NavEpochMsg | daemon, daemon-sats-pos-38400 |
| NavEOE | NavEpochMsg | daemon, daemon-sats-pos-38400, ecef-38400, survey-38400, ubx-nav-time-all-38400, nmea-ubx-eoe-38400, ubx-nav-pos-38400, time-gal-38400, time-bds-38400, time-glo-38400, time-gps-38400 |
| NavSat | SatellitesMsg | daemon-sats-pos-38400 |
| NavSig | SatellitesMsg | daemon-sats-pos-38400 |
| NavSvin | SurveyMsg | survey-38400 |
| NavClock | (no Msg) | ubx-nav-time-all-38400 |
| TimTP | TimeMsg (PrePulse) | daemon, daemon-sats-pos-38400, ubx-tp-tai-vel-38400, ubx-tp-tai-38400, survey-38400, time-gal-38400, time-bds-38400, time-glo-38400, time-gps-38400 |
| NMEA RMC | PosGeo+TimeMsg+VelGeo | factory, nmea-rmc-gga, nmea-rmc-gll |
| NMEA GGA | PosGeo | factory, nmea-rmc-gga |
| NMEA VTG | VelGeo | factory |
| NMEA GSA+GSV | SatellitesMsg | factory |
| NMEA GLL | (future) | factory, nmea-rmc-gll |

Captures without EOE: factory, nmea-rmc-gga, nmea-rmc-gll, ubx-tp-tai-vel-38400, ubx-tp-tai-velecef-38400, ubx-tp-tai-38400.

## Verify

- Each `.jsonl` contains only the expected message types (no residual messages from prior config): use `jq -r 'select(.out != true) | .msg' <file> | sort | uniq -c | sort -rn` to check
- `satpulsetool decode` parses every packet in each log
- Every `ubxbin` message that produces a `gpsprot.Msg` appears in at least one trace
- Post-pulse TimeMsg present in multiple traces (for timemsg-sync-testing)
- At least one capture exercises each of: with EOE, without EOE, NMEA only, UBX only

After collecting captures, replay them with `out/amd64/satpulsetool replay <file>` and examine the event output for anomalies:

- Compare time message events across the per-constellation captures (time-gps, time-gal, time-bds, time-glo). TAI times should be consistent (differing only by capture time offset). The gnss field should reflect the configured constellation for each message type.
- Check that TimTP events show the correct gnss for each `--time-gnss` setting and that TAI times are plausible.
- Check that NAV-TIMEUTC events have a non-null gnss field for all constellations.
- Verify that taiTime values from different NAV-TIME variants within the same epoch agree to within a few nanoseconds.

## Files to create/update

- `gps/testdata/packets/u-blox/ZED-F9P/HW.toml` (already created)
- `configs/gpsmsg/ubx.toml` (updated with message enable/disable tags)
- All `.jsonl` files captured from live receiver

## Adapting for other receivers

When capturing from a different u-blox receiver:

- Check product category (HPG, FTS, standard) -- this changes which messages high-level config enables
- Check protocol version -- older protocols may not support NavSat, NavSig, NavEOE, NavTimeLS
- Check connection type (USB vs UART) -- changes MSGOUT key port offset (USB=3, UART1=1)
- FTS receivers: TimTos replaces TimTP; NavTimeGPS is not used for tai
- Non-HPG receivers: NavPosLLH/NavPosECEF are used directly (no HP variants)
- Older receivers (protocol < 27): no NavSig
- Older receivers (protocol < 18): no NavEOE, NavTimeLS
