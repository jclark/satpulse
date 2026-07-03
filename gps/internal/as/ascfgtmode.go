package as

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/geopos"
)

// Time mode (positioning mode) has no mode register on Allystar: the
// state IS the CFG-FIXEDECEF and CFG-SURVEY registers. A nonzero
// FIXEDECEF is position-hold ("fixed") mode; nonzero SURVEY parameters
// with a zero FIXEDECEF mean a survey is running, and its completion
// auto-writes the surveyed mean into FIXEDECEF; both zero is mobile.
// (All hardware-established; see the plan.)
//
// Transitions follow from the model: mobile zeroes both registers; an
// explicit fixed position zeroes SURVEY first, so a completing survey
// cannot overwrite it, then writes FIXEDECEF; a survey zeroes
// FIXEDECEF, so completion re-transfers and the readback shows the
// survey, then writes the parameters. The satpulsed default path
// (SetStatic without a Mode property) preserves an existing fixed
// position and a running survey unless SurveyAgain is set.
//
// The registers store no position accuracy, so FixedPosAcc reads back
// zero and ConfigSupportFixedPosAcc is not declared.

// needsTMode reports whether the target involves the time mode.
func (c *Configurator) needsTMode() bool {
	return c.target.UsesAny(gpsprot.PropIDMode) || c.target.Opts.SetStatic
}

// generateTModeQuery polls the two registers holding the time mode.
func (c *Configurator) generateTModeQuery() {
	if !c.needsTMode() {
		return
	}
	c.addPollReq(asbin.CfgFixedECEFID, func(m asbin.Msg) {
		if fe, ok := m.(*asbin.CfgFixedECEF); ok {
			c.fixedEcef = fe
		}
	})
	c.addPollReq(asbin.CfgSurveyID, func(m asbin.Msg) {
		if sv, ok := m.(*asbin.CfgSurvey); ok {
			c.survey = sv
		}
	})
}

// generateTModeSet computes and sends the time mode registers,
// preserving a fixed position and a running survey on the SetStatic
// default path unless SurveyAgain is set.
func (c *Configurator) generateTModeSet() {
	mode, haveMode := c.target.Props.GetMode()
	setStatic := c.target.Opts.SetStatic
	if !haveMode && !setStatic {
		return
	}
	if c.fixedEcef == nil || c.survey == nil {
		return // no readback: the property does not exist here
	}
	survey := c.target.Opts.Survey
	if !haveMode {
		if c.haveFixedPos() {
			return // setStatic preserves an existing fixed position
		}
		if c.surveyRunning() && survey.Flags&gpsprot.SurveyAgain == 0 {
			return // do not disturb a running survey
		}
		mode = gpsprot.Mode{Static: true}
	} else if !mode.Static && setStatic {
		mode = gpsprot.Mode{Static: true}
	}
	if !mode.Static {
		c.setSurvey(asbin.CfgSurvey{})
		c.setFixedEcef(asbin.CfgFixedECEF{})
		return
	}
	if mode.PosType == gpsprot.PosTypeNone {
		c.setFixedEcef(asbin.CfgFixedECEF{})
		c.setSurvey(asbin.CfgSurvey{
			MinDur:   uint32(survey.MinDur.Round(time.Second) / time.Second),
			AccLimit: uint32(math.Round(survey.AccLimit.Meters() * 1000)),
		})
		return
	}
	c.setSurvey(asbin.CfgSurvey{})
	c.setFixedEcef(fixedEcefFromMode(mode))
}

// haveFixedPos reports whether the readback shows a fixed position.
func (c *Configurator) haveFixedPos() bool {
	fe := c.fixedEcef
	return fe != nil && (fe.X != 0 || fe.Y != 0 || fe.Z != 0)
}

// surveyRunning reports whether the readback shows survey parameters
// with no fixed position yet: a survey in progress.
func (c *Configurator) surveyRunning() bool {
	return c.survey != nil && (c.survey.MinDur != 0 || c.survey.AccLimit != 0) &&
		!c.haveFixedPos()
}

// fixedEcefFromMode converts the mode's fixed position to register
// form. An LLH position is converted to ECEF.
func fixedEcefFromMode(mode gpsprot.Mode) asbin.CfgFixedECEF {
	var x, y, z float64
	if mode.PosType == gpsprot.PosTypeLLH {
		ecef := geopos.WGS84.LLHtoECEF(geopos.LLH{
			Lat:    mode.FixedPosLLH[0].Degrees(),
			Lon:    mode.FixedPosLLH[1].Degrees(),
			Height: mode.Height.Meters(),
		})
		x, y, z = ecef[0], ecef[1], ecef[2]
	} else {
		x = mode.FixedPosECEF[0].Meters()
		y = mode.FixedPosECEF[1].Meters()
		z = mode.FixedPosECEF[2].Meters()
	}
	return asbin.CfgFixedECEF{
		X: int32(math.Round(x * 100)),
		Y: int32(math.Round(y * 100)),
		Z: int32(math.Round(z * 100)),
	}
}

// setFixedEcef sends a CFG-FIXEDECEF set, recording the accepted
// values on ACK.
func (c *Configurator) setFixedEcef(fe asbin.CfgFixedECEF) {
	c.addSetReq(&fe, func() { c.fixedEcef = &fe })
}

// setSurvey sends a CFG-SURVEY set, recording the accepted values on
// ACK.
func (c *Configurator) setSurvey(sv asbin.CfgSurvey) {
	c.addSetReq(&sv, func() { c.survey = &sv })
}

// tmodeConfigProps reports the positioning mode from the register
// readbacks: a nonzero fixed position is static+fixed, survey
// parameters without one are a survey in progress (static), and both
// zero is mobile.
func (c *Configurator) tmodeConfigProps(props *gpsprot.ConfigProps) {
	if c.fixedEcef == nil || c.survey == nil {
		return
	}
	fixed := c.haveFixedPos()
	m := gpsprot.Mode{Static: fixed || c.surveyRunning()}
	if fixed {
		m.PosType = gpsprot.PosTypeECEF
		m.FixedPosECEF = gpsprot.Point3D{
			gpsprot.Meters(float64(c.fixedEcef.X) / 100),
			gpsprot.Meters(float64(c.fixedEcef.Y) / 100),
			gpsprot.Meters(float64(c.fixedEcef.Z) / 100),
		}
	}
	props.SetMode(m)
}
