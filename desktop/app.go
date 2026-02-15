package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
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

// App is the Wails application struct.
// Its exported methods are bound to the frontend.
type App struct {
	ctx        context.Context
	lg         *slog.Logger
	mu         sync.Mutex
	state      ConnState
	conn       gpsio.Conn
	connCtx    context.Context
	connCancel context.CancelFunc
	connWg     sync.WaitGroup
	pb         *bcast.Bcast[scan.Packet]
	configCh   chan configRequest
	probe      ReceiverEvent
	mh          *msgHandler
	msgFile     *msgfile.Parsed
	msgFilePath string
	sendCancel  context.CancelFunc
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

func (a *App) closeLocked() {
	if a.sendCancel != nil {
		a.sendCancel()
		a.sendCancel = nil
	}
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
	a.connWg.Go(func() { gpsio.Scan(connCtx, a.lg, conn, pCh, nil) })
	pb := bcast.New(pCh)
	a.connWg.Go(func() { pb.Run(connCtx, a.lg) })
	sub := pb.Subscribe()
	a.connWg.Go(func() { a.packetEventWorker(sub) })
	pktSub := pb.Subscribe()
	procs := gpsreg.CreatePacketProcessors(nil)
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
	mh := &msgHandler{
		ctx: a.ctx,
		ls:  ptime.LeapSecond2016(),
	}
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
func (a *App) GetAllSignals(gnss []string) map[string][]string {
	m := make(map[string][]string)
	for _, name := range gnss {
		g, err := gpsprot.ParseGNSS(name)
		if err != nil {
			continue
		}
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
			m[name] = names
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
	Status  string `json:"status"`
	Current int    `json:"current,omitempty"`
	Total   int    `json:"total,omitempty"`
	Error   string `json:"error,omitempty"`
}

// SendMsgFile sends messages for the given tag from the loaded message file.
// Returns immediately after starting the send goroutine.
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
	ctx, cancel := context.WithCancel(a.ctx)
	a.mu.Lock()
	if a.state != StateConnected {
		a.mu.Unlock()
		cancel()
		return fmt.Errorf("not connected")
	}
	a.state = StateSending
	a.sendCancel = cancel
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gps:state", StateSending)
	total := len(rawMsgs)
	a.connWg.Go(func() {
		defer func() {
			a.mu.Lock()
			a.sendCancel = nil
			a.mu.Unlock()
		}()
		for i, rm := range rawMsgs {
			if ctx.Err() != nil {
				runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
					Status: "cancelled", Current: i + 1, Total: total,
				})
				a.setEndState(StateConnected)
				return
			}
			_, err := conn.Write(rm.Bytes)
			if err != nil {
				runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
					Status: "error", Current: i + 1, Total: total, Error: err.Error(),
				})
				a.setEndState(StateConnected)
				return
			}
			a.lg.Info("sent message", "index", i+1, "total", total, "tag", rm.Tag, "bytes", len(rm.Bytes))
			runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
				Status: "sent", Current: i + 1, Total: total,
			})
			if rm.Delay > 0 {
				runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
					Status: "delaying", Current: i + 1, Total: total,
				})
				select {
				case <-time.After(rm.Delay):
				case <-ctx.Done():
					runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
						Status: "cancelled", Current: i + 1, Total: total,
					})
					a.setEndState(StateConnected)
					return
				}
				runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
					Status: "delayed", Current: i + 1, Total: total,
				})
			}
		}
		runtime.EventsEmit(a.ctx, "gps:msgsend", MsgSendEvent{
			Status: "done", Current: total, Total: total,
		})
		a.setEndState(StateConnected)
	})
	return nil
}

