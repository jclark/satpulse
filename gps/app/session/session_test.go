package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

// fakeConn implements gpsio.Conn. Reads block until data is sent with
// send or the connection is killed, which makes Read return io.EOF as
// an unplugged or reset device does.
type fakeConn struct {
	mu      sync.Mutex
	pending []byte
	dataCh  chan []byte
	closed  chan struct{}
	stopped bool
	writes  [][]byte
}

var _ gpsio.Conn = (*fakeConn)(nil)

func newFakeConn() *fakeConn {
	return &fakeConn{dataCh: make(chan []byte), closed: make(chan struct{})}
}

func (c *fakeConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		c.mu.Unlock()
		return n, nil
	}
	c.mu.Unlock()
	select {
	case b := <-c.dataCh:
		n := copy(p, b)
		c.mu.Lock()
		c.pending = append(c.pending, b[n:]...)
		c.mu.Unlock()
		return n, nil
	case <-c.closed:
		return 0, io.EOF
	}
}

func (c *fakeConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, append([]byte(nil), b...))
	return len(b), nil
}

func (c *fakeConn) Buffered() (int, error) { return 0, nil }
func (c *fakeConn) Drain() error           { return nil }
func (c *fakeConn) ReadOnly() bool         { return false }
func (c *fakeConn) Direct() bool           { return true }
func (c *fakeConn) LocalAddr() string      { return "fake" }
func (c *fakeConn) Stop()                  { c.kill() }
func (c *fakeConn) Close() error           { c.kill(); return nil }

// kill makes Read return EOF, as when the device is unplugged or a
// reset re-enumerates it.
func (c *fakeConn) kill() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		c.stopped = true
		close(c.closed)
	}
}

func (c *fakeConn) send(b []byte) {
	select {
	case c.dataCh <- b:
	case <-c.closed:
	}
}

func (c *fakeConn) writeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.writes)
}

func (c *fakeConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopped
}

// fakeOpener hands out queued fakeConns, one per Open call.
type fakeOpener struct {
	mu     sync.Mutex
	conns  []*fakeConn
	socket bool
	opens  int
}

func (o *fakeOpener) Open(_ context.Context) (gpsio.Conn, int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.opens++
	if len(o.conns) == 0 {
		return nil, 0, errors.New("no more conns")
	}
	c := o.conns[0]
	o.conns = o.conns[1:]
	return c, 9600, nil
}

func (o *fakeOpener) Socket() bool { return o.socket }

func (o *fakeOpener) openCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.opens
}

// blockingOpener is a fakeOpener whose Open blocks until gate is
// closed, so a test can interleave calls while an open is pending.
type blockingOpener struct {
	fakeOpener
	gate chan struct{}
}

func (o *blockingOpener) Open(ctx context.Context) (gpsio.Conn, int, error) {
	<-o.gate
	return o.fakeOpener.Open(ctx)
}

// gatedSink wraps fakeSink, blocking gps:state emissions on gate
// while gating is enabled, so a test can hold an emission in flight
// across another transition.
type gatedSink struct {
	fakeSink
	gating atomic.Bool
	gate   chan struct{}
}

// reentrantSink disconnects synchronously on the first state event.
type reentrantSink struct {
	fakeSink
	s    *Session
	once sync.Once
}

func (rs *reentrantSink) Emit(ev Event) {
	rs.fakeSink.Emit(ev)
	if ev.Name == EventState {
		rs.once.Do(rs.s.Disconnect)
	}
}

// blockingWriter holds writes until gate is closed and counts how many
// writes have entered.
type blockingWriter struct {
	gate    chan struct{}
	entered chan struct{}
	calls   atomic.Int32
}

func (w *blockingWriter) Write(b []byte) (int, error) {
	w.calls.Add(1)
	w.entered <- struct{}{}
	<-w.gate
	return len(b), nil
}

func (gs *gatedSink) Emit(ev Event) {
	if ev.Name == EventState && gs.gating.Load() {
		<-gs.gate
	}
	gs.fakeSink.Emit(ev)
}

// fakeSink records emitted events. Wants reports wantPacket for
// gps:packet and true for everything else.
type fakeSink struct {
	mu         sync.Mutex
	events     []Event
	wantPacket bool
}

