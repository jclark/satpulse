// Package corrsink connects to a correction source, scans packets, and writes
// them to a serial port.  It handles reconnection internally and exposes a
// bcast of scanned packets so callers can subscribe for UI updates, logging,
// or other purposes.
package corrsink

import (
	"context"
	"io"
	"log/slog"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
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

// State represents the connection state of a Sink.
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

// Sink reads correction packets from a Source and writes them to a
// serial port via a pruning queue.  Scanned packets are broadcast
// to subscribers so that subscriber timing reflects true
// network-receive time.
type Sink struct {
	// Packets broadcasts every scanned packet from the correction
	// source.  The caller should subscribe before calling Run.
	// The bcast lives for the entire duration of Run, surviving
	// reconnects -- subscribers are not affected by network drops.
	Packets *bcast.Bcast[scan.Packet]
	pktCh   chan scan.Packet
}

// NewSink creates a Sink.  The caller should subscribe to
// s.Packets before calling Run.
func NewSink() *Sink {
	ch := make(chan scan.Packet)
	return &Sink{
		Packets: bcast.New(ch),
		pktCh:   ch,
	}
}

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
	scanBufSize    = 16
)

// Run connects to the correction source, scans packets, and writes
// each to serialConn via WritePacket.  On network error, Run
// reconnects with exponential backoff (1s, 2s, 4s, ... capped at
// 30s).  It calls onState on each connection state change.
// Run blocks until ctx is cancelled or serialConn errors.
// On cancellation, Run waits for all internal goroutines to exit
// before returning.
func (s *Sink) Run(ctx context.Context, lg *slog.Logger,
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
	qCh := make(chan scan.Packet)
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
// error it reconnects with exponential backoff.  It closes pktCh on
// exit.
func (s *Sink) reader(ctx context.Context, lg *slog.Logger,
	source Source, pktFormats []gpsprot.PacketFormat,
	onState func(State, error)) {
	defer close(s.pktCh)
	if onState == nil {
		onState = func(State, error) {}
	}
	backoff := initialBackoff
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
			onState(Reconnecting, err)
			if !s.backoffWait(ctx, &backoff) {
				return
			}
			onState(Connecting, nil)
			continue
		}
		onState(Connected, nil)
		backoff = initialBackoff
		// cancel goroutine: closes conn when ctx is done to unblock Scan
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			case <-done:
			}
		}()
		readErr := s.readLoop(ctx, conn, pktFormats)
		conn.Close()
		close(done)
		if ctx.Err() != nil {
			return
		}
		if readErr == nil {
			lg.Info("correction source closed connection")
		}
		onState(Reconnecting, readErr)
		if !s.backoffWait(ctx, &backoff) {
			return
		}
		onState(Connecting, nil)
	}
}

// readLoop scans packets from conn and sends them to the bcast input
// channel.  Returns nil on clean EOF, the read error otherwise, or
// nil if cancelled via ctx.
func (s *Sink) readLoop(ctx context.Context,
	conn net.Conn, pktFormats []gpsprot.PacketFormat) error {
	scanner := scan.New(conn, scanBufSize, pktFormats)
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
	}
}

// backoffWait waits for the current backoff duration, then doubles it
// (capped at maxBackoff).  Returns false if ctx was cancelled.
func (s *Sink) backoffWait(ctx context.Context, backoff *time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(*backoff):
	}
	*backoff *= 2
	if *backoff > maxBackoff {
		*backoff = maxBackoff
	}
	return true
}

// queue subscribes to the bcast and mediates between the reader and
// the writer.  It maintains a FIFO ordered by insertion time, plus a
// map from message type to queue position for O(1) dedup.  On
// enqueue, if a packet of the same message type is already in the
// queue, the old entry is removed.  When the subscription channel
// closes, the queue discards remaining packets, closes the writer
// channel, and returns.
func (s *Sink) queue(subCh <-chan scan.Packet, writerCh chan<- scan.Packet) {
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
func (s *Sink) writer(ctx context.Context, lg *slog.Logger,
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
// removed.
type pruningQueue struct {
	entries []pqEntry
}

type pqEntry struct {
	pkt     scan.Packet
	msgID   string
	hasMore bool
}

func (q *pruningQueue) len() int {
	return len(q.entries)
}

func (q *pruningQueue) enqueue(pkt scan.Packet) {
	msgID := pkt.Format.MsgID([]byte(pkt.Data))
	hasMore := false
	if mpf, ok := pkt.Format.(gpsprot.MultiPacketFormat); ok {
		hasMore = mpf.HasMore([]byte(pkt.Data))
	}
	// If the last entry has more packets coming with the same msgID,
	// this packet is a continuation of the same group.
	n := len(q.entries)
	if n > 0 && q.entries[n-1].hasMore && q.entries[n-1].msgID == msgID {
		q.entries = append(q.entries, pqEntry{pkt: pkt, msgID: msgID, hasMore: hasMore})
		return
	}
	// Remove all entries with the same msgID.
	q.entries = slices.DeleteFunc(q.entries, func(e pqEntry) bool {
		return e.msgID == msgID
	})
	q.entries = append(q.entries, pqEntry{pkt: pkt, msgID: msgID, hasMore: hasMore})
}

func (q *pruningQueue) dequeue() scan.Packet {
	e := q.entries[0]
	q.entries = q.entries[1:]
	return e.pkt
}
