package nmea

import (
	"bytes"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/scantest"
)

func TestGoodNMEA(t *testing.T) {
	nmeaOK(t, "$GPGGA,092725.00,4717.11399,N,00833.91590,E,1,08,1.01,499.6,M,48.0,M,,*5B\r\n")
	nmeaOK(t, "$GPGLL,4717.11364,N,00833.91565,E,092321.00,A,A*60\r\n")
	nmeaOK(t, "$PUBX,41,1,0007,0003,19200,0*25\r\n")
	nmeaOK(t, "$GPTXT,01,01,02,u-blox ag - www.u-blox.com*50\r\n")
	nmeaOK(t, "$GPVTG,77.52,T,,M,0.004,N,0.008,K,A*06\r\n")
	nmeaOK(t, "$GPZDA,082710.00,16,09,2002,00,00*64\r\n")
	// from Unicore - this exceeds 82 character NMEA maximum
	nmeaOK(t, "$GNRMC,114650.00,A,1343.90931561,N,10038.68511804,E,0.005,221.7,040525,0.5,W,A,C*59\r\n")
}

func nmeaOK(t *testing.T, data string) {
	if !gpsprot.IsValidPacket(PacketFormat, ([]byte)(data)) {
		t.Fatalf("gpsprot.IsValidPacket says not valid: %s", data)
	}
	for range 10 {
		buf, nRandom := scantest.InsertRandomPrefix(data, '$')
		startPos, endPos, ok := scantest.FindPacket(PacketFormat, buf)
		if !ok || startPos != nRandom || endPos-startPos != len(data) {
			t.Fatalf("packet parsed incorrectly: %s", data)
		}
		pkt := buf[startPos:endPos]
		extracted := PacketFormat.ExtractChecksum(pkt)
		computed := PacketFormat.ComputeChecksum(pkt)
		if !bytes.Equal(extracted, computed) {
			t.Fatalf("checksum of NMEA packet did not have expected value: extracted %X, computed %X, data %s", extracted, computed, data)
		}
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
	if gpsprot.IsValidPacket(PacketFormat, ([]byte)(data)) {
		t.Fatalf("gpsprot.IsValidPacket says valid: %s", data)
	}
	for range 10 {
		buf, _ := scantest.InsertRandomPrefix(data, '$')
		_, _, ok := scantest.FindPacket(PacketFormat, buf)
		if ok {
			t.Fatalf("bad packet accepted incorrectly: %s", data)
		}
	}
}
