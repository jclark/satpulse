package gpsevent

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/logfile"
	"github.com/jclark/satpulse/gps/app/stream"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/internal/refclock"
	"github.com/jclark/satpulse/time/internal/timemsg"
	"github.com/jclark/satpulse/time/internal/ts"
	"github.com/jclark/satpulse/time/lib/median"
	"github.com/jclark/satpulse/time/lib/ntime"
	"github.com/jclark/satpulse/time/lib/ntpshm"
	"github.com/jclark/satpulse/time/phctime"
	"golang.org/x/sys/unix"
)

const LogExtension = ".jsonl"
const precisionCalibrationSamples = 61

// SHMWriter writes samples to an NTP SHM segment.
type SHMWriter interface {
	Write(clockTime, receiveTime time.Time, leap ptime.LeapSecondKind)
	Close() error
}

type samplePrecisionSetter interface {
	setSamplePrecision(time.Duration)
}

type calibratingSHMWriter struct {
	w   *ntpshm.Writer
	win *median.Window[time.Duration]
}

// NewSHMWriter configures precision handling for an NTP SHM writer.
func NewSHMWriter(w *ntpshm.Writer, precision *int8) SHMWriter {
	if w == nil {
		return nil
	}
	if precision != nil {
		w.SetPrecision(*precision)
		return w
	}
	return &calibratingSHMWriter{
		w:   w,
		win: median.New[time.Duration](precisionCalibrationSamples),
	}
}

func (w *calibratingSHMWriter) Write(clockTime, receiveTime time.Time, leap ptime.LeapSecondKind) {
	w.w.Write(clockTime, receiveTime, leap)
}

func (w *calibratingSHMWriter) Close() error {
	return w.w.Close()
}

func (w *calibratingSHMWriter) setSamplePrecision(p time.Duration) {
	if w.win == nil {
		return
	}
	if w.win.Len() == 0 {
		w.w.SetPrecision(ntpshm.Precision(p))
	}
	w.win.Add(p)
	if w.win.Len() == precisionCalibrationSamples {
		w.w.SetPrecision(ntpshm.Precision(w.win.Median()))
		w.win = nil
	}
}

// tickHandler forwards filled TimeMsgs from the TimeTicker to Observer.Tick.
type tickHandler struct {
	gpsprot.DefaultHandler
	obs obs.Observer
}

func (h *tickHandler) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	h.obs.Tick(msg, tRead)
}

type Dispatcher struct {
	gpsprot.DefaultHandler
	pktProcs              map[gpsprot.Tag]gpsprot.PacketProcessor
	obs                   obs.Observer // never nil
	controller            *phcsync.Controller
	rc                    *refclock.ProxyRefClock
	shm                   SHMWriter
	sps                   samplePrecisionSetter
	timeMsgBuffer         *timemsg.Buffer
	timeTicker            gpsprot.TimeTicker
	pvAccum               gpsprot.PVMsgAccum
	ls                    ptime.LeapSecond
	lg                    *slog.Logger
	lf                    logfile.LogFile
	loggedUnknownProtocol bool
	loggedSurveyComplete  bool
	tStart                time.Time
}

