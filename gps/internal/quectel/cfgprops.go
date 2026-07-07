package quectel

import (
	"fmt"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
)

// signalBits maps each PQTMCFGSIGNAL mask bit to its gpsprot signal,
// in the sentence's field order (GPS, GLONASS, Galileo, BDS, QZSS,
// NavIC); the slice index is the bit number.
var signalBits = [6][]gpsprot.Signal{
	{gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C, gpsprot.SigGPSL5},
	{gpsprot.SigGLOL1, gpsprot.SigGLOL2},
	{gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b, gpsprot.SigGALE6},
	{gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C,
		gpsprot.SigBDSB2a, gpsprot.SigBDSB2b},
	{gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5, gpsprot.SigQZSSL6},
	{gpsprot.SigNAVICL5},
}

// signalUniverse returns every signal with a CFGSIGNAL mask bit.
func signalUniverse() gpsprot.SignalSet {
	var ss gpsprot.SignalSet
	for _, sigs := range signalBits {
		for _, sig := range sigs {
			ss |= 1 << sig
		}
	}
	return ss
}

func signalMasks(m *qtmmsg.CfgSignal) [6]*qtmmsg.HexByte {
	return [6]*qtmmsg.HexByte{&m.GPSSig, &m.GLOSig, &m.GALSig, &m.BDSSig, &m.QZSSig, &m.NACSig}
}

func cnstGates(m *qtmmsg.CfgCnst) [6]*uint8 {
	return [6]*uint8{&m.GPS, &m.GLONASS, &m.Galileo, &m.BDS, &m.QZSS, &m.NavIC}
}