// CancelMsgSend cancels an in-progress send operation.
func (a *App) CancelMsgSend() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != StateSending {
		return fmt.Errorf("not sending")
	}
	if a.sendCancel != nil {
		a.sendCancel()
	}
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
		v := a.Value.Any()
		if e, ok := v.(error); ok {
			v = e.Error()
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

// PacketEvent is emitted to the frontend for each received GPS packet.
type PacketEvent struct {
	Tag       string `json:"tag"`
	Msg       string `json:"msg,omitempty"`
	Bin       string `json:"bin,omitempty"`
	Ascii     string `json:"ascii,omitempty"`
	Timestamp string `json:"timestamp"`
}

func (a *App) packetEventWorker(sub <-chan scan.Packet) {
	for pkt := range sub {
		if len(pkt.Data) == 0 {
			continue
		}
		b := []byte(pkt.Data)
		ev := PacketEvent{
			Tag:       string(pkt.Tag()),
			Timestamp: time.Now().Format(time.TimeOnly),
		}
		if pkt.Format != nil {
			ev.Msg = pkt.Format.MsgID(b)
		}
		if useBinary(pkt.Format, b) {
			ev.Bin = fmt.Sprintf("%x", b)
		} else {
			ev.Ascii = pkt.Data
		}
		runtime.EventsEmit(a.ctx, "gps:packet", ev)
	}
}

func useBinary(pf gpsprot.PacketFormat, data []byte) bool {
	if pf == nil {
		return containsBinary(data)
	}
	return data[0] < 0x20 || data[0] >= 0x7F
}

func containsBinary(data []byte) bool {
	for _, b := range data {
		if (b < 0x20 && b != '\t' && b != '\r' && b != '\n') || b >= 0x7F {
			return true
		}
	}
	return false
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

// MsgBundle holds the most recent message of each type within a navigation epoch.
// Cleared after each NavEpochMsg.
type MsgBundle struct {
	PosGeo  *gpsprot.PosGeoMsg
	PosECEF *gpsprot.PosECEFMsg
	VelGeo  *gpsprot.VelGeoMsg
	VelECEF *gpsprot.VelECEFMsg
	Time    *gpsprot.TimeMsg
}

// EpochPVT is emitted as "gps:epochPVT" at the end of each navigation epoch.
type EpochPVT struct {
	Pos  *EpochPos  `json:"pos,omitempty"`
	Vel  *EpochVel  `json:"vel,omitempty"`
	Time *EpochTime `json:"time,omitempty"`
}

// EpochPos carries position in both geodetic and ECEF frames.
type EpochPos struct {
	Lat       float64    `json:"lat"`                  // degrees
	Lon       float64    `json:"lon"`                  // degrees
	Height    *float64   `json:"height,omitempty"`     // meters above WGS84
	HeightMSL *float64   `json:"heightMSL,omitempty"`  // meters above MSL
	ECEF      *[3]float64 `json:"ecef,omitempty"`      // meters
}

// EpochVel carries velocity fields from the current epoch.
type EpochVel struct {
	GroundSpeed *float64    `json:"groundSpeed,omitempty"` // m/s
	Speed3D     *float64    `json:"speed3D,omitempty"`     // m/s
	Course      *float64    `json:"course,omitempty"`      // degrees, true north
	VelNED      *[3]float64 `json:"velNED,omitempty"`      // m/s [N,E,D]
	VelECEF     *[3]float64 `json:"velECEF,omitempty"`     // m/s [X,Y,Z]
}

// EpochTime carries time representations for the current epoch.
type EpochTime struct {
	TAI       string  `json:"tai,omitempty"`       // TAI seconds.nanoseconds
	UTC       string  `json:"utc,omitempty"`       // ISO 8601
	Accuracy  *int64  `json:"accuracy,omitempty"`  // nanoseconds
	UTCOffset *uint8  `json:"utcOffset,omitempty"` // leap seconds
	GNSS      string  `json:"gnss,omitempty"`
}

// msgHandler implements gpsprot.MsgHandler and emits "gps:msg" events.
type msgHandler struct {
	gpsprot.DefaultHandler
	ctx       context.Context
	ls        ptime.LeapSecond
	cur       MsgBundle
	refLatLon atomic.Pointer[[2]float64]
}

func (h *msgHandler) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	h.cur.Time = msg
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "time",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
}

func (h *msgHandler) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	msg.UpdateLeapSecond(&h.ls)
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
	h.cur.PosGeo = msg
	ll := [2]float64{msg.LatLon[0].Degrees(), msg.LatLon[1].Degrees()}
	if h.refLatLon.Swap(&ll) == nil {
		runtime.EventsEmit(h.ctx, "gps:initialPos", ll)
	}
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "posGeo",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
}

func (h *msgHandler) PosECEF(msg *gpsprot.PosECEFMsg, tRead time.Time) {
	h.cur.PosECEF = msg
	if h.cur.PosGeo == nil {
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
}

func (h *msgHandler) VelGeo(msg *gpsprot.VelGeoMsg, tRead time.Time) {
	h.cur.VelGeo = msg
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "velGeo",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
}

func (h *msgHandler) VelECEF(msg *gpsprot.VelECEFMsg, tRead time.Time) {
	h.cur.VelECEF = msg
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "velECEF",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
}

func (h *msgHandler) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	runtime.EventsEmit(h.ctx, "gps:msg", MsgEvent{
		Kind: "navEpoch",
		Msg:  msg,
		Time: tRead.Format(time.TimeOnly),
	})
	ev := h.buildEpochPVT()
	h.cur = MsgBundle{}
	if ev.Pos != nil || ev.Vel != nil || ev.Time != nil {
		runtime.EventsEmit(h.ctx, "gps:epochPVT", ev)
	}
}

