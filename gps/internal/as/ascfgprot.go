package as

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// tagNMEA is the packet tag of the nmea processor. On a port where the
// Allystar probe succeeded, NMEA is the same receiver talking, so it
// feeds the message watcher too. Declared here rather than imported from
// that package, which would cycle through its tests.
const tagNMEA gpsprot.Tag = "NMEA"

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
	est    *msgWatcher // shared with the Configurator it creates
}

var _ gpsprot.ConfigProtocol = (*ConfigProtocol)(nil)
var _ gpsprot.HandledNativeMsgHandler = (*ConfigProtocol)(nil)

// NewConfigProtocol creates a ConfigProtocol for Allystar receivers.
func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{est: newMsgWatcher()}
}

// NativeMsg processes the messages the data path did not consume:
// MON-VER (probe answer), ACK/NAK and poll responses (routed to the
// Configurator), and anything else the receiver sends, which all feeds
// the message watcher.
func (cp *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	if tag != Tag && tag != tagNMEA {
		return nil
	}
	cp.est.observe(msg, tRead)
	if ver, ok := msg.(*asbin.MonVer); ok {
		cp.ver = ver
		cp.probed = true
		return nil
	}
	if cp.cfg == nil {
		return nil
	}
	var err error
	if m, ok := msg.(asbin.Msg); ok && tag == Tag {
		err = cp.cfg.nativeMsg(m, tRead)
	}
	cp.cfg.searchStep(tRead)
	return err
}

// HandledNativeMsg feeds the message watcher the messages the data path
// consumed - the NAV messages that are on and the parsed NMEA sentences -
// which the Configurator would otherwise never see.
func (cp *ConfigProtocol) HandledNativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	if tag != Tag && tag != tagNMEA {
		return nil
	}
	cp.est.observe(msg, tRead)
	if cp.cfg != nil {
		cp.cfg.searchStep(tRead)
	}
	return nil
}

// ProbePackets returns an empty-payload MON-VER poll. State-neutral and
// repeatable; a receiver on a healthy line answers within tens of
// milliseconds, and the default NMEA load is comfortable at the
// factory 115200 baud, so probing relies on repetition rather than
// long waits.
func (cp *ConfigProtocol) ProbePackets() ([][]byte, time.Duration) {
	return [][]byte{asbin.Poll(asbin.MonVerID)}, 0
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
	cp.cfg = newConfigurator(target, cp.ver, cp.est)
	return cp.cfg, nil
}
