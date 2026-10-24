package ntrip

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/scan"
)

// makeRTCM builds a valid RTCM packet with the given message type and
// payload length (excluding the 3-byte header and 3-byte CRC).
func makeRTCM(msgType uint16, payloadLen int) []byte {
	totalPayload := max(payloadLen+3, 3)  // +3 for header
	pkt := make([]byte, 3+totalPayload+3) // preamble+len + payload + crc
	pkt[0] = 0xD3
	pkt[1] = byte(totalPayload >> 8)
	pkt[2] = byte(totalPayload)
	pkt[3] = byte(msgType >> 4)
	pkt[4] = byte(msgType << 4)
	crc := rtcmbin.Checksum(pkt[:3+totalPayload])
	copy(pkt[3+totalPayload:], crc[:])
	return pkt
}

// makeRTCMMSM builds a valid RTCM MSM packet with the multiple message
// bit set or clear.  payloadLen must be >= 10 to hold the MSM header.
func makeRTCMMSM(msgType uint16, mmb bool, payloadLen int) []byte {
	pkt := makeRTCM(msgType, max(payloadLen, 10))
	if mmb {
		pkt[9] |= 0x02
	}
	// recompute CRC
	totalPayload := len(pkt) - 6
	crc := rtcmbin.Checksum(pkt[:3+totalPayload])
	copy(pkt[3+totalPayload:], crc[:])
	return pkt
}

type fakePacketFormat struct {
	tag   gpsprot.Tag
	msgID string
}

func (f fakePacketFormat) Tag() gpsprot.Tag { return f.tag }

func (f fakePacketFormat) Next(state gpsprot.ScanState, _ []byte, _, _ int) gpsprot.ScanState {
	return state
}

func (f fakePacketFormat) IsFinal(gpsprot.ScanState) bool { return true }
func (f fakePacketFormat) MsgID([]byte) string            { return f.msgID }
func (f fakePacketFormat) ExtractChecksum([]byte) []byte  { return nil }
func (f fakePacketFormat) ComputeChecksum([]byte) []byte  { return nil }

func (f fakePacketFormat) RescanOnBadChecksum(bool, []byte) bool {
	return false
}

func (f fakePacketFormat) IsBinary() bool { return false }

func TestPruningQueueDropsStale(t *testing.T) {
	var q PruningQueue
	pf := rtcm.PacketFormat
	pkt1005a := makeRTCM(1005, 10)
	pkt1077 := makeRTCM(1077, 10)
	pkt1005b := makeRTCM(1005, 12)
	q.Enqueue(scan.Packet{Format: pf, Data: string(pkt1005a)})
	q.Enqueue(scan.Packet{Format: pf, Data: string(pkt1077)})
	q.Enqueue(scan.Packet{Format: pf, Data: string(pkt1005b)})
	if q.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", q.Len())
	}
	// dequeue should return 1077 first (oldest surviving), then 1005b
	p1 := q.Dequeue()
	if p1.Format.MsgID([]byte(p1.Data)) != "1077" {
		t.Errorf("expected 1077 first, got %s", p1.Format.MsgID([]byte(p1.Data)))
	}
	p2 := q.Dequeue()
	if p2.Format.MsgID([]byte(p2.Data)) != "1005" {
		t.Errorf("expected 1005 second, got %s", p2.Format.MsgID([]byte(p2.Data)))
	}
	if p2.Data != string(pkt1005b) {
		t.Errorf("expected updated 1005 packet")
	}
}

func TestPruningQueueDoesNotPruneNonRTCM(t *testing.T) {
	var q PruningQueue
	pf := fakePacketFormat{tag: "NMEA", msgID: "GSV"}
	q.Enqueue(scan.Packet{Format: pf, Data: "first", ChecksumValid: true})
	q.Enqueue(scan.Packet{Format: pf, Data: "second", ChecksumValid: true})
	if q.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", q.Len())
	}
	if got := q.Dequeue().Data; got != "first" {
		t.Errorf("first dequeue = %q, want first", got)
	}
	if got := q.Dequeue().Data; got != "second" {
		t.Errorf("second dequeue = %q, want second", got)
	}
}

