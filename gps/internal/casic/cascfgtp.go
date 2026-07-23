package casic

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// Time pulse configuration uses CFG-TP, whose output-mode, time-base,
// and time-source values differ between V5 and V6; casbin defines the
// family-specific wire values. A set is a read-modify-write: the query
// phase polls CFG-TP and the
// set phase merges the target properties into the readback. The
// encoding fully determines what the receiver stores (microsecond
// fields, float32 delay), so the acknowledged set is recorded as the
// assumed configuration.

// tpProps are the properties realized by CFG-TP. The antenna cable
// delay maps to the CFG-TP user time delay field.
const tpProps = gpsprot.PropIDTimePulse | gpsprot.PropIDTimeGNSS | gpsprot.PropIDAntennaCableDelay

// needsTP reports whether the target involves the time pulse.
func (c *Configurator) needsTP() bool {
	return c.target.UsesAny(tpProps)
}

// setsTP reports whether the target changes the time pulse.
func (c *Configurator) setsTP() bool {
	return c.target.Props.SetsAny(tpProps)
}

// generateTPQuery polls CFG-TP when the target involves the time pulse.
func (c *Configurator) generateTPQuery() {
	if !c.needsTP() {
		return
	}
	c.addPollReq(casbin.CfgTPID, func(m casbin.Msg) {
		if tp, ok := m.(*casbin.CfgTP); ok {
			c.tp = tp
		}
	})
}

// generateTPSet merges the target's time pulse properties into the
// CFG-TP readback and sends the result. Without a readback (the poll
// was NAKed or unanswered) there is nothing to merge into: the
// property does not exist on this receiver and nothing is sent.
func (c *Configurator) generateTPSet() {
	if !c.setsTP() || c.tp == nil {
		return
	}
	tp := *c.tp
	props := &c.target.Props
	if w, ok := props.GetTimePulseWidth(); ok {
		if w == 0 {
			tp.PPSOutMode = c.ppsOffMode()
		} else {
			tp.Width = casbin.CfgTPMicroseconds(w)
			if tp.PPSOutMode == c.ppsOffMode() {
				tp.PPSOutMode = c.ppsOutMode(false)
			}
		}
	}
	if p, ok := props.GetTimePulsePeriod(); ok {
		tp.Interval = casbin.CfgTPMicroseconds(p)
	}
	if r, ok := props.GetTimePulsePolarityRising(); ok {
		tp.Polarity = casbin.CfgTPPolarityRising
		if !r {
			tp.Polarity = casbin.CfgTPPolarityFalling
		}
	}
	if l, ok := props.GetTimePulseOnlyWhenLocked(); ok && tp.PPSOutMode != c.ppsOffMode() {
		tp.PPSOutMode = c.ppsOutMode(l)
	}
	if a, ok := props.GetTimePulseAlignToGNSS(); ok {
		tp.TBase = c.tBase(a)
	}
	if g, ok := props.GetTimeGNSS(); ok {
		if src, ok := c.tSrcMode(g); ok {
			tp.TSrcMode = src
		}
	}
	if d, ok := props.GetAntennaCableDelay(); ok {
		tp.UserDelay = casbin.CfgTPUserDelaySeconds(d)
	}
	if tp == *c.tp {
		return
	}
	c.addSetReq(&tp, func() { c.tp = &tp })
}

// ppsOffMode returns this family's disabled pulse-output mode.
func (c *Configurator) ppsOffMode() casbin.CfgTPPPSOutMode {
	if c.family == familyV6 {
		return casbin.CfgTPPPSOutV6Off
	}
	return casbin.CfgTPPPSOutV5Off
}

// ppsOutMode returns the pulse output mode for an enabled pulse.
func (c *Configurator) ppsOutMode(onlyWhenLocked bool) casbin.CfgTPPPSOutMode {
	if c.family == familyV6 {
		if onlyWhenLocked {
			return casbin.CfgTPPPSOutV6PositionTimeReliable
		}
		return casbin.CfgTPPPSOutV6AlwaysOn
	}
	if onlyWhenLocked {
		return casbin.CfgTPPPSOutV5FixOnly
	}
	return casbin.CfgTPPPSOutV5On
}

// onlyWhenLocked interprets a PPSOutMode readback.
func (c *Configurator) onlyWhenLocked(mode casbin.CfgTPPPSOutMode) bool {
	if c.family == familyV6 {
		return mode == casbin.CfgTPPPSOutV6SatelliteSync ||
			mode == casbin.CfgTPPPSOutV6PositionTimeValid ||
			mode == casbin.CfgTPPPSOutV6PositionTimeReliable
	}
	return mode == casbin.CfgTPPPSOutV5FixOnly
}

