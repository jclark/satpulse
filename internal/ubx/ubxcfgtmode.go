package ubx

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

// ErrTmodeLLHNotSupported is returned when trying to convert tmodeConfig with LLH coordinates to TMODE.
// TMODE only supports ECEF coordinates.
var ErrTmodeLLHNotSupported = errors.New("TMODE does not support LLH coordinates, only ECEF")

type tmodeMode uint8

const (
	tmodeDisabled tmodeMode = iota
	tmodeSurveyIn
	tmodeFixed
)

// resurveyMethod specifies how to force a new survey when already in survey mode
type resurveyMethod uint8

const (
	// resurveyDisable forces a new survey by disabling then re-enabling survey mode (legacy behavior)
	resurveyDisable resurveyMethod = iota
	// resurveyChange forces a new survey by changing survey parameters slightly (gen9 behavior)
	resurveyChange
)

// tmodeInfo specifies which configuration information to retrieve/require
type tmodeInfo uint8

const (
	tmodeInfoMode     tmodeInfo = 1 << iota // Just the current time mode
	tmodeInfoSurvey                         // Survey parameters regardless of current mode
	tmodeInfoFixed                          // Fixed position parameters regardless of current mode
	tmodeInfoRelevant                       // Mode-dependent: survey if survey mode, fixed if fixed mode
	tmodeInfoAll      tmodeInfo = tmodeInfoMode | tmodeInfoSurvey | tmodeInfoFixed // Everything
	tmodeInfoNone     tmodeInfo = 0 // No information requested
)

type tmodeConfig struct {
	// Core time mode fields
	mode tmodeMode

	// Fixed position fields (split high-precision format)
	ecefHP [3]int8  // High-precision fractional parts in 0.1mm
	ecef   [3]int32 // Main ECEF values in cm

	useLLH   bool     // Use LLH instead of ECEF
	latLonHP [2]int8  // Lat/Lon high-precision fractional parts in degrees * 1e-9
	heightHP int8     // Height high-precision fractional part in 0.1mm
	latLon   [2]int32 // Lat/Lon main values in degrees * 1e-7
	height   int32    // Height main value in cm (same units as ECEF)

	fixedPosAcc uint32 // Position accuracy in 0.1mm units

	// Survey parameters
	svinMinDur   uint32 // seconds
	svinAccLimit uint32 // Survey accuracy limit in 0.1mm units
}

var tmodeConfigDisabled = tmodeConfig{
	mode: tmodeDisabled,
}

// createTmodeConfigs returns up to two tmodeConfigs necessary to give effect to target.
// Two tmodeConfigs are returned when we want to force a survey: first one will have disabled mode, and second one will have survey mode.
// Otherwise, at most one tmodeConfig is returned, and the second one will be nil.
// If no tmodeConfig is needed, then both returned tmodeConfigs will be nil.
// cur is accessed as follows:
// - if there is no Mode property and SetStatic is false, then cur is never accessed
func createTmodeConfigs(target *gpsprot.ConfigTarget, cur *tmodeConfig, method resurveyMethod) (*tmodeConfig, *tmodeConfig, error) {
	mode, haveMode := target.Props.GetMode()
	setStatic := target.Opts.SetStatic
	if !haveMode && !setStatic {
		return nil, nil, nil // No mode specified and not setStatic, nothing to do
	}
	survey := target.Opts.Survey

	if !haveMode {
		// setStatic must be true at this
		if cur.mode == tmodeFixed {
			// setStatic doesn't change tmodeFixed, so there's nothing to do
			return nil, nil, nil
		}
		
		if cur.mode == tmodeSurveyIn && survey.Flags&gpsprot.SurveyAgain == 0 {
			// if we are already in survey mode and we are not forcing a new survey, then setStatic doesn't force anything
			// XXX should we check if the survey parameters are the same?
			// I think not: satpulsed does this by default, and we shouldn't mess with an existing survey
			return nil, nil, nil
		}
		// have setStatic but no explicit Mode property and not in tmodeFixed mode;
		// same as if we had a Mode property set to Static
		mode = gpsprot.Mode{Static: true}
	} else if !mode.Static && setStatic {
		// strange case: we have a Mode property set as non-static, but SetStatic is set;
		// again, same as if we had a Mode property set to Static
		mode = gpsprot.Mode{Static: true}
	}
	tmc, err := newTmodeConfig(mode, survey)
	if err != nil {
		return nil, nil, err
	}
	if tmc.mode == tmodeSurveyIn && survey.Flags&gpsprot.SurveyAgain != 0 {
		switch method {
		case resurveyDisable:
			if cur.mode == tmodeSurveyIn {
				return &tmodeConfigDisabled, tmc, nil
			}
		case resurveyChange:
			if tmc.svinAccLimit == cur.svinAccLimit {
				tmc.svinAccLimit++ // increment by 1 (0.1mm)	
			}
		}
	} 
	return tmc, nil, nil
}

