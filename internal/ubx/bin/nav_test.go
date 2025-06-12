package bin

import "testing"

func TestNavTimeGPS(t *testing.T) {
	testMsgType(t, NavTimeGPS{
		ITOW:  0x12345678,
		FTOW:  0,
		Week:  6523,
		LeapS: 37,
		Valid: 0,
		TAcc:  173,
	})
}
