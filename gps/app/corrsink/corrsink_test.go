package corrsink

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jclark/crc24q"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/scan"
)

// makeRTCM builds a valid RTCM packet with the given message type and
// payload length (excluding the 3-byte header and 3-byte CRC).
func makeRTCM(msgType uint16, payloadLen int) []byte {
	totalPayload := max(payloadLen+3, 3) // +3 for header
	pkt := make([]byte, 3+totalPayload+3) // preamble+len + payload + crc
	pkt[0] = 0xD3
	pkt[1] = byte(totalPayload >> 8)
	pkt[2] = byte(totalPayload)
	pkt[3] = byte(msgType >> 4)
	pkt[4] = byte(msgType << 4)
	crc := crc24q.Checksum(pkt[:3+totalPayload])
	pkt[3+totalPayload] = byte(crc >> 16)
	pkt[3+totalPayload+1] = byte(crc >> 8)
	pkt[3+totalPayload+2] = byte(crc)
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
	crc := crc24q.Checksum(pkt[:3+totalPayload])
	pkt[3+totalPayload] = byte(crc >> 16)
	pkt[3+totalPayload+1] = byte(crc >> 8)
	pkt[3+totalPayload+2] = byte(crc)
	return pkt
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

func (s *pipeSource) Connect(ctx context.Context) (net.Conn, error) {
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

func (mockOutPort) Write(p []byte) (int, error)           { return len(p), nil }
func (mockOutPort) Buffered() (int, error)                 { return 0, nil }
func (mockOutPort) TransmitTime(nBytes int) time.Duration  { return 0 }

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
	sink := NewSink()
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

func TestCleanShutdownOnCancel(t *testing.T) {
	src, client := newPipeSource()
	defer src.close()
	defer client.Close()
	mw := &mockWriter{}
	portLock := gpsio.NewOutPortLock(mockOutPort{})
	sink := NewSink()
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
	sink := NewSink()
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
	sink := NewSink()
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
	sink := NewSink()
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

func (s *reconnectSource) Connect(ctx context.Context) (net.Conn, error) {
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
