# Septentrio message-file configuration and reply handling (#340)

Core sub-plan of **`plan/septentrio-core.md`** (#340; a checkbox in that
issue, not a separate issue): Tier 2 configuration via message files. It talks to the
receiver's ASCII command line, not SBF, so it **depends on neither
`sbfbin.md` nor `septentrio-msg.md`** and can be implemented and tested
against the reference guide -- before hardware and independently of the
SBF decode work.

## Problem / scope

Septentrio receivers (mosaic-X5, mosaic-G5) are configured over a
line-based ASCII command interface (mosaic reference guide sec 3),
separate from the SBF binary output stream. In satpulse's support-tier
model this is Tier 2: configuration via a `-m` message file
(`gps/msgfile/`), as opposed to Tier 1's device-independent
`gpsprot.ConfigProtocol` (a later phase). The target receiver is the
mosaic-G5; the mosaic-X5 is a related but distinct model, and this plan
calls out where the two diverge.

Two message files already exist under `configs/gpsmsg/septentrio/`
(`mosaic.toml`, shared between models, and `mosaic-g5.toml`, which
`[[include]]`s it and adds G5-only entries). They are draft/untested
(hardware has not arrived -- see `CLAUDE.local.md`) and they already
set `responsePattern = "septentrio"` in anticipation of this phase.
What is missing, and what this plan designs, is:

1. How the receiver's ASCII replies get from the wire into satpulse's
   packet framing (no new binary/checksummed format is needed).
2. A new `responsePattern = "septentrio"` analyzer
   (`gps/msgfile/sept.go`) implementing real ack/nak correlation for
   these replies, wired into the existing `ResponsePattern` enum,
   `LineMsg.analyzeRequest`, and the correlator's analyzer map.
3. The remaining command-coverage gaps in the two message files, and
   the model-aware (G5 vs X5) constraints a future contributor must
   respect when extending them.

## The command-line interface (guide sec 3.1, identical on both models)

Every command is a line: a mnemonic or full name (`set*`, `get*`,
`exe*`, `lst*`) plus comma-separated arguments, terminated by
CR/LF/CRLF. The receiver echoes a reply and then prints a prompt (e.g.
`COM1>`) before it will accept the next command. This is a hard
protocol requirement, not a nicety -- guide sec 3.1: *"The prompt
indicates the termination of the processing of a given command. When
sending multiple commands to the receiver, it is necessary to wait for
the prompt between each command."* Commands are single-flight: a client
must wait for the prompt before sending the next one.

**Consequence: satpulse must detect the prompt.** Since the reply's
state-line count is not fixed (guide: "one or more additional lines
... depending on the command"), the prompt is the *only* definitive
"command done" marker -- the ack line and a fixed timeout are not
enough to know processing finished. So the `COMx>` prompt must be
framed and consumed, not treated as human-only decoration; it is framed
as the tail of the single reply packet (see "Packet framing"), which is
what makes the packet's arrival the completion signal.

The guide (sec 3.1.3) actually defines four distinct reply framings,
not two:

- **`$R: <command>`** -- for valid `set`/`get`/`exe` commands, the
  first line is an EXACT copy of the command as the user typed it,
  prefixed with `$R: `. One or more additional lines usually follow,
  reporting the resulting configuration (e.g. `setNMEAOutput, stream1,
  com1, GGA, sec1` echoes back, then a second line `NMEAOutput,
  stream1, com1, GGA, sec1` reports the new state). Some commands
  produce no extra line; the guide does not promise a fixed count.
- **`$R; <command>`** -- for `lst` commands, the first line is again
  an exact echo, but prefixed with `$R;` (semicolon). It is followed
  by a pseudo-prompt line `---->` and one or more `$--BLOCK` sections;
  if a `lst` command has to emit several sections, each intermediate
  one ends with another `---->`, and only the very last section ends
  with the real prompt.
- **`$R! <CanonicalName>`** -- used by the small set of user-management
  commands (`login`/`logout`/`lstCurrentUser`). Unlike `$R:`, this is
  NOT a verbatim echo: it reports the
  command's canonical name (e.g. typing `lcu` or `login, admin, admin`
  gets back `$R! lstCurrentUser` / `$R! LogIn`), never the arguments
  (so a password is never echoed back). One or more state lines
  follow, same as `$R:`.
- **`$R? <name>: <error text>`** -- for invalid or unauthorized
  commands. `<name>` is not reliably the command's own name: examples
  in the guide show it as the command's canonical name for
  authentication failures (`$R? LogIn: Wrong username or password!`)
  but as the underlying configuration item's name for authorization
  failures on an otherwise-valid command (`$R? SBFOutput: Not
  authorized!` in response to a rejected `setSBFOutput`/`sso`). There
  is no general rule for deriving `<name>` from the sent command text.

