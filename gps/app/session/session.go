// Package session implements an interactive session with a GPS
// receiver: connect, probe, configure, send messages, monitor,
// disconnect. It is the application core shared by GUI shells; each
// shell provides a Sink that delivers session events to its UI
// transport.
//
// A Session is a state machine over ConnState, driven by the methods
// below and reported as gps:state events: Connect moves Disconnected
// -> Connecting -> Connected (probing on the way, with the result as
// a gps:receiver event), and Disconnect returns to Disconnected from
// any state. The receiver operations -- ReadConfig, ApplyConfig,
// SendMsgFile -- require StateConnected and are mutually exclusive:
// each holds the receiver port for its duration (StateConfiguring or
// StateSending), and a call while another operation is in progress
// returns an error rather than queueing. StartCorrections also
// requires StateConnected; the correction session it starts persists
// across receiver operations and ends with the connection.
//
// Methods are safe for concurrent use, and concurrent calls have
// defined semantics regardless of how the shell behaves: the last
// Connect or Disconnect wins (a superseded Connect closes any
// late-opened transport, returns an error, and cannot start a
// connection manager), and racing receiver operations lose to
// whichever holds the port. Operation completion is scoped to the
// pipeline run where it started. All state changes flow to the shell
// as ordered events. Event callbacks may call the snapshot accessors
// (State, Receiver, Speed, CorrectionsState), which also serve
// late-joining consumers.
package session

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/stream"
	"github.com/jclark/satpulse/gps/gpsdecode"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/msgfile"
	"github.com/jclark/satpulse/gps/nmeasyn"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/scan"
)

// ConnState represents the unified connection/activity state, emitted
// as gps:state events. Connecting and Reconnecting are transitional;
// Configuring and Sending mean an exclusive receiver operation holds
// the port, and further operations are refused until it completes.
type ConnState string

const (
	StateDisconnected ConnState = "disconnected"
	StateConnecting   ConnState = "connecting"
	StateReconnecting ConnState = "reconnecting" // transport lost after a reset; re-opening and re-probing
	StateConnected    ConnState = "connected"
	StateConfiguring  ConnState = "configuring"
	StateSending      ConnState = "sending"
)

const (
	packetLogChannelSize = 64
	// connectOpenTimeout bounds the wait for the device on the initial
	// Connect; a missing device should fail fast.
	connectOpenTimeout = 2 * time.Second
	// reopenTimeout bounds one reconnect attempt: USB re-enumeration
	// after a reset takes a few seconds.
	reopenTimeout = 10 * time.Second
	// reopenDelay separates reconnect attempts.
	reopenDelay = time.Second
	// maxReopenAttempts is how many times reopen retries the device node
	// after a reset re-enumerates it; 0 would disable reconnect.
	maxReopenAttempts = 3
)

// configRequest is sent to packetWorker to run gpscfg.Configure.
type configRequest struct {
	ctx      context.Context
	target   *gpsprot.ConfigTarget
	resultCh chan<- configResult
}

// configResult is the response from a configRequest.
type configResult struct {
	result *gpscfg.Result
	err    error
}

// sendStepReq is sent from the coordinator to the worker for each message.
// The worker writes the message, waits for its delay, and (if next is non-nil)
// waits for ReadyToSend(next) before replying.
type sendStepReq struct {
	rm    msgfile.RawMsg
	next  *msgfile.RawMsg // nil for the last message
	reply chan<- error
}

// EventName identifies a session event on the UI wire.
type EventName string

// The session events and their payload types.
const (
	EventState        EventName = "gps:state"        // ConnState
	EventReceiver     EventName = "gps:receiver"     // ReceiverEvent
	EventSpeed        EventName = "gps:speed"        // int (host serial port speed)
	EventLog          EventName = "gps:log"          // LogEvent
	EventMsg          EventName = "gps:msg"          // MsgEvent
	EventTime         EventName = "gps:time"         // *gpsprot.TimeMsg
	EventEpochPVT     EventName = "gps:epochPVT"     // gpsprot.PVMsgBundle
	EventInitialPos   EventName = "gps:initialPos"   // [2]float64 (lat, lon in degrees)
	EventNMEAPosition EventName = "gps:nmeaPosition" // NMEAPositionEvent
	EventMsgSend      EventName = "gps:msgsend"      // MsgSendEvent
	EventResponse     EventName = "gps:response"     // ResponseEvent
	EventCorrections  EventName = "gps:corrections"  // CorrEvent
	EventCorrPacket   EventName = "gps:corrpacket"   // *gpsprot.CorReportMsg
	EventBaseARP      EventName = "gps:basearp"      // BaseARPEvent
	EventPacket       EventName = "gps:packet"       // gpsio.PacketLogEntry
)

// Event is a named event with a JSON-serializable payload.
type Event struct {
	Name EventName
	Data any // one of the payload types documented on the EventName constants
}

// Sink delivers events to the UI transport. Called from session
// goroutines; Emit must not block (drop or buffer).
type Sink interface {
	// Emit may call Session snapshot accessors, but must not synchronously
	// call other Session methods: events can originate on goroutines those
	// methods wait for.
	Emit(Event)
	// Wants reports whether anyone is listening for this event.
	// The session uses it to suppress expensive high-rate streams
	// (gps:packet). A sink whose transport always listens returns
	// true unconditionally.
	Wants(EventName) bool
}

func emit(sink Sink, name EventName, data any) {
	sink.Emit(Event{Name: name, Data: data})
}

// Options configures a Session.
type Options struct {
	PacketLog io.Writer // optional JSONL packet log; writes are serialized (nil to disable)
}

// Session is an interactive session with a GPS receiver.
// Its methods are safe for concurrent use.
type Session struct {
	lg   *slog.Logger
	sink Sink
	opts Options
	mu   sync.Mutex
	// lifecycleMu serializes connection shutdown and manager startup.
	// It is not held while opening a transport, so a later lifecycle
	// call can still supersede a pending Connect.
	lifecycleMu sync.Mutex
	// op and vendor are set by Connect and immutable while the
	// connection lives.
	op     Opener
	vendor gpsreg.Vendor
	state  ConnState
	// stateSeq counts state transitions; stateEmitSeq is the last
	// transition whose state has been published. emitStateChange uses
	// them to keep gps:state events ordered when transitions race.
	stateSeq      int
	stateEmitSeq  int
	stateEmitMu   sync.Mutex
	stateEmitting bool
	// connCtx spans the logical connection, Connect to Disconnect;
	// runCtx spans one packet pipeline run within it. They differ only
	// when a reset-bearing operation kills the transport: the manager
	// ends the run, re-opens the transport, and starts a new run under
	// the same connCtx.
	connCtx    context.Context
	connCancel context.CancelFunc
	// connectGen is reserved at the start of each lifecycle call, so a
	// Connect can tell that a later Connect or Disconnect superseded it
	// during shutdown or while Open was pending.
	connectGen   int
	runCtx       context.Context
	runCancel    context.CancelFunc
	connWg       sync.WaitGroup
	conn         gpsio.Conn
	portLock     gpsio.OutPortLock
	pb           *bcast.Bcast[scan.Packet]
	configCh     chan configRequest
	probe        ReceiverEvent
	speed        int
	resetPending bool
	mh           *msgHandler
	gga          *ggaMonitor
	msgFile      *msgfile.Parsed
	sendCancel   context.CancelFunc
	workerCancel context.CancelFunc
	respSession  atomic.Int32
	corrState    CorrEvent
	corrCancel   context.CancelFunc
	corrWg       *sync.WaitGroup
	corrStopping bool
	packetLogMu  sync.Mutex
	packetLogW   io.Writer
}

// New creates a Session that delivers events to sink.
func New(lg *slog.Logger, sink Sink, opts Options) *Session {
	return &Session{
		lg:         lg,
		sink:       sink,
		opts:       opts,
		packetLogW: opts.PacketLog,
		state:      StateDisconnected,
		corrState:  CorrEvent{State: "stopped"},
	}
}

func (s *Session) emit(name EventName, data any) {
	emit(s.sink, name, data)
}

// setStateLocked records a state transition. Call with s.mu held,
// then emitStateChange after releasing it.
func (s *Session) setStateLocked(st ConnState) {
	s.state = st
	s.stateSeq++
}

