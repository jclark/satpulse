package casic

import (
	"errors"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// Signal selection branches by family. V6 uses CFG-NAVBAND with a
// per-signal bitmask (bit positions per protocol section 1.4, the
// casbin.Sig* constants); the readback's sigIDMask lists the signals
// the unit can receive, which is the per-unit supported set (the
// dual-band F8N and single-band AT632 report different masks). V5 uses
// CFG-NAVX with a constellation bitmask and is single-band, so its
// supported set is GPS L1 C/A, BDS B1I, GLONASS L1.
//
// Realization per the semantics: the requested set is intersected with
// what is knowable in advance (the fixed V5 family set; the signals
// CFG-NAVBAND can express on V6), and a request left with no
// non-augmentation signal of a major GNSS fails rather than writing a
// selection that would disable every constellation (the ubx guard).
// On V5 the ACKed values are reported as achieved. On V6 the ACK means
// "enabled the intersection with my capability" - the silicon clamps
// the reception mask - so the achieved set is read back, the one case
// where a post-set readback is legitimate.

// navBandSignals maps CFG-NAVBAND bit positions to signals. BDS B1I
// has two bits (GEO and MEO satellites); both follow SigBDSB1I.
var navBandSignals = []struct {
	bit casbin.SigID
	sig gpsprot.Signal
}{
	{casbin.SigGPSL1CA, gpsprot.SigGPSL1CA},
	{casbin.SigGPSL5, gpsprot.SigGPSL5},
	{casbin.SigSBASL1, gpsprot.SigSBASL1CA},
	{casbin.SigSBASL5, gpsprot.SigSBASL5},
	{casbin.SigGLOL1, gpsprot.SigGLOL1},
	{casbin.SigGALE1, gpsprot.SigGALE1},
	{casbin.SigGALE5a, gpsprot.SigGALE5a},
	{casbin.SigBDSB1IGEO, gpsprot.SigBDSB1I},
	{casbin.SigBDSB1IMEO, gpsprot.SigBDSB1I},
	{casbin.SigBDSB1C, gpsprot.SigBDSB1C},
	{casbin.SigBDSB2a, gpsprot.SigBDSB2a},
	{casbin.SigQZSSL1CA, gpsprot.SigQZSSL1CA},
	{casbin.SigQZSSL5, gpsprot.SigQZSSL5},
	{casbin.SigNAVICL5, gpsprot.SigNAVICL5},
}

// v5NavSystems maps CFG-NAVX NavSystem bits to the single signal each
// V5 constellation carries.
var v5NavSystems = []struct {
	bit  uint8
	gnss gpsprot.GNSS
	sig  gpsprot.Signal
}{
	{casbin.NavSysGPS, gpsprot.GPS, gpsprot.SigGPSL1CA},
	{casbin.NavSysBDS, gpsprot.BDS, gpsprot.SigBDSB1I},
	{casbin.NavSysGLN, gpsprot.GLO, gpsprot.SigGLOL1},
}

// v5Signals is the V5 supported signal set.
const v5Signals = gpsprot.SignalSet(1<<gpsprot.SigGPSL1CA) |
	gpsprot.SignalSet(1<<gpsprot.SigBDSB1I) |
	gpsprot.SignalSet(1<<gpsprot.SigGLOL1)

// signalsToNavBand converts a signal set to a CFG-NAVBAND mask.
func signalsToNavBand(ss gpsprot.SignalSet) uint32 {
	var mask uint32
	for _, e := range navBandSignals {
		if ss.Contains(e.sig) {
			mask |= 1 << e.bit
		}
	}
	return mask
}

// navBandToSignals converts a CFG-NAVBAND mask to a signal set.
func navBandToSignals(mask uint32) gpsprot.SignalSet {
	var ss gpsprot.SignalSet
	for _, e := range navBandSignals {
		if mask&(1<<e.bit) != 0 {
			ss |= gpsprot.SignalSetOf(e.sig)
		}
	}
	return ss
}

// needsSignals reports whether the target involves signal selection.
func (c *Configurator) needsSignals() bool {
	return c.target.UsesAny(gpsprot.PropIDSignalsEnabled)
}

// generateSignalQuery polls the family's signal selection message.
func (c *Configurator) generateSignalQuery() {
	if !c.needsSignals() {
		return
	}
	if c.family == familyV6 {
		c.pollNavBand()
	} else {
		c.pollNavx()
	}
}

func (c *Configurator) pollNavBand() {
	c.addPollReq(casbin.CfgNavBandID, func(m casbin.Msg) {
		if nb, ok := m.(*casbin.CfgNavBand); ok {
			c.navBand = nb
		}
	})
}

// pollNavx polls CFG-NAVX, which carries both the V5 constellation
// selection and the V5 minimum elevation.
func (c *Configurator) pollNavx() {
	c.addPollReq(casbin.CfgNavxID, func(m casbin.Msg) {
		if nx, ok := m.(*casbin.CfgNavx); ok {
			c.navx = nx
		}
	})
}

// generateSignalSet sends the requested signal selection. On V6 both
// masks must be written: the reception list (sigIDMask) is what gates
// tracking, and writing only the positioning list leaves all signals
// tracked (observed on the F8N). The receiver clamps the written
// reception list to what its hardware can receive (observed on the
// AT632: a dual-band mask came back clamped to the L1-band subset),
// so signals the unit does not have drop out in the verify readback -
// the silent intersection of the semantics, performed by the silicon.
func (c *Configurator) generateSignalSet() {
	requested, ok := c.target.Props.GetSignalsEnabled()
	if !ok {
		return
	}
	if c.family == familyV6 {
		if c.navBand == nil {
			return // property absent on this receiver
		}
		mask := signalsToNavBand(requested)
		if !c.checkSignals(navBandToSignals(mask)) {
			return
		}
		nb := *c.navBand
		nb.SigBandAuto = 0
		nb.SigIDMaskFix = mask
		nb.SigIDMask = mask
		c.addSetReq(&nb, func() { c.navBand = &nb })
		// The acknowledged values are not the whole truth here: the
		// silicon clamps the written reception list to the hardware's
		// capability (verified on the AT632), which is not knowable in
		// advance, so the achieved set needs one readback.
		c.pollNavBand()
		return
	}
	if c.navx == nil {
		return
	}
	want := requested & v5Signals
	if !c.checkSignals(want) {
		return
	}
	gs := want.GNSSSet()
	var sys uint8
	for _, e := range v5NavSystems {
		if gs.Contains(e.gnss) {
			sys |= e.bit
		}
	}
	c.addSetReq(&casbin.CfgNavx{Mask: casbin.NavxNavSystem, NavSystem: sys},
		func() { c.navx.NavSystem = sys })
}

// checkSignals reports whether the signal set a set request would
// realize keeps at least one non-augmentation signal of a major GNSS,
// matching the ubx guard. If not, the request is failed instead of
// sent: writing it would disable every constellation, which is never
// what a request that named signals meant.
func (c *Configurator) checkSignals(ss gpsprot.SignalSet) bool {
	if ss&gpsprot.SigSetMajor&^gpsprot.SigSetAugment != 0 {
		return true
	}
	c.addFailedReq(errors.New("no supported non-augmentation major-GNSS signal in the requested set"))
	return false
}

// signalConfigProps reports the enabled signal set from the readback.
// With automatic band selection active the receiver uses everything it
// can receive.
func (c *Configurator) signalConfigProps(props *gpsprot.ConfigProps) {
	if c.navBand != nil {
		mask := c.navBand.SigIDMaskFix & c.navBand.SigIDMask
		if c.navBand.SigBandAuto != 0 {
			mask = c.navBand.SigIDMask
		}
		props.SetSignalsEnabled(navBandToSignals(mask))
		return
	}
	if c.navx != nil {
		var ss gpsprot.SignalSet
		for _, e := range v5NavSystems {
			if c.navx.NavSystem&e.bit != 0 {
				ss |= gpsprot.SignalSetOf(e.sig)
			}
		}
		props.SetSignalsEnabled(ss)
	}
}