`$R!` and `$R?` do not carry a text-identical, or even
mechanically-derivable, echo of what was sent; the analyzer design
below is built to not need one.

## Packet framing: one reply format

None of the four reply shapes carries a checksum, so the NMEA
`PacketFormat` cannot frame them: `nmeamsg.CheckSyntax` hard-requires
the trailing `*HH` triplet, so a bare `$R: setNMEAOutput, stream1,
com1, GGA, sec1\r\n` reaches CR still unframed and is abandoned. The
reply channel needs its own real `PacketFormat`; it cannot ride the
`EmptyTag` fallback, since `gps/scan` makes no guarantee about how
unmatched bytes are divided into packets (the `bufferLines` re-slicing
in `internal/gpscmd/response.go` is best-effort, not a reliable packet
boundary).

**A whole reply is one packet, from the leading `$R` through the
terminating prompt's `>`.** Guide sec 3.1.3 licenses this directly: the
ASCII reply to a `set`/`get`/`exe` command, *including the terminating
prompt, is atomic* -- it "cannot be broken by other messages from the
receivers". Every tag in the two message files issues `set`/`get`/`exe`
(plus `login`), so the byte run from `$R` to the prompt always arrives
contiguously, with no SBF or NMEA interleaved: it is one well-defined
packet, and the prompt inside it is the "command done" signal. So there
is a single format and a single tag (`TagReply`), framing the echo
line, every state line, and the prompt together -- not the earlier
sketch of a `$R` echo-line format plus a separate state-line/prompt
format. (An `lst` reply is the one multi-unit case: its `$--BLOCK`
sections frame as further `TagReply` packets, which the analyzer
stitches back together -- see below.)

The format is checksum-free (modeled on `nov.AbbrevAsciiPacketFormat`
in `gps/internal/nov/abbrevasciipacket.go`:
`ExtractChecksum`/`ComputeChecksum` return nil). It syncs on `$` then
`R`, disambiguating from SBF's `$@` and from NMEA's checksummed `$`-led
sentences the same way the SBF format syncs `$` then `@`. It is
registered beside the SBF format for the Septentrio vendor;
`CreatePacketFormats` prepends the common NMEA and RTCM formats, so the
reply format follows NMEA in scan order. That is harmless: a `$R` reply
carries no `*HH`, so the NMEA format cannot frame it, and the reply
format frames it on its `$`+`R` header.

### What a valid packet is

A reply packet is

> ( `$R` <type> | `$--` ) <body> <terminator>

- **Header** -- either the two bytes `$R` then a **type char**, one of
  `:` `;` `!` `?` (the four reply shapes of sec 3.1.3), or `$--`, which
  opens a `$--BLOCK` section of an `lst` reply. The `R`+type char, or
  the second `-`, is what anchors the format against any other `$`-led
  run.
- **Body** -- any (possibly empty) sequence of **content chars** and
  strict `CR LF` pairs. A content char is printable ASCII `0x20`-`0x7E`
  except `$`; a line break is always a `CR LF` pair, never a lone `CR`
  or lone `LF`. (Replies are CRLF-delimited; only the commands we
  *send* may use bare CR or LF.)
- **Terminator** -- the *first* `CR LF` + **token** + `>` after the
  header. A token is either `----` (four hyphens, the `lst`
  pseudo-prompt) or four chars matching `[A-Z][A-Z0-9]{3}`. The packet
  ends at that `>`: it is the last byte, and no trailing `CR LF` is
  consumed, because the prompt has none -- the receiver waits after it.

A 4096-byte length cap (tunable) bounds each packet; a candidate that
reaches the cap with no terminator is not a packet. `$R:`/`$R!`/`$R?`
replies are small -- the largest in the guide,
`getReceiverCapabilities`, is well under 200 bytes. An `lst` reply can
run to kilobytes, but it frames unit by unit and each `$--BLOCK`
section is bounded by its own `---->` (or the final prompt), so no
single packet approaches the cap for config-style listings; the cap is
a backstop, the way `nov`'s `abbrevMaxLength = 160` bounds a single
line. A pathological single block larger than the cap -- e.g. a raw
`lstRecordedFile` download -- would be dropped, which is out of scope.

Two properties make this definition well-formed:

**Content never contains `$`, so a packet can never run across the next
message boundary.** The character set the guide allows in reply strings
(sec 3.1.4, `ABC...xyz0-9` plus `!#%@()*+-./:;<=>?[\]^_'{|}~`) has no
`$` -- a literal `$` is reachable only inside a password, via the
`%%DL` escape, and passwords are never echoed. So everything that
begins with `$` is a *new* message (SBF `$@`, NMEA `$`, a Septentrio
`$T*` event line such as `$TE`, broadcast before a reset, or `$TD`, the
ASCII display, the next `$R`) or a `$--BLOCK` -- none of which carries a
`$R<type>` header, so none can be mistaken for a continuation of the
current body.

