package scan

import (
	"context"
	"io"
	"strings"
	"testing"
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
	ctx := context.Background()
	r := strings.NewReader(data)
	s := New(r, 64)
	f, err := s.Scan(ctx)
	trimmed := nmeaTrim(data)
	if err != nil {
		t.Fatalf(`error reading packet "%s"`, trimmed)
	}
	if f.Kind != NMEA {
		t.Fatalf(`NMEA message "%s" not recognized as valid`, trimmed)
	}
	if f.Data != data {
		t.Fatalf(`wrong data for "%s"`, trimmed)
	}
	f, err = s.Scan(ctx)
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
	ctx := context.Background()
	r := strings.NewReader(data)
	s := New(r, 64)
	f, _ := s.Scan(ctx)
	if f.Kind != Invalid {
		t.Fatalf(`NMEA message "%s" not recognized as invalid`, nmeaTrim(data))
	}
}

func nmeaTrim(data string) string {
	trimmed := data
	if len(data) > 0 && data[len(data)-1] == '\n' {
		trimmed = data[0 : len(data)-1]
	}
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\r' {
		trimmed = trimmed[0 : len(trimmed)-1]
	}
	return trimmed
}
