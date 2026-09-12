package as

import (
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// Signal selection uses the CFG-NAVSAT per-signal mask. The ACK's
// semantics here are intersection (established on all three test
// units): the receiver acknowledges any mask and enables the
// intersection with its capability - reasonable receiver semantics,
// not a defect - so the acknowledgement means "enabled requested AND
// supported" without naming the result, and the achieved set is read
// back to report the value the receiver says it enabled (the
// semantics require every silent intersection to be visible in the
// achieved set). The requested mask goes to the wire as-is; the
// silicon's clamp, revealed by the readback, is the only intersection.

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

// supportsRTCM reports whether the receiver has RTCM output. Capability
// keys off the MON-VER chip number (owner ruling: key off the identity
// the receiver reports): the HD8xxx family has no RTCM output - every
// 0xF8 CFG-MSG target NAKs on the TAU1201 - while HD9xxx and the D10P do.
// An unknown chip is assumed to have it, so NAK-driven absence tells the
// truth if the guess is wrong.
func (c *Configurator) supportsRTCM() bool {
	return !strings.HasPrefix(hwChipNumber(c.ver.HwVersion.String()), "HD8")
}

// hwChipNumber returns the chip name from the MON-VER hardware string:
// the part before the '.' that separates it from the per-unit hash
// (e.g. "HD9510.4740d9ec2" -> "HD9510", "D10PA.6511ab528" -> "D10PA").
// A string without a '.' is returned unchanged.
func hwChipNumber(hw string) string {
	chip, _, _ := strings.Cut(hw, ".")
	return chip
}

// generateSignalSet sends the requested signal selection. The silicon
// clamps signals it does not support (and the TAU1302 couples some), so
// the achieved set comes from the post-set readback: the ACK means
// "enabled what is supported" without naming it (see the file comment).
func (c *Configurator) generateSignalSet() {
	requested, ok := c.target.Props.GetSignalsEnabled()
	if !ok {
		return
	}
	if c.navSat == nil {
		return // property absent on this receiver
	}
	ns := asbin.CfgNavSat{EnableMask: signalsToNavSat(requested)}
	c.addSetReq(&ns, nil)
	c.pollNavSat()
}

// signalConfigProps reports the enabled signal set from the readback.
func (c *Configurator) signalConfigProps(props *gpsprot.ConfigProps) {
	if c.navSat != nil {
		props.SetSignalsEnabled(navSatToSignals(c.navSat.EnableMask))
	}
}
