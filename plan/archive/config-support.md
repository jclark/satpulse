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

Add the support bitset type to `gpsprot`:

```go
type ConfigSupportFlags uint32

const (
	ConfigSupportBand ConfigSupportFlags = 1 << iota
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
	ConfigSupportLast = ConfigSupportRTCMQZSS
)

const ConfigSupportRTCMMSM = ConfigSupportRTCMMSM4 | ConfigSupportRTCMMSM7
```

`ConfigSupportLast` is exported because packages outside `gpsprot`
need a stable bound when iterating over individual public support flags.
Keep it equal to the highest declared single-bit flag and test that it
matches the flag table.

The bitset belongs to the configuration result, not to
`ReceiverInfo`.  It describes what the current SatPulse configuration
implementation can configure for the probed receiver/protocol; it is
not a pure hardware capability.

Add a method to `gpsprot.Configurator`:

```go
ConfigSupport() ConfigSupportFlags
```

Add a sibling field to the configuration result:

```go
type Result struct {
	ReceiverInfo  *gpsprot.ReceiverInfo
	ConfigSupport gpsprot.ConfigSupportFlags
	ConfigProps   *gpsprot.ConfigProps
	// ...
}
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
["band", "speed", "fixedPos", "raw"]
```

When a JSON or TypeScript surface exposes configuration results, it
should expose `configSupport` as a string list next to receiver
information, not inside `ReceiverInfo`.  Do not add it to the current
time-server `InitSSE` event yet; wait until there is a concrete
frontend or NTRIP consumer for that surface.

## Flag meanings

`ConfigSupportBand`
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
- `Band`: false before generation 9.  For generation 9 or later, true
  for product categories `HPG`, `TIM`, and `SPGL1L5`; false otherwise.

Return this value from `(*ubx.Configurator).ConfigSupport`.

## Unicore computation

Define a package-level constant for the Unicore implementation support
set:

```go
const configSupport gpsprot.ConfigSupportFlags = ...
```

The current Unicore configuration implementation is not conditioned on
specific product models, so do not maintain a model allowlist.  The
constant should include:

- `Band`: true.
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

Return this constant from `(*unc.Configurator).ConfigSupport`.

## satpulsetool gps requirements

`internal/gpscmd/gpsflags.go` should return or record the
`ConfigSupportFlags` required by the user's requested configuration.
This requirement set is compared with `gpscfg.Result.ConfigSupport` to
produce warnings for missing support.  Track requirements as a map
from required support flag to the CLI option that requested it, plus a
single special case for RTCM MSM where either MSM4 or MSM7 satisfies
the option.  Emit one warning per missing CLI option, for example
`receiver does not support the following option` with
`option=--fixed-pos-ecef`, rather than warning with raw support flag
names.

Initial mapping:

- `--band`: `ConfigSupportBand`.
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
Supports: band, speed, survey, surveyAcc, surveyMsg, fixedPos, fixedPosAcc, raw, rtcmMSM4, rtcmMSM7, rtcmBaseID, rtcmQZSS
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

- `Band` covers the signal picker/band-selection distinction from
  the constellation checkboxes.
- `Speed` covers the serial speed selector.
- `Survey`, `SurveyAcc`, and `SurveyMsg` cover the survey-in mode
  controls and the "Report survey progress" option.
- `FixedPos` and `FixedPosAcc` cover fixed position and its accuracy
  field, without exposing native LLH/ECEF support.
- `Raw` covers the raw message group.
- `RTCMMSM4`, `RTCMMSM7`, `RTCMBaseID`, and `RTCMQZSS` cover the RTCM
  group and NTRIP STR needs.

When the desktop GUI branch needs this, it should consume
`configSupport` from the configuration-result/readback flow as a
sibling to receiver information.  It should not need protocol-specific
tables in frontend code.  The current time-server dashboard does not
need a `configSupport` field in `InitSSE`.

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
  `MarshalJSON`.
- Configuration result JSON/TypeScript tests, where applicable, should
  include `configSupport` as a string list outside `ReceiverInfo`.  Do
  not add this to the time-server `InitSSE` tests yet.
- `gps/internal/ubx`: table tests for `configSupport()` using existing
  `testVers` values: LEA-6T, M8F/M8P, F9P, F9T before and after MSM4
  support, F10S, and X20P.
- `gps/internal/unc`: test the package-level support constant and
  `(*Configurator).ConfigSupport`.
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
