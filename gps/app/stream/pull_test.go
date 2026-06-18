package stream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
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

const testRTCM1005 = "\xD3\x00\x13\x3E\xD7\xD3\x02\x02\x98\x0E\xDE\xEF\x34\xB4\xBD\x62\xAC\x09\x41\x98\x6F\x33\x36\x0B\x98"

func TestCorReportFromPacket(t *testing.T) {
	tRead := time.Unix(1, 2)
	pkt := scan.Packet{
		Format:        rtcm.PacketFormat,
		Data:          testRTCM1005,
		TRead:         tRead,
		ChecksumValid: true,
	}

	msg, err := CorReportFromPacket(pkt)
	if err != nil {
		t.Fatalf("CorReportFromPacket returned error: %v", err)
	}
	if msg == nil {
		t.Fatal("CorReportFromPacket returned nil")
	}
	if msg.Source != gpsprot.CorReportSourcePull {
		t.Errorf("Source = %v, want %v", msg.Source, gpsprot.CorReportSourcePull)
	}
	if msg.Tag != rtcm.Tag || msg.MsgID != "1005" {
		t.Errorf("Tag/MsgID = %q/%q, want RTCM/1005", msg.Tag, msg.MsgID)
	}
	if !msg.NBytes.IsSet() || msg.NBytes.Get() != len(testRTCM1005) {
		t.Errorf("NBytes = (%v, %v), want set %d", msg.NBytes.Get(), msg.NBytes.IsSet(), len(testRTCM1005))
	}
	if !msg.ChecksumOK.IsSet() || !msg.ChecksumOK.Get() {
		t.Errorf("ChecksumOK = (%v, %v), want set true", msg.ChecksumOK.Get(), msg.ChecksumOK.IsSet())
	}
	if msg.FinalFragment.IsSet() {
		t.Errorf("FinalFragment set for non-MSM packet: %v", msg.FinalFragment.Get())
	}
	wantBaseID, ok := rtcmbin.ReferenceStationID(testRTCM1005)
	if !ok {
		t.Fatal("ReferenceStationID returned false for test packet")
	}
	if !msg.RTCMRefBaseID.IsSet() || msg.RTCMRefBaseID.Get() != wantBaseID {
		t.Errorf("RTCMRefBaseID = (%v, %v), want set %d",
			msg.RTCMRefBaseID.Get(), msg.RTCMRefBaseID.IsSet(), wantBaseID)
	}
	if _, ok := msg.NativeMsg.(*rtcmbin.MT1005); !ok {
		t.Fatalf("NativeMsg = %T, want *rtcmbin.MT1005", msg.NativeMsg)
	}
}

func TestCorReportFromPacketInvalidChecksum(t *testing.T) {
	data := []byte(testRTCM1005)
	data[len(data)-1] ^= 0x01
	pkt := scan.Packet{
		Format:        rtcm.PacketFormat,
		Data:          string(data),
		ChecksumValid: false,
	}

	msg, err := CorReportFromPacket(pkt)
	if err != nil {
		t.Fatalf("CorReportFromPacket returned error: %v", err)
	}
	if msg == nil {
		t.Fatal("CorReportFromPacket returned nil")
	}
	if msg.MsgID != "1005" {
		t.Errorf("MsgID = %q, want 1005", msg.MsgID)
	}
	if !msg.ChecksumOK.IsSet() || msg.ChecksumOK.Get() {
		t.Errorf("ChecksumOK = (%v, %v), want set false", msg.ChecksumOK.Get(), msg.ChecksumOK.IsSet())
	}
	if msg.NativeMsg != nil {
		t.Errorf("NativeMsg = %T, want nil", msg.NativeMsg)
	}
	if msg.RTCMRefBaseID.IsSet() {
		t.Errorf("RTCMRefBaseID set for invalid checksum: %d", msg.RTCMRefBaseID.Get())
	}
}

func TestCorReportFromPacketMSMFinalFragment(t *testing.T) {
	tests := []struct {
		name string
		mmb  bool
		want bool
	}{
		{"more follow", true, false},
		{"final", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := makeRTCMMSM(1077, tt.mmb, 19)
			msg, err := CorReportFromPacket(scan.Packet{
				Format:        rtcm.PacketFormat,
				Data:          string(data),
				ChecksumValid: true,
			})
			if err != nil {
				t.Fatalf("CorReportFromPacket returned error: %v", err)
			}
			if msg == nil {
				t.Fatal("CorReportFromPacket returned nil")
			}
			if !msg.FinalFragment.IsSet() || msg.FinalFragment.Get() != tt.want {
				t.Errorf("FinalFragment = (%v, %v), want set %v",
					msg.FinalFragment.Get(), msg.FinalFragment.IsSet(), tt.want)
			}
		})
	}
}