func TestPruningQueuePrunesOnlyRTCMInMixedStream(t *testing.T) {
	var q PruningQueue
	rtcmPF := rtcm.PacketFormat
	nmeaPF := fakePacketFormat{tag: "NMEA", msgID: "GSV"}
	oldRTCM := string(makeRTCM(1005, 10))
	newRTCM := string(makeRTCM(1005, 12))
	q.Enqueue(scan.Packet{Format: rtcmPF, Data: oldRTCM, ChecksumValid: true})
	q.Enqueue(scan.Packet{Format: nmeaPF, Data: "$GPGSV,1", ChecksumValid: true})
	q.Enqueue(scan.Packet{Format: rtcmPF, Data: newRTCM, ChecksumValid: true})
	q.Enqueue(scan.Packet{Format: nmeaPF, Data: "$GPGSV,2", ChecksumValid: true})
	if q.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", q.Len())
	}
	want := []string{"$GPGSV,1", newRTCM, "$GPGSV,2"}
	for i := range want {
		if got := q.Dequeue().Data; got != want[i] {
			t.Fatalf("dequeue %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestPruningQueueKeepsUnformattedFIFO(t *testing.T) {
	var q PruningQueue
	q.Enqueue(scan.Packet{Data: "raw1"})
	q.Enqueue(scan.Packet{Data: "raw2"})
	if q.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", q.Len())
	}
	if got := q.Dequeue().Data; got != "raw1" {
		t.Errorf("first dequeue = %q, want raw1", got)
	}
	if got := q.Dequeue().Data; got != "raw2" {
		t.Errorf("second dequeue = %q, want raw2", got)
	}
}

func TestPruningQueueDropsOldestAtMaxLen(t *testing.T) {
	var q PruningQueue
	var dropped scan.Packet
	var ok bool
	for i := range PruningQueueMaxLen + 3 {
		if pkt, droppedOK := q.Enqueue(scan.Packet{Data: string([]byte{byte(i)})}); droppedOK {
			dropped = pkt
			ok = true
		}
	}
	if !ok {
		t.Fatal("expected a dropped packet")
	}
	if got := dropped.Data[0]; got != 2 {
		t.Errorf("last dropped packet = %d, want 2", got)
	}
	if q.Len() != PruningQueueMaxLen {
		t.Fatalf("expected %d entries, got %d", PruningQueueMaxLen, q.Len())
	}
	if got := q.Dequeue().Data[0]; got != 3 {
		t.Errorf("oldest surviving packet = %d, want 3", got)
	}
}

func TestPruningQueueSameEpochNotPruned(t *testing.T) {
	var q PruningQueue
	pf := rtcm.PacketFormat
	// Epoch 0: 1077(more), 1087(more), 1077(more) -- same msgID twice in same epoch
	pkt1077a := makeRTCMMSM(1077, true, 20)
	pkt1087 := makeRTCMMSM(1087, true, 20)
	pkt1077b := makeRTCMMSM(1077, true, 22)
	q.Enqueue(scan.Packet{Format: pf, Data: string(pkt1077a)})
	q.Enqueue(scan.Packet{Format: pf, Data: string(pkt1087)})
	q.Enqueue(scan.Packet{Format: pf, Data: string(pkt1077b)})
	if q.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", q.Len())
	}
	p1 := q.Dequeue()
	if p1.Data != string(pkt1077a) {
		t.Error("expected first 1077 packet")
	}
	p2 := q.Dequeue()
	if p2.Data != string(pkt1087) {
		t.Error("expected 1087 packet")
	}
	p3 := q.Dequeue()
	if p3.Data != string(pkt1077b) {
		t.Error("expected second 1077 packet")
	}
}

