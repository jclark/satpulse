package septentrio

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/scantest"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
	"github.com/jclark/satpulse/gps/scan"
)

func makePacket(mid sbfbin.MsgID, body []byte) []byte {
	n := sbfbin.HeaderLen + len(body)
	pkt := make([]byte, sbfbin.HeaderLen, n)
	pkt[0] = sbfbin.Sync1
	pkt[1] = sbfbin.Sync2
	binary.LittleEndian.PutUint16(pkt[4:6], uint16(mid))
	binary.LittleEndian.PutUint16(pkt[6:8], uint16(n))
	pkt = append(pkt, body...)
	binary.LittleEndian.PutUint16(pkt[2:4], sbfbin.CRC16(pkt[4:]))
	return pkt
}

func minimalBody() []byte {
	return []byte{0x44, 0x33, 0x22, 0x11, 0x66, 0x55, 0, 0}
}

func TestPacketFormatValid(t *testing.T) {
	pkt := makePacket(sbfbin.EndOfMeasID, minimalBody())
	start, end, ok := scantest.FindPacket(PacketFormat, pkt)
	if start != 0 || end != len(pkt) || !ok {
		t.Fatalf("FindPacket = (%d, %d, %v), want (0, %d, true)", start, end, ok, len(pkt))
	}
	if !gpsprot.IsValidPacket(PacketFormat, pkt) {
		t.Fatalf("valid packet was rejected")
	}
	if !bytes.Equal(PacketFormat.ExtractChecksum(pkt), PacketFormat.ComputeChecksum(pkt)) {
		t.Fatalf("checksum mismatch")
	}
	if got := PacketFormat.MsgID(pkt); got != "EndOfMeas" {
		t.Fatalf("MsgID = %q, want %q", got, "EndOfMeas")
	}
}

func TestPacketFormatTruncatedHeader(t *testing.T) {
	if _, _, ok := scantest.FindPacket(PacketFormat, []byte{sbfbin.Sync1, sbfbin.Sync2, 0}); ok {
		t.Fatalf("truncated header scanned as a packet")
	}
}

func TestPacketFormatBadLength(t *testing.T) {
	for _, length := range []uint16{8, 10} {
		pkt := []byte{sbfbin.Sync1, sbfbin.Sync2, 0, 0, 0, 0, byte(length), byte(length >> 8)}
		if gpsprot.IsValidPacket(PacketFormat, pkt) {
			t.Fatalf("length %d scanned as valid", length)
		}
	}
}

func TestPacketFormatRescanOnBadChecksum(t *testing.T) {
	bad := makePacket(sbfbin.EndOfMeasID, minimalBody())
	bad[len(bad)-1] ^= 0x80
	good := makePacket(sbfbin.PVTGeodeticID, minimalBody())
	s := scan.New(bytes.NewReader(append(bad, good...)), 64, []gpsprot.PacketFormat{PacketFormat})
	pkt, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if pkt.Format != PacketFormat || pkt.ChecksumValid || pkt.Data != string(bad) {
		t.Fatalf("first scan did not return the checksum-bad packet")
	}
	pkt, err = s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if pkt.Format != PacketFormat || !pkt.ChecksumValid || pkt.Data != string(good) {
		t.Fatalf("second scan did not recover following packet")
	}
}

func TestPacketFormatEmbeddedSync(t *testing.T) {
	body := make([]byte, 1008)
	binary.LittleEndian.PutUint32(body[:4], 0x11223344)
	binary.LittleEndian.PutUint16(body[4:6], 0x5566)
	body[400] = sbfbin.Sync1
	body[401] = sbfbin.Sync2
	pkt := makePacket(sbfbin.ChannelStatusID, body)
	start, end, ok := scantest.FindPacket(PacketFormat, pkt)
	if start != 0 || end != len(pkt) || !ok {
		t.Fatalf("embedded sync split packet: start=%d end=%d ok=%v", start, end, ok)
	}
}

func TestPacketFormatRejectsOtherDollarFormats(t *testing.T) {
	for _, s := range []string{"$R", "$T", "$-", "$&", "$G"} {
		if _, _, ok := scantest.FindPacket(PacketFormat, []byte(s)); ok {
			t.Fatalf("%q scanned as SBF", s)
		}
	}
}

func TestPacketFormatMasksRevisionAndNamesUnknown(t *testing.T) {
	pkt := makePacket(sbfbin.PVTGeodeticID|2<<13, minimalBody())
	if got := PacketFormat.MsgID(pkt); got != "PVTGeodetic" {
		t.Fatalf("revision MsgID = %q, want %q", got, "PVTGeodetic")
	}
	pkt = makePacket(sbfbin.AuxAntPositionsID, minimalBody())
	if got := PacketFormat.MsgID(pkt); got != "AuxAntPositions" {
		t.Fatalf("unknown named MsgID = %q, want %q", got, "AuxAntPositions")
	}
}
