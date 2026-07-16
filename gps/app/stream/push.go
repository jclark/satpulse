package stream

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/scan"
)

// writeTimeout bounds how long a single conn.Write may block before
// the writer treats the destination as dead and reconnects.  A dead
// remote can leave the TCP connection "established" forever without
// RST/FIN; once the kernel send buffer fills, Write blocks until TCP
// keepalives expire (default 2 hours on Linux).  30 s is generous
// for transient hiccups, short enough that recovery is timely.
const writeTimeout = 30 * time.Second

const udpWriteTimeout = 200 * time.Millisecond

// Destination is the push counterpart of Source: it returns a
// writable net.Conn after a successful handshake.  Unlike Source, it
// returns net.Conn (not io.ReadCloser) because Push only writes after
// connect and never needs to recover buffered prefix bytes.
type Destination interface {
	// Connect establishes a connection to the destination and
	// returns a net.Conn for writing payload bytes.  Connect must
	// respect ctx cancellation.
	Connect(ctx context.Context) (net.Conn, error)
}

// NtripDestination performs the Ntrip v1 SOURCE handshake and
// returns the raw socket on success.  It carries only the fields
// that get written onto the wire during the handshake, plus the
// address to dial.  Built once at startup by the daemon; stateless
// thereafter -- Connect does not mutate it.
type NtripDestination struct {
	Addr       string // "host:port"
	Mountpoint string
	Password   string
	// STR is the source-table entry fields 3..n (without the leading
	// "STR;<mountpoint>;" prefix).  Empty means omit the STR: header.
	STR       string
	UserAgent NtripUserAgent
}

// Connect dials the caster, performs the v1 SOURCE handshake, and
// returns the raw net.Conn on success.  The caller writes the
// payload directly to the returned conn.
func (d *NtripDestination) Connect(ctx context.Context) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", d.Addr)
	if err != nil {
		return nil, err
	}
	// Close conn on ctx cancellation during the handshake.  The
	// dial itself is covered by DialContext.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	err = d.handshake(conn)
	close(done)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// fatalConnectError wraps an endpoint Connect error that no amount
// of retrying can fix (e.g. a rejected NTRIP password or an invalid
// mountpoint).  The stream stops rather than reconnecting when it
// sees one.  Errors not wrapped this way are treated as transient.
type fatalConnectError struct{ err error }

func (e *fatalConnectError) Error() string { return e.err.Error() }
func (e *fatalConnectError) Unwrap() error { return e.err }

// isFatalConnect reports whether err (or anything it wraps) is fatal.
func isFatalConnect(err error) bool {
	var fe *fatalConnectError
	return errors.As(err, &fe)
}

// handshake writes the v1 SOURCE request and reads the caster
// response.  Returns nil on "ICY 200 OK".  A non-OK response yields
// an error wrapped in *fatalConnectError when the caster's rejection
// is permanent (bad password, invalid mountpoint); transient
// rejections such as "Mount Point Taken" return a plain error so the
// writer keeps retrying.
func (d *NtripDestination) handshake(conn net.Conn) error {
	if _, err := conn.Write([]byte(d.request())); err != nil {
		return err
	}
	br := bufio.NewReaderSize(conn, 4096)
	line, err := br.ReadSlice('\n')
	if err != nil {
		return err
	}
	if bytes.Equal(line, []byte("ICY 200 OK\r\n")) {
		return nil
	}
	resp := strings.TrimSuffix(string(line), "\r\n")
	err = fmt.Errorf("Ntrip: %s", resp)
	if ntripFatalResponse(resp) {
		return &fatalConnectError{err}
	}
	return err
}

// ntripFatalResponse reports whether an NTRIP v1 SOURCE error line
// describes a permanent rejection.  "Mount Point Taken" and "Already
// Connected" are transient (another server holds the mountpoint and
// will eventually disconnect), and the ambiguous "Mount Point Taken
// or Invalid" from old casters is treated as transient since it may
// be Taken.  A bad password or a definitively invalid mountpoint is
// fatal -- the configuration must change before a retry can succeed.
func ntripFatalResponse(resp string) bool {
	switch {
	case strings.Contains(resp, "Mount Point Taken or Invalid"):
		return false
	case strings.Contains(resp, "Bad Password"):
		return true
	case strings.Contains(resp, "Mount Point Invalid"):
		return true
	}
	return false
}

