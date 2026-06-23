package scan_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/scantest"
	"github.com/jclark/satpulse/gps/internal/spartn"
	"github.com/jclark/satpulse/gps/scan"
)

var spartnFormats = []gpsprot.PacketFormat{spartn.PacketFormat}

// spartnFrames holds real, unencrypted SPARTN frames captured from a u-blox
// PointPerfect IP service, one per message type/subtype present in the capture.
var spartnFrames = []struct {
	name   string
	hexstr string
	msg    string // expected MsgID ("type-subtype")
}{
	{"ocb-gps", "73000f21036ce013ce8021a0811017f0b1a2fdf3245fbf7a8bf7f2117f0f822fddb345fc22e88506dd", "0-0"},
	{"ocb-glonass", "73000d2014bdd013cec0030a0617f05ea25e8e745fb6808bf623117f3d222fdcbe403e2af6", "0-1"},
	{"ocb-galileo", "73000fa8236ce013ce81001c4020a0be7f7117d274a2f9f0645fbf628bf8c9917ef71a2f9e9d40c5db1f", "0-2"},
	{"ocb-beidou", "73001027336c7013ce8020810006018815b0baa2fe08545fc0de8bf7a9d17efe5a2fdf8145fc13289dd5f3", "0-3"},
	{"hpac-gps", "73020da208f7e7f98013ce8013a0141804e021a0811009f7054f82a7c15650a67091802a95578310", "1-0"},
	{"hpac-glonass", "73020cab18f7e94a7013ce8013a0141804e0030a0633511b4a832e35978eda089913e0ecdcb1", "1-1"},
	{"hpac-galileo", "73020e2328f7e7f98013ce8013a0141804e00038804140a5f58ff627311485b2a40d958894301ef12f", "1-2"},
	{"hpac-beidou", "73020eaa38f7e7f91013ce8013a0141804e08204001806202b3b54a1aaab4529e2ac24575a29348a7093", "1-3"},
	{"gad", "730404a108f7e7f98013ce8013b651bc9b43405f9140", "2-0"},
}

func TestSpartn(t *testing.T) {
	for _, tt := range spartnFrames {
		t.Run(tt.name, func(t *testing.T) {
			buf, packetIndex := scantest.InsertRandomPrefix(string(mustHexDecode(tt.hexstr)), spartn.PreambleByte)
			r := bytes.NewReader(buf)
			result := make([]byte, 0, len(buf))
			scanner := scan.New(r, 10, spartnFormats)
			found := false
			for {
				pkt, err := scanner.Scan()
				if pkt.HasTag(spartn.Tag) {
					if len(result) != packetIndex {
						t.Fatalf("Expected SPARTN after %d bytes, but got after %d bytes", packetIndex, len(result))
					}
					if id := spartn.PacketFormat.MsgID([]byte(pkt.Data)); id != tt.msg {
						t.Fatalf("MsgID = %q, want %q", id, tt.msg)
					}
					found = true
				}
				result = append(result, []byte(pkt.Data)...)
				if err != nil {
					if err == io.EOF {
						break
					}
					t.Fatalf("Error scanning packet: %v", err)
				}
			}
			if !bytes.Equal(result, buf) {
				t.Fatalf("Expected %x, but got %x", buf, result)
			}
			if !found {
				t.Fatalf("No SPARTN packet found")
			}
		})
	}
}

// TestSpartnStrayPreamble confirms the scanner recovers from a stray preamble
// byte (0x73 = 's', common in text) in the lead-in and still finds the frame.
func TestSpartnStrayPreamble(t *testing.T) {
	junk := []byte("press s to start\x73\x00\x00")
	buf := append(junk, mustHexDecode(spartnFrames[0].hexstr)...)
	r := bytes.NewReader(buf)
	result := make([]byte, 0, len(buf))
	scanner := scan.New(r, 10, spartnFormats)
	found := false
	for {
		pkt, err := scanner.Scan()
		if pkt.HasTag(spartn.Tag) {
			if len(result) != len(junk) {
				t.Fatalf("Expected SPARTN after %d bytes, but got after %d bytes", len(junk), len(result))
			}
			found = true
		}
		result = append(result, []byte(pkt.Data)...)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("Error scanning packet: %v", err)
		}
	}
	if !bytes.Equal(result, buf) {
		t.Fatalf("Expected %x, but got %x", buf, result)
	}
	if !found {
		t.Fatalf("No SPARTN packet found after stray preamble")
	}
}
