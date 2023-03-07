package ubx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jclark/gps2phc/internal/gpsmsg"
	"github.com/jclark/gps2phc/internal/ubx/bin"
)

type Version struct {
	HW         string
	SW         string
	Extensions []string
	FW         FWVer
	Prot       ProtVer
	Mod        string
	Flash      bool
	GNSS       []gpsmsg.MajorGNSS
}

type ProtVer struct {
	Major, Minor byte
}

type FWVer struct {
	ProductCategory string // SPG, HPG, TIM etc
	Major, Minor    byte
}

func (m *Message) Version() *Version {
	parsed, ok := m.um.(*bin.MonVer)
	if !ok {
		return nil
	}
	x := make([]string, len(parsed.Extension))
	for i := range parsed.Extension {
		x[i] = Latin1ZToString(parsed.Extension[i][:])
	}
	v := &Version{
		HW:         Latin1ZToString(parsed.HwVersion[:]),
		SW:         Latin1ZToString(parsed.SwVersion[:]),
		Extensions: x,
		FW:         findFWVer(x),
		Prot:       findProtVer(x),
		Mod:        findString(x, modRegexp),
		GNSS:       findGNSS(x),
	}
	v.Flash = findFlash(v.SW, x)
	return v
}

// Latin1ZString create a string from a ISO Latin-1, nul-terminated byte slice.
// This can be used for the fields of MonVer
func Latin1ZToString(chars []byte) string {
	r := make([]rune, 0)
	for _, ch := range chars {
		if ch == 0 {
			break
		}
		r = append(r, rune(ch))
	}
	return string(r)
}

func (pv ProtVer) String() string {
	return fmt.Sprintf("%d.%0d", pv.Major, pv.Minor)
}

var fwVerRegexp = regexp.MustCompile(`^FWVER[= ]([A-Z]{3}) ([1-9][0-9]?)\.([0-9][0-9])$`)
var protVerRegexp = regexp.MustCompile(`^PROTVER[= ]([1-9][0-9]?)\.([0-9][0-9])$`)
var modRegexp = regexp.MustCompile(`^MOD[= ]([A-Z][-A-Z0-9]+)$`)
var fisRegexp = regexp.MustCompile(`^FIS[= ]0[xX]`)
var gnssRegexp = regexp.MustCompile(`^GPS(;[A-Z]{3,4})*$`)

func findFWVer(extensions []string) FWVer {
	submatches := findSubmatch(extensions, fwVerRegexp)
	if submatches == nil {
		return FWVer{}
	}
	return FWVer{submatches[1], mustAtob(submatches[2]), mustAtob(submatches[3])}
}

func findProtVer(extensions []string) ProtVer {
	submatches := findSubmatch(extensions, protVerRegexp)
	if submatches == nil {
		return ProtVer{}
	}
	return ProtVer{mustAtob(submatches[1]), mustAtob(submatches[2])}
}

func findGNSS(extensions []string) []gpsmsg.MajorGNSS {
	var gnss []gpsmsg.MajorGNSS
	submatches := findSubmatch(extensions, gnssRegexp)
	if submatches == nil {
		return nil
	}
	names := strings.Split(submatches[0], ";")
	for _, name := range names {
		switch name {
		case "GPS":
			gnss = append(gnss, gpsmsg.GPS)
		case "GLO":
			gnss = append(gnss, gpsmsg.GLONASS)
		case "GAL":
			gnss = append(gnss, gpsmsg.Galileo)
		case "BDS":
			gnss = append(gnss, gpsmsg.BeiDou)
		}
	}
	return gnss
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