// emitStateChange publishes the state as a gps:state event. Emissions
// are serialized and each publishes the freshest state, so the last
// event consumers see always matches the final state even when
// transitions race; an intermediate state that was overtaken before
// its emission may be coalesced away. Must not be called with s.mu
// held (the Sink may call Session snapshot accessors).
func (s *Session) emitStateChange() {
	s.stateEmitMu.Lock()
	if s.stateEmitting {
		s.stateEmitMu.Unlock()
		return
	}
	s.stateEmitting = true
	for {
		s.mu.Lock()
		if s.stateSeq == s.stateEmitSeq {
			s.mu.Unlock()
			s.stateEmitting = false
			s.stateEmitMu.Unlock()
			return
		}
		s.stateEmitSeq = s.stateSeq
		st := s.state
		s.mu.Unlock()
		s.stateEmitMu.Unlock()
		s.emit(EventState, st)
		s.stateEmitMu.Lock()
	}
}

// setEndState transitions to target if the originating run is still
// current and connected, or to Disconnected if the connection was lost.
// Returns the state actually set.
// Used by send/config goroutines to avoid resurrecting state after
// disconnect. While the state is Connecting or Reconnecting or the
// run is ending, a (re)connect owns the state and the transition is
// skipped: an operation can only be ending in those states because a
// newer Connect cancelled it.
func (s *Session) setEndState(runCtx context.Context, target ConnState) ConnState {
	s.mu.Lock()
	st := s.state
	if s.runCtx != runCtx {
		s.mu.Unlock()
		return st
	}
	if st == StateDisconnected || st == StateConnecting || st == StateReconnecting {
		s.mu.Unlock()
		return st
	}
	if s.connCtx != nil && s.connCtx.Err() != nil {
		s.setStateLocked(StateDisconnected)
		s.mu.Unlock()
		s.emitStateChange()
		return StateDisconnected
	}
	if s.runCtx != nil && s.runCtx.Err() != nil {
		s.mu.Unlock()
		return st
	}
	s.setStateLocked(target)
	s.mu.Unlock()
	s.emitStateChange()
	return target
}

// stateErrLocked returns the error for an operation that requires
// StateConnected: a distinct message when an exclusive operation
// holds the port, "not connected" otherwise. Call with s.mu held.
func (s *Session) stateErrLocked() error {
	if s.state == StateConfiguring || s.state == StateSending {
		return errors.New("another operation is in progress")
	}
	return errors.New("not connected")
}

// probeDone transitions to Connected at the end of a (re)connect probe.
// It reports false, leaving the state alone, if the connection was
// closed meanwhile; the closer owns the state then.
func (s *Session) probeDone() bool {
	s.mu.Lock()
	if s.state == StateDisconnected || (s.connCtx != nil && s.connCtx.Err() != nil) {
		s.mu.Unlock()
		return false
	}
	s.setStateLocked(StateConnected)
	s.mu.Unlock()
	s.emitStateChange()
	return true
}

// cancelWorkerLocked cancels the response worker and invalidates its session
// so any in-flight gps:response events are dropped by the frontend.
// Must be called with s.mu held.
func (s *Session) cancelWorkerLocked() {
	if s.workerCancel != nil {
		s.workerCancel()
		s.workerCancel = nil
		s.respSession.Add(1)
	}
}

