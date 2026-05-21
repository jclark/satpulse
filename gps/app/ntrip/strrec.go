package ntrip

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/lib/geopos"
)

// streamFormat is the value emitted in field 4 of the STR record.
const streamFormat = "RTCM 3.3"

// StreamRecordBuilder returns a closure that builds STR-record
// fields-3..n strings.  The constructor reads from *shared and from
// gps state, derives nav-system and carrier from enabled signals,
// and captures the resolved values plus everything else needed in
// the closure.  Neither the constructor nor the closure mutates
// *shared or *StreamConfig.
//
// info or props may be nil; an empty versionInfo is allowed.
// rtcmSupport describes the configured RTCM MSM output used for
// format-details synthesis.  If it has no MSM support bit set,
// format-details synthesis is skipped.  configCapturePos is an
// opportunistic PosGeoMsg captured during gpscfg.Configure, used as a
// lat/lon fallback when no fixed position is configured; may be nil.
// Lat/lon resolution order: shared.Lat/Lon overrides ->
// Mode.FixedPos* -> configCapturePos -> 0.00.
//
// The returned closure takes the per-stream StreamConfig, the
// stream's mountpoint name, whether that stream requires client
// authentication (drives the STR record's authentication field: "B"
// when true, "N" when false), and whether the caster converts MSM7
// packets to MSM4 for this stream (drives substitution of MSM7
// message numbers in the format-details field).
func StreamRecordBuilder(
	shared *SharedStreamConfig,
	props *gpsprot.ConfigProps,
	info *gpsprot.ReceiverInfo,
	versionInfo string,
	rtcmSupport gpsprot.ConfigSupportFlags,
	configCapturePos *gpsprot.PosGeoMsg,
) func(sc *StreamConfig, name string, hasAuth, msm7to4 bool) string {
	gnss, signals := enabledGNSSAndSignals(props)
	rtcmGNSS := rtcmOutputGNSS(gnss, rtcmSupport)
	navSystem := buildNavSystem(rtcmGNSS)
	carrier := buildCarrier(signals, rtcmGNSS)

	formatDetails := shared.FormatDetails
	msm := rtcmMSMLevel(rtcmSupport)
	if formatDetails == "" && msm > 0 {
		formatDetails = buildFormatDetails(rtcmGNSS, msm)
	}

	network := shared.Network
	if network == "" {
		network = DefaultNetwork
	}
	country := shared.Country
	if country == "" {
		country = DefaultCountry
	}

	lat, lon := resolveLatLon(shared, props, configCapturePos)

	generator := shared.Generator
	if generator == "" {
		if info != nil && info.Hardware != "" {
			generator = info.Hardware
		} else if versionInfo != "" {
			generator = versionInfo
		} else {
			generator = "satpulse"
		}
	}

	sharedBitrate := shared.Bitrate

	return func(sc *StreamConfig, name string, hasAuth, msm7to4 bool) string {
		identifier := sc.Description
		if identifier == "" {
			identifier = name
		}
		bitrate := sc.Bitrate
		if bitrate == 0 {
			bitrate = sharedBitrate
		}
		authentication := "N"
		if hasAuth {
			authentication = "B"
		}
		fd := formatDetails
		if msm7to4 {
			fd = convertFormatDetailsMSM7to4(fd)
		}
		fields := [...]string{
			identifier,
			streamFormat,
			fd,
			strconv.Itoa(carrier),
			navSystem,
			network,
			country,
			formatLatLon(lat),
			formatLatLon(lon),
			"0",
			"0",
			generator,
			"none",
			authentication,
			"N",
			strconv.Itoa(bitrate),
			"",
		}
		return strings.Join(fields[:], ";")
	}
}

// msm7NumberRE matches an MSM7 RTCM message number (1077, 1087, 1097,
// 1107, 1117, 1127, 1137) as a standalone token.  Group 1 captures the
// three-digit prefix shared with the corresponding MSM4 number.
var msm7NumberRE = regexp.MustCompile(`\b(10[7-9]|11[0-3])7\b`)

// convertFormatDetailsMSM7to4 rewrites any MSM7 RTCM message number
// in a format-details string to the corresponding MSM4 number (e.g.
// 1077 -> 1074, 1087 -> 1084).  Non-MSM7 tokens pass through
// unchanged.
func convertFormatDetailsMSM7to4(s string) string {
	return msm7NumberRE.ReplaceAllString(s, "${1}4")
}

// enabledGNSSAndSignals returns the enabled GNSS set and signal set
// from props, or zero values if props is nil or has no signals set.
func enabledGNSSAndSignals(props *gpsprot.ConfigProps) (gpsprot.GNSSSet, gpsprot.SignalSet) {
	if props == nil {
		return 0, 0
	}
	signals, ok := props.GetSignalsEnabled()
	if !ok {
		return 0, 0
	}
	return signals.GNSSSet(), signals
}