func TestCorReportFromPacketNonRTCM(t *testing.T) {
	msg, err := CorReportFromPacket(scan.Packet{})
	if err != nil {
		t.Fatalf("CorReportFromPacket returned error: %v", err)
	}
	if msg != nil {
		t.Fatalf("CorReportFromPacket returned %#v, want nil", msg)
	}
}

// pipeSource returns a Source backed by net.Pipe.  The caller writes
// correction data to the returned net.Conn.
type pipeSource struct {
	mu   sync.Mutex
	conn net.Conn
}

func newPipeSource() (*pipeSource, net.Conn) {
	server, client := net.Pipe()
	return &pipeSource{conn: server}, client
}

func (s *pipeSource) Connect(ctx context.Context) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return nil, errors.New("closed")
	}
	return s.conn, nil
}

func (s *pipeSource) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

// mockWriter records WritePacket calls.
type mockWriter struct {
	mu   sync.Mutex
	pkts []string
	err  error // if set, WritePacket returns this
}

func (w *mockWriter) WritePacket(p []byte, _ gpsprot.PacketFormat) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return 0, w.err
	}
	w.pkts = append(w.pkts, string(p))
	return len(p), nil
}

func (w *mockWriter) packets() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]string, len(w.pkts))
	copy(cp, w.pkts)
	return cp
}

func (w *mockWriter) reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pkts = nil
}

func (w *mockWriter) setError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.err = err
}

// mockOutPort satisfies gpsio.OutPort for NewOutPortLock.
type mockOutPort struct{}

func (mockOutPort) Write(p []byte) (int, error) { return len(p), nil }
func (mockOutPort) Buffered() (int, error)      { return 0, nil }
func (mockOutPort) ReadOnly() bool              { return false }
func (mockOutPort) Direct() bool                { return false }

func testLogger() *slog.Logger {
	return slog.Default()
}

// connectedCh returns an onState callback and a channel that closes
// when the sink reaches Connected state.
func connectedCh() (func(State, error), <-chan struct{}) {
	ch := make(chan struct{})
	var once sync.Once
	return func(st State, _ error) {
		if st == Connected {
			once.Do(func() { close(ch) })
		}
	}, ch
}

// waitPackets polls mw until it has at least n packets or the deadline
// expires.
func waitPackets(t *testing.T, mw *mockWriter, n int) []string {
	t.Helper()
	pkts, ok := waitPacketsWithin(mw, n, 5*time.Second)
	if !ok {
		t.Fatalf("timed out waiting for %d packets, got %d", n, len(pkts))
	}
	return pkts
}

