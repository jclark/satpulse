package main

import (
	"context"
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

// ReceiverInfo holds detected GPS receiver information.
type ReceiverInfo struct {
	OK             bool     `json:"ok"`
	Error          string   `json:"error,omitempty"`
	Vendor         string   `json:"vendor,omitempty"`
	Hardware       string   `json:"hardware,omitempty"`
	Firmware       string   `json:"firmware,omitempty"`
	SupportedGNSS  []string `json:"supportedGNSS,omitempty"`
	PacketFormats  []string `json:"packetFormats,omitempty"`
	Constellations []string `json:"constellations,omitempty"`
	Mode           string   `json:"mode,omitempty"`
	PPSEnabled     *bool    `json:"ppsEnabled,omitempty"`
	TimeGNSS       string   `json:"timeGNSS,omitempty"`
}

// showProps is the set of properties to request when detecting a receiver.
const showProps = gpsprot.PropIDSignalsEnabled |
	gpsprot.PropIDMode |
	gpsprot.PropIDTimePulse |
	gpsprot.PropIDTimeGNSS |
	gpsprot.PropIDAntennaCableDelay

// DetectReceiver probes the GPS receiver and returns its identity and current configuration.
func (a *App) DetectReceiver() ReceiverInfo {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return ReceiverInfo{Error: "not connected"}
	}
	target := gpsprot.NewConfigTarget()
	target.Get = showProps
	target.Opts.ForceProbe = gpsprot.ForceProbeWhenNoConfig
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	pCh := make(chan scan.Packet, 1)
	wg.Go(func() { gpsio.Scan(ctx, a.lg, conn, pCh, nil) })
	rslt, err := gpscfg.Configure(ctx, a.lg,
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
		if sigs, ok := rslt.ConfigProps.GetSignalsEnabled(); ok {
			for _, group := range sigs.GNSSStringGroups() {
				r.Constellations = append(r.Constellations, group[0])
			}
		}
		if mode, ok := rslt.ConfigProps.GetMode(); ok {
			if mode.Static {
				r.Mode = "static"
			} else {
				r.Mode = "mobile"
			}
		}
		if tp, ok := rslt.ConfigProps.GetTimePulse(); ok {
			enabled := tp.Width > 0
			r.PPSEnabled = &enabled
		}
		if tg, ok := rslt.ConfigProps.GetTimeGNSS(); ok {
			r.TimeGNSS = tg.String()
		}
	}
	return r
}

// ConfigureGNSS enables the specified GNSS constellations on the receiver.
// gnss is a comma-separated list like "GPS,GAL,BDS,GLO".
func (a *App) ConfigureGNSS(gnss string) Result {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return Result{Error: "not connected"}
	}
	names := strings.Split(gnss, ",")
	var systems []gpsprot.GNSS
	for _, name := range names {
		g, err := gpsprot.ParseGNSS(strings.TrimSpace(name))
		if err != nil {
			return Result{Error: err.Error()}
		}
		systems = append(systems, g)
	}
	sigs := gpsprot.BandAll.SignalSet(systems...)
	target := gpsprot.NewConfigTarget()
	target.Props.SetSignalsEnabled(sigs)
	return a.runConfig(target)
}

// ConfigurePPS sets the PPS output.
// width is the pulse width in seconds (0 to disable).
func (a *App) ConfigurePPS(width float64) Result {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return Result{Error: "not connected"}
	}
	target := gpsprot.NewConfigTarget()
	target.Props.SetPPS(time.Duration(width * float64(time.Second)))
	return a.runConfig(target)
}

// ConfigureMode sets the receiver to static or mobile mode.
func (a *App) ConfigureMode(static bool) Result {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return Result{Error: "not connected"}
	}
	target := gpsprot.NewConfigTarget()
	target.Props.SetMode(gpsprot.Mode{Static: static})
	return a.runConfig(target)
}

// runConfig sends a configuration to the connected receiver.
func (a *App) runConfig(target *gpsprot.ConfigTarget) Result {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	pCh := make(chan scan.Packet, 1)
	wg.Go(func() { gpsio.Scan(ctx, a.lg, conn, pCh, nil) })
	_, err := gpscfg.Configure(ctx, a.lg,
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
