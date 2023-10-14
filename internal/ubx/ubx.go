package ubx

import (
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

// Protocol-specific handler
type ProtHandler interface {
	Version(msg *Version, tRead time.Time)
	UBX(msg bin.Msg, tRead time.Time)
}

func ProcessPacketData(data string, tRead time.Time, h gpsmsg.Handler, ph ProtHandler) error {
	um, err := bin.ParseMsg(data)
	if err != nil {
		return err
	}
	Dispatch(um, tRead, h, ph)
	return nil
}

func Dispatch(m bin.Msg, tRead time.Time, h gpsmsg.Handler, ph ProtHandler) {
	var time *gpsmsg.Time
	var timeMode *gpsmsg.TimeMode
	switch mt := m.(type) {
	case *bin.NavTimeLS:
		ls := leapSecond(mt)
		if ls != nil && h != nil {
			h.LeapSecond(ls, tRead)
			return
		}
	case *bin.NavTimeGPS:
		time = timeNavTimeGPS(mt)
	case *bin.NavTimeBDS:
		time = timeNavTimeBDS(mt)
	case *bin.NavTimeGal:
		time = timeNavTimeGal(mt)
	case *bin.NavTimeGLO:
		time = timeNavTimeGLO(mt)
	case *bin.NavTimeUTC:
		time = timeNavTimeUTC(mt)
	case *bin.NavPVT:
		time = timeNavPVT(mt)
	case *bin.TimTP:
		time = timeTimTP(mt)
	case *bin.TimTos:
		time = timeTimTos(mt)
	case *bin.CfgTmode2:
		timeMode = timeMode2(mt)
	case *bin.CfgTmode3:
		timeMode = timeMode3(mt)
	case *bin.MonVer:
		ver := monVer(mt)
		if ver != nil && ph != nil {
			ph.Version(ver, tRead)
		}
	default:
		if ph != nil {
			ph.UBX(m, tRead)
		}
	}
	if h != nil {
		if time != nil {
			h.Time(time, tRead)
		}
		if timeMode != nil {
			h.TimeMode(timeMode, tRead)
		}
	}
}
