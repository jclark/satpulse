package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sync"
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