func (fs *fakeSink) Emit(ev Event) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.events = append(fs.events, ev)
}

func (fs *fakeSink) Wants(name EventName) bool {
	return name != EventPacket || fs.wantPacket
}

// states returns the payloads of the gps:state events in emission order.
func (fs *fakeSink) states() []ConnState {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	var sts []ConnState
	for _, ev := range fs.events {
		if ev.Name == EventState {
			sts = append(sts, ev.Data.(ConnState))
		}
	}
	return sts
}

func (fs *fakeSink) count(name EventName) int {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	n := 0
	for _, ev := range fs.events {
		if ev.Name == name {
			n++
		}
	}
	return n
}

// testSession creates a Session and arranges for it to be disconnected
// at the end of the test, so a failing synctest test does not leave
// blocked goroutines in the bubble.
func testSession(t *testing.T, fs *fakeSink) *Session {
	s := New(slog.New(slog.DiscardHandler), fs, Options{})
	t.Cleanup(s.Disconnect)
	return s
}

// waitForState lets the session run (advancing the fake clock) until it
// reaches the wanted state.
func waitForState(t *testing.T, s *Session, want ConnState) {
	t.Helper()
	for range 20000 {
		synctest.Wait()
		if s.State() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state = %v, want %v", s.State(), want)
}

// makeGGA returns a valid NMEA GGA sentence with a fix.
func makeGGA() []byte {
	body := "GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,"
	return fmt.Appendf(nil, "$%s*%02X\r\n", body, nmeamsg.Checksum([]byte(body)))
}

func TestConnectDisconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fs := &fakeSink{}
		s := testSession(t, fs)
		op := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
		if err := s.Connect(op, gpsreg.VendorUnknown); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		waitForState(t, s, StateConnected)
		r := s.Receiver()
		if !r.OK || r.Warning == "" {
			t.Errorf("Receiver() = %+v, want OK with a warning (nothing detected)", r)
		}
		if got := s.Speed(); got != 9600 {
			t.Errorf("Speed() = %d, want 9600", got)
		}
		s.Disconnect()
		if got := s.Receiver(); !reflect.DeepEqual(got, ReceiverEvent{}) {
			t.Errorf("Receiver() after Disconnect = %+v, want zero", got)
		}
		expect := []ConnState{StateConnecting, StateConnected, StateDisconnected}
		if got := fs.states(); !reflect.DeepEqual(got, expect) {
			t.Errorf("state events = %v, want %v", got, expect)
		}
	})
}

// TestConnectSuperseded checks the supersession contract: a Connect or
// Disconnect issued while another Connect's open is pending wins; the
// pending attempt returns an error, closes its late-opened transport,
// and emits no state events of its own.
func TestConnectSuperseded(t *testing.T) {
	tests := []struct {
		name         string
		interleave   func(t *testing.T, s *Session)
		expectStates []ConnState
	}{
		{
			name:         "disconnect during open",
			interleave:   func(t *testing.T, s *Session) { s.Disconnect() },
			expectStates: []ConnState{StateConnecting, StateDisconnected},
		},
		{
			name: "connect during open",
			interleave: func(t *testing.T, s *Session) {
				op := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
				if err := s.Connect(op, gpsreg.VendorUnknown); err != nil {
					t.Fatalf("second Connect: %v", err)
				}
				waitForState(t, s, StateConnected)
			},
			expectStates: []ConnState{StateConnecting, StateConnecting, StateConnected},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fs := &fakeSink{}
				s := testSession(t, fs)
				conn := newFakeConn()
				op := &blockingOpener{fakeOpener: fakeOpener{conns: []*fakeConn{conn}}, gate: make(chan struct{})}
				errCh := make(chan error, 1)
				go func() { errCh <- s.Connect(op, gpsreg.VendorUnknown) }()
				synctest.Wait()
				tc.interleave(t, s)
				close(op.gate)
				if err := <-errCh; err == nil {
					t.Fatal("superseded Connect returned nil, want error")
				}
				synctest.Wait()
				if !conn.isClosed() {
					t.Error("superseded attempt's conn was not closed")
				}
				if got := fs.states(); !reflect.DeepEqual(got, tc.expectStates) {
					t.Errorf("state events = %v, want %v", got, tc.expectStates)
				}
			})
		})
	}
}

