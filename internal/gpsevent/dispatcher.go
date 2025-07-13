package gpsevent

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/satpulse/internal/combine"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/logfile"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/ts"
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
	cb                    *combine.Combiner
	mon                   *mon.Monitor
	ls                    ptime.LeapSecond
	lg                    *slog.Logger
	lf                    logfile.LogFile
	lastTime              ptime.Time
	loggedUnknownProtocol bool
	loggedSurveyComplete  bool
	tStart                time.Time
}

func NewDispatcher(lg *slog.Logger, pktProcs map[gpsprot.Tag]gpsprot.PacketProcessor, m *mon.Monitor, ls ptime.LeapSecond, phcFlags phc.DriverFlags, pulseWidth time.Duration, obs obs.Observer, eventLogPath string, tStart time.Time) (*Dispatcher, error) {
	var combiner *combine.Combiner
	if m != nil {
		pt := combine.PulseType{
			EdgesPerPulse: phcFlags.Edges(),
			PulseWidth:    pulseWidth,
		}
		ccfg := combine.Config{}
		ccfg.SetDefault(pt)
		if phcFlags&phc.DriverPoll4Hz != 0 {
			ccfg.PulsePollInterval = time.Second / 4
		}
		var err error
		combiner, err = combine.NewCombiner(pt, m, lg, ccfg)
		if err != nil {
			return nil, err
		}
	}
	d := Dispatcher{
		pktProcs: pktProcs,
		cb:       combiner,
		mon:      m,
		ls:       ls,
		lg:       lg,
		obs:      obs,
		tStart:   tStart,
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
	// close the monitor before the observer, since the monitor uses the observer
	if d.mon != nil {
		defer d.mon.Close()
	}
	var ticker *time.Ticker
	var tickerCh <-chan time.Time
	var firstTsDeadline <-chan time.Time
	if d.mon != nil {
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
				d.mon.Pause()
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
			d.mon.Tick(t)
		case <-firstTsDeadline:
			lg.Warn("no PTP hardware clock external timestamps being received")
			firstTsDeadline = nil
		case <-sig:
			if d.mon != nil {
				d.mon.ReopenLog()
			}
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
		lg.Error("error processing packet", "err", err, "tag", tag, "data", pkt.Data)
	}
}

type LogEvent struct {
	T          time.Time              `json:"t"`
	Nanos      time.Duration          `json:"nanos"`
	Timestamp  *Timestamp             `json:"timestamp,omitempty"`
	Time       *gpsprot.TimeMsg       `json:"time,omitempty"`
	Survey     *gpsprot.SurveyMsg     `json:"survey,omitempty"`
	LeapSecond *gpsprot.LeapSecondMsg `json:"leapSecond,omitempty"`
	Satellites *gpsprot.SatellitesMsg `json:"satellites,omitempty"`
}

type Timestamp struct {
	T     ptime.Time    `json:"t"`
	Era   ptime.Era     `json:"era"`
	Delay time.Duration `json:"delay,omitempty"`
}

func (d *Dispatcher) timestamp(e ts.Event) {
	// timestamp events only occur when PHC is available, so d.mon and d.cb are non-nil
	var delay time.Duration
	trp := e.TReadPHC.T
	if !trp.IsZero() && !e.TReadPHC.Era.Uncertain() && e.TReadPHC.Era == e.Ts.Era {
		delay = trp.Sub(e.Ts.T)
		if delay == 0 {
			d.lg.Info("unexpected zero timestamp delay", "ts", e.Ts)
		}
	} else {
		d.lg.Info("timestamp delay cannot be determined", "ts", e.Ts, "tReadPHC", e.TReadPHC)
	}
	d.logEvent(LogEvent{T: e.TRead, Timestamp: &Timestamp{T: e.Ts.T, Era: e.Ts.Era, Delay: delay}})
	// Call PulseEdge before SysSample, because the former might change the sync status
	d.cb.PulseEdge(e.Ts, e.TRead, delay)
	if !trp.IsZero() {
		d.mon.SysSample(trp, e.TRead)
	}
}


func (d *Dispatcher) Time(mt *gpsprot.TimeMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, Time: mt})
	sec, ok := mt.ComputeTAITime(d.ls)
	if !ok {
		return
	}
	if mt.UTCTime != nil && mt.TAITime.IsZero() {
		d.lg.Debug("computed TAI time from UTC time", "tai", sec)
	}
	secRnd := sec.Round(time.Second)
	if d.cb != nil {
		d.cb.TimeMsg(secRnd, tRead, mt.PulseOffset, mt.Ref)
	}
	if mt.Ref != gpsprot.PrePulse && secRnd > d.lastTime {
		d.lastTime = secRnd
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


func (d *Dispatcher) Satellites(msg *gpsprot.SatellitesMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, Satellites: msg})
}


func (d *Dispatcher) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, LeapSecond: msg})
	if msg.UpdateLeapSecond(&d.ls) && d.mon != nil {
		d.mon.SetLeapSecond(d.ls)
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
