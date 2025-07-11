package ubx

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

type PacketExchanger struct {
	ver     *Version
	cfg     *Configurator
	nmhNext gpsprot.NativeMsgHandler
}

var _ gpsprot.PacketExchanger = (*PacketExchanger)(nil)
var _ gpsprot.NativeMsgHandler = (*PacketExchanger)(nil)

func newPacketExchanger(nmhNext gpsprot.NativeMsgHandler) *PacketExchanger {
	return &PacketExchanger{
		nmhNext: nmhNext,
	}
}

func (px *PacketExchanger) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	m := msg.(bin.Msg)
	if px.cfg != nil {
		handled, err := px.cfg.processMsg(m, tRead)
		if err != nil || handled {
			return err
		}
	}
	switch mt := m.(type) {
	case *bin.MonVer:
		px.ver = monVer(mt)
	default:
		if px.nmhNext != nil {
			return px.nmhNext.NativeMsg(tag, msgID, msg, tRead)
		}
	}
	return nil
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
	px.cfg = newConfigurator(target, px.ver)
	return px.cfg, nil
}
