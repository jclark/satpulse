package stream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/spartnbin"
	"github.com/jclark/satpulse/gps/scan"
)

// CorReportFromPacket converts a scanned correction-source packet into a
// pull-source correction report. Non-correction packets return nil. The
// MsgID field is extracted even when ChecksumOK is false; consumers must
// treat it as advisory for checksum-invalid packets.
func CorReportFromPacket(pkt scan.Packet) (*gpsprot.CorReportMsg, error) {
	switch {
	case pkt.HasTag(gpsreg.TagRTCM):
		return corReportFromRTCM(pkt)
	case pkt.HasTag(gpsreg.TagSPARTN):
		return corReportFromSPARTN(pkt)
	default:
		return nil, nil
	}
}

func corReportFromPacket(pkt scan.Packet) *gpsprot.CorReportMsg {
	return &gpsprot.CorReportMsg{
		Source:     gpsprot.CorReportSourcePull,
		Tag:        pkt.Tag(),
		MsgID:      pkt.Format.MsgID([]byte(pkt.Data)),
		NBytes:     opt.Make(len(pkt.Data)),
		ChecksumOK: opt.Make(pkt.ChecksumValid),
	}
}

func corReportFromRTCM(pkt scan.Packet) (*gpsprot.CorReportMsg, error) {
	msg := corReportFromPacket(pkt)
	if mmb, ok := rtcmbin.MultipleMessageBit(pkt.Data); ok {
		msg.FinalFragment = opt.Make(!mmb)
	}
	if !pkt.ChecksumValid {
		return msg, nil
	}
	if id, ok := rtcmbin.ReferenceStationID(pkt.Data); ok {
		msg.RTCMRefBaseID = opt.Make(id)
	}
	native, err := rtcmbin.ParseMsg(pkt.Data)
	if err != nil {
		return msg, err
	}
	msg.NativeMsg = native
	return msg, nil
}

func corReportFromSPARTN(pkt scan.Packet) (*gpsprot.CorReportMsg, error) {
	msg := corReportFromPacket(pkt)
	if !pkt.ChecksumValid {
		return msg, nil
	}
	native, err := spartnbin.Parse([]byte(pkt.Data))
	if err != nil {
		return msg, err
	}
	msg.NativeMsg = native
	return msg, nil
}

// PacketWriter writes a packet to the serial port.
// gpsio.SerialConn satisfies this interface.
type PacketWriter interface {
	WritePacket(p []byte, fmt gpsprot.PacketFormat) (int, error)
}

// ReadWriteDeadlineCloser is a correction-source connection that can also
// accept post-handshake client writes such as NMEA GGA upload.
type ReadWriteDeadlineCloser interface {
	io.Reader
	io.Writer
	io.Closer
	SetWriteDeadline(time.Time) error
}

// Source provides a network connection for correction data.
type Source interface {
	// Connect establishes a connection to the correction source and
	// returns a connection that delivers raw correction data.
	// Connect must respect ctx cancellation.
	Connect(ctx context.Context) (ReadWriteDeadlineCloser, error)
}

// TCPSource connects to a TCP address.
type TCPSource struct {
	Addr string // "host:port"
}

// Connect dials the TCP address.
func (s *TCPSource) Connect(ctx context.Context) (ReadWriteDeadlineCloser, error) {
	return (&net.Dialer{}).DialContext(ctx, "tcp", s.Addr)
}

// NtripUserAgent carries the fields used to build an Ntrip client's
// User-Agent header.  Ntrip requires the header to start with "NTRIP ".
type NtripUserAgent struct {
	Version string // e.g. "1.2.3"
}

// NtripSource is an Ntrip v1 client.  It connects to an Ntrip caster,
// sends a v1 request, and returns a stream of RTCM bytes on success.
// Only "ICY 200 OK" is accepted as a successful response.
type NtripSource struct {
	Addr       string // "host:port"
	Mountpoint string
	Username   string
	Password   string
	UserAgent  NtripUserAgent
}

