# CASIC configurator ([#229](https://github.com/jclark/satpulse/issues/229))

Implement `gpsprot.ConfigProtocol` and `gpsprot.Configurator` for CASIC receivers (Zhongke Microelectronics), enabling `satpulsetool gps` and `satpulsed` to configure CASIC receivers (probe, enable messages, set time pulse, time mode, signal selection).

Uses [casictool](https://github.com/jclark/casictool) (Python, V5 only) as reference for what configuration packets to send.

## Design constraints

- **Not a port of the UBX configurator.** UBX (`gps/internal/ubx`) was adapted onto the `gpsprot.ConfigDirector` interface from an interface designed for something else and carries baggage. Its shape - a long roster of single-purpose step functions (`valGet`, `valSet`, `setCfg`, `reset`, ...), each producing one request - is not natural; functions can each do more. Compare the Unicore configurator (`gps/internal/unc`), which is completely different: a few `generate*Commands` functions each build a whole batch of commands, and the configurator turns them into requests in one place. Study both to learn the ConfigDirector contract (retries, pausing, multi-response, time windows via `AdvanceTimeTo()`), then design the CASIC shape on its own merits - closer in spirit to unc than to ubx.
- **Fallback is first-class.** The documentation is not good enough to know up front which models support which CFG messages. The configurator makes a reasonable guess that the receiver supports the preferred configuration, sends it, and treats a NAK as an expected, recoverable outcome: fall back to the next best thing and carry on. This is not a condition to surface to the user; it is how configuration normally proceeds.
- **Feature scope is everything staged below:** binary/NMEA output control, PVT and satellite message enabling, nav rate, NVM save/load/reset, time pulse, time mode / survey-in, GNSS and signal-band selection, baud-rate change. Raw-measurement and RTCM output depend on stage 0 evidence of receiver support (see stage 2b).

## V5 vs V6 firmware

Zhongke receivers ship with two significantly different firmware families:

- **V5.x** (5N series, e.g. ATGM332D-5N31): GPS/BDS/GLONASS only, NAV class (0x01) messages, default 9600 baud
- **V6.x** (6N/F8N series, e.g. ATGM332D-6N74): adds Galileo/QZSS/SBAS/IRNSS, NAV2 class (0x11) messages, default 115200 baud

Both use the same packet framing (0xBA 0xCE sync, same checksum) and share most CFG messages. The configurator handles both with a version enum, branching only where needed.

### CFG message compatibility

**V6 has compatible layout with V5:**

| Message | ID | Size | Purpose |
|---------|------|------|---------|
| CFG-MSG | 0x06 0x01 | 4 | Message rate control |
| CFG-PRT | 0x06 0x00 | 8 | Port config (V6 adds RTCM bits, same layout) |
| CFG-RATE | 0x06 0x04 | 4 | Nav update rate (V6 fills reserved field) |
| CFG-CFG | 0x06 0x05 | 4 | Save/load NVM |
| CFG-RST | 0x06 0x02 | 4 | Reset receiver |
| CFG-TP | 0x06 0x03 | 16 | Time pulse (V6 extends enum values compatibly) |

**V5-only:**

| Message | ID | Size | Purpose |
|---------|------|------|---------|
| CFG-NAVX | 0x06 0x07 | 44 | GNSS selection (3-bit mask) + nav engine |
| CFG-TMODE | 0x06 0x06 | 40 | Time mode with R8 ECEF coords |

**V6-only:**

| Message | ID | Size | Purpose |
|---------|------|------|---------|
| CFG-NAVBAND | 0x06 0x0F | 12 | Per-signal bitmask selection |
| CFG-TMODE2 | 0x06 0x16 | 28 | Time mode with scaled I4 ECEF coords |
| CFG-NMEA | 0x06 0x12 | 8 | NMEA output configuration |
| CFG-NAVLIMIT | 0x06 0x0A | 8 | Satellite filtering (min elev, min CNO) |

### Receiver detection and family selection

The probe is an empty-payload CFG-RATE poll. Both families answer it
with a data message and an ACK on the fast CFG lane, so detection
takes ~100 ms and never rests on a NAK (non-CASIC receivers have been
seen NAKing packets that are not theirs). The data message is the
identification; probing succeeds only once the ACK also arrives, so
the probe's CFG transaction is closed before configuration starts and
the configurator's first CFG request honors the one-CFG-in-flight
rule. A retried probe's leftover replies are consumed at the protocol
layer and never reach the configurator's ACK correlation. The
readback selects the family: its byte 2 is fixRateHz on V6, a rate
with no zero value, and reserved-as-zero on V5. The readback also
seeds the configurator, whose rate phase then skips a no-op CFG-RATE
write.

Version and hardware identity come from configurator requests, exempt
from phase gating so the rest of configuration overlaps them: a
MON-VER poll on V6 (the data message is a one-shot arriving at the
receiver's next 1 Hz output tick, tracked through
ConfigRequestMaybeComplete after its ACK; its HwVersion drives the
class-based capability flags, so the message phase waits for it), and
concurrent PCAS06 SW/HW text queries on V5.

This replaced the original design - a CFG-MSG poll of MON-VER, with
V5 assumed on NAK - after the reply-timing measurements (see
"Receiver identification data and reply timing" below): the original
probe rested detection on a NAK and spent ~1 s per V6 detection
waiting for the tick-scheduled MON-VER data.

## Already implemented

- `gps/lib/casbin/ack.go` - ACK-ACK and ACK-NAK
- `gps/lib/casbin/mon.go` - MON-VER with Latin1Z32 strings
- `gps/lib/casbin/common.go` - ParseMsg, Serialize, Poll, PackMsg, Checksum (errata-corrected)
- `gps/lib/casbin/other.go` - all CFG message ID constants
- `gps/lib/casbin/nav.go` - NAV (V5) and NAV2 (V6) navigation messages
- `gps/lib/casbin/tim.go` - TIM-TP
- `gps/internal/casic/` - packet processing and message conversion for both V5 and V6
- `configs/gpsmsg/zhongke/` message files (`atgm332d-v5.toml`, `atgm332d-v6.toml`, `at632.toml`) - provide a basis for much of the configurator's command set, and can be used for experimentation on hardware via `satpulsetool gps -m <file> -t <tag>`

## Stage 0: Hardware validation with message files

Before writing configurator code, validate receiver behaviour on real hardware using `satpulsetool gps -m <file> -t <tag>` with the TOML message files. This catches firmware bugs and protocol misunderstandings early, while the message files are cheap to edit.

**Goals:**
- Verify every CFG message the configurator will use gets ACK (not NAK) on real hardware
- Confirm that CFG-MSG enables actually produce the expected output messages
- Discover firmware quirks (e.g. MON-VER NAK, CFG-TMODE garbage bytes)
- Ensure the TOML message files cover all commands needed by later stages

**Design-shaping unknowns to resolve here** - these may affect the internal shape of the code; document the findings:

- How many configuration requests can be in flight at once? Working rule: multiple in-flight requests are fine, but avoid two whose ACKs would be ambiguous - a CASIC ACK/NAK identifies the request only by class+id, so never have two unacknowledged requests with the same class+id outstanding. This is what `gps/msgfile` already implements (`Correlator.ReadyToSend`). Crucial: before committing to the concurrency model, run tests on both firmware families confirming that pipelining requests with distinct class+id actually works - every request ACKed, nothing dropped or misattributed, including across a mix of sets and polls. If pipelining turns out not to work, the configurator must serialize instead, and that shapes the code.
- How is a response correlated to a request - ACK/NAK by class+id, poll-response echo, or both? How are failures signalled?
- What is the baud-rate-change handshake, and what is its timing? When is it safe to resume sending at the new rate?
- Does re-sending CFG-TMODE/TMODE2 with mode=1 restart an in-progress survey, or is mode=0 then mode=1 needed? (Determines the `SurveyAgain` implementation; see stage 5.)
- Where do V5 and V6 diverge in enum semantics (e.g. CFG-TP polarity, TSrcMode ranges, NMEA message ids)? Confirm against silicon, not just docs.

**V5:** `atgm332d-v5.toml` — verify tags emit the same bytes as casictool (see `casic_hwtest.py`).

**Both:** Check completeness of both TOML files against `configs/gpsmsg/tags.md`.

**V6:** `configs/gpsmsg/zhongke/atgm332d-v6.toml` — hardware validation of untested messages. Key things to validate:
- CFG-TP ppsOutMode values and timeRef/tBase polarity
- CFG-TMODE2 (not yet in TOML file — add and test)
- CFG-NAVBAND signal masks for each constellation
- CFG-NAVLIMIT min elevation
- MON-VER response via CFG-MSG poll
- RXM2/RTCM output support (V6 spec documents these; test whether any receiver we have supports them)

Add any missing message file entries for commands that will be needed. With stage 0 and stage 1 complete, users can already configure receivers manually via `satpulsetool gps -m <file> -t <tag>`, which sends the commands and displays decoded responses.

---

## Stage 1: CFG message structs

Add CFG message structs to `gps/lib/casbin/cfg.go`, registered via `regMsg[]()`.

**Shared structs:**
- `CfgMsg` - ClsID U1, MsgID U1, Rate U2
- `CfgPrt` - PortID U1, ProtoMask U1, Mode U2, BaudRate U4
- `CfgRate` - FixIntervalMs U2, FixRateHz U1, Res U1 (V5 treats first 2 bytes as single U2 interval field)
- `CfgCfg` - Res1 U2, OpMode U1, Res2 U1 (V5 uses first U2 as section mask; V6 reserves it)
- `CfgRst` - NavBbrMask U2, ResetMode U1, StartMode U1
- `CfgTP` - Interval U4, Width U4, PPSOutMode U1, Polarity I1, TBase U1, TSrcMode U1, UserDelay R4
  - PPSOutMode: V6 uses 0-7 (0=off, 1=time non-empty, 2=sat sync, 3=pos+time valid, 5=reliable, 7=always on); V5 subset 0-3 (0=off, 1=on, 2=maintain, 3=fix only)
  - TBase: V6 0=GNSS, 1=UTC; V5 inverted: 0=UTC, 1=satellite time
  - TSrcMode: V6 0-3=force GPS/BDS/GLN/GAL, 4-8=primary, 9=auto; V5 0=GPS, 1=BDS, 2=GLN, 4-6=primary

**V5-only structs:**
- `CfgNavx` - Mask U4, DynModel U1, FixMode U1, MinSVs U1, MaxSVs U1, MinCNO U1, Res1 U1, IniFix3D U1, MinElev I1, DrLimit U1, NavSystem U1, WnRollOver U2, FixedAlt R4, FixedAltVar R4, PDop R4, TDop R4, PAcc R4, TAcc R4, StaticHoldTh R4
- `CfgTMode` - Mode U2, Res U2, EcefX R8, EcefY R8, EcefZ R8, PosVar R4, SvinMinDur U4, SvinVarLimit R4

**V6-only structs:**
- `CfgNavBand` - SigBandAuto U1, Res1 U1, Res2 U2, SigIDMaskFix U4, SigIDMask U4
- `CfgTMode2` - TimFixMode U1, BandMode U1, AntDetMode U1, TSrcMode U1, XFixed I4, YFixed I4, ZFixed I4, FixedPacc U4, SvinMinDur U4, SvinPaccLim U4
- `CfgNmea` - NmeaVer U1, LatLonReso U1, HeightReso U1, GsaPlus U1, NmeaValidOpen U1, Res U1, Res2 U2
- `CfgNavLimit` - MinSVs U1, MaxSVs U1, MinCNO U1, MinElev I1, Res U4

**Tests:** `gps/lib/casbin/cfg_test.go` with round-trip tests.

**Independently useful:** Once CFG structs are registered, `satpulsetool gps decode` can parse CFG responses from receivers (e.g. poll responses, ACK/NAK with class/id). Combined with stage 0 message files, this gives a complete send-and-decode workflow without needing the full configurator.

As each CFG struct is implemented, add the corresponding get-* poll tags to the TOML message files (e.g. `get-pps` to poll CFG-TP, `get-rate` to poll CFG-RATE). These are only useful once decoding works.

---

## Stage 2: ConfigProtocol with probing and NMEA message control

**Files to create:**
- `gps/internal/casic/cascfgprot.go` - ConfigProtocol
- `gps/internal/casic/cascfg.go` - Configurator
- `gps/internal/casic/cascfgmsg.go` - message enabling

**Files to modify:**
- `gps/gpsreg/reg.go` - add to `CreateConfigProtocols()`

**Implement:**
- `ProbePackets()` / `ProbeOK()` - receiver detection (originally a
  MON-VER poll; since redesigned, see "Receiver detection and family
  selection")
- `NativeMsg()` - route ACK/NAK and CFG responses to Configurator
- `Configure()` - create Configurator with version info
- Step: `setMsg` for `--nmea-out` and `--binary`/`--nmea` flags via CFG-MSG:
  - All use class 0x4E with per-sentence IDs
  - GGA=0x00, GLL=0x01, GSA=0x02, GSV=0x03, RMC=0x04, VTG=0x05 (same on V5 and V6)
  - ZDA differs: V6=0x06, V5=0x08 (per `casic-fm.md` section 1.4)
  - `--binary` disables NMEA output; `--nmea` enables NMEA and disables binary
  - Protocol enable/disable via CFG-PRT protoMask bits

**Functionality:** `satpulsetool gps --binary --nmea --nmea-out RMC,GGA` works with CASIC receivers.

---

## Stage 2a: PVT message enabling

Step: `setMsg` for `--pvt-out` flags via CFG-MSG (class, id, rate).

CASIC native messages for PVT:
- navPv: V5 NAV-PV (0x01 0x03), V6 NAV2-PVH (0x11 0x03) — geodetic pos+vel, fix quality
- navSol: V5 NAV-SOL (0x01 0x02), V6 NAV2-SOL (0x11 0x02) — ECEF pos+vel, TAI time (week+TOW), fix quality+PDOP
- navTimeUTC: V5 NAV-TIMEUTC (0x01 0x10), V6 NAV2-TIMEUTC (0x11 0x05) — UTC time
- navDop: V5 NAV-DOP (0x01 0x01), V6 NAV2-DOP (0x11 0x01) — full DOP set (PDOP, HDOP, VDOP, TDOP)
- timTP: TIM-TP (0x02 0x00) on both — time of next pulse (TAI from week+TOW)

Flag-to-message logic:

```
if tp:           timTP = true
if tp && after:  flags |= time    // ensure a time msg follows the pulse

if time && TAI:  navSol = true    // TAI is a modifier: use SOL instead of TIMEUTC
else if time:    navTimeUTC = true

if (pos || vel) && ecef:  navSol = true   // ecef is a modifier: use SOL instead of PV
else if pos || vel:       navPv = true

if qual:  navSol = true; navDop = true    // fix level from SOL, full DOPs from DOP
```

Not supported: `leap`, `epoch` (no CASIC equivalents), `survey` (see stage 5).

Also `setRate` - CFG-RATE for navigation update rate. Message rates in CFG-MSG are per-fix, so 1Hz output requires the fix rate to be at least 1Hz (same as UBX).

**Functionality:** `satpulsetool gps --pvt-out pos,time,tp` works.

---

## Stage 2b: Satellite message control

Step: `setMsg` for `--sats-out` flags via CFG-MSG:
- V5: NAV-GPSINFO (0x01 0x20) + NAV-BDSINFO (0x01 0x21) + NAV-GLNINFO (0x01 0x22) for `sat`; no `signal` support
- V6: NAV2-SIG (0x11 0x06) provides both satellite and per-signal info for `sat` and `signal`

**Functionality:** `satpulsetool gps --sats-out sat,signal` works.

Note: `--raw-out` and `--rtcm-out` deferred pending stage 0 testing of whether any receiver we have supports RXM2/RTCM output.

---

## Stage 3: Persistent NVM operations

Steps using shared CFG messages (identical for V5 and V6):
- `setCfg` - CFG-CFG for save/load NVM
- `reset` - CFG-RST for reset

**Functionality:** `satpulsetool gps --save --save-all --reset --reload --factory-reset` works.

### V6 reset behaviour (characterized on hardware)

Characterized on AT632 (2026-06-14) by dirtying config across several
dimensions (min-elev, PPS width, NMEA-output set, a binary message via
`--pvt-out`) and observing the result of each reset via `--show-config`
and a post-reset `--capture`. All three V6 resets *restart* the receiver
(boot banner + dropped fix), so the test harness must allow settle time
after them.

| Command | binary cmd | config effect |
|---------|-----------|---------------|
| `--reset` | CFG-RST cold | preserves config (correct cold-start) |
| `--factory-reset` | CFG-RST factory | resets all config to defaults |
| `--reload` | CFG-CFG opMode 2 | broken: keeps RAM, does not load flash |

Binary `--factory-reset` produces a config byte-for-byte equivalent to
the NMEA `PCAS10,3` factory start, so it is a complete reset. An earlier
assumption that the binary resets were no-ops on V6 (and that PCAS10 was
the only working path) was wrong: only `--reload` is broken. The replay
harness accordingly uses `reset_args="--factory-reset"` for the V6
receiver stubs to start each scenario from a clean factory baseline; no
NMEA message file is needed. The broken `--reload` is surfaced to the
user via a capability flag; see "Follow-ups (implemented)".

---

## Stage 4: Time pulse configuration

Steps:
- `pollTP` - poll CFG-TP to read current settings
- `setTP` - CFG-TP to configure time pulse (shared struct, compatible enums)

Map `gpsprot.TimePulse` to `casbin.CfgTP`:
- Width -> CfgTP.Width (microseconds)
- Period -> CfgTP.Interval (microseconds)
- PolarityRising -> CfgTP.Polarity (0=rising, 1=falling)
- OnlyWhenLocked -> CfgTP.PPSOutMode (V5: 0=off, 3=fix only; V6: 5=reliable, 7=always on)
- AlignToGNSS -> CfgTP.TBase (V6: 0=GNSS, 1=UTC; V5 inverted: 0=UTC, 1=satellite time)
- TimeGNSS -> CfgTP.TSrcMode (V6: 0-3=force GPS/BDS/GLN/GAL, 4-8=primary, 9=auto; V5: 0=GPS, 1=BDS, 2=GLN, 4-6=primary)

**Functionality:** `satpulsetool gps --pps --time-gnss` works.

---

## Stage 5: Time mode configuration

Steps (version-branching):
- V5: poll/set CFG-TMODE (R8 ECEF coords in meters, R4 variance in m^2)
- V6: poll/set CFG-TMODE2 (I4 ECEF coords scaled 0.01m, U4 accuracy in mm)

Mode values: 0=auto/real-time, 1=survey-in, 2=fixed position (same for both).

Errata: CFG-TMODE mode field upper 2 bytes contain garbage; parse as U2.

The `satpulsed` default path (no fixed position, not mobile) sets `SetStatic=true`. Follow the same logic as UBX (`ubxcfgtmode.go`): preserve existing fixed position, don't restart an existing survey unless `SurveyAgain` is set. Note: whether `SurveyAgain` is implementable (i.e. whether the receiver can be forced to restart a survey) needs to be determined in stage 0.

**Functionality:** `satpulsetool gps --survey --fixed-pos-ecef --mobile` works. `satpulsed` default static mode triggers survey-in.

---

## Stage 6: GNSS signal selection

Steps (version-branching):
- V5: poll/set CFG-NAVX with navSystem bitmask (GPS=0x01, BDS=0x02, GLN=0x04; mask bit 0x0100)
- V6: poll/set CFG-NAVBAND with per-signal bitmask (24-bit signal mask)

V5 is constellation-level only. V6 has per-signal granularity.

**Functionality:** `satpulsetool gps --gnss --band` works.

---

## Stage 7: Baud rate change

Step: `setPrt` - CFG-PRT to change baud rate. This is separate because the serial port speed changes mid-stream, requiring `ConfigRequest.GetSpeedChangeAfter()` and `MaybeSpeedChangeSucceeded()` handling. Must be one of the last steps since communication breaks if the speed change fails.

**Functionality:** `satpulsetool gps --speed 115200` works.

---

## Verification

- Every CFG the configurator sends to a real receiver (both firmware families) is acknowledged - or handled by a documented fallback - and the resulting receiver state matches intent.
- Deterministic offline tests using a CASIC fake-receiver test double driven through the real `PacketProcessor` -> `ConfigProtocol` -> `ConfigDirector` path, mirroring `gps/gpsprot/configprotocol_test.go` and the ubx/unc configurator tests.
- Round-trip serialize/parse tests for every new CFG struct in `gps/lib/casbin`.

## Follow-ups (implemented)

Two capability flags were added after the core configurator landed. Each
lets a receiver advertise whether it supports an option, so `gpscmd`
warns (via the `req.require` / `warnMissingConfigSupport` machinery)
rather than failing silently when the option needs a capability the
receiver lacks.

- **`ConfigSupportReload`**. `--reload` maps to `gpsprot.ResetReload`,
  but on V6 the CFG-CFG opMode 2 ("load FLASH to current") is ACKed and
  never applied - confirmed on hardware (with NAV2-PVH enabled in RAM and
  *disabled* in flash, `--reload` restarted the receiver yet kept
  emitting NAV2-PVH, so flash was not loaded). It is the only broken V6
  reset, and the only one without a PCAS equivalent (see "V6 reset
  behaviour" under stage 3); binary `--reset` and `--factory-reset` work.
  u-blox, Unicore, and CASIC V5 set the flag; CASIC V6 omits it and
  treats `ResetReload` as a true no-op instead of sending the ineffective
  command.
- **`ConfigSupportPort`**. `--show-port` names the active port, but CASIC
  cannot identify which UART is active, so it omits the flag and
  `--show-port` warns. The serial speed is independent of port
  identification and is reported regardless: CASIC polls CFG-PRT and
  reports the wired UART's baud. u-blox sets the flag.

## Receiver identification data and reply timing (measured)

How the identification mechanisms behave on the two firmware
families, with the values observed on the four attached units. This
is a record of protocol facts and measured responses only; it drove
the probe redesign described under "Receiver detection and family
selection".

Units measured:

| Device | Module / chip | Firmware | Family | Speed |
|--------|---------------|----------|--------|-------|
| ttyUSB0 | ATGM332D-5N31 / AT6558D | URANUS5 V5.3.0.0 | V5 | 9600 |
| ttyUSB1 | AT372-AT6668-6P-34 | URANUS6 V6.2.3.0 | V6 | 115200 |
| ttyUSB2 | ATGM332D-AT9880-F8N-76 | URANUS6 V6.3.2.0 | V6 | 115200 |
| ttyUSB3 | AT362-AT6668-6T-30 | URANUS6 V6.3.0.0 | V6 | 115200 |

### The MON-VER poll and PCAS06 query mechanisms

The binary MON-VER poll is sent as a CFG-MSG (class 0x06, id 0x01)
whose payload targets MON-VER (class 0x0A, id 0x04) at rate 0xFFFF
(output once). Exact bytes:

```
ba ce 04 00 06 01 0a 04 ff ff 0e 04 05 01
   sync  len   cls/id  tgt   rate  checksum
```

A PCAS06 query is an NMEA sentence `$PCAS06,<info>*cs`. Each query
produces one `$GPTXT,01,01,02,<KEY>=<value>*cs` reply; there is no
burst and no query-all value. On V6, identical queries repeated
within one output cycle coalesce into a single reply (see Reply
timing).

Info values and their reply keys:

| info | key | meaning | documented |
|------|-----|---------|------------|
| 0 | SW | firmware family + version | V5, V6 |
| 1 | HW | hardware model + serial | V5, V6 |
| 2 | MO | enabled systems (working mode) | V5, V6 |
| 3 | CI / UI | id value | CI on V5; UI on V6 (undocumented) |
| 4 | SM | receivable signal bands + systems | V6 |
| 5 | BS | bootloader / upgrade code | V5 |
| 6 | IC | chip designation + serials | V6 (answers on V5, undocumented there) |

### MON-VER behaviour

V6 answers the CFG-MSG poll with an ACK-ACK and a MON-VER data
message. The two come from different scheduling lanes and their
order is phase-dependent (see Reply timing below); the data message
precedes the ACK only when the poll lands just before a 1 Hz output
tick. The payload is 64 bytes: two null-padded 32-byte latin1
strings, SwVersion (bytes 0-31) then HwVersion (bytes 32-63).
SwVersion carries a literal `SW=` prefix; HwVersion has no prefix.

Raw reply (ttyUSB1):

```
ba ce 40 00 0a 04
53 57 3d 55 52 41 4e 55 53 36 2c 56 36 2e 32 2e 33 2e 30 00...  "SW=URANUS6,V6.2.3.0"
41 54 33 37 32 2d 41 54 36 36 36 38 2d 36 50 2d 33 34 00...     "AT372-AT6668-6P-34"
aa 4d 1f 24
```

Observed values (keys as they appear on the wire):

| device | SwVersion | HwVersion |
|--------|-----------|-----------|
| ttyUSB1 | SW=URANUS6,V6.2.3.0 | AT372-AT6668-6P-34 |
| ttyUSB2 | SW=URANUS6,V6.3.2.0 | ATGM332D-AT9880-F8N-76 |
| ttyUSB3 | SW=URANUS6,V6.3.0.0 | AT362-AT6668-6T-30 |

V5 does not support MON-VER: it answers ACK-NAK to the CFG-MSG poll,
and no version data is returned.

### PCAS06 responses

V5 (ttyUSB0):

```
$GPTXT,01,01,02,SW=URANUS5,V5.3.0.0
$GPTXT,01,01,02,HW=AT6558D,0000000000000
$GPTXT,01,01,02,MO=GB
$GPTXT,01,01,02,CI=01B94154
$GPTXT,01,01,02,BS=SOC_BootLoader,V6.2.0.2
$GPTXT,01,01,02,IC=AT6558D-5N-32-1C520900,AJ03DHL-C1-002138
```

info 4 (SM) produced no reply.

ttyUSB1 (AT372-AT6668-6P-34):

```
$GPTXT,01,01,02,SW=URANUS6,V6.2.3.0
$GPTXT,01,01,02,HW=AT372,0004040600626
$GPTXT,01,01,02,MO=GBEQ
$GPTXT,01,01,02,SM=00080C81,GPS,BD2,GAL,QZS
$GPTXT,01,01,02,BS=BOOT8A,V8.0.1.0
$GPTXT,01,01,02,IC=AT6668-6P-34-00000A30,EA05A3J-22-438091959
```

info 3 produced no reply on this unit.

ttyUSB2 (ATGM332D-AT9880-F8N-76):

```
$GPTXT,01,01,02,SW=URANUS6,V6.3.2.0
$GPTXT,01,01,02,HW=ATGM332D,0032519800024
$GPTXT,01,01,02,MO=GBE
$GPTXT,01,01,02,SM=0000CD85,GPS,BD2,BD3,GAL
$GPTXT,01,01,02,BS=BOOT8V,V8.0.5.1
$GPTXT,01,01,02,IC=AT9880-F8N-76-E1000C41,EG49B3J-33-496202618
$GPTXT,01,01,02,UI=00146085
```

ttyUSB3 (AT362-AT6668-6T-30):

```
$GPTXT,01,01,02,SW=URANUS6,V6.3.0.0
$GPTXT,01,01,02,HW=AT362,0005117200485
$GPTXT,01,01,02,MO=GBQ
$GPTXT,01,01,02,SM=00080C01,GPS,BD2,QZS
$GPTXT,01,01,02,BS=BOOT8A,V8.0.3.0
$GPTXT,01,01,02,IC=AT6668-6T-30-00000A30,EA05A3J-21-438072726
$GPTXT,01,01,02,UI=00C84014
```

Field formats observed:

- SW: firmware family and version; contains URANUS5 or URANUS6.
- HW: `<model>,<serial>`. On V6 the model is the module/board name; on
  V5 it is the chip (AT6558D). The V5 serial was all-zeros.
- IC: `<chip-designation>-<8hex-serial>,<production-serial>`. The
  designation carries the variant/grade suffix (`-6P-34`, `-F8N-76`,
  `-6T-30`, `-5N-32`).
- MO: enabled systems (G=GPS, B=BDS, R=GLONASS, E=GALILEO, Q=QZSS,
  N=IRNSS, S=SBAS). Reflects current configuration.
- SM: `<hex band mask>,<system list>`. Receivable signal bands.
  Reflects current configuration. Not answered by V5.
- BS: bootloader / upgrade code and version.
- CI (V5) / UI (V6, info 3): an id value. CI is documented (customer
  id); UI is undocumented and appeared on two of three V6 units.

### Relationships between MON-VER and PCAS on V6

- SW equals MON-VER.SwVersion byte-for-byte, including the `SW=` prefix.
  PCAS TXT-SW is the same string delivered over NMEA.
- MON-VER.HwVersion is `<HW model>` + `-` + `<IC designation>`, with the
  serials removed and no prefix:

  | HW model | IC designation | MON-VER.HwVersion |
  |----------|----------------|-------------------|
  | AT372 | AT6668-6P-34 | AT372-AT6668-6P-34 |
  | ATGM332D | AT9880-F8N-76 | ATGM332D-AT9880-F8N-76 |
  | AT362 | AT6668-6T-30 | AT362-AT6668-6T-30 |

- MON-VER carries no serial numbers; HW and IC do.

On V5 the HW model (`AT6558D`) is a prefix of the IC designation
(`AT6558D-5N-32`): PCAS HW reports the chip, and IC reports the same
chip with its variant suffix. On V6 the HW model and IC designation
are distinct names, which is why MON-VER.HwVersion concatenates them.

### Reply timing

Measured 2026-07-23 on all four units: floods of 30 back-to-back CFG
polls (CFG-PRT/CFG-TP/CFG-RATE), send-on-ACK paced streams of the
same, five MON-VER polls in a row, and repeated and mixed PCAS06
queries.

V5 (ATGM332D-5N31) has one path with immediate replies. Every input -
CFG request or PCAS06 query - is processed on arrival and its reply
(data + ACK, NAK, or GPTXT) emitted immediately. Nothing coalesces:
five identical PCAS06 queries produce five replies, five MON-VER
polls five NAKs. A 30-poll flood loses nothing; its apparent
ACK-latency ramp is the 9600 line draining the response burst. All
delay ever observed on this unit is TX-queue drain: at the default
9600 baud under the default NMEA load (~1050 bytes/s offered against
~960 capacity) replies queue behind the backlog for seconds (measured
up to ~4.2 s, none lost in 28 queries), while on a quiet line they
arrive in tens of milliseconds.

V6 (AT372-6P, ATGM332D-F8N, AT362-6T) has two lanes.

CFG transaction lane. CFG requests are queued (at least 30 deep; a
30-poll flood loses nothing on any unit) and serviced on a roughly
100 ms quantum: a send-on-ACK stream settles at one ACK per ~100 ms,
flat, on all three units. An empty-payload CFG query's readback and
ACK arrive together in one service slot, data first; only the CFG-MSG
one-shot request (rate 0xFFFF) - a set whose requested output is
delivered by the tick scheduler - has its data trail the ACK. Under a
flood, ACK latency grows with queue depth - emitted in batches ~40 ms
apart on the AT372-6P and ATGM332D-F8N, streamed continuously on the
AT362-6T - so ACK delay is the backpressure signal. The ACK closes
the transaction: further CFG requests are accepted and serviced while
a one-shot output is still pending (four transactions completed
inside one ACK-to-data window).

Output lane. One-shot outputs (CFG-MSG rate 0xFFFF, e.g. MON-VER) and
GPTXT query replies are not queued per request: each request sets a
pending flag, and the 1 Hz output cycle's generation instant emits
every pending item in one batch and clears the flags. Consequently:

- Latency is uniform 0-1 s, set purely by the request's phase
  relative to the tick.
- Identical repeated requests coalesce: five MON-VER polls yield one
  data message (all three units); five identical PCAS06 queries
  yielded one reply on the AT362-6T and two, on consecutive ticks, on
  the AT372-6P and ATGM332D-F8N (a query landing after the generation
  instant re-arms the flag for the next tick).
- Distinct queries sent together are answered together in one tick
  when all arrive before the generation instant, and split across
  ticks otherwise (observed once each on the ATGM332D-F8N and
  AT362-6T).

ACK/data order is phase, not firmware: the ACK comes at the next
~100 ms service slot and the data at the next 1 Hz tick, so the data
precedes the ACK only when the request lands just before a tick.

### Probe phase

gpscfg sends its probe immediately after the first received packet,
and the receiver's periodic output burst marks the tick - so the
probe lands just after a tick and output-lane replies (MON-VER data,
GPTXT replies) take nearly the full second. In the recorded capture
sessions the MON-VER data delay concentrates hard at ~1.01 s
(p90 = median over ~120 samples). Probes triggered by the silence
timer instead land at random phase. The CFG-RATE probe's own reply is
on the fast CFG lane, so detection is phase-independent; the phase
lock now affects only the V6 identity readback.

## Known errata

1. **Checksum byte order**: already handled in `casbin.Checksum()`
2. **MON-VER**: V5 NAKs the poll and returns no version data; V5
   identity comes from PCAS06 queries instead
3. **CFG-MSG query**: empty payload returns all rates, not just one
4. **CFG-TMODE mode field**: upper 2 bytes contain unknown values; parse as U2 + U2 reserved

## Reference materials

- Python prototype (V5 only): https://github.com/jclark/casictool (`casic.py`, `connection.py`, `job.py`)
- V5 protocol spec (`casic2.md`), V6 protocol spec (`zkw3.md`), firmware manual with NMEA sentence IDs and PCAS commands (`casic-fm.md`): in the local gps-protocol-docs collection, not committed here
- Errata/notes: https://github.com/jclark/casictool (`spec/notes.md`)
- UBX and Unicore configurators (gpsprot interface implementations): `gps/internal/ubx`, `gps/internal/unc`
- TOML message files: `configs/gpsmsg/zhongke/atgm332d-v5.toml`, `configs/gpsmsg/zhongke/atgm332d-v6.toml`
