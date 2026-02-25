package sseobs

import (
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/lib/sse"
)

// SSE data structures

// InitSSE is the type of the SSE for the initialization event
type InitSSE struct {
	Receiver *gpsprot.ReceiverInfo `json:"receiver,omitempty"`
}

// TimeSSE is the type of the SSE for time events
type TimeSSE struct {
	UTC string `json:"utc"`
	TAI int64  `json:"tai"`
}

// SurveySSE is the type of the SSE for survey progress
type SurveySSE struct {
	X          gpsprot.Length             `json:"x,omitzero"`
	Y          gpsprot.Length             `json:"y,omitzero"`
	Z          gpsprot.Length             `json:"z,omitzero"`
	Accuracy   gpsprot.Length             `json:"accuracy"`
	Alt        opt.Val[gpsprot.Length]    `json:"alt,omitzero"`
	LatLon     opt.Val[[2]gpsprot.Angle]  `json:"latLon,omitzero"`
	ObsTime    gpsprot.Duration            `json:"obsTime"`
	ObsCount   uint32                     `json:"obsCount"`
	InProgress bool                       `json:"inProgress"`
	Valid      bool                       `json:"valid"`
}

// SatellitesSSE is the type of the SSE for satellite events
type SatellitesSSE struct {
	SVs []gpsprot.SVInfo `json:"svs"`
}

// SampleSSE is the type of the SSE for PHC sample events
type SampleSSE struct {
	Offset            float64 `json:"offset"` // in nanoseconds
	Freq              float64 `json:"freq"`   // in parts per billion
	StepCount         uint32  `json:"stepCount"`
	StepCountChanging bool    `json:"stepCountChanging,omitempty"`
	Outlier           bool    `json:"outlier,omitempty"`
	SyncState         string  `json:"syncState"`
}

// PosVelSSE is the SSE event data for position and velocity.
// Emitted once per navigation epoch. Fields are omitted when
// no data is available for that category.
type PosVelSSE struct {
	// Geodetic position
	LatLon    opt.Val[[2]gpsprot.Angle]  `json:"latLon,omitzero"`
	Height    opt.Val[gpsprot.Length]     `json:"height,omitzero"`
	HeightMSL opt.Val[gpsprot.Length]     `json:"heightMSL,omitzero"`
	PosECEF   opt.Val[[3]gpsprot.Length]  `json:"posECEF,omitzero"`
	// Velocity
	GroundSpeed opt.Val[gpsprot.Speed]    `json:"groundSpeed,omitzero"`
	Speed3D     opt.Val[gpsprot.Speed]    `json:"speed3D,omitzero"`
	Course      opt.Val[gpsprot.Angle]    `json:"course,omitzero"`
	VelNED      opt.Val[[3]gpsprot.Speed] `json:"velNED,omitzero"`
	VelECEF     opt.Val[[3]gpsprot.Speed] `json:"velECEF,omitzero"`
}

// QualitySSE is the SSE event data for solution quality metadata.
// Emitted once per navigation epoch.
type QualitySSE struct {
	Fix         []string         `json:"fix"`
	Corrections gpsprot.CorrKind `json:"corrections,omitzero"`
	// Accuracy estimates
	AccHor         opt.Val[gpsprot.Length] `json:"accHor,omitzero"`
	AccVert        opt.Val[gpsprot.Length] `json:"accVert,omitzero"`
	AccPos         opt.Val[gpsprot.Length] `json:"accPos,omitzero"`
	AccSpeed       opt.Val[gpsprot.Speed]  `json:"accSpeed,omitzero"`
	AccGroundSpeed opt.Val[gpsprot.Speed]  `json:"accGroundSpeed,omitzero"`
	AccCourse      opt.Val[gpsprot.Angle]  `json:"accCourse,omitzero"`
	// Dilution of precision
	GDOP opt.Val[float64] `json:"gdop,omitzero"`
	PDOP opt.Val[float64] `json:"pdop,omitzero"`
	HDOP opt.Val[float64] `json:"hdop,omitzero"`
	VDOP opt.Val[float64] `json:"vdop,omitzero"`
	TDOP opt.Val[float64] `json:"tdop,omitzero"`
	// Satellite counts
	NumSVUsed    opt.Val[uint16] `json:"numSVUsed,omitzero"`
	NumSVTracked opt.Val[uint16] `json:"numSVTracked,omitzero"`
	// Signals used in the solution
	SignalsUsed gpsprot.SignalSet `json:"signalsUsed,omitzero"`
	// Correction metadata
	DiffAge       opt.Val[float64] `json:"diffAge,omitzero"`
	RTCMRefBaseID opt.Val[uint16]  `json:"rtcmRefBaseID,omitzero"`
}

