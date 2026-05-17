# Configuration support flags (#203)

Issue #203 should be implemented as one compact bitset that describes
what the SatPulse configuration engine can configure for the probed
receiver/protocol.  The bitset is not a complete hardware capability
model.  It is a warning and UI-enablement surface for the options that
SatPulse actually exposes.

The support value must be cheap to compute from the normal receiver
probe.  For u-blox this means `MON-VER` plus data already available in
the current probe path.  For Unicore this means `VERSIONB`.

Related: #126 and #238 need MSM4-vs-MSM7 and QZSS MSM information for
automatic NTRIP STR records.

## Data model

Add a single field to `gpsprot.ReceiverInfo`:

```go
type ConfigSupportFlags uint32

const (
	ConfigSupportBands ConfigSupportFlags = 1 << iota
	ConfigSupportSpeed
	ConfigSupportSurvey
	ConfigSupportSurveyAcc
	ConfigSupportSurveyMsg
	ConfigSupportFixedPos
	ConfigSupportFixedPosAcc
	ConfigSupportRaw
	ConfigSupportRTCMMSM4
	ConfigSupportRTCMMSM7
	ConfigSupportRTCMBaseID
	ConfigSupportRTCMQZSS
)

const ConfigSupportRTCMMSM = ConfigSupportRTCMMSM4 | ConfigSupportRTCMMSM7
```

`ReceiverInfo` gets:

```go
ConfigSupport ConfigSupportFlags `json:"configSupport,omitempty"`
```

`gpsprot` owns the string representation:

```go
func (f ConfigSupportFlags) Items() []string
func (f ConfigSupportFlags) String() string
func (f ConfigSupportFlags) MarshalJSON() ([]byte, error)
```

`Items` returns the stable CLI-facing names in declaration order.
`String` joins `Items` with `", "`.  `MarshalJSON` serializes the
same item list, so JSON consumers see:

```json
["bands", "speed", "fixedPos", "raw"]
```

The TypeScript generator should expose `ReceiverInfo.configSupport`
as a string list.  The desktop GUI can use those names directly rather
than depending on the numeric bit values.

## Flag meanings

`ConfigSupportBands`
: The receiver/config protocol supports meaningful non-L1 band
selection.  This is the flag for `--band` and the desktop signal
picker's band-level controls.  It does not replace `SupportedGNSS`;
constellation availability still comes from `ReceiverInfo.SupportedGNSS`.

`ConfigSupportSpeed`
: The configuration engine can change the receiver serial speed.  This
is false for Unicore until speed changes are implemented there.

`ConfigSupportSurvey`
: Survey-in mode can be configured.

`ConfigSupportSurveyAcc`
: Survey-in target accuracy can be configured.  This is separate from
`ConfigSupportSurvey` because Unicore currently supports survey time
but has no equivalent target-accuracy parameter in `MODE BASE TIME`.

`ConfigSupportSurveyMsg`
: Survey progress output can be configured.  This maps to
`PVTMsgSurvey`, including timing presets that include survey progress.

`ConfigSupportFixedPos`
: Fixed position mode can be configured.  This deliberately does not
say whether the native receiver format is LLH or ECEF; SatPulse can
convert.

`ConfigSupportFixedPosAcc`
: Fixed position accuracy can be configured.  This is separate because
Unicore fixed position mode currently does not configure the accuracy
field.

`ConfigSupportRaw`
: Raw observation/navigation message output can be configured.  Do not
split this into observation and navigation flags unless a supported
receiver creates a real distinction.

`ConfigSupportRTCMMSM4`
: RTCM MSM4 output can be configured.

`ConfigSupportRTCMMSM7`
: RTCM MSM7 output can be configured.

`ConfigSupportRTCMBaseID`
: The RTCM reference station ID can be configured.

`ConfigSupportRTCMQZSS`
: QZSS RTCM MSM output can be configured.  This is not implied by
`ConfigSupportRTCMMSM*` plus QZSS support: u-blox supports QZSS
tracking but the protocol docs and current config tables do not expose
QZSS MSM output keys.

There is no separate `ConfigSupportRTCM` bit.  Code that needs to know
whether any RTCM MSM output is supported should test
`ConfigSupportRTCMMSM`.

## Non-flags

Do not add flags for these in the first implementation:

