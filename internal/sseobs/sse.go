package sseobs

import (
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/geopos"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/sse"
)

// SSE data structures - copied exactly from existing code

// TimeSSE is the type of the SSE for time events
type TimeSSE struct {
	UTC string `json:"utc"`
	TAI int64  `json:"tai"`
}

// SurveySSE is the type of the SSE for survey progress
type SurveySSE struct {
	X          float64     `json:"x"`
	Y          float64     `json:"y"`
	Z          float64     `json:"z"`
	Accuracy   float64     `json:"accuracy"`
	Alt        *float64    `json:"alt,omitempty"`
	LatLon     *[2]float64 `json:"latLon,omitempty"`
	ObsTime    uint32      `json:"obsTime"`
	ObsCount   uint32      `json:"obsCount"`
	InProgress bool        `json:"inProgress"`
	Valid      bool        `json:"valid"`
}

// SatellitesSSE is the type of the SSE for satellite events
type SatellitesSSE struct {
	SVs []gpsprot.SVInfo `json:"svs"`
}

// SampleEvent is the type of the SSE for PHC sample events
type SampleEvent struct {
	Offset            float64 `json:"offset"` // in nanoseconds
	Freq              float64 `json:"freq"`   // in parts per billion
	StepCount         uint32  `json:"stepCount"`
	StepCountChanging bool    `json:"stepCountChanging,omitempty"`
	Outlier           bool    `json:"outlier,omitempty"`
	SyncState         string  `json:"syncState"`
}

// SSEObserver implements obs.Observer for Server-Sent Events
type SSEObserver struct {
	obs.DefaultObserver
	sseCh    chan<- sse.Event
	lg       *slog.Logger
	lastTime ptime.Time
	ls       ptime.LeapSecond
}

// New creates a new SSE observer with the provided channel and leap second.
// The sseCh parameter must be non-nil.
func New(sseCh chan<- sse.Event, ls ptime.LeapSecond, lg *slog.Logger) *SSEObserver {
	if sseCh == nil {
		panic("sseCh must be non-nil")
	}
	return &SSEObserver{
		sseCh: sseCh,
		ls:    ls,
		lg:    lg,
	}
}

// SetLeapSecond updates the leap second used for time formatting
func (o *SSEObserver) SetLeapSecond(ls ptime.LeapSecond) {
	o.ls = ls
}

// Release implements Observer - closes the SSE channel
func (o *SSEObserver) Release() {
	close(o.sseCh)
}

// Sample implements mon.Sampler - generates PHC sample SSE events (copied exactly from monitor.go)
func (o *SSEObserver) Sample(data mon.SampleData) {
	stepCount, changing := data.Era.StepCount()
	event, err := sse.Make("phc", &SampleEvent{
		Offset:            float64(data.Offset),
		StepCount:         uint32(stepCount),
		StepCountChanging: changing,
		Freq:              data.Freq,
		Outlier:           data.Kind == mon.SampleOutlier,
		SyncState:         data.SyncState.String(),
	})
	if err != nil {
		o.lg.Error("error creating sample event", "err", err)
		return
	}
	o.sseCh <- event
}

// Time implements gpsprot.MsgHandler - generates time SSE events (copied exactly from dispatcher.go)
func (o *SSEObserver) Time(mt *gpsprot.TimeMsg, tRead time.Time) {
	if mt.Ref == gpsprot.PrePulse {
		return
	}
	sec, ok := mt.ComputeTAITime(o.ls)
	if !ok {
		return
	}
	secRnd := sec.Round(time.Second)
	if secRnd <= o.lastTime {
		return
	}
	o.sendSSE("time", TimeSSE{
		UTC: o.ls.FormatTime(secRnd),
		TAI: int64(secRnd) / 1e9,
	})
	o.lastTime = secRnd
}

// Survey implements gpsprot.MsgHandler - generates survey SSE events (copied exactly from dispatcher.go)
func (o *SSEObserver) Survey(m *gpsprot.SurveyMsg, tRead time.Time) {
	ecef := geopos.ECEF{}
	for i := range ecef {
		ecef[i] = m.Position[i].Meters()
	}
	sse := SurveySSE{
		X:          ecef[0],
		Y:          ecef[1],
		Z:          ecef[2],
		Accuracy:   m.Accuracy.Meters(),
		ObsTime:    uint32(m.ObsTime / time.Second),
		ObsCount:   m.ObsCount,
		InProgress: m.InProgress,
		Valid:      m.Valid,
	}
	llh, err := geopos.WGS84.ECEFtoLLH(ecef)
	if err == nil {
		latLon := [2]float64{llh.Lat, llh.Lon}
		sse.LatLon = &latLon
		h := llh.Height
		sse.Alt = &h
	}
	o.sendSSE("survey", sse)
}

// Satellites implements gpsprot.MsgHandler - generates satellites SSE events (copied exactly from dispatcher.go)
func (o *SSEObserver) Satellites(msg *gpsprot.SatellitesMsg, tRead time.Time) {
	o.sendSSE("satellites", SatellitesSSE{SVs: msg.SVs})
}

// sendSSE is a helper method to send SSE events (copied exactly from dispatcher.go)
func (o *SSEObserver) sendSSE(name string, data any) {
	event, err := sse.Make(name, data)
	if err != nil {
		o.lg.Error("failed to create SSE event", "name", name, "err", err)
	} else {
		o.sseCh <- event
	}
}