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
	h    gpsmsg.Handler
	ph   ProtHandler
}

var _ gpsmsg.Protocol = (*Protocol)(nil)

type Ack struct {
	msgID bin.MsgID
	gpsmsg.Ack
}

func (prot *Protocol) ProcessPacket(data string, tRead time.Time) error {
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
	if Dispatch(m, tRead, prot.h) {
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
		if prot.ph != nil {
			prot.ph.UBX(m, tRead)
		}
	}
	return nil
}

func (prot *Protocol) SetHandler(h gpsmsg.Handler) {
	prot.h = h
}

func (prot *Protocol) SetProtHandler(ph ProtHandler) {
	prot.ph = ph
}

func (prot *Protocol) ack(msgID bin.MsgID, ok bool, t time.Time) {
	prot.acks = append(prot.acks, &Ack{msgID, gpsmsg.Ack{OK: ok, TRead: t}})
}

func (prot *Protocol) Version() *Version {
	return prot.ver
}

func (prot *Protocol) FindAck(packet []byte, tSent time.Time) *gpsmsg.Ack {
	return &prot.FindAckByMsgId(bin.PacketMsgId(packet), tSent).Ack
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

func (prot *Protocol) Configure(target *gpsmsg.ConfigMap) gpsmsg.Configurator {
	if prot.ver == nil {
		panic("Configure called before probe OK")
	}
	steps := normalConfigSteps
	if prot.ver.protVerGreater(23, 1) {
		steps = newConfigSteps
	}
	prot.cfg = &Configurator{
		ver:    prot.ver,
		target: target,
		steps:  steps,
	}
	return prot.cfg
}
