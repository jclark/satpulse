package casic

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
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
// Probing polls MON-VER via CFG-MSG's poll rate. V6 firmware answers
// with MON-VER then ACK; V5 firmware does not support MON-VER and
// answers ACK-NAK - either response proves a CASIC receiver.
//
// Every probe poll is eventually acknowledged, and responses arrive in
// request order, so the ACK/NAK of a slow probe can arrive after
// Configure when the Configurator already has its own CFG-MSG request
// outstanding (seen on the V5, whose saturated 9600 line delays probe
// responses by several seconds). pollsPending counts unacknowledged
// probe polls; their CFG-MSG acknowledgements are consumed here and
// never forwarded to the Configurator.
type ConfigProtocol struct {
	probed       bool           // some response identified a CASIC receiver
	ver          *casbin.MonVer // nil when MON-VER is unsupported (V5)
	cfg          *Configurator
	pollsPending int // probe polls sent and not yet acknowledged
	verPending   int // MON-VER responses received whose ACK is still due
}

var _ gpsprot.ConfigProtocol = (*ConfigProtocol)(nil)

// NewConfigProtocol creates a ConfigProtocol for CASIC receivers.
func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{}
}

// NativeMsg processes CASIC messages routed from the packet processor,
// and NMEA sentences for the GPTXT replies to PCAS06 version queries.
func (cp *ConfigProtocol) NativeMsg(tag gpsprot.Tag, msgID string, msg interface{}, tRead time.Time) error {
	if tag == nmea.Tag {
		if s, ok := msg.(*nmea.Sentence); ok && cp.cfg != nil {
			return cp.cfg.nativeText(s.Payload, tRead)
		}
		return nil
	}
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
		if cp.pollsPending > 0 {
			cp.verPending++
		}
	case *casbin.AckAck:
		if casbin.MakeMsgID(mt.ClsID, mt.MsgID) == casbin.CfgMsgID && cp.verPending > 0 {
			cp.verPending--
			cp.pollsPending--
			return nil
		}
	case *casbin.AckNak:
		if casbin.MakeMsgID(mt.ClsID, mt.MsgID) == casbin.CfgMsgID && cp.pollsPending > 0 {
			cp.pollsPending--
			cp.probed = true
			return nil
		}
	}
	if cp.cfg != nil {
		return cp.cfg.nativeMsg(m, tRead)
	}
	return nil
}

// ProbePackets returns a CFG-MSG poll of MON-VER. A receiver on a
// healthy line answers within tens of milliseconds; probing relies on
// repeated probes rather than on changing receiver state. A V5
// receiver at its default 9600 baud with the default NMEA load
// saturates its line, delaying responses by about six seconds or
// losing them entirely; detection there is best-effort, and reliable
// configuration needs the baud rate persistently raised first (see
// the receiver notes).
func (cp *ConfigProtocol) ProbePackets() ([][]byte, time.Duration) {
	cp.pollsPending++
	pkt, _ := casbin.Serialize(&casbin.CfgMsg{Target: casbin.MonVerID, Rate: casbin.CfgMsgRatePoll})
	return [][]byte{pkt}, 0
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
