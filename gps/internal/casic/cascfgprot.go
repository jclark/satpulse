package casic

import (
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// Vendor is the receiver vendor name reported in ReceiverInfo.
const Vendor = "Zhongke"

// fwFamily distinguishes the two CASIC firmware families. They share
// packet framing and most CFG messages but differ in message classes
// (NAV vs NAV2), some CFG ids, and several enum encodings.
type fwFamily int

const (
	familyV5 fwFamily = iota // URANUS5: NAV class, GPS/BDS/GLN only
	familyV6                 // URANUS6: NAV2 class, adds GAL/QZSS/SBAS/IRNSS
)

// ConfigProtocol implements gpsprot.ConfigProtocol for CASIC receivers.
//
// Probing polls MON-VER via CFG-MSG (rate 0xFFFF). V6 firmware answers
// with MON-VER (then ACK); V5 firmware does not support MON-VER and
// answers ACK-NAK - either response proves a CASIC receiver. The NAK
// carries only the class/id of the request, so any NAK of CFG-MSG is
// accepted: during probing no other CFG-MSG request is outstanding.
type ConfigProtocol struct {
	probed bool           // some response identified a CASIC receiver
	ver    *casbin.MonVer // nil when MON-VER is unsupported (V5)
	cfg    *Configurator
}

var _ gpsprot.ConfigProtocol = (*ConfigProtocol)(nil)

// NewConfigProtocol creates a ConfigProtocol for CASIC receivers.
func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{}
}

// NativeMsg processes CASIC messages routed from the packet processor.
func (cp *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	if tag != Tag {
		return nil
	}
	m, ok := msg.(casbin.Msg)
	if !ok {
		return nil
	}
	switch mt := m.(type) {
	case *casbin.MonVer:
		cp.ver = mt
		cp.probed = true
	case *casbin.AckNak:
		if casbin.MakeMsgID(mt.ClsID, mt.MsgID) == casbin.CfgMsgID && cp.cfg == nil {
			cp.probed = true
		}
	}
	if cp.cfg != nil {
		return cp.cfg.nativeMsg(m, tRead)
	}
	return nil
}

// ProbePacket returns a CFG-MSG poll of MON-VER.
func (cp *ConfigProtocol) ProbePacket() []byte {
	pkt, _ := casbin.Serialize(&casbin.CfgMsg{Target: casbin.MonVerID, Rate: casbin.PollRate})
	return pkt
}

// ProbeOK reports whether a CASIC receiver has been identified.
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

// Configurator implements gpsprot.Configurator for CASIC receivers.
type Configurator struct {
	target *gpsprot.ConfigTarget
	ver    *casbin.MonVer // nil when MON-VER is unsupported (V5)
	family fwFamily
}

var _ gpsprot.Configurator = (*Configurator)(nil)

func newConfigurator(target *gpsprot.ConfigTarget, ver *casbin.MonVer) *Configurator {
	family := familyV5
	if ver != nil && !strings.Contains(ver.SwVersion.String(), "URANUS5") {
		family = familyV6
	}
	return &Configurator{target: target, ver: ver, family: family}
}

// ReceiverInfo returns static information about the GPS receiver.
// A V5 receiver does not answer MON-VER, so its firmware and hardware
// strings are unknown and reported empty.
func (c *Configurator) ReceiverInfo() *gpsprot.ReceiverInfo {
	info := &gpsprot.ReceiverInfo{Vendor: Vendor, SupportedGNSS: c.supportedGNSS()}
	if c.ver != nil {
		info.Firmware = c.ver.SwVersion.String()
		info.Hardware = c.ver.HwVersion.String()
		info.VendorSpecific = c.ver
	}
	return info
}

// supportedGNSS returns the constellations the firmware family can use.
// V5 supports GPS/BDS/GLONASS only; the V6 supported set should be
// refined from CFG-NAVBAND query results once implemented.
func (c *Configurator) supportedGNSS() gpsprot.GNSSSet {
	set := gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.BDS, gpsprot.GLO)
	if c.family == familyV6 {
		set |= gpsprot.GNSSSetOf(gpsprot.GAL, gpsprot.QZSS, gpsprot.SBAS, gpsprot.NAVIC)
	}
	return set
}

// ConfigSupport returns the configuration options this implementation
// supports. None of the optional capabilities are implemented yet.
func (c *Configurator) ConfigSupport() gpsprot.ConfigSupportFlags {
	return 0
}

// ConfigProps returns the current configuration of the GPS receiver.
func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
	return &gpsprot.ConfigProps{}
}

// GenerateRequests generates configuration requests. No configuration
// steps are implemented yet.
func (c *Configurator) GenerateRequests() error {
	return nil
}

// GetRequestCount returns the current number of requests and whether
// the slice is complete.
func (c *Configurator) GetRequestCount() (int, bool) {
	return 0, true
}

// Request returns the ConfigRequest at the given index.
func (c *Configurator) Request(index int) gpsprot.ConfigRequest {
	panic("no requests")
}

func (c *Configurator) nativeMsg(m casbin.Msg, tRead time.Time) error {
	return nil
}
