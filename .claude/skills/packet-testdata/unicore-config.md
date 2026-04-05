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

