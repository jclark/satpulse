# Septentrio receiver support: core (#340)

Add support for Septentrio receivers (mosaic-X5, mosaic-G5 and family).
Septentrio uses SBF (Septentrio Binary Format) for output and a line-based
ASCII command interface for configuration.

This is the **core**: the essential support that makes a Septentrio
receiver a usable satpulse device -- SBF decode, conversion to the
device-independent `gpsprot` Msg set (time/PPS, position, velocity,
satellites, leap seconds, quality), and Tier 2 message-file configuration.
Two capabilities are deliberately deferred to follow-up issues (see
"Follow-up add-ons"): high-level (Tier 1) configuration, and RINEX
observation output.

This plan is the parent index for three **sub-plans**. They are checkboxes
in this one issue, not separate issues:

- [ ] **`plan/sbfbin.md`** -- the `gps/lib/sbfbin` SBF wire-format codec
  (framing, CRC, per-block decode/encode, the `gpsprot.PacketFormat`
  scanner, and a do-nothing `PacketProcessor`). The foundation; no
  dependencies.
- [ ] **`plan/septentrio-msg.md`** -- the `gps/internal/septentrio`
  conversion of decoded SBF blocks into `gpsprot` Msgs, the epoch model
  (`EndOfProtocolEpoch`), and registration. **Depends on `sbfbin.md`.**
- [ ] **`plan/septentrio-msgfile.md`** -- Tier 2 configuration: the
  `[[line]]` message files under `configs/gpsmsg/septentrio/`, the `$R`
  reply `PacketFormat`, and the `"septentrio"` response analyzer.
  **Independent of `septentrio-msg.md`** (it is the ASCII command channel;
  its `$R` format does not touch SBF decode).

## Reference material and target models

Source locations (the Septentrio reference guides, a reference Python SBF
parser, and example SBF captures) are in `CLAUDE.local.md`.

**Both the mosaic-G5 and the mosaic-X5 (and family) are supported.** They
share the SBF format and the ASCII command interface and are byte-identical
for the blocks and commands core needs; they differ in firmware
capabilities (notably PPP / Galileo HAS, dual PPS, and the network stack,
each G5- or X5-specific). The sub-plans call out G5-vs-X5 differences
inline, and decode/config handle both models rather than assume one.

**Initial hardware testing is on the G5** (on order) -- that is the
hardware in hand -- so the sub-plans lead with G5; X5-specific paths are
validated when an X5 is available. Until any hardware lands, development is
driven by the guides, the reference parser, and the example captures (which
are X5-line).

## Dependencies within core

Fairly decoupled. The only intra-core dependency is `septentrio-msg` ->
`sbfbin` (the conversion consumes `sbfbin`'s decode structs).
`septentrio-msgfile` depends on neither `sbfbin` nor `septentrio-msg`, so it
can proceed in parallel. `VendorSeptentrio` already exists
(`gps/gpsreg/reg.go:32`).

## Follow-up add-ons (separate issues)

Each is its own issue and plan, built on top of core:

- **#341, `plan/septentrio-config.md`** -- high-level (Tier 1)
  `ConfigProtocol` for device-independent `--gnss`/`--pps`/mode/etc.
  **Depends on `sbfbin.md`** (probe/`ReceiverSetup` decode) and,
  procedurally, on the rest of core (it drives the ASCII command channel
  from `msgfile`). Its signal configuration uses the device-independent
  `gpsprot.Signal`/`SignalSet` (coarse, already in `gpsprot`), not the
  finer signal-number -> `SignalID` reported-signal table that `msg`
  builds -- so it does not depend on `msg`.
- **#342, `plan/sbf-rinex.md`** -- `gps/lib/rnxsbf` RINEX observation
  conversion from `MeasEpoch`. **Depends on `sbfbin.md`** (just the
  `MeasEpoch`/`MeasExtra` decode) and `gps/lib/rinex`; nothing else.

## Phasing

Core first -- it is the usable product. Within core, `sbfbin` and
`septentrio-msgfile` can proceed in parallel; `septentrio-msg` follows
`sbfbin`. Registration in `gps/gpsreg/reg.go` lands with the pieces that
need it. Each sub-plan is independently testable against the example SBF
captures (and, for `msgfile`, hand-built reply bytes) before hardware
arrives.

The two add-ons come after core, in either order, and are independent
of each other. RINEX (#342) needs only `sbfbin`; high-level config
(#341) needs `sbfbin` (probe/`ReceiverSetup` decode) plus `msgfile`'s
`$R` reply `PacketFormat` to drive the ASCII command channel. Neither
blocks the other, and neither blocks core.

Issues: core is #340 (the three sub-plans are checkboxes in it); the
add-ons are #341 (high-level config) and #342 (RINEX).
