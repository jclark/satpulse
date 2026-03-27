# Captures using low-level configuration (message files)

This applies to all receivers. Message files provide protocol-specific commands in TOML format.

## Message file library

Existing message files are in `configs/gpsmsg/`. Each file targets a specific receiver or family:

- `ubx.toml` -- u-blox receivers (all generations)
- `ubx9.toml` -- u-blox Gen9+ receivers (includes `ubx.toml`; adds CFG-VALSET MSGOUT tags for USB port)
- `um980.toml` -- Unicore UM980/981/982
- `allystar.toml` -- Allystar TAU1201 and similar
- `techtotop.toml` -- Techtotop/Taidou receivers
- `atgm332d-v5.toml`, `atgm332d-v6.toml` -- Zhongke ATGM332D variants
- `lc29h.toml`, `lg290p.toml` -- Quectel receivers
- `bynav.toml` -- Bynav receivers
- `sinognss.toml` -- SinoGNSS receivers
- `at632.toml` -- Allystar AT6558/AT632x

See `configs/gpsmsg/format.md` for the TOML format specification and `configs/gpsmsg/tags.md` for tag naming conventions.

The tag naming conventions in `tags.md` define standard tag names for common operations (e.g., enabling/disabling specific messages, setting output rates). These standard names allow packet capture to work in a reasonably uniform way across receivers -- the same tag names (e.g., `get-version`, `enable-nav-pvt`) map to vendor-specific commands in each message file.

## Using message files for captures

```
satpulsetool gps -d <device> -s <baud> --vendor <vendor> \
  -m configs/gpsmsg/<file>.toml -t <tag1>,<tag2>,... \
  --packet-log <output>.jsonl --capture 30
```

Key points:
- `-m` specifies the message file path.
- `-t` selects which tags to send (comma-separated, sent in order listed).
- `--vendor` restricts protocol probing to the correct vendor.
- `-m` cannot be combined with high-level config flags in the same invocation, but can follow a prior high-level config invocation.
- Use `--show-tags` to list available tags: `satpulsetool gps -m <file> --show-tags`

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
