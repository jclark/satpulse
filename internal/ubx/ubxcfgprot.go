package ubx

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

type ConfigProtocol struct {
	ver     *Version
	cfg     *Configurator
}

var _ gpsprot.ConfigProtocol2 = (*ConfigProtocol)(nil)
var _ gpsprot.NativeMsgHandler = (*ConfigProtocol)(nil)

func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{}
}

func (px *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	if tag != Tag {
		return nil
	}
	m, ok := msg.(bin.Msg)
	if !ok {
		return nil
	}
	if px.cfg != nil {
		handled, err := px.cfg.processMsg(m, tRead)
		if err != nil || handled {
			return err
		}
	}
	switch mt := m.(type) {
	case *bin.MonVer:
		px.ver = monVer(mt)
	}
	return nil
}

func (px *ConfigProtocol) Version() *Version {
	return px.ver
}

func (px *ConfigProtocol) ProbePacket() []byte {
	return bin.Poll(bin.MonVerID)
}

func (px *ConfigProtocol) ProbeOK() bool {
	return px.ver != nil
}

// Configure2 creates a Configurator2 for the given configuration target.
func (px *ConfigProtocol) Configure2(target *gpsprot.ConfigTarget) (gpsprot.Configurator2, error) {
	if px.ver == nil {
		panic("Configure2 called before probe OK")
	}
	px.cfg = newConfigurator(target, px.ver)
	return px.cfg, nil
}
