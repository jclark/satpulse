package as

import (
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// Signal selection uses the CFG-NAVSAT per-signal mask. The silicon
// ACKs any mask and clamps it to the hardware's capability - even a
// zero mask is accepted and applied - so the achieved set comes from a
// post-set verify poll (the narrow exception to on-ACK readback,
// earned by hardware evidence on all three test units). The receiver
// itself thus performs the semantics' silent intersection with the
// supported set; requesting the full mask and reading back is how a
// unit's signal plan is discovered.

// navSatSignals maps CFG-NAVSAT mask bits to signals. The protocol
// documentation names the bits; "BEIDOU B2" is taken as B2I (the
// legacy pair of B1I) and "BEIDOU B5" as B2b.
var navSatSignals = []struct {
	mask asbin.CfgNavSatMask
	sig  gpsprot.Signal
}{
	{asbin.CfgNavSatMaskGPSL1, gpsprot.SigGPSL1CA},
	{asbin.CfgNavSatMaskGPSL1C, gpsprot.SigGPSL1C},
	{asbin.CfgNavSatMaskGPSL2C, gpsprot.SigGPSL2C},
	{asbin.CfgNavSatMaskGPSL5, gpsprot.SigGPSL5},
	{asbin.CfgNavSatMaskGLONASSG1, gpsprot.SigGLOL1},
	{asbin.CfgNavSatMaskGLONASSG2, gpsprot.SigGLOL2},
	{asbin.CfgNavSatMaskBEIDOUB1, gpsprot.SigBDSB1I},
	{asbin.CfgNavSatMaskBEIDOUB1C, gpsprot.SigBDSB1C},
	{asbin.CfgNavSatMaskBEIDOUB2, gpsprot.SigBDSB2I},
	{asbin.CfgNavSatMaskBEIDOUB5, gpsprot.SigBDSB2b},
	{asbin.CfgNavSatMaskBEIDOUB2A, gpsprot.SigBDSB2a},
	{asbin.CfgNavSatMaskBEIDOUB3I, gpsprot.SigBDSB3I},
	{asbin.CfgNavSatMaskGALILEOE1, gpsprot.SigGALE1},
	{asbin.CfgNavSatMaskGALILEOE5A, gpsprot.SigGALE5a},
	{asbin.CfgNavSatMaskGALILEOE5B, gpsprot.SigGALE5b},
	{asbin.CfgNavSatMaskGALILEOE6, gpsprot.SigGALE6},
	{asbin.CfgNavSatMaskQZSSL1, gpsprot.SigQZSSL1CA},
	{asbin.CfgNavSatMaskQZSSL1C, gpsprot.SigQZSSL1C},
	{asbin.CfgNavSatMaskQZSSL2C, gpsprot.SigQZSSL2C},
	{asbin.CfgNavSatMaskQZSSL5, gpsprot.SigQZSSL5},
	{asbin.CfgNavSatMaskQZSSL6, gpsprot.SigQZSSL6},
	{asbin.CfgNavSatMaskSBASL1, gpsprot.SigSBASL1CA},
	{asbin.CfgNavSatMaskIRNSSL5, gpsprot.SigNAVICL5},
}

// signalsToNavSat converts a signal set to a CFG-NAVSAT mask. Signals
// with no Allystar bit drop out silently - their absence is knowable
// in advance and shows in the achieved set.
func signalsToNavSat(ss gpsprot.SignalSet) asbin.CfgNavSatMask {
	var mask asbin.CfgNavSatMask
	for _, e := range navSatSignals {
		if ss.Contains(e.sig) {
			mask |= e.mask
		}
	}
	return mask
}

// navSatToSignals converts a CFG-NAVSAT mask to a signal set.
func navSatToSignals(mask asbin.CfgNavSatMask) gpsprot.SignalSet {
	var ss gpsprot.SignalSet
	for _, e := range navSatSignals {
		if mask&e.mask != 0 {
			ss |= gpsprot.SignalSetOf(e.sig)
		}
	}
	return ss
}

// needsSignals reports whether the target involves signal selection.
func (c *Configurator) needsSignals() bool {
	return c.target.UsesAny(gpsprot.PropIDSignalsEnabled)
}

// generateSignalQuery polls CFG-NAVSAT.
func (c *Configurator) generateSignalQuery() {
	if !c.needsSignals() {
		return
	}
	c.pollNavSat()
}

func (c *Configurator) pollNavSat() {
	c.addPollReq(asbin.CfgNavSatID, func(m asbin.Msg) {
		if ns, ok := m.(*asbin.CfgNavSat); ok {
			c.navSat = ns
		}
	})
}

// hwPlan is the capability plan deduced from the HDxxxx chip number of
// the MON-VER hardware string (owner ruling: capability keys off the
// identity the receiver reports).
type hwPlan struct {
	signals asbin.CfgNavSatMask
	rtcm    bool
}

// signalPlans is the supported signal set per chip number, established
// by writing the full mask and reading back the clamp on each test
// unit. Intersecting a signal request with the plan before writing
// matters beyond neatness: the TAU1302 silicon COUPLES - writing a GAL
// E5 bit it lacks switches on GAL E1 - so unsupported bits must not
// reach the wire on known hardware. An unknown chip gets the full
// request and the silicon's clamp, revealed by the verify poll.
var signalPlans = map[string]asbin.CfgNavSatMask{
	"HD8040": 0x4108237, // TAU1201: L1/L5 dual band
	"HD9510": 0x4108237, // TAU951M: L1/L5 dual band
	"HD9310": 0x8042437, // TAU1302: L1/L2 dual band
}

// hwPlan returns the identity-deduced capability plan. RTCM capability
// is family-granular (HD + two digits): the HD80xx have no RTCM output
// (every 0xF8 CFG-MSG target NAKs on the TAU1201), while all HD93xx do
// (owner) and the HD95xx TAU951M is hardware-verified. Unknown
// families claim RTCM so NAK-driven absence can tell the truth.
func (c *Configurator) hwPlan() hwPlan {
	chip := hwChipNumber(c.ver.HwVersion.String())
	plan := hwPlan{signals: ^asbin.CfgNavSatMask(0), rtcm: !strings.HasPrefix(chip, "HD80")}
	if mask, ok := signalPlans[chip]; ok {
		plan.signals = mask
	}
	return plan
}

// hwChipNumber extracts the leading HDxxxx chip number, or returns the
// whole string unchanged if it does not start with HD plus a digit.
func hwChipNumber(hw string) string {
	if !strings.HasPrefix(hw, "HD") {
		return hw
	}
	n := len(hw)
	for i := 2; i < len(hw); i++ {
		if hw[i] < '0' || hw[i] > '9' {
			n = i
			break
		}
	}
	if n == 2 {
		return hw
	}
	return hw[:n]
}

// generateSignalSet sends the requested signal selection, intersected
// with the identity-deduced supported set. The acknowledged values are
// still not the whole truth: on unknown hardware the silicon clamps
// the written mask to its capability, and coupling is possible, so the
// achieved set always comes from one readback.
func (c *Configurator) generateSignalSet() {
	requested, ok := c.target.Props.GetSignalsEnabled()
	if !ok {
		return
	}
	if c.navSat == nil {
		return // property absent on this receiver
	}
	ns := asbin.CfgNavSat{EnableMask: signalsToNavSat(requested) & c.hwPlan().signals}
	c.addSetReq(&ns, nil)
	c.pollNavSat()
}

// signalConfigProps reports the enabled signal set from the readback.
func (c *Configurator) signalConfigProps(props *gpsprot.ConfigProps) {
	if c.navSat != nil {
		props.SetSignalsEnabled(navSatToSignals(c.navSat.EnableMask))
	}
}
