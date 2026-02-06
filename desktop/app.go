package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/scan"
)

// App is the Wails application struct.
// Its exported methods are bound to the frontend.
type App struct {
	ctx context.Context
	lg  *slog.Logger
	mu  sync.Mutex
	conn          gpsio.Conn
	captureCancel context.CancelFunc
}

// NewApp creates a new App.
func NewApp() *App {
	return &App{
		lg: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) shutdown(_ context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closeLocked()
}

func (a *App) closeLocked() {
	if a.captureCancel != nil {
		a.captureCancel()
		a.captureCancel = nil
	}
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
	a.conn = conn
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
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return ReceiverInfo{Error: "not connected"}
	}
	target := gpsprot.NewConfigTarget()
	target.Get = getProps
	target.Opts.ForceProbe = gpsprot.ForceProbeWhenNoConfig
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	lg := a.eventLogger()
	var wg sync.WaitGroup
	pCh := make(chan scan.Packet, 1)
	wg.Go(func() { gpsio.Scan(ctx, lg, conn, pCh, nil) })
	rslt, err := gpscfg.Configure(ctx, lg,
		gpsreg.CreatePacketProcessors(nil),
		gpsreg.CreateConfigProtocols(),
		target, pCh, conn)
	conn.Stop()
	for range pCh {
	}
	wg.Wait()
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
	conn := a.conn
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	lg := a.eventLogger()
	var wg sync.WaitGroup
	pCh := make(chan scan.Packet, 1)
	wg.Go(func() { gpsio.Scan(ctx, lg, conn, pCh, nil) })
	_, err := gpscfg.Configure(ctx, lg,
		gpsreg.CreatePacketProcessors(nil),
		gpsreg.CreateConfigProtocols(),
		target, pCh, conn)
	conn.Stop()
	for range pCh {
	}
	wg.Wait()
	if err != nil {
		return Result{Error: err.Error()}
	}
	return Result{OK: true}
}

// eventLogger returns a logger that emits log messages as Wails events
// so the frontend can display progress, and also logs to stderr.
func (a *App) eventLogger() *slog.Logger {
	base := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	eh := &eventHandler{ctx: a.ctx, base: base}
	return slog.New(eh)
}

// eventHandler is an slog.Handler that emits "gps:log" events to the frontend
// for INFO and above, while forwarding all messages to a base handler.
type eventHandler struct {
	ctx   context.Context
	base  slog.Handler
	attrs []slog.Attr
	group string
}

// LogEvent is emitted to the frontend for progress messages.
type LogEvent struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

func (h *eventHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.base.Enabled(context.Background(), level)
}

func (h *eventHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelInfo {
		msg := r.Message
		var parts []string
		r.Attrs(func(a slog.Attr) bool {
			parts = append(parts, fmt.Sprintf("%s=%v", a.Key, a.Value))
			return true
		})
		for _, a := range h.attrs {
			parts = append(parts, fmt.Sprintf("%s=%v", a.Key, a.Value))
		}
		if len(parts) > 0 {
			msg += " " + strings.Join(parts, " ")
		}
		runtime.EventsEmit(h.ctx, "gps:log", LogEvent{
			Level:   r.Level.String(),
			Message: msg,
			Time:    r.Time.Format(time.TimeOnly),
		})
	}
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
	Data      string `json:"data"`
	Timestamp string `json:"timestamp"`
}

// StartCapture begins streaming GPS packets as "gps:packet" events.
func (a *App) StartCapture() Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		return Result{Error: "not connected"}
	}
	if a.captureCancel != nil {
		return Result{Error: "capture already running"}
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.captureCancel = cancel
	conn := a.conn
	go func() {
		pCh := make(chan scan.Packet, 4)
		var wg sync.WaitGroup
		wg.Go(func() { gpsio.Scan(ctx, a.lg, conn, pCh, nil) })
		for {
			select {
			case <-ctx.Done():
				conn.Stop()
				for range pCh {
				}
				wg.Wait()
				return
			case pkt, ok := <-pCh:
				if !ok {
					return
				}
				runtime.EventsEmit(a.ctx, "gps:packet", PacketEvent{
					Tag:       string(pkt.Tag()),
					Data:      pkt.Data,
					Timestamp: time.Now().Format(time.TimeOnly),
				})
			}
		}
	}()
	return Result{OK: true}
}

// StopCapture stops the packet capture.
func (a *App) StopCapture() Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.captureCancel != nil {
		a.captureCancel()
		a.captureCancel = nil
	}
	return Result{OK: true}
}
