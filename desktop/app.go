package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/scan"
)

// App is the Wails application struct.
// Its exported methods are bound to the frontend.
type App struct {
	ctx        context.Context
	lg         *slog.Logger
	mu         sync.Mutex
	conn       gpsio.Conn
	connCancel context.CancelFunc
	connWg     sync.WaitGroup
	pb         *bcast.Bcast[scan.Packet]
}

// NewApp creates a new App.
func NewApp() *App {
	return &App{}
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

func (a *App) closeLocked() {
	if a.connCancel != nil {
		a.connCancel()
		a.connCancel = nil
	}
	if a.pb != nil {
		a.pb.Close()
		a.pb = nil
	}
	// Must unlock while waiting: goroutines may need the lock during shutdown.
	a.mu.Unlock()
	a.connWg.Wait()
	a.mu.Lock()
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
	defer a.mu.Unlock()
	a.closeLocked()
	conn, err := gpsio.OpenSerial(device, speed)
	if err != nil {
		return Result{Error: err.Error()}
	}
	connCtx, connCancel := context.WithCancel(a.ctx)
	pCh := make(chan scan.Packet, 1)
	a.connWg.Go(func() { gpsio.Scan(connCtx, a.lg, conn, pCh, nil) })
	pb := bcast.New(pCh)
	a.connWg.Go(func() { pb.Run(connCtx, a.lg) })
	sub := pb.Subscribe()
	a.connWg.Go(func() { a.packetEventWorker(sub) })
	msgSub := pb.Subscribe()
	a.connWg.Go(func() { a.msgDecoderWorker(msgSub) })
	a.conn = conn
	a.connCancel = connCancel
	a.pb = pb
	return Result{OK: true}
}

// Disconnect closes the GPS connection.
func (a *App) Disconnect() Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closeLocked()
	return Result{OK: true}
}

// IsConnected returns true if a GPS connection is open.
func (a *App) IsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.conn != nil
}

// SignalInfo describes one signal within a GNSS constellation.
type SignalInfo struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
}

// GNSSInfo describes a GNSS constellation and its signals.
type GNSSInfo struct {
	Name    string       `json:"name"`
	Signals []SignalInfo `json:"signals"`
}

// GetAllSignals returns the full signal catalog grouped by GNSS constellation.
func (a *App) GetAllSignals() []GNSSInfo {
	var result []GNSSInfo
	for g := gpsprot.GNSS(1); g <= gpsprot.GNSSLast; g++ {
		allSigs := gpsprot.BandAll.SignalSet(g)
		if allSigs == 0 {
			continue
		}
		gi := GNSSInfo{Name: g.String()}
		for sig := range allSigs.Signals() {
			gi.Signals = append(gi.Signals, SignalInfo{
				Index: int(sig),
				Name:  sig.String(),
			})
		}
		result = append(result, gi)
	}
	return result
}

// ReceiverInfo holds detected GPS receiver information and current configuration.
type ReceiverInfo struct {
	OK            bool              `json:"ok"`
	Error         string            `json:"error,omitempty"`
	Vendor        string            `json:"vendor,omitempty"`
	Hardware      string            `json:"hardware,omitempty"`
	Firmware      string            `json:"firmware,omitempty"`
	SupportedGNSS []string          `json:"supportedGNSS,omitempty"`
	PacketFormats []string          `json:"packetFormats,omitempty"`
	Config        map[string]any    `json:"config,omitempty"`
	Signals       [][]string        `json:"signals,omitempty"`
	SignalIndices []int             `json:"signalIndices,omitempty"`
}

// getProps lists the configuration properties to retrieve on detect.
const getProps = gpsprot.PropIDSignalsEnabled |
	gpsprot.PropIDMode |
	gpsprot.PropIDTimePulse |
	gpsprot.PropIDTimeGNSS |
	gpsprot.PropIDAntennaCableDelay |
	gpsprot.PropIDMinElevation |
	gpsprot.PropIDNavMsgAuth