// request builds the v1 SOURCE request bytes.
func (d *NtripDestination) request() string {
	var b strings.Builder
	fmt.Fprintf(&b, "SOURCE %s /%s\r\n", d.Password, d.Mountpoint)
	fmt.Fprintf(&b, "Source-Agent: %s\r\n", d.sourceAgent())
	if d.STR != "" {
		fmt.Fprintf(&b, "STR: %s\r\n", d.STR)
	}
	b.WriteString("\r\n")
	return b.String()
}

func (d *NtripDestination) sourceAgent() string {
	if d.UserAgent.Version == "" {
		return "NTRIP satpulse"
	}
	return "NTRIP satpulse/" + d.UserAgent.Version
}

// UDPSink is a UDP push destination: the address datagrams are sent
// to.  Built once at startup; Run dials it and streams to it.
type UDPSink struct {
	Addr   string // "host:port"
	pktTag gpsprot.Tag
	lg     *slog.Logger
}

// NewUDPSink creates a UDP push destination.
func NewUDPSink(addr string, lg *slog.Logger, pktTag gpsprot.Tag) *UDPSink {
	if lg == nil {
		lg = slog.Default()
	}
	return &UDPSink{Addr: addr, pktTag: pktTag, lg: lg}
}

// Run dials the destination and forwards selected packets to it as
// UDP datagrams until ctx is cancelled or packets closes.  UDP is
// connectionless: there is no handshake or reconnect, so dialing is
// all the setup there is.  When pktTag is empty every non-empty
// packet is sent; otherwise only packets carrying pktTag with a valid
// checksum.  A write failure drops the datagram and is logged rather
// than returned -- UDP is fire-and-forget.
func (s *UDPSink) Run(ctx context.Context, packets *bcast.Bcast[scan.Packet]) error {
	if s.lg == nil {
		s.lg = slog.Default()
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", s.Addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	qCtx, qCancel := context.WithCancel(ctx)
	defer qCancel()
	qCh := make(chan scan.Packet, 1)
	qReady := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		s.queue(qCtx, qCancel, packets, qCh, qReady)
	})
	<-qReady
	s.lg.Info("udp push connected", "addr", conn.RemoteAddr(), "protocol", s.pktTag)
	wg.Go(func() {
		s.writer(qCtx, conn, qCh)
	})
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// queue subscribes to packets, filters for this UDP
// destination, applies the shared queue policy, and emits packets to
// writerCh.  When the subscription closes it cancels ctx so the
// writer wakes even if it is blocked in a write.
func (s *UDPSink) queue(ctx context.Context, cancel context.CancelFunc,
	packets *bcast.Bcast[scan.Packet], writerCh chan<- scan.Packet, ready chan<- struct{}) {
	defer close(writerCh)
	subCh := packets.Subscribe()
	defer packets.Unsubscribe(subCh)
	if ready != nil {
		close(ready)
	}
	var q pruningQueue
	// If sendCh is non-nil, it is writerCh and next is ready to send.
	var sendCh chan<- scan.Packet
	var next scan.Packet
	for {
		select {
		case <-ctx.Done():
			return
		case pkt, ok := <-subCh:
			if !ok {
				cancel()
				return
			}
			if pkt.Data == "" {
				continue
			}
			if !s.pktTag.IsZero() && !(pkt.HasTag(s.pktTag) && pkt.ChecksumValid) {
				continue
			}
			if dropped, ok := q.enqueue(pkt); ok {
				msgID := ""
				if dropped.Format != nil {
					msgID = dropped.Format.MsgID([]byte(dropped.Data))
				}
				s.lg.Warn("udp push queue full, dropped oldest packet",
					"addr", s.Addr, "protocol", s.pktTag,
					"tag", dropped.Tag(), "msgid", msgID, "len", len(dropped.Data))
			}
			if sendCh == nil {
				next = q.dequeue()
				sendCh = writerCh
			}
		case sendCh <- next:
			if q.len() > 0 {
				next = q.dequeue()
			} else {
				sendCh = nil
			}
		}
	}
}