func TestLifecycleCallsDuringShutdown(t *testing.T) {
	fs := &fakeSink{}
	s := testSession(t, fs)
	drain := make(chan struct{})
	s.connWg.Go(func() { <-drain })
	op1 := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
	op2 := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
	err1 := make(chan error, 1)
	err2 := make(chan error, 1)
	go func() { err1 <- s.Connect(op1, gpsreg.VendorUnknown) }()
	waitGen := func(want int) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for {
			s.mu.Lock()
			got := s.connectGen
			s.mu.Unlock()
			if got == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("connect generation = %d, want %d", got, want)
			}
			time.Sleep(time.Millisecond)
		}
	}
	waitGen(1)
	go func() { err2 <- s.Connect(op2, gpsreg.VendorUnknown) }()
	waitGen(2)
	close(drain)
	if err := <-err1; err == nil {
		t.Fatal("first Connect returned nil, want superseded error")
	}
	if err := <-err2; err != nil {
		t.Fatalf("second Connect: %v", err)
	}
	if got := op1.openCount(); got != 0 {
		t.Errorf("first opener called %d times, want 0", got)
	}
	if got := op2.openCount(); got != 1 {
		t.Errorf("second opener called %d times, want 1", got)
	}
}

func TestStaleLifecycleCallDoesNotCloseWinner(t *testing.T) {
	tests := []struct {
		name string
		call func(s *Session, gen int, op Opener) error
	}{
		{
			name: "connect",
			call: func(s *Session, gen int, op Opener) error {
				return s.connect(gen, op, gpsreg.VendorUnknown)
			},
		},
		{
			name: "disconnect",
			call: func(s *Session, gen int, _ Opener) error {
				s.disconnect(gen)
				return nil
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fs := &fakeSink{}
				s := testSession(t, fs)
				staleGen := s.reserveLifecycle()
				winnerConn := newFakeConn()
				winnerOp := &fakeOpener{conns: []*fakeConn{winnerConn}}
				if err := s.Connect(winnerOp, gpsreg.VendorUnknown); err != nil {
					t.Fatalf("winning Connect: %v", err)
				}
				waitForState(t, s, StateConnected)
				staleOp := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
				err := tc.call(s, staleGen, staleOp)
				if tc.name == "connect" && err == nil {
					t.Fatal("stale Connect returned nil, want superseded error")
				}
				if winnerConn.isClosed() {
					t.Error("stale lifecycle call closed the winning connection")
				}
				if got := s.State(); got != StateConnected {
					t.Errorf("state = %v, want %v", got, StateConnected)
				}
				if got := staleOp.openCount(); got != 0 {
					t.Errorf("stale opener called %d times, want 0", got)
				}
			})
		})
	}
}

// TestOperationInProgress checks that an operation refused because an
// exclusive operation holds the port says so, rather than claiming the
// session is not connected.
func TestOperationInProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fs := &fakeSink{}
		s := testSession(t, fs)
		op := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
		if err := s.Connect(op, gpsreg.VendorUnknown); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		waitForState(t, s, StateConnected)
		done := make(chan struct{})
		go func() {
			s.ReadConfig(context.Background())
			close(done)
		}()
		waitForState(t, s, StateConfiguring)
		if err := s.SendMsgFile("x", "", false); err == nil || err.Error() != "another operation is in progress" {
			t.Errorf("SendMsgFile during ReadConfig: err = %v, want operation-in-progress", err)
		}
		<-done
	})
}

