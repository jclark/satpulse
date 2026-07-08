package quectel

import "regexp"

// fwVersion is an LG290P firmware version, the R<major>A<minor>S
// suffix of the PQTMVERNO version string (LG290P03AANR02A01S is
// R02A01S = {2, 1}). The zero value means unknown: the version string
// did not match the LG290P pattern, and feature gating is optimistic.
type fwVersion struct {
	major, minor int
}

var fwVersionPat = regexp.MustCompile(`^LG290P\w*R([0-9]{2})A([0-9]{2})S$`)

// parseFwVersion extracts the firmware version from a PQTMVERNO
// version string. It returns the zero fwVersion for anything that is
// not an LG290P version pattern (other family modules or unrecognized
// strings).
func parseFwVersion(verStr string) fwVersion {
	m := fwVersionPat.FindStringSubmatch(verStr)
	if m == nil {
		return fwVersion{}
	}
	return fwVersion{major: atoi2(m[1]), minor: atoi2(m[2])}
}

func atoi2(s string) int {
	return int(s[0]-'0')*10 + int(s[1]-'0')
}

// Feature minimums per the LG290P firmware release notes. Commands
// and messages not listed here predate R01A04S and are treated as
// always present.
var (
	fwZDA     = fwVersion{1, 4} // GBS/GNS/GST/ZDA standard NMEA messages
	fwRTCM33  = fwVersion{1, 4} // RTCM3-1033 message
	fwEleThd  = fwVersion{1, 5} // PQTMCFGELETHD
	fwQZSSL6  = fwVersion{1, 5} // PQTMCFGSIGNAL QZSS L6 bit
	fwNAV     = fwVersion{1, 5} // PQTMNAV and PQTMEOE messages
	fwPPS2    = fwVersion{2, 1} // PQTMCFGPPS2
	fwRTCM230 = fwVersion{2, 1} // RTCM-1230 message
)

// has reports whether firmware version v offers a feature with the
// given minimum version. Unknown versions (zero v) are optimistic:
// the feature is assumed present and NAK recovery handles absence.
// Gating never overrides what the receiver actually answers.
func (v fwVersion) has(min fwVersion) bool {
	if v == (fwVersion{}) {
		return true
	}
	return v.major > min.major || (v.major == min.major && v.minor >= min.minor)
}
