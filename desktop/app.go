package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jclark/satpulse/desktop/serialenum"
	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsdecode"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/msgfile"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/scan"
)

// ConnState represents the unified connection/activity state.
type ConnState string

const (
	StateDisconnected ConnState = "disconnected"
	StateConnecting   ConnState = "connecting"
	StateConnected    ConnState = "connected"
	StateConfiguring  ConnState = "configuring"
	StateSending      ConnState = "sending"
)

// configRequest is sent to packetWorker to run gpscfg.Configure.
type configRequest struct {
	target   *gpsprot.ConfigTarget
	resultCh chan<- configResult
}

// configResult is the response from a configRequest.
type configResult struct {
	result *gpscfg.Result
	err    error
}

// writeReq is sent from the coordinator to the worker for each message.
type writeReq struct {
	rm    msgfile.RawMsg
	reply chan<- error
}

// App is the Wails application struct.
// Its exported methods are bound to the frontend.
type App struct {
	ctx          context.Context
	lg           *slog.Logger
	mu           sync.Mutex
	state        ConnState
	conn         gpsio.Conn
	connCtx      context.Context
	connCancel   context.CancelFunc
	connWg       sync.WaitGroup
	pb           *bcast.Bcast[scan.Packet]
	configCh     chan configRequest
	probe        ReceiverEvent
	mh           *msgHandler
	msgFile      *msgfile.Parsed
	msgFilePath  string
	sendCancel   context.CancelFunc
	workerCancel context.CancelFunc
	respSession  atomic.Int32
}

// NewApp creates a new App.
func NewApp() *App {
	return &App{state: StateDisconnected}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	base := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	a.lg = slog.New(&eventHandler{ctx: ctx, base: base})
}

func (a *App) shutdown(_ context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closeLocked()
}

// setState transitions the connection state and emits a gps:state event.
// Must NOT be called with a.mu held -- Wails event dispatch could re-enter App methods.
func (a *App) setState(s ConnState) {
	a.mu.Lock()
	a.state = s
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:state", s)
}

// setEndState transitions to target if still connected, or to Disconnected
// if the connection was lost. Returns the state actually set.
// Used by send/config goroutines to avoid resurrecting state after disconnect.
func (a *App) setEndState(target ConnState) ConnState {
	a.mu.Lock()
	if a.state == StateDisconnected {
		a.mu.Unlock()
		return StateDisconnected
	}
	if a.connCtx != nil && a.connCtx.Err() != nil {
		a.state = StateDisconnected
		a.mu.Unlock()
		runtime.EventsEmit(a.ctx, "gps:state", StateDisconnected)
		return StateDisconnected
	}
	a.state = target
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:state", target)
	return target
}


// cancelWorkerLocked cancels the response worker and invalidates its session
// so any in-flight gps:response events are dropped by the frontend.
// Must be called with a.mu held.
func (a *App) cancelWorkerLocked() {
	if a.workerCancel != nil {
		a.workerCancel()
		a.workerCancel = nil
		a.respSession.Add(1)
	}
}

func (a *App) closeLocked() {
	if a.sendCancel != nil {
		a.sendCancel()
		a.sendCancel = nil
	}
	a.cancelWorkerLocked()
	if a.connCancel != nil {
		a.connCancel()
		a.connCancel = nil
	}
	if a.pb != nil {
		a.pb.Close()
		a.pb = nil
	}
	a.configCh = nil
	a.state = StateDisconnected
	// Must unlock while waiting: goroutines may need the lock during shutdown.
	a.mu.Unlock()
	a.connWg.Wait()
	a.mu.Lock()
	a.mh = nil
	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
}