// SSEObserver implements obs.Observer for Server-Sent Events
type SSEObserver struct {
	gpsprot.PVMsgAccum
	sseCh      chan<- sse.Event
	lg         *slog.Logger
	ls         ptime.LeapSecond
	timeTicker gpsprot.TimeTicker
	initEvent  sse.Event
}

// New creates a new SSE observer with the provided channel and leap second.
// The sseCh parameter must be non-nil.
func New(sseCh chan<- sse.Event, ls ptime.LeapSecond, lg *slog.Logger, cfgResult *gpscfg.Result) *SSEObserver {
	if sseCh == nil {
		panic("sseCh must be non-nil")
	}
	initEvent, err := sse.Make("init", InitSSE{
		Receiver: cfgResult.ReceiverInfo,
	})
	if err != nil {
		lg.Error("failed to create SSE event", "name", "init", "err", err)
	}
	o := &SSEObserver{
		sseCh:     sseCh,
		ls:        ls,
		lg:        lg,
		initEvent: initEvent,
	}
	o.timeTicker = *gpsprot.NewTimeTicker(&timeHandler{obs: o}, ls)
	return o
}

func (o *SSEObserver) InitEvent() sse.Event {
	return o.initEvent
}

// SetLeapSecond updates the leap second used for time formatting
func (o *SSEObserver) SetLeapSecond(ls ptime.LeapSecond) {
	o.ls = ls
	o.timeTicker.SetLeapSecond(ls)
}

// Release implements Observer - closes the SSE channel
func (o *SSEObserver) Release() {
	close(o.sseCh)
}

// Sample implements phcsync.Sampler - generates PHC sample SSE events
func (o *SSEObserver) Sample(data phcsync.Sample) {
	stepCount, changing := data.Era.StepCount()
	event := SampleSSE{
		Offset:            float64(data.Offset),
		StepCount:         uint32(stepCount),
		StepCountChanging: changing,
		Freq:              data.Freq,
		Outlier:           data.Kind == phcsync.SampleOutlier,
		SyncState:         data.Mode.String(),
	}
	o.sendSSE("phc", event)
}

// Time routes raw TimeMsgs through the TimeTicker for per-epoch filling.
func (o *SSEObserver) Time(mt *gpsprot.TimeMsg, tRead time.Time) {
	o.timeTicker.Time(mt, tRead)
}

// timeHandler is the downstream target for TimeTicker.
// It receives filled TimeMsgs and generates SSE events.
type timeHandler struct {
	gpsprot.DefaultHandler
	obs *SSEObserver
}

// Time generates a time SSE event from a filled TimeMsg.
// Only emits when the time is an integral second.
func (h *timeHandler) Time(mt *gpsprot.TimeMsg, _ time.Time) {
	tai := mt.TAITime
	if tai.IsZero() {
		return
	}
	if tai.Round(time.Second) != tai {
		return
	}
	h.obs.sendSSE("time", TimeSSE{
		UTC: h.obs.ls.FormatTime(tai),
		TAI: int64(tai) / 1e9,
	})
}

// Survey implements gpsprot.MsgHandler - generates survey SSE events (copied exactly from dispatcher.go)
func (o *SSEObserver) Survey(m *gpsprot.SurveyMsg, tRead time.Time) {
	event := SurveySSE{
		Accuracy:   m.Accuracy,
		ObsTime:    m.ObsTime,
		ObsCount:   m.ObsCount,
		InProgress: m.InProgress,
		Valid:      m.Valid,
	}
	if !m.Position.IsZero() {
		event.X = m.Position[0]
		event.Y = m.Position[1]
		event.Z = m.Position[2]
		ecef := geopos.ECEF{
			m.Position[0].Meters(),
			m.Position[1].Meters(),
			m.Position[2].Meters(),
		}
		llh, err := geopos.WGS84.ECEFtoLLH(ecef)
		if err == nil {
			event.LatLon.Set([2]gpsprot.Angle{
				gpsprot.DegreesFromFloat(llh.Lat),
				gpsprot.DegreesFromFloat(llh.Lon),
			})
			event.Alt.Set(gpsprot.Meters(llh.Height))
		}
	}
	o.sendSSE("survey", event)
}