// writer receives packets from the queue and writes them as UDP
// datagrams.  Write errors are logged and otherwise ignored.
func (s *UDPSink) writer(ctx context.Context, conn net.Conn, qCh <-chan scan.Packet) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	for {
		var pkt scan.Packet
		var ok bool
		select {
		case <-ctx.Done():
			return
		case pkt, ok = <-qCh:
			if !ok {
				return
			}
		}
		if err := conn.SetWriteDeadline(time.Now().Add(udpWriteTimeout)); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.lg.Warn("udp push set deadline failed", "addr", conn.RemoteAddr(), "err", err)
			continue
		}
		n, err := conn.Write([]byte(pkt.Data))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.lg.Warn("udp push write failed", "addr", conn.RemoteAddr(),
				"len", len(pkt.Data), "err", err)
			continue
		}
		if n != len(pkt.Data) {
			s.lg.Warn("udp push short write", "addr", conn.RemoteAddr(),
				"wrote", n, "len", len(pkt.Data))
		}
	}
}

// Push consumes scanned packets from the receiver's packet bcast and
// writes them to a remote Destination via a pruning queue and a
// reconnecting writer.  Unlike Pull, Push does not own a packet
// bcast.
type Push struct {
	dest    Destination
	pktTag  gpsprot.Tag
	msm7to4 bool
	lg      *slog.Logger
}

// NewPush creates a Push.
func NewPush(dest Destination, lg *slog.Logger, pktTag gpsprot.Tag, msm7to4 bool) *Push {
	if lg == nil {
		lg = slog.Default()
	}
	return &Push{dest: dest, lg: lg, pktTag: pktTag, msm7to4: msm7to4}
}

// Run starts the packet queue and adaptive writer.  Run blocks until
// ctx is cancelled or the input bcast is closed.  Returns ctx.Err()
// on parent ctx cancellation, nil on clean bcast-close shutdown.
func (s *Push) Run(ctx context.Context, packets *bcast.Bcast[scan.Packet], onState func(State, error)) error {
	if s.lg == nil {
		s.lg = slog.Default()
	}
	qCtx, qCancel := context.WithCancel(ctx)
	defer qCancel()
	qCh := make(chan scan.Packet, 1)
	reconnectCh := make(chan struct{}, 1)
	qReady := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		s.queue(qCtx, qCancel, packets, reconnectCh, qCh, qReady)
	})
	<-qReady
	wg.Go(func() {
		s.writer(qCtx, qCancel, qCh, reconnectCh, onState)
	})
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// queue subscribes to packets, filters by the configured packet tag,
// applies the pruning policy, and emits packets to writerCh.  On
// reconnectCh it advances the MSM epoch so dedup never spans a
// broken connection.  When the subscription closes it calls
// cancel to wake the writer regardless of where the writer is
// blocked.
func (s *Push) queue(ctx context.Context, cancel context.CancelFunc,
	packets *bcast.Bcast[scan.Packet], reconnectCh <-chan struct{}, writerCh chan scan.Packet, ready chan<- struct{}) {
	defer close(writerCh)
	subCh := packets.Subscribe()
	defer packets.Unsubscribe(subCh)
	if ready != nil {
		close(ready)
	}
	var q pruningQueue
	// If sendCh is non-nil, it is writerCh and next is ready to send.
	var sendCh chan<- scan.Packet
	var next scan.Packet
	for {
		select {
		case <-ctx.Done():
			return
		case <-reconnectCh:
			// Best-effort drop of anything that was about to
			// land on the broken connection: drain writerCh
			// (we are its only sender, so a non-blocking
			// receive is safe), reset next, and bump the MSM
			// epoch.  Packets enqueued by subCh between the
			// writer's failure and our picking up reconnectCh
			// remain in q under the old epoch and may still be
			// sent after reconnect; they self-heal as fresh
			// same-msgID updates arrive.
			select {
			case <-writerCh:
			default:
			}
			sendCh = nil
			q.reconnect()
			if q.len() > 0 {
				next = q.dequeue()
				sendCh = writerCh
			}
		case pkt, ok := <-subCh:
			if !ok {
				cancel()
				return
			}
			if pkt.Tag() != s.pktTag || !pkt.ChecksumValid {
				break
			}
			if dropped, ok := q.enqueue(pkt); ok {
				msgID := ""
				if dropped.Format != nil {
					msgID = dropped.Format.MsgID([]byte(dropped.Data))
				}
				s.lg.Warn("stream push queue full, dropped oldest packet",
					"protocol", s.pktTag,
					"tag", dropped.Tag(), "msgid", msgID, "len", len(dropped.Data))
			}
			if sendCh == nil {
				next = q.dequeue()
				sendCh = writerCh
			}
		case sendCh <- next:
			if q.len() > 0 {
				next = q.dequeue()
			} else {
				sendCh = nil
			}
		}
	}
}

