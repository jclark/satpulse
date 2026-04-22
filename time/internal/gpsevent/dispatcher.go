package gpsevent

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/logfile"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/time/internal/phcsample"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/internal/refclock"
	"github.com/jclark/satpulse/time/internal/timemsg"
	"github.com/jclark/satpulse/time/internal/ts"
	"github.com/jclark/satpulse/time/phctime"
	"golang.org/x/sys/unix"
)

const LogExtension = ".jsonl"

// TimePulsePVTMsgFlags are the PVT message flags when time pulse is enabled.
const TimePulsePVTMsgFlags = gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTimePulseAfter | gpsprot.PVTMsgTAI | gpsprot.PVTMsgLeapSecond | gpsprot.PVTMsgSurvey | gpsprot.PVTMsgQuality | gpsprot.PVTMsgEpoch

// NoTimePulsePVTMsgFlags are the PVT message flags when time pulse is not enabled.
const NoTimePulsePVTMsgFlags = gpsprot.PVTMsgTime | gpsprot.PVTMsgLeapSecond | gpsprot.PVTMsgSurvey | gpsprot.PVTMsgQuality | gpsprot.PVTMsgEpoch

// PulseReceiver is the dispatcher's sink for PHC pulse-edge events.
// phcsync.Controller (disciplined mode) and phcsample.Generator
// (free-running mode) both satisfy it; the dispatcher uses a single
// typed field for the shared Pulse call and recovers the concrete type
// at construction time for mode-specific paths.
type PulseReceiver interface {
	Pulse(timestamp phctime.Time, tRead phctime.Sample)
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
	pulse                 PulseReceiver
	controller            *phcsync.Controller
	generator             *phcsample.Generator
	rc                    *refclock.ProxyRefClock
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

// NewDispatcher constructs a Dispatcher. The pulse argument selects the
// runtime mode:
//   - *phcsync.Controller: PHC-disciplined (phcsync steers the PHC).
//   - *phcsample.Generator: PHC free-running (phcsample emits samples).
//   - nil:                 serial timing (samples come from time messages).
func NewDispatcher(lg *slog.Logger, pktProcs map[gpsprot.Tag]gpsprot.PacketProcessor, pulse PulseReceiver, rc *refclock.ProxyRefClock, ls ptime.LeapSecond, obs obs.Observer, eventLogPath string, tStart time.Time) (*Dispatcher, error) {
	var controller *phcsync.Controller
	var generator *phcsample.Generator
	switch p := pulse.(type) {
	case *phcsync.Controller:
		controller = p
	case *phcsample.Generator:
		generator = p
	case nil:
		// serial timing mode
	default:
		panic("gpsevent: unexpected PulseReceiver type")
	}
	// Always create timeMsgBuffer (useful even without PHC)
	timeMsgBuffer := timemsg.NewBuffer(lg, 5*time.Second, ls, gpsprot.GPS)

	// Inject buffer into controller (for pulse/message correlation) or
	// into generator (as sawtooth pulse-correction source).
	if controller != nil {
		controller.SetTimeMsgBuffer(timeMsgBuffer)
	} else if generator != nil {
		generator.SetPulseCorrector(timeMsgBuffer)
	}
	d := Dispatcher{
		pktProcs:      pktProcs,
		pulse:         pulse,
		controller:    controller,
		generator:     generator,
		rc:            rc,
		timeMsgBuffer: timeMsgBuffer,
		timeTicker:    *gpsprot.NewTimeTicker(&tickHandler{obs: obs}, ls),
		ls:            ls,
		lg:            lg,
		obs:           obs,
		tStart:        tStart,
	}
	multiHandler := gpsprot.NewMultiHandler(&d, obs)
	for _, pp := range pktProcs {
		pp.SetMsgHandler(multiHandler)
		pp.SetNativeMsgHandler(&d)
	}
	// Route UTC time-message samples. In serial mode the Dispatcher
	// forwards them to rc.Sample; in free-running mode it routes them
	// to the generator; in PHC-disciplined mode no sink is installed.
	if generator != nil || (controller == nil && rc != nil) {
		timeMsgBuffer.SetMsgUTCTimer(&d)
	}
	err := d.lf.Open(eventLogPath, true)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

const tickPeriod = time.Second / 4

func (d *Dispatcher) Run(tsCh <-chan ts.Event, pktCh <-chan scan.Packet) {
	// loop until both channels are closed
	defer d.obs.Release()
	if d.rc != nil {
		defer d.rc.Close()
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

	for tsCh != nil || pktCh != nil {
		select {
		case e, ok := <-tsCh:
			if !ok {
				lg.Debug("timestamp channel of event dispatcher goroutine was closed")
				tsCh = nil
				continue
			}
			if e.Kind == ts.PauseEvent {
				if d.controller != nil {
					d.controller.Pause()
				} else if d.generator != nil {
					// Replace with a fresh instance; nothing in
					// the prior one is worth preserving across
					// a PHC era transition. d.pulse has to track
					// the swap so post-resume edges reach the new
					// instance, not the discarded one.
					d.generator = d.generator.NewInstance()
					d.pulse = d.generator
				}
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

type LogEvent struct {
	T          time.Time              `json:"t"`
	Nanos      time.Duration          `json:"nanos"`
	PulseEdge  *PulseEdge             `json:"pulseEdge,omitempty"`
	Time       *gpsprot.TimeMsg       `json:"time,omitempty"`
	PosGeo     *gpsprot.PosGeoMsg     `json:"posGeo,omitempty"`
	PosECEF    *gpsprot.PosECEFMsg    `json:"posECEF,omitempty"`
	VelGeo     *gpsprot.VelGeoMsg     `json:"velGeo,omitempty"`
	VelECEF    *gpsprot.VelECEFMsg    `json:"velECEF,omitempty"`
	Survey     *gpsprot.SurveyMsg     `json:"survey,omitempty"`
	LeapSecond *gpsprot.LeapSecondMsg `json:"leapSecond,omitempty"`
	Satellites *gpsprot.SatellitesMsg `json:"satellites,omitempty"`
	NavEpoch   *gpsprot.NavEpochMsg   `json:"navEpoch,omitempty"`
}

type PulseEdge struct {
	T     ptime.Time  `json:"t"`
	Era   phctime.Era `json:"era"`
	TRead ptime.Time  `json:"tRead"`
}

func (d *Dispatcher) timestamp(e ts.Event) {
	// timestamp events only occur when PHC is available, so d.pulse is
	// set to either the controller (disciplined) or the generator
	// (free-running). The cross-sample hop is mode-specific.
	d.pulse.Pulse(e.Ts, e.TReadMono)
	if d.controller != nil {
		d.sysSample(e.TReadWall.PHC.T, e.TReadWall.Sys)
	} else if d.generator != nil {
		d.genSample(e.TReadWall.PHC.T, e.TReadWall.Sys)
	}

	// Log event with monotonic time and full sample info
	d.logEvent(LogEvent{
		T: e.TReadMono.Sys,
		PulseEdge: &PulseEdge{
			T:     e.Ts.T,
			Era:   e.Ts.Era,
			TRead: e.TReadMono.PHC.T,
		},
	})
}

// sysSample generates a sample of system time vs true time (based on PHC)
func (d *Dispatcher) sysSample(ref ptime.Time, sys time.Time) {
	// Send refclock sample if in tracking mode
	if d.rc == nil || ref.IsZero() || d.controller.Mode() != phcsync.ModeTracking {
		return
	}
	offset := d.ls.TimeToSys(ref).Sub(sys).Seconds()
	leap := d.ls.StateAt(ref).LeapTonight
	err := d.rc.Sample(sys, offset, leap)
	if err != nil {
		d.lg.Warn("refclock sample failed", "err", err)
		return
	}
	d.obs.NTPSample(sys, offset, leap, ref)
}

// genSample drives the phcsample Generator for the current cross-sample
// and forwards successful offsets to the refclock and the observer.
func (d *Dispatcher) genSample(ref ptime.Time, sys time.Time) {
	if d.rc == nil || ref.IsZero() {
		return
	}
	off, err := d.generator.Generate(ref, sys)
	if err != nil {
		if !errors.Is(err, phcsample.ErrNotReady) {
			d.lg.Warn("could not derive offset from PHC", "err", err)
		}
		return
	}
	leap := d.ls.StateAt(ref).LeapTonight
	if err := d.rc.Sample(sys, off, leap); err != nil {
		d.lg.Warn("refclock sample failed", "err", err)
		return
	}
	d.obs.NTPSample(sys, off, leap, ref)
}

// MsgUTCTime implements timemsg.MsgUTCTimer. In serial timing mode it
// feeds chrony SOCK samples directly; in PHC free-running mode it
// forwards to the Generator.
func (d *Dispatcher) MsgUTCTime(utc time.Time, tRead time.Time, leap ptime.LeapSecondKind) {
	if d.generator != nil {
		d.generator.MsgUTCTime(utc, tRead, leap)
		return
	}
	offset := utc.Sub(tRead).Seconds()
	err := d.rc.Sample(tRead, offset, leap)
	if err != nil {
		d.lg.Warn("refclock sample failed", "err", err)
		return
	}
	d.obs.NTPSample(tRead, offset, leap, ptime.Time(0))
}

func (d *Dispatcher) Time(mt *gpsprot.TimeMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, Time: mt})

	d.timeMsgBuffer.Time(mt, tRead)

	// Notify controller that a time message arrived
	if d.controller != nil {
		d.controller.TimeMessage()
	}

	d.timeTicker.Time(mt, tRead)
}

func (d *Dispatcher) Survey(m *gpsprot.SurveyMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, Survey: m})
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
	d.logEvent(LogEvent{T: tRead, PosGeo: msg})
	d.pvAccum.PosGeo(msg, tRead)
}

func (d *Dispatcher) PosECEF(msg *gpsprot.PosECEFMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, PosECEF: msg})
	d.pvAccum.PosECEF(msg, tRead)
}

func (d *Dispatcher) VelGeo(msg *gpsprot.VelGeoMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, VelGeo: msg})
	d.pvAccum.VelGeo(msg, tRead)
}