// Result is the standard return type for frontend-bound methods.
type Result struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Connect opens a serial connection to a GPS receiver.
func (a *App) Connect(device string, speed int) Result {
	a.mu.Lock()
	a.closeLocked()
	a.state = StateConnecting
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:state", StateConnecting)
	conn, err := gpsio.OpenSerial(device, speed)
	if err != nil {
		a.setState(StateDisconnected)
		return Result{Error: err.Error()}
	}
	connCtx, connCancel := context.WithCancel(a.ctx)
	pCh := make(chan scan.Packet, 1)
	pLog, plCh := gpsio.NewPacketLog(gpsreg.PacketFormats)
	conn.SetPacketLog(pLog)
	a.connWg.Go(func() { gpsio.Scan(connCtx, a.lg, conn, pCh, pLog, gpsreg.PacketFormats) })
	pb := bcast.New(pCh)
	a.connWg.Go(func() { pb.Run(connCtx, a.lg) })
	a.connWg.Go(func() { a.packetLogWorker(plCh) })
	pktSub := pb.Subscribe()
	procs := gpsreg.CreatePacketProcessors(gpsreg.VendorUnknown)
	configCh := make(chan configRequest)
	a.mu.Lock()
	a.conn = conn
	a.connCtx = connCtx
	a.connCancel = connCancel
	a.pb = pb
	a.configCh = configCh
	a.mu.Unlock()
	a.connWg.Go(func() { a.packetWorker(procs, pktSub, configCh) })
	return Result{OK: true}
}

// ReceiverEvent is emitted as the "gps:receiver" event payload.
type ReceiverEvent struct {
	OK            bool                              `json:"ok"`
	Error         string                            `json:"error,omitempty"`
	Warning       string                            `json:"warning,omitempty"`
	Info          opt.Val[gpsprot.ReceiverInfo]      `json:",omitzero"`
	PacketFormats []string                           `json:"packetFormats,omitempty"`
}

// packetWorker is the single goroutine that owns packet processing.
// It runs an initial probe, then loops processing packets for message decoding.
// Configuration requests arrive via configCh and are executed inline.
func (a *App) packetWorker(procs map[gpsprot.Tag]gpsprot.PacketProcessor, sub <-chan scan.Packet, configCh <-chan configRequest) {
	te := &timeEmitter{ctx: a.ctx}
	tt := gpsprot.NewTimeTicker(te, ptime.LeapSecond2016())
	mh := &msgHandler{ctx: a.ctx, tt: tt}
	a.mu.Lock()
	a.mh = mh
	a.probe = ReceiverEvent{}
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:receiver", ReceiverEvent{})
	gpsprot.SetAllMsgHandlers(procs, mh)
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return
	}
	// Run initial probe, matching the daemon's pattern of Configure before dispatch.
	target := gpsprot.NewConfigTarget()
	target.Opts.ForceProbe = true
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	rslt, err := gpscfg.Configure(ctx, a.lg, procs,
		gpsreg.CreateConfigProtocols(), target, sub, conn)
	cancel()
	if err != nil && !errors.Is(err, gpscfg.ErrNoProbeResponse) && !errors.Is(err, gpscfg.ErrNotDetected) {
		runtime.EventsEmit(a.ctx, "gps:receiver", ReceiverEvent{Error: err.Error()})
		a.setEndState(StateDisconnected)
		return
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
	a.mu.Lock()
	a.probe = r
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:receiver", r)
	a.setEndState(StateConnected)
	// Main packet processing loop with inline config handling.
	for {
		select {
		case pkt, ok := <-sub:
			if !ok {
				return
			}
			a.handleMsgPacket(procs, pkt)
		case req, ok := <-configCh:
			if !ok {
				return
			}
			a.mu.Lock()
			conn = a.conn
			a.mu.Unlock()
			ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
			rslt, err := gpscfg.Configure(ctx, a.lg, procs,
				gpsreg.CreateConfigProtocols(), req.target, sub, conn)
			cancel()
			req.resultCh <- configResult{result: rslt, err: err}
		}
	}
}

// GetReceiverState returns the cached receiver identification result.
func (a *App) GetReceiverState() ReceiverEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.probe
}