// DetectReceiver probes the GPS receiver and returns its identity and current configuration.
func (a *App) DetectReceiver() ReceiverInfo {
	a.mu.Lock()
	conn, pb := a.conn, a.pb
	a.mu.Unlock()
	if conn == nil {
		return ReceiverInfo{Error: "not connected"}
	}
	target := gpsprot.NewConfigTarget()
	target.Get = getProps
	target.Opts.ForceProbe = true
	sub := pb.Subscribe()
	defer pb.Unsubscribe(sub)
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	rslt, err := gpscfg.Configure(ctx, a.lg,
		gpsreg.CreatePacketProcessors(nil),
		gpsreg.CreateConfigProtocols(),
		target, sub, conn)
	if err != nil {
		return ReceiverInfo{Error: err.Error()}
	}
	r := ReceiverInfo{OK: true}
	if rslt.ReceiverInfo != nil {
		ri := rslt.ReceiverInfo
		r.Vendor = ri.Vendor
		r.Hardware = ri.Hardware
		r.Firmware = ri.Firmware
		for _, g := range ri.SupportedGNSS.Items() {
			r.SupportedGNSS = append(r.SupportedGNSS, g.String())
		}
	}
	for _, tag := range rslt.PacketFormatsDetected {
		r.PacketFormats = append(r.PacketFormats, string(tag))
	}
	if rslt.ConfigProps != nil {
		r.Config = configPropsToMap(rslt.ConfigProps)
		if sigs, ok := rslt.ConfigProps.GetSignalsEnabled(); ok {
			r.Signals = sigs.GNSSStringGroups()
			for sig := range sigs.Signals() {
				r.SignalIndices = append(r.SignalIndices, int(sig))
			}
		}
	}
	return r
}

// configPropsToMap extracts config properties into a flat map for display.
func configPropsToMap(cp *gpsprot.ConfigProps) map[string]any {
	m := make(map[string]any)
	if sigs, ok := cp.GetSignalsEnabled(); ok {
		m["signalsEnabled"] = sigs.String()
	}
	if mode, ok := cp.GetMode(); ok {
		if mode.Static {
			m["mode"] = "static"
		} else {
			m["mode"] = "mobile"
		}
	}
	if tp, ok := cp.GetTimePulse(); ok {
		tpm := map[string]any{
			"width":          float64(tp.Width) / float64(time.Second),
			"period":         float64(tp.Period) / float64(time.Second),
			"alignToGNSS":    tp.AlignToGNSS,
			"onlyWhenLocked": tp.OnlyWhenLocked,
			"polarityRising": tp.PolarityRising,
		}
		m["timePulse"] = tpm
		m["ppsEnabled"] = tp.Width > 0
	}
	if tg, ok := cp.GetTimeGNSS(); ok {
		m["timeGNSS"] = tg.String()
	}
	if cd, ok := cp.GetAntennaCableDelay(); ok {
		m["antennaCableDelay"] = float64(cd) / float64(time.Nanosecond)
	}
	if nma, ok := cp.GetNavMsgAuth(); ok {
		switch nma {
		case gpsprot.NavMsgAuthNone:
			m["navMsgAuth"] = "none"
		case gpsprot.NavMsgAuthOSNMA:
			m["navMsgAuth"] = "OSNMA"
		}
	}
	if el, ok := cp.GetMinElevation(); ok {
		m["minElevation"] = el.Degrees()
	}
	return m
}

// ConfigUpdate holds all configuration changes to apply at once.
type ConfigUpdate struct {
	SignalIndices    []int    `json:"signalIndices"`
	SetSignals      bool     `json:"setSignals"`
	Mode            string   `json:"mode"`
	SetMode         bool     `json:"setMode"`
	PPSWidth        float64  `json:"ppsWidth"`
	PPSPeriod       float64  `json:"ppsPeriod"`
	PPSAlignToGNSS  bool     `json:"ppsAlignToGNSS"`
	PPSOnlyLocked   bool     `json:"ppsOnlyLocked"`
	PPSRising       bool     `json:"ppsRising"`
	SetPPS          bool     `json:"setPPS"`
	TimeGNSS        string   `json:"timeGNSS"`
	SetTimeGNSS     bool     `json:"setTimeGNSS"`
	CableDelay      float64  `json:"cableDelay"`
	SetCableDelay   bool     `json:"setCableDelay"`
	MinElevation    float64  `json:"minElevation"`
	SetMinElevation bool     `json:"setMinElevation"`
}

// ApplyConfig sends all configuration changes to the receiver at once.
func (a *App) ApplyConfig(cfg ConfigUpdate) Result {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return Result{Error: "not connected"}
	}
	target := gpsprot.NewConfigTarget()
	if cfg.SetSignals {
		sigs := buildSignalSet(cfg.SignalIndices)
		if sigs == 0 {
			return Result{Error: "no signals selected"}
		}
		target.Props.SetSignalsEnabled(sigs)
	}
	if cfg.SetMode {
		switch cfg.Mode {
		case "static":
			target.Props.SetMode(gpsprot.Mode{Static: true})
		case "mobile":
			target.Props.SetMode(gpsprot.Mode{Static: false})
		default:
			return Result{Error: fmt.Sprintf("unknown mode: %q", cfg.Mode)}
		}
	}
	if cfg.SetPPS {
		tp := gpsprot.TimePulse{
			Width:          time.Duration(cfg.PPSWidth * float64(time.Second)),
			Period:         time.Duration(cfg.PPSPeriod * float64(time.Second)),
			AlignToGNSS:    cfg.PPSAlignToGNSS,
			OnlyWhenLocked: cfg.PPSOnlyLocked,
			PolarityRising: cfg.PPSRising,
		}
		target.Props.SetTimePulse(tp)
	}
	if cfg.SetTimeGNSS {
		g, err := gpsprot.ParseGNSS(cfg.TimeGNSS)
		if err != nil {
			return Result{Error: err.Error()}
		}
		target.Props.SetTimeGNSS(g)
	}
	if cfg.SetCableDelay {
		target.Props.SetAntennaCableDelay(time.Duration(cfg.CableDelay * float64(time.Nanosecond)))
	}
	if cfg.SetMinElevation {
		target.Props.SetMinElevation(gpsprot.DegreesFromFloat(cfg.MinElevation))
	}
	if target.Props.IsEmpty() {
		return Result{Error: "no configuration changes specified"}
	}
	return a.runConfig(target)
}

