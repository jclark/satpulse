package ubx

import (
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

// Protocol-specific handler
type ProtHandler interface {
	UBX(msg bin.Msg, tRead time.Time)
}

func ProcessPacket(data string, tRead time.Time, h gpsmsg.Handler, ph ProtHandler) error {
	m, err := bin.ParseMsg(data)
	if err != nil {
		return err
	}
	if !Dispatch(m, tRead, h) && ph != nil {
		ph.UBX(m, tRead)
	}
	return nil
}

func Dispatch(m bin.Msg, tRead time.Time, h gpsmsg.Handler) bool {
	var time *gpsmsg.Time
	switch mt := m.(type) {
	case *bin.NavTimeLS:
		ls := leapSecond(mt)
		if ls == nil {
			return false
		}
		if h != nil {
			h.LeapSecond(ls, tRead)
		}
		return true
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
	default:
		return false
	}
	if h != nil {
		h.Time(time, tRead)
	}
	return true
}

// PacketMsgID returns a human-readable identifier of the packet type.
func PacketMsgID(packet []byte) string {
	return "UBX-" + bin.PacketMsgId(packet).String()
}

func PacketAckable(packet []byte) bool {
	return bin.PacketMsgId(packet).Ackable()
}