func (s *Session) closeLocked() {
	// Cancel and mark disconnected before stopCorrLocked releases s.mu
	// to wait, so no call can pass a connected-state check and start
	// work on the connection while it is being closed.
	if s.sendCancel != nil {
		s.sendCancel()
		s.sendCancel = nil
	}
	s.cancelWorkerLocked()
	if s.connCancel != nil {
		s.connCancel()
		s.connCancel = nil
	}
	s.setStateLocked(StateDisconnected)
	s.stopCorrLocked()
	if s.pb != nil {
		s.pb.Close()
		s.pb = nil
	}
	s.configCh = nil
	// Must unlock while waiting: goroutines may need the lock during shutdown.
	s.mu.Unlock()
	s.connWg.Wait()
	s.mu.Lock()
	s.mh = nil
	s.gga = nil
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

// endRun tears down the current pipeline run: it stops corrections and
// any in-flight send, cancels the run context, and closes the packet
// broadcast and the transport. Unlike closeLocked it does not wait for
// the connection goroutines -- it is called from the connection
// manager, which is itself one of them.
func (s *Session) endRun() {
	s.mu.Lock()
	// As in closeLocked, cancel the run before stopCorrLocked releases
	// s.mu to wait, so nothing new starts on the dying connection.
	if s.sendCancel != nil {
		s.sendCancel()
		s.sendCancel = nil
	}
	s.cancelWorkerLocked()
	if s.runCancel != nil {
		s.runCancel()
		s.runCancel = nil
	}
	s.stopCorrLocked()
	if s.pb != nil {
		s.pb.Close()
		s.pb = nil
	}
	s.configCh = nil
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.mu.Unlock()
}

// endConn transitions to Disconnected after the connection ends on its
// own (transport lost, probe failed, reconnect exhausted) rather than
// by Disconnect.
func (s *Session) endConn() {
	s.mu.Lock()
	if s.connCancel != nil {
		s.connCancel()
		s.connCancel = nil
	}
	if s.state != StateDisconnected {
		s.setStateLocked(StateDisconnected)
	}
	s.mu.Unlock()
	s.emitStateChange()
}

// enterReconnect transitions to Reconnecting, or reports false if the
// connection was closed meanwhile.
func (s *Session) enterReconnect() bool {
	s.mu.Lock()
	if s.state == StateDisconnected || s.connCtx == nil || s.connCtx.Err() != nil {
		s.mu.Unlock()
		return false
	}
	s.setStateLocked(StateReconnecting)
	s.mu.Unlock()
	s.emitStateChange()
	return true
}

// Opener opens (and re-opens) the connection to the receiver.
type Opener interface {
	// Open connects to the receiver. It returns the host serial port
	// speed, or 0 if the transport has none.
	Open(ctx context.Context) (conn gpsio.Conn, speed int, err error)
	// Socket reports a proxy connection: sets ConfigOptions.Socket
	// for gpscfg.Configure and gates reset operations in the UI.
	Socket() bool
}

// SerialOpener opens a local serial device.
type SerialOpener struct {
	Device string
	Speed  int // 0 means use the device's current speed
}

// deviceWaitInterval is how often Open retries a missing device node.
const deviceWaitInterval = 200 * time.Millisecond

// Open opens the serial device. If the device node does not exist, it
// waits for it to appear until ctx is done: after a USB reset the
// receiver re-enumerates and the node vanishes briefly (and Connect
// works when run just after plugging the device in). On ctx expiry the
// last open error is returned. The device is assumed to come back
// under the same node: a receiver that re-enumerates under a
// different name is not found (known limitation).
func (o SerialOpener) Open(ctx context.Context) (gpsio.Conn, int, error) {
	for {
		conn, speed, err := gpsio.OpenSerial(o.Device, o.Speed)
		if err == nil {
			return conn, speed, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, 0, err
		}
		select {
		case <-ctx.Done():
			return nil, 0, err
		case <-time.After(deviceWaitInterval):
		}
	}
}

// Socket reports that a serial device is not a proxy connection.
func (o SerialOpener) Socket() bool { return false }

// SocketOpener connects to a unix socket, typically the proxy.socket
// of a running satpulsed.
type SocketOpener struct {
	Path string
}

// Open connects to the unix socket.
func (o SocketOpener) Open(_ context.Context) (gpsio.Conn, int, error) {
	conn, err := gpsio.OpenSocket(o.Path)
	if err != nil {
		return nil, 0, err
	}
	return conn, 0, nil
}

// Socket reports that this is a proxy connection.
func (o SocketOpener) Socket() bool { return true }

// Connect opens a connection to a GPS receiver via op. It is
// asynchronous: it returns once the transport is open, and probe
// progress arrives as events. vendor narrows probing and packet format
// detection; VendorUnknown probes for all supported vendors. A Connect
// or Disconnect issued while shutdown or the open is pending supersedes
// it: a late-opened transport is closed and Connect returns an error.
func (s *Session) Connect(op Opener, vendor gpsreg.Vendor) error {
	return s.connect(s.reserveLifecycle(), op, vendor)
}

func (s *Session) reserveLifecycle() int {
	s.mu.Lock()
	s.connectGen++
	gen := s.connectGen
	s.mu.Unlock()
	return gen
}

func (s *Session) connect(gen int, op Opener, vendor gpsreg.Vendor) error {
	s.lifecycleMu.Lock()
	s.mu.Lock()
	if s.connectGen != gen {
		s.mu.Unlock()
		s.lifecycleMu.Unlock()
		return errors.New("connection attempt superseded")
	}
	s.closeLocked()
	if s.connectGen != gen {
		s.mu.Unlock()
		s.lifecycleMu.Unlock()
		return errors.New("connection attempt superseded")
	}
	s.setStateLocked(StateConnecting)
	s.mu.Unlock()
	s.lifecycleMu.Unlock()
	s.emitStateChange()
	ctx, cancel := context.WithTimeout(context.Background(), connectOpenTimeout)
	conn, speed, err := op.Open(ctx)
	cancel()
	s.lifecycleMu.Lock()
	s.mu.Lock()
	if s.connectGen != gen {
		s.mu.Unlock()
		s.lifecycleMu.Unlock()
		if err == nil {
			conn.Close()
		}
		return errors.New("connection attempt superseded")
	}
	if err != nil {
		s.setStateLocked(StateDisconnected)
		s.mu.Unlock()
		s.lifecycleMu.Unlock()
		s.emitStateChange()
		return err
	}
	connCtx, connCancel := context.WithCancel(context.Background())
	s.op = op
	s.vendor = vendor
	s.connCtx = connCtx
	s.connCancel = connCancel
	s.connWg.Go(func() { s.connManager(connCtx, conn, speed) })
	s.mu.Unlock()
	s.lifecycleMu.Unlock()
	return nil
}

// connVerdict says how a pipeline run ended.
type connVerdict int

const (
	verdictDisconnect  connVerdict = iota // Disconnect or a new Connect closed the connection
	verdictLost                           // transport died with no reset in flight
	verdictReconnect                      // transport died after a reset: re-open and re-probe
	verdictProbeFailed                    // the probe failed hard
)

// connManager owns the connection for its lifetime. It runs one packet
// pipeline at a time, reconnecting after a reset-bearing operation
// kills the transport (a reset over USB re-enumerates the device), and
// exits when the connection ends.
func (s *Session) connManager(connCtx context.Context, conn gpsio.Conn, speed int) {
	for {
		switch s.runConn(connCtx, conn, speed) {
		case verdictDisconnect:
			return
		case verdictReconnect:
			if !s.enterReconnect() {
				return
			}
			var err error
			conn, speed, err = s.reopen(connCtx)
			if err != nil {
				if connCtx.Err() == nil {
					s.lg.Warn("could not reconnect to the GPS", "err", err)
				}
				s.endConn()
				return
			}
		default: // verdictLost, verdictProbeFailed
			s.endConn()
			return
		}
	}
}

// reopen re-opens the transport after a reset-bearing operation killed
// it. Each attempt gets reopenTimeout for the device node to come back.
func (s *Session) reopen(connCtx context.Context) (gpsio.Conn, int, error) {
	var lastErr error
	for i := range maxReopenAttempts {
		if i > 0 {
			select {
			case <-connCtx.Done():
				return nil, 0, connCtx.Err()
			case <-time.After(reopenDelay):
			}
		}
		ctx, cancel := context.WithTimeout(connCtx, reopenTimeout)
		conn, speed, err := s.op.Open(ctx)
		cancel()
		if err == nil {
			return conn, speed, nil
		}
		lastErr = err
		if connCtx.Err() != nil {
			return nil, 0, connCtx.Err()
		}
	}
	return nil, 0, lastErr
}

// runConn wires the packet pipeline for an open transport, runs the
// packetWorker until the run ends, and tears the pipeline down.
func (s *Session) runConn(connCtx context.Context, conn gpsio.Conn, speed int) connVerdict {
	runCtx, runCancel := context.WithCancel(connCtx)
	portLock := gpsio.NewOutPortLock(conn)
	pCh := make(chan scan.Packet, 1)
	pktFormats := gpsreg.CreatePacketFormats(s.vendor)
	pLog, plCh := gpsio.NewPacketLog(pktFormats, packetLogChannelSize)
	if sc, ok := conn.(*gpsio.SerialConn); ok {
		sc.SetPacketLog(pLog)
	} else {
		// No output-side packet logging on this transport.
		pLog.SemiClose()
	}
	s.connWg.Go(func() { gpsio.Scan(runCtx, s.lg, conn, pCh, pLog, pktFormats) })
	pb := bcast.New(pCh)
	s.connWg.Go(func() { pb.Run(runCtx, s.lg) })
	s.connWg.Go(func() { s.packetLogWorker(plCh) })
	pktSub := pb.Subscribe()
	// Fresh packet processors each run: they are stateful and a
	// reconnect must not resume from the dead connection's state.
	procs := gpsreg.CreatePacketProcessors(s.vendor)
	configCh := make(chan configRequest)
	s.mu.Lock()
	s.conn = conn
	s.runCtx = runCtx
	s.runCancel = runCancel
	s.portLock = portLock
	s.pb = pb
	s.configCh = configCh
	s.speed = speed
	s.resetPending = false
	s.mu.Unlock()
	v := s.packetWorker(runCtx, conn, procs, pktSub, configCh, portLock)
	pb.Unsubscribe(pktSub)
	s.endRun()
	return v
}

// ReceiverEvent is emitted as the "gps:receiver" event payload.
type ReceiverEvent struct {
	OK            bool                          `json:"ok"`
	Error         string                        `json:"error,omitempty"`
	Warning       string                        `json:"warning,omitempty"`
	Info          opt.Val[gpsprot.ReceiverInfo] `json:",omitzero"`
	PacketFormats []string                      `json:"packetFormats,omitempty"`
}

// packetWorker is the single goroutine that owns packet processing.
// It runs an initial probe, then loops processing packets for message decoding.
// Configuration requests arrive via configCh and are executed inline.
func (s *Session) packetWorker(runCtx context.Context, conn gpsio.Conn, procs map[gpsprot.Tag]gpsprot.PacketProcessor, sub <-chan scan.Packet, configCh <-chan configRequest, portLock gpsio.OutPortLock) connVerdict {
	te := &timeEmitter{sink: s.sink}
	tt := gpsprot.NewTimeTicker(te, ptime.LeapSecond2016())
	mh := &msgHandler{sink: s.sink, tt: tt}
	// Synthesize a GGA each epoch into a selector, and monitor the selected
	// feed for the live Position display and the VRS NMEA-send gate. Always
	// on for the connection; the cost is one GGA per epoch into a cap-1 chan.
	sel := stream.NewGGASelector()
	synth := nmeasyn.New(sel)
	socket := s.op.Socket()
	s.mu.Lock()
	s.mh = mh
	s.probe = ReceiverEvent{}
	mon := newGGAMonitor(s.sink, runCtx, sel.Packets())
	s.gga = mon
	s.mu.Unlock()
	s.emit(EventReceiver, ReceiverEvent{})
	gpsprot.SetAllMsgHandlers(procs, gpsprot.NewMultiHandler(mh, synth))
	s.connWg.Go(mon.run)
	// Acquire portLock for probe.
	var port gpsio.OutPort
	select {
	case <-runCtx.Done():
		return verdictDisconnect
	case port = <-portLock:
	}
	// Run initial probe, matching the daemon's pattern of Configure before dispatch.
	target := gpsprot.NewConfigTarget()
	target.Opts.ForceProbe = true
	target.Opts.Socket = socket
	rslt, err := gpscfg.Configure(runCtx, s.lg, procs,
		gpsreg.CreateConfigProtocols(s.vendor), target, sub, conn)
	portLock <- port
	s.emitSpeed(conn)
	if err != nil && !errors.Is(err, gpscfg.ErrNoProbeResponse) && !errors.Is(err, gpscfg.ErrNotDetected) {
		if runCtx.Err() != nil {
			return verdictDisconnect
		}
		s.emit(EventReceiver, ReceiverEvent{Error: err.Error()})
		return verdictProbeFailed
	}
	r := ReceiverEvent{OK: true}
	if rslt != nil {
		if rslt.ReceiverInfo != nil {
			r.Info.Set(*rslt.ReceiverInfo)
		}
		for _, tag := range rslt.PacketFormatsDetected {
			r.PacketFormats = append(r.PacketFormats, string(tag))
		}
	}
	if err != nil {
		r.Warning = err.Error()
	}
	s.mu.Lock()
	s.probe = r
	s.mu.Unlock()
	s.emit(EventReceiver, r)
	if !s.probeDone() {
		return verdictDisconnect
	}
	// Main packet processing loop with inline config handling.
	for {
		select {
		case pkt, ok := <-sub:
			if !ok {
				return s.streamDied(runCtx)
			}
			sel.Packet(pkt)
			s.handleMsgPacket(procs, pkt)
		case req, ok := <-configCh:
			if !ok {
				return verdictDisconnect
			}
			// Acquire portLock to pause corrections during config.
			select {
			case <-runCtx.Done():
				req.resultCh <- configResult{err: runCtx.Err()}
				return verdictDisconnect
			case port = <-portLock:
			}
			// Remember a reset-bearing request until the run ends or the
			// next request: the transport dying is then expected, and the
			// manager reconnects instead of disconnecting.
			reset := req.target.Opts.Reset != gpsprot.ResetNone
			s.mu.Lock()
			s.resetPending = reset
			s.mu.Unlock()
			req.target.Opts.Socket = socket
			rslt, err := gpscfg.Configure(req.ctx, s.lg, procs,
				gpsreg.CreateConfigProtocols(s.vendor), req.target, sub, conn)
			portLock <- port
			s.emitSpeed(conn)
			if reset && errors.Is(err, io.ErrUnexpectedEOF) {
				// The reset killed the transport mid-run, as expected over
				// USB: report success and let the manager reconnect.
				err = nil
			}
			req.resultCh <- configResult{result: rslt, err: err}
		}
	}
}

// streamDied classifies the packet stream closing: a deliberate close,
// an expected transport loss from a reset (reconnect), or an unexpected
// loss such as an unplugged device (disconnect).
func (s *Session) streamDied(runCtx context.Context) connVerdict {
	if runCtx.Err() != nil {
		return verdictDisconnect
	}
	s.mu.Lock()
	reset := s.resetPending
	s.mu.Unlock()
	if reset {
		return verdictReconnect
	}
	return verdictLost
}

// Receiver returns the cached receiver identification result.
func (s *Session) Receiver() ReceiverEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.probe
}

