// Package casmsg builds CASIC proprietary PCAS NMEA sentences and
// interprets their GPTXT responses. The binary CASIC protocol is in
// gps/lib/casbin; PCAS text commands matter because some operations
// (quieting NMEA output, querying the version of V5 firmware that does
// not answer MON-VER) are only available, or only reliable, as text.
package casmsg

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

// Sentence wraps a sentence payload (the text between $ and *) as a
// complete NMEA sentence with checksum and CR/LF.
func Sentence(payload string) string {
	return fmt.Sprintf("$%s*%02X\r\n", payload, nmeamsg.Checksum([]byte(payload)))
}

// QuietAll returns the PCAS03 command that disables all NMEA output.
// The setting is RAM-only: the receiver reverts on restart or reload.
// On a V5 receiver at its default 9600 baud this is the only way to
// keep the line from saturating (see CONTEXT in gps/internal/casic).
func QuietAll() string {
	return Sentence("PCAS03,0,0,0,0,0,0,0,0,0,0,,,0,0,,,,0")
}

// PCAS06 query info values. The reply is a GPTXT sentence whose text
// is a key=value pair; see ParseTxtInfo.
const (
	QueryFirmwareVersion = 0 // reply key SW
	QueryHardware        = 1 // reply key HW (model and serial)
	QueryWorkingMode     = 2 // reply key MO (e.g. "GB", "GBR")
	QueryCustomerID      = 3 // reply key CI
	QueryUpgradeCode     = 5 // reply key BS
	QueryChipID          = 6 // reply key IC (undocumented; chip variant and serial)
)

// Query returns a PCAS06 query sentence for the given info value.
func Query(info int) string {
	return Sentence(fmt.Sprintf("PCAS06,%d", info))
}

// ParseTxtInfo extracts the key/value reply to a PCAS06 query from a
// GPTXT sentence payload. Replies have the form
// "GPTXT,01,01,02,SW=URANUS5,V5.3.0.0": message id 02 (notification)
// and a two-letter key; the value here is "URANUS5,V5.3.0.0".
// Returns ok=false for any other GPTXT sentence.
func ParseTxtInfo(payload string) (key, value string, ok bool) {
	fields := strings.SplitN(payload, ",", 5)
	if len(fields) < 5 || fields[0] != "GPTXT" || fields[3] != "02" {
		return "", "", false
	}
	k, v, found := strings.Cut(fields[4], "=")
	if !found || len(k) != 2 {
		return "", "", false
	}
	return k, v, true
}
