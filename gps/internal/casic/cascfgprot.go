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
// Probing polls CFG-RATE, which both firmware families answer with a
// data message and an ACK on the fast CFG lane (see the reply-timing
// record in plan/archive/casic-config.md). Probing is recognition
// only: it succeeds on the first intact solicited response - the
// readback or its ACK, whichever survives the line - since only a
// CASIC receiver produces either in CASIC framing. A NAK is never
// taken as identification: other vendors' receivers have been seen
// NAKing packets that are not theirs. The readback selects the family
// (zkw3.md documents its byte 2 as fixRateHz, a rate with no zero
// value, while casic2.md reserves the same byte as zero, so nonzero
// means V6) and seeds the Configurator, whose rate phase can then
// skip a no-op CFG-RATE write; when the readback was corrupted and
// probing completed on the ACK alone, the Configurator re-polls it
// (see generateFamilyReqs).
//
// The first configuration request may be written milliseconds before
// the probe poll's ACK arrives. That transient overlap of two CFG
// transactions is accepted: waiting out every probe reply provably
// overruns the detection window on a saturated V5 line, and the
// receivers queue requests at least 30 deep (measured).
type ConfigProtocol struct {
	rate         *casbin.CfgRate // probe readback; nil when it never survived the line
	probed       bool            // an intact probe response identified a CASIC receiver
	tProbed      time.Time       // when the identifying response arrived
	pollsPending int             // probe polls sent and not yet ACKed or NAKed
	cfg          *Configurator
}

// probeStragglerWindow is how long after the identifying response a
// CFG-RATE ACK or NAK can still be a probe reply. Stragglers are the
// retry probe's replies: the retry lags the first probe by 1.5 s and
// its replies drain within a few seconds even on a saturated 9600
// line (measured), so past this window a nonzero pollsPending means
// the straggler was lost, and CFG-RATE acknowledgements belong to the
// Configurator again.
const probeStragglerWindow = 6 * time.Second

var _ gpsprot.ConfigProtocol = (*ConfigProtocol)(nil)

// NewConfigProtocol creates a ConfigProtocol for CASIC receivers.
func NewConfigProtocol() *ConfigProtocol {
	return &ConfigProtocol{}
}

// NativeMsg captures the probe's CFG-RATE readback until a
// Configurator exists, then routes CASIC messages and NMEA sentences
// (the GPTXT replies to the V5 identity queries) to it.
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
	// While probe polls are unresolved and a straggler can still
	// arrive, CFG-RATE ACKs and NAKs belong to the probe and are
	// consumed here, before and after Configure: responses arrive in
	// request order, so they can only be probe replies, and a retried
	// probe's late ACK must never reach the Configurator's ACK
	// correlation. When a probe ACK was itself lost, this claim is
	// stale, and within the window it steals one acknowledgment from
	// a Configurator CFG-RATE request that completes there - a chosen
	// residual: the theft costs that request one retry and can never
	// falsely confirm anything, and the fatal stacking (two pending
	// polls plus an in-window completion) is unreachable, since a
	// second probe exists only on a line too slow to reach a
	// Configurator CFG-RATE request within the window. Readback data
	// always flows to the Configurator - any genuine readback answers
	// the family re-poll correctly, and the speed-change confirmation
	// poll it could theoretically confuse runs long after the window
	// (and behind a baud switch that garbles old-speed stragglers
	// anyway).
	if cp.pollsPending > 0 && (cp.tProbed.IsZero() || tRead.Sub(cp.tProbed) <= probeStragglerWindow) {
		switch mt := m.(type) {
		case *casbin.AckAck:
			if casbin.MakeMsgID(mt.ClsID, mt.MsgID) == casbin.CfgRateID {
				cp.pollsPending--
				cp.probe(tRead)
				return nil
			}
		case *casbin.AckNak:
			if casbin.MakeMsgID(mt.ClsID, mt.MsgID) == casbin.CfgRateID {
				cp.pollsPending--
				return nil
			}
		}
	}
	if cp.cfg != nil {
		return cp.cfg.nativeMsg(m, tRead)
	}
	if r, ok := m.(*casbin.CfgRate); ok && cp.pollsPending > 0 {
		cp.rate = r
		cp.probe(tRead)
	}
	return nil
}

// probe records a successful identification.
func (cp *ConfigProtocol) probe(tRead time.Time) {
	if !cp.probed {
		cp.probed = true
		cp.tProbed = tRead
	}
}

// ProbePackets returns an empty-payload CFG-RATE poll. Both families
// answer on the fast CFG lane, so probing is quick on a healthy line.
// A V5 receiver at its default 9600 baud with the default NMEA load
// saturates its line and the reply queues behind the TX backlog for
// seconds; detection there is best-effort, and reliable configuration
// needs the baud rate persistently raised first (see the receiver
// notes).
func (cp *ConfigProtocol) ProbePackets() ([][]byte, time.Duration) {
	cp.pollsPending++
	pkt, _ := casbin.PackMsg(casbin.CfgRateID, nil)
	return [][]byte{pkt}, 0
}

// ProbeOK reports whether a CASIC receiver has been identified: the
// first intact solicited response - readback or ACK - suffices. A NAK
// never identifies.
func (cp *ConfigProtocol) ProbeOK() bool {
	return cp.probed
}

// Configure creates a Configurator for the given configuration
// target. The probe readback is normally in hand; when it was
// corrupted and probing completed on the ACK alone, rate is nil and
// the Configurator opens by re-polling it (see generateFamilyReqs).
func (cp *ConfigProtocol) Configure(target *gpsprot.ConfigTarget) (gpsprot.Configurator, error) {
	if !cp.probed {
		panic("Configure called before probe OK")
	}
	cp.cfg = newConfigurator(target, cp.rate)
	return cp.cfg, nil
}