func rtcmMSMLevel(support gpsprot.ConfigSupportFlags) int {
	if support&gpsprot.ConfigSupportRTCMMSM4 != 0 {
		return 4
	}
	if support&gpsprot.ConfigSupportRTCMMSM7 != 0 {
		return 7
	}
	return 0
}

func rtcmOutputGNSS(gnss gpsprot.GNSSSet, support gpsprot.ConfigSupportFlags) gpsprot.GNSSSet {
	out := gnss & gpsprot.MajorGNSSSet
	if support&gpsprot.ConfigSupportRTCMQZSS != 0 && gnss.Contains(gpsprot.QZSS) {
		out |= gpsprot.GNSSSetOf(gpsprot.QZSS)
	}
	return out
}

// rtcmGNSSByMSMBase is the RTCM MSM-capable GNSS order used by
// source-table synthesis.  The caller decides which entries are
// active for the current receiver.
var rtcmGNSSByMSMBase = func() []gpsprot.GNSS {
	gs := (gpsprot.MajorGNSSSet | gpsprot.GNSSSetOf(gpsprot.QZSS)).Items()
	slices.SortFunc(gs, func(a, b gpsprot.GNSS) int {
		return int(rtcm.MSMMsgType(a, 1)) - int(rtcm.MSMMsgType(b, 1))
	})
	return gs
}()

// buildNavSystem returns the nav-system string from the enabled GNSS
// set that formatDetails advertises MSM for, e.g. "GPS+GLO+GAL+BDS".
func buildNavSystem(gnss gpsprot.GNSSSet) string {
	var parts []string
	for _, g := range rtcmGNSSByMSMBase {
		if gnss.Contains(g) {
			parts = append(parts, g.String())
		}
	}
	return strings.Join(parts, "+")
}

// buildCarrier returns the carrier value: 0 if no signal, 1 if only
// L1 signals, else 2.  Computed from the GNSS set advertised by
// nav-system and formatDetails.
func buildCarrier(signals gpsprot.SignalSet, gnss gpsprot.GNSSSet) int {
	bands := (signals & signalSetForGNSS(gnss)).Bands()
	if bands == 0 {
		return 0
	}
	if bands == gpsprot.BandL1 {
		return 1
	}
	return 2
}

func signalSetForGNSS(gnss gpsprot.GNSSSet) gpsprot.SignalSet {
	var signals gpsprot.SignalSet
	for _, g := range gnss.Items() {
		signals |= gpsprot.BandAll.SignalSet(g)
	}
	return signals
}

// buildFormatDetails synthesises the format-details string from the
// enabled GNSS set and the MSM level.
func buildFormatDetails(gnss gpsprot.GNSSSet, msm int) string {
	parts := []string{"1005(1)"}
	for _, g := range rtcmGNSSByMSMBase {
		if !gnss.Contains(g) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d(1)", rtcm.MSMMsgType(g, msm)))
	}
	if gnss.Contains(gpsprot.GLO) {
		parts = append(parts, "1230(1)")
	}
	return strings.Join(parts, ",")
}

// resolveLatLon returns latitude and longitude in degrees, trying in
// order: shared overrides (validated to be set together), Mode.FixedPos*,
// the opportunistically captured PosGeoMsg, and finally (0, 0).
func resolveLatLon(shared *SharedStreamConfig, props *gpsprot.ConfigProps, configCapturePos *gpsprot.PosGeoMsg) (lat, lon float64) {
	if shared.Lat.IsSet() {
		return shared.Lat.Get().Degrees(), shared.Lon.Get().Degrees()
	}
	if props != nil {
		if mode, ok := props.GetMode(); ok {
			switch mode.PosType {
			case gpsprot.PosTypeECEF:
				ecef := geopos.ECEF{
					mode.FixedPosECEF[0].Meters(),
					mode.FixedPosECEF[1].Meters(),
					mode.FixedPosECEF[2].Meters(),
				}
				if llh, err := geopos.WGS84.ECEFtoLLH(ecef); err == nil {
					return llh.Lat, llh.Lon
				}
			case gpsprot.PosTypeLLH:
				return mode.FixedPosLLH[0].Degrees(), mode.FixedPosLLH[1].Degrees()
			}
		}
	}
	if configCapturePos != nil {
		return configCapturePos.LatLon[0].Degrees(), configCapturePos.LatLon[1].Degrees()
	}
	return
}

// formatLatLon formats a latitude or longitude in degrees with two
// decimal places (Ntrip source-table convention).
func formatLatLon(deg float64) string {
	return strconv.FormatFloat(deg, 'f', 2, 64)
}
