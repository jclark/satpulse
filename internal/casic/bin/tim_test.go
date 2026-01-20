package bin

import (
	"testing"
)

func TestTimTPParse(t *testing.T) {
	// TIM-TP packet from test data
	pkt := parseHex(t, "bace180002002890c80400000000000000000037cb406209100307010000a9d1a548")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg error: %v", err)
	}
	tp, ok := msg.(*TimTP)
	if !ok {
		t.Fatalf("expected *TimTP, got %T", msg)
	}
	if tp.RunTime != 0x04c89028 {
		t.Errorf("RunTime = 0x%x, want 0x%x", tp.RunTime, 0x04c89028)
	}
	if tp.QErr != 0.0 {
		t.Errorf("QErr = %f, want 0.0", tp.QErr)
	}
	if tp.Wn != 2402 {
		t.Errorf("Wn = %d, want 2402", tp.Wn)
	}
	// TOW should be around 13943 seconds (start of next second after 03:51:56)
	if tp.TOW < 13900 || tp.TOW > 14000 {
		t.Errorf("TOW = %f, expected ~13943", tp.TOW)
	}
	// RefTime: low nibble is GNSS (0=GPS), high nibble is time base (1=GNSS)
	if tp.RefTimeGNSS() != GPS {
		t.Errorf("RefTimeGNSS() = %d, want %d", tp.RefTimeGNSS(), GPS)
	}
	if tp.RefTimeBase() != TimTPTimeBaseGNSS {
		t.Errorf("RefTimeBase() = %d, want %d", tp.RefTimeBase(), TimTPTimeBaseGNSS)
	}
	if tp.UTCValid != TimTPUTCValidParams {
		t.Errorf("UTCValid = %d, want %d", tp.UTCValid, TimTPUTCValidParams)
	}
}

func TestTimTPRoundtrip(t *testing.T) {
	m := TimTP{
		RunTime:  12345,
		QErr:     0.000001,
		TOW:      123456.789,
		Wn:       2402,
		RefTime:  0x10, // GPS, GNSS base
		UTCValid: TimTPUTCValidParams,
	}
	testMsgType(t, m)
}
