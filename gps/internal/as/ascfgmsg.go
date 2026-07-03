package as

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
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
// complete). These are not nakOK: every tested unit accepts all seven
// ids, so a NAK is a genuine refusal. GSV goes first because it
// carries the most traffic, so turning it off frees the line soonest.
func (c *Configurator) generateNMEAReqs(flags gpsprot.NMEAMsgFlags) {
	msgs := []struct {
		flag gpsprot.NMEAMsgFlags
		mid  asbin.MsgID
	}{
		{gpsprot.NMEAMsgGSV, asbin.NmeaGsvID},
		{gpsprot.NMEAMsgRMC, asbin.NmeaRmcID},
		{gpsprot.NMEAMsgGGA, asbin.NmeaGgaID},
		{gpsprot.NMEAMsgGSA, asbin.NmeaGsaID},
		{gpsprot.NMEAMsgZDA, asbin.NmeaZdaID},
		{gpsprot.NMEAMsgVTG, asbin.NmeaVtgID},
		{gpsprot.NMEAMsgGLL, asbin.NmeaGllID},
	}
	for _, m := range msgs {
		var rate uint8
		if flags&m.flag != 0 {
			rate = 1
		}
		cls, id := m.mid.Unpack()
		c.addReq(&asbin.CfgMsg{MsgClass: cls, MsgID: id, Rate: rate})
	}
}
