package ubx

import (
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

// Protocol-specific handler
type ProtHandler interface {
	Version(msg *Version, tRead time.Time)
	Ack(msgID bin.MsgID, ok bool, tRead time.Time)
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
	case *bin.MonVer:
		ver := monVer(mt)
		if ver != nil && ph != nil {
			ph.Version(ver, tRead)
		}
	case *bin.AckAck:
		if ph != nil {
			ph.Ack(mt.MsgID, true, tRead)
		}
	case *bin.AckNak:
		if ph != nil {
			ph.Ack(mt.MsgID, false, tRead)
		}
	default:
		if ph != nil {
			ph.UBX(m, tRead)
		}
	}
	if h != nil && time != nil {
		h.Time(time, tRead)
	}
}
