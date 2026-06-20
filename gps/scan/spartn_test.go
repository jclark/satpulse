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

// Real encrypted SPARTN frame (message 0-1) from a u-blox D9S receiver.
const spartnEx = "\x73\x00\x1f\xed\x13\x2a\x38\x5c\x2b\xc8\x87\x01\xa8\xdd\x92\x95\x78\x80\x66\x33\x97\xa9\x3f\x46\xdc\x6b\x3a\x9a\x64\x19\x89\x58\xfd\x5e\xc6\x66\x2e\xcd\x50\xa5\x4e\xe3\x83\xff\xc9\xcd\xcb\x9d\x3f\x13\xac\x19\xf5\x09\x80\x66\xf4\x63\x71\x31\xcf\x98\xd8\xde\xd1\xb7\xa8\x17\xc2\x2e\xba\x31\x3a\xea\xf6\x44"

func TestSpartn(t *testing.T) {
	buf, packetIndex := scantest.InsertRandomPrefix(spartnEx, spartn.PreambleByte)
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
}

// TestSpartnStrayPreamble confirms the scanner recovers from a stray preamble
// byte (0x73 = 's', common in text) in the lead-in and still finds the frame.
func TestSpartnStrayPreamble(t *testing.T) {
	junk := []byte("press s to start\x73\x00\x00")
	buf := append(junk, spartnEx...)
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
