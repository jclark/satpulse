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
	"github.com/jclark/satpulse/gps/scan"
)

// defaultPullFormats are the packet formats stream pull recognises
// from a correction source.  RTCM is currently the only one.
var defaultPullFormats = []gpsprot.PacketFormat{gpsreg.RTCMPacketFormat}

// CorReportFromPacket converts a scanned correction-source packet into a
// pull-source correction report. Non-RTCM packets return nil. The MsgID
// field is extracted even when ChecksumOK is false; consumers must treat it
// as advisory for checksum-invalid packets.
func CorReportFromPacket(pkt scan.Packet) (*gpsprot.CorReportMsg, error) {
	if !pkt.HasTag(gpsreg.TagRTCM) {
		return nil, nil
	}
	msg := &gpsprot.CorReportMsg{
		Source:     gpsprot.CorReportSourcePull,
		Tag:        gpsreg.TagRTCM,
		MsgID:      rtcmbin.ExtractMsgID(pkt.Data),
		NBytes:     opt.Make(len(pkt.Data)),
		ChecksumOK: opt.Make(pkt.ChecksumValid),
	}
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

// PacketWriter writes a packet to the serial port.
// gpsio.SerialConn satisfies this interface.
type PacketWriter interface {
	WritePacket(p []byte, fmt gpsprot.PacketFormat) (int, error)
}

// Source provides a network connection for correction data.
type Source interface {
	// Connect establishes a connection to the correction source and
	// returns an io.ReadCloser that delivers raw correction data.
	// Connect must respect ctx cancellation.
	Connect(ctx context.Context) (io.ReadCloser, error)
}

// TCPSource connects to a TCP address.
type TCPSource struct {
	Addr string // "host:port"
}

// Connect dials the TCP address.
func (s *TCPSource) Connect(ctx context.Context) (io.ReadCloser, error) {
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
func (s *NtripSource) Connect(ctx context.Context) (io.ReadCloser, error) {
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
// returns an io.ReadCloser over the body.  On error, the caller
// closes conn.
func (s *NtripSource) handshake(conn net.Conn) (io.ReadCloser, error) {
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
	return struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(leftover), conn), conn}, nil
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
	Packets *bcast.Bcast[scan.Packet]
	pktCh   chan scan.Packet
}

// NewPull creates a Pull.  The caller should subscribe to
// s.Packets before calling Run.
func NewPull() *Pull {
	ch := make(chan scan.Packet)
	return &Pull{
		Packets: bcast.New(ch),
		pktCh:   ch,
	}
}

// PullSetup is a Pull paired with the resources it needs to run
// (source, packet writer, output port lock, and packet formats).
// Built by (*PullConfig).Prepare.
type PullSetup struct {
	pull       *Pull
	source     Source
	addr       string
	pktFormats []gpsprot.PacketFormat
	pw         PacketWriter
	portLock   gpsio.OutPortLock
}

// Addr returns the source address string, for use in log lines.
func (s *PullSetup) Addr() string {
	return s.addr
}

// Bcast returns the packet broadcast for pull-observer subscribers.
func (s *PullSetup) Bcast() *bcast.Bcast[scan.Packet] {
	return s.pull.Packets
}

// Run runs the prepared Pull.  It blocks until ctx is cancelled or
// the serial writer returns a fatal error.
func (s *PullSetup) Run(ctx context.Context, lg *slog.Logger,
	onState func(State, error)) error {
	return s.pull.Run(ctx, lg, s.source, s.pw, s.portLock, s.pktFormats, onState)
}

const scanBufSize = 16

// errReconnect is sent as a scan.Packet.ReadError to signal the
// pruning queue that the connection was lost.
var errReconnect = errors.New("reconnect")

// Run connects to the correction source, scans packets, and writes
// each to serialConn via WritePacket.  On network error, Run
// reconnects with adaptive backoff.  It calls onState on each
// connection state change.
// Run blocks until ctx is cancelled or serialConn errors.
// On cancellation, Run waits for all internal goroutines to exit
// before returning.
func (s *Pull) Run(ctx context.Context, lg *slog.Logger,
	source Source,
	pw PacketWriter,
	portLock gpsio.OutPortLock,
	pktFormats []gpsprot.PacketFormat,
	onState func(State, error)) error {
	iCtx, iCancel := context.WithCancel(ctx)
	defer iCancel()
	// writeErr captures a fatal write error so Run can return it
	// instead of context.Canceled.
	var writeErr error
	var pipelineWg sync.WaitGroup
	var bcastWg sync.WaitGroup
	// start bcast goroutine
	bcastWg.Go(func() {
		s.Packets.Run(iCtx, lg)
	})
	// subscribe to bcast before starting reader to avoid missing packets
	subCh := s.Packets.Subscribe()
	// queue channel from pruning queue to writer
	qCh := make(chan scan.Packet, 1)
	// start writer
	pipelineWg.Go(func() {
		writeErr = s.writer(iCtx, lg, pw, portLock, qCh, iCancel)
	})
	// start pruning queue
	pipelineWg.Go(func() {
		s.queue(lg, subCh, qCh)
	})
	// start reader
	pipelineWg.Go(func() {
		s.reader(iCtx, lg, source, pktFormats, onState)
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
	onState func(State, error)) {
	defer close(s.pktCh)
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
