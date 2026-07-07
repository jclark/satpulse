package quectel

import (
	"fmt"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
)

// convertToProps populates props from the as-found tuples. A nil
// tuple contributes nothing: its properties are absent.
func (f *asFound) convertToProps(props *gpsprot.ConfigProps) {
	if f.uart != nil {
		props.SetBaudRate(f.uart.BaudRate)
		props.SetPort(fmt.Sprintf("UART%d", f.uart.Index))
	}
	if f.eleThd != nil {
		props.SetMinElevation(gpsprot.DegreesFromFloat(float64(f.eleThd.Ele)))
	}
	if f.rsid != nil {
		props.SetRTCMBaseID(f.rsid.ID)
	}
	if f.pps2 != nil {
		setTimePulse(props, f.pps2.Enable, f.pps2.Duration, f.pps2.Mode, f.pps2.Polarity,
			time.Duration(f.pps2.Period)*time.Millisecond)
	} else if f.pps != nil {
		setTimePulse(props, f.pps.Enable, f.pps.Duration, f.pps.Mode, f.pps.Polarity, time.Second)
	}
}

// generatePropSets appends property set requests for the target's
// requested properties, as minimal read-modify-writes against the
// as-found tuples: a property already at its requested value
// generates no request. It returns an error for a requested value
// the configurator itself cannot express.
func (c *Configurator) generatePropSets() error {
	props := &c.target.Props
	if deg, ok := props.GetMinElevation(); ok && c.fw.has(fwEleThd) {
		ele := qtmmsg.Fixed1(deg.Degrees())
		if c.found.eleThd == nil || c.found.eleThd.Ele != ele {
			m := &qtmmsg.CfgEleThd{Ele: ele}
			c.reqs = append(c.reqs, c.propSet(m, func() { c.found.eleThd = m }))
		}
	}
	if id, ok := props.GetRTCMBaseID(); ok {
		if c.found.rsid == nil || c.found.rsid.ID != id {
			m := &qtmmsg.CfgRSID{ID: id}
			c.reqs = append(c.reqs, c.propSet(m, func() { c.found.rsid = m }))
		}
	}
	return c.generateTimePulseSet()
}

// propSet builds a property set request. A code 3 refusal
// (unsupported command) is absence; codes 1 and 2 refuse a genuine
// value request and fail the request. The acknowledgement records
// the request's own tuple as the assumed configuration.
func (c *Configurator) propSet(m qtmmsg.CfgMsg, onOK func()) *request {
	payload, err := qtmmsg.EncodeWrite(m)
	if err != nil {
		panic(err)
	}
	return &request{
		phase:    phaseSet,
		payload:  payload,
		sentence: m.Sentence(),
		onOK:     onOK,
		onError:  func(rc qtmmsg.ResponseClass) bool { return rc.ErrCode == "3" },
	}
}

// generateTimePulseSet builds the time-pulse write when the target
// sets any pulse property: the target's values applied over the
// as-found tuple (or the documented defaults when the as-found pulse
// is disabled, whose readback hides the stored values). Writes use
// the legacy CfgPPS form only - it exists on every firmware version
// and preserves the PPS2-only Period and Userdelay fields - so the
// pulse period is fixed at 1 s and a target requesting any other
// period is refused here. (The PPS2 write form is also no wider:
// hardware rejects Duration > 900 ms despite the spec's (0, Period]
// range.)
func (c *Configurator) generateTimePulseSet() error {
	props := &c.target.Props
	if !props.SetsAny(gpsprot.PropIDTimePulse) {
		return nil
	}
	if p, ok := props.GetTimePulsePeriod(); ok && p != time.Second {
		return fmt.Errorf("time pulse period is fixed at 1s, cannot set %v", p)
	}
	base := qtmmsg.CfgPPS{Index: 1, Enable: 1, Duration: 100, Mode: qtmmsg.PPSModeAlways, Polarity: 1}
	enable := true
	if f := c.found.pps2; f != nil {
		if f.Enable != 0 {
			base.Duration, base.Mode, base.Polarity = f.Duration, f.Mode, f.Polarity
		} else {
			enable = false
		}
	} else if f := c.found.pps; f != nil {
		if f.Enable != 0 {
			base.Duration, base.Mode, base.Polarity = f.Duration, f.Mode, f.Polarity
		} else {
			enable = false
		}
	}
	if w, ok := props.GetTimePulseWidth(); ok {
		enable = w != 0
		if w != 0 {
			base.Duration = uint16(w / time.Millisecond)
		}
	}
	if b, ok := props.GetTimePulseOnlyWhenLocked(); ok {
		if b {
			base.Mode = qtmmsg.PPSModeFixOnly
		} else {
			base.Mode = qtmmsg.PPSModeAlways
		}
	}
	if b, ok := props.GetTimePulsePolarityRising(); ok {
		if b {
			base.Polarity = 1
		} else {
			base.Polarity = 0
		}
	}
	if !enable {
		base.Enable = 0
	}
	m := &base
	if f := c.found.pps2; f != nil && f.Enable == m.Enable &&
		(m.Enable == 0 || (f.Duration == m.Duration && f.Mode == m.Mode && f.Polarity == m.Polarity)) {
		return nil
	}
	if f := c.found.pps; c.found.pps2 == nil && f != nil && f.Enable == m.Enable &&
		(m.Enable == 0 || (f.Duration == m.Duration && f.Mode == m.Mode && f.Polarity == m.Polarity)) {
		return nil
	}
	c.reqs = append(c.reqs, c.propSet(m, func() {
		c.found.pps = m
		if f := c.found.pps2; f != nil {
			// A CfgPPS write leaves Period and Userdelay unchanged.
			f.Enable, f.Duration, f.Mode, f.Polarity = m.Enable, m.Duration, m.Mode, m.Polarity
		}
	}))
	return nil
}

// setTimePulse converts a PPS/PPS2 tuple. The pulse is always aligned
// to the GNSS second - the protocol has no alignment knob - so
// AlignToGNSS is reported as the constant true. A disabled tuple
// reads back truncated (Duration through Userdelay unreported), so
// only the zero width and the alignment constant are knowable then.
func setTimePulse(props *gpsprot.ConfigProps, enable uint8, durMs uint16, mode, polarity uint8, period time.Duration) {
	if enable == 0 {
		props.SetTimePulseWidth(0)
		props.SetTimePulseAlignToGNSS(true)
		return
	}
	props.SetTimePulse(gpsprot.TimePulse{
		Width:          time.Duration(durMs) * time.Millisecond,
		Period:         period,
		AlignToGNSS:    true,
		OnlyWhenLocked: mode == qtmmsg.PPSModeFixOnly,
		PolarityRising: polarity == 1,
	})
}
