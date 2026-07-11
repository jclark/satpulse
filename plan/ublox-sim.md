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

Related: #206 (validating cfg.yaml against the official u-blox
interface description JSON; handled separately, see The interface
description JSON below), #357 (the satpulseweb work whose config GUI
is the main thing this makes testable).

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
  `ubxcfgval`'s message keys (`KeyM`) encode. RTCM MSM is a special
  case: u-blox's integration manual warns against outputting MSM4
  and MSM7 together, so the recording contains MSM7 only (the
  capture kit explicitly disables MSM4), and the bank
  loader derives each epoch's MSM4 entries from the recorded MSM7
  messages with `rtcmbin`'s existing MSM7-to-MSM4 conversion (the
  same one the Ntrip push path uses). The MSM4 MSGOUT keys then
  gate derived content exactly as the other keys gate recorded
  content.
  Emission is paced by
  the baud rate configured in the database (CFG-UART1-BAUDRATE):
  each message occupies its transmission time at the line rate, and
  the rest of the epoch is idle -- the burst-then-idle shape
  satpulse's scan layer sees on real hardware, for whatever subset
  and baud rate are configured.
- The **config engine** is a layered key-value database that answers
  CFG-VALGET/VALSET/VALDEL and MON-VER polls with the ACK/NAK rules
  from the interface description.

Input side: the scan layer frames the input stream (UBX, RTCM and
SPARTN formats); config-class and poll messages go to the config
engine, and a correction input message (RTCM or SPARTN) is answered
with a synthesized UBX-RXM-COR -- which a real receiver outputs upon
successful parsing of a correction input message -- gated by its own
MSGOUT key like any output message. Everything else is ignored.
Output side: an atomic mux --
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
  is unknown to the receiver FW; nothing is applied. A group
  wildcard whose group has no keys in the inventory is not an
  unknown key: it expands to zero items and the message is ACKed,
  with the VALGET response simply carrying nothing for that group.
  A recorded ZED-F9P shows this (gps/testdata/config/u-blox/
  ZED-F9P/gpshwtest001/019.jsonl): the Configurator's signals poll
  wildcards both the SIGNAL group and the group of
  CFG-GPS_L5_HEALTH_OVERRIDE, which the F9P database has no keys
  in, and the receiver ACKs it, returning only the SIGNAL items.
- All three are limited to 64 items per message; VALGET responses to
  wildcard polls are paginated via the `position` field.
- Wildcards are key arithmetic: item part 0xffff means all items in
  the group, group part 0xfff means all items in all groups.
- Configuration validity is checked only when applying to the RAM
  layer. The simulator skips this check entirely and ACKs any
  well-formed set of known keys -- slightly more permissive than
  real firmware, acceptable at smoke depth. If the gap ever matters,
  the vendor JSON's type/range metadata would support generic value
  validation without per-key code.

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

## Port discovery (CFG-PRT)