func TestPruningQueueNewEpochPrunesOld(t *testing.T) {
	var q PruningQueue
	pf := rtcm.PacketFormat
	// Epoch 0: 1077(more), 1087(last)
	old1077 := makeRTCMMSM(1077, true, 20)
	old1087 := makeRTCMMSM(1087, false, 20)
	q.Enqueue(scan.Packet{Format: pf, Data: string(old1077)})
	q.Enqueue(scan.Packet{Format: pf, Data: string(old1087)})
	if q.Len() != 2 {
		t.Fatalf("expected 2 entries after epoch 0, got %d", q.Len())
	}
	// Epoch 1: 1077(more), 1087(last) -- should replace epoch 0
	new1077 := makeRTCMMSM(1077, true, 22)
	new1087 := makeRTCMMSM(1087, false, 22)
	q.Enqueue(scan.Packet{Format: pf, Data: string(new1077)})
	q.Enqueue(scan.Packet{Format: pf, Data: string(new1087)})
	if q.Len() != 2 {
		t.Fatalf("expected 2 entries after epoch 1, got %d", q.Len())
	}
	p1 := q.Dequeue()
	if p1.Data != string(new1077) {
		t.Error("expected new 1077 packet")
	}
	p2 := q.Dequeue()
	if p2.Data != string(new1087) {
		t.Error("expected new 1087 packet")
	}
}

func TestPruningQueueNewEpochPrunesDuplicateMsgIDs(t *testing.T) {
	var q PruningQueue
	pf := rtcm.PacketFormat
	// Epoch 0: two 1077 packets (split) + 1087
	old1077a := makeRTCMMSM(1077, true, 20)
	old1077b := makeRTCMMSM(1077, true, 22)
	old1087 := makeRTCMMSM(1087, false, 20)
	q.Enqueue(scan.Packet{Format: pf, Data: string(old1077a)})
	q.Enqueue(scan.Packet{Format: pf, Data: string(old1077b)})
	q.Enqueue(scan.Packet{Format: pf, Data: string(old1087)})
	if q.Len() != 3 {
		t.Fatalf("expected 3 entries after epoch 0, got %d", q.Len())
	}
	// Epoch 1: single 1077 -- must prune both old 1077 entries
	new1077 := makeRTCMMSM(1077, false, 24)
	q.Enqueue(scan.Packet{Format: pf, Data: string(new1077)})
	// old 1087 remains (different msgID), both old 1077s pruned
	if q.Len() != 2 {
		t.Fatalf("expected 2 entries after epoch 1, got %d", q.Len())
	}
	p1 := q.Dequeue()
	if p1.Format.MsgID([]byte(p1.Data)) != "1087" {
		t.Errorf("expected 1087 first, got %s", p1.Format.MsgID([]byte(p1.Data)))
	}
	p2 := q.Dequeue()
	if p2.Data != string(new1077) {
		t.Error("expected new 1077 packet")
	}
}

func TestPruningQueueMSMAndNonMSM(t *testing.T) {
	var q PruningQueue
	pf := rtcm.PacketFormat
	// Non-MSM 1005 should still be pruned normally
	pkt1005a := makeRTCM(1005, 10)
	pkt1077 := makeRTCMMSM(1077, false, 20)
	pkt1005b := makeRTCM(1005, 12)
	q.Enqueue(scan.Packet{Format: pf, Data: string(pkt1005a)})
	q.Enqueue(scan.Packet{Format: pf, Data: string(pkt1077)})
	q.Enqueue(scan.Packet{Format: pf, Data: string(pkt1005b)})
	if q.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", q.Len())
	}
	p1 := q.Dequeue()
	if p1.Format.MsgID([]byte(p1.Data)) != "1077" {
		t.Errorf("expected 1077 first, got %s", p1.Format.MsgID([]byte(p1.Data)))
	}
	p2 := q.Dequeue()
	if p2.Data != string(pkt1005b) {
		t.Error("expected updated 1005 packet")
	}
}