// newTmodeConfig creates tmodeConfig from gpsprot.Mode and gpsprot.Survey values.
func newTmodeConfig(mode gpsprot.Mode, survey gpsprot.Survey) (*tmodeConfig, error) {
	tc := &tmodeConfig{}
	if !mode.Static {
		tc.mode = tmodeDisabled
		return tc, nil
	}
	if mode.PosType == gpsprot.PosTypeNone {
		tc.mode = tmodeSurveyIn
		tc.svinMinDur = uint32(survey.MinDur.Round(time.Second) / time.Second)
		tc.svinAccLimit = tmodeAcc(survey.AccLimit)
		return tc, nil
	}
	tc.mode = tmodeFixed
	tc.fixedPosAcc = tmodeAcc(mode.FixedPosAcc)
	tc.useLLH = mode.PosType == gpsprot.PosTypeLLH
	if tc.useLLH {
		for i := range 2 {
			deg, frac, err := splitAngle(mode.FixedPosLLH[i])
			if err != nil {
				return nil, err
			}
			tc.latLon[i] = deg
			tc.latLonHP[i] = frac
		}
		cm, frac, err := splitLength(mode.Height)
		if err != nil {
			return nil, err
		}
		tc.height = cm
		tc.heightHP = frac
	} else {
		for i := range 3 {
			cm, frac, err := splitLength(mode.FixedPosECEF[i])
			if err != nil {
				return nil, err
			}
			tc.ecef[i] = cm
			tc.ecefHP[i] = frac
		}
	}
	return tc, nil
}

// getMode converts tmodeConfig to gpsprot.Mode.
func (tc *tmodeConfig) getMode() gpsprot.Mode {
	mode := gpsprot.Mode{}
	
	switch tc.mode {
	case tmodeDisabled:
		mode.Static = false
		return mode
		
	case tmodeSurveyIn:
		mode.Static = true
		mode.PosType = gpsprot.PosTypeNone
		return mode
		
	case tmodeFixed:
		mode.Static = true
		mode.FixedPosAcc = gpsprot.Length(tc.fixedPosAcc) * (gpsprot.Millimeter / 10)
		
		if tc.useLLH {
			mode.PosType = gpsprot.PosTypeLLH
			for i := range 2 {
				mode.FixedPosLLH[i] = angleHP(tc.latLon[i], tc.latLonHP[i])
			}
			mode.Height = lengthHP(tc.height, tc.heightHP)
		} else {
			mode.PosType = gpsprot.PosTypeECEF
			for i := range 3 {
				mode.FixedPosECEF[i] = lengthHP(tc.ecef[i], tc.ecefHP[i])
			}
		}
		return mode
	}
	
	return mode
}

// toTmodeMsg converts tmodeConfig to a tmode msg using another tmode msg as the basis.
// The basis says what kind of tmode message to produce.
// The basis is used for fields that are not relevant to what is specified in tmodeConfig.
func (tc *tmodeConfig) toTmodeMsg(msg bin.Msg) (bin.Msg, error) {
	switch tm := msg.(type) {
	case *bin.CfgTmode3:
		cpy := *tm
		tc.toTmode3(&cpy, false)
		return &cpy, nil
	case *bin.CfgTmode2:
		cpy := *tm
		tc.toTmode2(&cpy, false)
		return &cpy, nil
	case *bin.CfgTmode:
		cpy := *tm
		err := tc.toTmode(&cpy, false)
		if err != nil {
			return nil, err
		}
		return &cpy, nil
	}
	panic("unexpected message type for toTmodeMsg")
}

