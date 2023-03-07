package ubx

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/jclark/gps2phc/internal/ubx/bin"
)

type Version struct {
	HW   string
	SW   string
	Prot ProtVer
}

type ProtVer struct {
	major, minor byte
}

func (m *Message) Version() *Version {
	parsed, ok := m.um.(*bin.MonVer)
	if !ok {
		return nil
	}
	return &Version{
		HW:   Latin1ZToString(parsed.HwVersion[:]),
		SW:   Latin1ZToString(parsed.SwVersion[:]),
		Prot: protVer(parsed),
	}
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

func (v ProtVer) String() string {
	return fmt.Sprintf("%d.%0d", v.major, v.minor)
}

var protVerRegexp = regexp.MustCompile(`^PROTVER[= ]([1-9][0-9]?)\.([0-9][0-9])$`)

func protVer(m *bin.MonVer) ProtVer {
	submatches := findExtension(m, protVerRegexp)
	if submatches == nil {
		return ProtVer{}
	}
	return ProtVer{mustAtob(submatches[1]), mustAtob(submatches[2])}
}

func mustAtob(s string) byte {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(`could not convert UBX "` + s + `" to integer: ` + err.Error())
	}
	return byte(n)
}

func findExtension(m *bin.MonVer, re *regexp.Regexp) []string {
	for _, buf := range m.Extension {
		submatches := re.FindStringSubmatch(Latin1ZToString(buf[:]))
		if submatches != nil {
			return submatches
		}
	}
	return nil
}