func waitPacketsWithin(mw *mockWriter, n int, timeout time.Duration) ([]string, bool) {
	deadline := time.After(timeout)
	for {
		pkts := mw.packets()
		if len(pkts) >= n {
			return pkts, true
		}
		select {
		case <-deadline:
			return pkts, false
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func primeSink(t *testing.T, client net.Conn, mw *mockWriter) {
	t.Helper()
	// Connected only means the source dial succeeded. Send warm-up packets until
	// one reaches the writer so the rest of the test starts from a live pipeline.
	warmup := makeRTCM(1230, 10)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Write(warmup); err != nil {
			t.Fatalf("warm-up write failed: %v", err)
		}
		if _, ok := waitPacketsWithin(mw, 1, 100*time.Millisecond); ok {
			mw.reset()
			return
		}
	}
	t.Fatal("timed out warming up sink pipeline")
}

func TestPacketsFlowToWriter(t *testing.T) {
	src, client := newPipeSource()
	defer src.close()
	defer client.Close()
	mw := &mockWriter{}
	portLock := gpsio.NewOutPortLock(mockOutPort{})
	sink := NewPull()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pkt1005 := makeRTCM(1005, 16)
	pkt1077 := makeRTCM(1077, 16)
	onState, connected := connectedCh()
	done := make(chan error, 1)
	go func() {
		done <- sink.Run(ctx, testLogger(), src, mw, portLock,
			[]gpsprot.PacketFormat{rtcm.PacketFormat}, onState)
	}()
	<-connected
	primeSink(t, client, mw)
	if _, err := client.Write(pkt1005); err != nil {
		t.Fatalf("failed to write first packet: %v", err)
	}
	pkts := waitPackets(t, mw, 1)
	if pkts[0] != string(pkt1005) {
		t.Errorf("first packet mismatch")
	}
	if _, err := client.Write(pkt1077); err != nil {
		t.Fatalf("failed to write second packet: %v", err)
	}
	pkts = waitPackets(t, mw, 2)
	if pkts[1] != string(pkt1077) {
		t.Errorf("second packet mismatch")
	}
	cancel()
	runErr := <-done
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		t.Errorf("unexpected Run error: %v", runErr)
	}
}

func TestReadLoopLogsInvalidChecksum(t *testing.T) {
	valid := makeRTCM(1005, 16)
	invalid := append([]byte(nil), valid...)
	invalid[len(invalid)-1] ^= 0x01
	var logBuf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logBuf, nil))
	sink := &Pull{pktCh: make(chan scan.Packet, 3)}
	err := sink.readLoop(context.Background(), lg, io.NopCloser(bytes.NewReader(append(valid, invalid...))),
		[]gpsprot.PacketFormat{rtcm.PacketFormat}, newBackoff())
	if err != nil {
		t.Fatalf("readLoop returned error: %v", err)
	}
	gotLog := logBuf.String()
	if !strings.Contains(gotLog, "invalid correction checksum") {
		t.Fatalf("log output %q does not contain invalid checksum message", gotLog)
	}
	if !strings.Contains(gotLog, "msg=1005") {
		t.Fatalf("log output %q does not contain RTCM message ID", gotLog)
	}
	if !strings.Contains(gotLog, "tag=RTCM") {
		t.Fatalf("log output %q does not contain packet tag", gotLog)
	}
	pkt := <-sink.pktCh
	if !pkt.ChecksumValid {
		t.Fatal("first packet checksum invalid, want valid")
	}
	pkt = <-sink.pktCh
	if pkt.ChecksumValid {
		t.Fatal("second packet checksum valid, want invalid")
	}
}

func TestPruningQueueDropsStale(t *testing.T) {
	var q pruningQueue
	pf := rtcm.PacketFormat
	pkt1005a := makeRTCM(1005, 10)
	pkt1077 := makeRTCM(1077, 10)
	pkt1005b := makeRTCM(1005, 12)
	q.enqueue(scan.Packet{Format: pf, Data: string(pkt1005a)})
	q.enqueue(scan.Packet{Format: pf, Data: string(pkt1077)})
	q.enqueue(scan.Packet{Format: pf, Data: string(pkt1005b)})
	if q.len() != 2 {
		t.Fatalf("expected 2 entries, got %d", q.len())
	}
	// dequeue should return 1077 first (oldest surviving), then 1005b
	p1 := q.dequeue()
	if p1.Format.MsgID([]byte(p1.Data)) != "1077" {
		t.Errorf("expected 1077 first, got %s", p1.Format.MsgID([]byte(p1.Data)))
	}
	p2 := q.dequeue()
	if p2.Format.MsgID([]byte(p2.Data)) != "1005" {
		t.Errorf("expected 1005 second, got %s", p2.Format.MsgID([]byte(p2.Data)))
	}
	if p2.Data != string(pkt1005b) {
		t.Errorf("expected updated 1005 packet")
	}
}

func TestPruningQueueSameEpochNotPruned(t *testing.T) {
	var q pruningQueue
	pf := rtcm.PacketFormat
	// Epoch 0: 1077(more), 1087(more), 1077(more) -- same msgID twice in same epoch
	pkt1077a := makeRTCMMSM(1077, true, 20)
	pkt1087 := makeRTCMMSM(1087, true, 20)
	pkt1077b := makeRTCMMSM(1077, true, 22)
	q.enqueue(scan.Packet{Format: pf, Data: string(pkt1077a)})
	q.enqueue(scan.Packet{Format: pf, Data: string(pkt1087)})
	q.enqueue(scan.Packet{Format: pf, Data: string(pkt1077b)})
	if q.len() != 3 {
		t.Fatalf("expected 3 entries, got %d", q.len())
	}
	p1 := q.dequeue()
	if p1.Data != string(pkt1077a) {
		t.Error("expected first 1077 packet")
	}
	p2 := q.dequeue()
	if p2.Data != string(pkt1087) {
		t.Error("expected 1087 packet")
	}
	p3 := q.dequeue()
	if p3.Data != string(pkt1077b) {
		t.Error("expected second 1077 packet")
	}
}

