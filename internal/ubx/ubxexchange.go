package ubx

import (
	"errors"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

type PacketExchanger struct {
	ver *Version
	cfg *Configurator
	h   gpsprot.MsgHandler
	ph  ProtHandler
}

var _ gpsprot.PacketExchanger = (*PacketExchanger)(nil)

func (px *PacketExchanger) ProcessPacket(data string, tRead time.Time) error {
	m, err := bin.ParseMsg(data)
	if err != nil {
		return err
	}
	if px.cfg != nil {
		done, err := px.cfg.processMsg(m, tRead)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	if Dispatch(m, tRead, px.h) {
		return nil
	}
	switch mt := m.(type) {
	case *bin.MonVer:
		px.ver = monVer(mt)
	default:
		if px.ph != nil {
			px.ph.UBX(m, tRead)
		}
	}
	return nil
}

func (px *PacketExchanger) SetHandler(h gpsprot.MsgHandler) {
	px.h = h
}

func (px *PacketExchanger) SetProtHandler(ph ProtHandler) {
	px.ph = ph
}

func (px *PacketExchanger) Version() *Version {
	return px.ver
}

func (px *PacketExchanger) ProbePacket() []byte {
	return bin.Poll(bin.MonVerID)
}

func (px *PacketExchanger) ProbeOK() bool {
	return px.ver != nil
}

func (px *PacketExchanger) Configure(target *gpsprot.ConfigTarget) (gpsprot.Configurator, error) {
	if px.ver == nil {
		panic("Configure called before probe OK")
	}
	if target.Opts.Flash && !px.ver.Flash {
		return nil, errors.New("cannot save to flash: receiver does not have flash memory")
	}
	px.cfg = newConfigurator(target, px.ver)
	return px.cfg, nil
}
