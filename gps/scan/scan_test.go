package scan_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/scan"
)

const (
	fakeStateStarted gpsprot.ScanState = iota + gpsprot.ScanStateSync + 1
	fakeStateComplete
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

type lateRejectFormat struct{}

func (lateRejectFormat) Tag() gpsprot.Tag {
	return "LATE"
}

func (lateRejectFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	switch state {
	case gpsprot.ScanStateSync:
		if buf[nextScanIndex] == 'x' {
			return fakeStateStarted
		}
	case fakeStateStarted:
		if packetLen < 3 {
			return fakeStateStarted
		}
	}
	return gpsprot.ScanStateSync
}

func (lateRejectFormat) IsFinal(gpsprot.ScanState) bool {
	return false
}

func (lateRejectFormat) MsgID([]byte) string {
	return ""
}

func (lateRejectFormat) ExtractChecksum([]byte) []byte {
	return nil
}

func (lateRejectFormat) ComputeChecksum([]byte) []byte {
	return nil
}

func (lateRejectFormat) RescanOnBadChecksum(bool, []byte) bool {
	return false
}

func (lateRejectFormat) IsBinary() bool {
	return false
}

type fakeValidFormat struct{}

func (fakeValidFormat) Tag() gpsprot.Tag {
	return "VALID"
}

func (fakeValidFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	switch state {
	case gpsprot.ScanStateSync:
		if buf[nextScanIndex] == 'v' {
			return fakeStateStarted
		}
	case fakeStateStarted:
		if packetLen < 2 {
			return fakeStateStarted
		}
		return fakeStateComplete
	}
	return gpsprot.ScanStateSync
}

func (fakeValidFormat) IsFinal(state gpsprot.ScanState) bool {
	return state == fakeStateComplete
}

func (fakeValidFormat) MsgID([]byte) string {
	return ""
}

func (fakeValidFormat) ExtractChecksum([]byte) []byte {
	return nil
}

func (fakeValidFormat) ComputeChecksum([]byte) []byte {
	return nil
}

func (fakeValidFormat) RescanOnBadChecksum(bool, []byte) bool {
	return false
}

func (fakeValidFormat) IsBinary() bool {
	return false
}

func TestScanRestartsAtInteriorPacketStart(t *testing.T) {
	s := scan.New(strings.NewReader("xvab"), 16, []gpsprot.PacketFormat{lateRejectFormat{}, fakeValidFormat{}})
	pkt, err := s.Scan()
	if err != nil {
		t.Fatalf("scan invalid prefix: %v", err)
	}
	if pkt.Format != nil {
		t.Fatalf("first packet tag = %q, want invalid", pkt.Tag())
	}
	if pkt.Data != "x" {
		t.Fatalf("first packet data = %q, want %q", pkt.Data, "x")
	}
	pkt, err = s.Scan()
	if err != nil {
		t.Fatalf("scan valid packet: %v", err)
	}
	if !pkt.HasTag("VALID") {
		t.Fatalf("second packet tag = %q, want VALID", pkt.Tag())
	}
	if pkt.Data != "vab" {
		t.Fatalf("second packet data = %q, want %q", pkt.Data, "vab")
	}
}

type timeoutError struct{}

func (timeoutError) Error() string { return "read timeout" }

func (timeoutError) Timeout() bool { return true }

// scriptedReader returns a fixed sequence of reads. A step carrying an error
// returns that error with no data; otherwise it returns the step's bytes.
type scriptedReader struct {
	steps []scriptStep
	i     int
}

type scriptStep struct {
	data string
	err  error
}

func (r *scriptedReader) Read(p []byte) (int, error) {
	if r.i >= len(r.steps) {
		return 0, io.EOF
	}
	step := r.steps[r.i]
	r.i++
	if step.err != nil {
		return 0, step.err
	}
	return copy(p, step.data), nil
}

// TestScanSalvagesInteriorStartOnTimeout checks that a read timeout in the
// middle of a candidate packet still backs up to an interior packet start: the
// leading bytes come back as a plain invalid packet with no ReadError (the
// timeout is suppressed because those bytes were read successfully), and the
// interior packet is recovered on the next Scan.
func TestScanSalvagesInteriorStartOnTimeout(t *testing.T) {
	r := &scriptedReader{steps: []scriptStep{
		{data: "xv"},
		{err: timeoutError{}},
		{data: "ab"},
	}}
	s := scan.New(r, 16, []gpsprot.PacketFormat{lateRejectFormat{}, fakeValidFormat{}})

	pkt, err := s.Scan()
	if err != nil {
		t.Fatalf("scan invalid prefix: %v", err)
	}
	if pkt.Format != nil {
		t.Fatalf("first packet tag = %q, want invalid", pkt.Tag())
	}
	if pkt.ReadError != nil {
		t.Fatalf("first packet ReadError = %v, want nil (timeout suppressed by resync)", pkt.ReadError)
	}
	if pkt.Data != "x" {
		t.Fatalf("first packet data = %q, want %q", pkt.Data, "x")
	}
	pkt, err = s.Scan()
	if err != nil {
		t.Fatalf("scan recovered packet: %v", err)
	}
	if !pkt.HasTag("VALID") {
		t.Fatalf("second packet tag = %q, want VALID", pkt.Tag())
	}
	if pkt.Data != "vab" {
		t.Fatalf("second packet data = %q, want %q", pkt.Data, "vab")
	}
}

// TestScanReportsTimeoutWithNoInteriorStart checks the other arm: a mid-packet
// timeout with no possible interior start still surfaces as a ReadError
// wrapping the timeout, with the partial candidate returned as the packet data.
func TestScanReportsTimeoutWithNoInteriorStart(t *testing.T) {
	r := &scriptedReader{steps: []scriptStep{
		{data: "vz"},
		{err: timeoutError{}},
	}}
	s := scan.New(r, 16, []gpsprot.PacketFormat{lateRejectFormat{}, fakeValidFormat{}})

	pkt, err := s.Scan()
	if err != nil {
		t.Fatalf("scan: unexpected err %v", err)
	}
	if pkt.Format != nil {
		t.Fatalf("packet tag = %q, want invalid", pkt.Tag())
	}
	if !errors.Is(pkt.ReadError, timeoutError{}) {
		t.Fatalf("ReadError = %v, want wrapped timeout", pkt.ReadError)
	}
	if pkt.Data != "vz" {
		t.Fatalf("packet data = %q, want %q", pkt.Data, "vz")
	}
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

// TestAbbrevAsciiScanning replays the byte stream captured from a UM980
// responding to the LOGLIST command: the NMEA ack, the three-line
// abbreviated ASCII response, a stray CR/LF, and a following Unicore
// binary RECTIMEB packet.
func TestAbbrevAsciiScanning(t *testing.T) {
	stream := "$command,LOGLIST,response: OK*18\r\n" +
		"<LOGLIST COM3 17548 95.000000 FINE 2413 3196.000000 42155794 830 18\r\n" +
		"<\t1\r\n" +
		"<\tRECTIMEB COM3 1 \r\n" +
		"\r\n" +
		string(mustHexDecode("aa44b55f66002c0000a06d0948c830008c44000000121800000000004ee2998db8e03fbf75152bf986b14d3e00000000000032c0ea0700000405003478e6000001000000bded3313"))

	expect := []struct {
		tag           gpsprot.Tag
		data          string
		checksumValid bool
		altChecksum   bool // UM980 computes NMEA response checksums including the '$'
	}{
		{gpsreg.TagNMEA, "$command,LOGLIST,response: OK*18\r\n", false, true},
		{gpsreg.TagNovAtelAbbrevAscii, "<LOGLIST COM3 17548 95.000000 FINE 2413 3196.000000 42155794 830 18\r\n", true, false},
		{gpsreg.TagNovAtelAbbrevAscii, "<\t1\r\n", true, false},
		{gpsreg.TagNovAtelAbbrevAscii, "<\tRECTIMEB COM3 1 \r\n", true, false},
		{"", "\r\n", false, false},
		{gpsreg.TagUnicoreBin, "", true, false}, // data checked by tag only
	}

	s := scan.New(strings.NewReader(stream), 1024, gpsreg.CreatePacketFormats([]gpsreg.Vendor{gpsreg.VendorUnicore}))
	for i, e := range expect {
		pkt, err := s.Scan()
		if err != nil {
			t.Fatalf("packet %d: scan error: %v", i, err)
		}
		if pkt.Tag() != e.tag {
			t.Fatalf("packet %d: tag = %q, want %q (data %q)", i, pkt.Tag(), e.tag, pkt.Data)
		}
		if e.data != "" && pkt.Data != e.data {
			t.Errorf("packet %d: data = %q, want %q", i, pkt.Data, e.data)
		}
		if e.tag != "" && pkt.ChecksumValid != e.checksumValid {
			t.Errorf("packet %d: ChecksumValid = %v, want %v", i, pkt.ChecksumValid, e.checksumValid)
		}
		if e.altChecksum && !pkt.AltChecksumValid() {
			t.Errorf("packet %d: AltChecksumValid = false, want true", i)
		}
	}
}
