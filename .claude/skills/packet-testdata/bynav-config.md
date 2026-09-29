# Bynav specific capture details

Read `novatel-config.md` first. This file covers what is specific to Bynav receivers. The message file is `configs/gpsmsg/bynav/m10-m20.toml`; the M10 and M20 sets are in `gps/testdata/packets/bynav/M10/` and `gps/testdata/packets/bynav/M20/`, and their HW.toml files record the receiver behaviour seen.

## Command replies

Commands are answered in abbreviated ASCII, `<OK` or `<ERROR:` with the error text, followed by a port prompt such as `[COM1]`. Message files for Bynav set `responsePattern = "novatel"`, so `satpulsetool gps` reports whether each command was accepted. The exceptions are `get-gnss` (WORKFREQS) and `get-mode` (RTKTYPE), queries whose plain-text replies that pattern does not show, so they use none and report no OK.

## Identification

`LOG VERSION` (the `get-version` tag) replies with an NMEA `$BDVER` sentence; its first field is the firmware (for example `V7.82_AB1AD3_T`).

## Dual captures

A port can output the same log in ASCII and binary at once, so make `-dual` captures (see `novatel-config.md`).

## Resets

- `reload` (RESET) restarts with the saved configuration and keeps the satellite data.
- RTKTYPE (`mode-base`, `mode-rover`) and OBSFREQ (`fix-rate-*`) are kept across RESET without SAVECONFIG (seen on an M20), so after base-mode captures send `mode-rover`, and check with `get-mode`.
- There is no cold start: every FRESET, including `FRESET STANDARD` (`factory-reset`), is a factory reset that also clears the saved configuration, although UG017 describes the options as clearing only satellite data.

## Logs

- BESTXYZ is accepted but always reports SOL_COMPUTED/NONE with all fields zero, and the Bynav variant does not decode it.
- The data line of an abbreviated PSRDOP log has no CR/LF terminator.
- PSRPOS is rejected; RANGE and RAWEPHEM are accepted but produce nothing.
- On an M20, logs requested once in rover mode: BESTPVT (10000), TRACKSTAT, HEADING and HEADING2 (all zero with one antenna), ANTIJAMTYPE (10092), EFUSEID (10091) and the ephemeris logs are output, as well as the logs the message file enables. VERSIONB, MARKTIME, MARK2TIME and PASHR are accepted but produce nothing. GLORAWEPHEM, RAWALM, REFSTATIONB, RADMI and GPGLL are rejected, as are the ASCII AUTHORIZATIONA, NMEATALKERA, RTKCONFIGA, REFSTATIONINFOA, PJKPARAA, IPCONFIGA, NTRIPCONFIGA and SHIFTDATUMA.
- `get-authorization` (`LOG AUTHORIZATION ONCE`) shows the licence: on the M20, AuthMode M2-0 with INS disabled. Its INS and IMU logs (INSPVA, INSPVAX, INSPOS, INSATT, INSSTDEV, INSCALSTATUS, CORRIMUDATA, CORRIMUDATAS, RAWIMU, RAWIMUS, RAWIMUSX, RAWIMUX) are accepted but produce nothing, and a ONCE request for one stays in LOGLIST until RESET.
- On the M20, QZSSEPHEMERIS at `ONTIME 1` (in `nov-eph`) was missing from one 30 s capture and output only twice in one 20 s test, but was output every second in four other runs; the cause is not known. Check that it is in a capture before keeping it.
- The fix rate also limits the log output rate. On an M20, logs were at first output at most once a second, and an ONTIME period below 1 s was accepted but gave one log a second. `SET OBSFREQ <n>` (2 to 10; the `fix-rate-*` tags) raised the limit to n per second at once, each log with a different solution, and was kept across RESET without SAVECONFIG, although UG017 says OBSFREQ takes effect only after SAVECONFIG and a reboot. 0 and 1 are rejected, so the original once-a-second rate cannot be restored with SET.

## RTCM

- RTCM is output only in base mode: `mode-base` (RTKTYPE BASE) is enough, with no FIX or save, and the base position is the receiver's own single-point solution. In rover mode `LOG RTCMnnnn` is accepted but nothing is output.
- The RTCM log names take no `B` suffix: probe with `log rtcm<n> once`, since `log rtcm<n>b once` is rejected.
- On an M20 in base mode, a ONCE request outputs 1003 to 1006, 1011, 1012, 1019, 1020, 1033, 1042, 1044, 1046, 1104, 1230 and MSM4 to MSM7 for GPS, GLONASS, Galileo, QZSS and BDS. 1048, 1105 to 1107 and 1134 to 1137 are accepted but produce nothing; 1001, 1002, 1007 to 1010, 1013, 1045 and MSM1 to MSM3 are rejected.
- A port outputs one observation message per system: requesting another (for example MSM7 or 1004 after MSM4 for GPS) replaces the earlier one, so `rtcm-legacy`, `rtcm-msm4` and `rtcm-msm7` need separate captures.
