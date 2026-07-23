package ubx

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

type ConfigProtocol struct {
	ver *Version
	cfg *Configurator
}

var _ gpsprot.ConfigProtocol = (*ConfigProtocol)(nil)
var _ gpsprot.NativeMsgHandler = (*ConfigProtocol)(nil)

func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{}
}

func (px *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	if tag != Tag {
		return nil
	}
	m, ok := msg.(ubxbin.Msg)
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
	case *ubxbin.MonVer:
		px.ver = monVer(mt)
	}
	return nil
}

func (px *ConfigProtocol) Version() *Version {
	return px.ver
}

func (px *ConfigProtocol) ProbePackets() ([][]byte, time.Duration) {
	return [][]byte{ubxbin.Poll(ubxbin.MonVerID)}, 0
}

func (px *ConfigProtocol) ProbeOK() bool {
	return px.ver != nil
}

// Configure creates a Configurator for the given configuration target.
func (px *ConfigProtocol) Configure(target *gpsprot.ConfigTarget) (gpsprot.Configurator, error) {
	if px.ver == nil {
		panic("Configure called before probe OK")
	}
	px.cfg = newConfigurator(target, px.ver)
	return px.cfg, nil
}