// TestCancelledOpSkipsEndState checks that an operation cancelled by a
// replacement Connect does not complete into the new attempt: its
// setEndState must leave the Connecting state and the event stream
// alone, so the new connection proceeds to Connected.
func TestCancelledOpSkipsEndState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fs := &fakeSink{}
		s := testSession(t, fs)
		op := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
		if err := s.Connect(op, gpsreg.VendorUnknown); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		waitForState(t, s, StateConnected)
		cfgDone := make(chan error, 1)
		go func() {
			_, err := s.ReadConfig(context.Background())
			cfgDone <- err
		}()
		waitForState(t, s, StateConfiguring)
		op2 := &blockingOpener{fakeOpener: fakeOpener{conns: []*fakeConn{newFakeConn()}}, gate: make(chan struct{})}
		errCh := make(chan error, 1)
		go func() { errCh <- s.Connect(op2, gpsreg.VendorUnknown) }()
		synctest.Wait()
		if err := <-cfgDone; err == nil {
			t.Fatal("cancelled ReadConfig returned nil error")
		}
		if got := s.State(); got != StateConnecting {
			t.Fatalf("state during pending open = %v, want %v", got, StateConnecting)
		}
		close(op2.gate)
		if err := <-errCh; err != nil {
			t.Fatalf("second Connect: %v", err)
		}
		waitForState(t, s, StateConnected)
		expect := []ConnState{StateConnecting, StateConnected, StateConfiguring,
			StateConnecting, StateConnected}
		if got := fs.states(); !reflect.DeepEqual(got, expect) {
			t.Errorf("state events = %v, want %v", got, expect)
		}
	})
}

// TestLifecycleEventOrder checks that racing lifecycle transitions
// cannot leave a stale final state event: an emission delayed past a
// newer transition is followed by the newer state.
func TestLifecycleEventOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gs := &gatedSink{gate: make(chan struct{})}
		s := New(slog.New(slog.DiscardHandler), gs, Options{})
		t.Cleanup(s.Disconnect)
		op := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
		if err := s.Connect(op, gpsreg.VendorUnknown); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		waitForState(t, s, StateConnected)
		gs.gating.Store(true)
		done := make(chan struct{})
		go func() {
			s.Disconnect()
			close(done)
		}()
		synctest.Wait() // Disconnect is now blocked emitting Disconnected
		op2 := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
		errCh := make(chan error, 1)
		go func() { errCh <- s.Connect(op2, gpsreg.VendorUnknown) }()
		gs.gating.Store(false)
		close(gs.gate)
		<-done
		if err := <-errCh; err != nil {
			t.Fatalf("Connect during delayed emit: %v", err)
		}
		waitForState(t, s, StateConnected)
		expect := []ConnState{StateConnecting, StateConnected, StateDisconnected,
			StateConnecting, StateConnected}
		if got := gs.states(); !reflect.DeepEqual(got, expect) {
			t.Errorf("state events = %v, want %v", got, expect)
		}
	})
}

func TestStateEventReentrantDisconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rs := &reentrantSink{}
		s := New(slog.New(slog.DiscardHandler), rs, Options{})
		rs.s = s
		op := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
		if err := s.Connect(op, gpsreg.VendorUnknown); err == nil {
			t.Fatal("Connect returned nil, want superseded error")
		}
		if got := s.State(); got != StateDisconnected {
			t.Errorf("state = %v, want %v", got, StateDisconnected)
		}
		expect := []ConnState{StateConnecting, StateDisconnected}
		if got := rs.states(); !reflect.DeepEqual(got, expect) {
			t.Errorf("state events = %v, want %v", got, expect)
		}
	})
}

func TestEndStateScopedToRun(t *testing.T) {
	s := New(slog.New(slog.DiscardHandler), &fakeSink{}, Options{})
	oldRun, oldCancel := context.WithCancel(context.Background())
	defer oldCancel()
	newRun, newCancel := context.WithCancel(context.Background())
	defer newCancel()
	s.runCtx = oldRun
	s.state = StateConfiguring
	s.runCtx = newRun
	if got := s.setEndState(oldRun, StateConnected); got != StateConfiguring {
		t.Errorf("setEndState returned %v, want %v", got, StateConfiguring)
	}
	if got := s.State(); got != StateConfiguring {
		t.Errorf("state = %v, want %v", got, StateConfiguring)
	}
}

