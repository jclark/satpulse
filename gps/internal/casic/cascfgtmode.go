package casic

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// Time mode (positioning mode) configuration branches by family:
// V5 uses CFG-TMODE with ECEF metres and accuracy variances; V6 uses
// CFG-TMODE2 with scaled ECEF coordinates and position accuracies.
// casbin owns the wire modes, units, and scales.
//
// The satpulsed default path sets SetStatic without a Mode property:
// preserve an existing fixed position, do not restart a running survey
// unless SurveyAgain is set. Forcing a new survey sends auto mode
// first and then survey mode, which restarts regardless of whether
// re-sending survey mode alone would.

// tmodeTarget is the family-independent time mode to realize.
type tmodeTarget struct {
	mode    tmodeTargetMode
	ecef    [3]float64 // m, fixed position
	posAcc  float64    // m, fixed position accuracy
	svinDur uint32     // s, min survey duration
	svinAcc float64    // m, survey accuracy limit
}

// tmodeTargetMode is the family-independent timing-mode intent.
type tmodeTargetMode uint8

const (
	tmodeTargetMobile tmodeTargetMode = iota
	tmodeTargetSurvey
	tmodeTargetFixed
)

// needsTMode reports whether the target involves the time mode.
func (c *Configurator) needsTMode() bool {
	return c.target.UsesAny(gpsprot.PropIDMode) || c.target.Opts.SetStatic
}

// generateTModeQuery polls the family's time mode message.
func (c *Configurator) generateTModeQuery() {
	if !c.needsTMode() {
		return
	}
	if c.family == familyV6 {
		c.addPollReq(casbin.CfgTMode2ID, func(m casbin.Msg) {
			if tm, ok := m.(*casbin.CfgTMode2); ok {
				c.tm6 = tm
			}
		})
	} else {
		c.addPollReq(casbin.CfgTModeID, func(m casbin.Msg) {
			if tm, ok := m.(*casbin.CfgTMode); ok {
				c.tm5 = tm
			}
		})
	}
}

// curTModeMode returns the current time mode from the readback.
func (c *Configurator) curTModeMode() (tmodeTargetMode, bool) {
	if c.tm6 != nil {
		switch c.tm6.TimFixMode {
		case casbin.CfgTMode2Realtime:
			return tmodeTargetMobile, true
		case casbin.CfgTMode2Survey:
			return tmodeTargetSurvey, true
		case casbin.CfgTMode2Fixed:
			return tmodeTargetFixed, true
		default:
			return tmodeTargetMode(c.tm6.TimFixMode), true
		}
	}
	if c.tm5 != nil {
		switch c.tm5.Mode {
		case casbin.CfgTModeAuto:
			return tmodeTargetMobile, true
		case casbin.CfgTModeSurvey:
			return tmodeTargetSurvey, true
		case casbin.CfgTModeFixed:
			return tmodeTargetFixed, true
		default:
			return tmodeTargetMode(c.tm5.Mode), true
		}
	}
	return 0, false
}

// generateTModeSet computes and sends the time mode configuration,
// applying the same mode-transition rules as the UBX configurator
// (preserve a fixed position, restart a survey only when asked).
func (c *Configurator) generateTModeSet() {
	mode, haveMode := c.target.Props.GetMode()
	setStatic := c.target.Opts.SetStatic
	if !haveMode && !setStatic {
		return
	}
	cur, haveCur := c.curTModeMode()
	if !haveCur {
		return // no readback: the property does not exist here
	}
	survey := c.target.Opts.Survey
	if !haveMode {
		if cur == tmodeTargetFixed {
			return // setStatic preserves an existing fixed position
		}
		if cur == tmodeTargetSurvey && survey.Flags&gpsprot.SurveyAgain == 0 {
			return // do not disturb a running survey
		}
		mode = gpsprot.Mode{Static: true}
	} else if !mode.Static && setStatic {
		mode = gpsprot.Mode{Static: true}
	}
	tt := newTModeTarget(mode, survey)
	if tt.mode == tmodeTargetSurvey {
		c.survey = true
	}
	if tt.mode == tmodeTargetSurvey && cur == tmodeTargetSurvey && survey.Flags&gpsprot.SurveyAgain != 0 {
		c.addTModeSet(&tmodeTarget{mode: tmodeTargetMobile}, false)
		// Both requests are generated before the first ACK updates the
		// assumed state. The second write is the command-like half of the
		// explicit restart and must not compare equal to the original
		// survey readback.
		c.addTModeSet(tt, true)
		return
	}
	c.addTModeSet(tt, false)
}

