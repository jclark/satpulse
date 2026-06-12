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

// fatalConnectError wraps a Destination.Connect error that no amount
// of retrying can fix (e.g. a rejected NTRIP password or an invalid
// mountpoint).  The writer stops rather than reconnecting when it
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

// Push consumes scanned packets from the receiver's packet bcast and
// writes them to a remote Destination via a pruning queue and a
// reconnecting writer.  Unlike Pull, Push does not own a packet
// bcast.
type Push struct{}

// NewPush creates a Push.
func NewPush() *Push {
	return &Push{}
}

// Run subscribes to packets, filters by pktTag, queues with the
// shared pruning policy, and writes to dest with adaptive reconnect.
// Run blocks until ctx is cancelled or the input bcast is closed.
// Returns ctx.Err() on parent ctx cancellation, nil on clean
// bcast-close shutdown.
func (s *Push) Run(ctx context.Context, lg *slog.Logger,
	packets *bcast.Bcast[scan.Packet],
	dest Destination,
	pktTag gpsprot.Tag,
	msm7to4 bool,
	onState func(State, error)) error {
	iCtx, iCancel := context.WithCancel(ctx)
	defer iCancel()
	subCh := packets.Subscribe()
	qCh := make(chan scan.Packet, 1)
	reconnectCh := make(chan struct{}, 1)
	var wg sync.WaitGroup
	wg.Go(func() {
		s.queue(iCtx, iCancel, packets, subCh, reconnectCh, qCh, pktTag)
	})
	wg.Go(func() {
		s.writer(iCtx, iCancel, lg, dest, qCh, reconnectCh, msm7to4, onState)
	})
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// queue reads scanned packets from subCh, filters by pktTag,
// applies the pruning policy, and emits packets to writerCh.  On
// reconnectCh it advances the MSM epoch so dedup never spans a
// broken connection.  When subCh closes (bcast closed) it calls
// iCancel to wake the writer regardless of where the writer is
// blocked, then performs cleanup.
func (s *Push) queue(iCtx context.Context, iCancel context.CancelFunc,
	packets *bcast.Bcast[scan.Packet],
	subCh <-chan scan.Packet,
	reconnectCh <-chan struct{},
	writerCh chan scan.Packet,
	pktTag gpsprot.Tag) {
	cleanup := func() {
		close(writerCh)
		packets.Unsubscribe(subCh)
	}
	var q pruningQueue
	var outCh chan<- scan.Packet
	var front scan.Packet
	for {
		select {
		case <-iCtx.Done():
			cleanup()
			return
		case <-reconnectCh:
			// Best-effort drop of anything that was about to
			// land on the broken connection: drain writerCh
			// (we are its only sender, so a non-blocking
			// receive is safe), reset front, and bump the MSM
			// epoch.  Packets enqueued by subCh between the
			// writer's failure and our picking up reconnectCh
			// remain in q under the old epoch and may still be
			// sent after reconnect; they self-heal as fresh
			// same-msgID updates arrive.
			select {
			case <-writerCh:
			default:
			}
			outCh = nil
			q.reconnect()
			if q.len() > 0 {
				front = q.dequeue()
				outCh = writerCh
			}
		case pkt, ok := <-subCh:
			if !ok {
				iCancel()
				cleanup()
				return
			}
			if pkt.Tag() != pktTag || !pkt.ChecksumValid {
				break
			}
			q.enqueue(pkt)
			if outCh == nil {
				front = q.dequeue()
				outCh = writerCh
			}
		case outCh <- front:
			if q.len() > 0 {
				front = q.dequeue()
			} else {
				outCh = nil
			}
		}
	}
}

// writer owns the outbound reconnect loop.  On a transient connect
// failure it backs off; on connect success it streams packets from
// qCh; on write error it closes the connection, signals reconnectCh
// (non-blocking) so the queue can advance the MSM epoch, and
// reconnects.  On a fatal connect error (a permanent caster
// rejection) it reports State Failed and calls iCancel to stop the
// whole Push instead of retrying.
func (s *Push) writer(ctx context.Context, iCancel context.CancelFunc,
	lg *slog.Logger,
	dest Destination,
	qCh <-chan scan.Packet,
	reconnectCh chan<- struct{},
	msm7to4 bool,
	onState func(State, error)) {
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
		conn, err := dest.Connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if isFatalConnect(err) {
				lg.Error("ntrip push giving up", "error", err)
				onState(Failed, err)
				// Wake the queue so Run's wg.Wait returns; the
				// queue cleans up the subscription on iCtx.Done.
				iCancel()
				return
			}
			lg.Error("ntrip push connect failed", "error", err)
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
		writeErr := s.writeLoop(ctx, lg, conn, qCh, msm7to4, b)
		conn.Close()
		close(done)
		if ctx.Err() != nil {
			return
		}
		if writeErr == nil {
			return
		}
		lg.Error("ntrip push write failed", "error", writeErr)
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
func (s *Push) writeLoop(ctx context.Context, lg *slog.Logger,
	conn net.Conn, qCh <-chan scan.Packet, msm7to4 bool, b *backoff) error {
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
		if msm7to4 {
			out, err := rtcmbin.MSM7ConvertPacket(data, 4)
			if err != nil {
				lg.Debug("MSM7->MSM4 conversion failed, forwarding unchanged",
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
