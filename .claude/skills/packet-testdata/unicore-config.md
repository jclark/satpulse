# Unicore specific capture details

## Dual ASCII/binary captures

Unicore receivers (UM980, UM981, UM982) use NovAtel-family message formats where every data message has both an ASCII (`A`) and binary (`B`) variant carrying the same data. For example, BESTNAVA and BESTNAVB both carry the same position+velocity solution.

Captures should include simultaneous ASCII+binary for all message types. This enables:
- Testing both ASCII and binary decoders against identical data
- Cross-format consistency verification
- Scanner testing with mixed packet formats in the same stream

Name these captures with a `-dual` suffix (e.g., `bestnav-dual.jsonl`).

## Four packet format tags

Unicore receivers produce packets with four different tags:
- **UNCB** -- Unicore binary (sync: `0xAA 0x44 0xB5`, header starts with digit for CPU idle %)
- **UNCA** -- Unicore ASCII (starts with `#`, first field after name is numeric)
- **NOVB** -- NovAtel binary (same sync bytes, header format differs)
- **NOVA** -- NovAtel ASCII (starts with `#`, first field after name is alphabetic port name)

All four may appear in a single capture when both Unicore-native and NovAtel-format messages are enabled.

## High-level config only enables binary

`--pvt-out`, `--sats-out`, and `--binary` only generate `*B` (binary) commands. To get dual captures:

1. Use high-level config to enable binary variants
2. Use ephemeral `-m -` or message file tags to add ASCII variants

## Ephemeral message files (`-m -`)

When using `-m -` to pipe TOML via stdin, the TOML **must** include `[default.line]` with `responsePattern` and `delay`. Without these, commands are sent without waiting for acknowledgment and may not take effect.

```
printf '[default.line]\nresponsePattern = "unicore"\ndelay = 0.1\n[[line]]\ntext = "BESTNAVA 1"\n[[line]]\ntext = "RECTIMEA 1"\n' | \
  satpulsetool gps -d <device> -s <baud> --vendor unicore -m - \
  --packet-log <file> --capture 30
```

## How `--pvt-out` maps to Unicore messages

From `gps/internal/unc/cfgopts.go`:

- `time` or `tp` -> RECTIMEB
- `pos`/`vel`/`qual` -> BESTNAVB (or BESTNAVXYZB with `ecef`). BESTNAV carries both position and velocity in a single message.
- `qual` -> STADOPB
- `leap` -> GPSUTCB, BD3UTCB, GALUTCB (per enabled GNSS). Note: BDSUTCB is not enabled by high-level config.
- `sat` -> SATSINFOB + BESTSATB

Messages not reachable via high-level config:
- PPPNAV (requires PPP configuration via message file tags)
- PPSSTATUS (no high-level flag)
- BDSUTC (only BD3UTC is enabled by `leap`)
- STADOP ASCII variant (use `-m -` with `STADOPA 1`)
- All ASCII variants of any message
- NovAtel-format messages (BESTPOS, BESTXYZ)

## Baud rate changes

`--speed` is a no-op on Unicore (`gps/internal/unc/config.go`: "Unicore doesn't yet use speed changes"). To change baud, use the `speed-<rate>-com<N>` tags in `configs/gpsmsg/um980.toml`:

```
satpulsetool gps -d <device> -s <current_baud> --vendor unicore \
  -m configs/gpsmsg/um980.toml -t speed-460800-com3
```

The COM port number depends on how the receiver is wired. Determine the current port by querying `LOGLIST`:

```
printf '[default.line]\nresponsePattern = "unicore"\ndelay = 0.2\n[[line]]\ntext = "LOGLIST"\n' | \
  satpulsetool gps -d <device> -s <baud> --vendor unicore -m -
```

The first response line is `<LOGLIST COMn ...`, where `COMn` is the port. On the lab UM980 rig, `/dev/ttyUSB0` is on COM3. Changes via `CONFIG COM<N> <baud>` are RAM-only; the receiver reverts to the NVM-saved baud after power cycle. Restore at the end of a capture session with the matching `speed-<default>-com<N>` tag.

## Bandwidth notes