// Connect dials the caster, performs the Ntrip v1 handshake, and
// returns a reader over the RTCM body.
func (s *NtripSource) Connect(ctx context.Context) (ReadWriteDeadlineCloser, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return nil, err
	}
	// Close conn on ctx cancellation during write/read.  The dial
	// itself is covered by DialContext.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	rc, err := s.handshake(conn)
	close(done)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return rc, nil
}

// handshake writes the Ntrip v1 request, reads the status line, and
// returns a connection over the body.  On error, the caller
// closes conn.
func (s *NtripSource) handshake(conn net.Conn) (ReadWriteDeadlineCloser, error) {
	if _, err := conn.Write([]byte(s.request())); err != nil {
		return nil, err
	}
	br := bufio.NewReaderSize(conn, 4096)
	line, err := br.ReadSlice('\n')
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(line, []byte("ICY 200 OK\r\n")) {
		return nil, fmt.Errorf("Ntrip: %s", strings.TrimSuffix(string(line), "\r\n"))
	}
	if br.Buffered() == 0 {
		return conn, nil
	}
	leftover, _ := br.Peek(br.Buffered())
	return &readBufferedConn{Reader: io.MultiReader(bytes.NewReader(leftover), conn), conn: conn}, nil
}

type readBufferedConn struct {
	io.Reader
	conn net.Conn
}