// Disconnect closes the GPS connection.
func (a *App) Disconnect() Result {
	a.mu.Lock()
	a.closeLocked()
	a.probe = ReceiverEvent{}
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:state", StateDisconnected)
	return Result{OK: true}
}

// IsConnected returns true if a GPS connection is open.
func (a *App) IsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state != StateDisconnected
}

// GetConnState returns the current connection state.
func (a *App) GetConnState() ConnState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// GetAllSignals returns the full signal catalog for the given GNSS constellations.
// If gs is zero, all constellations are returned.
func (a *App) GetAllSignals(gs gpsprot.GNSSSet) map[string][]string {
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
				names = append(names, sig.String())
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
	gpsprot.PropIDNavMsgAuth

// ReadConfig reads back the current configuration from the receiver.
func (a *App) ReadConfig() (*gpsprot.ConfigProps, error) {
	a.mu.Lock()
	if a.state != StateConnected {
		a.mu.Unlock()
		return nil, fmt.Errorf("not connected")
	}
	a.cancelWorkerLocked()
	a.state = StateConfiguring
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:state", StateConfiguring)
	target := gpsprot.NewConfigTarget()
	target.Get = readProps
	cr := a.sendConfigRequest(target)
	a.setEndState(StateConnected)
	if cr.err != nil {
		return nil, cr.err
	}
	return cr.result.ConfigProps, nil
}

// ApplyConfig sends configuration changes to the receiver.
// The frontend sends JSON matching gpsprot.ConfigTarget's shape.
func (a *App) ApplyConfig(cfg gpsprot.ConfigTarget) Result {
	a.mu.Lock()
	if a.state != StateConnected {
		a.mu.Unlock()
		return Result{Error: "not connected"}
	}
	a.cancelWorkerLocked()
	a.state = StateConfiguring
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:state", StateConfiguring)
	if cfg.NoOp() {
		a.setEndState(StateConnected)
		return Result{Error: "no configuration changes specified"}
	}
	r := a.runConfig(&cfg)
	a.setEndState(StateConnected)
	return r
}



// DecodeOptions controls how DecodePacket interprets its input.
type DecodeOptions struct {
	Hex bool `json:"hex"` // data is hex-encoded binary
	Out bool `json:"out"` // packet is outbound (sent to receiver)
}

// DecodePacket decodes a packet and returns the decoded fields.
func (a *App) DecodePacket(data string, opts DecodeOptions) (*gpsdecode.DecodeResult, error) {
	var b []byte
	if opts.Hex {
		var err error
		b, err = hex.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("invalid hex: %w", err)
		}
	} else {
		b = []byte(data)
	}
	_, r, err := gpsdecode.Decode(gpsreg.PacketFormats, b, opts.Out)
	if err != nil {
		return nil, nil
	}
	return r, nil
}

// MsgFileTag is a tag from a loaded message file.
type MsgFileTag struct {
	Tag      string `json:"tag"`
	Desc     string `json:"desc,omitempty"`
	MsgCount int    `json:"msgCount"`
}

// MsgFileInfo is the result of loading a message file.
type MsgFileInfo struct {
	Path string       `json:"path"`
	Tags []MsgFileTag `json:"tags"`
}

// LoadMsgFile opens a file dialog, loads the selected TOML message file,
// and returns the available tags. Returns nil if the user cancels the dialog.
func (a *App) LoadMsgFile() (*MsgFileInfo, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Open message file",
		Filters: []runtime.FileFilter{
			{DisplayName: "TOML files", Pattern: "*.toml"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	mf, err := msgfile.Load(path)
	if err != nil {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Error loading message file",
			Message: err.Error(),
		})
		return nil, err
	}
	descs, _ := mf.TagDescs()
	tags := make([]MsgFileTag, len(descs))
	for i, td := range descs {
		tags[i] = MsgFileTag{Tag: td.Tag, Desc: td.Desc, MsgCount: td.MsgCount}
	}
	a.mu.Lock()
	a.msgFile = mf
	a.msgFilePath = path
	a.mu.Unlock()
	return &MsgFileInfo{Path: filepath.Base(path), Tags: tags}, nil
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
	Kind       string `json:"kind"`              // "ack", "other", "maybe"
	ResponseTo int    `json:"responseTo"`         // 0-based index of sent message, or -1
	MsgCount   int    `json:"msgCount,omitempty"` // ack only: total messages sent for this tag
	AckError   string `json:"ackError,omitempty"` // ack only: empty = accepted, non-empty = rejected
	Tag        string `json:"tag,omitempty"`      // protocol tag
	MsgID      string `json:"msgID,omitempty"`    // message ID
	Text       string `json:"text,omitempty"`     // full text for text protocols
	Bin        string `json:"bin,omitempty"`      // hex string for binary protocols
}

