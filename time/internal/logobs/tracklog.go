package logobs

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/app/logfile"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/obs"
)

const TrackLogExtension = ".jsonl"

// TrackLogEntry is a single trackpoint with position, DOP, and fix type.
// A zero entry (T == "") indicates no valid position is available.
type TrackLogEntry struct {
	T   string                  `json:"t"`
	Lat gpsprot.Angle           `json:"lat"`
	Lon gpsprot.Angle           `json:"lon"`
	Ele opt.Val[gpsprot.Length] `json:"ele,omitzero"`
	// DOP uses float32 to avoid json.Marshal artifacts from float64
	// (e.g. 0.82 -> 0.8200000000000001). DOP has only 2 decimal places.
	HDOP    opt.Val[float32] `json:"hdop,omitzero"`
	VDOP    opt.Val[float32] `json:"vdop,omitzero"`
	PDOP    opt.Val[float32] `json:"pdop,omitzero"`
	NumSat  opt.Val[uint16]  `json:"numsat,omitzero"`
	FixType string           `json:"fixType,omitzero"`
}

// NewTrackLogEntry builds a TrackLogEntry from a NavEpochMsg, PVMsgBundle, and UTCTime.
// Returns a zero entry (T == "") if PosGeo or utc are unavailable.
func NewTrackLogEntry(msg *gpsprot.NavEpochMsg, pv *gpsprot.PVMsgBundle, utc *ptime.UTCTime) TrackLogEntry {
	if utc == nil || !pv.PosGeo.IsSet() {
		return TrackLogEntry{}
	}
	pg := pv.PosGeo.Get()
	e := TrackLogEntry{
		T:   utc.SysTime().Format("2006-01-02T15:04:05.000Z"),
		Lat: pg.LatLon[0],
		Lon: pg.LatLon[1],
		Ele: pg.HeightMSL,
	}
	if msg.SolutionDim != gpsprot.SolutionDimTimeOnly {
		setDOP(&e.HDOP, msg.DOP.Hor)
		setDOP(&e.VDOP, msg.DOP.Vert)
		setDOP(&e.PDOP, msg.DOP.Pos)
	}
	e.NumSat = msg.NumSVUsed
	e.FixType = fixType(msg.FixLevel, msg.SolutionDim, msg.Correction)
	return e
}

// TrackLogObserver writes one JSONL trackpoint per navigation epoch.
type TrackLogObserver struct {
	obs.DefaultObserver
	lg  *slog.Logger
	lf  logfile.LogFile
	utc *ptime.UTCTime
}

// NewTrackLogObserver creates a TrackLogObserver that writes trackpoints to a JSONL file.
func NewTrackLogObserver(lg *slog.Logger, path string) (*TrackLogObserver, error) {
	o := &TrackLogObserver{lg: lg}
	if err := o.lf.Open(path, true); err != nil {
		return nil, err
	}
	return o, nil
}

// Tick caches the latest UTC time for use in trackpoint timestamps.
func (o *TrackLogObserver) Tick(msg *gpsprot.TimeMsg, _ time.Time) {
	o.utc = msg.UTCTime.Ptr()
}

// NavEpochPV writes a trackpoint if PosGeo and UTC time are available.
func (o *TrackLogObserver) NavEpochPV(msg *gpsprot.NavEpochMsg, pv *gpsprot.PVMsgBundle, _ time.Time) {
	if o.lf.File == nil {
		return
	}
	e := NewTrackLogEntry(msg, pv, o.utc)
	if e.T == "" {
		return
	}
	b, err := json.Marshal(&e)
	if err != nil {
		o.lf.HandleWriteError(err, o.lg)
		return
	}
	b = append(b, '\n')
	_, err = o.lf.File.Write(b)
	o.lf.HandleWriteError(err, o.lg)
}

// ReopenLog handles log rotation.
func (o *TrackLogObserver) ReopenLog() {
	o.lf.Reopen(o.lg)
}

// Release closes the log file.
func (o *TrackLogObserver) Release() {
	o.lf.Close(o.lg)
}

func setDOP(dst *opt.Val[float32], src opt.Val[float64]) {
	if src.IsSet() {
		dst.Set(float32(src.Get()))
	}
}

func fixType(level gpsprot.FixLevel, dim gpsprot.SolutionDim, corr gpsprot.CorrKind) string {
	if level == gpsprot.FixLevelNone {
		return "none"
	}
	if dim == gpsprot.SolutionDim2D {
		return "2d"
	}
	if dim != gpsprot.SolutionDim3D {
		return ""
	}
	if corr != 0 {
		return "dgps"
	}
	return "3d"
}
