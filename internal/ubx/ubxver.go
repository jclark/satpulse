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
	HW         string          `json:"hw"`
	SW         string          `json:"sw"`
	Extensions []string        `json:"extensions,omitempty"`
	FW         *FWVer          `json:"fw,omitempty"`
	Prot       *ProtVer        `json:"prot,omitempty"`
	Mod        string          `json:"mod"`
	Flash      bool            `json:"flash"`
	GNSS       gpsprot.GNSSSet `json:"gnss,omitempty"`
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
		// The LEA-5T currently available on eBat are LEA-5T-0-003 which is firmware 6.02,
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
	v.Flash = findFlash(v.SW, x)
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

var fwVerRegexp = regexp.MustCompile(`^FWVER[= ]([A-Z]{3}) ([1-9][0-9]?)\.([0-9][0-9])$`)

// LEA-M8F with protocol version 16 has a line `FTS 1.01` without any preceding FWVER
var fwVerOldRegexp = regexp.MustCompile(`^(FTS|TIM|HPG|SPG) ([1-9][0-9]?)\.([0-9][0-9])$`)
var protVerRegexp = regexp.MustCompile(`^PROTVER[= ]([1-9][0-9]?)\.([0-9][0-9])$`)
var modRegexp = regexp.MustCompile(`^MOD[= ]([A-Z][-A-Z0-9]+)$`)
var fisRegexp = regexp.MustCompile(`^FIS[= ]0[xX]`)
var gnssRegexps = []*regexp.Regexp{
	// Not sure how NavIC will show up: NAVIC or NavIC
	// Neither a major GNSS nor just an augmentation system, so not sure which list it will be in
	regexp.MustCompile(`^GPS(;[A-Z][A-Za-z]{2,4})*$`),
	regexp.MustCompile(`^(SBAS|QZSS)(;[A-Z][A-Za-z]{2,4})*$`),
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
			gnss |= gpsprot.GNSSFlag(g)
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