// newTModeTarget converts the device-independent mode and survey
// parameters. An LLH fixed position is converted to ECEF.
func newTModeTarget(mode gpsprot.Mode, survey gpsprot.Survey) *tmodeTarget {
	tt := &tmodeTarget{}
	if !mode.Static {
		tt.mode = tmodeTargetMobile
		return tt
	}
	if mode.PosType == gpsprot.PosTypeNone {
		tt.mode = tmodeTargetSurvey
		tt.svinDur = uint32(survey.MinDur.Round(time.Second) / time.Second)
		tt.svinAcc = survey.AccLimit.Meters()
		return tt
	}
	tt.mode = tmodeTargetFixed
	tt.posAcc = mode.FixedPosAcc.Meters()
	ecef, _ := mode.FixedPosToECEF()
	for i := range 3 {
		tt.ecef[i] = ecef[i].Meters()
	}
	return tt
}

// addTModeSet encodes and sends the time mode for this family. V6
// fields not covered by the model (band, antenna detection, time
// source) are preserved from the readback. force is used only for the
// second half of an explicitly requested survey restart.
func (c *Configurator) addTModeSet(tt *tmodeTarget, force bool) {
	if c.family == familyV6 {
		m := *c.tm6
		switch tt.mode {
		case tmodeTargetMobile:
			m.TimFixMode = casbin.CfgTMode2Realtime
		case tmodeTargetSurvey:
			m.TimFixMode = casbin.CfgTMode2Survey
		case tmodeTargetFixed:
			m.TimFixMode = casbin.CfgTMode2Fixed
		}
		m.XFixed = int32(math.Round(tt.ecef[0] * casbin.CfgTMode2FixedPositionScale))
		m.YFixed = int32(math.Round(tt.ecef[1] * casbin.CfgTMode2FixedPositionScale))
		m.ZFixed = int32(math.Round(tt.ecef[2] * casbin.CfgTMode2FixedPositionScale))
		m.FixedPacc = uint32(math.Round(tt.posAcc * casbin.CfgTMode2PositionAccuracyScale))
		m.SvinMinDur = tt.svinDur
		m.SvinPaccLim = uint32(math.Round(tt.svinAcc * casbin.CfgTMode2PositionAccuracyScale))
		if !force && m == *c.tm6 {
			return
		}
		c.addSetReq(&m, func() { c.tm6 = &m })
		return
	}
	m := &casbin.CfgTMode{
		EcefX:        tt.ecef[0],
		EcefY:        tt.ecef[1],
		EcefZ:        tt.ecef[2],
		PosVar:       casbin.CfgTModeVarianceFromAccuracy(tt.posAcc),
		SvinMinDur:   tt.svinDur,
		SvinVarLimit: casbin.CfgTModeVarianceFromAccuracy(tt.svinAcc),
	}
	switch tt.mode {
	case tmodeTargetMobile:
		m.Mode = casbin.CfgTModeAuto
	case tmodeTargetSurvey:
		m.Mode = casbin.CfgTModeSurvey
	case tmodeTargetFixed:
		m.Mode = casbin.CfgTModeFixed
	}
	cur := *c.tm5
	cur.Res = 0 // upper half of the queried mode field is readback garbage
	if !force && *m == cur {
		return
	}
	c.addSetReq(m, func() { c.tm5 = m })
}

// tmodeConfigProps reports the positioning mode from the readback.
func (c *Configurator) tmodeConfigProps(props *gpsprot.ConfigProps) {
	mode, ok := c.curTModeMode()
	if !ok {
		return
	}
	m := gpsprot.Mode{Static: mode != tmodeTargetMobile}
	if mode == tmodeTargetFixed {
		m.PosType = gpsprot.PosTypeECEF
		if c.tm6 != nil {
			m.FixedPosECEF = gpsprot.Point3D{
				gpsprot.Meters(float64(c.tm6.XFixed) / casbin.CfgTMode2FixedPositionScale),
				gpsprot.Meters(float64(c.tm6.YFixed) / casbin.CfgTMode2FixedPositionScale),
				gpsprot.Meters(float64(c.tm6.ZFixed) / casbin.CfgTMode2FixedPositionScale),
			}
			m.FixedPosAcc = gpsprot.Meters(float64(c.tm6.FixedPacc) / casbin.CfgTMode2PositionAccuracyScale)
		} else {
			m.FixedPosECEF = gpsprot.Point3D{
				gpsprot.Meters(c.tm5.EcefX),
				gpsprot.Meters(c.tm5.EcefY),
				gpsprot.Meters(c.tm5.EcefZ),
			}
			m.FixedPosAcc = gpsprot.Meters(casbin.CfgTModeAccuracyFromVariance(c.tm5.PosVar))
		}
	}
	props.SetMode(m)
}