// SendMsgFile sends messages for the given tag from the loaded message file.
// Returns immediately after starting the coordinator and worker goroutines.
func (a *App) SendMsgFile(tag string) error {
	a.mu.Lock()
	if a.state != StateConnected {
		a.mu.Unlock()
		return fmt.Errorf("not connected")
	}
	mf := a.msgFile
	if mf == nil {
		a.mu.Unlock()
		return fmt.Errorf("no message file loaded")
	}
	conn := a.conn
	pb := a.pb
	connCtx := a.connCtx
	if a.sendCancel != nil {
		a.sendCancel()
	}
	a.cancelWorkerLocked()
	a.mu.Unlock()
	msgs, err := mf.TaggedMsgs([]string{tag})
	if err != nil {
		return err
	}
	rawMsgs, err := msgfile.ToRaw(msgs)
	if err != nil {
		return err
	}
	if len(rawMsgs) == 0 {
		return fmt.Errorf("no messages for tag %q", tag)
	}
	sendCtx, sendCancel := context.WithCancel(a.ctx)
	workerCtx, workerCancel := context.WithCancel(connCtx)
	a.mu.Lock()
	if a.state != StateConnected {
		a.mu.Unlock()
		sendCancel()
		workerCancel()
		return fmt.Errorf("not connected")
	}
	a.state = StateSending
	a.sendCancel = sendCancel
	a.workerCancel = workerCancel
	session := int(a.respSession.Add(1))
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:state", StateSending)
	total := len(rawMsgs)
	writeReqCh := make(chan writeReq)
	// Start the worker goroutine.
	a.connWg.Go(func() {
		a.sendWorker(workerCtx, pb, conn, writeReqCh, session)
	})
	// Coordinator goroutine.
	a.connWg.Go(func() {
		defer func() {
			close(writeReqCh)
			a.finishSend()
		}()
		runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
			Session: session, Status: "started", Total: total,
		})
		replyCh := make(chan error, 1)
		for i, rm := range rawMsgs {
			if sendCtx.Err() != nil {
				runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
					Session: session, Status: "cancelled", Current: i + 1, Total: total,
				})
				return
			}
			select {
			case writeReqCh <- writeReq{rm: rm, reply: replyCh}:
			case <-sendCtx.Done():
				runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
					Session: session, Status: "cancelled", Current: i + 1, Total: total,
				})
				return
			}
			var wErr error
			select {
			case wErr = <-replyCh:
			case <-sendCtx.Done():
				runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
					Session: session, Status: "cancelled", Current: i + 1, Total: total,
				})
				return
			}
			if wErr != nil {
				runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
					Session: session, Status: "error", Current: i + 1, Total: total, Error: wErr.Error(),
				})
				return
			}
			a.lg.Info("sent message", "index", i+1, "total", total, "tag", rm.Tag, "bytes", len(rm.Bytes))
			runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
				Session: session, Status: "sent", Current: i + 1, Total: total,
			})
			if rm.Delay > 0 {
				runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
					Session: session, Status: "delaying", Current: i + 1, Total: total,
				})
				select {
				case <-time.After(rm.Delay):
				case <-sendCtx.Done():
					runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
						Session: session, Status: "cancelled", Current: i + 1, Total: total,
					})
					return
				}
				runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
					Session: session, Status: "delayed", Current: i + 1, Total: total,
				})
			}
		}
		runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
			Session: session, Status: "done", Current: total, Total: total,
		})
	})
	return nil
}

