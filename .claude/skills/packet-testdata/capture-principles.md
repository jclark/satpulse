# Capture design principles

## Goal

Collect packet logs (JSONL files) that exercise every implemented decode path in the receiver's lib and domain layers, with time messages suitable for deterministic replay testing.

## Principles

- Every capture must include at least one time message (for timemsg sync testing).
- A capture of a single time message type is fine -- testing that a lone time message works is valuable.
- Don't capture a single non-time message plus only an end-of-epoch marker (if the receiver has one); combine it with something useful.
- If the receiver supports end-of-epoch markers (currently u-blox and Quectel), some captures should include them and some should lack them -- testing epoch detection both ways matters. Most receivers do not have end-of-epoch markers.
- Reload between every capture to ensure a clean receiver state. Without this, messages enabled by a prior capture leak into the next one.
- UBX-only captures that use message file tags (`-m ... -t`) must first disable NMEA with `--binary`. The message file step does not probe or reset, so whatever protocol state exists before it runs carries through. Without `--binary`, default NMEA output remains enabled and pollutes the capture. Run `--binary` as a separate high-level config step before the `-m` step.

## What to capture

The captures should cover:

1. **Default output** -- whatever the receiver sends at factory settings. This is usually NMEA sentences.
2. **Daemon set** -- the message set that satpulsed typically enables. This is the most important UBX/binary trace.
3. **Individual message coverage** -- messages that are implemented in the lib layer but not covered by the daemon set. Group these sensibly with time messages.
4. **Low-level-only messages** -- messages that high-level config cannot enable. These need message file tags.
5. **Per-constellation time** -- if the receiver supports configuring which GNSS system the time pulse references, capture one trace per constellation with all time message variants enabled.
6. **NMEA subsets** -- specific NMEA sentence combinations needed for testing (e.g., RMC+GGA for timing correlation, GLL for future parsing).
7. **Survey** -- if the receiver supports survey-in, a short survey capture to exercise survey messages.
8. **Cross-protocol satellites** -- NMEA GSV/GSA alongside native satellite messages in the same capture. This enables cross-protocol validation: verify-replay.py checks that NMEA and native satellite lists are consistent in IDs, look angles, and CN0.

## Troubleshooting

### Baud rate confusion

If you lose track of the receiver's current baud rate (e.g. after a reload or speed change), probe at candidate speeds with no config options:

```
satpulsetool gps -d <device> -s 9600
satpulsetool gps -d <device> -s 38400
```

The one that shows receiver info is the current speed. "Framing errors" means wrong speed.

## Packet log format

Captures use `satpulsetool gps --packet-log <file> --capture <seconds>`. The output is JSONL with one packet per line:

```json
{"t":"2026-03-27T00:40:18.078959Z","tag":"UBX","msg":"NAV-PVT","bin":"b562...","out":false}
{"t":"2026-03-27T00:40:18.079123Z","tag":"NMEA","msg":"GNRMC","ascii":"$GNRMC,...\r\n","out":false}
```

Fields:
- `t` -- read timestamp (microsecond precision)
- `tag` -- protocol tag (UBX, NMEA, RTCM, etc.)
- `msg` -- message identifier
- `bin` or `ascii` -- hex-encoded binary data or ASCII text
- `out` -- true for outgoing (transmitted) packets

## Naming convention

File names indicate the content and baud rate:

- `factory.jsonl` -- default output at default/low baud
- `daemon.jsonl` -- daemon message set
- `daemon-sats-pos-38400.jsonl` -- daemon + satellites + position at 38400
- `ecef-38400.jsonl` -- ECEF messages at 38400
- `ubx-tp-tai-vel-38400.jsonl` -- specific UBX message combination at 38400
- `time-gal-38400.jsonl` -- per-constellation time capture at 38400
- `nmea-rmc-gga.jsonl` -- specific NMEA sentence subset
- `survey-38400.jsonl` -- survey-in capture at 38400

Include baud rate suffix when it differs from the default/factory rate.