func (tc *tmodeConfig) fromTmodeMsg(msg bin.Msg)  {
	if msg == nil {
		return
	}
	switch tm := msg.(type) {
	case *bin.CfgTmode3:
		tc.fromTmode3(tm)
	case *bin.CfgTmode2:
		tc.fromTmode2(tm)
	case *bin.CfgTmode:
		tc.fromTmode(tm)
	default:
		// tmode not supported, do nothing
	}
}

// toTmode3 converts the tmodeConfig to CfgTmode3.
// If all is true, it sets all fields regardless of mode.
// If all is false, it only sets fields relevant to the current mode.
func (tc *tmodeConfig) toTmode3(tm *bin.CfgTmode3, all bool) {
	tm.Version = 0
	tm.Flags = 0

	// Set mode in flags
	switch tc.mode {
	case tmodeDisabled:
		tm.Flags |= bin.CfgTmode3Disabled
	case tmodeSurveyIn:
		tm.Flags |= bin.CfgTmode3SurveyIn
	case tmodeFixed:
		tm.Flags |= bin.CfgTmode3FixedMode
	}

	// Set coordinate system flag and values for fixed mode
	if tc.mode == tmodeFixed || all {
		if tc.useLLH {
			// LLH coordinates
			tm.Flags |= bin.CfgTmode3LLA
			tm.EcefXOrLat = tc.latLon[0]     // degrees * 1e-7
			tm.EcefYOrLon = tc.latLon[1]     // degrees * 1e-7
			tm.EcefZOrAlt = tc.height        // cm
			tm.EcefXOrLatHP = tc.latLonHP[0] // degrees * 1e-9
			tm.EcefYOrLonHP = tc.latLonHP[1] // degrees * 1e-9
			tm.EcefZOrAltHP = tc.heightHP    // 0.1mm
		} else {
			// ECEF coordinates (LLA flag not set)
			tm.EcefXOrLat = tc.ecef[0]     // cm
			tm.EcefYOrLon = tc.ecef[1]     // cm
			tm.EcefZOrAlt = tc.ecef[2]     // cm
			tm.EcefXOrLatHP = tc.ecefHP[0] // 0.1mm
			tm.EcefYOrLonHP = tc.ecefHP[1] // 0.1mm
			tm.EcefZOrAltHP = tc.ecefHP[2] // 0.1mm
		}
		// Fixed position accuracy
		tm.FixedPosAcc = tc.fixedPosAcc // 0.1mm
	}

	// Survey-in parameters for survey mode
	if tc.mode == tmodeSurveyIn || all {
		tm.SvinMinDur = tc.svinMinDur     // seconds
		tm.SvinAccLimit = tc.svinAccLimit // 0.1mm
	}
}

// fromTmode3 converts CfgTmode3 to tmodeConfig.
func (tc *tmodeConfig) fromTmode3(tm *bin.CfgTmode3) {
	// Extract mode from flags
	switch tm.Flags & bin.CfgTmode3Mode {
	case bin.CfgTmode3Disabled:
		tc.mode = tmodeDisabled
	case bin.CfgTmode3SurveyIn:
		tc.mode = tmodeSurveyIn
	case bin.CfgTmode3FixedMode:
		tc.mode = tmodeFixed
	}

	// Check coordinate system and extract values
	if tm.Flags&bin.CfgTmode3LLA != 0 {
		// LLH coordinates
		tc.useLLH = true
		tc.latLon[0] = tm.EcefXOrLat     // degrees * 1e-7
		tc.latLon[1] = tm.EcefYOrLon     // degrees * 1e-7
		tc.height = tm.EcefZOrAlt        // cm
		tc.latLonHP[0] = tm.EcefXOrLatHP // degrees * 1e-9
		tc.latLonHP[1] = tm.EcefYOrLonHP // degrees * 1e-9
		tc.heightHP = tm.EcefZOrAltHP    // 0.1mm
	} else {
		// ECEF coordinates
		tc.useLLH = false
		tc.ecef[0] = tm.EcefXOrLat     // cm
		tc.ecef[1] = tm.EcefYOrLon     // cm
		tc.ecef[2] = tm.EcefZOrAlt     // cm
		tc.ecefHP[0] = tm.EcefXOrLatHP // 0.1mm
		tc.ecefHP[1] = tm.EcefYOrLonHP // 0.1mm
		tc.ecefHP[2] = tm.EcefZOrAltHP // 0.1mm
	}

	// Fixed position accuracy
	tc.fixedPosAcc = tm.FixedPosAcc // 0.1mm

	// Survey-in parameters
	tc.svinMinDur = tm.SvinMinDur     // seconds
	tc.svinAccLimit = tm.SvinAccLimit // 0.1mm
}