**No reply body ever contains the sequence `CR LF <token> >`, so the
first such match is always the true prompt.** This is what lets the
terminator be a first-match without risking a premature cut. A state
line is a comma-separated list of config items, never a bare 4-char
token immediately followed by `>`; and the token alphabet -- every
connection descriptor on both models (`COM1`-`COM4`, `USB1`-`USB2`,
`DSK1` on G5, plus `IP10`-`IP17`/`NTR1`/`IPS1`/`IPR1` on X5), `STOP`
(the reset/halt prompt), and `----` -- is exactly the set of tokens a
prompt can be. In practice satpulse connects over a COM or USB port and
the prompt reflects our own descriptor (sec 3.1.3), so only
`COMx>`/`USBx>` (plus `STOP>` and `---->`) ever actually appear.

### Tricky cases the definition covers

- **A state line that begins with a 4-char uppercase run is body, not a
  terminator.** `$R: grc\r\nReceiverCapabilities, Main, ...\r\nCOM1>` is
  one packet: after the `CR LF`, `Rece` is four valid token chars, but
  the fifth byte is a letter, not `>`, so it is not a terminator and the
  body runs on to the real `COM1>`.
- **A `$R:` reply with no state line still frames.** Some commands print
  only the echo line: `$R: <cmd>\r\nCOM1>` -- empty body, immediate
  terminator.
- **`lst` output frames unit by unit.** `$R;
  lstAsciiDisplay\r\n---->` frames (the `----` token terminates it), and
  each following `$--BLOCK` section frames as its own packet -- the
  format also syncs on `$--` -- with the last section ending at the real
  prompt (see below).
- **An unsolicited event abutting a reply does not extend it.**
  `exeResetReceiver` yields `$R: erst, ...\r\nResetReceiver, Soft,
  none\r\nSTOP>` (one packet -- `STOP` is a valid token) immediately
  followed by an unsolicited `$TE ResetReceiver\r\nSTOP>`; the `$TE`
  line has no `$R` header, so it is not part of the packet and its
  trailing `STOP>` frames nothing.
- **A bare prompt is not a packet.** The reply to a comment (`#...`) or
  an empty command line is just `COMx>` with no `$R` (sec 3.1.3), which
  the format does not frame -- harmless, since no tag sends comments or
  empty lines. Likewise a mid-body `$`, a control or high-bit byte, an
  unpaired `CR` or `LF`, a non-token 4-char group before `>`, or an
  over-length run all mean "not a packet".

**A `lst` reply frames unit by unit, and the analyzer stitches the
units back together.** Per sec 3.1.3 *every* `lst` reply has the
pseudo-prompt `---->` as its second line, before the first `$--BLOCK`
section; the guide's own single-block example makes this concrete:
`$R; lstAsciiDisplay\r\n---->\r\n$--BLOCK 1 / 1\r\n...\r\nCOM1>`. Each
unit -- the `$R;` opener and each `$--BLOCK` section -- frames as its
own packet: the format syncs on `$--` as well as `$R<type>`, and a
`---->` closes a packet just as a real prompt does. The analyzer
(next section) treats the `$R;` opener as the ack -- the command was
accepted, so `OK` is reported immediately (`responseAckMore`) -- while
keeping the request open so the read loop keeps reading and
single-flight pacing holds. Each intermediate `$--BLOCK` (ending in
`---->`) is shown but not correlated (`responseInfo`); the final
`$--BLOCK`, ending at the real prompt, completes the command with no
second ack line (`responseDone`). This frames cleanly for config-style
`lst` output, which is printable with no mid-line `$` (the `$--BLOCK`
header is the only `$`, and it opens each packet); a raw
`lstRecordedFile` file download, whose block content is arbitrary
bytes, will not frame and is out of scope. The `$R! lstCurrentUser`
reply, which despite its name uses the `$R!` shape rather than `$R;`,
frames as a single ordinary reply.

Verified on a mosaic-G5: `lstInternalFile, Identification` returns a
five-`$--BLOCK` XML identity dump; `get-identity: OK` is reported at the
opener, all six units (opener plus five blocks, the fifth ending at
`USB1>`) display in order, and the command completes at that final
prompt. A normal command sent after it is held until the prompt, then
acked cleanly.

## The `"septentrio"` responsePattern analyzer

New file `gps/msgfile/sept.go`, mirroring the shape of `unc.go` (the
Unicore analyzer): a package-level `analyzeUnicoreAck`-style response
classifier plus a `LineMsg.analyzeRequestSeptentrio` method.

### Ack/nak classification

```go
func analyzeSeptentrioResponse(pkt string) responseAnalysis
```

