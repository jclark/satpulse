package as

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// Speed changes: a CFG-PRT baud set always switches the live rate of
// the port it ARRIVES on, immediately - the portID field selects only
// which stored record is written - and the transition destroys the
// ACK at both rates (hardware-established). So the change is one set
// of record 0 carrying the host-side speed switch, confirmed by a
// solicited CFG-ELEV poll pipelined at the new rate (a distinct
// class+id, so it does not queue behind the unconfirmable set), with
// MaybeSpeedChangeSucceeded from incidental traffic as the secondary
// path. Once confirmed, a second set aligns record 1 at the new rate
// (a no-rate-change set, which ACKs normally), so both stored records
// agree and boot behavior is deterministic whatever the physical
// wiring. Which UART the host is on is unknowable and irrelevant.

// generateSpeedReqs generates the baud rate change and, for readback
// targets, the CFG-PRT record polls.
func (c *Configurator) generateSpeedReqs() {
	baud, ok := c.target.Props.GetBaudRate()
	if !ok {
		return
	}
	m := &asbin.CfgPrt{PortID: 0, Baudrate: baud}
	c.speedReq = c.addMsg(m, asReq{speedAfter: int(baud)})
	c.addPollReq(asbin.CfgElevID, func(asbin.Msg) {
		if c.speedReq.state == reqAwaitingAck {
			c.speedReq.state = reqSucceeded
		}
	})
	c.addSetReq(&asbin.CfgPrt{PortID: 1, Baudrate: baud}, nil)
}

// generatePrtQuery polls both CFG-PRT records when the target asks to
// read the serial speed and no new speed is being set.
func (c *Configurator) generatePrtQuery() {
	if c.target.Get&gpsprot.PropIDBaudRate == 0 || c.target.Props.SetsAny(gpsprot.PropIDBaudRate) {
		return
	}
	for port := range 2 {
		pkt := asbin.PollPrt(port)
		c.add(&asReq{mid: asbin.CfgPrtID, packet: pkt, optional: true,
			onData: func(m asbin.Msg) {
				if prt, ok := m.(*asbin.CfgPrt); ok && prt.PortID == 0 {
					c.prt0 = prt
				}
			}})
	}
}

// speedConfigProps reports the serial speed: a confirmed change
// reports the new rate; a readback target reports record 0's stored
// rate. The active UART is not identifiable (and a set always targets
// the arriving port), so no port name is ever reported.
func (c *Configurator) speedConfigProps(props *gpsprot.ConfigProps) {
	if c.speedReq != nil && c.speedReq.state == reqSucceeded {
		props.SetBaudRate(uint32(c.speedReq.speedAfter))
	} else if c.prt0 != nil {
		props.SetBaudRate(c.prt0.Baudrate)
	}
}