// toTmode2 converts the tmodeConfig to CfgTmode2.
// If all is true, it sets all fields regardless of mode.
// If all is false, it only sets fields relevant to the current mode.
func (tc *tmodeConfig) toTmode2(tm *bin.CfgTmode2, all bool) {
	tm.Flags = 0

	// Set mode in separate field
	switch tc.mode {
	case tmodeDisabled:
		tm.TimeMode = bin.CfgTmode2Disabled
	case tmodeSurveyIn:
		tm.TimeMode = bin.CfgTmode2SurveyIn
	case tmodeFixed:
		tm.TimeMode = bin.CfgTmode2FixedMode
	}

	// Set coordinate system flag and values for fixed mode
	if tc.mode == tmodeFixed || all {
		if tc.useLLH {
			tm.Flags |= bin.CfgTmode2LLA
			// LLH coordinates (no HP fields in TMODE2)
			tm.EcefXOrLat = tc.latLon[0] // degrees * 1e-7
			tm.EcefYOrLon = tc.latLon[1] // degrees * 1e-7
			tm.EcefZOrAlt = tc.height    // cm
		} else {
			// ECEF coordinates (LLA flag not set)
			tm.EcefXOrLat = tc.ecef[0] // cm
			tm.EcefYOrLon = tc.ecef[1] // cm
			tm.EcefZOrAlt = tc.ecef[2] // cm
		}
		// Fixed position accuracy (convert from 0.1mm to mm with rounding)
		fixedPosAccMm, _ := divModRound(int64(tc.fixedPosAcc), 10)
		tm.FixedPosAcc = uint32(fixedPosAccMm)
	}

	// Survey-in parameters for survey mode
	if tc.mode == tmodeSurveyIn || all {
		tm.SvinMinDur = tc.svinMinDur // seconds
		svinAccLimitMm, _ := divModRound(int64(tc.svinAccLimit), 10)
		tm.SvinAccLimit = uint32(svinAccLimitMm) // mm (convert from 0.1mm)
	}
}

// fromTmode2 converts CfgTmode2 to tmodeConfig.
func (tc *tmodeConfig) fromTmode2(tm *bin.CfgTmode2) {
	// Extract mode from separate field
	switch tm.TimeMode {
	case bin.CfgTmode2Disabled:
		tc.mode = tmodeDisabled
	case bin.CfgTmode2SurveyIn:
		tc.mode = tmodeSurveyIn
	case bin.CfgTmode2FixedMode:
		tc.mode = tmodeFixed
	}

	// Check coordinate system and extract values
	if tm.Flags&bin.CfgTmode2LLA != 0 {
		// LLH coordinates
		tc.useLLH = true
		tc.latLon[0] = tm.EcefXOrLat // degrees * 1e-7
		tc.latLon[1] = tm.EcefYOrLon // degrees * 1e-7
		tc.height = tm.EcefZOrAlt    // cm
		// HP fields are zero for TMODE2
		tc.latLonHP[0] = 0
		tc.latLonHP[1] = 0
		tc.heightHP = 0
	} else {
		// ECEF coordinates
		tc.useLLH = false
		tc.ecef[0] = tm.EcefXOrLat // cm
		tc.ecef[1] = tm.EcefYOrLon // cm
		tc.ecef[2] = tm.EcefZOrAlt // cm
		// HP fields are zero for TMODE2
		tc.ecefHP[0] = 0
		tc.ecefHP[1] = 0
		tc.ecefHP[2] = 0
	}

	// Fixed position accuracy (convert from mm to 0.1mm)
	tc.fixedPosAcc = tm.FixedPosAcc * 10 // 0.1mm

	// Survey-in parameters
	tc.svinMinDur = tm.SvinMinDur          // seconds
	tc.svinAccLimit = tm.SvinAccLimit * 10 // 0.1mm (convert from mm)
}

