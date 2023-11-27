package ubx

import (
	"errors"
	"time"

	"github.com/jclark/satpulse/internal/gpsmsg"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

type Protocol struct {
	ver  *Version
	acks []*Ack
	cfg  *Configurator
	h    gpsmsg.MsgHandler
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

func (prot *Protocol) SetHandler(h gpsmsg.MsgHandler) {
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
	a := prot.FindAckByMsgId(bin.PacketMsgId(packet), tSent)
	if a == nil {
		return nil
	}
	return &a.Ack
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

func (prot *Protocol) Configure(target *gpsmsg.ConfigMap, opts gpsmsg.ConfigOptions) (gpsmsg.Configurator, error) {
	if prot.ver == nil {
		panic("Configure called before probe OK")
	}
	if opts.Flash && !prot.ver.Flash {
		return nil, errors.New("cannot save to flash: receiver does not have flash memory")
	}
	steps := normalConfigSteps
	if prot.ver.protVerGreater(23, 1) {
		steps = newConfigSteps
	}
	prot.cfg = &Configurator{
		ver:    prot.ver,
		target: target,
		steps:  steps,
		opts:   opts,
	}
	return prot.cfg, nil
}