// Disconnect closes the GPS connection: it cancels any in-flight
// receiver operation and correction session, waits for the connection
// goroutines to exit, and emits StateDisconnected. It supersedes a
// Connect whose open is still pending and is safe to call in any
// state.
func (s *Session) Disconnect() {
	s.disconnect(s.reserveLifecycle())
}

func (s *Session) disconnect(gen int) {
	s.lifecycleMu.Lock()
	s.mu.Lock()
	if s.connectGen != gen {
		s.mu.Unlock()
		s.lifecycleMu.Unlock()
		return
	}
	s.closeLocked()
	current := s.connectGen == gen
	if current {
		s.probe = ReceiverEvent{}
	}
	s.mu.Unlock()
	s.lifecycleMu.Unlock()
	if current {
		s.emitStateChange()
	}
}

// CorrEvent is the payload for "gps:corrections" events.
type CorrEvent struct {
	State      string `json:"state"`          // "connecting", "connected", "reconnecting", "failed", "stopped"
	Mode       string `json:"mode,omitempty"` // "tcp" or "ntrip"
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	Mountpoint string `json:"mountpoint,omitempty"` // ntrip only
	Error      string `json:"error,omitempty"`      // last error (set during reconnecting or failed)
}

func (s *Session) setCorrStateLocked(ev CorrEvent) {
	s.corrState = ev
}

func (s *Session) emitCorrState(ev CorrEvent) {
	s.mu.Lock()
	s.setCorrStateLocked(ev)
	s.mu.Unlock()
	s.emit(EventCorrections, ev)
}

// CorrectionsState returns the last known corrections state.
func (s *Session) CorrectionsState() CorrEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.corrState
}

// CorrectionSource is the configuration passed from the frontend to
// start a correction session.
type CorrectionSource struct {
	Mode       string `json:"mode"` // "tcp" or "ntrip"
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Mountpoint string `json:"mountpoint"` // ntrip only
	Username   string `json:"username"`   // ntrip only, may be empty
	Password   string `json:"password"`   // ntrip only, may be empty
	NMEASend   bool   `json:"nmeaSend"`   // ntrip only: upload position as NMEA GGA (VRS)
}

// BaseARPEvent is the payload for "gps:basearp" events, emitted when
// an RTCM 1005 or 1006 message is received with a base station ARP.
type BaseARPEvent struct {
	StationID uint16     `json:"stationID"`
	ECEF      [3]float64 `json:"ecef"` // meters
}

// emitCorrPacket converts a scanned correction packet into a
// gpsprot.CorReportMsg and emits it as a "gps:corrpacket" event. The
// parsed RTCM message is consumed here to emit a "gps:basearp" event
// for 1005/1006; it is not put on the wire (NativeMsg is cleared).
func (s *Session) emitCorrPacket(pkt scan.Packet) {
	msg, err := stream.CorReportFromPacket(pkt)
	if err != nil || msg == nil {
		return
	}
	if mt, ok := msg.NativeMsg.(*rtcmbin.MT1005); ok {
		s.emit(EventBaseARP, BaseARPEvent{
			StationID: mt.StationID,
			ECEF:      mt.ECEF(),
		})
	} else if mt, ok := msg.NativeMsg.(*rtcmbin.MT1006); ok {
		s.emit(EventBaseARP, BaseARPEvent{
			StationID: mt.StationID,
			ECEF:      mt.MT1005.ECEF(),
		})
	}
	msg.NativeMsg = nil
	s.emit(EventCorrPacket, msg)
}

