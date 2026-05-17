package ntrip

import (
	"fmt"
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
// info or props may be nil; an empty versionInfo is allowed.  msm is
// 0 to skip format-details synthesis, or 4/7 etc.  configCapturePos
// is an opportunistic PosGeoMsg captured during gpscfg.Configure,
// used as a lat/lon fallback when no fixed position is configured;
// may be nil.  Lat/lon resolution order: shared.Lat/Lon overrides ->
// Mode.FixedPos* -> configCapturePos -> 0.00.
//
// The returned closure takes the per-stream StreamConfig, the
// stream's mountpoint name, and whether that stream requires client
// authentication (drives the STR record's authentication field: "B"
// when true, "N" when false).
func StreamRecordBuilder(
	shared *SharedStreamConfig,
	props *gpsprot.ConfigProps,
	info *gpsprot.ReceiverInfo,
	versionInfo string,
	msm int,
	configCapturePos *gpsprot.PosGeoMsg,
) func(sc *StreamConfig, name string, hasAuth bool) string {
	gnss, signals := enabledGNSSAndSignals(props)
	navSystem := buildNavSystem(gnss)
	carrier := buildCarrier(signals)

	formatDetails := shared.FormatDetails
	if formatDetails == "" && msm > 0 {
		formatDetails = buildFormatDetails(gnss, msm)
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

	return func(sc *StreamConfig, name string, hasAuth bool) string {
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
		fields := [...]string{
			identifier,
			streamFormat,
			formatDetails,
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

// majorGNSSByMSMBase is the major GNSS constellations sorted by MSM
// message-number base (the numeric Ntrip source-table convention).
// Only major GNSS are synthesised because satpulse's RTCM config
// only enables MSM messages for them (see
// gps/internal/ubx/ubxver.go: rtcmSupport).  Operators can override
// via [ntrip].formatDetails when they need something different.
var majorGNSSByMSMBase = func() []gpsprot.GNSS {
	gs := gpsprot.MajorGNSSSet.Items()
	slices.SortFunc(gs, func(a, b gpsprot.GNSS) int {
		return int(rtcm.MSMMsgType(a, 1)) - int(rtcm.MSMMsgType(b, 1))
	})
	return gs
}()

// buildNavSystem returns the nav-system string from the enabled GNSS
// set, restricted to major constellations (since that's what
// formatDetails advertises MSM for), e.g. "GPS+GLO+GAL+BDS".
func buildNavSystem(gnss gpsprot.GNSSSet) string {
	var parts []string
	for _, g := range majorGNSSByMSMBase {
		if gnss.Contains(g) {
			parts = append(parts, g.String())
		}
	}
	return strings.Join(parts, "+")
}

// buildCarrier returns the carrier value: 0 if no signal, 1 if only
// L1 signals, else 2.  Computed from major-GNSS signals only, since
// that's the subset advertised by nav-system and formatDetails.
func buildCarrier(signals gpsprot.SignalSet) int {
	bands := (signals & gpsprot.SigSetMajor).Bands()
	if bands == 0 {
		return 0
	}
	if bands == gpsprot.BandL1 {
		return 1
	}
	return 2
}

// buildFormatDetails synthesises the format-details string from the
// enabled GNSS set and the MSM level.  Only major GNSS appear, since
// satpulse's RTCM configuration only enables MSM for those.
func buildFormatDetails(gnss gpsprot.GNSSSet, msm int) string {
	parts := []string{"1005(1)"}
	for _, g := range majorGNSSByMSMBase {
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