// finishSend clears sendCancel and transitions state to Connected (or Disconnected).
func (a *App) finishSend() {
	a.mu.Lock()
	a.sendCancel = nil
	s := StateConnected
	if a.state == StateDisconnected || (a.connCtx != nil && a.connCtx.Err() != nil) {
		s = StateDisconnected
	}
	a.state = s
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:state", s)
}

// sendWorker owns conn.Write, PacketAnalyzer, line buffer, and the broadcast
// subscriber. It runs until writeReqCh is closed and a tail timer expires,
// or workerCtx is cancelled.
func (a *App) sendWorker(workerCtx context.Context, pb *bcast.Bcast[scan.Packet], conn gpsio.Conn, writeReqCh <-chan writeReq, session int) {
	sub := pb.Subscribe()
	defer pb.Unsubscribe(sub)
	pa := msgfile.NewPacketAnalyzer()
	var lineBuf []byte
	var tailCh <-chan time.Time
	for {
		select {
		case req, ok := <-writeReqCh:
			if !ok {
				tailCh = time.After(3 * time.Second)
				writeReqCh = nil
				continue
			}
			_, err := conn.Write(req.rm.Bytes)
			if err == nil {
				pa.NotifySent(req.rm)
			}
			req.reply <- err
		case pkt, ok := <-sub:
			if !ok {
				return
			}
			if pkt.IsInterPacketTimeout() {
				continue
			}
			if pkt.Format == nil {
				lineBuf = a.bufferWorkerLines(pa, lineBuf, []byte(pkt.Data), session)
				continue
			}
			lineBuf = a.flushWorkerLine(pa, lineBuf, session)
			result := pa.Analyze(pkt.Tag(), pkt.Data)
			if result.Kind != msgfile.NotResponse {
				a.emitResponse(result, &pkt, session)
			}
		case <-tailCh:
			return
		case <-workerCtx.Done():
			return
		}
	}
}

