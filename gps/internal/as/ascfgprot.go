package as

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// Vendor is the receiver vendor name reported in ReceiverInfo.
const Vendor = "Allystar"

// ConfigProtocol implements gpsprot.ConfigProtocol for Allystar
// receivers.
//
// Probing polls MON-VER directly (empty payload). Every tested
// firmware answers with the MON-VER message itself, and polls get no
// acknowledgement, so a late probe response is just another MON-VER
// message: the Configurator never requests MON-VER (ReceiverInfo comes
// from the probe's answer), so there is nothing to misattribute.
type ConfigProtocol struct {
	probed bool          // a MON-VER response identified an Allystar receiver
	ver    *asbin.MonVer // the probe's MON-VER answer
	cfg    *Configurator
}

var _ gpsprot.ConfigProtocol = (*ConfigProtocol)(nil)

// NewConfigProtocol creates a ConfigProtocol for Allystar receivers.
func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{}
}

// NativeMsg processes Allystar messages routed from the packet
// processor.
func (cp *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	if tag != Tag {
		return nil
	}
	m, ok := msg.(asbin.Msg)
	if !ok {
		return nil
	}
	if ver, ok := m.(*asbin.MonVer); ok {
		cp.ver = ver
		cp.probed = true
		return nil
	}
	if cp.cfg != nil {
		return cp.cfg.nativeMsg(m, tRead)
	}
	return nil
}

// ProbePacket returns an empty-payload MON-VER poll. State-neutral and
// repeatable; a receiver on a healthy line answers within tens of
// milliseconds, and the default NMEA load is comfortable at the
// factory 115200 baud, so probing relies on repetition rather than
// long waits.
func (cp *ConfigProtocol) ProbePacket() []byte {
	pkt, _ := asbin.PackMsg(asbin.MonVerID, nil)
	return pkt
}

// ProbeOK reports whether an Allystar receiver has been identified.
func (cp *ConfigProtocol) ProbeOK() bool {
	return cp.probed
}

// Configure creates a Configurator for the given configuration target.
func (cp *ConfigProtocol) Configure(target *gpsprot.ConfigTarget) (gpsprot.Configurator, error) {
	if !cp.probed {
		panic("Configure called before probe OK")
	}
	cp.cfg = newConfigurator(target, cp.ver)
	return cp.cfg, nil
}