// StartCorrections starts forwarding correction packets from the
// given source to the GPS receiver. It requires StateConnected and
// returns once forwarding is set up; connection progress, redials,
// and failures arrive as gps:corrections events. At most one
// correction session exists at a time: starting while one is running
// replaces it (the old session is stopped first). It returns an
// error for an invalid cfg, when not connected, when a stop is still
// draining, or (with cfg.NMEASend) when the receiver has no usable
// fix to upload.
func (s *Session) StartCorrections(cfg CorrectionSource) error {
	if cfg.Host == "" {
		return fmt.Errorf("host is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("invalid port")
	}
	if cfg.NMEASend && cfg.Mode != "ntrip" {
		return fmt.Errorf("NMEA send requires ntrip")
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	var source stream.Source
	switch cfg.Mode {
	case "tcp":
		source = &stream.TCPSource{Addr: addr}
	case "ntrip":
		source = &stream.NtripSource{
			Addr:       addr,
			Mountpoint: cfg.Mountpoint,
			Username:   cfg.Username,
			Password:   cfg.Password,
		}
	default:
		return fmt.Errorf("invalid source mode")
	}
	s.mu.Lock()
	if s.state != StateConnected || s.runCtx == nil || s.runCtx.Err() != nil {
		err := s.stateErrLocked()
		s.mu.Unlock()
		return err
	}
	if s.corrStopping {
		s.mu.Unlock()
		return fmt.Errorf("corrections stopping")
	}
	// Stop any existing correction session. stopCorrLocked releases
	// s.mu while it waits, so the connection may have been closed or
	// lost meanwhile; recheck before capturing it.
	s.stopCorrLocked()
	if s.state != StateConnected || s.runCtx == nil || s.runCtx.Err() != nil {
		err := s.stateErrLocked()
		s.mu.Unlock()
		return err
	}
	conn := s.conn
	portLock := s.portLock
	runCtx := s.runCtx
	// VRS: when sending position as NMEA, gate atomically on a usable
	// position. startSession primes the sender's feed with the current
	// selected GGA, so Pull uploads without delay (WaitReady returns at
	// once); a nil feed means no usable fix, so refuse rather than stall.
	var selectedGGA <-chan scan.Packet
	nmeaInterval := time.Duration(0)
	if cfg.NMEASend {
		if s.gga == nil {
			s.mu.Unlock()
			return fmt.Errorf("not connected")
		}
		selectedGGA = s.gga.startSession()
		if selectedGGA == nil {
			s.mu.Unlock()
			return fmt.Errorf("waiting for a position fix")
		}
		nmeaInterval = stream.DefaultNMEASendInterval
	}
	corrCtx, corrCancel := context.WithCancel(runCtx)
	s.corrCancel = corrCancel
	wg := &sync.WaitGroup{}
	s.corrWg = wg
	sink := stream.NewPull(source, s.lg, packetWriter(conn), portLock, gpsreg.CreateCorrectionFormats(), nmeaInterval)
	s.setCorrStateLocked(CorrEvent{
		State:      "connecting",
		Mode:       cfg.Mode,
		Host:       cfg.Host,
		Port:       cfg.Port,
		Mountpoint: cfg.Mountpoint,
	})
	var failed atomic.Bool
	onState := func(st stream.State, err error) {
		ev := CorrEvent{
			State:      st.String(),
			Mode:       cfg.Mode,
			Host:       cfg.Host,
			Port:       cfg.Port,
			Mountpoint: cfg.Mountpoint,
		}
		if err != nil {
			ev.Error = err.Error()
		}
		if st == stream.Failed {
			failed.Store(true)
		}
		s.emitCorrState(ev)
	}
	// Subscribe before starting goroutines to avoid missing packets.
	pktSub := sink.Packets.Subscribe()
	// Register all correction goroutines before releasing the lock,
	// so a concurrent stop will wait for them.
	wg.Go(func() {
		defer sink.Packets.Unsubscribe(pktSub)
		for pkt := range pktSub {
			if pkt.Format == nil {
				continue
			}
			s.emitCorrPacket(pkt)
		}
	})
	wg.Go(func() {
		sink.Run(corrCtx, selectedGGA, onState)
		if failed.Load() {
			return
		}
		s.emitCorrState(CorrEvent{
			State:      "stopped",
			Mode:       cfg.Mode,
			Host:       cfg.Host,
			Port:       cfg.Port,
			Mountpoint: cfg.Mountpoint,
		})
	})
	// Bridge into connWg using a captured wg, so the connWg goroutine
	// is independent of s.corrWg and won't conflict with a new session.
	s.connWg.Go(func() { wg.Wait() })
	s.mu.Unlock()
	return nil
}

// StopCorrections stops the correction forwarding and waits for the
// correction goroutines to exit; it is a no-op when none is running.
// It returns an error only when a stop is already in progress.
func (s *Session) StopCorrections() error {
	s.mu.Lock()
	if s.corrStopping {
		s.mu.Unlock()
		return fmt.Errorf("corrections stopping")
	}
	s.stopCorrLocked()
	s.mu.Unlock()
	return nil
}

// stopCorrLocked cancels the correction context and waits for the
// correction goroutines to exit. Must be called with s.mu held;
// temporarily releases s.mu while waiting.
func (s *Session) stopCorrLocked() {
	if s.gga != nil {
		s.gga.endSession()
	}
	if s.corrCancel == nil {
		s.setCorrStateLocked(CorrEvent{State: "stopped"})
		return
	}
	s.corrCancel()
	s.corrCancel = nil
	wg := s.corrWg
	s.corrWg = nil
	s.corrStopping = true
	s.mu.Unlock()
	wg.Wait()
	s.mu.Lock()
	s.corrStopping = false
}

// packetWriter adapts conn for stream.NewPull. SerialConn implements
// WritePacket itself, logging the write with its known format; other
// transports have no output packet log, so a plain Write suffices.
func packetWriter(conn gpsio.Conn) stream.PacketWriter {
	if pw, ok := conn.(stream.PacketWriter); ok {
		return pw
	}
	return plainWriter{conn}
}

type plainWriter struct{ gpsio.Conn }

func (w plainWriter) WritePacket(p []byte, _ gpsprot.PacketFormat) (int, error) {
	return w.Write(p)
}

// State returns the current connection state.
func (s *Session) State() ConnState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Speed returns the host serial port speed, or 0 if the transport has
// none.
func (s *Session) Speed() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.speed
}

// SignalCatalog returns the full signal catalog for the given GNSS constellations.
// If gs is zero, all constellations are returned.
func (s *Session) SignalCatalog(gs gpsprot.GNSSSet) map[string][]string {
	if gs == 0 {
		gs = gpsprot.SigSetAll.GNSSSet()
	}
	m := make(map[string][]string)
	for _, g := range gs.Items() {
		sigs := gpsprot.BandAll.SignalSet(g)
		if sigs == 0 {
			continue
		}
		var names []string
		for sig := range sigs.Signals() {
			if sig.GNSS() == g {
				names = append(names, sig.UnqualifiedName())
			}
		}
		if len(names) > 0 {
			m[g.String()] = names
		}
	}
	return m
}

// readProps lists the configuration properties to retrieve on readback.
const readProps = gpsprot.PropIDSignalsEnabled |
	gpsprot.PropIDMode |
	gpsprot.PropIDTimePulse |
	gpsprot.PropIDTimeGNSS |
	gpsprot.PropIDAntennaCableDelay |
	gpsprot.PropIDMinElevation |
	gpsprot.PropIDNavMsgAuth |
	gpsprot.PropIDBaudRate |
	gpsprot.PropIDPort

// ReadConfig reads back the current configuration from the receiver.
// It is an exclusive receiver operation: it requires StateConnected,
// holds the port as StateConfiguring until the run completes, and
// returns an error if the session is not connected or another
// operation is in progress.
func (s *Session) ReadConfig(ctx context.Context) (*gpsprot.ConfigProps, error) {
	s.mu.Lock()
	if s.state != StateConnected {
		err := s.stateErrLocked()
		s.mu.Unlock()
		return nil, err
	}
	s.cancelWorkerLocked()
	runCtx := s.runCtx
	s.setStateLocked(StateConfiguring)
	s.mu.Unlock()
	s.emitStateChange()
	target := gpsprot.NewConfigTarget()
	target.Get = readProps
	cr := s.sendConfigRequest(ctx, target)
	s.setEndState(runCtx, StateConnected)
	if cr.err != nil {
		return nil, cr.err
	}
	return cr.result.ConfigProps, nil
}

// ApplyConfig sends configuration changes to the receiver. It removes
// read-only properties from target.Props and sets target.Opts.Socket to
// match the transport. Like ReadConfig it is an exclusive receiver
// operation: it requires StateConnected, holds the
// port as StateConfiguring until the run completes, and returns an
// error if the session is not connected or another operation is in
// progress. Reset operations are refused over a proxy connection: a
// reset would kill the daemon that owns the port, and its restart
// would reapply the daemon's own configuration on top of the
// session's.
func (s *Session) ApplyConfig(ctx context.Context, target *gpsprot.ConfigTarget) error {
	target.Props.ClearReadOnlyProps()
	s.mu.Lock()
	if s.state != StateConnected {
		err := s.stateErrLocked()
		s.mu.Unlock()
		return err
	}
	if target.Opts.Reset != gpsprot.ResetNone && s.op.Socket() {
		s.mu.Unlock()
		return fmt.Errorf("reset operations are not available over a proxy connection")
	}
	s.cancelWorkerLocked()
	runCtx := s.runCtx
	s.setStateLocked(StateConfiguring)
	s.mu.Unlock()
	s.emitStateChange()
	if target.NoOp() {
		s.setEndState(runCtx, StateConnected)
		return fmt.Errorf("no configuration changes specified")
	}
	cr := s.sendConfigRequest(ctx, target)
	s.setEndState(runCtx, StateConnected)
	return cr.err
}

// DecodePacket decodes a packet and returns the decoded fields.
// It returns nil if the packet is not in any of the given formats.
func DecodePacket(formats []gpsprot.PacketFormat, data []byte, out bool) (*gpsdecode.DecodeResult, error) {
	_, r, err := gpsdecode.Decode(formats, data, out)
	if err != nil {
		return nil, nil
	}
	return r, nil
}

// MsgFileTag is a tag from a loaded message file.
type MsgFileTag struct {
	Tag       string `json:"tag"`
	Desc      string `json:"desc,omitempty"`
	MsgCount  int    `json:"msgCount"`
	NeedsPort bool   `json:"needsPort"` // ubxvalport: requires a --port
	SaveAware bool   `json:"saveAware"` // ubxval/ubxvalport: honors --save
}

// SetMsgFile sets the message file that SendMsgFile sends from and
// returns the available tags. The shell obtains the parsed file
// however it likes: native dialog, library path, upload.
func (s *Session) SetMsgFile(mf *msgfile.Parsed) []MsgFileTag {
	descs, _ := mf.TagDescs()
	tags := make([]MsgFileTag, len(descs))
	for i, td := range descs {
		needsPort, saveAware := msgTagCaps(mf, td.Tag)
		tags[i] = MsgFileTag{Tag: td.Tag, Desc: td.Desc, MsgCount: td.MsgCount,
			NeedsPort: needsPort, SaveAware: saveAware}
	}
	s.mu.Lock()
	s.msgFile = mf
	s.mu.Unlock()
	return tags
}

// msgTagCaps reports whether the messages for tag take a port argument
// (ubxvalport) and whether they honor the save flag (ubxval or
// ubxvalport). A single tag always maps to one message type, so a type
// switch on the filtered slice is sufficient.
func msgTagCaps(mf *msgfile.Parsed, tag string) (needsPort, saveAware bool) {
	msgs, err := mf.TaggedMsgs([]string{tag})
	if err != nil {
		return false, false
	}
	switch msgs.(type) {
	case []msgfile.UBXValPortMsg:
		return true, true
	case []msgfile.UBXValMsg:
		return false, true
	}
	return false, false
}

// MsgSendEvent is emitted as "gps:msgsend" during SendMsgFile.
type MsgSendEvent struct {
	Session int    `json:"session"`
	Status  string `json:"status"`
	Current int    `json:"current,omitempty"`
	Total   int    `json:"total,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ResponseEvent is emitted as "gps:response" during SendMsgFile.
type ResponseEvent struct {
	Session    int    `json:"session"`
	Kind       string `json:"kind"`               // "ack", "other", "maybe"
	ResponseTo int    `json:"responseTo"`         // 0-based index of sent message, or -1
	MsgCount   int    `json:"msgCount,omitempty"` // ack only: total messages sent for this tag
	AckError   string `json:"ackError,omitempty"` // ack only: empty = accepted, non-empty = rejected
	Tag        string `json:"tag,omitempty"`      // protocol tag
	MsgID      string `json:"msgID,omitempty"`    // message ID
	Text       string `json:"text,omitempty"`     // full text for text protocols
	Bin        string `json:"bin,omitempty"`      // hex string for binary protocols
}

// SendMsgFile sends messages for the given tag from the loaded
// message file (see SetMsgFile). It is an exclusive receiver
// operation: it requires StateConnected and a loaded file, holds the
// port as StateSending while the messages go out, and returns an
// error if the session is not connected or another operation is in
// progress. It returns once sending has started; progress and
// receiver responses arrive as gps:msgsend and gps:response events,
// and response listening continues briefly after the last message.
func (s *Session) SendMsgFile(tag string, port string, save bool) error {
	s.mu.Lock()
	if s.state != StateConnected || s.runCtx == nil || s.runCtx.Err() != nil {
		err := s.stateErrLocked()
		s.mu.Unlock()
		return err
	}
	mf := s.msgFile
	if mf == nil {
		s.mu.Unlock()
		return fmt.Errorf("no message file loaded")
	}
	conn := s.conn
	pb := s.pb
	runCtx := s.runCtx
	portLock := s.portLock
	if s.sendCancel != nil {
		s.sendCancel()
	}
	s.cancelWorkerLocked()
	s.mu.Unlock()
	msgs, err := mf.TaggedMsgs([]string{tag})
	if err != nil {
		return err
	}
	rawMsgs, err := msgfile.ToRaw(msgs, port, save)
	if err != nil {
		return err
	}
	if len(rawMsgs) == 0 {
		return fmt.Errorf("no messages for tag %q", tag)
	}
	sendCtx, sendCancel := context.WithCancel(context.Background())
	workerCtx, workerCancel := context.WithCancel(runCtx)
	s.mu.Lock()
	if s.state != StateConnected || runCtx.Err() != nil {
		err := s.stateErrLocked()
		s.mu.Unlock()
		sendCancel()
		workerCancel()
		return err
	}
	s.setStateLocked(StateSending)
	s.sendCancel = sendCancel
	s.workerCancel = workerCancel
	session := int(s.respSession.Add(1))
	s.mu.Unlock()
	s.emitStateChange()
	total := len(rawMsgs)
	stepCh := make(chan sendStepReq)
	// Start the worker goroutine.
	s.connWg.Go(func() {
		s.sendWorker(workerCtx, pb, conn, portLock, stepCh, session)
	})
	// Coordinator goroutine.
	s.connWg.Go(func() {
		defer func() {
			close(stepCh)
			s.finishSend(runCtx)
		}()
		s.emit(EventMsgSend, MsgSendEvent{
			Session: session, Status: "started", Total: total,
		})
		replyCh := make(chan error, 1)
		for i, rm := range rawMsgs {
			if sendCtx.Err() != nil {
				s.emit(EventMsgSend, MsgSendEvent{
					Session: session, Status: "cancelled", Current: i + 1, Total: total,
				})
				return
			}
			var next *msgfile.RawMsg
			if i+1 < len(rawMsgs) {
				next = &rawMsgs[i+1]
			}
			select {
			case stepCh <- sendStepReq{rm: rm, next: next, reply: replyCh}:
			case <-sendCtx.Done():
				s.emit(EventMsgSend, MsgSendEvent{
					Session: session, Status: "cancelled", Current: i + 1, Total: total,
				})
				return
			}
			var wErr error
			select {
			case wErr = <-replyCh:
			case <-sendCtx.Done():
				s.emit(EventMsgSend, MsgSendEvent{
					Session: session, Status: "cancelled", Current: i + 1, Total: total,
				})
				return
			}
			if wErr != nil {
				s.emit(EventMsgSend, MsgSendEvent{
					Session: session, Status: "error", Current: i + 1, Total: total, Error: wErr.Error(),
				})
				return
			}
			s.lg.Info("sent message", "index", i+1, "total", total, "tag", rm.Tag, "bytes", len(rm.Bytes))
			s.emit(EventMsgSend, MsgSendEvent{
				Session: session, Status: "sent", Current: i + 1, Total: total,
			})
		}
		s.emit(EventMsgSend, MsgSendEvent{
			Session: session, Status: "done", Current: total, Total: total,
		})
	})
	return nil
}

// finishSend clears sendCancel and transitions state to Connected (or
// Disconnected if the connection was lost).
func (s *Session) finishSend(runCtx context.Context) {
	s.mu.Lock()
	if s.runCtx == runCtx {
		s.sendCancel = nil
	}
	s.mu.Unlock()
	s.setEndState(runCtx, StateConnected)
}

// sendWorker owns conn.Write, Correlator, line buffer, and the broadcast
// subscriber. It runs until stepCh is closed and all expected responses
// have arrived (or a deadline expires), or workerCtx is cancelled.
func (s *Session) sendWorker(workerCtx context.Context, pb *bcast.Bcast[scan.Packet], conn gpsio.Conn, portLock gpsio.OutPortLock, stepCh <-chan sendStepReq, session int) {
	sub := pb.Subscribe()
	defer pb.Unsubscribe(sub)
	var port gpsio.OutPort
	select {
	case <-workerCtx.Done():
		return
	case port = <-portLock:
	}
	defer func() { portLock <- port }()
	cor := msgfile.NewCorrelator()
	var lineBuf []byte
	var deadline time.Time
	var tailCh <-chan time.Time
	for {
		select {
		case req, ok := <-stepCh:
			if !ok {
				// Coordinator is done sending. Enter tail phase.
				if !cor.CanAcceptMore() {
					return
				}
				remaining := time.Until(deadline)
				if remaining <= 0 {
					return
				}
				tailCh = time.After(remaining)
				stepCh = nil
				continue
			}
			// Write the message.
			_, err := conn.Write(req.rm.Bytes)
			if err != nil {
				req.reply <- err
				continue
			}
			cor.NotifyMsgSent(req.rm)
			// Update running deadline for tail phase.
			if d := time.Now().Add(req.rm.WaitLimit); d.After(deadline) {
				deadline = d
			}
			// Wait for delay + pacing, processing packets throughout.
			lineBuf = s.workerWaitStep(workerCtx, sub, cor, req, lineBuf, session, deadline)
		case pkt, ok := <-sub:
			if !ok {
				return
			}
			lineBuf = s.processWorkerPacket(cor, lineBuf, pkt, session)
			if tailCh != nil && !cor.CanAcceptMore() {
				return
			}
		case <-tailCh:
			return
		case <-workerCtx.Done():
			return
		}
	}
}

// workerWaitStep handles the delay and pacing wait after a successful write.
// It processes packets throughout, then replies to the coordinator.
func (s *Session) workerWaitStep(workerCtx context.Context, sub <-chan scan.Packet, cor *msgfile.Correlator, req sendStepReq, lineBuf []byte, session int, deadline time.Time) []byte {
	// Wait for delay.
	if req.rm.Delay > 0 {
		timer := time.NewTimer(req.rm.Delay)
		defer timer.Stop()
	delayLoop:
		for {
			select {
			case <-workerCtx.Done():
				req.reply <- workerCtx.Err()
				return lineBuf
			case <-timer.C:
				break delayLoop
			case pkt, ok := <-sub:
				if !ok {
					req.reply <- nil
					return lineBuf
				}
				lineBuf = s.processWorkerPacket(cor, lineBuf, pkt, session)
			}
		}
	}
	// Wait for pacing if there is a next message.
	if req.next != nil && !cor.ReadyToSend(*req.next) {
		remaining := time.Until(deadline)
		if remaining > 0 {
			timer := time.NewTimer(remaining)
			defer timer.Stop()
		paceLoop:
			for {
				select {
				case <-workerCtx.Done():
					req.reply <- workerCtx.Err()
					return lineBuf
				case <-timer.C:
					break paceLoop
				case pkt, ok := <-sub:
					if !ok {
						req.reply <- nil
						return lineBuf
					}
					lineBuf = s.processWorkerPacket(cor, lineBuf, pkt, session)
					if cor.ReadyToSend(*req.next) {
						break paceLoop
					}
				}
			}
		}
	}
	req.reply <- nil
	return lineBuf
}

// processWorkerPacket handles a single packet from the broadcast subscriber.
func (s *Session) processWorkerPacket(cor *msgfile.Correlator, lineBuf []byte, pkt scan.Packet, session int) []byte {
	if pkt.IsInterPacketTimeout() {
		return lineBuf
	}
	if pkt.Format == nil {
		return s.bufferWorkerLines(cor, lineBuf, []byte(pkt.Data), session)
	}
	lineBuf = s.flushWorkerLine(cor, lineBuf, session)
	result := cor.CorrelatePacket(pkt.Tag(), pkt.Data)
	s.emitCorrelation(result, &pkt, session)
	return lineBuf
}

func (s *Session) bufferWorkerLines(cor *msgfile.Correlator, lineBuf, data []byte, session int) []byte {
	for _, b := range data {
		if b == '\n' {
			lineBuf = s.flushWorkerLine(cor, lineBuf, session)
		} else if b == '\r' {
			// skip
		} else if b >= 0x20 && b <= 0x7E || b == '\t' {
			lineBuf = append(lineBuf, b)
		} else {
			lineBuf = lineBuf[:0]
		}
	}
	return lineBuf
}

func (s *Session) flushWorkerLine(cor *msgfile.Correlator, lineBuf []byte, session int) []byte {
	if len(lineBuf) == 0 {
		return lineBuf
	}
	line := string(lineBuf)
	lineBuf = lineBuf[:0]
	result := cor.CorrelatePacket(gpsprot.EmptyTag, line)
	if !s.sessionCurrent(session) {
		return lineBuf
	}
	if (result.Ack == msgfile.AckAck || result.Ack == msgfile.AckNak) && result.InResponseTo != nil {
		s.emit(EventResponse, s.makeAckEvent(result, session))
	}
	if result.Relevance >= msgfile.LevelMaybeResponse {
		ev := s.makePacketEvent(result, nil, session)
		ev.Text = line
		s.emit(EventResponse, ev)
	}
	return lineBuf
}

// emitCorrelation emits up to two ResponseEvents for a correlated packet:
// one for the ACK (if any) and one for the displayable payload (if any).
func (s *Session) emitCorrelation(r msgfile.Correlation, pkt *scan.Packet, session int) {
	if !s.sessionCurrent(session) {
		return
	}
	if (r.Ack == msgfile.AckAck || r.Ack == msgfile.AckNak) && r.InResponseTo != nil {
		s.emit(EventResponse, s.makeAckEvent(r, session))
	}
	if r.Relevance >= msgfile.LevelMaybeResponse {
		s.emit(EventResponse, s.makePacketEvent(r, pkt, session))
	}
}

// sessionCurrent returns true if the given session matches the current respSession.
func (s *Session) sessionCurrent(session int) bool {
	return int(s.respSession.Load()) == session
}

func (s *Session) makeAckEvent(r msgfile.Correlation, session int) ResponseEvent {
	ev := ResponseEvent{Session: session, Kind: "ack", ResponseTo: -1}
	if r.InResponseTo != nil {
		ev.ResponseTo = r.InResponseTo.Index
		ev.MsgCount = r.InResponseTo.Count
	}
	if r.Ack == msgfile.AckNak {
		ev.AckError = r.NakError
		if ev.AckError == "" {
			ev.AckError = "NAK"
		}
	}
	return ev
}

func (s *Session) makePacketEvent(r msgfile.Correlation, pkt *scan.Packet, session int) ResponseEvent {
	kind := "other"
	if r.Relevance == msgfile.LevelMaybeResponse {
		kind = "maybe"
	}
	ev := ResponseEvent{Session: session, Kind: kind, ResponseTo: -1}
	if pkt != nil && pkt.Format != nil {
		ev.Tag = string(pkt.Format.Tag())
		ev.MsgID = pkt.Format.MsgID([]byte(pkt.Data))
		b := []byte(pkt.Data)
		if b[0] < 0x20 || b[0] >= 0x7F {
			ev.Bin = hex.EncodeToString(b)
		} else {
			ev.Text = pkt.Data
		}
	}
	return ev
}

// CancelMsgSend cancels the in-progress SendMsgFile, interrupting any
// pacing delay or response wait immediately; the send reports
// "cancelled" as a gps:msgsend event. It returns an error if no send
// is in progress.
func (s *Session) CancelMsgSend() error {
	s.mu.Lock()
	if s.state != StateSending {
		s.mu.Unlock()
		return fmt.Errorf("not sending")
	}
	if s.sendCancel != nil {
		s.sendCancel()
	}
	s.cancelWorkerLocked()
	s.mu.Unlock()
	return nil
}

// sendConfigRequest sends a config request to packetWorker and waits for the result.
func (s *Session) sendConfigRequest(ctx context.Context, target *gpsprot.ConfigTarget) configResult {
	s.mu.Lock()
	configCh, runCtx := s.configCh, s.runCtx
	s.mu.Unlock()
	if configCh == nil {
		return configResult{err: fmt.Errorf("not connected")}
	}
	resultCh := make(chan configResult, 1)
	select {
	case configCh <- configRequest{ctx: ctx, target: target, resultCh: resultCh}:
	case <-ctx.Done():
		return configResult{err: ctx.Err()}
	case <-runCtx.Done():
		return configResult{err: runCtx.Err()}
	}
	select {
	case cr := <-resultCh:
		return cr
	case <-runCtx.Done():
		return configResult{err: runCtx.Err()}
	}
}

// emitSpeed reads the current host serial port speed and emits gps:speed.
// Called after any gpscfg.Configure since that is the only path that can
// change the host speed (via SerialConn.WriteThenChangeSpeed).
func (s *Session) emitSpeed(conn gpsio.Conn) {
	s.mu.Lock()
	if sc, ok := conn.(*gpsio.SerialConn); ok {
		s.speed = sc.Speed()
	}
	speed := s.speed
	s.mu.Unlock()
	s.emit(EventSpeed, speed)
}

// logHandler is an slog.Handler that emits "gps:log" events to the sink
// and forwards all records to a base handler.
type logHandler struct {
	sink  Sink
	base  slog.Handler
	attrs []slog.Attr
	group string
}

// NewLogHandler returns an slog.Handler that mirrors records to the
// sink as gps:log events and forwards them to base.
func NewLogHandler(sink Sink, base slog.Handler) slog.Handler {
	return &logHandler{sink: sink, base: base}
}

// LogEvent is emitted to the frontend for progress messages.
type LogEvent struct {
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Time      string         `json:"time"`
	Component string         `json:"component,omitempty"`
	Attrs     map[string]any `json:"attrs,omitempty"`
}

func (h *logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *logHandler) Handle(ctx context.Context, r slog.Record) error {
	ev := LogEvent{
		Level:     r.Level.String(),
		Message:   r.Message,
		Time:      r.Time.Format(time.TimeOnly),
		Component: h.group,
	}
	attrs := make(map[string]any)
	put := func(a slog.Attr) {
		v := a.Value.Resolve().Any()
		if e, ok := v.(error); ok {
			v = e.Error()
		}
		if _, err := json.Marshal(v); err != nil {
			v = fmt.Sprint(v)
		}
		attrs[a.Key] = v
	}
	for _, a := range h.attrs {
		put(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		put(a)
		return true
	})
	if len(attrs) > 0 {
		ev.Attrs = attrs
	}
	emit(h.sink, EventLog, ev)
	return h.base.Handle(ctx, r)
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logHandler{
		sink:  h.sink,
		base:  h.base.WithAttrs(attrs),
		attrs: append(h.attrs, attrs...),
		group: h.group,
	}
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	return &logHandler{
		sink:  h.sink,
		base:  h.base.WithGroup(name),
		attrs: h.attrs,
		group: name,
	}
}

// packetLogWorker writes packet log entries to the configured packet
// log and emits them as gps:packet events. The event is gated on
// Wants because the stream is high-rate: a web client only pays for
// packet streaming while it is displaying packets.
func (s *Session) packetLogWorker(ch <-chan gpsio.PacketLogEntry) {
	for entry := range ch {
		s.writePacketLogEntry(entry)
		if s.sink.Wants(EventPacket) {
			s.emit(EventPacket, entry)
		}
	}
}

func (s *Session) writePacketLogEntry(entry gpsio.PacketLogEntry) {
	bytes, err := json.Marshal(entry)
	if err != nil {
		s.lg.Warn("failed to convert packet to JSON", "err", err)
		return
	}
	bytes = append(bytes, '\n')
	s.packetLogMu.Lock()
	w := s.packetLogW
	if w == nil {
		s.packetLogMu.Unlock()
		return
	}
	_, err = w.Write(bytes)
	if err != nil {
		s.packetLogW = nil
	}
	s.packetLogMu.Unlock()
	if err != nil {
		s.lg.Warn("error writing to packet log, disabling it", "err", err)
	}
}

// MsgEvent is the envelope emitted as "gps:msg" to the frontend.
type MsgEvent struct {
	Kind string `json:"kind"`
	Msg  any    `json:"msg"`
	Time string `json:"time"`
}

// VelNEDtoECEF converts a velocity from NED (m/s) to ECEF using the last known position.
// Returns nil if no position is available.
func (s *Session) VelNEDtoECEF(n, e, d float64) *[3]float64 {
	s.mu.Lock()
	mh := s.mh
	s.mu.Unlock()
	if mh == nil {
		return nil
	}
	ref := mh.refLatLon.Load()
	if ref == nil {
		return nil
	}
	v := [3]float64(geopos.WGS84.NEDtoECEF(geopos.VelNED{n, e, d}, ref[0], ref[1]))
	return &v
}

// VelECEFtoNED converts a velocity from ECEF (m/s) to NED using the last known position.
// Returns nil if no position is available.
func (s *Session) VelECEFtoNED(vx, vy, vz float64) *[3]float64 {
	s.mu.Lock()
	mh := s.mh
	s.mu.Unlock()
	if mh == nil {
		return nil
	}
	ref := mh.refLatLon.Load()
	if ref == nil {
		return nil
	}
	v := [3]float64(geopos.WGS84.ECEFtoNED(geopos.VelECEF{vx, vy, vz}, ref[0], ref[1]))
	return &v
}

// timeEmitter is the downstream handler for TimeTicker.
// It emits a filled TimeMsg as the "gps:time" event at each epoch boundary.
type timeEmitter struct {
	gpsprot.DefaultHandler
	sink Sink
}

func (e *timeEmitter) Time(msg *gpsprot.TimeMsg, _ time.Time) {
	emit(e.sink, EventTime, msg)
}

// msgHandler implements gpsprot.MsgHandler and emits "gps:msg" events.
type msgHandler struct {
	gpsprot.PVMsgAccum
	tt        *gpsprot.TimeTicker
	sink      Sink
	refLatLon atomic.Pointer[[2]float64]
}

func (h *msgHandler) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	emit(h.sink, EventMsg, MsgEvent{
		Kind: "time",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.tt.Time(msg, tRead)
}

func (h *msgHandler) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	h.tt.LeapSecond(msg, tRead)
	emit(h.sink, EventMsg, MsgEvent{
		Kind: "leapSecond",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
}

func (h *msgHandler) Survey(msg *gpsprot.SurveyMsg, tRead time.Time) {
	emit(h.sink, EventMsg, MsgEvent{
		Kind: "survey",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
}

func (h *msgHandler) Satellites(msg *gpsprot.SatellitesMsg, tRead time.Time) {
	emit(h.sink, EventMsg, MsgEvent{
		Kind: "satellites",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
}

func (h *msgHandler) PosGeo(msg *gpsprot.PosGeoMsg, tRead time.Time) {
	ll := [2]float64{msg.LatLon[0].Degrees(), msg.LatLon[1].Degrees()}
	if h.refLatLon.Swap(&ll) == nil {
		emit(h.sink, EventInitialPos, ll)
	}
	emit(h.sink, EventMsg, MsgEvent{
		Kind: "posGeo",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.PVMsgAccum.PosGeo(msg, tRead)
}

func (h *msgHandler) PosECEF(msg *gpsprot.PosECEFMsg, tRead time.Time) {
	if !h.PVMsgBundle.PosGeo.IsSet() {
		ecef := geopos.ECEF{msg.Pos[0].Meters(), msg.Pos[1].Meters(), msg.Pos[2].Meters()}
		if llh, err := geopos.WGS84.ECEFtoLLH(ecef); err == nil {
			ll := [2]float64{llh.Lat, llh.Lon}
			if h.refLatLon.Swap(&ll) == nil {
				emit(h.sink, EventInitialPos, ll)
			}
		}
	}
	emit(h.sink, EventMsg, MsgEvent{
		Kind: "posECEF",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.PVMsgAccum.PosECEF(msg, tRead)
}

func (h *msgHandler) VelGeo(msg *gpsprot.VelGeoMsg, tRead time.Time) {
	emit(h.sink, EventMsg, MsgEvent{
		Kind: "velGeo",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.PVMsgAccum.VelGeo(msg, tRead)
}

func (h *msgHandler) VelECEF(msg *gpsprot.VelECEFMsg, tRead time.Time) {
	emit(h.sink, EventMsg, MsgEvent{
		Kind: "velECEF",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.PVMsgAccum.VelECEF(msg, tRead)
}

func (h *msgHandler) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	emit(h.sink, EventMsg, MsgEvent{
		Kind: "navEpoch",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.tt.NavEpoch(msg, tRead)
	h.PVMsgBundle.FillDerived()
	b := h.PVMsgBundle
	h.PVMsgAccum.NavEpoch(msg, tRead)
	if b.PosGeo.IsSet() || b.VelGeo.IsSet() {
		emit(h.sink, EventEpochPVT, b)
	}
}

func (s *Session) handleMsgPacket(procs map[gpsprot.Tag]gpsprot.PacketProcessor, pkt scan.Packet) {
	if pkt.IsInterPacketTimeout() {
		for _, pp := range procs {
			pp.Idle(pkt.TRead)
		}
		return
	}
	if pkt.Format == nil {
		return
	}
	tag := pkt.Format.Tag()
	pp := procs[tag]
	if pp == nil {
		return
	}
	if !pkt.ChecksumValid && !pkt.AltChecksumValid() {
		s.lg.Warn(pkt.ChecksumError().Error(), "tag", tag, "len", len(pkt.Data))
		return
	}
	msgID, err := pp.ProcessPacket(pkt.Data, pkt.TRead)
	if err != nil {
		s.lg.Warn("error processing packet", gpsio.PacketWarnAttrs(err, pkt, msgID)...)
	}
}

// ggaMonitor is the sole reader of a connection's selected-GGA feed (the
// GGASelector output). It tracks the latest usable GGA -- the exact packet
// the VRS GGASender would upload -- emits gps:nmeaPosition for the live
// Position display, and while an NMEA-send session is active forwards the
// feed into the channel handed to Pull.Run. It is the single owner of
// "ready": the Position display, the Connect gate, and the connect-without-
// delay guarantee are all this one fact read three times.
type ggaMonitor struct {
	sink    Sink
	ctx     context.Context // run ctx; run exits when it is done
	in      <-chan scan.Packet
	mu      sync.Mutex
	have    bool
	latest  scan.Packet      // latest usable GGA packet
	session chan scan.Packet // non-nil while a session is forwarding
}

func newGGAMonitor(sink Sink, ctx context.Context, in <-chan scan.Packet) *ggaMonitor {
	return &ggaMonitor{sink: sink, ctx: ctx, in: in}
}

// NMEAPositionEvent is the payload for "gps:nmeaPosition". Valid is false
// when the receiver currently has no usable fix (Position blank, Connect
// gated); otherwise Lat/Lon carry the live position.
type NMEAPositionEvent struct {
	Valid bool    `json:"valid"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
}

// run reads the selected-GGA feed until the connection ends.
func (m *ggaMonitor) run() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case pkt, ok := <-m.in:
			if !ok {
				return
			}
			m.handle(pkt)
		}
	}
}

