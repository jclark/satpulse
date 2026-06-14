package ubx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

type Version struct {
	HW            string          `json:"hw"`
	SW            string          `json:"sw"`
	Extensions    []string        `json:"extensions,omitempty"`
	FW            *FWVer          `json:"fw,omitempty"`
	Prot          *ProtVer        `json:"prot,omitempty"`
	Mod           string          `json:"mod"`
	RunsFromFlash bool            `json:"runsFromFlash"`
	GNSS          gpsprot.GNSSSet `json:"gnss,omitempty"`
}

type ProtVer struct {
	Major byte `json:"major"`
	Minor byte `json:"minor"`
}

type FWVer struct {
	ProductCategory string `json:"productCategory"` // SPG, HPG, TIM etc
	Major           byte   `json:"major"`
	Minor           byte   `json:"minor"`
}

func (v *Version) ProductCategory() string {
	fw := v.FW
	if fw == nil {
		return ""
	}
	return fw.ProductCategory
}

func (v *Version) tmodeLevel() int {
	switch v.ProductCategory() {
	case "":
		// We are trying to support LEA-6T here (and maybe LEA-5T)
		// The LEA-6T doesn't report itself as a specific product category.
		// If a module has protocol 14.00 or later, it might be a 7th gen, which
		// won't have timing support.
		// If it is less than 14.00, but at least 12.00, it is more likely to be a 6th gen.
		// I don't think anybody will be using 6th gen at this point, unless it's a timing receiver.
		// The LEA-5T currently available on eBay are LEA-5T-0-003 which is firmware 6.02,
		// implying protocol 12.02.
		// There aren't any 7th gen timing modules, and 8th gen reports the product category as "TIM".
		// XXX We should recover from this if we get a NAK.
		if v.protVerAtLeast(14, 0) || !v.protVerAtLeast(12, 0) {
			break
		}
		return 1
	case "FTS", "TIM":
		return 2
	case "HPG":
		return 3
	}
	return 0
}

// rawLevel returns an integer representing what raw output messages are supported.
// 0 means none; 1 means RAW/SFRB, 2 means RAWX/SFRBX
func (v *Version) rawLevel() int {
	switch v.ProductCategory() {
	case "":
		break
	case "FTS", "TIM", "HPG":
		// we only get the category in protocol version 18 and later
		// and RAWX/SFRBX is support from protocol version 17
		return 2
	default:
		return 0
	}
	if v.protVerAtLeast(12, 0) && !v.protVerAtLeast(14, 0) {
		// 12.00 <= PROTVER < 14.00
		// guess it's a LEA-5T or LEA-6T
		return 1
	}
	return 0
}

// isLegacy returns true for Gen8 and earlier devices
func (v *Version) genAtLeast9() bool {
	return v.protVerAtLeast(27, 0)
}

func (v *Version) genUpTo8() bool {
	return !v.protVerGreater(23, 1)
}

type rtcmSupport struct {
	gnss     gpsprot.GNSSSet
	msgs     gpsprot.RTCMMsgFlags
	df003Out bool
}

func (v *Version) rtcmSupport() rtcmSupport {
	g := v.GNSS & gpsprot.MajorGNSSSet // nothing supports NavIC
	switch v.ProductCategory() {
	case "HPG":
		if v.genUpTo8() {
			// Gen 8 doesn't support GAL
			g &^= gpsprot.GNSSSetOf(gpsprot.GAL)
		}
		return rtcmSupport{
			gnss:     g,
			msgs:     gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
			df003Out: v.protVerAtLeast(27, 12),
		}
	case "TIM":
		// ZED-F9T has differential timing support
		// NEO-F10T doesn't
		if !v.isModel("ZED-F9T") {
			break
		}
		// 29.25 added support for MSM4
		if v.protVerAtLeast(29, 25) {
			return rtcmSupport{
				gnss:     g,
				msgs:     gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7,
				df003Out: true,
			}
		}
		return rtcmSupport{
			gnss:     g,
			msgs:     gpsprot.RTCMMsgMSM7,
			df003Out: v.protVerAtLeast(29, 20),
		}
	}
	// RTCM requires HPG or TIM
	return rtcmSupport{}
}

