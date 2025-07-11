package ubx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
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

func (v *Version) rtcmSupport() (gpsprot.GNSSSet, gpsprot.RTCMMsgFlags) {
	g := v.GNSS & gpsprot.MajorGNSSSet // nothing supports NavIC
	switch v.ProductCategory() {
	case "HPG":
		if v.genUpTo8() {
			// Gen 8 doesn't support GAL
			g &^= gpsprot.GNSSSetOf(gpsprot.GAL)
		}
		return g, gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7
	case "TIM":
		// ZED-F9T has differential timing support
		// NEO-F10T doesn't
		if v.Mod != "ZED-F9T" {
			break
		}
		// 29.25 added support for MSM4
		if v.protVerAtLeast(29, 25) {
			return g,  gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7
		}
		return g, gpsprot.RTCMMsgMSM7
	}
	// RTCM requires HPG or TIM
	return 0, 0
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

func monVer(parsed *bin.MonVer) *Version {
	x := make([]string, len(parsed.Extension))
	for i := range parsed.Extension {
		x[i] = bin.Latin1ZToString(parsed.Extension[i][:])
	}
	v := &Version{
		HW:         bin.Latin1ZToString(parsed.HwVersion[:]),
		SW:         bin.Latin1ZToString(parsed.SwVersion[:]),
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
