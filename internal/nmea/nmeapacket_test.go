package nmea

import (
	"testing"

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
	for range(10) {
		buf, nRandom := scantest.InsertRandomPrefix(data, '$')
		startPos, endPos, ok := scantest.FindPacket(PacketFormat, buf)
		if !ok || startPos != nRandom || endPos - startPos != len(data) {
			t.Fatalf("packet parsed incorrectly: %s", data)
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
	for range(10) {
		buf, _ := scantest.InsertRandomPrefix(data, '$')
		_, _, ok := scantest.FindPacket(PacketFormat, buf)
		if ok {
			t.Fatalf("bad packet accepted incorrectly: %s", data)
		}
	}
}