// toTmode converts the tmodeConfig to CfgTmode.
// If all is true, it sets all fields regardless of mode.
// If all is false, it only sets fields relevant to the current mode.
// Returns error if accuracy values would cause overflow in variance calculations.
func (tc *tmodeConfig) toTmode(tm *bin.CfgTmode, all bool) error {
	// TMODE only supports ECEF coordinates, not LLH
	if tc.useLLH && (tc.mode == tmodeFixed || all) {
		return ErrTmodeLLHNotSupported
	}

	// Set mode in separate field
	switch tc.mode {
	case tmodeDisabled:
		tm.TimeMode = bin.CfgTmodeDisabled
	case tmodeSurveyIn:
		tm.TimeMode = bin.CfgTmodeSurveyIn
	case tmodeFixed:
		tm.TimeMode = bin.CfgTmodeFixedMode
	}

	// ECEF coordinates and fixed position accuracy for fixed mode
	if tc.mode == tmodeFixed || all {
		// ECEF coordinates only (no LLH support in TMODE)
		tm.FixedPosX = tc.ecef[0] // cm
		tm.FixedPosY = tc.ecef[1] // cm
		tm.FixedPosZ = tc.ecef[2] // cm

		// Fixed position variance (convert from 0.1mm accuracy to mm² variance)
		var err error
		tm.FixedPosVar, err = tmodeAccToVar(tc.fixedPosAcc, "fixed position accuracy")
		if err != nil {
			return err
		}
	}

	// Survey-in parameters for survey mode
	if tc.mode == tmodeSurveyIn || all {
		tm.SvinMinDur = tc.svinMinDur // seconds
		var err error
		tm.SvinVarLimit, err = tmodeAccToVar(tc.svinAccLimit, "survey accuracy limit")
		if err != nil {
			return err
		}
	}

	return nil
}

// fromTmode converts CfgTmode to tmodeConfig.
func (tc *tmodeConfig) fromTmode(tm *bin.CfgTmode) {
	// Extract mode from separate field
	switch tm.TimeMode {
	case bin.CfgTmodeDisabled:
		tc.mode = tmodeDisabled
	case bin.CfgTmodeSurveyIn:
		tc.mode = tmodeSurveyIn
	case bin.CfgTmodeFixedMode:
		tc.mode = tmodeFixed
	}

	// ECEF coordinates only (no LLH support in TMODE)
	tc.useLLH = false
	tc.ecef[0] = tm.FixedPosX // cm
	tc.ecef[1] = tm.FixedPosY // cm
	tc.ecef[2] = tm.FixedPosZ // cm
	// HP fields are zero for TMODE
	tc.ecefHP[0] = 0
	tc.ecefHP[1] = 0
	tc.ecefHP[2] = 0
	// LLH fields are unused
	tc.latLon[0] = 0
	tc.latLon[1] = 0
	tc.height = 0
	tc.latLonHP[0] = 0
	tc.latLonHP[1] = 0
	tc.heightHP = 0

	// Fixed position accuracy (convert from mm² variance to 0.1mm accuracy)
	tc.fixedPosAcc = tmodeVarToAcc(tm.FixedPosVar)

	// Survey-in parameters
	tc.svinMinDur = tm.SvinMinDur // seconds
	tc.svinAccLimit = tmodeVarToAcc(tm.SvinVarLimit)
}

// tmodeAccToVar converts accuracy in 0.1mm to variance in mm² with overflow check.
func tmodeAccToVar(acc01mm uint32, name string) (uint32, error) {
	accMm, _ := divModRound(int64(acc01mm), 10)
	if accMm > 65535 {
		return 0, fmt.Errorf("%s %d mm too large for TMODE (max 65535 mm)", name, accMm)
	}
	return uint32(accMm * accMm), nil
}