// tBase returns the time base for an alignment choice; V5 inverts the
// encoding.
func (c *Configurator) tBase(alignToGNSS bool) casbin.CfgTPTBase {
	if c.family == familyV5 {
		if alignToGNSS {
			return casbin.CfgTPTBaseV5Satellite
		}
		return casbin.CfgTPTBaseV5UTC
	}
	if alignToGNSS {
		return casbin.CfgTPTBaseV6GNSS
	}
	return casbin.CfgTPTBaseV6UTC
}

// alignToGNSS interprets a TBase readback (inverse of tBase).
func (c *Configurator) alignToGNSS(tBase casbin.CfgTPTBase) bool {
	if c.family == familyV5 {
		return tBase == casbin.CfgTPTBaseV5Satellite
	}
	return tBase == casbin.CfgTPTBaseV6GNSS
}

// tSrcMode returns the forced time source for a GNSS, when this family
// has one.
func (c *Configurator) tSrcMode(g gpsprot.GNSS) (casbin.CfgTPTSrcMode, bool) {
	if c.family == familyV6 {
		switch g {
		case gpsprot.GPS:
			return casbin.CfgTPTSrcV6ForceGPS, true
		case gpsprot.BDS:
			return casbin.CfgTPTSrcV6ForceBDS, true
		case gpsprot.GLO:
			return casbin.CfgTPTSrcV6ForceGLN, true
		case gpsprot.GAL:
			return casbin.CfgTPTSrcV6ForceGAL, true
		}
		return casbin.CfgTPTSrcV6ForceGPS, false
	}
	switch g {
	case gpsprot.GPS:
		return casbin.CfgTPTSrcV5ForceGPS, true
	case gpsprot.BDS:
		return casbin.CfgTPTSrcV5ForceBDS, true
	case gpsprot.GLO:
		return casbin.CfgTPTSrcV5ForceGLN, true
	}
	return casbin.CfgTPTSrcV5ForceGPS, false
}

// timeGNSS interprets a TSrcMode readback, covering both the forced
// and the primary ("main") encodings. The GAL values exist on V6 only.
func (c *Configurator) timeGNSS(src casbin.CfgTPTSrcMode) (gpsprot.GNSS, bool) {
	if c.family == familyV6 {
		switch src {
		case casbin.CfgTPTSrcV6ForceGPS, casbin.CfgTPTSrcV6PrimaryGPS:
			return gpsprot.GPS, true
		case casbin.CfgTPTSrcV6ForceBDS, casbin.CfgTPTSrcV6PrimaryBDS:
			return gpsprot.BDS, true
		case casbin.CfgTPTSrcV6ForceGLN, casbin.CfgTPTSrcV6PrimaryGLN:
			return gpsprot.GLO, true
		case casbin.CfgTPTSrcV6ForceGAL, casbin.CfgTPTSrcV6PrimaryGAL:
			return gpsprot.GAL, true
		}
		return 0, false
	}
	switch src {
	case casbin.CfgTPTSrcV5ForceGPS, casbin.CfgTPTSrcV5PrimaryGPS:
		return gpsprot.GPS, true
	case casbin.CfgTPTSrcV5ForceBDS, casbin.CfgTPTSrcV5PrimaryBDS:
		return gpsprot.BDS, true
	case casbin.CfgTPTSrcV5ForceGLN, casbin.CfgTPTSrcV5PrimaryGLN:
		return gpsprot.GLO, true
	}
	return 0, false
}

// tpConfigProps reports the time pulse properties from the latest
// CFG-TP readback.
func (c *Configurator) tpConfigProps(props *gpsprot.ConfigProps) {
	if c.tp == nil {
		return
	}
	width := casbin.CfgTPDuration(c.tp.Width)
	if c.tp.PPSOutMode == c.ppsOffMode() {
		width = 0
	}
	props.SetTimePulse(gpsprot.TimePulse{
		Width:          width,
		Period:         casbin.CfgTPDuration(c.tp.Interval),
		AlignToGNSS:    c.alignToGNSS(c.tp.TBase),
		OnlyWhenLocked: c.onlyWhenLocked(c.tp.PPSOutMode),
		PolarityRising: c.tp.Polarity == casbin.CfgTPPolarityRising,
	})
	if g, ok := c.timeGNSS(c.tp.TSrcMode); ok {
		props.SetTimeGNSS(g)
	}
	props.SetAntennaCableDelay(casbin.CfgTPUserDelayDuration(c.tp.UserDelay))
}
