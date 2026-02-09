# Message configuration semantics

This document defines the semantics of the message configuration flags. It covers what the flags mean and how they map to `ConfigOptions`. UI rendering is defined in [phase5c.md](phase5c.md) and [ui-panel-configuration.md](ui-panel-configuration.md).

## Standardized protocols

NMEA and RTCM are standardized protocols. The receiver has a protocol-level enable/disable for each, plus per-message rate control within the protocol.

### Three-way control

Both NMEA and RTCM use the same three-way pattern:

1. **Don't configure** -- leave the protocol and its messages untouched. The `Option` is not set. No configuration requests are generated for this protocol.

2. **Configure** -- take control of the listed messages within the protocol. The `Option` is set with the selected message flags OR'd with the `Other` flag (`NMEAMsgOther` / `RTCMMsgOther`). The protocol is enabled at the port level. Listed messages get explicit rates (1 if selected, 0 if not). Unlisted messages in the protocol are left alone. The `Other` flag is always included; it ensures the protocol stays enabled even when no listed messages are selected.

3. **Disable** -- disable the protocol entirely at the port level. The `Option` is set to 0 (no flags). No messages of this protocol are output regardless of individual message settings.

### Mapping to ConfigOptions

| State | NMEA | RTCM |
|---|---|---|
| Don't configure | `NMEAMsg` not set | `RTCMMsg` not set |
| Configure | `NMEAMsg.Set(selected \| NMEAMsgOther)` | `RTCMMsg.Set(selected \| RTCMMsgOther)` |
| Disable | `NMEAMsg.Set(0)` | `RTCMMsg.Set(0)` |

### NMEA messages

Individually selectable messages:

| Name | Flag | Description |
|---|---|---|
| RMC | `NMEAMsgRMC` | Recommended minimum (position, time, course) |
| GGA | `NMEAMsgGGA` | Fix data (position, altitude, fix quality) |
| GSA | `NMEAMsgGSA` | DOP and active satellites |
| GSV | `NMEAMsgGSV` | Satellites in view |
| ZDA | `NMEAMsgZDA` | Time and date |
| VTG | `NMEAMsgVTG` | Course and speed over ground |

### RTCM messages

Individually selectable messages:

| Name | Flag | Description |
|---|---|---|
| MSM4 | `RTCMMsgMSM4` | MSM4 messages for all enabled GNSS |
| MSM7 | `RTCMMsgMSM7` | MSM7 messages for all enabled GNSS |
| ARP | `RTCMMsgARP` | Antenna reference point (message 1005) |

Constraints:
- MSM4 and MSM7 are mutually exclusive.
- `RTCMMsgLax` makes the MSM4/MSM7 choice a preference rather than a requirement. Without `Lax`, requesting an MSM type that the hardware does not support is an error. With `Lax`, the library falls back to whichever MSM type the hardware supports.

`RTCMMsgAuto` is a convenience constant equivalent to `RTCMMsgMSM4 | RTCMMsgARP | RTCMMsgLax` -- prefer MSM4 with ARP, falling back to MSM7 if the hardware requires it.

### RTCM flag combinations

| Selection | Flags |
|---|---|
| MSM4 | `RTCMMsgMSM4 \| RTCMMsgLax \| RTCMMsgOther` (plus `RTCMMsgARP` if selected) |
| MSM7 | `RTCMMsgMSM7 \| RTCMMsgLax \| RTCMMsgOther` (plus `RTCMMsgARP` if selected) |
| Only ARP | `RTCMMsgARP \| RTCMMsgOther` |
| Nothing | `RTCMMsgOther` (protocol stays enabled, no specific messages) |

## Proprietary messages

Different GPS receiver vendors use different proprietary binary protocols (e.g. u-blox UBX, SiRF Binary, Trimble TSIP). Each protocol divides information across messages in its own way -- for example, one vendor might put position and velocity in a single message, while another uses separate messages for each. The messages are not standardized in any way, and the mapping between information content and message structure varies across vendors and even across firmware versions from the same vendor.

The only way to provide a uniform, protocol-independent interface for controlling proprietary message output is to describe the *information* the user wants, not the specific messages. The user specifies what information they want (e.g. position, time-pulse time, satellite positions), and the library determines which vendor-specific messages need to be enabled to provide that information. The enabled messages are decoded into the abstract `*Msg` types defined in `gpsprot/msg.go` (`TimeMsg`, `LeapSecondMsg`, `SurveyMsg`, `SatellitesMsg`) and delivered through the `MsgHandler` interface.

All proprietary message categories use the same two-way control:

1. **Don't configure** -- leave messages in this category untouched. The flags are not set (for `Option[T]` types, the Option is not set; for `PVTMsgFlags`, the value is zero). No configuration requests are generated for this category.

2. **Configure** -- declare the desired information for this category. The library maps the selected information flags to the appropriate vendor-specific messages. Messages not needed for the selected information are turned off.

### Satellite messages

Controlled by `SatsMsgFlags`, stored in `ConfigOptions.SatsMsg` as `Option[SatsMsgFlags]`.

Satellite messages produce `SatellitesMsg` events. There are two kinds of satellite information:

| Name | Flag | Description |
|---|---|---|
| Satellite positions | `SatsMsgSat` | Information about each tracked space vehicle: look angles (azimuth, elevation) and whether it is used in the navigation solution |
| Signals | `SatsMsgSignal` | Information about each signal received from each space vehicle: signal ID, C/N0, and whether the signal is used |

