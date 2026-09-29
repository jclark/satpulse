# Bynav specific capture details

Read `novatel-config.md` first. This file covers what is specific to Bynav receivers. The message file is `configs/gpsmsg/bynav/m10-m20.toml`; the M10 and M20 sets are in `gps/testdata/packets/bynav/M10/` and `gps/testdata/packets/bynav/M20/`, and their HW.toml files record the receiver behaviour seen.

## Command replies

Commands are answered in abbreviated ASCII, `<OK` or `<ERROR:` with the error text, followed by a port prompt such as `[COM1]`. Message files for Bynav set `responsePattern = "novatel"`, so `satpulsetool gps` reports whether each command was accepted.

## Identification

`LOG VERSION` (the `get-version` tag) replies with an NMEA `$BDVER` sentence; its first field is the firmware (for example `V7.82_AB1AD3_T`).

## Dual captures

A port can output the same log in ASCII and binary at once, so make `-dual` captures (see `novatel-config.md`).

## Resets

- `reload` (RESET) restarts with the saved configuration and keeps the satellite data.
- RTKTYPE and OBSFREQ are kept across RESET without SAVECONFIG (seen on an M20), so after base-mode captures send `RTKTYPE ROVER`.
- There is no cold start: every FRESET, including `FRESET STANDARD`, is a factory reset that also clears the saved configuration, although UG017 describes the options as clearing only satellite data.

## Logs

- BESTXYZ is accepted but always reports SOL_COMPUTED/NONE with all fields zero, and the Bynav variant does not decode it.
- The data line of an abbreviated PSRDOP log has no CR/LF terminator.
- PSRPOS is rejected; RANGE and RAWEPHEM are accepted but produce nothing.
- The maximum log output rate is limited: on an M20 it was at first one log a second, and an ONTIME period below 1 s was accepted but gave one log a second. `SET OBSFREQ <n>` (2 to 10) raised the maximum to n per second at once and was kept across RESET without SAVECONFIG, although UG017 says OBSFREQ sets the observation frequency and takes effect only after SAVECONFIG and a reboot. 0 and 1 are rejected, so the original one-a-second limit cannot be restored with SET. OBSFREQ does not restrict observation epochs to its own period: at OBSFREQ 2, RANGECMPB and BESTPOSB logged with `ONTIME 1 0.3` had epochs at x.3 s.

## RTCM

- RTCM is output only in base mode: `RTKTYPE BASE` is enough, with no FIX or save, and the base position is the receiver's own single-point solution. In rover mode `LOG RTCMnnnn` is accepted but nothing is output.
- The RTCM log names take no `B` suffix: probe with `log rtcm<n> once`, since `log rtcm<n>b once` is rejected.
- A port outputs one observation message per system: requesting another (for example MSM7 or 1004 after MSM4 for GPS) replaces the earlier one, so legacy and MSM messages need separate captures.
