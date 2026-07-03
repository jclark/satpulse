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
func (c *Configurator) minElevConfigProps(props *gpsprot.ConfigProps) {
	if c.elev == nil {
		return
	}
	props.SetMinElevation(gpsprot.DegreesFromFloat(float64(c.elev.NaviMask) * 180 / math.Pi))
}
