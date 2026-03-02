# Tag naming conventions

Tags group related messages and are selected with the `-t` flag when running `satpulsetool gps`.

## General rules

- Tags use lowercase with hyphens as separators
- Enable/disable pairs use `-off` suffix: `nmea-rmc` / `nmea-rmc-off`
- Query commands use `get-` prefix: `get-pps`, `get-gnss`
- Parametric commands include the value: `min-elev-15`, `speed-115200`

## Standard tags

## Version query

| Tag | Description |
|-----|-------------|
| `get-version` | Query firmware and hardware version |

## NMEA message control

Individual tags for each NMEA sentence type:

| Tag | Description |
|-----|-------------|
| `nmea-gga` | Enable GGA (position, quality, satellites) |
| `nmea-gga-off` | Disable GGA |
| `nmea-gll` | Enable GLL (geographic position - latitude/longitude) |
| `nmea-gll-off` | Disable GLL |
| `nmea-gsa` | Enable GSA (active satellites and DOP) |
| `nmea-gsa-off` | Disable GSA |
| `nmea-gsv` | Enable GSV (satellites in view) |
| `nmea-gsv-off` | Disable GSV |
| `nmea-rmc` | Enable RMC (time, position, velocity) |
| `nmea-rmc-off` | Disable RMC |
| `nmea-zda` | Enable ZDA (time and date) |
| `nmea-zda-off` | Disable ZDA |
| `nmea-daemon` | Enable RMC, GGA, GSV, GSA (messages used by satpulse daemon) |

## Binary message control

Protocol-prefixed tags for proprietary binary messages:

| Tag Pattern | Examples |
|-------------|----------|
| `asbin-*` | `asbin-nav-time`, `asbin-nav-timeutc`, `asbin-nav-svinfo`, `asbin-nav-svin` |
| `ubx-*` | `ubx-nav-pvt`, `ubx-tim-tp` |
| `casbin-*` | `casbin-nav2-sol`, `casbin-nav2-timeutc` |
| `pqtm-*` | `pqtm-epe`, `pqtm-vel`, `pqtm-pvt` |

Each gets an `-off` variant for disabling.

## NMEA version

| Tag | Description |
|-----|-------------|
| `get-nmea-ver` | Query current NMEA version |
| `nmea-ver-3` | Set NMEA version 3.01 |
| `nmea-ver-400` | Set NMEA version 4.00 |
| `nmea-ver-410` | Set NMEA version 4.10 (adds signal ID to GSV) |
| `nmea-ver-411` | Set NMEA version 4.11 (changes talker IDs: GB for BeiDou, GQ for QZSS) |

## Elevation mask

| Tag | Description |
|-----|-------------|
| `get-min-elev` | Query current elevation mask |
| `min-elev-0` | Set minimum elevation to 0 degrees |
| `min-elev-5` | Set minimum elevation to 5 degrees |
| `min-elev-10` | Set minimum elevation to 10 degrees |
| `min-elev-15` | Set minimum elevation to 15 degrees |
| `min-elev-20` | Set minimum elevation to 20 degrees |
| `min-elev-25` | Set minimum elevation to 25 degrees |
| `min-elev-30` | Set minimum elevation to 30 degrees |
| `min-elev-35` | Set minimum elevation to 35 degrees |
| `min-elev-40` | Set minimum elevation to 40 degrees |
| `min-elev-45` | Set minimum elevation to 45 degrees |

## Constellation selection

| Tag | Description |
|-----|-------------|
| `get-gnss` | Query current constellation settings |
| `gnss-gps` | Enable GPS only (all available bands) |
| `gnss-gal` | Enable Galileo only (all available bands) |
| `gnss-glo` | Enable GLONASS only (all available bands) |
| `gnss-bds` | Enable BeiDou only (all available bands) |
| `gnss-gps-gal` | Enable GPS and Galileo (all available bands) |
| `gnss-all` | Enable all constellations (all available bands) |

## PPS configuration

| Tag | Description |
|-----|-------------|
| `get-pps` | Query current PPS configuration |
| `pps` | Enable PPS with 0.1s pulse width, only when locked |
| `pps-off` | Disable PPS output |

## Fix rate

| Tag | Description |
|-----|-------------|
| `get-fix-rate` | Query current fix interval |
| `fix-rate-1` | Set fix rate to 1 Hz |
| `fix-rate-2` | Set fix rate to 2 Hz |
| `fix-rate-5` | Set fix rate to 5 Hz |
| `fix-rate-10` | Set fix rate to 10 Hz |
| `fix-rate-20` | Set fix rate to 20 Hz |

Fix rate is the number of navigation solutions computed per second.
Message output rate is sometimes specified as a multiple of this:
one message every N fixes. To get 1 Hz messages from a 10 Hz fix rate,
set the message rate to 10.

## Port configuration

| Tag | Description |
|-----|-------------|
| `get-uart` | Query UART configuration |
| `speed-9600` | Set baud rate to 9600 |
| `speed-19200` | Set baud rate to 19200 |
| `speed-38400` | Set baud rate to 38400 |
| `speed-57600` | Set baud rate to 57600 |
| `speed-115200` | Set baud rate to 115200 |
| `speed-230400` | Set baud rate to 230400 |
| `speed-460800` | Set baud rate to 460800 |
| `speed-921600` | Set baud rate to 921600 |

## Restart commands

| Tag | Description |
|-----|-------------|
| `hot-start` | Keep ephemeris data (fastest restart) |
| `warm-start` | Clear ephemeris, keep almanac |
| `cold-start` | Clear all satellite data |

## Configuration management

| Tag | Description |
|-----|-------------|
| `save` | Save configuration to NVM |
| `reload` | Reload configuration from NVM |
| `reset` | Reload from NVM AND clear satellite data |
| `factory-reset` | Restore factory defaults and reboot |

## Survey-in (base station)

| Tag | Description |
|-----|-------------|
| `get-survey` | Query current survey configuration |
| `survey` | Start survey-in (default: 2000s, 20m accuracy) |
| `survey-off` | Stop survey / return to mobile mode |
| `mobile` | Alias for survey-off (some receivers) |

## Fixed position (base station)

| Tag | Description |
|-----|-------------|
| `get-fixed-pos` | Query current fixed ECEF position |
| `fixed-pos-example` | Set fixed ECEF position (example coordinates - replace with yours) |
| `fixed-pos-off` | Clear fixed position |

## RTCM output (base station)

| Tag | Description |
|-----|-------------|
| `rtcm-arp` | Enable ARP message (1005) |
| `rtcm-msm4` | Enable MSM4 for all constellations (1074/1084/1094/1124) |
| `rtcm-msm7` | Enable MSM7 for all constellations (1077/1087/1097/1127) |
| `rtcm-off` | Disable all RTCM messages |

## PPP (Precise Point Positioning)

| Tag | Description |
|-----|-------------|
| `ppp-has` | Enable PPP with Galileo HAS source |
| `ppp-b2b` | Enable PPP with BeiDou B2b source |
| `ppp-has-b2b` | Enable PPP with fused HAS+B2b source |
| `ppp-off` | Disable PPP |

## RTK mode (if applicable)

| Tag | Description |
|-----|-------------|
| `mode-base` | Set base station mode |
| `mode-rover` | Set rover mode |