Raw observations are bandwidth-heavy. The default 115200 baud is not enough for the full dual ASCII+binary OBSVM set -- even just OBSVMB+OBSVMA at 1Hz saturates the link and the scanner produces null/garbled fragments. Bump to 460800 (or higher) for any raw observation capture beyond OBSVMB alone. Use the `-460800` (or matching) suffix in the capture filename.

The cross-format capture (OBSVMB + RANGECMPB + MSM7, no ASCII, no time) is much smaller and fits comfortably at 460800.

## How `--raw-out` maps to Unicore messages

From `generateRawMsgCommands` in `gps/internal/unc/cfgopts.go`:

- `obs` => OBSVMB.
- `nav` => `GPSEPHB`, `BDSEPHB`, `GLOEPHB`, `GALEPHB`, `QZSSEPHB`, gated by `--gnss`. Messages for disabled constellations are not enabled.

High-level config only enables the binary uncompressed master variants. To cover the other implemented (and unimplemented-but-on-the-wire) raw paths, follow the high-level step with `-m -` to add:

- ASCII master uncompressed: `OBSVMA 1`.
- Compressed master and slave, binary and ASCII: `OBSVMCMPB 1`, `OBSVMCMPA 1`, `OBSVHCMPB 1`, `OBSVHCMPA 1`. The slave-antenna form produces no records on single-antenna setups, but the configuration is harmless.
- NovAtel-compatible compressed range, binary: `RANGECMPB 1` (Unicore msg id 140, per `gps/lib/novmsg/other.go`).
- ASCII ephemeris per enabled GNSS: `GPSEPHA 1`, `BDSEPHA 1`, `GLOEPHA 1`, `GALEPHA 1`, `QZSSEPHA 1`.

Suggested capture names: `raw-obs-dual.jsonl`, `raw-nav-dual.jsonl`, or `raw-obs-nav-dual.jsonl` -- the `-dual` suffix follows the existing Unicore convention for binary + ASCII in one trace.

`OBSVBASEB` and `OBSVHB` (uncompressed base/slave) are not enabled. They're out of scope for the master-antenna receiver under test.

## Raw cross-format capture

The cross-format capture (`raw-cross.jsonl`) enables three raw observation formats simultaneously, binary only, with no time message: OBSVMB (native), RANGECMPB (NovAtel-compatible), and RTCM MSM7. Each format carries its own GPS time, which is what the cross-format test compares.

```
satpulsetool gps -d <device> -s <baud> --binary --pvt-out off --raw-out obs --rtcm-out MSM7
printf '[default.line]\nresponsePattern = "unicore"\ndelay = 0.2\n[[line]]\ntext = "RANGECMPB 1"\n' | \
  satpulsetool gps -d <device> -s <baud> --vendor unicore -m -
satpulsetool gps -d <device> -s <baud> --packet-log raw-cross.jsonl --capture 30 --vendor unicore
```

`--pvt-out off` is essential here -- it suppresses the default high-level config behavior of adding daemon time messages.

## No default output

Unlike u-blox, the UM980 has no default message output after reload or factory reset. `--reload` produces a clean state with zero messages, which means there is no factory NMEA capture to collect.

## No end-of-epoch marker

Epoch detection uses (Week, MillisecondsOfWeek) header tuples, not an explicit end-of-epoch message. There is no equivalent of UBX NAV-EOE.

## PPP captures

PPPNAV requires PPP to be enabled via message file tags:

1. Set SIGNALGROUP (e.g., `signalgroup-2` for E6-HAS) -- causes receiver reset
2. Enable PPP (e.g., `ppp-has`) -- CONFIG PPP ENABLE, TIMEOUT, CONVERGE
3. Enable binary messages via high-level config
4. Wait for PPP convergence (can take 10-30 minutes for E6-HAS)
5. Capture with dual ASCII/binary via `-m -`

Monitor convergence with BESTPOSA -- look for `SOL_COMPUTED,PPP` in the output. PPPNAV messages appear regardless of convergence status (they show the current PPP solution state).

## No default output

The UM980 factory default has no message output on COM ports. Unlike u-blox, there is no default NMEA stream. `--reload` produces a clean state with zero messages. There is no factory NMEA capture to collect -- instead, enable NMEA explicitly via message file tags (e.g., `nmea-daemon`).

## Reload after SIGNALGROUP change

SIGNALGROUP changes cause a receiver reset. Allow `sleep 5` after the signalgroup command before sending further commands.

