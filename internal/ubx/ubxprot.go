package ubx

import (
	"errors"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

type Protocol struct {
	ver *Version
	cfg *Configurator
	h   gpsprot.MsgHandler
	ph  ProtHandler
}

var _ gpsprot.Protocol = (*Protocol)(nil)

func (prot *Protocol) ProcessPacket(data string, tRead time.Time) error {
	m, err := bin.ParseMsg(data)
	if err != nil {
		return err
	}
	if prot.cfg != nil {
		done, err := prot.cfg.processMsg(m, tRead)
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
	case *bin.MonVer:
		prot.ver = monVer(mt)
	default:
		if prot.ph != nil {
			prot.ph.UBX(m, tRead)
		}
	}
	return nil
}

func (prot *Protocol) SetHandler(h gpsprot.MsgHandler) {
	prot.h = h
}

func (prot *Protocol) SetProtHandler(ph ProtHandler) {
	prot.ph = ph
}

func (prot *Protocol) Version() *Version {
	return prot.ver
}

func (prot *Protocol) ProbePacket() []byte {
	return bin.Poll(bin.MonVerID)
}

func (prot *Protocol) ProbeOK() bool {
	return prot.ver != nil
}

func (prot *Protocol) Configure(target *gpsprot.ConfigTarget) (gpsprot.Configurator, error) {
	if prot.ver == nil {
		panic("Configure called before probe OK")
	}
	if target.Opts.Flash && !prot.ver.Flash {
		return nil, errors.New("cannot save to flash: receiver does not have flash memory")
	}
	prot.cfg = newConfigurator(target, prot.ver)
	return prot.cfg, nil
}
