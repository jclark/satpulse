package stream

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/scan"
)

// fakeDest is a scriptable Destination for tests.  Each Connect call
// invokes next, which may return a (client) net.Conn or an error.
// The server-side of each successful connect is recorded in servers
// for the test to read or close.
type fakeDest struct {
	mu      sync.Mutex
	servers []net.Conn
	next    func(ctx context.Context) (net.Conn, error)
}

func (d *fakeDest) Connect(ctx context.Context) (net.Conn, error) {
	return d.next(ctx)
}

// newPipeDest returns a fakeDest whose Connect always returns a fresh
// net.Pipe() pair.  The client side is returned to the caller (the
// writer); the server side is appended to servers.
func newPipeDest() *fakeDest {
	d := &fakeDest{}
	d.next = func(ctx context.Context) (net.Conn, error) {
		client, server := net.Pipe()
		d.mu.Lock()
		d.servers = append(d.servers, server)
		d.mu.Unlock()
		return client, nil
	}
	return d
}

// serverConn returns the i-th server-side conn, polling until it
// exists.  Waits up to 5s, generous enough for a worst-case backoff
// retry between connections.
func (d *fakeDest) serverConn(t *testing.T, i int) net.Conn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		n := len(d.servers)
		d.mu.Unlock()
		if n > i {
			d.mu.Lock()
			defer d.mu.Unlock()
			return d.servers[i]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for serverConn[%d]", i)
	return nil
}

// runPush starts a bcast and runs Push in a goroutine.  Returns
// pktCh (write packets here), a closeInput function (idempotent),
// and a function that waits for Run to exit and returns its error.
// wait calls closeInput before tearing down the bcast.
func runPush(t *testing.T, ctx context.Context, dest Destination,
	pktTag gpsprot.Tag,
	onState func(State, error)) (chan<- scan.Packet, func(), func() error) {
	t.Helper()
	pktCh := make(chan scan.Packet)
	b := bcast.New((<-chan scan.Packet)(pktCh))
	bcastDone := make(chan struct{})
	go func() {
		b.Run(ctx, testLogger())
		close(bcastDone)
	}()
	pushDone := make(chan error, 1)
	push := NewPush()
	go func() {
		pushDone <- push.Run(ctx, testLogger(), b, dest, pktTag, false, onState)
	}()
	var closeOnce sync.Once
	closeInput := func() { closeOnce.Do(func() { close(pktCh) }) }
	wait := func() error {
		err := <-pushDone
		closeInput()
		b.Close()
		<-bcastDone
		return err
	}
	return pktCh, closeInput, wait
}

// readAllAvailable reads up to n bytes from r, blocking up to timeout.
// Returns whatever was read (possibly less than n).
func readAllAvailable(r net.Conn, n int, timeout time.Duration) []byte {
	r.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, n)
	got := 0
	for got < n {
		k, err := r.Read(buf[got:])
		got += k
		if err != nil {
			break
		}
	}
	return buf[:got]
}

func TestPushPacketsFlowToDestination(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest := newPipeDest()
	pkt1005 := makeRTCM(1005, 16)
	pktCh, _, wait := runPush(t, ctx, dest, rtcm.Tag, nil)
	server := dest.serverConn(t, 0)
	// Send a packet via the bcast.
	pktCh <- scan.Packet{Format: rtcm.PacketFormat, Data: string(pkt1005), ChecksumValid: true}
	got := readAllAvailable(server, len(pkt1005), 2*time.Second)
	if string(got) != string(pkt1005) {
		t.Errorf("got %x, want %x", got, pkt1005)
	}
	cancel()
	if err := wait(); err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("Run: %v", err)
	}
}

func TestPushFiltersByFormat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest := newPipeDest()
	pktCh, _, wait := runPush(t, ctx, dest, rtcm.Tag, nil)
	server := dest.serverConn(t, 0)
	// Send a non-RTCM packet (Format = nil simulates a skipped
	// packet); should be dropped by the queue.
	pktCh <- scan.Packet{Format: nil, Data: "ignored"}
	// Now send a real RTCM packet.
	pkt := makeRTCM(1077, 16)
	pktCh <- scan.Packet{Format: rtcm.PacketFormat, Data: string(pkt), ChecksumValid: true}
	got := readAllAvailable(server, len(pkt), 2*time.Second)
	if string(got) != string(pkt) {
		t.Errorf("filtered packet leaked through; got %x, want %x", got, pkt)
	}
	cancel()
	if err := wait(); err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("Run: %v", err)
	}
}