func (d *Dispatcher) VelECEF(msg *gpsprot.VelECEFMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, VelECEF: msg})
	d.pvAccum.VelECEF(msg, tRead)
}

func (d *Dispatcher) Satellites(msg *gpsprot.SatellitesMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, Satellites: msg})
}

func (d *Dispatcher) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, NavEpoch: msg})
	d.timeTicker.NavEpoch(msg, tRead)
	d.pvAccum.PVMsgBundle.FillDerived()
	pv := d.pvAccum.PVMsgBundle // copy before clearing
	d.pvAccum.NavEpoch(msg, tRead)
	d.obs.NavEpochPV(msg, &pv, tRead)
}

func (d *Dispatcher) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, LeapSecond: msg})
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
	d.lg.Debug("unused message from GPS receiver", "protocol", tag, "msgID", msgID)
	return nil
}

func (d *Dispatcher) logEvent(event LogEvent) {
	if d.lf.File == nil {
		return
	}
	event.Nanos = event.T.Sub(d.tStart)
	bytes, err := json.Marshal(event)
	if err != nil {
		d.lg.Warn("failed to convert event to JSON", "event", event, "err", err)
		return
	}
	bytes = append(bytes, '\n')
	_, err = d.lf.File.Write(bytes)
	d.lf.HandleWriteError(err, d.lg)
}
