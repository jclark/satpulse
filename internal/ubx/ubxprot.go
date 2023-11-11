package ubx

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsmsg"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

type Protocol struct {
	ver  *Version
	acks []*Ack
	cfg  *Configurator
}

type Ack struct {
	msgID bin.MsgID
	OK    bool
	TRead time.Time
}

func (prot *Protocol) ProcessPacket(data string, tRead time.Time, h gpsmsg.Handler, ph ProtHandler) error {
	m, err := bin.ParseMsg(data)
	if err != nil {
		return err
	}
	if prot.cfg != nil {
		done, err := prot.cfg.raw.AddMsg(m)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	if Dispatch(m, tRead, h) {
		return nil
	}
	switch mt := m.(type) {
	case *bin.AckAck:
		prot.ack(mt.MsgID, true, tRead)
	case *bin.AckNak:
		prot.ack(mt.MsgID, false, tRead)
	case *bin.MonVer:
		prot.ver = monVer(mt)
	default:
		if ph != nil {
			ph.UBX(m, tRead)
		}
	}
	return nil
}

func (prot *Protocol) ack(msgID bin.MsgID, ok bool, t time.Time) {
	prot.acks = append(prot.acks, &Ack{msgID, ok, t})
}

func (prot *Protocol) Version() *Version {
	return prot.ver
}

// XXX need to move this into something at the gpsmsg level
func (prot *Protocol) FindAck(packet []byte, tSent time.Time) (ack *Ack) {
	return prot.FindAckByMsgId(bin.PacketMsgId(packet), tSent)
}

func (prot *Protocol) FindAckByMsgId(msgID bin.MsgID, tSent time.Time) (ack *Ack) {
	stale := 0
	for i, a := range prot.acks {
		if !a.TRead.After(tSent) {
			stale = i + 1
		} else if a.msgID == msgID {
			ack = a
			break
		}
	}
	if stale > 0 {
		prot.acks = prot.acks[stale:]
	}
	return
}

func (prot *Protocol) ProbePacket() []byte {
	return bin.Poll(bin.MonVerID)
}

func (prot *Protocol) ProbeOK() bool {
	return prot.ver != nil
}

func (prot *Protocol) Configure(target *gpsmsg.Config) gpsmsg.Configurator {
	if prot.ver == nil {
		panic("Configure called before probe OK")
	}
	prot.cfg = &Configurator{
		ver:    prot.ver,
		target: target,
		steps:  normalConfigSteps,
	}
	return prot.cfg
}