func TestPruningQueueNewEpochPrunesOld(t *testing.T) {
	var q pruningQueue
	pf := rtcm.PacketFormat
	// Epoch 0: 1077(more), 1087(last)
	old1077 := makeRTCMMSM(1077, true, 20)
	old1087 := makeRTCMMSM(1087, false, 20)
	q.enqueue(scan.Packet{Format: pf, Data: string(old1077)})
	q.enqueue(scan.Packet{Format: pf, Data: string(old1087)})
	if q.len() != 2 {
		t.Fatalf("expected 2 entries after epoch 0, got %d", q.len())
	}
	// Epoch 1: 1077(more), 1087(last) -- should replace epoch 0
	new1077 := makeRTCMMSM(1077, true, 22)
	new1087 := makeRTCMMSM(1087, false, 22)
	q.enqueue(scan.Packet{Format: pf, Data: string(new1077)})
	q.enqueue(scan.Packet{Format: pf, Data: string(new1087)})
	if q.len() != 2 {
		t.Fatalf("expected 2 entries after epoch 1, got %d", q.len())
	}
	p1 := q.dequeue()
	if p1.Data != string(new1077) {
		t.Error("expected new 1077 packet")
	}
	p2 := q.dequeue()
	if p2.Data != string(new1087) {
		t.Error("expected new 1087 packet")
	}
}

func TestPruningQueueNewEpochPrunesDuplicateMsgIDs(t *testing.T) {
	var q pruningQueue
	pf := rtcm.PacketFormat
	// Epoch 0: two 1077 packets (split) + 1087
	old1077a := makeRTCMMSM(1077, true, 20)
	old1077b := makeRTCMMSM(1077, true, 22)
	old1087 := makeRTCMMSM(1087, false, 20)
	q.enqueue(scan.Packet{Format: pf, Data: string(old1077a)})
	q.enqueue(scan.Packet{Format: pf, Data: string(old1077b)})
	q.enqueue(scan.Packet{Format: pf, Data: string(old1087)})
	if q.len() != 3 {
		t.Fatalf("expected 3 entries after epoch 0, got %d", q.len())
	}
	// Epoch 1: single 1077 -- must prune both old 1077 entries
	new1077 := makeRTCMMSM(1077, false, 24)
	q.enqueue(scan.Packet{Format: pf, Data: string(new1077)})
	// old 1087 remains (different msgID), both old 1077s pruned
	if q.len() != 2 {
		t.Fatalf("expected 2 entries after epoch 1, got %d", q.len())
	}
	p1 := q.dequeue()
	if p1.Format.MsgID([]byte(p1.Data)) != "1087" {
		t.Errorf("expected 1087 first, got %s", p1.Format.MsgID([]byte(p1.Data)))
	}
	p2 := q.dequeue()
	if p2.Data != string(new1077) {
		t.Error("expected new 1077 packet")
	}
}

func TestPruningQueueMSMAndNonMSM(t *testing.T) {
	var q pruningQueue
	pf := rtcm.PacketFormat
	// Non-MSM 1005 should still be pruned normally
	pkt1005a := makeRTCM(1005, 10)
	pkt1077 := makeRTCMMSM(1077, false, 20)
	pkt1005b := makeRTCM(1005, 12)
	q.enqueue(scan.Packet{Format: pf, Data: string(pkt1005a)})
	q.enqueue(scan.Packet{Format: pf, Data: string(pkt1077)})
	q.enqueue(scan.Packet{Format: pf, Data: string(pkt1005b)})
	if q.len() != 2 {
		t.Fatalf("expected 2 entries, got %d", q.len())
	}
	p1 := q.dequeue()
	if p1.Format.MsgID([]byte(p1.Data)) != "1077" {
		t.Errorf("expected 1077 first, got %s", p1.Format.MsgID([]byte(p1.Data)))
	}
	p2 := q.dequeue()
	if p2.Data != string(pkt1005b) {
		t.Error("expected updated 1005 packet")
	}
}

