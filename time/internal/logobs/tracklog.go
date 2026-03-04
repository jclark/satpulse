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

type trackLogEntry struct {
	T       string                    `json:"t"`
	Lat     gpsprot.Angle             `json:"lat"`
	Lon     gpsprot.Angle             `json:"lon"`
	Ele     opt.Val[gpsprot.Length]   `json:"ele,omitzero"`
	// DOP uses float32 to avoid json.Marshal artifacts from float64
	// (e.g. 0.82 -> 0.8200000000000001). DOP has only 2 decimal places.
	HDOP    opt.Val[float32]          `json:"hdop,omitzero"`
	VDOP    opt.Val[float32]          `json:"vdop,omitzero"`
	PDOP    opt.Val[float32]          `json:"pdop,omitzero"`
	NumSat  opt.Val[uint16]           `json:"numsat,omitzero"`
	FixType string                    `json:"fixType,omitzero"`
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
	if err := o.lf.Open(path); err != nil {
		return nil, err
	}
	return o, nil
}

// Tick caches the latest UTC time for use in trackpoint timestamps.
func (o *TrackLogObserver) Tick(msg *gpsprot.TimeMsg, _ time.Time) {
	o.utc = msg.UTCTime
}

// NavEpochPV writes a trackpoint if PosGeo and UTC time are available.
func (o *TrackLogObserver) NavEpochPV(msg *gpsprot.NavEpochMsg, pv *gpsprot.PVMsgBundle, _ time.Time) {
	if o.lf.File == nil || o.utc == nil || !pv.PosGeo.IsSet() {
		return
	}
	pg := pv.PosGeo.Get()
	e := trackLogEntry{
		T:   o.utc.Date.Add(o.utc.TimeOfDay).Format("2006-01-02T15:04:05.000Z"),
		Lat: pg.LatLon[0],
		Lon: pg.LatLon[1],
		Ele: pg.HeightMSL,
	}
	if msg.FixDim != gpsprot.FixDimTimeOnly {
		setDOP(&e.HDOP, msg.DOP.Hor)
		setDOP(&e.VDOP, msg.DOP.Vert)
		setDOP(&e.PDOP, msg.DOP.Pos)
	}
	e.NumSat = msg.NumSVUsed
	e.FixType = fixType(msg.FixLevel, msg.FixDim)
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

func fixType(level gpsprot.FixLevel, dim gpsprot.FixDim) string {
	if level == gpsprot.FixLevelNone {
		return "none"
	}
	if dim == gpsprot.FixDim2D {
		return "2d"
	}
	if dim != gpsprot.FixDim3D {
		return ""
	}
	if level >= gpsprot.FixLevelCodeCorrected {
		return "dgps"
	}
	return "3d"
}
