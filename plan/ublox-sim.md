# u-blox receiver simulator (#362)

A fake u-blox receiver in Go: a process (or in-process object)
that speaks UBX over a byte stream, is faithful about the
Configuration interface (CFG-VALGET/VALSET/VALDEL with ACK/NAK
semantics per the interface description), and emits periodic nav
messages by replaying a recorded packet log, gated by its own MSGOUT
configuration. It sits behind a pty for black-box tests or behind an
in-process pipe for Go unit tests.

The purpose is to smoke-test configuration *wiring* end to end --
UI -> HTTP -> gps/app/session -> gpscfg -> port -> responses ->
properties -> UI -- without GPS hardware. It is explicitly not for
testing the config engine's semantics: those stay with the gpscfg
unit tests and with real-hardware testing (gpshwtest). A fake that is
slightly more permissive than real firmware cannot mask the class of
bug this exists to catch (broken wiring), so fidelity requirements
are modest and bounded.

Related: #206 (this plan delivers a parser for the official u-blox
interface description JSON, which is the other half of what #206
asks for), #357 (the satpulseweb work whose config GUI is the main
thing this makes testable).

## Motivation

- satpulsewb ships a device-independent config GUI (#357). Replaying
  a packet log through a FIFO exercises the monitor path only:
  gpscfg skips probing on a read-only port, so connect succeeds but
  ReadConfig/ApplyConfig never run. Automated end-to-end testing of
  the config path needs something that answers.
- The same object serves three harnesses: an in-process fake
  `gpsio.Conn` for session/gpscfg unit tests (a real protocol
  conversation under synctest instead of scripted canned bytes); a
  pty-hosted process for black-box smoke tests of satpulsewb and of
  satpulsed's startup config phase; and a hardware-free backend for
  developing and Playwright-testing the workbench UI.
- Config responses arrive interleaved with periodic nav traffic,
  exactly as on a real device, so the Configurator's
  skim-nav-while-awaiting-ACK path is exercised naturally.

## Architecture: two engines, one stream

Two independent producers multiplexed onto one output stream. The
only coupling is that the NAV engine reads the config database, for
message enablement and for output pacing:

- The **NAV engine** treats a recorded packet log as a per-epoch
  bank of message content and regenerates the timeline: at each
  epoch tick it emits the currently-enabled subset as a contiguous
  burst, the way a real receiver emits its enabled set, rather than
  replaying the recording's literal timeline with holes where
  filtered messages were. Enablement comes from the config
  database's MSGOUT key for each message (zero/nonzero as off/on at
  first; the values are rates, so per-N-epoch decimation is an easy
  later upgrade). The MSGOUT-key-to-message mapping is the one
  `ubxcfgval`'s message keys (`KeyM`) encode. Emission is paced by
  the baud rate configured in the database (CFG-UART1-BAUDRATE):
  each message occupies its transmission time at the line rate, and
  the rest of the epoch is idle -- the burst-then-idle shape
  satpulse's scan layer sees on real hardware, for whatever subset
  and baud rate are configured.
- The **config engine** is a layered key-value database that answers
  CFG-VALGET/VALSET/VALDEL and MON-VER polls with the ACK/NAK rules
  from the interface description.

Input side: a framing demux parses incoming frames with
`ubxbin.ParseMsg`; config-class and poll messages go to the config
engine, everything else is ignored. Output side: an atomic mux --
whole-packet writes under a mutex, so a config response is never
spliced into the middle of a replayed frame. The whole thing runs
over an `io.ReadWriter`, which is a pty slave in black-box use and
an in-process pipe in unit tests.

All frame plumbing exists: `ubxbin` has symmetric structs for
CfgValget (request and response versions), CfgValset, CfgValdel,
AckAck/AckNak, and MonVer, plus generic `ParseMsg` and `Serialize`,
and `ubxcfgval.UnmarshalItems`/marshal handle cfgData blobs.

## The config database