// ubxVariable are the capabilities that depend on the u-blox model or
// protocol version. The rest of ConfigSupportFull is supported by every
// u-blox receiver, so configSupport starts from full minus these and
// adds back the ones a given version has - a new universally supported
// flag then applies to u-blox without editing this code.
const ubxVariable = gpsprot.ConfigSupportBand | gpsprot.ConfigSupportSignal |
	gpsprot.ConfigSupportSurvey | gpsprot.ConfigSupportSurveyAcc |
	gpsprot.ConfigSupportSurveyMsg | gpsprot.ConfigSupportFixedPos |
	gpsprot.ConfigSupportFixedPosAcc | gpsprot.ConfigSupportRaw |
	gpsprot.ConfigSupportRTCMMSM4 | gpsprot.ConfigSupportRTCMMSM7 |
	gpsprot.ConfigSupportRTCMBaseID | gpsprot.ConfigSupportRTCMQZSS

func (v *Version) configSupport() gpsprot.ConfigSupportFlags {
	if v == nil {
		return 0
	}
	flags := gpsprot.ConfigSupportFull &^ ubxVariable
	if v.bandsConfigSupport() {
		flags |= gpsprot.ConfigSupportBand
	}
	if v.genAtLeast9() {
		flags |= gpsprot.ConfigSupportSignal
	}
	tmode := v.tmodeLevel()
	if tmode > 0 {
		flags |= gpsprot.ConfigSupportSurvey |
			gpsprot.ConfigSupportSurveyAcc |
			gpsprot.ConfigSupportFixedPos |
			gpsprot.ConfigSupportFixedPosAcc
		if v.protVerAtLeast(15, 0) && (tmode == 2 || tmode == 3) {
			flags |= gpsprot.ConfigSupportSurveyMsg
		}
	}
	if v.rawLevel() > 0 {
		flags |= gpsprot.ConfigSupportRaw
	}
	rtcm := v.rtcmSupport()
	if rtcm.msgs&gpsprot.RTCMMsgMSM4 != 0 {
		flags |= gpsprot.ConfigSupportRTCMMSM4
	}
	if rtcm.msgs&gpsprot.RTCMMsgMSM7 != 0 {
		flags |= gpsprot.ConfigSupportRTCMMSM7
	}
	if rtcm.df003Out {
		flags |= gpsprot.ConfigSupportRTCMBaseID
	}
	return flags
}

func (v *Version) bandsConfigSupport() bool {
	if !v.genAtLeast9() {
		return false
	}
	switch v.ProductCategory() {
	case "HPG", "TIM", "SPGL1L5":
		return true
	default:
		return false
	}
}

func (v *Version) singleBDSL1Signal() (gpsprot.Signal, bool) {
	if !v.protVerAtLeast(50, 0) {
		switch v.ProductCategory() {
		case "SPG":
			return gpsprot.SigBDSB1I, true
		case "SPGL1L5":
			return gpsprot.SigBDSB1C, true
		}
	}
	return 0, false
}

// tpIndex returns the time pulse index: 0 for TIMEPULSE, 1 for TIMEPULSE2.
func (v *Version) tpIndex() int {
	if v.genAtLeast9() {
		if v.isModel("ZED-X20P") {
			return 1
		}
	} else if v.ProductCategory() == "FTS" {
		return 1
	}
	return 0
}

// isModel reports whether v.Mod is name, optionally followed by a "-" variant suffix
// (e.g. "ZED-F9T" matches both "ZED-F9T" and "ZED-F9T-20B").
func (v *Version) isModel(name string) bool {
	if v == nil {
		return false
	}
	return v.Mod == name || strings.HasPrefix(v.Mod, name+"-")
}

func (v *Version) protVerAtLeast(major, minor byte) bool {
	if v == nil || v.Prot == nil {
		return false
	}
	return v.Prot.Major > major || (v.Prot.Major == major && v.Prot.Minor >= minor)
}

func (v *Version) protVerGreater(major, minor byte) bool {
	if v == nil || v.Prot == nil {
		return false
	}
	return v.Prot.Major > major || (v.Prot.Major == major && v.Prot.Minor > minor)
}

func monVer(parsed *ubxbin.MonVer) *Version {
	x := make([]string, len(parsed.Extension))
	for i := range parsed.Extension {
		x[i] = parsed.Extension[i].String()
	}
	v := &Version{
		HW:         parsed.HwVersion.String(),
		SW:         parsed.SwVersion.String(),
		Extensions: x,
		FW:         findFWVer(x),
		Prot:       findProtVer(x),
		Mod:        findString(x, modRegexp),
		GNSS:       findGNSS(x),
	}
	v.RunsFromFlash = findFlash(v.SW, x)
	setLegacyProtVer(v)
	return v
}