- Static/mobile.  On u-blox this is dynamic model or time mode policy,
  not an independent support capability.  The real time-mode
  capabilities are survey and fixed position.
- Fixed position LLH/ECEF.  Expose one `FixedPos` flag and convert
  coordinates in the backend as needed.
- GPS/GLO/GAL/BDS RTCM support.  For SatPulse's purposes, RTCM MSM
  support plus `SupportedGNSS` is enough for the major constellations.
  QZSS is the exception because u-blox is different.
- NMEA, basic PVT, satellite output, time pulse, time GNSS, antenna
  cable delay, minimum elevation, save, reload, reset, and factory
  reset.  These are either broadly supported by the current config
  engines or do not have enough receiver-specific behavior to justify
  a warning flag now.
- `SatsMsgSignal`, PVT TAI, PVT ECEF preference, and PVT epoch.  Add a
  flag later only if a concrete user-facing warning or desktop control
  needs it.

## u-blox computation

Add a helper on `ubx.Version`, for example:

```go
func (v *Version) configSupport() gpsprot.ConfigSupportFlags
```

The helper should use the same version-derived knowledge already used
by the configurator:

- `Speed`: true for UBX configuration.  Whether the current port is a
  UART remains a current-config/readback concern.
- `Survey`: `tmodeLevel() > 0`.
- `SurveyAcc`: `tmodeLevel() > 0`.
- `SurveyMsg`: protocol version at least 15 and `tmodeLevel()` is 2 or
  3.  Legacy TMODE receivers such as LEA-6T are not a priority for
  survey progress messages.
- `FixedPos`: `tmodeLevel() > 0`.
- `FixedPosAcc`: `tmodeLevel() > 0`.
- `Raw`: `rawLevel() > 0`.
- `RTCMMSM4`, `RTCMMSM7`, and `RTCMBaseID`: from `rtcmSupport()`.
  `RTCMBaseID` follows `df003Out`.
- `RTCMQZSS`: false for now.  The F9 and X20 protocol docs list
  GPS/GLO/GAL/BDS MSM output keys but not QZSS 1114/1117, and
  `ubxcfgmsg.go` has no QZSS RTCM message keys.
- `Bands`: conservative model/version logic.  True for known multiband
  families such as F9, F10, X20, and product categories that explicitly
  contain non-L1 bands.  False for known L1-only families such as M8
  timing modules, LEA-6T, and unknown receivers.

Set `ReceiverInfo.ConfigSupport` in `(*ubx.Configurator).ReceiverInfo`.

## Unicore computation

Add a helper based on `uncmsg.Version.Type`, for example:

```go
func configSupport(ver *uncmsg.Version) gpsprot.ConfigSupportFlags
```

Use a conservative model table.  For known Nebula IV models supported
by the current config engine, start with:

- `Bands`: true.
- `Speed`: false.
- `Survey`: true.
- `SurveyAcc`: false.  `MODE BASE TIME [T] [Distance]` has an optional
  distance parameter, but that is a saved-position reuse threshold, not
  a survey target accuracy.  Do not map SatPulse survey accuracy to it.
- `SurveyMsg`: false.
- `FixedPos`: true.
- `FixedPosAcc`: false.
- `Raw`: true.
- `RTCMMSM4`: true.
- `RTCMMSM7`: true.
- `RTCMBaseID`: true.
- `RTCMQZSS`: true.

For unknown Unicore product models, be conservative: expose only flags
that are known to be protocol-wide, or none if that is unclear.  This
can be loosened as hardware is tested.

Set `ReceiverInfo.ConfigSupport` in `(*unc.Configurator).ReceiverInfo`.

## satpulsetool gps requirements

`internal/gpscmd/gpsflags.go` should return or record the
`ConfigSupportFlags` required by the user's requested configuration.
This requirement set is compared with `ReceiverInfo.ConfigSupport` to
produce warnings for missing support.

Initial mapping:

