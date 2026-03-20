// Package stream connects to a correction source, scans packets, and writes
// them to a serial port.  It handles reconnection internally and exposes a
// bcast of scanned packets so callers can subscribe for UI updates, logging,
// or other purposes.
package stream

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/scan"
)

// PacketWriter writes a packet to the serial port.
// gpsio.SerialConn satisfies this interface.
type PacketWriter interface {
	WritePacket(p []byte, fmt gpsprot.PacketFormat) (int, error)
}

// Source provides a network connection for correction data.
type Source interface {
	// Connect establishes a connection to the correction source and
	// returns a net.Conn that delivers raw correction data.  Connect
	// must respect ctx cancellation.
	Connect(ctx context.Context) (net.Conn, error)
}

// TCPSource connects to a TCP address.
type TCPSource struct {
	Addr string // "host:port"
}

// Connect dials the TCP address.
func (s *TCPSource) Connect(ctx context.Context) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "tcp", s.Addr)
}

// State represents the connection state of a Pull.
type State int

const (
	Connecting   State = iota
	Connected
	Reconnecting
)

func (s State) String() string {
	switch s {
	case Connecting:
		return "connecting"
	case Connected:
		return "connected"
	case Reconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
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
		s.queue(subCh, qCh)
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
		readErr := s.readLoop(ctx, conn, pktFormats, b)
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
func (s *Pull) readLoop(ctx context.Context,
	conn net.Conn, pktFormats []gpsprot.PacketFormat, b *backoff) error {
	scanner := scan.New(conn, scanBufSize, pktFormats)
	lastDecay := time.Now()
	for {
		pkt, err := scanner.Scan()
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
// the writer.  It maintains a FIFO ordered by insertion time, plus a
// map from message type to queue position for O(1) dedup.  On
// enqueue, if a packet of the same message type is already in the
// queue, the old entry is removed.  When the subscription channel
// closes, the queue discards remaining packets, closes the writer
// channel, and returns.
func (s *Pull) queue(subCh <-chan scan.Packet, writerCh chan<- scan.Packet) {
	defer close(writerCh)
	defer s.Packets.Unsubscribe(subCh)
	var q pruningQueue
	var outCh chan<- scan.Packet
	var front scan.Packet
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

// pruningQueue is a FIFO that deduplicates by message type.  When a
// packet with the same message type is enqueued, the older entry is
// removed -- unless both entries belong to the same MSM epoch, in
// which case a single epoch can legitimately contain multiple packets
// with the same message type.
type pruningQueue struct {
	entries  []pqEntry
	msmEpoch uint64
}

type pqEntry struct {
	pkt      scan.Packet
	msgID    string
	msmEpoch *uint64
}

func (q *pruningQueue) len() int {
	return len(q.entries)
}

func (q *pruningQueue) reconnect() {
	q.msmEpoch++
}

func (q *pruningQueue) enqueue(pkt scan.Packet) {
	msgID := pkt.Format.MsgID([]byte(pkt.Data))
	var epoch *uint64
	if pkt.Format.Tag() == gpsreg.TagRTCM {
		if mmb, ok := rtcmbin.MultipleMessageBit(pkt.Data); ok {
			ep := q.msmEpoch
			epoch = &ep
			if !mmb {
				q.msmEpoch++
			}
		}
	}
	q.entries = slices.DeleteFunc(q.entries, func(e pqEntry) bool {
		return e.msgID == msgID && (epoch == nil || e.msmEpoch == nil || *epoch != *e.msmEpoch)
	})
	q.entries = append(q.entries, pqEntry{pkt: pkt, msgID: msgID, msmEpoch: epoch})
}

func (q *pruningQueue) dequeue() scan.Packet {
	e := q.entries[0]
	q.entries = q.entries[1:]
	return e.pkt
}
