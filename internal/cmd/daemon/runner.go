package daemon

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/geopos"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/sse"
	"github.com/jclark/satpulse/internal/tsync"
	"github.com/jclark/satpulse/internal/ubx"
	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
)

type SyncRunner struct {
	gpsprot.DefaultHandler
	inLog    io.Writer
	sseCh    chan<- sse.Event
	corr     *tsync.Correlator
	m        *mon.Monitor
	ls       ptime.LeapSecond
	lg       *slog.Logger
	lastTime ptime.Time
}

func NewSyncRunner(lg *slog.Logger, clk *phc.Clock, cfg *Config, guCh chan<- mon.GrandmasterUpdateRequest, sseCh chan<- sse.Event, inLog io.Writer) (*SyncRunner, error) {
	servo, err := tsync.NewServo(clk, lg, sseCh)
	if err != nil {
		return nil, err
	}
	ls := cfg.LeapSecond.leapSecond()
	m := mon.NewMonitor(ls, guCh, lg)
	s := SyncRunner{
		corr:  tsync.NewCorrelator(tsync.MultiSampler(servo, m), lg),
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
				if e.Era == phc.StaleEra {
					if nSkipped == 0 {
						lg.Debug("detected a stale PTP hardware clock timestamp", "t", e.T)
					}
					nSkipped++
				} else {
					if nSkipped > 0 {
						lg.Info("skipped stale PTP hardware clock timestamps", "n", nSkipped)
						nSkipped = 0
					}
					s.corr.PulseEdge(e.ClockTime, e.TRead)
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
		lg.Info("received data from GPS in unknown protocol (serial communication problem?)", "len", len(pkt.Data), "data", pkt.Data)
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
	if mt.Ref == gpsprot.NextPulse {
		s.corr.PulseOffset(secRnd, tRead, mt.PulseOffset)
	} else if secRnd > s.lastTime {
		s.corr.GPSTime(secRnd, tRead)
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
