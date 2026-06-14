# Capture design principles

## Goal

Collect packet logs (JSONL files) that exercise every implemented decode path in the receiver's lib and domain layers, with time messages suitable for deterministic replay testing.

## Principles

- Every capture must include at least one time message (for timemsg sync testing). The one exception is the raw cross-format comparison capture (see "What to capture" item 6), which is deliberately time-free.
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
5. **Raw observations and navigation data** -- on receivers that support `--raw-out`, capture raw observations (RXM-RAWX/OBSVMB) and raw navigation data (RXM-SFRBX or per-GNSS `*EPHB`). These feed RINEX conversion and downstream positioning work. Pair raw captures with a minimal time message set so they satisfy the "every capture must include at least one time message" rule. On Unicore, follow the dual ASCII+binary convention and include compressed and NovAtel-compatible variants too -- see `unicore-config.md`.
6. **Raw cross-format comparison** -- on receivers that emit raw observations in multiple formats simultaneously, capture all the formats in one binary-only trace, with no time message. The purpose is to test that the same physical epoch decodes consistently across formats (e.g., MSM7 vs UBX-RXM-RAWX, or OBSVMB vs RANGECMPB vs MSM7). The capture is deliberately time-free because each raw observation message carries its own GPS time; the cross-format test compares those embedded times. On u-blox F9P (PROTVER 27+): UBX-RXM-RAWX + RTCM MSM7. On Unicore (UM980 and similar): OBSVMB + RANGECMPB + RTCM MSM7. Receivers that don't emit MSM (e.g., LEA-M8T) get no cross-format capture.
7. **Per-constellation time** -- if the receiver supports configuring which GNSS system the time pulse references, capture one trace per constellation with all time message variants enabled.
8. **NMEA subsets** -- specific NMEA sentence combinations needed for testing (e.g., RMC+GGA for timing correlation, GLL for future parsing).
9. **Survey** -- if the receiver supports survey-in, a short survey capture to exercise survey messages.
10. **Cross-protocol satellites** -- NMEA GSV/GSA alongside native satellite messages in the same capture. This enables cross-protocol validation: verify-replay.py checks that NMEA and native satellite lists are consistent in IDs, look angles, and CN0.

## Cold-start capture

In addition to steady-state captures, collect a cold-start capture for each receiver. The purpose is to exercise code paths that handle incomplete data: missing UTC, missing TOW/WN, invalid time status, no fix, etc. Receivers go through several phases during cold start (no time, GNSS time only, UTC available, first fix) and the decode layer must handle all of them without errors.

A cold start clears satellite data but preserves NVM configuration, so the message configuration must be saved to NVM before the cold start. This means the cold-start capture should be done last, after all other captures, because saving to NVM changes the receiver's persistent state. A factory reset is needed afterwards to restore clean NVM state. Both saving to NVM and factory reset are destructive — ask the user for approval before proceeding.

The sequence is:

1. Configure the desired messages
2. Save to NVM (see high-level and low-level config docs for the mechanism)
3. Cold start
4. Capture immediately for 120 seconds
5. Factory reset (with user approval)
6. Verify the factory reset took effect: capture a few seconds and confirm the receiver is back to its factory output (e.g. default NMEA). On u-blox Gen9+/M10 a single `--factory-reset` has been observed not to revert the saved configuration (the clear and the hardware reset can race); if the receiver is still emitting the saved set, run `--factory-reset` again and re-verify.

Name the file `coldstart.jsonl`.

## Troubleshooting

### Receiver still flooding after reload

If `--reload` is followed by configuration probes that time out, or probes that keep reporting active packet formats from a prior capture, the receiver buffer is likely still flushing high-bandwidth messages. Reload restores NVM state but does not drain the output buffer or stop messages that were already queued.

Recovery: send `UNLOGALL` explicitly and wait a few seconds before further configuration. Example for Unicore:

```
printf '[default.line]\nresponsePattern = "unicore"\ndelay = 0.2\n[[line]]\ntext = "UNLOGALL"\n' | \
  satpulsetool gps -d <device> -s <baud> --vendor unicore -m -
sleep 3
```

After this, probes should report only NMEA (the receiver's response format), with no UNCA/UNCB lingering.

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
- `raw-obs.jsonl`, `raw-nav.jsonl`, `raw-obs-nav.jsonl` -- raw observation / navigation captures (`-dual` suffix on Unicore for ASCII+binary, e.g. `raw-obs-dual.jsonl`)
- `raw-cross.jsonl` -- raw cross-format comparison capture (binary only, no time message)

Include baud rate suffix when it differs from the default/factory rate.

## Parsing errors are bugs

If `satpulsetool replay` reports parsing errors (e.g., `error processing packet`), these are bugs in the decode layer that need fixing. Packet logs from real hardware are the primary way to discover edge cases in parsers (empty fields, truncated payloads, unexpected enum values). File an issue or fix them immediately -- don't ignore them as expected behavior.