func TestUnplugDisconnects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fs := &fakeSink{}
		s := testSession(t, fs)
		conn := newFakeConn()
		op := &fakeOpener{conns: []*fakeConn{conn}}
		if err := s.Connect(op, gpsreg.VendorUnknown); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		waitForState(t, s, StateConnected)
		conn.kill()
		waitForState(t, s, StateDisconnected)
		expect := []ConnState{StateConnecting, StateConnected, StateDisconnected}
		if got := fs.states(); !reflect.DeepEqual(got, expect) {
			t.Errorf("state events = %v, want %v", got, expect)
		}
		if got := op.openCount(); got != 1 {
			t.Errorf("open count = %d, want 1 (no reconnect without a reset)", got)
		}
	})
}

func TestResetReconnects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fs := &fakeSink{}
		s := testSession(t, fs)
		conn1, conn2 := newFakeConn(), newFakeConn()
		op := &fakeOpener{conns: []*fakeConn{conn1, conn2}}
		if err := s.Connect(op, gpsreg.VendorUnknown); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		waitForState(t, s, StateConnected)
		target := gpsprot.NewConfigTarget()
		target.Opts.Reset = gpsprot.ResetCold
		if err := s.ApplyConfig(context.Background(), target); !errors.Is(err, gpscfg.ErrNotDetected) {
			t.Fatalf("ApplyConfig: %v, want ErrNotDetected", err)
		}
		conn1.kill()
		waitForState(t, s, StateReconnecting)
		waitForState(t, s, StateConnected)
		if got := op.openCount(); got != 2 {
			t.Errorf("open count = %d, want 2", got)
		}
		if conn2.writeCount() == 0 {
			t.Errorf("no probe writes on the reopened connection")
		}
		expect := []ConnState{StateConnecting, StateConnected, StateConfiguring,
			StateConnected, StateReconnecting, StateConnected}
		if got := fs.states(); !reflect.DeepEqual(got, expect) {
			t.Errorf("state events = %v, want %v", got, expect)
		}
		s.Disconnect()
	})
}

func TestResetGatedOverProxy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fs := &fakeSink{}
		s := testSession(t, fs)
		op := &fakeOpener{conns: []*fakeConn{newFakeConn()}, socket: true}
		if err := s.Connect(op, gpsreg.VendorUnknown); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		waitForState(t, s, StateConnected)
		target := gpsprot.NewConfigTarget()
		target.Opts.Reset = gpsprot.ResetCold
		if err := s.ApplyConfig(context.Background(), target); err == nil {
			t.Errorf("ApplyConfig with reset over proxy: expected error")
		}
		if got := s.State(); got != StateConnected {
			t.Errorf("state = %v, want %v", got, StateConnected)
		}
		s.Disconnect()
	})
}

func TestRepeatedConfigRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fs := &fakeSink{}
		s := testSession(t, fs)
		op := &fakeOpener{conns: []*fakeConn{newFakeConn()}}
		if err := s.Connect(op, gpsreg.VendorUnknown); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		waitForState(t, s, StateConnected)
		// The packetWorker must keep serving config requests after each run.
		for i := range 2 {
			if _, err := s.ReadConfig(context.Background()); !errors.Is(err, gpscfg.ErrNotDetected) {
				t.Fatalf("ReadConfig %d: %v, want ErrNotDetected", i, err)
			}
			if got := s.State(); got != StateConnected {
				t.Fatalf("state after ReadConfig %d = %v", i, got)
			}
		}
		s.Disconnect()
	})
}

func TestPacketEventGating(t *testing.T) {
	tests := []struct {
		name       string
		wantPacket bool
	}{
		{name: "gated", wantPacket: false},
		{name: "wanted", wantPacket: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fs := &fakeSink{wantPacket: tc.wantPacket}
				s := testSession(t, fs)
				conn := newFakeConn()
				op := &fakeOpener{conns: []*fakeConn{conn}}
				if err := s.Connect(op, gpsreg.VendorUnknown); err != nil {
					t.Fatalf("Connect: %v", err)
				}
				waitForState(t, s, StateConnected)
				for range 3 {
					conn.send(makeGGA())
					synctest.Wait()
					time.Sleep(100 * time.Millisecond)
				}
				synctest.Wait()
				if got := fs.count(EventPacket) > 0; got != tc.wantPacket {
					t.Errorf("packet events emitted = %v, want %v", got, tc.wantPacket)
				}
				if fs.count(EventMsg) == 0 {
					t.Errorf("no gps:msg events from the GGA packets")
				}
				s.Disconnect()
			})
		})
	}
}