func (a *App) bufferWorkerLines(pa *msgfile.PacketAnalyzer, lineBuf, data []byte, session int) []byte {
	for _, b := range data {
		if b == '\n' {
			lineBuf = a.flushWorkerLine(pa, lineBuf, session)
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

func (a *App) flushWorkerLine(pa *msgfile.PacketAnalyzer, lineBuf []byte, session int) []byte {
	if len(lineBuf) == 0 {
		return lineBuf
	}
	line := string(lineBuf)
	lineBuf = lineBuf[:0]
	result := pa.Analyze(gpsprot.EmptyTag, line)
	if result.Kind != msgfile.NotResponse && a.sessionCurrent(session) {
		ev := a.makeResponseEvent(result, nil, session)
		ev.Text = line
		runtime.EventsEmit(a.ctx, "gps:response", ev)
	}
	return lineBuf
}

func (a *App) emitResponse(r msgfile.PacketAnalysis, pkt *scan.Packet, session int) {
	if !a.sessionCurrent(session) {
		return
	}
	ev := a.makeResponseEvent(r, pkt, session)
	runtime.EventsEmit(a.ctx, "gps:response", ev)
}

// sessionCurrent returns true if the given session matches the current respSession.
func (a *App) sessionCurrent(session int) bool {
	return int(a.respSession.Load()) == session
}

func (a *App) makeResponseEvent(r msgfile.PacketAnalysis, pkt *scan.Packet, session int) ResponseEvent {
	ev := ResponseEvent{Session: session, ResponseTo: -1}
	switch r.Kind {
	case msgfile.AckResponse:
		ev.Kind = "ack"
		ev.AckError = r.AckError
		if r.RelatedMsg != nil {
			ev.ResponseTo = r.RelatedMsg.Index
			ev.MsgCount = r.RelatedMsg.Count
		}
	case msgfile.OtherResponse:
		ev.Kind = "other"
	case msgfile.MaybeResponse:
		ev.Kind = "maybe"
	}
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

// CancelMsgSend cancels an in-progress send operation.
func (a *App) CancelMsgSend() error {
	a.mu.Lock()
	if a.state != StateSending {
		a.mu.Unlock()
		return fmt.Errorf("not sending")
	}
	if a.sendCancel != nil {
		a.sendCancel()
	}
	a.mu.Unlock()
	return nil
}

// sendConfigRequest sends a config request to packetWorker and waits for the result.
func (a *App) sendConfigRequest(target *gpsprot.ConfigTarget) configResult {
	a.mu.Lock()
	configCh, connCtx := a.configCh, a.connCtx
	a.mu.Unlock()
	if configCh == nil {
		return configResult{err: fmt.Errorf("not connected")}
	}
	resultCh := make(chan configResult, 1)
	select {
	case configCh <- configRequest{target: target, resultCh: resultCh}:
	case <-connCtx.Done():
		return configResult{err: connCtx.Err()}
	}
	select {
	case cr := <-resultCh:
		return cr
	case <-connCtx.Done():
		return configResult{err: connCtx.Err()}
	}
}

// runConfig sends a configuration to the connected receiver.
func (a *App) runConfig(target *gpsprot.ConfigTarget) Result {
	cr := a.sendConfigRequest(target)
	if cr.err != nil {
		return Result{Error: cr.err.Error()}
	}
	return Result{OK: true}
}

// eventHandler is an slog.Handler that emits "gps:log" events to the frontend
// and forwards all messages to a base handler (stderr).
type eventHandler struct {
	ctx   context.Context
	base  slog.Handler
	attrs []slog.Attr
	group string
}

// LogEvent is emitted to the frontend for progress messages.
type LogEvent struct {
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Time      string         `json:"time"`
	Component string         `json:"component,omitempty"`
	Attrs     map[string]any `json:"attrs,omitempty"`
}

func (h *eventHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.base.Enabled(context.Background(), level)
}

func (h *eventHandler) Handle(ctx context.Context, r slog.Record) error {
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
	runtime.EventsEmit(h.ctx, "gps:log", ev)
	return h.base.Handle(ctx, r)
}

func (h *eventHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &eventHandler{
		ctx:   h.ctx,
		base:  h.base.WithAttrs(attrs),
		attrs: append(h.attrs, attrs...),
		group: h.group,
	}
}

func (h *eventHandler) WithGroup(name string) slog.Handler {
	return &eventHandler{
		ctx:   h.ctx,
		base:  h.base.WithGroup(name),
		attrs: h.attrs,
		group: name,
	}
}

func (a *App) packetLogWorker(ch <-chan gpsio.PacketLogEntry) {
	for entry := range ch {
		runtime.EventsEmit(a.ctx, "gps:packet", entry)
	}
}

// ListPorts enumerates serial ports available on the system.
func (a *App) ListPorts() ([]serialenum.Port, error) {
	return serialenum.List()
}

// MsgEvent is the envelope emitted as "gps:msg" to the frontend.
type MsgEvent struct {
	Kind string `json:"kind"`
	Msg  any    `json:"msg"`
	Time string `json:"time"`
}

// LLH holds latitude, longitude (degrees) and height (meters) above the WGS84 ellipsoid.
type LLH struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Height float64 `json:"height"`
}

// CheckOnEarth returns true if the ECEF coordinates are roughly on Earth's surface.
func (a *App) CheckOnEarth(x, y, z float64) bool {
	return geopos.ECEF{x, y, z}.CheckOnEarth() == nil
}

// ECEFtoLLH converts Earth-Centered Earth-Fixed coordinates (meters) to latitude,
// longitude (degrees), and height above the WGS84 ellipsoid (meters).
func (a *App) ECEFtoLLH(x, y, z float64) (*LLH, error) {
	llh, err := geopos.WGS84.ECEFtoLLH(geopos.ECEF{x, y, z})
	if err != nil {
		return nil, err
	}
	return &LLH{Lat: llh.Lat, Lon: llh.Lon, Height: llh.Height}, nil
}

// LLHtoECEF converts latitude, longitude (degrees) and height (meters) to
// Earth-Centered Earth-Fixed coordinates (meters).
func (a *App) LLHtoECEF(lat, lon, height float64) [3]float64 {
	ecef := geopos.WGS84.LLHtoECEF(geopos.LLH{Lat: lat, Lon: lon, Height: height})
	return [3]float64(ecef)
}

// VelNEDtoECEF converts a velocity from NED (m/s) to ECEF using the last known position.
// Returns nil if no position is available.
func (a *App) VelNEDtoECEF(n, e, d float64) *[3]float64 {
	a.mu.Lock()
	mh := a.mh
	a.mu.Unlock()
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
func (a *App) VelECEFtoNED(vx, vy, vz float64) *[3]float64 {
	a.mu.Lock()
	mh := a.mh
	a.mu.Unlock()
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
	ctx context.Context
}

func (e *timeEmitter) Time(msg *gpsprot.TimeMsg, _ time.Time) {
	runtime.EventsEmit(e.ctx, "gps:time", msg)
}

// msgHandler implements gpsprot.MsgHandler and emits "gps:msg" events.
type msgHandler struct {
	gpsprot.PVMsgAccum
	tt        *gpsprot.TimeTicker
	ctx       context.Context
	refLatLon atomic.Pointer[[2]float64]
}

func (h *msgHandler) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "time",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.tt.Time(msg, tRead)
}

func (h *msgHandler) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	h.tt.LeapSecond(msg, tRead)
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "leapSecond",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
}

func (h *msgHandler) Survey(msg *gpsprot.SurveyMsg, tRead time.Time) {
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "survey",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
}

func (h *msgHandler) Satellites(msg *gpsprot.SatellitesMsg, tRead time.Time) {
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "satellites",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
}

func (h *msgHandler) PosGeo(msg *gpsprot.PosGeoMsg, tRead time.Time) {
	ll := [2]float64{msg.LatLon[0].Degrees(), msg.LatLon[1].Degrees()}
	if h.refLatLon.Swap(&ll) == nil {
		runtime.EventsEmit(h.ctx, "gps:initialPos", ll)
	}
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
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
				runtime.EventsEmit(h.ctx, "gps:initialPos", ll)
			}
		}
	}
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "posECEF",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.PVMsgAccum.PosECEF(msg, tRead)
}

