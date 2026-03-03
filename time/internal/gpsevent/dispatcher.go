package gpsevent

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/app/logfile"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/phctime"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/refclock"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/jclark/satpulse/time/internal/timemsg"
	"github.com/jclark/satpulse/time/internal/ts"
	"golang.org/x/sys/unix"
)

const LogExtension = ".jsonl"

// TimePulsePVTMsgFlags are the PVT message flags when time pulse is enabled.
const TimePulsePVTMsgFlags = gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTimePulseAfter | gpsprot.PVTMsgTAI | gpsprot.PVTMsgLeapSecond | gpsprot.PVTMsgSurvey

// NoTimePulsePVTMsgFlags are the PVT message flags when time pulse is not enabled.
const NoTimePulsePVTMsgFlags = gpsprot.PVTMsgPos | gpsprot.PVTMsgTime | gpsprot.PVTMsgLeapSecond | gpsprot.PVTMsgSurvey

// SetMsgOptions configures the message options suitably for the gpsevent package.
// It disables NMEA (which is slow on 8th gen) and enables the required PVT messages.
func SetMsgOptions(target *gpsprot.ConfigTarget, timePulseEnabled bool) {
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgNone) // Config is very slow on 8-th gen if NMEA is enabled
	if timePulseEnabled {
		target.Opts.PVTMsg = TimePulsePVTMsgFlags
	} else {
		target.Opts.PVTMsg = NoTimePulsePVTMsgFlags
	}
}

type Dispatcher struct {
	gpsprot.DefaultHandler
	pktProcs              map[gpsprot.Tag]gpsprot.PacketProcessor
	obs                   obs.Observer // never nil
	controller            *phcsync.Controller
	rc                    *refclock.ProxyRefClock
	timeMsgBuffer         *timemsg.Buffer
	pvAccum               gpsprot.PVMsgAccum
	ls                    ptime.LeapSecond
	lg                    *slog.Logger
	lf                    logfile.LogFile
	loggedUnknownProtocol bool
	loggedSurveyComplete  bool
	tStart                time.Time
}

func NewDispatcher(lg *slog.Logger, pktProcs map[gpsprot.Tag]gpsprot.PacketProcessor, controller *phcsync.Controller, rc *refclock.ProxyRefClock, ls ptime.LeapSecond, obs obs.Observer, eventLogPath string, tStart time.Time) (*Dispatcher, error) {
	// Always create timeMsgBuffer (useful even without PHC)
	timeMsgBuffer := timemsg.NewBuffer(lg, 5*time.Second, ls, gpsprot.GPS)

	// Inject buffer into controller if controller exists
	if controller != nil {
		controller.SetTimeMsgBuffer(timeMsgBuffer)
	}

	d := Dispatcher{
		pktProcs:      pktProcs,
		controller:    controller,
		rc:            rc,
		timeMsgBuffer: timeMsgBuffer,
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
	err := d.lf.Open(eventLogPath)
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
	_, err = pp.ProcessPacket(pkt.Data, pkt.TRead)
	if err != nil {
		lg.Warn("error processing packet", "err", err, "tag", tag, "data", pkt.Data)
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
	// timestamp events only occur when PHC is available, so d.controller is non-nil
	// Pass monotonic sample to controller (for PHC sync logic)
	d.controller.PulseEdge(phcsync.PulseEdge{
		Timestamp: e.Ts,
		TRead:     e.TReadMono,
	})

	// Pass wallclock sample to sysSample (for chrony)
	d.sysSample(e.TReadWall.PHC.T, e.TReadWall.Sys)

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
func (d *Dispatcher) sysSample(trp ptime.Time, tRead time.Time) {
	// Send refclock sample if in tracking mode
	if d.rc == nil || trp.IsZero() || d.controller.Mode() != phcsync.ModeTracking {
		return
	}
	err := d.rc.Sample(tRead, trp, d.ls)
	if err != nil {
		d.lg.Warn("refclock sample failed", "err", err)
	}
}

func (d *Dispatcher) Time(mt *gpsprot.TimeMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, Time: mt})

	d.timeMsgBuffer.Time(mt, tRead)

	// Notify controller that a time message arrived
	if d.controller != nil {
		d.controller.TimeMessage()
	}
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
	d.pvAccum.PVMsgBundle.FillDerived()
	pv := d.pvAccum.PVMsgBundle // copy before clearing
	d.pvAccum.NavEpoch(msg, tRead)
	d.obs.NavEpochPV(msg, &pv, tRead)
}

func (d *Dispatcher) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, LeapSecond: msg})
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
