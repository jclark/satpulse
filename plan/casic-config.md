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

### Version detection

1. Poll MON-VER via CFG-MSG (class=0x0A, id=0x04, rate=0xFFFF) — this is the standard CASIC poll mechanism
2. V6: ACK + MON-VER response; parse SwVersion string
3. V5: NAK (MON-VER not supported); assume V5

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
- `ProbePacket()` / `ProbeOK()` - MON-VER based detection with fallback
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
NMEA message file is needed. The broken `--reload` is the subject of a
follow-up; see "Things still to do".

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

## Things still to do

The configurator is implemented and exercised by replay tests, but the
following items remain. They are follow-ups under #229.

### 1. Surface an unsupported `--reload` via a `ConfigSupportReload` flag

`--reload` maps to `gpsprot.ResetReload` ("reload the configuration from
non-volatile memory without a reset/start"). On CASIC V6 (URANUS6) this
is a firmware no-op, confirmed on hardware (AT632, 2026-06-14): CFG-CFG
opMode 2 ("load FLASH config to current") is acknowledged but never
applied. With NAV2-PVH enabled in RAM and *disabled* in flash, `--reload`
restarted the receiver yet left NAV2-PVH still emitting - so flash was
not loaded into RAM. The only observable effect is that the GNSS engine
restarts *without* loading config (dropping the time lock). So `--reload`
on a V6 receiver silently does nothing useful and the user gets no
signal. (CASIC V5 URANUS5 reload works in place; u-blox and Unicore
reload work.)

`--reload` is the *only* broken V6 reset, and notably the only one
without a PCAS equivalent (see "V6 reset behaviour" under stage 3) -
binary `--reset` and `--factory-reset` work correctly.

Add a capability flag and warn on use when it is absent:

1. `gps/gpsprot/configprotocol.go`: add `ConfigSupportReload` to
   `ConfigSupportFlags` (append after `ConfigSupportRTCMQZSS`; bump
   `ConfigSupportLast`) and a `{ConfigSupportReload, "reload"}` entry in
   the flag-name table so it appears on the `Supports:` line.
2. Each backend declares it. u-blox, Unicore, and CASIC V5 set it; CASIC
   V6 does not - the firmware family already gates capabilities, so V6's
   support set simply omits it.
3. `internal/gpscmd`: `req.require(ConfigSupportReload, "--reload")` so
   `warnMissingConfigSupport` emits a warning when `--reload` is used on
   a receiver that lacks the flag. The command still proceeds for any
   other requested operations.
4. CASIC V6 `ResetReload` becomes a true no-op: the configurator must
   not issue the ineffective CFG-CFG opMode 2. With (3)'s warning,
   `--reload` on V6 is then an honest, documented no-op rather than a
   silent restart.

Notes: the `req.require` / `warnMissingConfigSupport` machinery already
implements "this option needs capability X; warn if absent" (see
`TestWarnMissingConfigSupport`). The decision is WARNING + no-op, not an
error, so combined invocations still succeed. The `Supports:` line
vocabulary gains `reload`; update the man page
(`docs/man/satpulsetool-gps.1.md`) and any Supports-line tests. Testing:
unit test that an omitting support set triggers the warning and that the
`Supports:` line shows `reload` only where present; gpscmd replay test
that `--reload` on a V6 receiver warns and emits no CFG-CFG opMode 2.

## Known errata

1. **Checksum byte order**: already handled in `casbin.Checksum()`
2. **MON-VER**: V5 NAKs the poll; assume V5 on NAK
3. **CFG-MSG query**: empty payload returns all rates, not just one
4. **CFG-TMODE mode field**: upper 2 bytes contain unknown values; parse as U2 + U2 reserved

## Reference materials

- Python prototype (V5 only): https://github.com/jclark/casictool (`casic.py`, `connection.py`, `job.py`)
- V5 protocol spec (`casic2.md`), V6 protocol spec (`zkw3.md`), firmware manual with NMEA sentence IDs and PCAS commands (`casic-fm.md`): in the local gps-protocol-docs collection, not committed here
- Errata/notes: https://github.com/jclark/casictool (`spec/notes.md`)
- UBX and Unicore configurators (gpsprot interface implementations): `gps/internal/ubx`, `gps/internal/unc`
- TOML message files: `configs/gpsmsg/zhongke/atgm332d-v5.toml`, `configs/gpsmsg/zhongke/atgm332d-v6.toml`
