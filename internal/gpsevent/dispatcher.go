package gpsevent

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/satpulse/internal/combine"
	"github.com/jclark/satpulse/internal/geopos"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/sse"
	"github.com/jclark/satpulse/internal/ubx"
	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
	"golang.org/x/sys/unix"
)

type Dispatcher struct {
	gpsprot.DefaultHandler
	inLog                 io.Writer
	sseCh                 chan<- sse.Event
	cb                    *combine.Combiner
	mon                   *mon.Monitor
	ls                    ptime.LeapSecond
	lg                    *slog.Logger
	lastTime              ptime.Time
	loggedUnknownProtocol bool
}

func NewDispatcher(lg *slog.Logger, m *mon.Monitor, ls ptime.LeapSecond, clk *phc.Clock, phcFlags phc.DriverFlags, pulseWidth time.Duration, sseCh chan<- sse.Event, inLog io.Writer) (*Dispatcher, error) {
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
		cb:    combiner,
		mon:   m,
		ls:    ls,
		lg:    lg,
		sseCh: sseCh,
		inLog: inLog,
	}
	return &d, nil
}

const tickPeriod = time.Second / 4

func (d *Dispatcher) Run(tsCh <-chan phc.TsEvent, pktCh <-chan scan.Packet) {
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
	lg.Debug("sync worker goroutine started")

	nSkipped := 0

	for tsCh != nil || pktCh != nil {
		select {
		case e, ok := <-tsCh:
			if ok {
				if e.Err != nil {
					lg.Info("error from PTP hardware clock timestamp channel", "err", e.Err)
					if e.Ts.T.IsZero() {
						continue
					}
				}
				if e.Ts.Era == phc.StaleEra {
					if nSkipped == 0 {
						lg.Debug("detected a stale PTP hardware clock timestamp", "t", e.Ts.T)
					}
					nSkipped++
				} else {
					if nSkipped > 0 {
						lg.Info("skipped stale PTP hardware clock timestamps", "n", nSkipped)
						nSkipped = 0
					}
					var delay time.Duration
					trp := e.TReadPHC.T
					if !trp.IsZero() && !e.TReadPHC.Era.Uncertain() && e.TReadPHC.Era == e.Ts.Era {
						delay = trp.Sub(e.Ts.T)
					}
					// Call PulseEdge before SysSample, because the former might change the sync status
					d.cb.PulseEdge(e.Ts, e.TRead, delay)
					if !trp.IsZero() {
						d.mon.SysSample(trp, e.TRead)
					}
				}
			} else {
				lg.Debug("timestamp channel of sync worker goroutine was closed")
				tsCh = nil
			}
		case pkt, ok := <-pktCh:
			if ok {
				d.handlePacket(pkt)
			} else {
				lg.Debug("packet channel of sync worker goroutine was closed")
				pktCh = nil
			}
		case t := <-ticker.C:
			d.mon.Tick(t)
		case <-sig:
			d.mon.ReopenLog()
		}
	}
}

type TimeEvent struct {
	UTC string `json:"utc"`
	TAI int64  `json:"tai"`
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

func (d *Dispatcher) Time(mt *gpsprot.TimeMsg, tRead time.Time) {
	if d.inLog != nil {
		bytes, err := json.Marshal(mt)
		if err != nil {
			d.lg.Info("failed to convert Time message to JSON", "err", err)
		} else {
			bytes = append(bytes, '\n')
			_, err = d.inLog.Write(bytes)
			if err != nil {
				d.lg.Info("failed to write to input log", "err", err)
			}
		}
	}
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
		d.sendEvent("time", TimeEvent{
			UTC: d.ls.FormatTime(secRnd),
			TAI: int64(secRnd) / 1e9,
		})
		d.lastTime = secRnd
	}
}

type SurveyEvent struct {
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

func (d *Dispatcher) Survey(m *gpsprot.SurveyMsg, _ time.Time) {
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
	d.sendEvent("survey", SurveyEvent{
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

func (d *Dispatcher) sendEvent(name string, data any) {
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