func (h *msgHandler) VelGeo(msg *gpsprot.VelGeoMsg, tRead time.Time) {
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "velGeo",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.PVMsgAccum.VelGeo(msg, tRead)
}

func (h *msgHandler) VelECEF(msg *gpsprot.VelECEFMsg, tRead time.Time) {
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "velECEF",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.PVMsgAccum.VelECEF(msg, tRead)
}

func (h *msgHandler) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "navEpoch",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	h.tt.NavEpoch(msg, tRead)
	h.PVMsgBundle.FillDerived()
	b := h.PVMsgBundle
	h.PVMsgAccum.NavEpoch(msg, tRead)
	if b.PosGeo.IsSet() || b.VelGeo.IsSet() {
		runtime.EventsEmit(h.ctx, "gps:epochPVT", b)
	}
}

func (a *App) handleMsgPacket(procs map[gpsprot.Tag]gpsprot.PacketProcessor, pkt scan.Packet) {
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
	err := pkt.ChecksumError()
	if err != nil {
		a.lg.Warn(err.Error(), "tag", tag, "len", len(pkt.Data))
	}
	_, err = pp.ProcessPacket(pkt.Data, pkt.TRead)
	if err != nil {
		a.lg.Error("error processing packet", "err", err, "tag", tag, "data", pkt.Data)
	}
}