func TestPacketLogWritesSerialized(t *testing.T) {
	w := &blockingWriter{gate: make(chan struct{}), entered: make(chan struct{}, 2)}
	s := New(slog.New(slog.DiscardHandler), &fakeSink{}, Options{PacketLog: w})
	done := make(chan struct{}, 2)
	go func() {
		s.writePacketLogEntry(gpsio.PacketLogEntry{Ascii: "one"})
		done <- struct{}{}
	}()
	<-w.entered
	started := make(chan struct{})
	go func() {
		close(started)
		s.writePacketLogEntry(gpsio.PacketLogEntry{Ascii: "two"})
		done <- struct{}{}
	}()
	<-started
	select {
	case <-w.entered:
		t.Error("second packet-log write entered before the first completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(w.gate)
	<-done
	<-done
	if got := w.calls.Load(); got != 2 {
		t.Errorf("writes = %d, want 2", got)
	}
}

func TestCorrectionsFailurePersistsAndRestartClearsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	conn := newFakeConn()
	s := New(slog.New(slog.DiscardHandler), &fakeSink{}, Options{})
	s.state = StateConnected
	s.runCtx = ctx
	s.conn = conn
	s.portLock = gpsio.NewOutPortLock(conn)
	defer func() {
		s.StopCorrections()
		cancel()
		conn.Close()
		s.connWg.Wait()
	}()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			_, err = io.WriteString(c, "HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n")
			c.Close()
		}
		done <- err
	}()
	cfg := CorrectionSource{Mode: "ntrip", Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Mountpoint: "MNT"}
	if err := s.StartCorrections(cfg); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	wg := s.corrWg
	s.mu.Unlock()
	wg.Wait()
	ln.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ev := s.CorrectionsState()
	if ev.State != "failed" || ev.Error != "Ntrip: HTTP/1.1 401 Unauthorized" {
		t.Fatalf("corrections state = %+v, want failed 401", ev)
	}

	ln, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan struct{})
	release := make(chan struct{})
	done = make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			close(accepted)
			<-release
			_, err = io.WriteString(c, "HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n")
			c.Close()
		}
		done <- err
	}()
	cfg.Port = ln.Addr().(*net.TCPAddr).Port
	if err := s.StartCorrections(cfg); err != nil {
		close(release)
		t.Fatal(err)
	}
	<-accepted
	if ev := s.CorrectionsState(); ev.State != "connecting" || ev.Error != "" {
		close(release)
		t.Fatalf("corrections state after restart = %+v, want connecting without error", ev)
	}
	close(release)
	s.mu.Lock()
	wg = s.corrWg
	s.mu.Unlock()
	wg.Wait()
	ln.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestNotConnected(t *testing.T) {
	s := New(slog.New(slog.DiscardHandler), &fakeSink{}, Options{})
	tests := []struct {
		name string
		call func() error
	}{
		{name: "ReadConfig", call: func() error { _, err := s.ReadConfig(context.Background()); return err }},
		{name: "ApplyConfig", call: func() error { return s.ApplyConfig(context.Background(), gpsprot.NewConfigTarget()) }},
		{name: "SendMsgFile", call: func() error { return s.SendMsgFile("tag", "", false) }},
		{name: "StartCorrections", call: func() error {
			return s.StartCorrections(CorrectionSource{Mode: "tcp", Host: "h", Port: 1})
		}},
		{name: "CancelMsgSend", call: func() error { return s.CancelMsgSend() }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Errorf("expected error when disconnected")
			}
		})
	}
}

func TestApplyConfigClearsReadOnlyProps(t *testing.T) {
	s := New(slog.New(slog.DiscardHandler), &fakeSink{}, Options{})
	target := gpsprot.NewConfigTarget()
	target.Props.SetPort("UART1")
	if err := s.ApplyConfig(context.Background(), target); err == nil {
		t.Fatal("ApplyConfig succeeded while disconnected")
	}
	if got := target.Props.ReadOnlyProps(); got != 0 {
		t.Errorf("read-only properties = %v, want none", got)
	}
}
