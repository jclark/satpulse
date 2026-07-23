package casic

import (
	"math"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/casmsg"
)

// Minimum elevation lives in CFG-NAVLIMIT on V6 and in CFG-NAVX on V5.
// The V6 set is a read-modify-write of CFG-NAVLIMIT; the V5 set is
// mask-applied (only the minimum elevation field takes effect), but the
// readback still comes from the CFG-NAVX poll shared with signal
// selection.

// needsMinElev reports whether the target involves minimum elevation.
func (c *Configurator) needsMinElev() bool {
	return c.target.UsesAny(gpsprot.PropIDMinElevation)
}

// generateMinElevQuery polls the message holding minimum elevation.
// On V5 the signal query may already poll CFG-NAVX.
func (c *Configurator) generateMinElevQuery() {
	if !c.needsMinElev() {
		return
	}
	if c.family == familyV6 {
		c.addPollReq(casbin.CfgNavLimID, func(m casbin.Msg) {
			if nl, ok := m.(*casbin.CfgNavLimit); ok {
				c.navLimit = nl
			}
		})
		return
	}
	if !c.needsSignals() {
		c.pollNavx()
	}
}

// generateMinElevSet sends the minimum elevation.
func (c *Configurator) generateMinElevSet() {
	elev, ok := c.target.Props.GetMinElevation()
	if !ok {
		return
	}
	// Ceil, as in the ubx conversion: rounding down would admit
	// satellites the user excluded.
	v := int64(math.Ceil(elev.Degrees()))
	if v < -90 || v > 90 {
		return
	}
	deg := int8(v)
	if c.family == familyV6 {
		if c.navLimit == nil {
			return // property absent on this receiver
		}
		nl := *c.navLimit
		nl.MinElev = deg
		c.addSetReq(&nl, func() { c.navLimit = &nl })
		return
	}
	if c.navx == nil {
		return
	}
	c.addSetReq(&casbin.CfgNavx{Mask: casbin.CfgNavxApplyMinElev, MinElev: deg},
		func() { c.navx.MinElev = deg })
}

// minElevConfigProps reports minimum elevation from the readback.
func (c *Configurator) minElevConfigProps(props *gpsprot.ConfigProps) {
	if c.family == familyV6 {
		if c.navLimit != nil {
			props.SetMinElevation(gpsprot.DegreesFromFloat(float64(c.navLimit.MinElev)))
		}
		return
	}
	if c.navx != nil {
		props.SetMinElevation(gpsprot.DegreesFromFloat(float64(c.navx.MinElev)))
	}
}

// generateVerQuery asks a V5 receiver for its version over NMEA: V5
// firmware does not answer MON-VER, but PCAS06 queries reply with
// GPTXT key=value sentences. The replies are optional - a receiver
// that never answers just leaves ReceiverInfo empty.
func (c *Configurator) generateVerQuery() {
	if c.ver != nil {
		return
	}
	c.addInfoQuery(casmsg.QueryFirmwareVersion, "SW", &c.pcasSW)
	c.addInfoQuery(casmsg.QueryHardware, "HW", &c.pcasHW)
}

// addInfoQuery sends a PCAS06 query and stores the value of the GPTXT
// reply carrying the given key.
func (c *Configurator) addInfoQuery(query int, key string, dst *string) {
	c.addTextReq(casmsg.Query(query), func(payload string) bool {
		k, v, ok := casmsg.ParseTxtInfo(payload)
		if !ok || k != key {
			return false
		}
		*dst = v
		return true
	})
}
