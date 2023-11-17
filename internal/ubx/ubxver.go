package ubx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/internal/gpsmsg"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

type Version struct {
	HW         string         `json:"hw"`
	SW         string         `json:"sw"`
	Extensions []string       `json:"extensions,omitempty"`
	FW         *FWVer         `json:"fw,omitempty"`
	Prot       *ProtVer       `json:"prot,omitempty"`
	Mod        string         `json:"mod"`
	Flash      bool           `json:"flash"`
	GNSS       gpsmsg.GNSSSet `json:"gnss,omitempty"`
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
	return v
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

func findGNSS(extensions []string) (gnss gpsmsg.GNSSSet) {
	for _, re := range gnssRegexps {
		submatches := findSubmatch(extensions, re)
		if submatches == nil {
			continue
		}
		names := strings.Split(submatches[0], ";")
		for _, name := range names {
			g, err := gpsmsg.ParseGNSS(name)
			if err != nil {
				continue
			}
			gnss |= gpsmsg.GNSSFlag(g)
		}
	}
	return
}

func findFlash(sw string, extensions []string) bool {
	if strings.HasPrefix(sw, "EXT CORE ") {
		return true
	}
	if strings.HasPrefix(sw, "ROM CORE ") {
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
