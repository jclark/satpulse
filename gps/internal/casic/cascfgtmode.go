package casic

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/geopos"
)

// Time mode (positioning mode) configuration branches by family:
// V5 uses CFG-TMODE (R8 ECEF meters, R4 variances in m^2, and the mode
// field parsed as U2 + garbage per the erratum); V6 uses CFG-TMODE2
// (I4 ECEF in 0.01 m, U4 accuracies in mm). Mode values are shared:
// 0=auto (mobile), 1=survey-in, 2=fixed position.
//
// The satpulsed default path sets SetStatic without a Mode property:
// preserve an existing fixed position, do not restart a running survey
// unless SurveyAgain is set. Forcing a new survey sends auto mode
// first and then survey mode, which restarts regardless of whether
// re-sending survey mode alone would.

// tmodeTarget is the family-independent time mode to realize.
type tmodeTarget struct {
	mode    uint8      // casbin.TModeAuto/TModeSurvey/TModeFixed
	ecef    [3]float64 // m, fixed position
	posAcc  float64    // m, fixed position accuracy
	svinDur uint32     // s, min survey duration
	svinAcc float64    // m, survey accuracy limit
}

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
func (c *Configurator) curTModeMode() (uint8, bool) {
	if c.tm6 != nil {
		return uint8(c.tm6.TimFixMode), true
	}
	if c.tm5 != nil {
		return uint8(c.tm5.Mode), true
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
		if cur == casbin.TModeFixed {
			return // setStatic preserves an existing fixed position
		}
		if cur == casbin.TModeSurvey && survey.Flags&gpsprot.SurveyAgain == 0 {
			return // do not disturb a running survey
		}
		mode = gpsprot.Mode{Static: true}
	} else if !mode.Static && setStatic {
		mode = gpsprot.Mode{Static: true}
	}
	tt := newTModeTarget(mode, survey)
	if tt.mode == casbin.TModeSurvey {
		c.survey = true
	}
	if tt.mode == casbin.TModeSurvey && cur == casbin.TModeSurvey && survey.Flags&gpsprot.SurveyAgain != 0 {
		c.addTModeSet(&tmodeTarget{mode: casbin.TModeAuto})
	}
	c.addTModeSet(tt)
}

// newTModeTarget converts the device-independent mode and survey
// parameters. An LLH fixed position is converted to ECEF.
func newTModeTarget(mode gpsprot.Mode, survey gpsprot.Survey) *tmodeTarget {
	tt := &tmodeTarget{}
	if !mode.Static {
		tt.mode = casbin.TModeAuto
		return tt
	}
	if mode.PosType == gpsprot.PosTypeNone {
		tt.mode = casbin.TModeSurvey
		tt.svinDur = uint32(survey.MinDur.Round(time.Second) / time.Second)
		tt.svinAcc = survey.AccLimit.Meters()
		return tt
	}
	tt.mode = casbin.TModeFixed
	tt.posAcc = mode.FixedPosAcc.Meters()
	if mode.PosType == gpsprot.PosTypeLLH {
		ecef := geopos.WGS84.LLHtoECEF(geopos.LLH{
			Lat:    mode.FixedPosLLH[0].Degrees(),
			Lon:    mode.FixedPosLLH[1].Degrees(),
			Height: mode.Height.Meters(),
		})
		tt.ecef = [3]float64{ecef[0], ecef[1], ecef[2]}
	} else {
		for i := range 3 {
			tt.ecef[i] = mode.FixedPosECEF[i].Meters()
		}
	}
	return tt
}

// addTModeSet encodes and sends the time mode for this family. V6
// fields not covered by the model (band, antenna detection, time
// source) are preserved from the readback.
func (c *Configurator) addTModeSet(tt *tmodeTarget) {
	if c.family == familyV6 {
		m := *c.tm6
		m.TimFixMode = casbin.CfgTMode2Mode(tt.mode)
		m.XFixed = int32(math.Round(tt.ecef[0] * 100))
		m.YFixed = int32(math.Round(tt.ecef[1] * 100))
		m.ZFixed = int32(math.Round(tt.ecef[2] * 100))
		m.FixedPacc = uint32(math.Round(tt.posAcc * 1000))
		m.SvinMinDur = tt.svinDur
		m.SvinPaccLim = uint32(math.Round(tt.svinAcc * 1000))
		c.addSetReq(&m, func() { c.tm6 = &m })
		return
	}
	m := &casbin.CfgTMode{
		Mode:         uint16(tt.mode),
		EcefX:        tt.ecef[0],
		EcefY:        tt.ecef[1],
		EcefZ:        tt.ecef[2],
		PosVar:       float32(tt.posAcc * tt.posAcc),
		SvinMinDur:   tt.svinDur,
		SvinVarLimit: float32(tt.svinAcc * tt.svinAcc),
	}
	c.addSetReq(m, func() { c.tm5 = m })
}

// tmodeConfigProps reports the positioning mode from the readback.
func (c *Configurator) tmodeConfigProps(props *gpsprot.ConfigProps) {
	mode, ok := c.curTModeMode()
	if !ok {
		return
	}
	m := gpsprot.Mode{Static: mode != casbin.TModeAuto}
	if mode == casbin.TModeFixed {
		m.PosType = gpsprot.PosTypeECEF
		if c.tm6 != nil {
			m.FixedPosECEF = gpsprot.Point3D{
				gpsprot.Meters(float64(c.tm6.XFixed) / 100),
				gpsprot.Meters(float64(c.tm6.YFixed) / 100),
				gpsprot.Meters(float64(c.tm6.ZFixed) / 100),
			}
			m.FixedPosAcc = gpsprot.Meters(float64(c.tm6.FixedPacc) / 1000)
		} else {
			m.FixedPosECEF = gpsprot.Point3D{
				gpsprot.Meters(c.tm5.EcefX),
				gpsprot.Meters(c.tm5.EcefY),
				gpsprot.Meters(c.tm5.EcefZ),
			}
			m.FixedPosAcc = gpsprot.Meters(math.Sqrt(float64(c.tm5.PosVar)))
		}
	}
	props.SetMode(m)
}