var swOldRegexp = regexp.MustCompile(`^([1-9][0-9]?)\.([0-9][0-9]) \([1-9][0-9]{3,5}\)$`)

func setLegacyProtVer(ver *Version) {
	if ver.Prot != nil {
		return
	}
	submatches := swOldRegexp.FindStringSubmatch(ver.SW)
	if submatches == nil {
		return
	}
	major := mustAtob(submatches[1])
	// LEA-5T and LEA-6T have versions from 4.xx up to 7.xx
	// and may not have a PROTVER line.
	// This is tested with a LEA-6T with firmware 6.02.
	// For other cases, it's an informed guess based on the docs.
	if major < 4 || major > 7 {
		return
	}
	ver.Prot = &ProtVer{
		Major: major + 6, // e.g. SW version 6.02 is Protocol version 12.02
		Minor: mustAtob(submatches[2]),
	}
}

func (pv ProtVer) String() string {
	return fmt.Sprintf("%d.%0d", pv.Major, pv.Minor)
}

func (fwv *FWVer) String() string {
	return fmt.Sprintf("%s %d.%02d", fwv.ProductCategory, fwv.Major, fwv.Minor)
}

// MAX-F10S has a product category of SPGL1L5, so let's accomodate at least SPGL1L2L5
var fwVerRegexp = regexp.MustCompile(`^FWVER[= ]([A-Z][A-Z0-9]+) ([1-9][0-9]?)\.([0-9][0-9])$`)

// LEA-M8F with protocol version 16 has a line `FTS 1.01` without any preceding FWVER
var fwVerOldRegexp = regexp.MustCompile(`^(FTS|TIM|HPG|SPG) ([1-9][0-9]?)\.([0-9][0-9])$`)
var protVerRegexp = regexp.MustCompile(`^PROTVER[= ]([1-9][0-9]?)\.([0-9][0-9])$`)
var modRegexp = regexp.MustCompile(`^MOD[= ]([A-Z][-A-Z0-9]+)$`)
var fisRegexp = regexp.MustCompile(`^FIS[= ]0[xX]`)
var gnssRegexps = []*regexp.Regexp{
	// Major GNSS
	regexp.MustCompile(`^GPS(;[A-Z][A-Za-z]{2,4})*$`),
	// Augmentation system
	regexp.MustCompile(`^(SBAS|QZSS)(;[A-Z][A-Za-z]{2,4})*$`),
	// Non-major GNSS
	// On the MAX-F10S, NAVIC shows up non its own line
	regexp.MustCompile(`^NAVIC(;[A-Z][A-Za-z]{2,4})*$`),
}

func findFWVer(extensions []string) *FWVer {
	submatches := findSubmatch(extensions, fwVerRegexp)
	if submatches == nil {
		submatches = findSubmatch(extensions, fwVerOldRegexp)
		if submatches == nil {
			return nil
		}
	}
	return &FWVer{submatches[1], mustAtob(submatches[2]), mustAtob(submatches[3])}
}

func findProtVer(extensions []string) *ProtVer {
	submatches := findSubmatch(extensions, protVerRegexp)
	if submatches == nil {
		return nil
	}
	return &ProtVer{mustAtob(submatches[1]), mustAtob(submatches[2])}
}

func findGNSS(extensions []string) (gnss gpsprot.GNSSSet) {
	for _, re := range gnssRegexps {
		submatches := findSubmatch(extensions, re)
		if submatches == nil {
			continue
		}
		names := strings.Split(submatches[0], ";")
		for _, name := range names {
			g, err := gpsprot.ParseGNSS(name)
			if err != nil {
				continue
			}
			gnss |= gpsprot.GNSSSetOf(g)
		}
	}
	return
}

func findFlash(sw string, extensions []string) bool {
	if strings.HasPrefix(sw, "EXT ") {
		return true
	}
	if strings.HasPrefix(sw, "ROM ") {
		return false
	}
	return findSubmatch(extensions, fisRegexp) != nil
}

func findString(extensions []string, re *regexp.Regexp) string {
	submatches := findSubmatch(extensions, re)
	if submatches == nil {
		return ""
	}
	return submatches[1]
}

func findSubmatch(extensions []string, re *regexp.Regexp) []string {
	for _, s := range extensions {
		submatches := re.FindStringSubmatch(s)
		if submatches != nil {
			return submatches
		}
	}
	return nil
}

func mustAtob(s string) byte {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(`could not convert UBX "` + s + `" to integer: ` + err.Error())
	}
	return byte(n)
}