func TestPushFiltersInvalidChecksum(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest := newPipeDest()
	pktCh, _, wait := runPush(t, ctx, dest, rtcm.Tag, nil)
	server := dest.serverConn(t, 0)
	// Checksum-invalid RTCM packet must not be forwarded.
	bad := makeRTCM(1005, 16)
	pktCh <- scan.Packet{Format: rtcm.PacketFormat, Data: string(bad)} // ChecksumValid zero value = false
	// Now send a valid packet to verify the pipeline is still alive
	// and the bad packet didn't slip through ahead of it.
	good := makeRTCM(1077, 16)
	pktCh <- scan.Packet{Format: rtcm.PacketFormat, Data: string(good), ChecksumValid: true}
	got := readAllAvailable(server, len(good), 2*time.Second)
	if string(got) != string(good) {
		t.Errorf("invalid-checksum packet leaked or pipeline stalled; got %x, want %x", got, good)
	}
	cancel()
	if err := wait(); err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("Run: %v", err)
	}
}

func TestPushBcastCloseShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Destination that never succeeds: Connect keeps failing.
	// This exercises the bcast-close path while the writer is in
	// the backoff sleep / Connect loop, not in writeLoop.
	dest := &fakeDest{}
	dest.next = func(ctx context.Context) (net.Conn, error) {
		return nil, errors.New("refused")
	}
	_, closeInput, wait := runPush(t, ctx, dest, rtcm.Tag, nil)
	// Give the writer a moment to enter its retry loop, then close
	// the input channel.  The queue should call iCancel and the
	// writer should wake.
	time.Sleep(50 * time.Millisecond)
	closeInput()
	done := make(chan error, 1)
	go func() { done <- wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Push.Run did not return after bcast close")
	}
}

func TestPushParentCtxCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dest := newPipeDest()
	_, _, wait := runPush(t, ctx, dest, rtcm.Tag, nil)
	dest.serverConn(t, 0)
	cancel()
	done := make(chan error, 1)
	go func() { done <- wait() }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Push.Run did not return after ctx cancel")
	}
}

func TestPushReconnectsAfterWriteError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest := newPipeDest()
	pktCh, _, wait := runPush(t, ctx, dest, rtcm.Tag, nil)
	server0 := dest.serverConn(t, 0)
	pkt1005 := makeRTCM(1005, 16)
	pkt1077 := makeRTCM(1077, 16)
	// First packet must traverse the pipeline to confirm the writer
	// is in the streaming phase before we tear down server0.
	pktCh <- scan.Packet{Format: rtcm.PacketFormat, Data: string(pkt1005), ChecksumValid: true}
	got := readAllAvailable(server0, len(pkt1005), 2*time.Second)
	if string(got) != string(pkt1005) {
		t.Fatalf("first packet: got %x, want %x", got, pkt1005)
	}
	// Close server side; the writer's next Write will fail.  Send a
	// packet to force the write attempt -- this packet is expected
	// to be lost (the plan documents that the in-flight packet may
	// be dropped during a reconnect).
	server0.Close()
	pktCh <- scan.Packet{Format: rtcm.PacketFormat, Data: string(pkt1005), ChecksumValid: true}
	// Wait for the writer to reconnect.
	server1 := dest.serverConn(t, 1)
	// Send a fresh packet; it should arrive on the new connection.
	pktCh <- scan.Packet{Format: rtcm.PacketFormat, Data: string(pkt1077), ChecksumValid: true}
	got = readAllAvailable(server1, len(pkt1077), 5*time.Second)
	if string(got) != string(pkt1077) {
		t.Errorf("after reconnect: got %x, want %x", got, pkt1077)
	}
	cancel()
	if err := wait(); err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("Run: %v", err)
	}
}

func TestPushStopsOnFatalConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Connect always fails with a fatal error; Push must give up
	// rather than retry forever.
	var calls atomic.Int32
	dest := &fakeDest{}
	dest.next = func(ctx context.Context) (net.Conn, error) {
		calls.Add(1)
		return nil, &fatalConnectError{errors.New("Ntrip: ERROR - Bad Password")}
	}
	var gotFailed atomic.Bool
	onState := func(st State, _ error) {
		if st == Failed {
			gotFailed.Store(true)
		}
	}
	_, _, wait := runPush(t, ctx, dest, rtcm.Tag, onState)
	done := make(chan error, 1)
	go func() { done <- wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Push.Run did not return after fatal connect error")
	}
	if !gotFailed.Load() {
		t.Error("onState was not called with Failed")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("Connect called %d times, want 1 (no retry on fatal error)", n)
	}
}

func TestNtripFatalResponse(t *testing.T) {
	cases := []struct {
		resp  string
		fatal bool
	}{
		{"ERROR - Bad Password", true},
		{"ERROR - Mount Point Invalid", true},
		{"ERROR - Mount Point Taken", false},
		{"ERROR - Already Connected", false},
		{"ERROR - Mount Point Taken or Invalid", false},
		{"some unexpected line", false},
	}
	for _, c := range cases {
		if got := ntripFatalResponse(c.resp); got != c.fatal {
			t.Errorf("ntripFatalResponse(%q) = %v, want %v", c.resp, got, c.fatal)
		}
	}
}

