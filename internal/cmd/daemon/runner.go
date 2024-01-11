package daemon

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/combine"
	"github.com/jclark/satpulse/internal/geopos"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/servo"
	"github.com/jclark/satpulse/internal/sse"
	"github.com/jclark/satpulse/internal/ubx"
	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
)

type SyncRunner struct {
	gpsprot.DefaultHandler
	inLog                 io.Writer
	sseCh                 chan<- sse.Event
	cb                    *combine.Combiner
	m                     *mon.Monitor
	ls                    ptime.LeapSecond
	lg                    *slog.Logger
	lastTime              ptime.Time
	loggedUnknownProtocol bool
}

func NewSyncRunner(lg *slog.Logger, clk *phc.Clock, phcFlags phc.DriverFlags, pulseWidth time.Duration, cfg *Config, gm *mon.Grandmaster, ntp *mon.ProxyRefClock, sseCh chan<- sse.Event, inLog io.Writer) (*SyncRunner, error) {
	servo, err := servo.New(clk, lg, sseCh)
	if err != nil {
		return nil, err
	}
	ls := cfg.LeapSecond.leapSecond()
	m := mon.NewMonitor(ls, gm, ntp, lg)
	pt := combine.PulseType{
		EdgesPerPulse: phcFlags.Edges(),
		PulseWidth:    pulseWidth,
	}
	ccfg := combine.Config{}
	ccfg.SetDefault(pt)
	if phcFlags&phc.DriverPoll4Hz != 0 {
		ccfg.PulsePollInterval = time.Second / 4
	}
	combiner, err := combine.NewCombiner(pt, combine.MultiSampler(servo, m), lg, ccfg)
	if err != nil {
		return nil, err
	}
	s := SyncRunner{
		cb:    combiner,
		m:     m,
		ls:    ls,
		lg:    lg,
		sseCh: sseCh,
		inLog: inLog,
	}
	return &s, nil
}

const tickPeriod = time.Second / 4

func (s *SyncRunner) run(tsCh <-chan phc.TsEvent, pktCh <-chan scan.Packet) {
	// loop until both channels are closed
	sseCh := s.sseCh
	if sseCh != nil {
		defer close(sseCh)
	}
	ticker := time.NewTicker(tickPeriod)
	defer ticker.Stop()
	lg := s.lg
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
						lg.Debug("PHC timestamp delay", "delay", delay)
					}
					// Call PulseEdge before SysSample, because the former might change the sync status
					s.cb.PulseEdge(e.Ts, e.TRead)
					if !trp.IsZero() {
						s.m.SysSample(trp, e.TRead)
					}
				}
			} else {
				lg.Debug("timestamp channel of sync worker goroutine was closed")
				tsCh = nil
			}
		case pkt, ok := <-pktCh:
			if ok {
				s.handlePacket(pkt)
			} else {
				lg.Debug("packet channel of sync worker goroutine was closed")
				pktCh = nil
			}
		case t := <-ticker.C:
			s.m.Tick(t)
		}
	}
}

type TimeEvent struct {
	UTC string `json:"utc"`
	TAI int64  `json:"tai"`
}

func (s *SyncRunner) handlePacket(pkt scan.Packet) {
	lg := s.lg
	switch pkt.Kind {
	case scan.NMEA:
		err := nmea.ProcessPacket(pkt.Data, pkt.TRead, s, nil)
		if err != nil {
			lg.Error("failed to parse NMEA message", "err", err)
		}
	case scan.UBX:
		err := ubx.ProcessPacket(pkt.Data, pkt.TRead, s, s)
		if err != nil {
			lg.Error("failed to parse UBX message", "err", err)
		}
	case scan.Invalid:
		if !s.loggedUnknownProtocol {
			lg.Info("received data from GPS in unknown protocol (serial communication problem?)", "len", len(pkt.Data), "data", pkt.Data)
			s.loggedUnknownProtocol = true
		}
	}
}

func (s *SyncRunner) Time(mt *gpsprot.TimeMsg, tRead time.Time) {
	if s.inLog != nil {
		bytes, err := json.Marshal(mt)
		if err != nil {
			s.lg.Info("failed to convert Time message to JSON", "err", err)
		} else {
			bytes = append(bytes, '\n')
			_, err = s.inLog.Write(bytes)
			if err != nil {
				s.lg.Info("failed to write to input log", "err", err)
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
		sec = s.ls.UTCtoTime(*u)
		s.lg.Debug("computed TAI time from UTC time", "tai", sec)
	}
	secRnd := sec.Round(time.Second)
	s.cb.TimeMsg(secRnd, tRead, mt.PulseOffset, mt.Ref)
	if mt.Ref != gpsprot.PrePulse && secRnd > s.lastTime {
		s.sendEvent("time", TimeEvent{
			UTC: s.ls.FormatTime(secRnd),
			TAI: int64(secRnd) / 1e9,
		})
		s.lastTime = secRnd
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

func (s *SyncRunner) Survey(m *gpsprot.SurveyMsg, _ time.Time) {
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
	s.sendEvent("survey", SurveyEvent{
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

func (s *SyncRunner) sendEvent(name string, data any) {
	if s.sseCh == nil {
		return
	}
	event, err := sse.Make(name, data)
	if err != nil {
		s.lg.Error("failed to create SSE event", "name", name, "err", err)
	} else {
		s.sseCh <- event
	}
}

func (s *SyncRunner) LeapSecond(msg *gpsprot.LeapSecondMsg, _ time.Time) {
	if msg.OffChangeTime <= s.ls.OffChangeTime {
		return
	}
	s.ls = msg.LeapSecond
	s.m.SetLeapSecond(s.ls)
}

func (s *SyncRunner) UBX(msg ubxbin.Msg, tRead time.Time) {
	s.lg.Debug("unused UBX message", "msg", msg)
}
