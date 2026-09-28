# ByNav specific capture details

Read `novatel-config.md` first. This file covers what is specific to ByNav receivers. The message file is `configs/gpsmsg/bynav/bynav.toml`; the M10 set is in `gps/testdata/packets/bynav/M10/`, and its HW.toml records the receiver behaviour seen.

## Command replies

Commands are answered in abbreviated ASCII, `<OK` or `<ERROR:` with the error text, followed by a port prompt such as `[COM1]`. Message files for ByNav set `responsePattern = "novatel"`, so `satpulsetool gps` reports whether each command was accepted.

## Identification

`LOG VERSION` (the `get-version` tag) replies with an NMEA `$BDVER` sentence; its first field is the firmware (for example `V7.82_AB1AD3_T`).

## Dual captures

A port can output the same log in ASCII and binary at once, so make `-dual` captures (see `novatel-config.md`).

## Resets

- `reload` (RESET) restarts with the saved configuration and keeps the satellite data.
- There is no cold start: every FRESET, including `FRESET STANDARD`, is a factory reset that also clears the saved configuration, although UG017 describes the options as clearing only satellite data.

## Logs

- BESTXYZ is accepted but always reports SOL_COMPUTED/NONE with all fields zero, and the ByNav variant does not decode it.
- The data line of an abbreviated PSRDOP log has no CR/LF terminator.
- PSRPOS is rejected; RANGE and RAWEPHEM are accepted but produce nothing.
