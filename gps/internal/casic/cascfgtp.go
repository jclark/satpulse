package casic

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// Time pulse configuration uses CFG-TP, shared by both families with
// diverging enum semantics:
//
// PPSOutMode: V5 0=off, 1=on, 2=maintain after fix lost, 3=fix only;
// V6 0=off, 1=time known, 2=sat sync, 3=pos+time valid, 5=reliable,
// 7=always on.
//
// TBase: V6 0=GNSS, 1=UTC; V5 INVERTED: 0=UTC, 1=satellite time.
//
// TSrcMode: V5 0=GPS, 1=BDS, 2=GLN (forced), 4-6 primary BDS/GPS/GLN;
// V6 0-3 force GPS/BDS/GLN/GAL, 4-8 primary, 9 auto.
//
// A set is a read-modify-write: the query phase polls CFG-TP and the
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
			tp.PPSOutMode = 0
		} else {
			tp.Width = uint32(w.Round(time.Microsecond) / time.Microsecond)
			if tp.PPSOutMode == 0 {
				tp.PPSOutMode = c.ppsOutMode(false)
			}
		}
	}
	if p, ok := props.GetTimePulsePeriod(); ok {
		tp.Interval = uint32(p.Round(time.Microsecond) / time.Microsecond)
	}
	if r, ok := props.GetTimePulsePolarityRising(); ok {
		tp.Polarity = 0
		if !r {
			tp.Polarity = 1
		}
	}
	if l, ok := props.GetTimePulseOnlyWhenLocked(); ok && tp.PPSOutMode != 0 {
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
		tp.UserDelay = float32(d.Seconds())
	}
	c.addSetReq(&tp, func() { c.tp = &tp })
}

// ppsOutMode returns the pulse output mode for an enabled pulse.
func (c *Configurator) ppsOutMode(onlyWhenLocked bool) uint8 {
	if c.family == familyV6 {
		if onlyWhenLocked {
			return 5 // reliable position and time
		}
		return 7 // always on
	}
	if onlyWhenLocked {
		return 3 // fix only
	}
	return 1 // on
}

// onlyWhenLocked interprets a PPSOutMode readback.
func (c *Configurator) onlyWhenLocked(mode uint8) bool {
	if c.family == familyV6 {
		return mode == 2 || mode == 3 || mode == 5
	}
	return mode == 3
}

// tBase returns the time base for an alignment choice; V5 inverts the
// encoding.
func (c *Configurator) tBase(alignToGNSS bool) uint8 {
	gnss := alignToGNSS
	if c.family == familyV5 {
		gnss = !gnss
	}
	if gnss {
		return 0
	}
	return 1
}

// alignToGNSS interprets a TBase readback (inverse of tBase).
func (c *Configurator) alignToGNSS(tBase uint8) bool {
	align := tBase == 0
	if c.family == familyV5 {
		align = !align
	}
	return align
}

// tSrcMode returns the forced time source for a GNSS, when this family
// has one.
func (c *Configurator) tSrcMode(g gpsprot.GNSS) (uint8, bool) {
	switch g {
	case gpsprot.GPS:
		return 0, true
	case gpsprot.BDS:
		return 1, true
	case gpsprot.GLO:
		return 2, true
	case gpsprot.GAL:
		if c.family == familyV6 {
			return 3, true
		}
	}
	return 0, false
}

// timeGNSS interprets a TSrcMode readback, covering both the forced
// and the primary ("main") encodings. The GAL values exist on V6 only.
func (c *Configurator) timeGNSS(src uint8) (gpsprot.GNSS, bool) {
	switch src {
	case 0, 5:
		return gpsprot.GPS, true
	case 1, 4:
		return gpsprot.BDS, true
	case 2, 6:
		return gpsprot.GLO, true
	case 3, 7:
		if c.family == familyV6 {
			return gpsprot.GAL, true
		}
	}
	return 0, false
}

// tpConfigProps reports the time pulse properties from the latest
// CFG-TP readback.
func (c *Configurator) tpConfigProps(props *gpsprot.ConfigProps) {
	if c.tp == nil {
		return
	}
	width := time.Duration(c.tp.Width) * time.Microsecond
	if c.tp.PPSOutMode == 0 {
		width = 0
	}
	props.SetTimePulse(gpsprot.TimePulse{
		Width:          width,
		Period:         time.Duration(c.tp.Interval) * time.Microsecond,
		AlignToGNSS:    c.alignToGNSS(c.tp.TBase),
		OnlyWhenLocked: c.onlyWhenLocked(c.tp.PPSOutMode),
		PolarityRising: c.tp.Polarity == 0,
	})
	if g, ok := c.timeGNSS(c.tp.TSrcMode); ok {
		props.SetTimeGNSS(g)
	}
	props.SetAntennaCableDelay(time.Duration(math.Round(float64(c.tp.UserDelay) * float64(time.Second))))
}