// TestPullQueuePrunesUnderBackpressure drives the queue goroutine with a writer
// side that never drains -- exactly a stalled serial sink -- and confirms it
// drops stale same-type corrections rather than buffering them all. The
// TestPruningQueue* tests above exercise the data structure directly; this one
// exercises the live queue() select loop under backpressure, which is what the
// pruning queue exists for.
func TestPullQueuePrunesUnderBackpressure(t *testing.T) {
	sink := NewPull()
	subCh := make(chan scan.Packet)
	// Same single-slot buffer as Run uses; leaving it undrained is the stall.
	writerCh := make(chan scan.Packet, 1)
	done := make(chan struct{})
	go func() {
		sink.queue(subCh, writerCh)
		close(done)
	}()
	defer func() {
		close(subCh)
		<-done
	}()

	pf := rtcm.PacketFormat
	const n = 12
	// A burst of distinct 1005 corrections (non-MSM, so deduped by message
	// type) while nothing drains writerCh: all but the couple already past the
	// queue pile up, and the stale ones are dropped.
	var newest string
	for i := range n {
		pkt := makeRTCM(1005, 10+i)
		newest = string(pkt)
		subCh <- scan.Packet{Format: pf, Data: string(pkt), ChecksumValid: true}
	}
	// A different-type sentinel marks the end of the burst in FIFO order.
	sentinel := makeRTCM(1006, 10)
	subCh <- scan.Packet{Format: pf, Data: string(sentinel), ChecksumValid: true}

	var got []scan.Packet
	timeout := time.After(time.Second)
	for {
		var pkt scan.Packet
		select {
		case pkt = <-writerCh:
		case <-timeout:
			t.Fatalf("timed out waiting for sentinel after %d packets", len(got))
		}
		if pkt.Data == string(sentinel) {
			break
		}
		got = append(got, pkt)
	}

	if len(got) >= n {
		t.Fatalf("queue did not prune under backpressure: %d of %d 1005 packets reached the writer", len(got), n)
	}
	if len(got) == 0 || got[len(got)-1].Data != newest {
		t.Fatalf("newest 1005 did not survive pruning: %d packets through, last != newest", len(got))
	}
}

func TestCleanShutdownOnCancel(t *testing.T) {
	src, client := newPipeSource()
	defer src.close()
	defer client.Close()
	mw := &mockWriter{}
	portLock := gpsio.NewOutPortLock(mockOutPort{})
	sink := NewPull()
	ctx, cancel := context.WithCancel(context.Background())
	onState, connected := connectedCh()
	done := make(chan error, 1)
	go func() {
		done <- sink.Run(ctx, testLogger(), src, mw, portLock,
			[]gpsprot.PacketFormat{rtcm.PacketFormat}, onState)
	}()
	<-connected
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestWriteErrorTriggersShutdown(t *testing.T) {
	src, client := newPipeSource()
	defer src.close()
	defer client.Close()
	writeErr := errors.New("serial port gone")
	mw := &mockWriter{}
	portLock := gpsio.NewOutPortLock(mockOutPort{})
	sink := NewPull()
	onState, connected := connectedCh()
	done := make(chan error, 1)
	go func() {
		done <- sink.Run(t.Context(), testLogger(), src, mw, portLock,
			[]gpsprot.PacketFormat{rtcm.PacketFormat}, onState)
	}()
	<-connected
	primeSink(t, client, mw)
	mw.setError(writeErr)
	if _, err := client.Write(makeRTCM(1005, 10)); err != nil {
		t.Fatalf("failed to write packet: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, writeErr) {
			t.Errorf("expected write error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not shut down after write error")
	}
}

func TestPortLockAcquiredPerWrite(t *testing.T) {
	src, client := newPipeSource()
	defer src.close()
	defer client.Close()
	mw := &mockWriter{}
	portLock := gpsio.NewOutPortLock(mockOutPort{})
	sink := NewPull()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	onState, connected := connectedCh()
	done := make(chan error, 1)
	go func() {
		done <- sink.Run(ctx, testLogger(), src, mw, portLock,
			[]gpsprot.PacketFormat{rtcm.PacketFormat}, onState)
	}()
	<-connected
	primeSink(t, client, mw)
	if _, err := client.Write(makeRTCM(1005, 10)); err != nil {
		t.Fatalf("failed to write first packet: %v", err)
	}
	waitPackets(t, mw, 1)
	if _, err := client.Write(makeRTCM(1077, 10)); err != nil {
		t.Fatalf("failed to write second packet: %v", err)
	}
	waitPackets(t, mw, 2)
	// verify portLock is available (not leaked)
	select {
	case port := <-portLock:
		portLock <- port
	case <-time.After(time.Second):
		t.Fatal("portLock leaked")
	}
	cancel()
	err := <-done
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("unexpected Run error: %v", err)
	}
}

func TestReconnectOnNetworkError(t *testing.T) {
	// use a reconnectable source that provides a new pipe each time
	rs := &reconnectSource{}
	mw := &mockWriter{}
	portLock := gpsio.NewOutPortLock(mockOutPort{})
	sink := NewPull()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var states []State
	var mu sync.Mutex
	onState := func(st State, _ error) {
		mu.Lock()
		states = append(states, st)
		mu.Unlock()
	}
	done := make(chan error, 1)
	go func() {
		done <- sink.Run(ctx, testLogger(), rs, mw, portLock,
			[]gpsprot.PacketFormat{rtcm.PacketFormat}, onState)
	}()
	// write a packet on the first connection, then close it
	conn1 := rs.waitConn(t)
	primeSink(t, conn1, mw)
	if _, err := conn1.Write(makeRTCM(1005, 10)); err != nil {
		t.Fatalf("failed to write first connection packet: %v", err)
	}
	waitPackets(t, mw, 1)
	conn1.Close()
	// wait for reconnect and second connection
	conn2 := rs.waitConn(t)
	if _, err := conn2.Write(makeRTCM(1077, 10)); err != nil {
		t.Fatalf("failed to write second connection packet: %v", err)
	}
	waitPackets(t, mw, 2)
	conn2.Close()
	cancel()
	err := <-done
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("unexpected Run error: %v", err)
	}
	// check state transitions include reconnecting
	mu.Lock()
	defer mu.Unlock()
	hasReconnecting := false
	for _, st := range states {
		if st == Reconnecting {
			hasReconnecting = true
		}
	}
	if !hasReconnecting {
		t.Errorf("expected Reconnecting state, got %v", states)
	}
}

