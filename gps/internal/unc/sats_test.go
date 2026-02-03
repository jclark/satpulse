package unc

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

func TestSatsInfo(t *testing.T) {
	tests := []struct {
		name     string
		packet   string
		expected *gpsprot.SatellitesMsg
	}{
		{
			name:   "From UM980 in Thailand",
			packet: "#SATSINFOA,97,GPS,FINE,2379,523890000,0,0,18,16;42,2,0,0,0,63,29,327,25,0,34,0,2,0,29,9,2,24,195,6,0,30,0,3,0,32,14,3,0,30,9,3,5,341,52,0,32,0,2,0,35,9,2,13,131,56,0,48,0,2,0,43,9,2,15,196,48,0,45,0,2,0,44,9,2,12,227,37,0,43,0,2,0,41,9,2,25,263,28,0,35,0,3,0,45,14,3,0,42,9,3,199,116,56,5,37,0,3,5,46,14,3,5,45,9,3,194,149,27,5,38,0,3,5,41,14,3,5,41,9,3,42,197,69,1,50,0,2,1,47,5,2,56,17,37,1,28,0,1,46,251,10,1,33,0,2,1,36,5,2,55,82,30,1,23,0,1,53,201,7,1,31,0,2,1,29,5,2,41,29,55,1,27,0,2,1,30,5,2,59,105,43,4,31,21,2,4,33,13,2,3,142,71,4,47,0,3,4,46,17,3,4,44,21,3,5,254,39,4,42,0,3,4,43,17,3,4,39,21,3,60,241,62,4,48,0,3,4,48,21,3,4,48,13,3,2,231,65,4,46,0,3,4,49,17,3,4,41,21,3,16,163,15,4,37,0,3,4,41,17,3,4,36,21,3,10,197,85,4,45,0,3,4,49,17,3,4,44,21,3,13,321,55,4,40,0,3,4,37,17,3,4,37,21,3,39,154,17,4,39,0,5,4,35,21,5,4,36,8,5,4,36,12,5,4,38,13,5,8,335,53,4,39,0,3,4,41,17,3,4,38,21,3,40,131,63,4,46,0,5,4,46,21,5,4,43,8,5,4,46,12,5,4,48,13,5,7,151,72,4,46,0,3,4,47,17,3,4,43,21,3,6,168,16,4,33,0,3,4,36,17,3,4,35,21,3,28,115,17,4,26,21,2,4,25,12,2,30,350,47,4,30,21,3,4,30,12,3,4,34,13,3,32,21,67,4,41,0,5,4,38,21,5,4,37,8,5,4,37,12,5,4,39,13,5,20,223,50,4,46,0,5,4,44,21,5,4,46,8,5,4,48,12,5,4,49,13,5,27,80,50,4,29,0,3,4,26,21,3,4,26,12,3,9,181,13,4,33,0,3,4,39,17,3,4,29,21,3,36,13,37,3,27,17,2,3,25,12,2,2,255,40,3,46,2,3,3,46,17,3,3,44,12,3,9,69,42,3,23,2,3,3,30,17,3,3,24,12,3,5,155,53,3,46,2,3,3,51,17,3,3,48,12,3,3,199,14,3,34,2,3,3,32,17,3,3,29,12,3,34,316,21,3,32,2,3,3,32,17,3,3,26,12,3,25,190,30,3,35,2,3,3,45,17,3,3,43,12,3,16,170,15,3,26,2,3,3,34,17,3,3,33,12,3*50745955\r\n",
			expected: &gpsprot.SatellitesMsg{
				Tag:          TagAscii,
				NativeMsgID:  "SATSINFO",
				UsedValidity: gpsprot.SatelliteUsedInvalid,
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 29}, LookAngles: &gpsprot.LookAngles{Azimuth: 327, Elevation: 25}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 34}, {ID: gpsprot.SigIDGPSL2P, CN0: 29}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 24}, LookAngles: &gpsprot.LookAngles{Azimuth: 195, Elevation: 6}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 30}, {ID: gpsprot.SigIDGPSL5Q, CN0: 32}, {ID: gpsprot.SigIDGPSL2P, CN0: 30}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 341, Elevation: 52}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 32}, {ID: gpsprot.SigIDGPSL2P, CN0: 35}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 13}, LookAngles: &gpsprot.LookAngles{Azimuth: 131, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 48}, {ID: gpsprot.SigIDGPSL2P, CN0: 43}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 15}, LookAngles: &gpsprot.LookAngles{Azimuth: 196, Elevation: 48}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 45}, {ID: gpsprot.SigIDGPSL2P, CN0: 44}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 12}, LookAngles: &gpsprot.LookAngles{Azimuth: 227, Elevation: 37}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 43}, {ID: gpsprot.SigIDGPSL2P, CN0: 41}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 25}, LookAngles: &gpsprot.LookAngles{Azimuth: 263, Elevation: 28}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 35}, {ID: gpsprot.SigIDGPSL5Q, CN0: 45}, {ID: gpsprot.SigIDGPSL2P, CN0: 42}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 116, Elevation: 56}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDQZSSL1CA, CN0: 37}, {ID: gpsprot.SigIDQZSSL5Q, CN0: 46}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.QZSS, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 149, Elevation: 27}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDQZSSL1CA, CN0: 38}, {ID: gpsprot.SigIDQZSSL5Q, CN0: 41}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 197, Elevation: 69}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGLOL1, CN0: 50}, {ID: gpsprot.SigIDGLOL2, CN0: 47}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 19}, LookAngles: &gpsprot.LookAngles{Azimuth: 17, Elevation: 37}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGLOL1, CN0: 28}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 251, Elevation: 10}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGLOL1, CN0: 33}, {ID: gpsprot.SigIDGLOL2, CN0: 36}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 18}, LookAngles: &gpsprot.LookAngles{Azimuth: 82, Elevation: 30}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGLOL1, CN0: 23}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 201, Elevation: 7}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGLOL1, CN0: 31}, {ID: gpsprot.SigIDGLOL2, CN0: 29}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 4}, LookAngles: &gpsprot.LookAngles{Azimuth: 29, Elevation: 55}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGLOL1, CN0: 27}, {ID: gpsprot.SigIDGLOL2, CN0: 30}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 59}, LookAngles: &gpsprot.LookAngles{Azimuth: 105, Elevation: 43}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB3I, CN0: 31}, {ID: gpsprot.SigIDBDSB2bI, CN0: 33}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 3}, LookAngles: &gpsprot.LookAngles{Azimuth: 142, Elevation: 71}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 47}, {ID: gpsprot.SigIDBDSB2I, CN0: 46}, {ID: gpsprot.SigIDBDSB3I, CN0: 44}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 254, Elevation: 39}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 42}, {ID: gpsprot.SigIDBDSB2I, CN0: 43}, {ID: gpsprot.SigIDBDSB3I, CN0: 39}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 60}, LookAngles: &gpsprot.LookAngles{Azimuth: 241, Elevation: 62}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 48}, {ID: gpsprot.SigIDBDSB3I, CN0: 48}, {ID: gpsprot.SigIDBDSB2bI, CN0: 48}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 231, Elevation: 65}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 46}, {ID: gpsprot.SigIDBDSB2I, CN0: 49}, {ID: gpsprot.SigIDBDSB3I, CN0: 41}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 163, Elevation: 15}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 37}, {ID: gpsprot.SigIDBDSB2I, CN0: 41}, {ID: gpsprot.SigIDBDSB3I, CN0: 36}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 10}, LookAngles: &gpsprot.LookAngles{Azimuth: 197, Elevation: 85}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 45}, {ID: gpsprot.SigIDBDSB2I, CN0: 49}, {ID: gpsprot.SigIDBDSB3I, CN0: 44}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 13}, LookAngles: &gpsprot.LookAngles{Azimuth: 321, Elevation: 55}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 40}, {ID: gpsprot.SigIDBDSB2I, CN0: 37}, {ID: gpsprot.SigIDBDSB3I, CN0: 37}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 39}, LookAngles: &gpsprot.LookAngles{Azimuth: 154, Elevation: 17}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 39}, {ID: gpsprot.SigIDBDSB3I, CN0: 35}, {ID: gpsprot.SigIDBDSB1CP, CN0: 36}, {ID: gpsprot.SigIDBDSB2aP, CN0: 36}, {ID: gpsprot.SigIDBDSB2bI, CN0: 38}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 8}, LookAngles: &gpsprot.LookAngles{Azimuth: 335, Elevation: 53}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 39}, {ID: gpsprot.SigIDBDSB2I, CN0: 41}, {ID: gpsprot.SigIDBDSB3I, CN0: 38}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 40}, LookAngles: &gpsprot.LookAngles{Azimuth: 131, Elevation: 63}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 46}, {ID: gpsprot.SigIDBDSB3I, CN0: 46}, {ID: gpsprot.SigIDBDSB1CP, CN0: 43}, {ID: gpsprot.SigIDBDSB2aP, CN0: 46}, {ID: gpsprot.SigIDBDSB2bI, CN0: 48}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 7}, LookAngles: &gpsprot.LookAngles{Azimuth: 151, Elevation: 72}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 46}, {ID: gpsprot.SigIDBDSB2I, CN0: 47}, {ID: gpsprot.SigIDBDSB3I, CN0: 43}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 6}, LookAngles: &gpsprot.LookAngles{Azimuth: 168, Elevation: 16}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 33}, {ID: gpsprot.SigIDBDSB2I, CN0: 36}, {ID: gpsprot.SigIDBDSB3I, CN0: 35}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 28}, LookAngles: &gpsprot.LookAngles{Azimuth: 115, Elevation: 17}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB3I, CN0: 26}, {ID: gpsprot.SigIDBDSB2aP, CN0: 25}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 30}, LookAngles: &gpsprot.LookAngles{Azimuth: 350, Elevation: 47}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB3I, CN0: 30}, {ID: gpsprot.SigIDBDSB2aP, CN0: 30}, {ID: gpsprot.SigIDBDSB2bI, CN0: 34}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 32}, LookAngles: &gpsprot.LookAngles{Azimuth: 21, Elevation: 67}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 41}, {ID: gpsprot.SigIDBDSB3I, CN0: 38}, {ID: gpsprot.SigIDBDSB1CP, CN0: 37}, {ID: gpsprot.SigIDBDSB2aP, CN0: 37}, {ID: gpsprot.SigIDBDSB2bI, CN0: 39}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 20}, LookAngles: &gpsprot.LookAngles{Azimuth: 223, Elevation: 50}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 46}, {ID: gpsprot.SigIDBDSB3I, CN0: 44}, {ID: gpsprot.SigIDBDSB1CP, CN0: 46}, {ID: gpsprot.SigIDBDSB2aP, CN0: 48}, {ID: gpsprot.SigIDBDSB2bI, CN0: 49}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 27}, LookAngles: &gpsprot.LookAngles{Azimuth: 80, Elevation: 50}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 29}, {ID: gpsprot.SigIDBDSB3I, CN0: 26}, {ID: gpsprot.SigIDBDSB2aP, CN0: 26}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.BDS, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 181, Elevation: 13}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDBDSB1I, CN0: 33}, {ID: gpsprot.SigIDBDSB2I, CN0: 39}, {ID: gpsprot.SigIDBDSB3I, CN0: 29}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 36}, LookAngles: &gpsprot.LookAngles{Azimuth: 13, Elevation: 37}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGALE5bQ, CN0: 27}, {ID: gpsprot.SigIDGALE5aQ, CN0: 25}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 2}, LookAngles: &gpsprot.LookAngles{Azimuth: 255, Elevation: 40}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGALE1C, CN0: 46}, {ID: gpsprot.SigIDGALE5bQ, CN0: 46}, {ID: gpsprot.SigIDGALE5aQ, CN0: 44}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 9}, LookAngles: &gpsprot.LookAngles{Azimuth: 69, Elevation: 42}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGALE1C, CN0: 23}, {ID: gpsprot.SigIDGALE5bQ, CN0: 30}, {ID: gpsprot.SigIDGALE5aQ, CN0: 24}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 5}, LookAngles: &gpsprot.LookAngles{Azimuth: 155, Elevation: 53}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGALE1C, CN0: 46}, {ID: gpsprot.SigIDGALE5bQ, CN0: 51}, {ID: gpsprot.SigIDGALE5aQ, CN0: 48}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 3}, LookAngles: &gpsprot.LookAngles{Azimuth: 199, Elevation: 14}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGALE1C, CN0: 34}, {ID: gpsprot.SigIDGALE5bQ, CN0: 32}, {ID: gpsprot.SigIDGALE5aQ, CN0: 29}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 34}, LookAngles: &gpsprot.LookAngles{Azimuth: 316, Elevation: 21}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGALE1C, CN0: 32}, {ID: gpsprot.SigIDGALE5bQ, CN0: 32}, {ID: gpsprot.SigIDGALE5aQ, CN0: 26}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 25}, LookAngles: &gpsprot.LookAngles{Azimuth: 190, Elevation: 30}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGALE1C, CN0: 35}, {ID: gpsprot.SigIDGALE5bQ, CN0: 45}, {ID: gpsprot.SigIDGALE5aQ, CN0: 43}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 16}, LookAngles: &gpsprot.LookAngles{Azimuth: 170, Elevation: 15}, Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGALE1C, CN0: 26}, {ID: gpsprot.SigIDGALE5bQ, CN0: 34}, {ID: gpsprot.SigIDGALE5aQ, CN0: 33}}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the ASCII packet using uncmsg
			msg, err := uncmsg.ParseAsciiMessage([]byte(tt.packet))
			if err != nil {
				t.Fatalf("Failed to parse ASCII packet: %v", err)
			}

			_, ok := msg.Body.(*uncmsg.SatsInfo)
			if !ok {
				t.Fatalf("Expected SatsInfo message, got %T", msg.Body)
			}

			// Test the existing satellitesSatsInfo function
			result, err := satellitesMsgFromSatsInfo(&msg.Hdr, msg.Body.(*uncmsg.SatsInfo), TagAscii)
			if err != nil {
				t.Fatalf("Failed to convert SatsInfo: %v", err)
			}

			if !reflect.DeepEqual(result, tt.expected) {
				// Marshal the actual value as JSON for easy capture
				actualJSON, err := json.Marshal(result)
				if err != nil {
					t.Fatalf("Failed to marshal actual result to JSON: %v", err)
				}
				t.Errorf("SatellitesMsg does not match expected.\nActual value as JSON:\n%s", string(actualJSON))
			}

			// Verify binary packet size matches expected
			t.Run("binary size verification", func(t *testing.T) {
				// Count satellites and total frequencies
				satsInfo := msg.Body.(*uncmsg.SatsInfo)
				numSats := len(satsInfo.Sats)
				totalFreqs := 0
				for _, sat := range satsInfo.Sats {
					totalFreqs += len(sat.Freqs)
				}

				// Calculate expected binary size based on protocol.md
				expectedSize := 24 + // Header
					6 + // Fixed fields (Sat number, Version, 3 reserved, Frq flag)
					numSats*4 + // Per satellite (PRN, Azimuth, Elevation)
					totalFreqs*4 + // Per frequency (Sys status, SNR, Freq status, Freq No)
					4 // CRC

				// Serialize to binary to get actual size
				binaryPacket, err := uncmsg.SerializeBinMsg(msg)
				if err != nil {
					t.Fatalf("Failed to serialize to binary: %v", err)
				}

				actualSize := len(binaryPacket)
				if actualSize != expectedSize {
					t.Errorf("Binary packet size mismatch: got %d, expected %d (sats=%d, freqs=%d)",
						actualSize, expectedSize, numSats, totalFreqs)
				}
			})
		})
	}
}