func (m *ggaMonitor) handle(pkt scan.Packet) {
	pos, ok := stream.GGAPacketPosition(pkt)
	m.mu.Lock()
	if ok {
		m.have = true
		m.latest = pkt
		if m.session != nil {
			publishGGA(m.session, pkt)
		}
		m.mu.Unlock()
		emit(m.sink, EventNMEAPosition, NMEAPositionEvent{Valid: true, Lat: pos[0], Lon: pos[1]})
		return
	}
	// Unusable GGA: the receiver is reporting no fix. Drop the held position
	// so the Connect gate and Position display track the current fix. A
	// running session is unaffected -- GGASender keeps resending its own last
	// GGA. Emit the cleared event only on the have->not transition.
	wasHave := m.have
	m.have = false
	m.latest = scan.Packet{}
	m.mu.Unlock()
	if wasHave {
		emit(m.sink, EventNMEAPosition, NMEAPositionEvent{Valid: false})
	}
}

// startSession registers a primed selected-GGA channel for Pull.Run, or
// returns nil if no usable GGA is currently held (the caller must refuse).
// The channel is seeded with the latest usable GGA so the sender uploads
// at once, then receives each subsequent usable GGA. Combines the readiness
// check and the feed registration so StartCorrections gates atomically.
func (m *ggaMonitor) startSession() <-chan scan.Packet {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.have {
		return nil
	}
	ch := make(chan scan.Packet, 1)
	ch <- m.latest
	m.session = ch
	return ch
}

// endSession stops forwarding to a channel registered by startSession.
func (m *ggaMonitor) endSession() {
	m.mu.Lock()
	m.session = nil
	m.mu.Unlock()
}

// publishGGA sends pkt on a capacity-1 channel, dropping a stale unread value
// first -- mirroring GGASelector.publish so a slow sender never blocks the
// monitor.
func publishGGA(ch chan scan.Packet, pkt scan.Packet) {
	select {
	case ch <- pkt:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- pkt:
	default:
	}
}