// reconnectSource creates a new net.Pipe on each Connect call.
type reconnectSource struct {
	mu    sync.Mutex
	conns chan net.Conn
	once  sync.Once
}

func (s *reconnectSource) init() {
	s.conns = make(chan net.Conn, 10)
}

func (s *reconnectSource) Connect(ctx context.Context) (io.ReadCloser, error) {
	s.once.Do(s.init)
	server, client := net.Pipe()
	s.mu.Lock()
	s.conns <- client
	s.mu.Unlock()
	return server, nil
}

func (s *reconnectSource) waitConn(t *testing.T) net.Conn {
	t.Helper()
	s.once.Do(s.init)
	select {
	case c := <-s.conns:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for connection")
		return nil
	}
}

// ntripListener wraps a local TCP listener that scripts a single
// Ntrip handshake.  It captures the request bytes and writes the
// configured response.
type ntripListener struct {
	ln       net.Listener
	respond  func(conn net.Conn, req []byte)
	reqBytes []byte
	mu       sync.Mutex
	wg       sync.WaitGroup
}

func newNtripListener(t *testing.T, respond func(conn net.Conn, req []byte)) *ntripListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	l := &ntripListener{ln: ln, respond: respond}
	l.wg.Go(l.accept)
	return l
}

func (l *ntripListener) accept() {
	conn, err := l.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	br := bufio.NewReader(conn)
	var req []byte
	for {
		line, err := br.ReadString('\n')
		req = append(req, line...)
		if err != nil {
			break
		}
		if line == "\r\n" {
			break
		}
	}
	l.mu.Lock()
	l.reqBytes = req
	l.mu.Unlock()
	if l.respond != nil {
		l.respond(conn, req)
	}
}

func (l *ntripListener) addr() string {
	return l.ln.Addr().String()
}

func (l *ntripListener) request() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return string(l.reqBytes)
}

func (l *ntripListener) close() {
	l.ln.Close()
	l.wg.Wait()
}

func TestNtripConnectV1Handshake(t *testing.T) {
	// Single write combines status and body so the buffered-body
	// (over-read) branch is exercised deterministically.
	body := []byte{0xD3, 0x00, 0x04, 0x41, 0x02, 0x03, 0x04, 0x99, 0x88, 0x77}
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write(append([]byte("ICY 200 OK\r\n"), body...))
	})
	defer ln.close()
	src := &NtripSource{Addr: ln.addr(), Mountpoint: "MNT"}
	rc, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("body mismatch: got %x, want %x", got, body)
	}
}

