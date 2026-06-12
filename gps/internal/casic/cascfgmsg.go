package casic

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// generateMsgReqs generates the message output requests for the target.
func (c *Configurator) generateMsgReqs() {
	opts := &c.target.Opts
	if opts.NMEAMsg.IsSet() {
		c.generateNMEAReqs(opts.NMEAMsg.Get())
	}
}

// generateNMEAReqs sets the rate of each standard NMEA sentence via
// CFG-MSG: named sentences on, the rest off (a wire-format request is
// complete). GSV goes first because it carries the most traffic, so
// turning it off frees a saturated line soonest.
func (c *Configurator) generateNMEAReqs(flags gpsprot.NMEAMsgFlags) {
	zda := casbin.NmeaZdaID
	if c.family == familyV6 {
		zda = casbin.NmeaZdaV6ID
	}
	msgs := []struct {
		flag gpsprot.NMEAMsgFlags
		mid  casbin.MsgID
	}{
		{gpsprot.NMEAMsgGSV, casbin.NmeaGsvID},
		{gpsprot.NMEAMsgRMC, casbin.NmeaRmcID},
		{gpsprot.NMEAMsgGGA, casbin.NmeaGgaID},
		{gpsprot.NMEAMsgGSA, casbin.NmeaGsaID},
		{gpsprot.NMEAMsgZDA, zda},
		{gpsprot.NMEAMsgVTG, casbin.NmeaVtgID},
		{gpsprot.NMEAMsgGLL, casbin.NmeaGllID},
	}
	for _, m := range msgs {
		var rate uint16
		if flags&m.flag != 0 {
			rate = 1
		}
		c.addReq(&casbin.CfgMsg{Target: m.mid, Rate: rate})
	}
}