// Satellites implements gpsprot.MsgHandler - generates satellites SSE events
func (o *SSEObserver) Satellites(msg *gpsprot.SatellitesMsg, tRead time.Time) {
	o.sendSSE("satellites", SatellitesSSE{SVs: msg.SVs})
}

// ReopenLog implements Observer as a no-op.
func (o *SSEObserver) ReopenLog() {}

// LeapSecond updates both the local leap second and the TimeTicker's.
func (o *SSEObserver) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	msg.UpdateLeapSecond(&o.ls)
	o.timeTicker.LeapSecond(msg, tRead)
}

// NavEpoch resets the TimeTicker for the next epoch,
// then emits posvel and quality SSE events, and clears the bundle.
func (o *SSEObserver) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	o.timeTicker.NavEpoch(msg, tRead)
	o.PVMsgBundle.FillDerived()
	if pv := buildPosVelSSE(&o.PVMsgBundle); pv != nil {
		o.sendSSE("posvel", pv)
	}
	if q := buildQualitySSE(msg); q != nil {
		o.sendSSE("quality", q)
	}
	o.PVMsgAccum.NavEpoch(msg, tRead)
}

// sendSSE is a helper method to send SSE events
func (o *SSEObserver) sendSSE(name string, data any) {
	event, err := sse.Make(name, data)
	if err != nil {
		o.lg.Error("failed to create SSE event", "name", name, "err", err)
	} else {
		o.sseCh <- event
	}
}

func buildPosVelSSE(b *gpsprot.PVMsgBundle) *PosVelSSE {
	if !b.PosGeo.IsSet() && !b.PosECEF.IsSet() && !b.VelGeo.IsSet() && !b.VelECEF.IsSet() {
		return nil
	}
	var pv PosVelSSE
	if pg := b.PosGeo.Ptr(); pg != nil {
		pv.LatLon.Set(pg.LatLon)
		pv.Height = pg.Height
		pv.HeightMSL = pg.HeightMSL
	}
	if pe := b.PosECEF.Ptr(); pe != nil {
		pv.PosECEF.Set(pe.Pos)
	}
	if vg := b.VelGeo.Ptr(); vg != nil {
		pv.GroundSpeed = vg.GroundSpeed
		pv.Speed3D = vg.Speed3D
		pv.Course = vg.Course
		pv.VelNED = vg.VelNED
	}
	if ve := b.VelECEF.Ptr(); ve != nil {
		pv.VelECEF.Set(ve.Vel)
	}
	return &pv
}

func buildQualitySSE(msg *gpsprot.NavEpochMsg) *QualitySSE {
	fix := buildFixKeywords(msg)
	if fix == nil {
		return nil
	}
	q := &QualitySSE{
		Fix:           fix,
		Corrections:   msg.Correction,
		NumSVUsed:     msg.NumSVUsed,
		NumSVTracked:  msg.NumSVTracked,
		RTCMRefBaseID: msg.RTCMRefBaseID,
	}
	q.AccHor = msg.Acc.Hor
	q.AccVert = msg.Acc.Vert
	q.AccPos = msg.Acc.Pos
	q.AccSpeed = msg.Acc.Speed
	q.AccGroundSpeed = msg.Acc.GroundSpeed
	q.AccCourse = msg.Acc.Course
	q.GDOP = msg.DOP.Geom
	q.PDOP = msg.DOP.Pos
	q.HDOP = msg.DOP.Hor
	q.VDOP = msg.DOP.Vert
	q.TDOP = msg.DOP.Time
	if msg.DiffAge.IsSet() {
		q.DiffAge.Set(msg.DiffAge.Get().Seconds())
	}
	q.SignalsUsed = msg.SignalsUsed
	return q
}

func buildFixKeywords(msg *gpsprot.NavEpochMsg) []string {
	if msg.FixLevel == 0 {
		return nil
	}
	var kw []string
	kw = append(kw, msg.FixLevel.String())
	if msg.FixDim != 0 {
		kw = append(kw, msg.FixDim.String())
	}
	kw = append(kw, msg.AuxSrc.Items()...)
	return kw
}