- `--band`: `ConfigSupportBands`.
- `--speed`: `ConfigSupportSpeed`.
- `--survey`: `ConfigSupportSurvey`.
- Explicit `--survey-acc`: `ConfigSupportSurveyAcc`.
- `--pvt-out survey`, `--pvt-out ptp`, `--pvt-out ntp`: `ConfigSupportSurveyMsg`.
- `--fixed-pos-ecef`: `ConfigSupportFixedPos`.
- Explicit `--fixed-pos-acc`: `ConfigSupportFixedPosAcc`.
- `--raw-out obs` or `--raw-out nav`: `ConfigSupportRaw`.
- `--rtcm-out MSM4`: `ConfigSupportRTCMMSM4`.
- `--rtcm-out MSM7`: `ConfigSupportRTCMMSM7`.
- `--rtcm-out auto`: `ConfigSupportRTCMMSM`.
- `--rtcm-out ARP`: `ConfigSupportRTCMMSM`.
- `--rtcm-base-id`: `ConfigSupportRTCMBaseID`.
- RTCM MSM output with explicitly requested QZSS: `ConfigSupportRTCMQZSS`.

Options that disable output, such as `--raw-out none` or
`--rtcm-out none`, require no support.

Track explicitness for accuracy flags.  The parser currently supplies
defaults for survey and fixed position accuracy; those defaults should
not by themselves produce missing-support warnings.  Warn only when the
user explicitly asks to configure the accuracy value, or when the GUI
sends that field as an intentional edit.

For non-lax MSM requests, warn when the exact requested level is
missing.  For lax/auto requests, warn only if neither MSM4 nor MSM7 is
supported.

When receiver information is printed, whether `--show-receiver` was
explicit or implied by a no-op `satpulsetool gps` invocation, print
the support flags as a single stable comma-separated line:

```text
Supports: bands, speed, survey, surveyAcc, surveyMsg, fixedPos, fixedPosAcc, raw, rtcmMSM4, rtcmMSM7, rtcmBaseID, rtcmQZSS
```

Only include names for flags that are set, and omit the line when the
support set is zero.  Keep the names close to the CLI concepts rather
than Go constant names.  Use `ConfigSupportFlags.String()` for this
line so CLI output, JSON tests, and future UI/debug output share the
same ordering and names.

## Desktop GUI branch

The desktop GUI branch currently has a single configuration panel with
controls for time mode, survey-in parameters, fixed position, signals,
message groups, and serial speed.  The proposed flags have enough
granularity for these controls without becoming a receiver capability
taxonomy:

- `Bands` covers the signal picker/band-selection distinction from
  the constellation checkboxes.
- `Speed` covers the serial speed selector.
- `Survey`, `SurveyAcc`, and `SurveyMsg` cover the survey-in mode
  controls and the "Report survey progress" option.
- `FixedPos` and `FixedPosAcc` cover fixed position and its accuracy
  field, without exposing native LLH/ECEF support.
- `Raw` covers the raw message group.
- `RTCMMSM4`, `RTCMMSM7`, `RTCMBaseID`, and `RTCMQZSS` cover the RTCM
  group and NTRIP STR needs.

The GUI should consume `ReceiverInfo.configSupport` from the existing
receiver event/readback flow.  It should not need protocol-specific
tables in frontend code.

## NTRIP STR use

The NTRIP caster/server work can use the same bitset:

- MSM4 available: `ConfigSupportRTCMMSM4`.
- MSM7 available: `ConfigSupportRTCMMSM7`.
- QZSS MSM available: `ConfigSupportRTCMQZSS`.

The generated STR record should still combine this with
`ReceiverInfo.SupportedGNSS`, because support for an MSM level does not
mean every constellation is present on that receiver.

## Tests

Add focused unit tests rather than broad integration tests:

- `gps/gpsprot`: `ConfigSupportFlags.Items`, `String`, and
  `MarshalJSON`; JSON and TypeScript generation include
  `configSupport` as a string list.
- `gps/internal/ubx`: table tests for `configSupport()` using existing
  `testVers` values: LEA-6T, M8F/M8P, F9P, F9T before and after MSM4
  support, F10S, and X20P.
- `gps/internal/unc`: table tests for known product models such as
  UM960, UM980, UM982, UB9A0, and unknown.
- `internal/gpscmd`: flag parsing tests for the requirement bitset,
  including explicit-vs-default survey/fixed accuracy, MSM4/MSM7,
  auto/lax RTCM, QZSS RTCM, and disabling options.
- Desktop GUI: TypeScript compile/build on the `desktop-gui` branch
  after adding frontend constants and wiring.

Useful test commands:

```sh
go test -v ./gps/gpsprot ./gps/internal/ubx ./gps/internal/unc ./internal/gpscmd
```

On the desktop branch, also run the frontend build in
`desktop/frontend`.