// NewDispatcher creates the GPS event dispatcher.
func NewDispatcher(
	lg *slog.Logger,
	pktProcs map[gpsprot.Tag]gpsprot.PacketProcessor,
	controller *phcsync.Controller,
	rc *refclock.ProxyRefClock,
	shm SHMWriter,
	ls ptime.LeapSecond,
	obs obs.Observer,
	eventLogPath string,
	tStart time.Time,
) (*Dispatcher, error) {
	// Always create timeMsgBuffer (useful even without PHC)
	var minWindow time.Duration
	if controller != nil {
		minWindow = controller.RequiredMsgWindow()
	}
	timeMsgBuffer := timemsg.NewBuffer(lg, minWindow, ls, gpsprot.GPS)

	// Inject buffer into controller if controller exists
	if controller != nil {
		controller.SetTimeMsgBuffer(timeMsgBuffer)
	}
	d := Dispatcher{
		pktProcs:      pktProcs,
		controller:    controller,
		rc:            rc,
		timeMsgBuffer: timeMsgBuffer,
		timeTicker:    *gpsprot.NewTimeTicker(&tickHandler{obs: obs}, ls),
		ls:            ls,
		lg:            lg,
		obs:           obs,
		tStart:        tStart,
	}
	d.shm = shm
	if p, ok := shm.(samplePrecisionSetter); ok {
		d.sps = p
	}
	multiHandler := gpsprot.NewMultiHandler(&d, obs)
	for _, pp := range pktProcs {
		pp.SetMsgHandler(multiHandler)
		pp.SetNativeMsgHandler(&d)
	}
	// In serial timing mode (no PHC, but refclock configured), feed
	// NTP samples directly from time messages.
	if controller == nil && (rc != nil || shm != nil) {
		timeMsgBuffer.SetMsgUTCTimer(&d)
	}
	err := d.lf.Open(eventLogPath, true)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

const tickPeriod = time.Second / 4

func (d *Dispatcher) Run(tsCh <-chan ts.Event, pktCh <-chan scan.Packet, pullPktCh <-chan scan.Packet) {
	// loop until all input channels are closed
	defer d.obs.Release()
	if d.rc != nil {
		defer d.rc.Close()
	}
	if d.shm != nil {
		defer func() {
			if err := d.shm.Close(); err != nil {
				d.lg.Warn("SHM detach failed", "err", err)
			}
		}()
	}
	// close the controller before the observer, since the controller uses the observer
	if d.controller != nil {
		defer d.controller.Close()
	}
	var ticker *time.Ticker
	var tickerCh <-chan time.Time
	var firstTsDeadline <-chan time.Time
	if d.controller != nil {
		ticker = time.NewTicker(tickPeriod)
		defer ticker.Stop()
		tickerCh = ticker.C
	}
	if tsCh != nil {
		// give a warning if we haven't received a timestamp by the time this fires
		firstTsDeadline = time.After(time.Second * 2)
	}
	// Use SIGHUP as a signal to reopen the log file (e.g. after log rotation)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, unix.SIGHUP)
	lg := d.lg
	defer d.lf.Close(d.lg)
	lg.Debug("event dispatcher goroutine started")

	staleEra := ts.StaleEra
	nSkipped := 0

	for tsCh != nil || pktCh != nil || pullPktCh != nil {
		select {
		case e, ok := <-tsCh:
			if !ok {
				lg.Debug("timestamp channel of event dispatcher goroutine was closed")
				tsCh = nil
				continue
			}
			if e.Kind == ts.PauseEvent {
				d.controller.Pause()
				continue
			}
			if e.Kind == ts.ResumeEvent {
				staleEra = e.ResumeFunc()
				continue
			}
			if firstTsDeadline != nil {
				lg.Info("successfully received a external timestamp from the PTP hardware clock")
				firstTsDeadline = nil
			}
			if e.Ts.Era == staleEra {
				if nSkipped == 0 {
					lg.Debug("detected a stale PTP hardware clock timestamp", "t", e.Ts.T)
				}
				nSkipped++
			} else {
				if nSkipped > 0 {
					lg.Info("skipped stale PTP hardware clock timestamps", "n", nSkipped)
					nSkipped = 0
				}
				d.timestamp(e)
			}

		case pkt, ok := <-pktCh:
			if ok {
				d.handlePacket(pkt)
			} else {
				lg.Debug("packet channel of event dispatcher goroutine was closed")
				pktCh = nil
			}
		case pkt, ok := <-pullPktCh:
			if ok {
				d.handlePulledPacket(pkt)
			} else {
				lg.Debug("pull packet channel of event dispatcher goroutine was closed")
				pullPktCh = nil
			}
		case t := <-tickerCh:
			d.controller.Tick(t)
		case <-firstTsDeadline:
			lg.Warn("no PTP hardware clock external timestamps being received")
			firstTsDeadline = nil
		case <-sig:
			d.obs.ReopenLog()
			d.lf.Reopen(d.lg)
		}
	}
}

func (d *Dispatcher) handlePacket(pkt scan.Packet) {
	if pkt.IsInterPacketTimeout() {
		for _, pp := range d.pktProcs {
			pp.Idle(pkt.TRead)
		}
		return
	}
	lg := d.lg

	if pkt.Format == nil {
		lg.Debug("received invalid data from GPS receiver", "len", len(pkt.Data), "data", pkt.Data)
		if !d.loggedUnknownProtocol {
			lg.Info("received data from GPS in unrecognized format (serial communication problem?)",
				"len", len(pkt.Data), "data", pkt.Data)
			d.loggedUnknownProtocol = true
		}
		return
	}
	tag := pkt.Format.Tag()
	pp, ok := d.pktProcs[tag]
	if !ok {
		lg.Error("no processor registered for packet tag", "tag", tag)
		return
	}
	err := pkt.ChecksumError()
	if err != nil {
		lg.Warn(err.Error(), "tag", tag, "len", len(pkt.Data))
	}
	msgID, err := pp.ProcessPacket(pkt.Data, pkt.TRead)
	if err != nil {
		lg.Warn("error processing packet", gpsio.PacketWarnAttrs(err, pkt, msgID)...)
	}
}

func (d *Dispatcher) handlePulledPacket(pkt scan.Packet) {
	msg, err := stream.CorReportFromPacket(pkt)
	if err != nil {
		msgID := ""
		if msg != nil {
			msgID = msg.MsgID
		}
		d.lg.Warn("error processing pulled correction packet", gpsio.PacketWarnAttrs(err, pkt, msgID)...)
	}
	if msg == nil {
		return
	}
	// Packet processors emit through MultiHandler(&d, obs). Pull reports are
	// created here, so mirror that logger/observer fan-out explicitly.
	d.CorReport(msg, pkt.TRead)
	d.obs.CorReport(msg, pkt.TRead)
}

// LogEvent is a daemon event-log entry: the universal gpsprot event envelope
// with a payload that is either a gpsprot.Msg (GPS message records) or a
// *PulseEdge (pulse-edge records, with Type "pulseEdge").
type LogEvent gpsprot.Event[any]

// pulseEdgeType is the LogEvent.Type value for pulse-edge records.
const pulseEdgeType = "pulseEdge"

type PulseEdge struct {
	T     ptime.Time  `json:"t"`
	Era   phctime.Era `json:"era"`
	TRead ptime.Time  `json:"tRead"`
}

// UnmarshalJSON decodes a LogEvent, dispatching on the type discriminator:
// "pulseEdge" records decode Data as *PulseEdge, all others as the gpsprot.Msg
// named by Type. It accepts only the new envelope format; old sparse event
// logs are handled by gpsevent/migrate_log.go.
func (e *LogEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type string           `json:"type"`
		T    time.Time        `json:"t"`
		Mono gpsprot.Duration `json:"mono"`
		Data json.RawMessage  `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Type = raw.Type
	e.T = raw.T
	e.Mono = raw.Mono
	if raw.Type == pulseEdgeType {
		var pe PulseEdge
		if err := json.Unmarshal(raw.Data, &pe); err != nil {
			return err
		}
		e.Data = &pe
		return nil
	}
	msg, err := gpsprot.UnmarshalMsg(raw.Type, raw.Data)
	if err != nil {
		return err
	}
	e.Data = msg
	return nil
}

func (d *Dispatcher) timestamp(e ts.Event) {
	// timestamp events only occur when PHC is available, so d.controller is non-nil
	// Pass monotonic sample to controller (for PHC sync logic)
	d.controller.PulseEdge(phcsync.PulseEdge{
		Timestamp: e.Ts,
		TRead:     e.TReadMono,
	})

	// Pass wallclock sample to sysSample (for NTP refclocks)
	d.sysSample(e.TReadWall.PHC.T, e.TReadWall.Sys, e.Precision)

	// Log event with monotonic time and full sample info
	d.logEvent(LogEvent{
		Type: pulseEdgeType,
		T:    e.TReadMono.Sys,
		Data: &PulseEdge{
			T:     e.Ts.T,
			Era:   e.Ts.Era,
			TRead: e.TReadMono.PHC.T,
		},
	})
}

// sysSample generates a sample of system time vs true time (based on PHC)
func (d *Dispatcher) sysSample(ref ptime.Time, sys time.Time, samplePrecision time.Duration) {
	// Send refclock sample if in tracking mode
	if (d.rc == nil && d.shm == nil) || ref.IsZero() || d.controller.Mode() != phcsync.ModeTracking {
		return
	}
	if d.sps != nil {
		d.sps.setSamplePrecision(samplePrecision)
	}
	clockTime := d.ls.TimeToSys(ref)
	offset := clockTime.Sub(sys).Seconds()
	leap := d.ls.StateAt(ref).LeapTonight
	if d.rc != nil {
		if err := d.rc.Sample(ntime.Sys(sys), offset, leap); err != nil {
			d.lg.Warn("refclock sample failed", "err", err)
		}
	}
	if d.shm != nil {
		d.shm.Write(clockTime, sys, leap)
	}
	d.obs.NTPSample(sys, offset, leap, ref)
}

// MsgUTCTime implements timemsg.MsgUTCTimer for serial timing mode.
func (d *Dispatcher) MsgUTCTime(utc time.Time, tRead time.Time, leap ptime.LeapSecondKind) {
	offset := utc.Sub(tRead).Seconds()
	if d.rc != nil {
		if err := d.rc.Sample(ntime.Sys(tRead), offset, leap); err != nil {
			d.lg.Warn("refclock sample failed", "err", err)
		}
	}
	if d.shm != nil {
		d.shm.Write(utc, tRead, leap)
	}
	d.obs.NTPSample(tRead, offset, leap, ptime.Time(0))
}

func (d *Dispatcher) Time(mt *gpsprot.TimeMsg, tRead time.Time) {
	d.logMsg(mt, tRead)

	d.timeMsgBuffer.Time(mt, tRead)

	// Notify controller that a time message arrived
	if d.controller != nil {
		d.controller.TimeMessage()
	}

	d.timeTicker.Time(mt, tRead)
}

func (d *Dispatcher) Survey(m *gpsprot.SurveyMsg, tRead time.Time) {
	d.logMsg(m, tRead)
	if m.InProgress || !d.loggedSurveyComplete {
		d.loggedSurveyComplete = !m.InProgress
		d.lg.Info("survey progress",
			"position", m.Position,
			"accuracy", m.Accuracy,
			"inProgress", m.InProgress,
			"valid", m.Valid,
			"obsCount", m.ObsCount,
			"obsTime", m.ObsTime)
	}
}

func (d *Dispatcher) PosGeo(msg *gpsprot.PosGeoMsg, tRead time.Time) {
	d.logMsg(msg, tRead)
	d.pvAccum.PosGeo(msg, tRead)
}

func (d *Dispatcher) PosECEF(msg *gpsprot.PosECEFMsg, tRead time.Time) {
	d.logMsg(msg, tRead)
	d.pvAccum.PosECEF(msg, tRead)
}

func (d *Dispatcher) VelGeo(msg *gpsprot.VelGeoMsg, tRead time.Time) {
	d.logMsg(msg, tRead)
	d.pvAccum.VelGeo(msg, tRead)
}

func (d *Dispatcher) VelECEF(msg *gpsprot.VelECEFMsg, tRead time.Time) {
	d.logMsg(msg, tRead)
	d.pvAccum.VelECEF(msg, tRead)
}

func (d *Dispatcher) Satellites(msg *gpsprot.SatellitesMsg, tRead time.Time) {
	d.logMsg(msg, tRead)
}

func (d *Dispatcher) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	d.logMsg(msg, tRead)
	d.timeTicker.NavEpoch(msg, tRead)
	d.pvAccum.PVMsgBundle.FillDerived()
	pv := d.pvAccum.PVMsgBundle // copy before clearing
	d.pvAccum.NavEpoch(msg, tRead)
	d.obs.NavEpochPV(msg, &pv, tRead)
}

func (d *Dispatcher) CorReport(msg *gpsprot.CorReportMsg, tRead time.Time) {
	// Receiver-derived reports already reach observers through
	// MultiHandler(&d, obs), so the dispatcher side only records the event log.
	d.logMsg(msg, tRead)
}

func (d *Dispatcher) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	d.logMsg(msg, tRead)
	d.timeTicker.LeapSecond(msg, tRead)
	if msg.UpdateLeapSecond(&d.ls) {
		// Update timemsg.Buffer (always exists)
		d.timeMsgBuffer.LeapSecond(msg, tRead)

		// Update controller if it exists
		if d.controller != nil {
			d.controller.LeapSecond(d.ls)
		}
	}
}

func (d *Dispatcher) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	if !d.obs.NativeMsg(tag, msgID, msg, tRead) {
		d.lg.Debug("unused message from GPS receiver", "protocol", tag, "msgID", msgID)
	}
	return nil
}

// logMsg records a gpsprot.Msg as a GPS message event-log record.
func (d *Dispatcher) logMsg(msg gpsprot.Msg, tRead time.Time) {
	d.logEvent(LogEvent{Type: msg.MsgType(), T: tRead, Data: msg})
}

func (d *Dispatcher) logEvent(event LogEvent) {
	if d.lf.File == nil {
		return
	}
	event.Mono = gpsprot.Duration(event.T.Sub(d.tStart)) / gpsprot.Microsecond * gpsprot.Microsecond
	bytes, err := json.Marshal(event)
	if err != nil {
		d.lg.Warn("failed to convert event to JSON", "event", event, "err", err)
		return
	}
	bytes = append(bytes, '\n')
	_, err = d.lf.File.Write(bytes)
	d.lf.HandleWriteError(err, d.lg)
}
