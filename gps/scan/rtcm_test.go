package scan_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/scantest"
	"github.com/jclark/satpulse/gps/scan"
)

var rtcmFormats = []gpsprot.PacketFormat{rtcm.PacketFormat}

const rtcmEx = "\xD3\x00\x13\x3E\xD7\xD3\x02\x02\x98\x0E\xDE\xEF\x34\xB4\xBD\x62\xAC\x09\x41\x98\x6F\x33\x36\x0B\x98"

func TestRtcm(t *testing.T) {
	buf, packetIndex := scantest.InsertRandomPrefix(rtcmEx, 0xD3)
	r := bytes.NewReader(buf)
	result := make([]byte, 0, len(buf))
	scanner := scan.New(r, 10, rtcmFormats)
	found := false
	for {
		pkt, err := scanner.Scan()
		if pkt.HasTag(rtcm.Tag) {
			if len(result) != packetIndex {
				t.Fatalf("Expected RTCM after %d bytes, but got after %d bytes", packetIndex, len(result))
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
	if len(result) != len(buf) {
		t.Fatalf("Expected %d bytes, but got %d bytes, packetIndex %d", len(buf), len(result), packetIndex)
	}
	if !bytes.Equal(result, buf) {
		t.Fatalf("Expected %x, but got %x", buf, result)
	}
	if !found {
		t.Fatalf("No RTCM packet found")
	}

}

func FuzzScan(f *testing.F) {
	tc, _ := scantest.InsertRandomPrefix(rtcmEx, 0xD3)
	tc[len(tc)-1]++ // make it wrong checksum
	tc = append(tc, ([]byte)(randomUBXPacket())...)
	f.Add(tc)
	tc = []byte{}
	for range 10 {
		tc = append(tc, byte(0xD3))
		b, _ := scantest.InsertRandomPrefix(randomUBXPacket(), 0)
		tc = append(tc, b...)
	}
	for i := 0; i < len(tc); i += 13 {
		tc[i]++
	}
	f.Add(tc)
	f.Fuzz(func(t *testing.T, buf []byte) {
		r := bytes.NewReader(buf)
		result := make([]byte, 0, len(buf))
		scanner := scan.New(r, 10, rtcmFormats)
		for {
			pkt, err := scanner.Scan()
			result = append(result, []byte(pkt.Data)...)
			if err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("Error scanning packet: %v", err)
			}
		}
		if len(result) != len(buf) {
			t.Fatalf("Expected %d bytes, but got %d bytes", len(buf), len(result))
		}
		if !bytes.Equal(result, buf) {
			t.Fatalf("Produced different output for %x,", buf)
		}
	})
}