func buildSignalSet(indices []int) gpsprot.SignalSet {
	var ss gpsprot.SignalSet
	for _, idx := range indices {
		if idx >= 0 && idx < 64 {
			ss |= 1 << gpsprot.Signal(idx)
		}
	}
	return ss
}

// SaveConfig saves the current configuration to non-volatile memory.
func (a *App) SaveConfig() Result {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return Result{Error: "not connected"}
	}
	target := gpsprot.NewConfigTarget()
	target.Opts.Save = gpsprot.SaveAll
	return a.runConfig(target)
}

// ResetConfig resets the receiver configuration.
// resetType is "reload", "cold", or "factory".
func (a *App) ResetConfig(resetType string) Result {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return Result{Error: "not connected"}
	}
	target := gpsprot.NewConfigTarget()
	switch resetType {
	case "reload":
		target.Opts.Reset = gpsprot.ResetReload
	case "cold":
		target.Opts.Reset = gpsprot.ResetCold
	case "factory":
		target.Opts.Reset = gpsprot.ResetFactory
	default:
		return Result{Error: fmt.Sprintf("unknown reset type: %q", resetType)}
	}
	return a.runConfig(target)
}

// runConfig sends a configuration to the connected receiver.
func (a *App) runConfig(target *gpsprot.ConfigTarget) Result {
	a.mu.Lock()
	conn, pb := a.conn, a.pb
	a.mu.Unlock()
	sub := pb.Subscribe()
	defer pb.Unsubscribe(sub)
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	_, err := gpscfg.Configure(ctx, a.lg,
		gpsreg.CreatePacketProcessors(nil),
		gpsreg.CreateConfigProtocols(),
		target, sub, conn)
	if err != nil {
		return Result{Error: err.Error()}
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
	for _, a := range h.attrs {
		attrs[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
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

// ECEFtoLLH converts Earth-Centered Earth-Fixed coordinates (meters) to latitude,
// longitude (degrees), and height above the WGS84 ellipsoid (meters).
func (a *App) ECEFtoLLH(x, y, z float64) (*LLH, error) {
	llh, err := geopos.WGS84.ECEFtoLLH(geopos.ECEF{x, y, z})
	if err != nil {
		return nil, err
	}
	return &LLH{Lat: llh.Lat, Lon: llh.Lon, Height: llh.Height}, nil
}

// msgHandler implements gpsprot.MsgHandler and emits "gps:msg" events.
type msgHandler struct {
	ctx      context.Context
	ls       ptime.LeapSecond
	lastTime ptime.Time
}

func (h *msgHandler) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	if msg.Ref == gpsprot.PrePulse {
		return
	}
	sec, ok := msg.ComputeTAITime(h.ls)
	if !ok {
		return
	}
	secRnd := sec.Round(time.Second)
	if secRnd <= h.lastTime {
		return
	}
	h.lastTime = secRnd
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

// msgDecoderWorker subscribes to the packet broadcast and decodes packets
// into semantic messages, emitting them as "gps:msg" events.
func (a *App) msgDecoderWorker(sub <-chan scan.Packet) {
	procs := gpsreg.CreatePacketProcessors(nil)
	mh := &msgHandler{
		ctx: a.ctx,
		ls:  ptime.LeapSecond2016(),
	}
	for _, pp := range procs {
		pp.SetMsgHandler(mh)
	}
	for pkt := range sub {
		if len(pkt.Data) == 0 {
			continue
		}
		tag := pkt.Tag()
		pp := procs[tag]
		if pp == nil {
			continue
		}
		if pkt.IsInterPacketTimeout() {
			pp.Idle(pkt.TRead)
		} else {
			pp.ProcessPacket(pkt.Data, pkt.TRead)
		}
	}
}