func TestNtripDestinationBadPasswordFatal(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ERROR - Bad Password\r\n"))
	})
	defer ln.close()
	d := &NtripDestination{Addr: ln.addr(), Mountpoint: "MNT", Password: "wrong"}
	_, err := d.Connect(context.Background())
	if !isFatalConnect(err) {
		t.Errorf("bad password: isFatalConnect = false, want true (err %v)", err)
	}
}

func TestNtripDestinationMountTakenNotFatal(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ERROR - Mount Point Taken or Invalid\r\n"))
	})
	defer ln.close()
	d := &NtripDestination{Addr: ln.addr(), Mountpoint: "MNT", Password: "pw"}
	_, err := d.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if isFatalConnect(err) {
		t.Errorf("mount taken: isFatalConnect = true, want false (err %v)", err)
	}
}

// Ensure compile-time check that NtripDestination satisfies Destination.
var _ Destination = (*NtripDestination)(nil)

func TestNtripDestinationConnectSuccess(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ICY 200 OK\r\n"))
	})
	defer ln.close()
	d := &NtripDestination{
		Addr:       ln.addr(),
		Mountpoint: "MNT",
		Password:   "pw",
	}
	conn, err := d.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	conn.Close()
}

func TestNtripDestinationRequestFields(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ICY 200 OK\r\n"))
	})
	defer ln.close()
	d := &NtripDestination{
		Addr:       ln.addr(),
		Mountpoint: "MNT",
		Password:   "secret",
		STR:        "Bangkok;RTCM 3.3;1005(1),1077(1);2;GPS;Misc;XXX;13.75;100.50;0;0;satpulse;none;N;N;4800;",
		UserAgent:  NtripUserAgent{Version: "1.2.3"},
	}
	conn, err := d.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	conn.Close()
	req := waitForRequest(t, ln)
	if !strings.HasPrefix(req, "SOURCE secret /MNT\r\n") {
		t.Errorf("bad request line: %q", req)
	}
	if !strings.Contains(req, "\r\nSource-Agent: NTRIP satpulse/1.2.3\r\n") {
		t.Errorf("missing Source-Agent: %q", req)
	}
	if !strings.Contains(req, "\r\nSTR: Bangkok;") {
		t.Errorf("missing STR header: %q", req)
	}
	if strings.Contains(req, "Ntrip-Version") {
		t.Errorf("unexpected Ntrip-Version header: %q", req)
	}
	if !strings.HasSuffix(req, "\r\n\r\n") {
		t.Errorf("request not terminated by blank line: %q", req)
	}
}

func TestNtripDestinationNoSTR(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ICY 200 OK\r\n"))
	})
	defer ln.close()
	d := &NtripDestination{
		Addr:       ln.addr(),
		Mountpoint: "MNT",
		Password:   "pw",
	}
	conn, err := d.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	conn.Close()
	req := waitForRequest(t, ln)
	if strings.Contains(req, "STR:") {
		t.Errorf("unexpected STR header: %q", req)
	}
}

func TestNtripDestinationSourceAgentNoVersion(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ICY 200 OK\r\n"))
	})
	defer ln.close()
	d := &NtripDestination{
		Addr:       ln.addr(),
		Mountpoint: "MNT",
		Password:   "pw",
	}
	conn, err := d.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	conn.Close()
	req := waitForRequest(t, ln)
	if !strings.Contains(req, "\r\nSource-Agent: NTRIP satpulse\r\n") {
		t.Errorf("unexpected Source-Agent: %q", req)
	}
}

func TestNtripDestinationBadPassword(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ERROR - Bad Password\r\n"))
	})
	defer ln.close()
	d := &NtripDestination{Addr: ln.addr(), Mountpoint: "MNT", Password: "wrong"}
	if _, err := d.Connect(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	} else if !strings.Contains(err.Error(), "ERROR - Bad Password") {
		t.Errorf("error %q does not mention server response", err)
	}
}

func TestNtripDestinationMountTaken(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("ERROR - Mount Point Taken or Invalid\r\n"))
	})
	defer ln.close()
	d := &NtripDestination{Addr: ln.addr(), Mountpoint: "MNT", Password: "pw"}
	if _, err := d.Connect(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNtripDestinationUnexpectedResponse(t *testing.T) {
	ln := newNtripListener(t, func(conn net.Conn, _ []byte) {
		conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n"))
	})
	defer ln.close()
	d := &NtripDestination{Addr: ln.addr(), Mountpoint: "MNT", Password: "pw"}
	if _, err := d.Connect(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// waitForRequest polls the listener for the captured request bytes,
// returning the request once it is non-empty.  Fails the test on
// timeout.
func waitForRequest(t *testing.T, ln *ntripListener) string {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if req := ln.request(); req != "" {
			return req
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for request")
	return ""
}
