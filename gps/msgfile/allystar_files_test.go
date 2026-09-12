package msgfile

import (
	"path/filepath"
	"testing"

	"github.com/jclark/satpulse/gps/lib/asbin"
)

func TestAllystarModelFiles(t *testing.T) {
	tests := []struct {
		name       string
		wantRate   byte
		wantRTCM   bool
		wantEnable int
	}{
		{name: "tau1201", wantRate: 1, wantEnable: 18},
		{name: "tau13xx", wantRate: 1, wantRTCM: true, wantEnable: 27},
		{name: "tau951m", wantRate: 5, wantRTCM: true, wantEnable: 27},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("..", "..", "configs", "gpsmsg", "allystar", tc.name+".toml")
			mf, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := mf.ValidateTags(); err != nil {
				t.Fatal(err)
			}

			enables := 0
			hasRTCM := false
			hasGSTEnable := false
			hasGSTDisable := false
			var lastNMEAOff []byte
			for i := range mf.ASBIN {
				m := &mf.ASBIN[i]
				if m.Class != 0x06 || m.ID != 0x01 {
					continue
				}
				payload, err := m.Payload.Encode(asbin.Endian)
				if err != nil {
					t.Fatalf("tag %q: %v", m.getTag(), err)
				}
				if len(payload) != 3 {
					t.Fatalf("tag %q: CFG-MSG payload length %d, want 3", m.getTag(), len(payload))
				}
				if payload[0] == 0xF8 {
					hasRTCM = true
				}
				switch m.getTag() {
				case "nmea-gst":
					hasGSTEnable = payload[0] == 0xF0 && payload[1] == 0x08 && payload[2] == tc.wantRate
				case "nmea-gst-off":
					hasGSTDisable = payload[0] == 0xF0 && payload[1] == 0x08 && payload[2] == 0
				case "nmea-off":
					lastNMEAOff = payload
				}
				if payload[2] == 0 {
					continue
				}
				enables++
				if payload[2] != tc.wantRate {
					t.Errorf("tag %q: CFG-MSG rate %d, want %d", m.getTag(), payload[2], tc.wantRate)
				}
			}
			if enables != tc.wantEnable {
				t.Errorf("enabled CFG-MSG count %d, want %d", enables, tc.wantEnable)
			}
			if hasRTCM != tc.wantRTCM {
				t.Errorf("RTCM presence %v, want %v", hasRTCM, tc.wantRTCM)
			}
			if !hasGSTEnable || !hasGSTDisable {
				t.Error("missing valid nmea-gst or nmea-gst-off tag")
			}
			if len(lastNMEAOff) != 3 || lastNMEAOff[0] != 0xF0 || lastNMEAOff[1] != 0x08 || lastNMEAOff[2] != 0 {
				t.Errorf("last nmea-off payload % X, want F0 08 00", lastNMEAOff)
			}
		})
	}
}
