package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/mon"
	"github.com/jclark/gps4ptp/internal/nmea"
	"github.com/jclark/gps4ptp/internal/phc"
	"github.com/jclark/gps4ptp/internal/ptime"
	"github.com/jclark/gps4ptp/internal/scan"
	"github.com/jclark/gps4ptp/internal/sse"
	"github.com/jclark/gps4ptp/internal/tsync"
	"github.com/jclark/gps4ptp/internal/ubx"
)

type SyncRunner struct {
	gpsmsg.DefaultHandler

	sseCh    chan<- sse.Event
	corr     *tsync.Correlator
	m        *mon.Monitor
	ls       ptime.LeapSecond
	lg       *slog.Logger
	lastTime ptime.Time
}

func NewSyncRunner(lg *slog.Logger, clk *phc.Clock, cfg *Config, guCh chan<- mon.GrandmasterUpdateRequest, sseCh chan<- sse.Event) (*SyncRunner, error) {
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
	}
	return &s, nil
}

const tickPeriod = time.Second / 4

func (s *SyncRunner) run(tsCh <-chan phc.TsEvent, fCh <-chan scan.Frame) {
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

	for tsCh != nil || fCh != nil {
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
		case f, ok := <-fCh:
			if ok {
				s.handleFrame(f)
			} else {
				lg.Debug("frame channel of sync worker goroutine was closed")
				fCh = nil
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

func (s *SyncRunner) handleFrame(f scan.Frame) {
	// TODO: handle leapsecond messages
	lg := s.lg
	switch f.Kind {
	case scan.NMEA:
		err := nmea.ProcessFrameData(f.Data, f.TRead, s, s)
		if err != nil {
			lg.Error("failed to parse NMEA message", "err", err)
		}
	case scan.UBX:
		err := ubx.ProcessFrameData(f.Data, f.TRead, s, nil)
		if err != nil {
			lg.Error("failed to parse UBX message", "err", err)
		}
	case scan.Invalid:
		lg.Info("received data from GPS in unknown protocol (serial communication problem?)", "len", len(f.Data), "data", f.Data)
	}
}

func (s *SyncRunner) Time(mt *gpsmsg.Time, tRead time.Time) {
	if false {
		bytes, err := json.Marshal(mt)
		if err == nil {
			fmt.Println(string(bytes))
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
	if mt.PrecedesPulse {
		s.corr.PulseOffset(secRnd, tRead, mt.PulseOffset)
	} else if secRnd > s.lastTime {
		s.corr.GPSTime(secRnd, tRead)
		if s.sseCh != nil {
			te := TimeEvent{
				UTC: s.ls.FormatTime(secRnd),
				TAI: int64(secRnd) / 1e9,
			}
			event, err := sse.Make("time", te)
			if err != nil {
				s.lg.Error("failed to create SSE event", "err", err)
			} else {
				s.sseCh <- event
			}
		}
		s.lastTime = secRnd
	}
}

func (s *SyncRunner) NMEA(msg *nmea.Message, tRead time.Time) {
	nmeaLog(s.lg, msg)
}