When configured, both flags are always declared explicitly -- selected information is enabled, unselected information is disabled.

Note: in the future, satellite position information (azimuth/elevation) may be separated from whether each satellite/signal is used in the navigation solution. For now, both are controlled together by `SatsMsgSat`.

#### Mapping to ConfigOptions

| State | Value |
|---|---|
| Don't configure | `SatsMsg` not set |
| Configure | `SatsMsg.Set(selected flags)` |

### Raw messages

Controlled by `RawMsgFlags`, stored in `ConfigOptions.RawMsg` as `Option[RawMsgFlags]`.

Raw messages provide unprocessed observation and navigation data needed for generating RINEX files for post-processing. The two flags correspond to the two types of RINEX file:

| Name | Flag | RINEX file | Description |
|---|---|---|---|
| Observations | `RawMsgObs` | `.obs` | Raw observation data (pseudorange, carrier phase, Doppler) |
| Navigation data | `RawMsgNavData` | `.nav` | Broadcast navigation data (ephemeris, almanac) |

Unlike PVT and satellite messages, raw messages are not decoded into abstract `*Msg` events -- they are protocol-specific data passed through as-is.

When configured, both flags are always declared explicitly -- selected information is enabled, unselected information is disabled. Not all receivers support raw messages; the library returns an error if raw messages are requested on unsupported hardware.

#### Mapping to ConfigOptions

| State | Value |
|---|---|
| Don't configure | `RawMsg` not set |
| Configure | `RawMsg.Set(selected flags)` |

### PVT messages

Controlled by `PVTMsgFlags`, stored in `ConfigOptions.PVTMsg` as a bare `PVTMsgFlags` value (not wrapped in `Option[T]`). A zero value means "don't configure". Any nonzero value means "configure".

A GNSS receiver's core function is to compute PVT solutions -- determining position, velocity, and time from satellite signals. PVT messages report these solutions and related information. The flags are grouped by domain:

**Time:**

| Name | Flag | Description | Event type |
|---|---|---|---|
| Navigation time | `PVTMsgTime` | Time of navigation solution | `TimeMsg` |
| Time-pulse time | `PVTMsgTimePulse` | Time of the PPS pulse | `TimeMsg` |
| Leap second | `PVTMsgLeapSecond` | Most recently announced leap second | `LeapSecondMsg` |

Time modifiers (only meaningful when a time flag above is selected):

| Name | Flag | Description |
|---|---|---|
| TAI | `PVTMsgTAI` | Prefer TAI (or constant offset from TAI) rather than UTC |
| After pulse | `PVTMsgTimePulseAfter` | Ensure a time message is emitted after the time pulse |

TAI applies when `PVTMsgTime` or `PVTMsgTimePulse` is selected. After pulse applies when `PVTMsgTimePulse` is selected.

**Position and velocity:**

| Name | Flag | Description |
|---|---|---|
| Position | `PVTMsgPos` | Position (latitude/longitude/altitude by default) |
| Velocity | `PVTMsgVel` | Velocity (North/East/Down components by default) |

Position/velocity modifier (only meaningful when Position or Velocity is selected):

| Name | Flag | Description |
|---|---|---|
| ECEF | `PVTMsgECEF` | Prefer ECEF coordinates rather than geodetic/NED |

**Survey:**

| Name | Flag | Description | Event type |
|---|---|---|---|
| Survey progress | `PVTMsgSurvey` | Survey-in progress and results | `SurveyMsg` |

Only enables the survey progress message if a survey has been requested in the same configuration run.

**Turn off others:**

| Name | Flag | Description |
|---|---|---|
| Off | `PVTMsgOff` | Turn off PVT messages that are not explicitly enabled |

Without `PVTMsgOff`, only the selected messages are enabled; unselected messages are left as-is. With `PVTMsgOff`, unselected messages are explicitly turned off. This distinction exists because there are many PVT-related messages on some receivers, and turning them all off takes time -- the daemon omits `PVTMsgOff` for fast startup, while `satpulsetool` includes it for a clean, known state.

The library maps these abstract information flags to vendor-specific messages. A single vendor message may carry multiple kinds of information (e.g. u-blox UBX-NAV-PVT carries position, velocity, and time together), so the library optimizes message selection based on what is requested.

#### Mapping to ConfigOptions

| State | Value |
|---|---|
| Don't configure | `PVTMsg` is zero |
| Configure | `PVTMsg.Set(selected flags)` |

## References

- `gps/gpsprot/configtarget.go` -- flag type definitions (`PVTMsgFlags`, `NMEAMsgFlags`, `RTCMMsgFlags`, `SatsMsgFlags`, `RawMsgFlags`) and `ConfigOptions` struct
- `gps/gpsprot/msg.go` -- abstract `*Msg` types (`TimeMsg`, `LeapSecondMsg`, `SurveyMsg`, `SatellitesMsg`) and `MsgHandler` interface
- `gps/internal/ubx/ubxcfgmsg.go` -- u-blox implementation mapping flags to hardware-specific UBX message rates
- `internal/gpscmd/gpsflags.go` -- CLI flag parsing for `satpulsetool gps`
- `docs/man/satpulsetool-gps.1.md` -- man page documenting the CLI options
- `time/app/daemon/gps.go` -- daemon's PVT message flags (relevant for daemon preset)
