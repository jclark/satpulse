package scan

import (
	"io"
	"strings"
	"testing"

	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/scantest"
)

func TestGoodNMEA(t *testing.T) {
	nmeaOK(t, "$GPRMC,1,2,3*0F\r\n")
	nmeaOK(t, "$GPRM1,1,2,3*AE\r\n")
	nmeaOK(t, "$PUBX,1,2,3*2B\r\n")
	nmeaOK(t, "$GPTXT,1,2,how are you?*7F\r\n")
	nmeaOK(t, "$ABCDE*0F\r\n")
	nmeaOK(t, "$GPTXT,1,2,3,Hello^21*FF\r\n")
}

func nmeaOK(t *testing.T, data string) {
	r := strings.NewReader(data)
	s := New(r, 64)
	f, err := s.Scan()
	trimmed := scantest.TrimNMEA(data)
	if err != nil {
		t.Fatalf(`error reading packet "%s"`, trimmed)
	}
	if f.Kind != nmea.PacketKind {
		t.Fatalf(`NMEA message "%s" not recognized as valid`, trimmed)
	}
	if f.Data != data {
		t.Fatalf(`wrong data for "%s"`, trimmed)
	}
	f, err = s.Scan()
	if err != io.EOF || f.Kind != Invalid || f.Data != "" {
		t.Fatalf(`did not get EOF with no data after NMEA "%s"`, trimmed)
	}
}

func TestBadNMEA(t *testing.T) {
	nmeaBad(t, "$GPMRC,$1*FF\r\n")           // invalid char
	nmeaBad(t, "$GPTXT,1,2,3,Hello!*FF\r\n") // invalid char

	nmeaBad(t, "$ABCDEF,1*FF\r\n")    // address too long
	nmeaBad(t, "$ABC,1*FF\r\n")       // address too short
	nmeaBad(t, "$ABCdE,1*FF\r\n")     // lower case address
	nmeaBad(t, "$GPRMC,1,2,3*0f\r\n") // lower case hex
	nmeaBad(t, "$GPRMC,1,2,3*0\r\n")  // missing hex
	nmeaBad(t, "$ABCD,1*FF\r\n")      // should start with P
}

func nmeaBad(t *testing.T, data string) {
	r := strings.NewReader(data)
	s := New(r, 64)
	f, _ := s.Scan()
	if f.Kind != Invalid {
		t.Fatalf(`NMEA message "%s" not recognized as invalid`, scantest.TrimNMEA(data))
	}
}

func TestMixedNMEA(t *testing.T) {
	nmeaMixed(t, "$GPRMC,1,2,3*0F\r\n", 0)
	nmeaMixed(t, "Hello$GPRMC,1,2,3*0F\r\n", 5)
	nmeaMixed(t, "$GPRMC,1,2$GPRMC,1,2,3*0F\r\n", 10)
}

func nmeaMixed(t *testing.T, data string, nInvalidExpected int) {
	trimmed := scantest.TrimNMEA(data)
	r := strings.NewReader(data)
	s := New(r, 64)
	nInvalid := 0
	for  {
		f, err := s.Scan()
		if err == io.EOF {
			t.Fatalf(`EOF reading packet "%s" without NMEA packet; %d invalid bytes`, trimmed, nInvalid)
		}
		if err != nil {
			t.Fatalf(`error reading packet "%s"`, trimmed)
		}	
		if f.Kind == Invalid {
			nInvalid += len(f.Data)
			continue
		}
		if f.Kind == nmea.PacketKind {
			break
		} else {
			t.Fatalf(`no valid NMEA message found in "%s"`, trimmed)
		}
	}
	if nInvalidExpected != nInvalid {
		t.Fatalf(`wrong number of invalid bytes in "%s: expected %d; got %d"`, trimmed, nInvalidExpected, nInvalid)
	}
}