operates on the whole reply packet -- the recognized `TagReply`
packet handed straight to `CorrelatePacket`, not the `flushLine` path
(which only re-slices unrecognized `Format == nil` bytes) -- and
classifies on the `$R` type char at byte 2:

- `:`, `!`, or `;` (`$R:`/`$R!`/`$R;`) -> `responseAck`.
- `?` (`$R?`) -> `responseNak`, with `ackError` set to the reply's
  message text: the `<name>: <error text>` between the `$R? ` prefix
  and the terminating `\r\n<prompt>`, kept intact rather than trying to
  strip `<name>` (see below).

Every `TagReply` packet begins with `$R` plus one of those four
chars, so byte 2 alone classifies it; no headerless state line arrives
under this tag (the state lines ride inside the ack packet).
`factoryReset`'s `$R: factoryReset: Resetting receiver to factory
defaults.` needs no special case -- it is an ordinary `$R:` ack whose
text happens to carry an appended message after a colon.

### Correlation key: a shared constant, not the echoed text

Matching `ackCorrelate` against the sent text (the verbatim `$R:` echo,
guide sec 3.1.3) holds for `$R:` alone, but not for `$R!` (canonical
name, not verbatim text -- e.g. `login, admin, admin` comes back as
`LogIn`) or `$R?` (a command-or-config-item canonical name that
sometimes does not even match the command, e.g. `setSBFOutput`/`sso`
rejected as `SBFOutput`). So literal matching is out.

Instead, every Septentrio line message uses a single fixed
`ackCorrelate` value (e.g. `"cmd"`) on both the request and the
response side. Concretely:

- `LineMsg.analyzeRequestSeptentrio()` returns `ackTag:
  septentrio.TagReply, ackCorrelate: "cmd", expectAck:
  ExpectAckOrNak, expectData: expectDataWithAck` -- the whole reply is
  framed as one packet by the `$R` reply format, so it arrives under
  `TagReply`, not `EmptyTag`.
- `analyzeSeptentrioResponse` sets `ackCorrelate: "cmd"` on every ack
  and nak it recognizes, regardless of what text follows the `$R:`/
  `$R!`/`$R;`/`$R?` prefix.

This works because the protocol structurally guarantees at most one
outstanding, un-acked Septentrio command at a time (the guide's "not
queued" rule), and the existing `Correlator.ReadyToSend` already
enforces exactly that: it refuses to let `sendAllMsgs` send the next
message while a pending request shares the same `(ackTag,
ackCorrelate)` and has not yet resolved. With a single shared key,
`ReadyToSend` becomes the serialization gate that mirrors the
receiver's own rule -- the tool naturally waits for one command's
ack/nak before sending the next, which is what the protocol requires
anyway. If more than one Septentrio request is ever pending
simultaneously (which should not happen given `ReadyToSend`), the
match becomes ambiguous and `correlateAck` reports it as such rather
than misattributing a reply -- a safe degradation.

None of this affects what the user sees: `formatAck`/`formatPacket` in
`internal/gpscmd/response.go` display the message's own `Tag`/`Index`
(from `RawMsg.MsgID()`) for the "which request" label, and the full
received line's text for content -- neither depends on the internal
correlation string. So the shared key only simplifies matching; the
displayed ack/nak text is exactly what the receiver sent, `<name>:
<error text>` included.

### Data expectation: expectDataWithAck

The state lines and the prompt ride inside the single ack packet, so a
Septentrio request expects no *separate* data response:
`analyzeRequestSeptentrio` sets `expectData: expectDataWithAck` ("data
combined in ack packet"). The correlator then completes the request on
the ack or nak alone (`requestComplete`: `expectDataWithAck` is done at
`ackSuccess`/`ackFailed`), and `correlateAck` marks the ack packet
`LevelSoleResponse`, so its text -- the readback -- is displayed. There
is no per-command modeling of how many state lines follow (the guide
says the count is not fixed) and no `expectDataMultiple` accumulation:
the prompt that ends the packet *is* the "command done" marker, so
completion falls out of the framing itself. This is a single-flight
protocol, so there is no timer-based pacing. `exeResetReceiver` and
friends simply end their one packet in `STOP>` rather than `CD>`.

### Wiring

1. `ResponsePattern` enum (`gps/msgfile/msgfile.go`): add
   `ResponsePatternSeptentrio` to the `const` block and `"septentrio"`
   to `responsePatternStrings`. `ParseResponsePattern`/
   `MarshalText`/`UnmarshalText` need no change (they iterate the
   array generically). This is the value the two existing TOML files
   already reference in `[default.line].responsePattern` -- until this
   lands, `mosaic.toml`/`mosaic-g5.toml` fail to load (the enum rejects
   `"septentrio"`), so the enum and the JSON schema below must land
   with, or before, any use of the config files.
   Also add `"septentrio"` to the two `responsePattern` enums in
   `configs/gpsmsg/gpsmsg-schema.json` (`["none", "unicore"]`), which
   otherwise reject the value in editor/validation tooling.
2. `LineMsg.analyzeRequest` (`gps/msgfile/line.go`): add a branch
   alongside the existing Unicore one:

   ```go
   if lm.RespPattern != nil {
       switch *lm.RespPattern {
       case ResponsePatternUnicore:
           return lm.analyzeRequestUnicore()
       case ResponsePatternSeptentrio:
           return lm.analyzeRequestSeptentrio()
       }
   }
   ```

3. The `$R` reply `PacketFormat` and its `TagReply` live in
   `gps/internal/septentrio` (where vendor `PacketFormat`s live) and
   are registered in `gps/gpsreg/reg.go`: a `Tag` re-export plus entries
   in `allVendorPacketFormats` and
   `allVendorPacketFormatsMap[VendorSeptentrio]`, after the NMEA and
   RTCM formats that `CreatePacketFormats` prepends (see "Packet
   framing").
4. `NewCorrelator()` (`gps/msgfile/correlate.go`): add
   `septentrio.TagReply: septAnalyzer{}` to the `analyzers` map (a
   small struct type, paralleling `uncaAnalyzer{}`, whose
   `analyzeResponse` calls `analyzeSeptentrioResponse`).
5. `formatText`/`formatPacket` (`internal/gpscmd/response.go`) assume a
   single-line packet: `formatText` strips one trailing CR/LF and
   hex-dumps if any remaining byte is non-printable. A `TagReply`
   packet spans several CRLF-separated lines, so `formatText` must pass
   internal `CR LF` through -- displaying the reply as its lines --
   rather than falling back to the hex path.

## The existing message files

`configs/gpsmsg/septentrio/mosaic.toml` holds every command confirmed
to work identically on mosaic-X5 and mosaic-G5 (per the model
comparison below); `mosaic-g5.toml` `[[include]]`s it and adds G5-only
entries. Both set `[default.line]` `eol = "\r\n"` and `responsePattern
= "septentrio"` (already anticipating this phase's analyzer). Coverage
today, tag group by tag group (see `configs/gpsmsg/tags.md` for the
naming convention each tag follows):

- `get-version` -- `getReceiverInterface` followed by
  `getReceiverCapabilities`, compact ordinary command replies for receiver
  name, command-line interface version, supported signals, ports, and
  capabilities.
- NMEA output control on NMEA Stream1: each of `GGA`/`GLL`/`GSA`/
  `GSV`/`RMC`/`ZDA` has a self-contained `nmea-<s>-usb1` (sets USB1 +
  1 Hz + the sentence in one command, so it works on a factory-fresh
  stream), a composable `nmea-<s>-cur` (adds only the sentence, riding
  the stream's current Cd and interval), and one port-neutral
  `nmea-<s>-off`. Shared `nmea-port-usb1`/`-usb2`/`-com1`/`-com2` and
  `nmea-rate-1`/`-2`/`-5`/`-10`/`-20` setters, a port-neutral
  `nmea-off`, a `get-nmea` query, and the self-contained
  `nmea-daemon-usb1` group, all via `setNMEAOutput`'s `+`/`-`
  combinable syntax. Only sentences common to both models are used.
- NMEA version (`get-nmea-ver`, `nmea-ver-3`, `nmea-ver-410`) via
  `setNMEAVersion`'s two-profile (`v3x`/`v4x`) enum.
- Elevation mask (`get-min-elev`, `min-elev-0` through `-45`) via
  `setElevationMask, all, N` (both the tracking and PVT masks
  together, to match other vendors' single-mask semantics).
- Constellation selection (`get-gnss`, `gnss-gps`/`-gal`/`-glo`/`-bds`/
  `-gps-gal`/`-all`) via `setSatelliteTracking`.
- PPS (`get-pps`, `pps`, `pps-off`) via `setPPSParameters`, using only
  the argument positions and semantics common to both models (see
  model differences below).
- Fix rate (`get-fix-rate`, `fix-rate-1/2/5/10/20`) via `setSBFOutput`
  on a dedicated `Stream3` carrying `PVTGeodetic` -- Septentrio has no
  separate "solution rate" knob, only per-stream output interval.
- Port speed (`get-uart`, `speed-9600` through `-921600`) via
  `setCOMSettings`.
- Restart (`hot-start`, `cold-start`) via `exeResetReceiver`.
  `warm-start` is not implemented (see TODOs below).
- Configuration management (`save`, `reload`, `reset`, `factory-reset`)
  via `exeCopyConfigFile`/`exeResetReceiver`/`factoryReset`.
  `factory-reset` is a plain `factoryReset` (a `$R:` reply with a
  trailing message); it no longer prepends a `login` -- placeholder
  admin credentials do not authenticate, and `factoryReset` itself
  needs no session. No tag exercises the `$R!` (`login`) shape now;
  `login`/`logout`/`lstCurrentUser` are reachable only by hand.
- Survey-in (`get-survey`, `survey`, `survey-off`, `mobile`) via
  `getPVTMode`/`setPVTMode`, approximating auto-base self-survey
  (no duration/accuracy controls exist on this receiver family).
- Fixed position (`get-fixed-pos`, `fixed-pos-example`,
  `fixed-pos-off`) via `getStaticPosGeodetic`/`setStaticPosGeodetic`/
  `setPVTMode`.
- RTCM output (`rtcm-arp`, `rtcm-msm4`, `rtcm-msm7`, `rtcm-eph`,
  `rtcm-eph-off`, `rtcm-off`, `get-rtcm-base-id`,
  `rtcm-base-id-0`/`-1234`) via `setRTCMv3Output`/
  `setRTCMv3Formatting`. `rtcm-4072-0`/`-1` are not implemented (see
  TODOs below).
- RTK mode (`mode-base`, `mode-rover`) via `setPVTMode`, since
  Septentrio has no dedicated base/rover command, only PVT mode.
- SBF output control on SBF Stream1: per-block tags for every SBF
  block satpulse decodes (`gps/lib/sbfbin`), each in the same three
  forms as the NMEA sentences -- self-contained `sbf-<block>-usb1`,
  composable `sbf-<block>-cur`, and port-neutral `sbf-<block>-off` --
  plus shared `sbf-port-*`/`sbf-rate-*` setters, a port-neutral
  `sbf-off`, and a `get-sbf` query. (The earlier `sbf-daemon`/
  `get-sbf-daemon` convenience pair was dropped: no other message
  file carries a binary daemon group.)

`mosaic-g5.toml` adds the G5-only `ppp-has`/`ppp-off` pair (enabling
Galileo HAS via `setSignalUsage,,+GALE6BC` and `setPVTMode,,+PPP`).
`ppp-b2b`/`ppp-mdc`/`ppp-has-b2b` are not implemented: the G5 v1.1.0
guide documents only the `PPPGalileoHAS` capability, with no BeiDou
B2b or QZSS MADOCA-PPP correction-decode path (BDSB2b can be tracked
as a raw signal, but there is no PPP correction pipeline behind it on
this receiver).

Remaining command-coverage TODOs, already called out inline in the
TOML files and worth tracking here so they are not lost:

- `nmea-ver-400` -- no separate "4.00" profile exists; `setNMEAVersion`
  only offers `v3x`/`v4x`.
- `nmea-ver-411` -- `v4x` (used for `nmea-ver-410`) adds signal-ID
  fields, but no guide text ties a GB/GQ talker-ID change to a
  specific NMEA version selection; `setNMEATalkerID` only offers
  `auto`/`GP`/`GN`. Needs hardware confirmation before implementing.
- `warm-start` -- `exeResetReceiver`'s `EraseMemory` has no granular
  "ephemeris only" option; there is no way to clear ephemeris while
  keeping almanac.
- `rtcm-4072-0`/`rtcm-4072-1` -- RTCM message 4072 (u-blox's
  proprietary Moving Base message) is not in Septentrio's supported
  RTCMv3 list on either model; moving-base operation instead uses
  `setGNSSAttitude, MovingBase`, a different mechanism entirely.
- `survey` -- no configurable duration/accuracy threshold exists for
  auto-base self-survey; the implemented tag is the closest available
  approximation (continuous refinement, no stop condition).
- `ppp-b2b`/`ppp-mdc`/`ppp-has-b2b` -- no documented capability on
  G5 v1.1.0 (see above); revisit once real hardware and
  `getReceiverCapabilities` output are available.

## Model-aware command differences (target: mosaic-G5)

The mosaic-G5 firmware line differs from mosaic-X5 in ways that matter
for anyone extending these message files. The split between
`mosaic.toml` (shared) and `mosaic-g5.toml` (G5-specific) already
follows this rule; state it explicitly so future additions keep to it:

- **G5 has no network stack at all.** No `setNTRIPSettings` (NTRIP
  client/server/caster), no `setEthernetMode`, no `exeFTPUpgrade`/
  `setFTPPushRINEX`/`setFTPPushSBF`, and no `IP*`/`NTR*`/`IPS*`/`IPR*`
  connection-descriptor (`Cd`) values anywhere a command takes one
  (`setSBFOutput`, `setNMEAOutput`, `setRTCMv3Output`, etc.). Only
  `COM1-4`, `USB1-2`, `DSK1`, and `all` are valid `Cd` values on G5.
  Every `Cd` argument in `mosaic.toml` is written to this common
  subset for exactly this reason.
- **G5 has no RTCMv2.** `setRTCMv2Output`/`sr2o` and the related
  `sr2c`/`sr2h`/`sr2f`/`sr2i` commands are X5-only; the RTCMv3
  message-type enum in `setRTCMv3Output` is otherwise byte-identical
  between models, which is why `rtcm-*` tags are safe to share.
- **G5 has no `Meas3`/`RinexMeas3` SBF group** and G5's `setSBFOutput`
  therefore has a narrower block-group vocabulary than X5's; this
  matters for the `sbf-<block>-*` output tags. They cover the blocks
  satpulse decodes, none of which is in the G5-absent `Meas3` group;
  any future SBF-output addition should stay within blocks present on
  both models.
- **`setPPSParameters`'s fourth-from-last argument is renamed.** X5
  calls it `MaxSyncAge`; G5 calls it `MaxHoldover` (tied to a
  holdover/anti-spoofing concept G5 also adds `setHoldoverTrigger`/
  `sht` for). The argument position and "0 means never time out"
  behavior are unchanged, so the shared `pps`/`pps-off` command text
  in `mosaic.toml` is safe -- it just never names the argument.
  **G5 also has a second, independent PPS output**
  (`setPPS2Parameters`/`sps2`); X5 has none. Not covered by any tag
  today; would need a G5-only `pps2`/`pps2-off` addition in
  `mosaic-g5.toml` if ever exposed.
- **`setNMEAOutput` sentence lists differ.** X5 has `SNC`/`LLK`/`LLQ`/
  `GMP` sentences G5 lacks; G5 has `GGAaux1` (multi-antenna) X5 lacks.
  `mosaic.toml` only uses the common set (`GGA`/`GLL`/`GSA`/`GSV`/
  `RMC`/`ZDA`).
- **`setGalOSNMAUsage`/`sou` is shared**, but its strict mode's time
  source differs: X5 needs `setNTPClient` (it has a network stack);
  G5 has no NTP, so strict OSNMA depends on `exeSetTime` (manual/
  serial time) instead. No tag covers OSNMA today; a future addition
  needs this branch.
- **Config management is shared** (`exeCopyConfigFile`/
  `exeResetReceiver`) except X5's `EraseMemory` adds a
  `+TLSCertificate` option (G5 has no TLS, hence no certificate
  store). G5 firmware upgrade is `exeResetReceiver, Upgrade` plus a
  direct serial/USB transfer; X5-only `exeFTPUpgrade`/
  `setFTPPushRINEX`/`setFTPPushSBF` have no G5 equivalent.
- **L-band beam configuration differs in shape.** Both have
  `setLBandBeams`/`slbb` and `setLBandSelectMode`/`slsm`, but X5 has
  `auto`/`off`/`manual` modes with 2 beam slots and a richer LBAS2
  sub-channel scheme, while G5 has only `off`/`manual` with 5 beam
  slots, Inmarsat only. No tag covers L-band today; a future one
  would need a full model branch, not just an argument-name
  difference.

None of these differences affect the response-handling design above:
the ASCII command-line framing itself (prompt, `set`/`get`/`exe`/
`lst` command types, the four `$R*` reply prefixes) is word-for-word
identical between models per the guide's sec 3.1 -- this phase's
analyzer and packet-framing design apply unchanged to X5 if that model
is ever supported.

## Phasing (within this plan)

1. **The `$R` reply `PacketFormat`** in `gps/internal/septentrio` (+
   `TagReply`), framing the whole reply through the prompt,
   registered in `gps/gpsreg/reg.go` for the Septentrio vendor. Testable
   offline against hand-built `$R:`/`$R;`/`$R!`/`$R?` reply-plus-prompt
   byte sequences (including the not-a-packet cases: `$`, control byte,
   non-token 4-char group, over-length; and the `STOP>` and `---->`
   terminators).
2. `gps/msgfile/msgfile.go` -- add `ResponsePatternSeptentrio`.
3. `gps/msgfile/sept.go` -- `analyzeSeptentrioResponse`,
   `LineMsg.analyzeRequestSeptentrio` (`expectDataWithAck`), and the
   `septAnalyzer` type.
4. `gps/msgfile/line.go` -- dispatch branch in `analyzeRequest`.
5. `gps/msgfile/correlate.go` -- register `septAnalyzer{}` under
   `septentrio.TagReply` in `NewCorrelator()`.
6. `internal/gpscmd/response.go` -- let `formatText` display a
   multi-line reply packet as its lines instead of hex-dumping on the
   internal CR/LF.
7. Re-verify the two existing TOML files against the finished
   analyzer (`satpulsetool gps -m ... --show-tags`, and dry runs
   against a captured/replayed session once available -- see
   Testing) and fill in any of the command-coverage TODOs above that
   turn out to be tractable without hardware.

This phase adds the `$R` reply `PacketFormat` in
`gps/internal/septentrio`; the rest is confined to `gps/msgfile` and
`internal/gpscmd` plus the two config TOML files.

## Testing

- Format-level tests in `gps/internal/septentrio` (offline, no
  hardware): drive hand-built reply byte sequences through the scanner
  and assert one `TagReply` packet per reply, from `$R` through the
  prompt:
  - a plain `$R: <cmd>\r\n<state>\r\nCOM1>` reply frames as one packet.
  - a reply whose state line begins with a 4-char uppercase run --
    `$R: grc\r\nReceiverCapabilities, ...\r\nCOM1>` -- frames in full,
    i.e. the leading `Rece` is not mistaken for the terminator token
    (the tricky case an implementation is most likely to botch).
  - the `STOP>` and `---->` terminators each close a packet.
  - not a packet (nothing framed): a mid-reply `$`, a control or
    high-bit byte, an unpaired `CR`/`LF`, a non-token 4-char group
    before `>`, and an over-length run.
- Unit tests in `gps/msgfile/sept_test.go` (same package, following the
  `go-unit-test` skill and the existing `unc_test.go`/
  `TestCorrelatorUnicore` shape for a `TestCorrelatorSeptentrio`),
  feeding whole reply packets to the analyzer/correlator:
  - `$R: <cmd>\r\n<state>\r\nCOM1>` ack recognized and correlated to a
    pending request; the readback is surfaced (`LevelSoleResponse`).
  - `$R? <name>: <error>\r\nCOM1>` nak recognized, `ackError` carries
    the `<name>: <error>` text (prompt stripped), correlated to the
    same pending request despite no textual overlap with the sent
    command.
  - `$R! <CanonicalName>\r\n...\r\nCOM1>` (e.g. a `login` request)
    treated as ack.
  - `$R; <cmd>\r\n---->` (`lst` opener) is the ack: the command was
    accepted, so `OK` is reported now (`responseAckMore`), but the
    request stays open for the blocks still to come. Each intermediate
    `$--BLOCK <n> / <m>\r\n...\r\n---->` section is shown but not
    correlated (`responseInfo`); the final `$--BLOCK ...\r\nCOM1>`
    section, ending in the real prompt, completes the command with no
    second ack line (`responseDone`). See "Packet framing".
  - `factoryReset`'s `$R: factoryReset: <message>\r\nCOM1>` handled as
    an ordinary ack, completion driven by the packet's own terminating
    prompt. (`factoryReset` ends in the *normal* prompt, not `STOP>`: it
    only marks the config for reset on the next physical power-cycle, so
    it does not halt the command line -- only the reset/halt commands
    such as `exeResetReceiver` terminate in `STOP>`.)
  - The completing ack inherently includes the prompt: the packet is
    not emitted until its terminator (`IsFinal` at the `>`), so there
    is no separate "wait for the prompt" step in the correlator.
  - Two Septentrio requests in sequence: the second is not
    `ReadyToSend` until the first completes (the single-flight
    serialization).
- `satpulsetool gps -m configs/gpsmsg/septentrio/mosaic-g5.toml
  --show-tags` (and `-m mosaic.toml` alone) to validate the TOML still
  parses and every tag/description is well-formed -- no hardware or
  receiver connection required.
- Once hardware arrives: capture a real session for a representative
  tag from each reply shape (`get-version` for a `get*`, `factory-reset`
  for the plain-`$R:`-with-trailing-message `factoryReset`, a manual
  `login` for `$R!` since no tag issues one, and an ordinary `set*`
  tag for the common case), and compare against this design --
  particularly whether
  `$R?`'s `<name>` token behaves as the guide's few examples suggest,
  and whether replies are strictly CRLF-delimited, since both were
  inferred from prose rather than a documented grammar. Use the
  `gps-msg-test` skill for the mechanics.

## Open decisions

- **A few framing details rest on the guide's prose and want a captured
  session to confirm** -- none changes the design, since each degrades
  to a return-to-sync (a re-abandoned packet), not a misattribution:
  that replies are strictly CRLF-delimited (rule 2); that the prompt and
  `lst` pseudo-prompt are exactly `\r\n` + a `[A-Z][A-Z0-9]{3}`-or-`----`
  token + `>`, and that nothing else emits a standalone `\r\n<token>>`;
  and that the 4096-byte cap comfortably clears the longest real reply.
- **`$R?`'s `<name>` token is not modeled or stripped.** The analyzer
  treats the whole `<name>: <error text>` remainder as the nak's
  `ackError`, displayed verbatim. This avoids needing a per-command
  table mapping commands to the config-item name that might appear
  in a rejection, at the cost of the displayed error text sometimes
  naming a config item rather than the command the user typed (e.g.
  rejecting `sso`/`setSBFOutput` shows `SBFOutput:`, not
  `setSBFOutput:` or `sso:`). Acceptable since the text is still
  human-readable and accurate about what was rejected.
