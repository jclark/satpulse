package scan

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/scantest"
)

type packetType int

const (
	notAPacket packetType = iota
	validGNSSNMEA
	validApprovedNMEA
	validProprietaryNMEA
	otherPacket
)

type testCase struct {
	data     string
	expected packetType
}

type mixedTestCase struct {
	data         string
	invalidBytes int
}

func TestNMEA(t *testing.T) {
	tests := []testCase{
		// good cases
		{"$GPRMC,1,2,3*0F\r\n", validGNSSNMEA},
		{"$GPRM1,1,2,3*AE\r\n", validGNSSNMEA},
		{"$PUBX,1,2,3*2B\r\n", validProprietaryNMEA},
		{"$GPTXT,1,2,how are you?*7F\r\n", validGNSSNMEA},
		{"$ABCDE*0F\r\n", validApprovedNMEA},
		{"$GPTXT,1,2,3,Hello^21*FF\r\n", validGNSSNMEA},
		{"$PQTMVER,1,MODULE,LG290P03AANR01A03S,2024/04/30,10:53:07*32\r\n", validProprietaryNMEA},
		// bad cases
		{"$GPMRC,$1*FF\r\n", notAPacket},            // invalid char
		{"$GPTXT,1,2,3,Hello!*FF\r\n", otherPacket}, // ! not valid in NMEA approved sentence
		{"$ABCDEF,1*FF\r\n", otherPacket},           // address too long
		{"$ABC,1*FF\r\n", otherPacket},              // address too short
		{"$ABCdE,1*FF\r\n", otherPacket},            // lower case address
		{"$GPRMC,1,2,3*0f\r\n", notAPacket},         // lower case hex
		{"$GPRMC,1,2,3*0\r\n", notAPacket},          // missing hex
		{"$ABCD,1*FF\r\n", otherPacket},             // should start with P
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			r := strings.NewReader(tc.data)
			trimmed := scantest.TrimNMEA(tc.data)
			s := New(r, 64)
			pkt, err := s.Scan()
			if err != nil {
				t.Fatalf(`error reading packet "%s": %v`, trimmed, err)
			}
			var actual packetType
			if pkt.Tag() != nmea.Tag {
				actual = notAPacket
			} else {
				flags := nmea.CheckSyntax(pkt.Data)
				if flags.IsValidGNSSTalkerNMEA() {
					actual = validGNSSNMEA
				} else if flags.IsValidApprovedNMEA() {
					actual = validApprovedNMEA
				} else if flags.IsValidProprietaryNMEA() {
					actual = validProprietaryNMEA
				} else {
					actual = otherPacket
				}
			}
			if actual != tc.expected {
				t.Fatalf(`NMEA message "%s" expected %v, got %v`, trimmed, tc.expected, actual)
			}
			if actual == notAPacket {
				return
			}
			pkt, err = s.Scan()
			if err != io.EOF || pkt.Format != nil || pkt.Data != "" {
				t.Fatalf(`did not get EOF with no data after NMEA "%s"`, trimmed)
			}
		})
	}
}

func TestMixedNMEA(t *testing.T) {
	tests := []mixedTestCase{
		{"$GPRMC,1,2,3*0F\r\n", 0},
		{"Hello$GPRMC,1,2,3*0F\r\n", 5},
		{"$GPRMC,1,2$GPRMC,1,2,3*0F\r\n", 10},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			trimmed := scantest.TrimNMEA(tc.data)
			r := strings.NewReader(tc.data)
			s := New(r, 64)
			nInvalid := 0
			for {
				pkt, err := s.Scan()
				if err == io.EOF {
					t.Fatalf(`EOF reading packet "%s" without NMEA packet; %d invalid bytes`, trimmed, nInvalid)
				}
				if err != nil {
					t.Fatalf(`error reading packet "%s"`, trimmed)
				}
				if pkt.Format == nil {
					nInvalid += len(pkt.Data)
					continue
				}
				if pkt.HasTag(nmea.Tag) {
					break
				} else {
					t.Fatalf(`no valid NMEA message found in "%s"`, trimmed)
				}
			}
			if tc.invalidBytes != nInvalid {
				t.Fatalf(`wrong number of invalid bytes in "%s: expected %d; got %d"`, trimmed, tc.invalidBytes, nInvalid)
			}
		})
	}
}
