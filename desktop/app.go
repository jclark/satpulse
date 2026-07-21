package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jclark/satpulse/desktop/logdir"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jclark/satpulse/gps/app/logfile"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsdecode"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/serialenum"
	"github.com/jclark/satpulse/gps/msgfile"
)

// App is the Wails shell around gps/app/session: it binds session
// methods to the frontend, delivers session events over the Wails
// event bus, and owns the desktop-only pieces (native dialogs, log
// files, serial port enumeration).
type App struct {
	ctx       context.Context
	lg        *slog.Logger
	sess      *session.Session
	logDir    string
	systemLog *logfile.LogFile
	packetLog *logfile.LogFile
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// wailsSink delivers session events to the frontend via the Wails
// event bus.
type wailsSink struct {
	ctx context.Context
}

func (s wailsSink) Emit(ev session.Event) {
	runtime.EventsEmit(s.ctx, string(ev.Name), ev.Data)
}

// Wants reports true unconditionally: the desktop frontend is the
// only client and always listens, gps:packet included.
func (s wailsSink) Wants(session.EventName) bool { return true }

// startup is called when the app starts. The context is saved
// so we can call the runtime methods.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	base := slog.Handler(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	if err := a.openLogFiles(); err != nil {
		fmt.Fprintln(os.Stderr, "error opening log files:", err)
	} else if a.systemLog != nil {
		jsonHandler := slog.NewJSONHandler(a.systemLog.File, &slog.HandlerOptions{Level: slog.LevelDebug})
		base = slog.NewMultiHandler(base, jsonHandler)
	}
	sink := wailsSink{ctx: ctx}
	a.lg = slog.New(session.NewLogHandler(sink, base))
	opts := session.Options{}
	if a.packetLog != nil {
		opts.PacketLog = a.packetLog.File
	}
	a.sess = session.New(a.lg, sink, opts)
	if a.logDir != "" {
		a.lg.Info("opened desktop log files", "dir", a.logDir)
	}
}

// shutdown is called when the app terminates.
func (a *App) shutdown(_ context.Context) {
	a.sess.Disconnect()
	if a.packetLog != nil {
		a.packetLog.Close(a.lg)
	}
	if a.systemLog != nil {
		a.systemLog.Close(a.lg)
	}
}

// openLogFiles opens the desktop log files (system.jsonl and
// packets.jsonl) in the per-user log directory.
func (a *App) openLogFiles() error {
	dir, err := logdir.Path()
	if err != nil {
		return err
	}
	systemLog := &logfile.LogFile{}
	if err := systemLog.Open(filepath.Join(dir, "system.jsonl"), false); err != nil {
		return err
	}
	packetLog := &logfile.LogFile{}
	if err := packetLog.Open(filepath.Join(dir, "packets.jsonl"), false); err != nil {
		systemLog.Close(slog.Default())
		return err
	}
	a.logDir = dir
	a.systemLog = systemLog
	a.packetLog = packetLog
	return nil
}

// Result is the standard return type for frontend-bound action
// methods, adapting the session's error returns for the frontend.
type Result struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func toResult(err error) Result {
	if err != nil {
		return Result{Error: err.Error()}
	}
	return Result{OK: true}
}

// Connect opens the given serial device and starts a receiver scan.
func (a *App) Connect(device string, speed int) Result {
	return toResult(a.sess.Connect(session.SerialOpener{Device: device, Speed: speed}, gpsreg.VendorUnknown))
}

// Disconnect closes the connection.
func (a *App) Disconnect() Result {
	a.sess.Disconnect()
	return Result{OK: true}
}

// ListPorts returns the available serial ports.
func (a *App) ListPorts() ([]serialenum.Port, error) {
	return serialenum.List()
}

// GetConnState returns the current connection state.
func (a *App) GetConnState() session.ConnState {
	return a.sess.State()
}

// GetReceiverState returns the result of the last probe.
func (a *App) GetReceiverState() session.ReceiverEvent {
	return a.sess.Receiver()
}

// GetAllSignals returns the signal catalog for the given
// constellations; a zero GNSSSet means all constellations.
func (a *App) GetAllSignals(gs gpsprot.GNSSSet) map[string][]string {
	return a.sess.SignalCatalog(gs)
}

// ReadConfig reads the current receiver configuration.
func (a *App) ReadConfig() (*gpsprot.ConfigProps, error) {
	return a.sess.ReadConfig(a.ctx)
}

// ApplyConfig applies a configuration change to the receiver.
func (a *App) ApplyConfig(cfg gpsprot.ConfigTarget) Result {
	return toResult(a.sess.ApplyConfig(a.ctx, &cfg))
}

// MsgFileInfo describes a loaded message file.
type MsgFileInfo struct {
	Path string               `json:"path"`
	Tags []session.MsgFileTag `json:"tags"`
}

// LoadMsgFile prompts for a message file with the native file dialog,
// parses it, and installs it in the session. Returns nil if the user
// cancels the dialog.
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
	tags := a.sess.SetMsgFile(mf)
	return &MsgFileInfo{Path: filepath.Base(path), Tags: tags}, nil
}

// SendMsgFile sends the messages under the given tag of the loaded
// message file to the receiver.
func (a *App) SendMsgFile(tag string, port string, save bool) Result {
	return toResult(a.sess.SendMsgFile(tag, port, save))
}

// CancelMsgSend cancels an in-progress message file send.
func (a *App) CancelMsgSend() Result {
	return toResult(a.sess.CancelMsgSend())
}

// StartCorrections starts streaming corrections from the given source
// to the receiver.
func (a *App) StartCorrections(src session.CorrectionSource) Result {
	return toResult(a.sess.StartCorrections(src))
}

// StopCorrections stops streaming corrections.
func (a *App) StopCorrections() Result {
	return toResult(a.sess.StopCorrections())
}

// GetCorrectionsState returns the current corrections state.
func (a *App) GetCorrectionsState() session.CorrEvent {
	return a.sess.CorrectionsState()
}

// DecodeOptions controls packet decoding.
type DecodeOptions struct {
	Hex bool `json:"hex"` // data is hex-encoded binary
	Out bool `json:"out"` // packet is outbound (sent to receiver)
}

// DecodePacket decodes a single packet supplied by the user. It
// returns nil (with no error) if the packet cannot be decoded.
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
	return session.DecodePacket(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), b, opts.Out)
}

// LLH is a latitude/longitude/height position.
type LLH struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Height float64 `json:"height"`
}

// CheckOnEarth reports whether the given ECEF coordinates are
// plausibly on Earth.
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
	return a.sess.VelNEDtoECEF(n, e, d)
}

// VelECEFtoNED converts a velocity from ECEF (m/s) to NED using the last known position.
// Returns nil if no position is available.
func (a *App) VelECEFtoNED(vx, vy, vz float64) *[3]float64 {
	return a.sess.VelECEFtoNED(vx, vy, vz)
}

