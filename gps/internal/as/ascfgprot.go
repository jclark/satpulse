package as

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// tagNMEA and tagRTCM are the packet tags of the nmea and rtcm
// processors. On a port where the Allystar probe succeeded, both are the
// same receiver talking, so they feed the rate estimator too - RTCM
// closes the gap for an RTCM-only target on an otherwise-silent receiver,
// whose MSM epochs betray the native grid. Declared here rather than
// imported from those packages, which would cycle through their tests.
const (
	tagNMEA gpsprot.Tag = "NMEA"
	tagRTCM gpsprot.Tag = "RTCM"
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
	probed bool           // a MON-VER response identified an Allystar receiver
	ver    *asbin.MonVer  // the probe's MON-VER answer
	cfg    *Configurator
	est    *rateEstimator // shared with the Configurator it creates
}

var _ gpsprot.ConfigProtocol = (*ConfigProtocol)(nil)
var _ gpsprot.HandledNativeMsgHandler = (*ConfigProtocol)(nil)

// NewConfigProtocol creates a ConfigProtocol for Allystar receivers.
func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{est: newRateEstimator()}
}

// NativeMsg processes Allystar messages the data path did not consume:
// MON-VER (probe answer), ACK/NAK and poll responses (routed to the
// Configurator), and any unconsumed periodic message (fed to the rate
// estimator). NMEA on a port where the Allystar probe succeeded is the
// same receiver talking, so NMEA feeds the estimator too.
func (cp *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	if tag == tagNMEA {
		cp.observe(msg, tRead)
		return nil
	}
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
	cp.observe(m, tRead)
	if cp.cfg != nil {
		return cp.cfg.nativeMsg(m, tRead)
	}
	return nil
}

// HandledNativeMsg feeds the rate estimator the messages the data path
// consumed - the enabled NAV messages, parsed NMEA sentences, and RTCM
// frames - which the Configurator would otherwise never see. RTCM matters
// for an RTCM-only target: the enabled MSM output is the only periodic
// traffic, and its header epoch is the receiver's own clock (content
// time), so a self-set MSM resolves the native rate exactly.
func (cp *ConfigProtocol) HandledNativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	if tag == Tag || tag == tagNMEA || tag == tagRTCM {
		cp.observe(msg, tRead)
	}
	return nil
}

// observe feeds one message to the estimator and lets the Configurator
// react to any resolution (completing a pending resolve-phase watch).
func (cp *ConfigProtocol) observe(msg interface{}, tRead time.Time) {
	cp.est.observe(msg, tRead)
	if cp.cfg != nil {
		cp.cfg.onObserve()
	}
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
