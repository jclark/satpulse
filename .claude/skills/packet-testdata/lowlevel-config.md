# Captures using low-level configuration (message files)

This applies to all receivers. Message files provide protocol-specific commands in TOML format.

## Message file library

Existing message files are in per-vendor subdirectories of `configs/gpsmsg/`. Each file targets a specific receiver or family:

- `u-blox/ubx.toml` -- u-blox receivers (all generations)
- `u-blox/gen9.toml` -- u-blox Gen9+ receivers (includes `ubx.toml`; adds CFG-VALSET MSGOUT tags)
- `u-blox/gen8.toml` -- u-blox Gen8 and earlier (CFG-MSG rate tags)
- `unicore/um980.toml` -- Unicore UM980 and related Nebulas IV receivers
- `unicore/um982.toml` -- Unicore UM982 dual-antenna receivers
- `allystar/allystar.toml` -- Allystar TAU1201 and similar
- `techtotop/techtotop.toml` -- Techtotop/Taidou receivers
- `zhongke/atgm332d-v5.toml`, `zhongke/atgm332d-v6.toml` -- Zhongke ATGM332D/ATGM336H V5/V6 firmware
- `zhongke/at632.toml` -- Zhongke AT632-6T-30 timing receiver
- `quectel/lc29h.toml`, `quectel/lg290p.toml` -- Quectel receivers
- `bynav/bynav.toml` -- Bynav receivers
- `sinognss/sinognss.toml` -- SinoGNSS receivers

See `configs/gpsmsg/README.md` for the directory layout, `configs/gpsmsg/format.md` for the TOML format specification, and `configs/gpsmsg/tags.md` for tag naming conventions.

The tag naming conventions in `tags.md` define standard tag names for common operations (e.g., enabling/disabling specific messages, setting output rates). These standard names allow packet capture to work in a reasonably uniform way across receivers -- the same tag names (e.g., `get-version`, `enable-nav-pvt`) map to vendor-specific commands in each message file.

## Using message files for captures

```
satpulsetool gps -d <device> -s <baud> --vendor <vendor> \
  -m configs/gpsmsg/<vendor>/<file>.toml -t <tag1>,<tag2>,... \
  --port <port> \
  --packet-log <output>.jsonl --capture 30
```

Key points:
- `-m` specifies the message file path (now under a per-vendor subdirectory, e.g. `configs/gpsmsg/u-blox/gen9.toml`).
- `-t` selects which tags to send (comma-separated, sent in order listed).
- `--vendor` restricts protocol probing to the correct vendor.
- `--port <i2c|uart1|uart2|usb|spi>` supplies the active receiver port for port-dependent entries (u-blox `[[ubxvalport]]` MSGOUT tags). The `-m` invocation does not probe, so it cannot detect the port itself. Find the active port with `--show-port`. Omit `--port` only when no selected tag is port-dependent.
- `-m` cannot be combined with high-level config flags in the same invocation, but can follow a prior high-level config invocation.
- Use `--show-tags` to list available tags: `satpulsetool gps -m configs/gpsmsg/<vendor>/<file>.toml --show-tags`

## When to use low-level config

- Receivers without high-level config support (most vendors except u-blox and Unicore).
- Messages that high-level config cannot reach on receivers that do support it (e.g., u-blox NAV-TIMEBDS, NavPosLLH on HPG).
- Adding messages on top of a prior high-level config step (two-step approach).

## Adding new tags to message files

If the receiver's message file doesn't have the tags you need, add them using the `gps-msg-add` skill. Follow the tag naming conventions in `configs/gpsmsg/tags.md`.

For u-blox Gen9+ receivers, message enable/disable tags use CFG-VALSET with MSGOUT keys. The key formula and port offsets are documented in `ubx-config.md`.

## Resetting between captures (without high-level config)

For receivers that don't support `--reload`, reset between captures using:

- A reset tag from the message file (e.g., a software reset command).
- Power cycling the receiver (ask the user to do this).
- A factory-reset tag followed by re-applying baseline configuration.

The key requirement is that each capture starts from a known state with no leftover message configuration from the previous capture.

## Cold start

To save configuration to NVM: `-m <file> -t save`. To cold start: `-m <file> -t cold-start`. To factory reset afterwards: `-m <file> -t factory-reset`. Then `sleep 2` as the receiver may disconnect briefly during restart.