// tmodeVarToAcc converts variance in mm² to accuracy in 0.1mm.
func tmodeVarToAcc(varMm2 uint32) uint32 {
	accMm := uint32(math.Sqrt(float64(varMm2)))
	return accMm * 10 // convert to 0.1mm
}

// toItems converts tmodeConfig to configuration items.
// If all is true, it produces items for all fields regardless of mode.
// If all is false, it only produces items relevant to the current mode.
func (tc *tmodeConfig) toItems(items *[]ucv.Item, all bool) {
	// Always include mode
	mode := ucv.ETmodeModeDisabled
	switch tc.mode {
	case tmodeSurveyIn:
		mode = ucv.ETmodeModeSurveyIn
	case tmodeFixed:
		mode = ucv.ETmodeModeFixed
	}
	ucv.AddItem(items, ucv.KTmodeMode, mode)

	// For survey mode, include survey parameters
	if tc.mode == tmodeSurveyIn || all {
		ucv.AddItem(items, ucv.KTmodeSvinMinDur, uint64(tc.svinMinDur))
		ucv.AddItem(items, ucv.KTmodeSvinAccLimit, uint64(tc.svinAccLimit))
	}

	// For fixed mode, include position and accuracy
	if tc.mode == tmodeFixed || all {
		// Set position type
		if tc.useLLH {
			ucv.AddItem(items, ucv.KTmodePosType, ucv.ETmodePosTypeLlh)
			// LLH coordinates
			ucv.AddItem(items, ucv.KTmodeLat, int64(tc.latLon[0]))
			ucv.AddItem(items, ucv.KTmodeLon, int64(tc.latLon[1]))
			ucv.AddItem(items, ucv.KTmodeHeight, int64(tc.height))
			ucv.AddItem(items, ucv.KTmodeLatHp, int64(tc.latLonHP[0]))
			ucv.AddItem(items, ucv.KTmodeLonHp, int64(tc.latLonHP[1]))
			ucv.AddItem(items, ucv.KTmodeHeightHp, int64(tc.heightHP))
		} else {
			ucv.AddItem(items, ucv.KTmodePosType, ucv.ETmodePosTypeEcef)
			// ECEF coordinates
			ucv.AddItem(items, ucv.KTmodeEcefX, int64(tc.ecef[0]))
			ucv.AddItem(items, ucv.KTmodeEcefY, int64(tc.ecef[1]))
			ucv.AddItem(items, ucv.KTmodeEcefZ, int64(tc.ecef[2]))
			ucv.AddItem(items, ucv.KTmodeEcefXHp, int64(tc.ecefHP[0]))
			ucv.AddItem(items, ucv.KTmodeEcefYHp, int64(tc.ecefHP[1]))
			ucv.AddItem(items, ucv.KTmodeEcefZHp, int64(tc.ecefHP[2]))
		}
		// Fixed position accuracy
		ucv.AddItem(items, ucv.KTmodeFixedPosAcc, uint64(tc.fixedPosAcc))
	}
}

