package casbin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/lib/latin1z"
)

func TestMonVerRoundtrip(t *testing.T) {
	var sw, hw latin1z.StringZ32
	copy(sw[:], "CASIC V5.3.0.0")
	copy(hw[:], "HW-1234-REV-A")
	testMsgType[MonVer](t, MonVer{SwVersion: sw, HwVersion: hw})
}

func TestMonVerJSON(t *testing.T) {
	var m MonVer
	copy(m.SwVersion[:], "CASIC V5.3.0.0")
	copy(m.HwVersion[:], "HW-1234-REV-A")
	b, err := json.Marshal(&m)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"swVersion":"CASIC V5.3.0.0"`,
		`"hwVersion":"HW-1234-REV-A"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON %s\nmissing %s", s, want)
		}
	}
}
