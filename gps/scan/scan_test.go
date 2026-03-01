package scan_test

import (
	"io"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/scan"
)

// byteByByteReader wraps a reader and returns one byte at a time
type byteByByteReader struct {
	r io.Reader
}

func (b *byteByByteReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	return b.r.Read(p[:1])
}

// TestInvalidBytesBatching verifies that when reading byte-by-byte,
// consecutive invalid bytes are batched reasonably rather than
// returned as many single-byte packets.
func TestInvalidBytesBatching(t *testing.T) {
	// Simulate the scenario from packet.ttyUSB0.jsonl:
	// partial NMEA data "0.00,0.00,180126,,,A,V*01\r\n" followed by a valid packet
	invalidPrefix := "0.00,0.00,180126,,,A,V*01\r\n"
	validNMEA := "$GPRMC,1,2,3*0F\r\n"
	data := invalidPrefix + validNMEA

	r := &byteByByteReader{r: strings.NewReader(data)}
	s := scan.New(r, 64, []gpsprot.PacketFormat{nmea.PacketFormat})

	// Count how many invalid packets we get before the valid one
	invalidPacketCount := 0
	totalInvalidBytes := 0

	for {
		pkt, err := s.Scan()
		if err == io.EOF {
			t.Fatal("unexpected EOF before finding valid packet")
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if pkt.Format == nil {
			invalidPacketCount++
			totalInvalidBytes += len(pkt.Data)
			continue
		}

		if !pkt.HasTag(nmea.Tag) {
			t.Fatalf("expected NMEA packet, got %v", pkt.Tag())
		}
		break
	}

	// Verify total invalid bytes is correct
	if totalInvalidBytes != len(invalidPrefix) {
		t.Errorf("total invalid bytes: got %d, want %d", totalInvalidBytes, len(invalidPrefix))
	}

	// Invalid bytes should be batched reasonably, not returned as
	// many single-byte packets. Allow up to 3 packets for batching.
	if invalidPacketCount > 3 {
		t.Errorf("too many invalid packets: got %d, want at most 3 (bytes should be batched)", invalidPacketCount)
	}
}