func (h *msgHandler) buildEpochPVT() EpochPVT {
	var ev EpochPVT
	// Position: prefer PosGeo (native lat/lon)
	if g := h.cur.PosGeo; g != nil {
		p := &EpochPos{
			Lat: g.LatLon[0].Degrees(),
			Lon: g.LatLon[1].Degrees(),
		}
		if g.Height.IsSet() {
			v := g.Height.Get().Meters()
			p.Height = &v
		}
		if g.HeightMSL.IsSet() {
			v := g.HeightMSL.Get().Meters()
			p.HeightMSL = &v
		}
		// ECEF from native PosECEF or computed from LLH (requires height)
		if e := h.cur.PosECEF; e != nil {
			ecef := [3]float64{e.Pos[0].Meters(), e.Pos[1].Meters(), e.Pos[2].Meters()}
			p.ECEF = &ecef
		} else if p.Height != nil {
			ecef := [3]float64(geopos.WGS84.LLHtoECEF(geopos.LLH{
				Lat: p.Lat, Lon: p.Lon, Height: *p.Height,
			}))
			p.ECEF = &ecef
		}
		ev.Pos = p
	} else if e := h.cur.PosECEF; e != nil {
		ecef := [3]float64{e.Pos[0].Meters(), e.Pos[1].Meters(), e.Pos[2].Meters()}
		llh, err := geopos.WGS84.ECEFtoLLH(geopos.ECEF(ecef))
		if err == nil {
			ev.Pos = &EpochPos{
				Lat:    llh.Lat,
				Lon:    llh.Lon,
				Height: &llh.Height,
				ECEF:   &ecef,
			}
		}
	}
	// Velocity: copy native fields, convert units
	var vel *EpochVel
	if g := h.cur.VelGeo; g != nil {
		vel = &EpochVel{}
		if g.GroundSpeed.IsSet() {
			v := g.GroundSpeed.Get().MetersPerSecond()
			vel.GroundSpeed = &v
		}
		if g.Speed3D.IsSet() {
			v := g.Speed3D.Get().MetersPerSecond()
			vel.Speed3D = &v
		} else if g.VelNED.IsSet() {
			ned := g.VelNED.Get()
			n, e, d := ned[0].MetersPerSecond(), ned[1].MetersPerSecond(), ned[2].MetersPerSecond()
			v := math.Sqrt(n*n + e*e + d*d)
			vel.Speed3D = &v
		}
		if g.Course.IsSet() {
			v := g.Course.Get().Degrees()
			vel.Course = &v
		}
		if g.VelNED.IsSet() {
			ned := g.VelNED.Get()
			v := [3]float64{ned[0].MetersPerSecond(), ned[1].MetersPerSecond(), ned[2].MetersPerSecond()}
			vel.VelNED = &v
		}
	}
	if e := h.cur.VelECEF; e != nil {
		if vel == nil {
			vel = &EpochVel{}
		}
		v := [3]float64{e.Vel[0].MetersPerSecond(), e.Vel[1].MetersPerSecond(), e.Vel[2].MetersPerSecond()}
		vel.VelECEF = &v
	}
	// Cross-convert velocity frames when position is available
	if vel != nil && ev.Pos != nil {
		lat, lon := ev.Pos.Lat, ev.Pos.Lon
		if vel.VelNED != nil && vel.VelECEF == nil {
			v := [3]float64(geopos.WGS84.NEDtoECEF(geopos.VelNED(*vel.VelNED), lat, lon))
			vel.VelECEF = &v
		} else if vel.VelECEF != nil && vel.VelNED == nil {
			v := [3]float64(geopos.WGS84.ECEFtoNED(geopos.VelECEF(*vel.VelECEF), lat, lon))
			vel.VelNED = &v
		}
	}
	ev.Vel = vel
	// Time
	if t := h.cur.Time; t != nil {
		et := &EpochTime{}
		tai, ok := t.ComputeTAITime(h.ls)
		if ok {
			et.TAI = tai.String()
			// Derive UTC: TAI minus leap seconds
			utcNanos := int64(tai) - int64(h.ls.UTCOffAfter)*1e9
			utcTime := time.Unix(0, utcNanos).UTC()
			et.UTC = utcTime.Format(time.RFC3339Nano)
		} else if t.UTCTime != nil {
			et.UTC = t.UTCTime.Date.Add(t.UTCTime.TimeOfDay).UTC().Format(time.RFC3339Nano)
		}
		if t.Accuracy > 0 {
			v := int64(t.Accuracy)
			et.Accuracy = &v
		}
		if t.UTCOffset > 0 {
			v := t.UTCOffset
			et.UTCOffset = &v
		}
		if t.GNSS != 0 {
			et.GNSS = t.GNSS.String()
		}
		if et.TAI != "" || et.UTC != "" {
			ev.Time = et
		}
	}
	return ev
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
