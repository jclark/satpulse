# SinoGNSS specific capture details

Read `novatel-config.md` first. This file covers what is specific to SinoGNSS (ComNav) K8/K9 receivers. The message file is `configs/gpsmsg/sinognss/sinognss.toml`; the K901 set is in `gps/testdata/packets/sinognss/K901/`, and its HW.toml records the receiver behaviour seen.

## Command replies

Replies are `OK!` followed by `Command accepted! Port: COM1.`, or `Error! Invalid key word!` followed by the command echoed in upper case. They are not tagged packets and there is no response pattern, so check the output of each configuring `gps -m` session for `Error!`. A bad argument gets the same error as an unknown command.

## Identification

`log versiona once` (the `get-version` tag) gives model, PSN, hardware version, software version, boot version and compile date. Use the software version as the HW.toml `firmware` (for example `650ES-00000-1`); the boot version (`8.1.8`) is also printed in the boot banner after a reset.

## One format per log

A port holds one format per log: requesting TIMEA after TIMEB leaves only TIMEA. So there are no `-dual` captures; see `novatel-config.md`. This also affects captures that need a time message alongside an abbreviated log: `log time` replaces `log timeb`, so use an NMEA sentence such as GPRMC as the time message instead.

## Resets and the cold start

- `reload` (RESET) restarts with the saved configuration and keeps the almanac and ephemeris. Use it between captures, followed by a wait of about 12 s for a fix.
- There is no cold start. FRESET, the only command that clears satellite data, is a factory reset: it clears the saved configuration too.
- The factory configuration outputs nothing on COM1, so there is no factory capture. Output seen after a RESET is the saved configuration, not the factory output.

For the cold-start capture, send FRESET and the log requests in one session and capture from the reset, since the receiver outputs nothing until it is configured:

```
[default.line]
eol = "\r\n"
[[line]]
text = "FRESET"
delay = 3
[[line]]
text = "log timeb ontime 1"
...
```

```
satpulsetool gps -d <device> -s <baud> -m coldstart.toml --packet-log coldstart.jsonl --capture 120
```

FRESET destroys the saved configuration, so ask the user first, and restore and save the configuration they want afterwards (for example `nmea-ver-411,nmea-rmc,nmea-gsv,nmea-gsa,pps,save`).

## NMEA version

`SET NMEAV41 ON` (the `nmea-ver-411` tag) gives NMEA 4.11: GB and GQ talkers, GSA system ID, GSV signal ID and RMC navigational status. The factory default, `SET NMEAV41 OFF` (`nmea-ver-400`), gives the older style without those fields, with BD talkers and SinoGNSS's extended satellite numbers. The NMEA style comes from the saved configuration, so check which one the receiver is in and capture both (`nmea.jsonl` and `nmea-400.jsonl`).

## Base mode and survey

- RTCM 1005 is output only in base mode. Set it with `FIX position <lat> <lon> <hgt>`, where the height is above the geoid (ellipsoidal height minus the undulation that BESTPOS reports), using the antenna position from CLAUDE.local.md. RTCM 1006, 1007, 1008, 1033 and 1230 are output in rover mode too.
- For a survey capture, `POSAVE ON 0.01` finishes in about 35 s, after which BESTPOS reports FIXEDPOS; capture for 90 s. The message file's `survey` tag uses 0.56 hours.

A RESET leaves base mode.

## Logs

- GPSEPHEM, GALEPHEM, IRNEPHEM and BD2EPHEM all have message ID 71, so put at most one of them in a capture.
- SBASRAWFRAME and HASMESS reject `ONCHANGED`, and `ONCE` gives continuing output.
- Abbreviated ASCII TIME and PSRDOP have only the header line; other abbreviated logs have body lines. The abbreviated RANGE and RANGECMP bodies are thousands of lines per second; with them enabled at 115200, a 30 s capture had 27 or 28 epochs of each log instead of 30.
- ASCII IONUTC prints A1 as a copy of A0; the binary form is correct.