func (c *readBufferedConn) Write(p []byte) (int, error) { return c.conn.Write(p) }
func (c *readBufferedConn) Close() error                { return c.conn.Close() }
func (c *readBufferedConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// request builds the Ntrip v1 request bytes.
func (s *NtripSource) request() string {
	var b strings.Builder
	fmt.Fprintf(&b, "GET /%s HTTP/1.0\r\n", s.Mountpoint)
	fmt.Fprintf(&b, "User-Agent: %s\r\n", s.userAgent())
	if s.Username != "" {
		creds := base64.StdEncoding.EncodeToString([]byte(s.Username + ":" + s.Password))
		fmt.Fprintf(&b, "Authorization: Basic %s\r\n", creds)
	}
	b.WriteString("\r\n")
	return b.String()
}

func (s *NtripSource) userAgent() string {
	if s.UserAgent.Version == "" {
		return "NTRIP satpulse"
	}
	return "NTRIP satpulse/" + s.UserAgent.Version
}

// Pull reads correction packets from a Source and writes them to a
// serial port via a pruning queue.  Scanned packets are broadcast
// to subscribers so that subscriber timing reflects true
// network-receive time.
type Pull struct {
	// Packets broadcasts every scanned packet from the correction
	// source.  The caller should subscribe before calling Run.
	// The bcast lives for the entire duration of Run, surviving
	// reconnects -- subscribers are not affected by network drops.
	Packets          *bcast.Bcast[scan.Packet]
	source           Source
	lg               *slog.Logger
	pw               PacketWriter
	portLock         gpsio.OutPortLock
	pktFormats       []gpsprot.PacketFormat
	nmeaSendInterval time.Duration
	pktCh            chan scan.Packet
}

// NewPull creates a Pull. The caller should subscribe to s.Packets before
// calling Run.
func NewPull(source Source, lg *slog.Logger,
	pw PacketWriter,
	portLock gpsio.OutPortLock,
	pktFormats []gpsprot.PacketFormat,
	nmeaSendInterval time.Duration,
) *Pull {
	if lg == nil {
		lg = slog.Default()
	}
	ch := make(chan scan.Packet)
	return &Pull{
		Packets:          bcast.New(ch),
		source:           source,
		lg:               lg,
		pw:               pw,
		portLock:         portLock,
		pktFormats:       pktFormats,
		nmeaSendInterval: nmeaSendInterval,
		pktCh:            ch,
	}
}

const scanBufSize = 16
const ggaWriteTimeout = 5 * time.Second

// maxGGASendTimeouts is the maximum number of consecutive upload write-deadline
// timeouts the GGA sender tolerates; the next timeout drops the connection to
// force a reconnect.  Transient stalls keep the correction read stream up; each
// timeout takes up to ggaWriteTimeout, so a wedged upload is dropped within
// about (maxGGASendTimeouts+1)*ggaWriteTimeout.
const maxGGASendTimeouts = 2

// errReconnect is sent as a scan.Packet.ReadError to signal the
// pruning queue that the connection was lost.
var errReconnect = errors.New("reconnect")

// ErrNoUsableGGA reports that a selected-GGA feed ended without a usable fix.
var ErrNoUsableGGA = errors.New("no usable GGA with position fix")

// GGASender uploads selected GGA packets to a connected correction source.
type GGASender struct {
	selected <-chan scan.Packet
	interval time.Duration
	connCh   chan ReadWriteDeadlineCloser
	ready    chan struct{}
	done     chan struct{}
}

// NewGGASender creates a GGA sender for a selected-GGA packet feed.
// interval is the resend period; 0 uploads the latest GGA once per
// connection.
func NewGGASender(selected <-chan scan.Packet, interval time.Duration) *GGASender {
	return &GGASender{
		selected: selected,
		interval: interval,
		connCh:   make(chan ReadWriteDeadlineCloser, 1),
		ready:    make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Run holds the latest usable GGA and uploads it to the current
// connection: once on connect, then every interval if interval > 0.
func (s *GGASender) Run(ctx context.Context, lg *slog.Logger) {
	defer close(s.done)
	var tickCh <-chan time.Time
	if s.interval > 0 {
		t := time.NewTicker(s.interval)
		defer t.Stop()
		tickCh = t.C
	}
	// latest is the latest usable GGA wire bytes ("" until the first fix);
	// conn is the upload connection (nil when none); nTimeouts counts
	// consecutive upload write-deadline timeouts.
	var latest string
	var conn ReadWriteDeadlineCloser
	var nTimeouts int
	for {
		select {
		case pkt, ok := <-s.selected:
			if !ok {
				s.selected = nil
				if latest == "" {
					return
				}
				continue
			}
			if _, ok := GGAPacketPosition(pkt); !ok {
				continue
			}
			if latest == "" {
				close(s.ready) // first usable GGA; unblock reader()
			}
			latest = pkt.Data
		case <-tickCh:
			if conn != nil && latest != "" {
				conn, nTimeouts = sendGGA(ctx, lg, conn, latest, nTimeouts)
			}
		case conn = <-s.connCh:
			nTimeouts = 0
			if latest != "" {
				conn, nTimeouts = sendGGA(ctx, lg, conn, latest, nTimeouts)
			}
		case <-ctx.Done():
			return
		}
	}
}

// sendGGA uploads data to conn and returns the connection (nil if it was
// dropped) and the updated consecutive-timeout count.  A clean write timeout
// keeps the connection through a transient stall; a hard error, or more than
// maxGGASendTimeouts consecutive timeouts, drops it so the reader reconnects.
// Errors after cancellation are suppressed because the reader closes the same
// connection to unblock its scan during shutdown.
func sendGGA(ctx context.Context, lg *slog.Logger, conn ReadWriteDeadlineCloser, data string, nTimeouts int) (ReadWriteDeadlineCloser, int) {
	err := writeGGA(conn, data)
	if err != nil && ctx.Err() != nil {
		conn.Close()
		return nil, 0
	}
	switch {
	case err == nil:
		return conn, 0
	case errors.Is(err, errGGATimeout):
		nTimeouts++
		if nTimeouts == 1 {
			lg.Warn("NMEA GGA upload timed out; keeping connection")
		}
		if nTimeouts <= maxGGASendTimeouts {
			return conn, nTimeouts
		}
		lg.Warn("NMEA GGA upload timed out repeatedly; dropping connection", "timeouts", nTimeouts)
	default:
		lg.Warn("NMEA GGA upload failed", "err", err)
	}
	conn.Close()
	return nil, 0
}

// Ready reports whether the sender has already seen a usable GGA packet,
// so a caller can avoid logging that it is waiting when it is not.
func (s *GGASender) Ready() bool {
	select {
	case <-s.ready:
		return true
	default:
		return false
	}
}

// WaitReady waits until the sender has seen a usable GGA packet.
func (s *GGASender) WaitReady(ctx context.Context) error {
	select {
	case <-s.ready:
		return nil
	case <-s.done:
		select {
		case <-s.ready:
			return nil
		default:
			// Run closes done on ctx cancellation too; don't misreport a
			// clean cancel as a missing fix.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrNoUsableGGA
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SetWriter gives the sender the current post-handshake connection.
func (s *GGASender) SetWriter(ctx context.Context, w ReadWriteDeadlineCloser) bool {
	select {
	case s.connCh <- w:
		return true
	case <-ctx.Done():
		return false
	}
}

// Run connects to the correction source, scans packets, and writes
// each to serialConn via WritePacket.  On network error, Run
// reconnects with adaptive backoff.  It calls onState on each
// connection state change.
// Run blocks until ctx is cancelled or serialConn errors.
// On cancellation, Run waits for all internal goroutines to exit
// before returning.
func (s *Pull) Run(ctx context.Context, selectedGGA <-chan scan.Packet, onState func(State, error)) error {
	iCtx, iCancel := context.WithCancel(ctx)
	defer iCancel()
	// writeErr captures a fatal write error so Run can return it
	// instead of context.Canceled.
	var writeErr error
	var pipelineWg sync.WaitGroup
	var bcastWg sync.WaitGroup
	// start bcast goroutine
	bcastWg.Go(func() {
		s.Packets.Run(iCtx, s.lg)
	})
	var gs *GGASender
	if selectedGGA != nil {
		gs = NewGGASender(selectedGGA, s.nmeaSendInterval)
		pipelineWg.Go(func() {
			gs.Run(iCtx, s.lg)
		})
	}
	// subscribe to bcast before starting reader to avoid missing packets
	subCh := s.Packets.Subscribe()
	// queue channel from pruning queue to writer
	qCh := make(chan scan.Packet, 1)
	// start writer
	pipelineWg.Go(func() {
		writeErr = s.writer(iCtx, s.lg, s.pw, s.portLock, qCh, iCancel)
	})
	// start pruning queue
	pipelineWg.Go(func() {
		s.queue(s.lg, subCh, qCh)
	})
	// start reader
	pipelineWg.Go(func() {
		s.reader(iCtx, s.lg, s.source, s.pktFormats, gs, onState)
	})
	pipelineWg.Wait()
	s.Packets.Close()
	bcastWg.Wait()
	if writeErr != nil {
		return writeErr
	}
	return iCtx.Err()
}

// reader owns the reconnect loop.  It connects to the source, scans
// packets, and sends them into the bcast input channel.  On network
// error it reconnects with adaptive backoff.  It closes pktCh on
// exit.
func (s *Pull) reader(ctx context.Context, lg *slog.Logger,
	source Source, pktFormats []gpsprot.PacketFormat,
	gs *GGASender, onState func(State, error)) {
	defer close(s.pktCh)
	if onState == nil {
		onState = func(State, error) {}
	}
	if gs != nil {
		if !gs.Ready() {
			lg.Info("NMEA send pull waiting for first position fix before connecting")
		}
		if err := gs.WaitReady(ctx); err != nil {
			if ctx.Err() == nil {
				lg.Warn("NMEA send pull stopped before first position fix", "err", err)
			}
			return
		}
	}
	b := newBackoff()
	first := true
	for {
		if first {
			onState(Connecting, nil)
			first = false
		}
		conn, err := source.Connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			lg.Error("correction source connect failed", "error", err)
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
		if gs != nil && !gs.SetWriter(ctx, conn) {
			conn.Close()
			return
		}
		onState(Connected, nil)
		// cancel goroutine: closes conn when ctx is done to unblock Scan
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			case <-done:
			}
		}()
		readErr := s.readLoop(ctx, lg, conn, pktFormats, b)
		conn.Close()
		close(done)
		select {
		case s.pktCh <- scan.Packet{ReadError: errReconnect}:
		case <-ctx.Done():
			return
		}
		if ctx.Err() != nil {
			return
		}
		if readErr == nil {
			lg.Info("correction source closed connection")
		}
		b.increase()
		onState(Reconnecting, readErr)
		select {
		case <-ctx.Done():
			return
		case <-time.After(b.delay()):
		}
		onState(Connecting, nil)
	}
}

// readLoop scans packets from conn and sends them to the bcast input
// channel.  Returns nil on clean EOF, the read error otherwise, or
// nil if cancelled via ctx.  It calls b.decrease periodically while
// the connection is healthy.
func (s *Pull) readLoop(ctx context.Context, lg *slog.Logger,
	conn io.ReadCloser, pktFormats []gpsprot.PacketFormat, b *backoff) error {
	scanner := scan.New(conn, scanBufSize, pktFormats)
	lastDecay := time.Now()
	for {
		pkt, err := scanner.Scan()
		if pkt.Format != nil && !pkt.ChecksumValid {
			msgID := pkt.Format.MsgID([]byte(pkt.Data))
			lg.Warn("invalid correction checksum", "tag", pkt.Format.Tag(), "msg", msgID,
				"len", len(pkt.Data), "err", pkt.ChecksumError())
		}
		select {
		case s.pktCh <- pkt:
		case <-ctx.Done():
			return nil
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if time.Since(lastDecay) >= backoffDecay {
			b.decrease()
			lastDecay = time.Now()
		}
	}
}

// queue subscribes to the bcast and mediates between the reader and
// the writer using the shared pruning queue.  When the subscription
// channel closes, the queue discards remaining packets, closes the
// writer channel, and returns.
func (s *Pull) queue(lg *slog.Logger, subCh <-chan scan.Packet, writerCh chan<- scan.Packet) {
	defer close(writerCh)
	defer s.Packets.Unsubscribe(subCh)
	var q pruningQueue
	// If sendCh is non-nil, it is writerCh and next is ready to send.
	var sendCh chan<- scan.Packet
	var next scan.Packet
	for {
		select {
		case pkt, ok := <-subCh:
			if !ok {
				return
			}
			if pkt.Format == nil {
				if errors.Is(pkt.ReadError, errReconnect) {
					q.reconnect()
				}
				break
			}
			if dropped, ok := q.enqueue(pkt); ok {
				msgID := ""
				if dropped.Format != nil {
					msgID = dropped.Format.MsgID([]byte(dropped.Data))
				}
				lg.Warn("stream pull queue full, dropped oldest packet",
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

// writer receives packets from the queue and writes them to the
// serial port.
func (s *Pull) writer(ctx context.Context, lg *slog.Logger,
	pw PacketWriter, portLock gpsio.OutPortLock,
	qCh <-chan scan.Packet, cancel context.CancelFunc) error {
	for pkt := range qCh {
		var port gpsio.OutPort
		select {
		case <-ctx.Done():
			return nil
		case port = <-portLock:
		}
		_, err := pw.WritePacket([]byte(pkt.Data), pkt.Format)
		portLock <- port
		if err != nil {
			lg.Error("correction serial write failed", "error", err)
			cancel()
			return err
		}
	}
	return nil
}

// GGAPacketPosition returns the lat/lon of pkt when it is a usable GGA
// fix (checksum-valid, quality > 0, lat/lon set), and ok == false otherwise.
func GGAPacketPosition(pkt scan.Packet) ([2]float64, bool) {
	var pos [2]float64
	gga, ok := packetGGASentence(pkt)
	if !ok || gga.Fields.Quality == 0 || !gga.Fields.Lat.IsSet() || !gga.Fields.Lon.IsSet() {
		return pos, false
	}
	pos[0], pos[1] = gga.Fields.LatLon()
	return pos, true
}

// errGGATimeout reports a GGA upload that hit its write deadline without sending
// any bytes.  The stream framing is intact, so the sender keeps the connection
// and retries rather than reconnecting on a transient stall.
var errGGATimeout = errors.New("NMEA GGA upload timed out")

func writeGGA(w ReadWriteDeadlineCloser, data string) error {
	if err := w.SetWriteDeadline(time.Now().Add(ggaWriteTimeout)); err != nil {
		return err
	}
	n, err := io.WriteString(w, data)
	if e := w.SetWriteDeadline(time.Time{}); err == nil {
		err = e
	}
	if err != nil {
		// A clean timeout with nothing written is recoverable; a partial write
		// corrupts framing, so report it as a hard error.
		var ne net.Error
		if n == 0 && errors.As(err, &ne) && ne.Timeout() {
			return errGGATimeout
		}
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}