A pure database: the simulator needs no per-key knowledge to get and
set values. The wire format is self-describing (value size comes
from the key ID's size bits, `Key.NValueBytes`), and the protocol's
own rules are key-agnostic. From the X20 HPG 2.10 interface
description:

- VALGET, VALSET, and VALDEL each NAK the whole message if any key
  is unknown to the receiver FW; nothing is applied.
- All three are limited to 64 items per message; VALGET responses to
  wildcard polls are paginated via the `position` field.
- Wildcards are key arithmetic: item part 0xffff means all items in
  the group, group part 0xfff means all items in all groups.
- Configuration validity is checked only when applying to the RAM
  layer. The simulator skips this check entirely and ACKs any
  well-formed set of known keys -- slightly more permissive than
  real firmware, acceptable at smoke depth. If the gap ever matters,
  the JSON's type/range metadata supports generic value validation
  without per-key code.

Layers are modelled as per-layer maps (RAM, BBR, Flash, Default),
honouring the VALSET layer mask, the VALGET layer field (including
the Default layer, `CfgValgetLayerDefault` = 7), and VALDEL's
delete-from-BBR/Flash semantics.

The probe is answered with a recorded MON-VER replayed verbatim (see
Personality below); `ProbeOK` needs only a parsed MON-VER and
version detection parses PROTVER/FWVER/MOD from the real extension
strings.

Because satpulse's `AllKeys` list spans firmware generations, a
VALGET poll can contain keys a given personality does not have; a
spec-faithful NAK means the simulator exercises satpulse's real
handling of that case (the missing-key re-poll path in the
Configurator) for free.

## The interface description JSON (#206)

A new package parses the official u-blox interface description JSON
(formatVersion 2.1): configuration groups, items, configKey, type,
constants, wildcards, and the meta block. u-blox publishes these
files on GitHub (github.com/u-blox/u-blox-X20-interface-description-json)
under terms granting use, copy, modification, and distribution for
any purpose without fee, and the files are marked audience: public;
the file is vendored into the repo with the u-blox disclaimer
alongside.

The parser gets a second consumer immediately, which is the interop
#206 asks for: a test that validates `ubxcfgval`'s hand-maintained
`cfg.yaml` key numbers and types against the vendor database. Note
the scopes differ: the published JSON is X20 (PROTVER 50.11) while
much of cfg.yaml is F9P-era (PROTVER 27.11), so validation is
per-personality -- a cfg.yaml key absent from the X20 JSON is not
automatically an error.

## Personality: one recording session

One personality (the X20P) is enough; the simulator does not need to
model every receiver to test the UI. A personality is defined by
three artifacts, all reproducible in a single sitting with the real
receiver and re-runnable when new firmware lands:

1. **The vendor JSON**: key inventory, types, identity cross-check.
2. **A default-layer dump**: a message file of CFG-VALGET polls
   against the Default layer -- all-groups wildcard, one poll per
   64-item page, with spare pages for safety -- generated from the
   JSON (which supplies the total item count in advance), run via
   satpulsetool with `--packet-log`. The simulator's defaults loader
   parses the logged responses with `ubxbin.ParseMsg`. The same
   session captures the real MON-VER, which the simulator replays
   verbatim as its identity. The JSON's own sparse `default` fields
   are not used for seeding; a load-time consistency check between
   the dump and the JSON inventory is a free cross-check of u-blox's
   published database against their firmware.
3. **A message log** for the NAV engine: recorded with everything
   satpulse can ask for enabled (the `AllMsgKeys` messages -- NAV-SAT,
   the NAV-TIME* family, NAV-TIMELS, NAV-SVIN, TIM-SVIN, TIM-TP --
   plus the standard NMEA sentences), long enough to outlast any
   test run so the replay never loops and time never wraps.
   The log supplies content, not the timeline: the NAV engine
   groups it into epochs and re-times emission per the selected
   message set (see Architecture), so the recording's own
   inter-packet spacing is not reproduced. Replay timing must be
   realistic for the enabled subset because satpulse's scan layer
   is sensitive to it (inter-packet idle detection drives the NMEA
   satellite-buffer flush). A message that appears every Nth epoch
   in the recording naturally appears every Nth epoch in the
   replay, since epoch k of the replay draws on epoch k of the
   recording.

A factory-default personality then emits exactly what a factory-
default receiver emits, because the MSGOUT gate reads the seeded
defaults.

## What the harnesses assert

Smoke depth, matching the smoketest philosophy: probe succeeds and
identifies the personality; ReadConfig populates properties without
error; an apply round-trips and a re-read shows the change;
enabling a message via config makes it appear in the stream; no
unexpected errors. Not "property X produced the right VALSET items"
-- that is a hardware-test assertion.

The test suites that use the simulator (smoketest scenarios, the
satpulsewb Playwright journeys, session/gpscfg unit-test adoption)
are follow-on work in their own changes; this plan delivers the
simulator and its personality artifacts.

## Optional extensions (not initial scope)

Each is strictly additive and none is needed for UI smoke testing:
value validation from JSON type/range metadata; per-N-epoch MSGOUT
decimation; a survey-in simulation (NAV-SVIN progressing under
TMODE); CFG-RST dropping the pty to simulate USB re-enumeration
(which would exercise the session's reconnect state machine
black-box); an F9P personality seeded from `f9p_cfg.txt`.

## Open decisions

- Package placement: `gps/internal/` is viable (cmd/ can import it;
  the desktop module never needs it) vs `gps/lib/`. Also the JSON
  parser's own package name and whether it parses at runtime or
  feeds generation.
- The binary: a standalone test-only binary vs a dev subcommand of
  satpulsetool, and where it is built from.
- Where the vendored JSON and recorded artifacts live, and the
  message log's size budget (a recording long enough to outlast
  tests may be tens of MB; trimming, compression, or a
  keep-out-of-repo REQUIRES-style skip are the options).
- Delivery relative to the satpulseweb phase stack: the simulator
  touches only new packages, so it can be its own branch off master,
  independent of the stack; the harness integrations that consume it
  land later and depend on where the stack is.
