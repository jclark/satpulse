package as

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// Minimum elevation lives in CFG-ELEV's naviMask (radians): satellites
// below it are excluded from the position fix. A set is a
// read-modify-write preserving the separate tracking mask; the
// acknowledged set is recorded as the assumed configuration (ELEV
// echoes exactly - stage 0).

// needsMinElev reports whether the target involves minimum elevation.
func (c *Configurator) needsMinElev() bool {
	return c.target.UsesAny(gpsprot.PropIDMinElevation)
}

// generateMinElevQuery polls CFG-ELEV. The speed phase also polls it
// as its confirmation packet, but that instance needs no readback.
func (c *Configurator) generateMinElevQuery() {
	if !c.needsMinElev() {
		return
	}
	c.addPollReq(asbin.CfgElevID, func(m asbin.Msg) {
		if el, ok := m.(*asbin.CfgElev); ok {
			c.elev = el
		}
	})
}

// generateMinElevSet merges the minimum elevation into the CFG-ELEV
// readback, preserving the tracking mask.
func (c *Configurator) generateMinElevSet() {
	deg, ok := c.target.Props.GetMinElevation()
	if !ok || c.elev == nil {
		return
	}
	el := *c.elev
	el.NaviMask = float32(deg.Degrees() * math.Pi / 180)
	c.addSetReq(&el, func() { c.elev = &el })
}

// minElevConfigProps reports minimum elevation from the readback.
// naviMask is a float32 in radians, which cannot hold a whole degree
// exactly: 5 deg reads back as 4.999999855. Round to millidegrees,
// which is far coarser than the worst roundtrip error (3.3e-6 deg over
// 0-90 deg) and far finer than any usable mask, so what was asked for
// is what is reported.
func (c *Configurator) minElevConfigProps(props *gpsprot.ConfigProps) {
	if c.elev == nil {
		return
	}
	deg := float64(c.elev.NaviMask) * 180 / math.Pi
	props.SetMinElevation(gpsprot.Angle(math.Round(deg*1000)) * gpsprot.Millidegrees)
}