// writer owns the outbound reconnect loop.  On a transient connect
// failure it backs off; on connect success it streams packets from
// qCh; on write error it closes the connection, signals reconnectCh
// (non-blocking) so the queue can advance the MSM epoch, and
// reconnects.  On a fatal connect error (a permanent caster
// rejection) it reports State Failed and calls qCancel to stop the
// whole Push instead of retrying.
func (s *Push) writer(ctx context.Context, qCancel context.CancelFunc,
	qCh <-chan scan.Packet, reconnectCh chan<- struct{}, onState func(State, error)) {
	if onState == nil {
		onState = func(State, error) {}
	}
	b := newBackoff()
	first := true
	for {
		if first {
			onState(Connecting, nil)
			first = false
		}
		conn, err := s.dest.Connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if isFatalConnect(err) {
				s.lg.Error("ntrip push giving up", "error", err)
				onState(Failed, err)
				// Wake the queue so Run's wg.Wait returns; the
				// queue cleans up the subscription on qCtx.Done.
				qCancel()
				return
			}
			s.lg.Error("ntrip push connect failed", "error", err)
			b.increase()
			onState(Reconnecting, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(b.delay()):
			}
			onState(Connecting, nil)
			continue
		}
		b.decrease()
		onState(Connected, nil)
		// ctx-watcher: closes conn on ctx.Done() to unblock a
		// stalled Write that the per-write deadline alone would
		// take up to writeTimeout to abort.
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			case <-done:
			}
		}()
		writeErr := s.writeLoop(ctx, conn, qCh, b)
		conn.Close()
		close(done)
		if ctx.Err() != nil {
			return
		}
		if writeErr == nil {
			return
		}
		s.lg.Error("ntrip push write failed", "error", writeErr)
		select {
		case reconnectCh <- struct{}{}:
		default:
		}
		b.increase()
		onState(Reconnecting, writeErr)
		select {
		case <-ctx.Done():
			return
		case <-time.After(b.delay()):
		}
		onState(Connecting, nil)
	}
}

// writeLoop streams packets from qCh to conn.  Returns nil on clean
// exit (ctx cancelled or qCh closed), the write error otherwise.
func (s *Push) writeLoop(ctx context.Context, conn net.Conn, qCh <-chan scan.Packet, b *backoff) error {
	lastDecay := time.Now()
	for {
		var pkt scan.Packet
		var ok bool
		select {
		case <-ctx.Done():
			return nil
		case pkt, ok = <-qCh:
			if !ok {
				return nil
			}
		}
		data := pkt.Data
		if s.msm7to4 {
			out, err := rtcmbin.MSM7ConvertPacket(data, 4)
			if err != nil {
				s.lg.Debug("MSM7->MSM4 conversion failed, forwarding unchanged",
					"msgType", rtcmbin.ExtractMsgType(data), "err", err)
			} else {
				data = out
			}
		}
		if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
			return err
		}
		if _, err := conn.Write([]byte(data)); err != nil {
			return err
		}
		if time.Since(lastDecay) >= backoffDecay {
			b.decrease()
			lastDecay = time.Now()
		}
	}
}
