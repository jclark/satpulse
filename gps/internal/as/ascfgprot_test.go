package as

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestProbe(t *testing.T) {
	rcvr := &testReceiver{monVer: tau1201Ver()}
	cp := probe(t, rcvr)
	if !cp.ProbeOK() {
		t.Fatal("ProbeOK = false after MON-VER response")
	}
	cfg, errCount := configure(t, cp, rcvr, &gpsprot.ConfigTarget{})
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	got := cfg.ReceiverInfo()
	want := &gpsprot.ReceiverInfo{
		Vendor:        Vendor,
		Firmware:      "3.018.aab95e7",
		Hardware:      "HD8040D.9529b663",
		SupportedGNSS: navSatToSignals(tau1201Cap).GNSSSet(),
	}
	if got.VendorSpecific == nil {
		t.Error("VendorSpecific = nil, want MON-VER message")
	}
	want.VendorSpecific = got.VendorSpecific
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReceiverInfo\ngot  %+v\nwant %+v", got, want)
	}
}

func TestProbeSilentReceiver(t *testing.T) {
	rcvr := &testReceiver{} // no MON-VER answer: not an Allystar receiver
	cp := probe(t, rcvr)
	if cp.ProbeOK() {
		t.Fatal("ProbeOK = true with no MON-VER response")
	}
}