// fromCfgVals converts configuration items to tmodeConfig.
// info specifies which configuration information to retrieve/require.
// Returns ok=false if required keys are missing.
func (tc *tmodeConfig) fromCfgVals(vals *CfgVals, info tmodeInfo) bool {
	// Get mode if required
	if info&(tmodeInfoMode|tmodeInfoRelevant) != 0 {
		mode, ok := cfgValGet(vals, ucv.KTmodeMode)
		if !ok {
			return false
		}
		tc.mode = tmodeDisabled // Default to disabled
		extraInfo := tmodeInfoNone
		switch mode {
		case ucv.ETmodeModeSurveyIn:
			tc.mode = tmodeSurveyIn
			extraInfo = tmodeInfoSurvey
		case ucv.ETmodeModeFixed:
			tc.mode = tmodeFixed
			extraInfo = tmodeInfoFixed 
		}
		if info&tmodeInfoRelevant != 0 {
			info |= extraInfo // Add relevant info based on mode
		}
	}

	// Get survey parameters if required
	if info&tmodeInfoSurvey != 0 {
		if !cfgValGetUint32(vals, ucv.KTmodeSvinMinDur, &tc.svinMinDur) ||
			!cfgValGetUint32(vals, ucv.KTmodeSvinAccLimit, &tc.svinAccLimit) {
			return false
		}
	}

	// Get fixed position parameters if required
	if info&tmodeInfoFixed != 0 {
		// Position type determines coordinate system
		posType, ok := cfgValGet(vals, ucv.KTmodePosType)
		if !ok {
			return false
		}
		tc.useLLH = (posType == ucv.ETmodePosTypeLlh)

		if tc.useLLH {
			// LLH coordinates
			if !cfgValGetInt32(vals, ucv.KTmodeLat, &tc.latLon[0]) ||
				!cfgValGetInt32(vals, ucv.KTmodeLon, &tc.latLon[1]) ||
				!cfgValGetInt32(vals, ucv.KTmodeHeight, &tc.height) ||
				!cfgValGetInt8(vals, ucv.KTmodeLatHp, &tc.latLonHP[0]) ||
				!cfgValGetInt8(vals, ucv.KTmodeLonHp, &tc.latLonHP[1]) ||
				!cfgValGetInt8(vals, ucv.KTmodeHeightHp, &tc.heightHP) {
				return false
			}
		} else {
			// ECEF coordinates
			if !cfgValGetInt32(vals, ucv.KTmodeEcefX, &tc.ecef[0]) ||
				!cfgValGetInt32(vals, ucv.KTmodeEcefY, &tc.ecef[1]) ||
				!cfgValGetInt32(vals, ucv.KTmodeEcefZ, &tc.ecef[2]) ||
				!cfgValGetInt8(vals, ucv.KTmodeEcefXHp, &tc.ecefHP[0]) ||
				!cfgValGetInt8(vals, ucv.KTmodeEcefYHp, &tc.ecefHP[1]) ||
				!cfgValGetInt8(vals, ucv.KTmodeEcefZHp, &tc.ecefHP[2]) {
				return false
			}
		}
		if !cfgValGetUint32(vals, ucv.KTmodeFixedPosAcc, &tc.fixedPosAcc) {
			return false
		}
	}
	return true
}

// tmodeRequiredKeys returns the CFG-VAL keys that need to be fetched for the given info level
func tmodeRequiredKeys(info tmodeInfo) []ucv.AnyTypedKey {
	var keys []ucv.AnyTypedKey
	
	if info&tmodeInfoMode != 0 {
		keys = append(keys, ucv.KTmodeMode)
	}
	
	if info&tmodeInfoSurvey != 0 {
		keys = append(keys, 
			ucv.KTmodeSvinMinDur,
			ucv.KTmodeSvinAccLimit,
		)
	}
	
	if info&tmodeInfoFixed != 0 {
		keys = append(keys,
			ucv.KTmodePosType,
			ucv.KTmodeEcefX, ucv.KTmodeEcefY, ucv.KTmodeEcefZ,
			ucv.KTmodeEcefXHp, ucv.KTmodeEcefYHp, ucv.KTmodeEcefZHp,
			ucv.KTmodeLat, ucv.KTmodeLon, ucv.KTmodeHeight,
			ucv.KTmodeLatHp, ucv.KTmodeLonHp, ucv.KTmodeHeightHp,
			ucv.KTmodeFixedPosAcc,
		)
	}
	
	return keys
}

// Helper functions for getting values and storing in fields
func cfgValGetUint32(vals *CfgVals, key ucv.KeyU, ptr *uint32) bool {
	v, ok := cfgValGet(vals, key)
	if ok {
		*ptr = uint32(v)
	}
	return ok
}

func cfgValGetInt32(vals *CfgVals, key ucv.KeyI, ptr *int32) bool {
	v, ok := cfgValGet(vals, key)
	if ok {
		*ptr = int32(v)
	}
	return ok
}

func cfgValGetInt8(vals *CfgVals, key ucv.KeyI, ptr *int8) bool {
	v, ok := cfgValGet(vals, key)
	if ok {
		*ptr = int8(v)
	}
	return ok
}

// tmodeAcc converts a Length to the units used by tmodeConfig for accuracy i.e. 0.1mm.
func tmodeAcc(meters gpsprot.Length) uint32 {
	acc, _ := divModRound(int64(meters), int64(gpsprot.Millimeter/10))
	return uint32(acc)
}