func TestNtripConnectV1HandshakeSplitWrites(t *testing.T) {
	body := []byte{0xD3, 0x00, 0x04, 0x41, 0x02, 0x03, 0x04, 0x99, 0x88, 0x77}
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ICY 200 OK\r\n"))
		time.Sleep(50 * time.Millisecond)
		conn.Write(body)
	})
	defer ln.close()
	src := &NtripSource{Addr: ln.addr(), Mountpoint: "MNT"}
	rc, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("body mismatch: got %x, want %x", got, body)
	}
}

func TestNtripRequestHeaders(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ICY 200 OK\r\n"))
	})
	defer ln.close()
	src := &NtripSource{
		Addr:       ln.addr(),
		Mountpoint: "MNT",
		Username:   "user",
		Password:   "pw",
		UserAgent:  NtripUserAgent{Version: "1.2.3"},
	}
	rc, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	rc.Close()
	// give accept goroutine time to record the request
	deadline := time.Now().Add(time.Second)
	var req string
	for time.Now().Before(deadline) {
		req = ln.request()
		if req != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.HasPrefix(req, "GET /MNT HTTP/1.0\r\n") {
		t.Errorf("bad request line: %q", req)
	}
	if !strings.Contains(req, "\r\nUser-Agent: NTRIP satpulse/1.2.3\r\n") {
		t.Errorf("missing User-Agent: %q", req)
	}
	wantCreds := base64.StdEncoding.EncodeToString([]byte("user:pw"))
	if !strings.Contains(req, "\r\nAuthorization: Basic "+wantCreds+"\r\n") {
		t.Errorf("missing/wrong Authorization: %q", req)
	}
	if !strings.HasSuffix(req, "\r\n\r\n") {
		t.Errorf("request not terminated by blank line: %q", req)
	}
}

func TestNtripUserAgentNoVersion(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ICY 200 OK\r\n"))
	})
	defer ln.close()
	src := &NtripSource{Addr: ln.addr(), Mountpoint: "MNT"}
	rc, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	rc.Close()
	deadline := time.Now().Add(time.Second)
	var req string
	for time.Now().Before(deadline) {
		req = ln.request()
		if req != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(req, "\r\nUser-Agent: NTRIP satpulse\r\n") {
		t.Errorf("unexpected User-Agent: %q", req)
	}
}

func TestNtripNoAuthHeaderWhenNoUsername(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ICY 200 OK\r\n"))
	})
	defer ln.close()
	src := &NtripSource{Addr: ln.addr(), Mountpoint: "MNT", Password: "ignored"}
	rc, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	rc.Close()
	deadline := time.Now().Add(time.Second)
	var req string
	for time.Now().Before(deadline) {
		req = ln.request()
		if req != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Contains(req, "Authorization:") {
		t.Errorf("unexpected Authorization header: %q", req)
	}
}

func TestNtripErrorResponse(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ERROR - Bad Password\r\n"))
	})
	defer ln.close()
	src := &NtripSource{Addr: ln.addr(), Mountpoint: "MNT"}
	_, err := src.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ERROR - Bad Password") {
		t.Errorf("error did not mention status: %v", err)
	}
}

func TestNtripHTTPErrorResponse(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n"))
	})
	defer ln.close()
	src := &NtripSource{Addr: ln.addr(), Mountpoint: "MNT"}
	_, err := src.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP/1.1 401 Unauthorized") {
		t.Errorf("error did not contain status: %v", err)
	}
}

func TestNtripCtxCancelMidHandshake(t *testing.T) {
	// listener that accepts but never responds
	release := make(chan struct{})
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		<-release
	})
	defer func() {
		close(release)
		ln.close()
	}()
	src := &NtripSource{Addr: ln.addr(), Mountpoint: "MNT"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := src.Connect(ctx)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after ctx cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Connect did not return after ctx cancel")
	}
}

func TestNtripConnectionRefused(t *testing.T) {
	// Pick an unused port by listening and immediately closing.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	src := &NtripSource{Addr: addr, Mountpoint: "MNT"}
	_, err = src.Connect(context.Background())
	if err == nil {
		t.Fatal("expected dial error")
	}
}
