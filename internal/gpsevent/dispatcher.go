package gpsevent

import (
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/satpulse/internal/combine"
	"github.com/jclark/satpulse/internal/geopos"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/logfile"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/sse"
	"github.com/jclark/satpulse/internal/ts"
	"github.com/jclark/satpulse/internal/ubx"
	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
	"golang.org/x/sys/unix"
)

const LogExtension = ".jsonl"

type Dispatcher struct {
	gpsprot.DefaultHandler
	sseCh                 chan<- sse.Event
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

func NewDispatcher(lg *slog.Logger, m *mon.Monitor, ls ptime.LeapSecond, phcFlags phc.DriverFlags, pulseWidth time.Duration, sseCh chan<- sse.Event, eventLogPath string) (*Dispatcher, error) {
	pt := combine.PulseType{
		EdgesPerPulse: phcFlags.Edges(),
		PulseWidth:    pulseWidth,
	}
	ccfg := combine.Config{}
	ccfg.SetDefault(pt)
	if phcFlags&phc.DriverPoll4Hz != 0 {
		ccfg.PulsePollInterval = time.Second / 4
	}
	combiner, err := combine.NewCombiner(pt, m, lg, ccfg)
	if err != nil {
		return nil, err
	}
	d := Dispatcher{
		cb:     combiner,
		mon:    m,
		ls:     ls,
		lg:     lg,
		sseCh:  sseCh,
		tStart: time.Now(),
	}
	err = d.lf.Open(eventLogPath)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

const tickPeriod = time.Second / 4

func (d *Dispatcher) Run(tsCh <-chan ts.Event, pktCh <-chan scan.Packet) {
	// loop until both channels are closed
	sseCh := d.sseCh
	if sseCh != nil {
		defer close(sseCh)
	}
	// close the monitor before the sseCh, since the monitor uses the sseCh
	defer d.mon.Close()
	ticker := time.NewTicker(tickPeriod)
	defer ticker.Stop()
	// Use SIGHUP as a signal to reopen the log file (e.g. after log rotation)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, unix.SIGHUP)
	lg := d.lg
	defer d.lf.Close(d.lg)
	lg.Debug("event dispatcher goroutine started")

	nSkipped := 0
	// give a warning if we haven't received a timestamp by the time this fires
	firstTsDeadline := time.After(time.Second * 2)

	for tsCh != nil || pktCh != nil {
		select {
		case e, ok := <-tsCh:
			if ok {
				if firstTsDeadline != nil {
					lg.Info("successfully received a external timestamp from the PTP hardware clock")
					firstTsDeadline = nil
				}
				if e.Ts.Era == ts.StaleEra {
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
			} else {
				lg.Debug("timestamp channel of event dispatcher goroutine was closed")
				tsCh = nil
			}
		case pkt, ok := <-pktCh:
			if ok {
				d.handlePacket(pkt)
			} else {
				lg.Debug("packet channel of event dispatcher goroutine was closed")
				pktCh = nil
			}
		case t := <-ticker.C:
			d.mon.Tick(t)
		case <-firstTsDeadline:
			lg.Warn("no PTP hardware clock external timestamps being received")
			firstTsDeadline = nil
		case <-sig:
			d.mon.ReopenLog()
			d.lf.Reopen(d.lg)
		}
	}
}

func (d *Dispatcher) handlePacket(pkt scan.Packet) {
	lg := d.lg
	switch pkt.Kind {
	case scan.NMEA:
		err := nmea.ProcessPacket(pkt.Data, pkt.TRead, d, nil)
		if err != nil {
			lg.Error("failed to parse NMEA message", "err", err)
		}
	case scan.UBX:
		err := ubx.ProcessPacket(pkt.Data, pkt.TRead, d, d)
		if err != nil {
			lg.Error("failed to parse UBX message", "err", err)
		}
	case scan.Invalid:
		if !d.loggedUnknownProtocol {
			lg.Info("received data from GPS in unknown protocol (serial communication problem?)", "len", len(pkt.Data), "data", pkt.Data)
			d.loggedUnknownProtocol = true
		}
	}
}

type LogEvent struct {
	T          time.Time              `json:"t"`
	Nanos      time.Duration          `json:"nanos"`
	Timestamp  *Timestamp             `json:"timestamp,omitempty"`
	Time       *gpsprot.TimeMsg       `json:"time,omitempty"`
	Survey     *gpsprot.SurveyMsg     `json:"survey,omitempty"`
	LeapSecond *gpsprot.LeapSecondMsg `json:"leapSecond,omitempty"`
}

type Timestamp struct {
	T     ptime.Time    `json:"t"`
	Era   ptime.Era     `json:"era"`
	Delay time.Duration `json:"delay,omitempty"`
}

func (d *Dispatcher) timestamp(e ts.Event) {
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

type TimeSSE struct {
	UTC string `json:"utc"`
	TAI int64  `json:"tai"`
}

func (d *Dispatcher) Time(mt *gpsprot.TimeMsg, tRead time.Time) {
	d.logEvent(LogEvent{T: tRead, Time: mt})
	var sec ptime.Time
	if !mt.TAITime.IsZero() {
		sec = mt.TAITime
	} else {
		u := mt.UTCTime
		if u == nil {
			return
		}
		sec = d.ls.UTCtoTime(*u)
		d.lg.Debug("computed TAI time from UTC time", "tai", sec)
	}
	secRnd := sec.Round(time.Second)
	d.cb.TimeMsg(secRnd, tRead, mt.PulseOffset, mt.Ref)
	if mt.Ref != gpsprot.PrePulse && secRnd > d.lastTime {
		d.sendSSE("time", TimeSSE{
			UTC: d.ls.FormatTime(secRnd),
			TAI: int64(secRnd) / 1e9,
		})
		d.lastTime = secRnd
	}
}

// SurveySSE is the type of the SSE for survey progress.
type SurveySSE struct {
	X          float64    `json:"x"`
	Y          float64    `json:"y"`
	Z          float64    `json:"z"`
	Accuracy   float64    `json:"accuracy"`
	Alt        float64    `json:"alt"`
	LatLon     [2]float64 `json:"latLon,omitempty"`
	ObsTime    uint32     `json:"obsTime"`
	ObsCount   uint32     `json:"obsCount"`
	InProgress bool       `json:"inProgress"`
	Valid      bool       `json:"valid"`
}

func (d *Dispatcher) Survey(m *gpsprot.SurveyMsg, tRead time.Time) {
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
	ecef := geopos.ECEF{}
	for i := range ecef {
		ecef[i] = m.Position[i].Meters()
	}
	lla, err := geopos.WGS84.ECEFtoLLA(ecef)
	if err != nil {
		lla.Lat = math.NaN()
		lla.Lon = math.NaN()
		lla.Alt = math.NaN()
	}
	d.sendSSE("survey", SurveySSE{
		X:          ecef[0],
		Y:          ecef[1],
		Z:          ecef[2],
		Accuracy:   m.Accuracy.Meters(),
		LatLon:     [2]float64{lla.Lat, lla.Lon},
		Alt:        lla.Alt,
		ObsTime:    uint32(m.ObsTime / time.Second),
		ObsCount:   m.ObsCount,
		InProgress: m.InProgress,
		Valid:      m.Valid,
	})
}

func (d *Dispatcher) sendSSE(name string, data any) {
	if d.sseCh == nil {
		return
	}
	event, err := sse.Make(name, data)
	if err != nil {
		d.lg.Error("failed to create SSE event", "name", name, "err", err)
	} else {
		d.sseCh <- event
	}
}

func (d *Dispatcher) LeapSecond(msg *gpsprot.LeapSecondMsg, _ time.Time) {
	if msg.OffChangeTime <= d.ls.OffChangeTime {
		return
	}
	d.ls = msg.LeapSecond
	d.mon.SetLeapSecond(d.ls)
}

func (d *Dispatcher) UBX(msg ubxbin.Msg, tRead time.Time) {
	d.lg.Debug("unused UBX message", "msg", msg)
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