// convertToProps populates props from the as-found tuples. A nil
// tuple contributes nothing: its properties are absent.
func (f *asFound) convertToProps(props *gpsprot.ConfigProps) {
	if f.signal != nil && f.cnst != nil {
		masks, gates := signalMasks(f.signal), cnstGates(f.cnst)
		var ss gpsprot.SignalSet
		for i, sigs := range signalBits {
			if *gates[i] == 0 {
				continue
			}
			for bit, sig := range sigs {
				if *masks[i]&(1<<bit) != 0 {
					ss |= 1 << sig
				}
			}
		}
		props.SetSignalsEnabled(ss)
	}
	if f.uart != nil {
		props.SetBaudRate(f.uart.BaudRate)
		props.SetPort(fmt.Sprintf("UART%d", f.uart.Index))
	}
	if f.rcvrMode != nil && f.svin != nil {
		var m gpsprot.Mode
		// Position modes exist only under base receiver mode: a stored
		// SVIN mode is inert in rover mode (hardware-verified), so the
		// effective mode is mobile unless the receiver is a base.
		if f.rcvrMode.Mode == qtmmsg.RcvrModeBase {
			switch f.svin.Mode {
			case qtmmsg.SvinModeSurveyIn:
				m.Static = true
			case qtmmsg.SvinModeFixed:
				m.Static = true
				m.PosType = gpsprot.PosTypeECEF
				m.FixedPosECEF = gpsprot.Point3D{
					gpsprot.Meters(float64(f.svin.ECEFX)),
					gpsprot.Meters(float64(f.svin.ECEFY)),
					gpsprot.Meters(float64(f.svin.ECEFZ))}
			}
		}
		props.SetMode(m)
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
	c.generateSignalSets()
	c.generateModeSet()
	return c.generateTimePulseSet()
}

// generateModeSet builds the RCVRMODE/SVIN writes for the target's
// positioning mode. Both are stored-only, and position modes exist
// only under base receiver mode (hardware-verified), so the writes
// ride the same explicit save+reset gate as signals; without it the
// configurator does nothing, and the skip shows in the effective-mode
// readback. Entering base mode swaps
// the per-mode message table (NMEA off, RTCM on) and forces a 1 Hz
// fix rate; message output stays configurable live in base mode, so
// a daemon restores its feed on its next configuration pass.
func (c *Configurator) generateModeSet() {
	if c.found.rcvrMode == nil || c.found.svin == nil {
		return
	}
	mode, ok := c.target.Props.GetMode()
	// SetStatic means static without disturbing an existing fixed
	// position or survey: an effectively static receiver is left as
	// found; anything else is treated as a Mode{Static: true} survey
	// request, even over a non-static Mode property (ubx semantics).
	if c.target.Opts.SetStatic && (!ok || !mode.Static) {
		if c.effectivelyStatic() {
			return
		}
		mode, ok = gpsprot.Mode{Static: true}, true
	}
	if !ok {
		return
	}
	var svin qtmmsg.CfgSvin
	rcvr := uint8(qtmmsg.RcvrModeRover)
	if mode.Static {
		rcvr = qtmmsg.RcvrModeBase
		switch mode.PosType {
		case gpsprot.PosTypeECEF:
			svin = fixedSvin(geopos.ECEF{mode.FixedPosECEF[0].Meters(),
				mode.FixedPosECEF[1].Meters(), mode.FixedPosECEF[2].Meters()})
		case gpsprot.PosTypeLLH:
			svin = fixedSvin(geopos.WGS84.LLHtoECEF(geopos.LLH{
				Lat: mode.FixedPosLLH[0].Degrees(), Lon: mode.FixedPosLLH[1].Degrees(),
				Height: mode.Height.Meters()}))
		default:
			s := &c.target.Opts.Survey
			svin = qtmmsg.CfgSvin{Mode: qtmmsg.SvinModeSurveyIn,
				CfgCnt:     uint32(s.MinDur / time.Second),
				AccLimit3D: qtmmsg.Fixed1(s.AccLimit.Meters())}
		}
	}
	if c.found.rcvrMode.Mode == rcvr && *c.found.svin == svin {
		return
	}
	if !c.savesAndResets() {
		return
	}
	if c.found.rcvrMode.Mode != rcvr {
		m := &qtmmsg.CfgRcvrMode{Mode: rcvr}
		c.reqs = append(c.reqs, c.propSet(m, func() { c.found.rcvrMode = m }))
	}
	if *c.found.svin != svin {
		m := svin
		c.reqs = append(c.reqs, c.propSet(&m, func() { c.found.svin = &m }))
	}
}

// effectivelyStatic reports whether the as-found state is an
// effective static mode: base receiver mode with a survey-in or
// fixed SVIN. A stored SVIN is inert in rover mode. Precondition:
// found.rcvrMode and found.svin are non-nil.
func (c *Configurator) effectivelyStatic() bool {
	return c.found.rcvrMode.Mode == qtmmsg.RcvrModeBase &&
		(c.found.svin.Mode == qtmmsg.SvinModeSurveyIn || c.found.svin.Mode == qtmmsg.SvinModeFixed)
}

func fixedSvin(p geopos.ECEF) qtmmsg.CfgSvin {
	return qtmmsg.CfgSvin{Mode: qtmmsg.SvinModeFixed, ECEFX: qtmmsg.Fixed4(p[0]),
		ECEFY: qtmmsg.Fixed4(p[1]), ECEFZ: qtmmsg.Fixed4(p[2])}
}

// generateSignalSets builds the CFGSIGNAL/CFGCNST writes for the
// target's SignalsEnabled. Both commands are stored-only: they take
// effect at the next NVM reload, never live. Owner ruling: the writes
// are performed only when the target also saves and restarts, so
// persistence and the ~15 s outage are explicitly user-requested;
// otherwise the configurator does nothing and never leaves
// stored-but-ineffective state behind - the skip shows in the
// readback, from which frontends derive any warning. Requested
// signals the receiver has no mask bit for
// are dropped by intersection; a constellation with no requested
// signals is disabled via its CFGCNST gate, its mask kept as found.
func (c *Configurator) generateSignalSets() {
	ss, ok := c.target.Props.GetSignalsEnabled()
	if !ok || c.found.signal == nil || c.found.cnst == nil {
		return
	}
	sig, cnst := *c.found.signal, *c.found.cnst
	masks, gates := signalMasks(&sig), cnstGates(&cnst)
	for i, sigs := range signalBits {
		var mask qtmmsg.HexByte
		for bit, s := range sigs {
			if s == gpsprot.SigQZSSL6 && !c.fw.has(fwQZSSL6) {
				continue
			}
			if ss.Contains(s) {
				mask |= 1 << bit
			}
		}
		if mask != 0 {
			*masks[i] = mask
			*gates[i] = 1
		} else {
			*gates[i] = 0
		}
	}
	if sig == *c.found.signal && cnst == *c.found.cnst {
		return
	}
	if !c.savesAndResets() {
		return
	}
	if sig != *c.found.signal {
		m := sig
		c.reqs = append(c.reqs, c.propSet(&m, func() { c.found.signal = &m }))
	}
	if cnst != *c.found.cnst {
		m := cnst
		c.reqs = append(c.reqs, c.propSet(&m, func() { c.found.cnst = &m }))
	}
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