Below protocol version 50 the Configurator learns the active
receiver port not from MON-COMMS (whose current-port field only
works fully on the X20 series) but by polling the legacy UBX-CFG-PRT
message (`ubxcfg.go` `pollPrt`, gated on `needsPort`). The shipped
F9P personality is protVer 27.50, so any config run that touches the
port -- reading it, setting messages, or changing baud -- goes down
this path; a bare NAK there fails the whole run ("NACK for request
CFG-PRT"), so CFG-PRT handling is in scope for pre-protVer-50
personalities.

CFG-PRT is a poll, not a set: on the val-based path the Configurator
only reads it (proto and baud changes go through CFG-VALSET), so the
simulator answers the poll and does not model a CFG-PRT set. The
response is synthesized from the same database the val messages see
-- the legacy protocol view of one RAM: PortID from the polled port,
baud from its `BAUDRATE` key (UART only), and the in/out protocol
masks from its `INPROT`/`OUTPROT` boolean keys -- in the spirit of
`monComms`. A CFG poll is acknowledged (interface description
"Acknowledgement" rule) and the Configurator's correlator awaits
both the response and the ACK, so the simulator emits the CFG-PRT
response followed by an ACK-ACK. A no-payload poll returns the port
the poll arrived on (the simulated port); a 1-byte poll for that same
port behaves identically; a 1-byte poll for any other port is NAKed,
since the simulator models only its own port.

## The interface description JSON (#206)

The simulator does not use the official u-blox interface description
JSON: the personality file supplies everything it needs (the key
inventory including all MSGOUT message keys, the defaults, and the
identity), and the published JSON is marked filtered -- a subset of
the firmware database -- so seeding supported keys from it would
make the simulator NAK keys a real receiver ACKs. The JSON parser
and compiled-table work (a mkconsts-style `mkdb.go`, previously in
this branch, recoverable from its history) moves to a separate
change on master focused on #206 -- validating `ubxcfgval`'s
hand-maintained `cfg.yaml` key numbers, types and constants against
the vendor database -- probably in a subdirectory of
`gps/lib/ubxcfgval`.

## Personality: one recording session

A personality is captured in a single sitting with the real
receiver, re-runnable when new firmware lands, and is defined by two
artifacts:

1. **The personality file** (`<model>-personality.ubx`), the
   required argument of `satpulsetool ubxsim`: a raw UBX stream
   holding the receiver's MON-VER and MON-GNSS responses and its
   Default-layer CFG-VALGET dump (all-groups wildcard polls, one per
   64-item page, with spare pages for safety; polls past the end of
   the database return empty pages on the X20 and NAKs on F9P-era
   firmware). The capture runs via satpulsetool with `--packet-log`
   and the file is produced from the log with `satpulsetool pack`.
   MON-VER is the simulator's identity; MON-GNSS is what the
   Configurator polls for GNSS selection and signal plans when
   configuring signals or RTCM. The Default layer doubles as the key
   inventory, so a dump-seeded personality answers configuration
   traffic for the full firmware database of the receiver it was
   recorded from.
2. **A message log** for the NAV engine: recorded with everything
   satpulse can ask for enabled (the `AllMsgKeys` messages -- NAV-SAT,
   the NAV-TIME* family, NAV-TIMELS, NAV-SVIN, TIM-SVIN, TIM-TP --
   plus the standard NMEA sentences and RTCM output: the MSM7 set,
   1230, and 1005. MSM7 only, with MSM4 explicitly disabled (see
   Architecture); 1005 appears in the recording
   only if the receiver is in base mode. The log must be long
   enough to outlast any test run so the replay never loops and
   time never wraps.
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

A recorded ZED-F9P personality (HPG 1.51, PROTVER 27.50, 1499
Default-layer items, captured 2026-07-09) is checked in at
`gps/app/ubxsim/testdata/f9p/f9p-personality.ubx` and is what unit
tests load. The X20P personality comes from its own recording
sitting. Without a replay log the simulator behaves like a receiver
with no antenna connected -- silent, but answering all configuration
traffic.

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

## Decisions taken

- Package placement: the simulator is `gps/app/ubxsim` (application
  layer; it runs goroutines); the compiled vendor database is its
  `internal/ubxdb` subpackage.
- The binary: a dev subcommand of satpulsetool (`satpulsetool
  ubxsim`, `internal/ubxsimcmd`), hosting the simulator behind a
  pty (Linux and macOS; everything but the pty is portable).
- The personality file is a required positional argument of
  `satpulsetool ubxsim`; there is no built-in personality and the
  simulator does not consume the vendor JSON (see The interface
  description JSON above). The capture kit lives in
  `gps/app/ubxsim/capture/`.

## Open decisions

- The replay log's size budget (a recording long enough to outlast
  tests may be tens of MB; trimming, compression, or a
  keep-out-of-repo REQUIRES-style skip are the options). Personality
  files are small and live in `gps/app/ubxsim/testdata/<model>/`.
- Delivery relative to the satpulseweb phase stack: the simulator
  touches only new packages, so it can be its own branch off master,
  independent of the stack; the harness integrations that consume it
  land later and depend on where the stack is.
