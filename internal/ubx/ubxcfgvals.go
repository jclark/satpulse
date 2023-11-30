package ubx

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

type CfgVals struct {
	ucv.Map
}

var AllKeys = []ucv.AnyTypedKey{
	ucv.KNavspgDynmodel,
	ucv.KNavspgUtcstandard,
	ucv.KRateMeas,
	ucv.KRateNav,
	ucv.KRateTimeref,
	ucv.KSignalBdsEna,
	ucv.KSignalGalEna,
	ucv.KSignalGloEna,
	ucv.KSignalGpsEna,
	ucv.KSignalNavicEna,
	ucv.KSignalQzssEna,
	ucv.KSignalSbasEna,
	ucv.KTmodeEcefX,
	ucv.KTmodeEcefXHp,
	ucv.KTmodeEcefY,
	ucv.KTmodeEcefYHp,
	ucv.KTmodeEcefZ,
	ucv.KTmodeEcefZHp,
	ucv.KTmodeMode,
	ucv.KTmodePosType,
	ucv.KTmodeSvinMinDur,
	ucv.KTmodeSvinAccLimit,
	ucv.KTpAlignToTowTp1,
	ucv.KTpAntCabledelay,
	ucv.KTpDutyTp1,
	ucv.KTpLenLockTp1,
	ucv.KTpLenTp1,
	ucv.KTpPeriodLockTp1,
	ucv.KTpPeriodTp1,
	ucv.KTpPolTp1,
	ucv.KTpPulseDef,
	ucv.KTpPulseLengthDef,
	ucv.KTpSyncGnssTp1,
	ucv.KTpTimegridTp1,
	ucv.KTpTp1Ena,
	ucv.KTpUseLockedTp1,
}

var AllMsgKeys = []ucv.KeyM{
	ucv.KUbxNavTimegps,
	ucv.KUbxNavTimegal,
	ucv.KUbxNavTimeglo,
	ucv.KUbxNavTimebds,
	ucv.KUbxNavTimeutc,
	ucv.KUbxNavTimels,
	ucv.KUbxNavSvin,
	ucv.KUbxTimSvin,
	ucv.KUbxTimTp,
}

func MakeCfgVals() CfgVals {
	return CfgVals{make(ucv.Map)}
}

func (vals *CfgVals) isNil() bool {
	return vals.Map == nil
}

func (raw *CfgVals) AddData(cfgData []byte) error {
	items, err := ucv.UnmarshalItems(cfgData)
	if err != nil {
		return err
	}
	raw.AddItems(items)
	return nil
}

func (raw *CfgVals) Cook(ver *Version, port ucv.Port, cm *gpsprot.ConfigMap) {
	if v, ok := cfgValGet(raw, ucv.KTmodeMode); ok {
		gpsprot.CfgTimeMode.Set(cm, tmodeModeToTimeMode(v))
	}
	if v, ok := raw.cookTmodeECEF(); ok {
		gpsprot.CfgFixedPosECEF.Set(cm, v)
	}
	if v, ok := raw.getTimePulseAlignToGNSS(); ok {
		gpsprot.CfgTimePulseAlignToGNSS.Set(cm, v)
	}
	if v, ok := raw.getTimePulsePeriod(); ok {
		gpsprot.CfgTimePulsePeriod.Set(cm, v)
	}
	if v, ok := raw.getTimePulseWidth(); ok {
		gpsprot.CfgTimePulseWidth.Set(cm, v)
	}
	if v, ok := raw.getTimePulseOnlyWhenLocked(); ok {
		gpsprot.CfgTimePulseOnlyWhenLocked.Set(cm, v)
	}
	if v, ok := cfgValGet(raw, ucv.KTpAntCabledelay); ok {
		gpsprot.CfgAntennaCableDelay.Set(cm, time.Duration(v))
	}
	if v, ok := cfgValGet(raw, ucv.KTpPolTp1); ok {
		gpsprot.CfgTimePulsePolarityRising.Set(cm, v)
	}
	if v, ok := cfgValGet(raw, ucv.KNavspgDynmodel); ok {
		gpsprot.CfgStationary.Set(cm, v == ucv.ENavspgDynmodelStat)
	}
	if v, ok := cfgValGet(raw, portOutprotNmeaKey(port)); ok {
		gpsprot.CfgNMEAEnabled.Set(cm, v)
	}
	if v, ok := cfgValGet(raw, portBaudRateKey(port)); ok {
		gpsprot.CfgBaudRate.Set(cm, uint32(v))
	}
}

// Change generates changes that will take a known configuration to a desired configuration.
// The known argument gives what is known about the current configuration.
// cm is the desired configuration.
// If additional keys are needed to compile the configuration correctly,
// they are returned in the ucv.Key slice.
// Typically this function will get called twice.
// The first time, known will be empty, and some more keys will be needed.
// The caller will then fetch the additional keys, add them to known and call again.
func (known *CfgVals) Change(cm *gpsprot.ConfigMap, opts gpsprot.ConfigOptions, ver *Version, port ucv.Port) ([]ucv.Item, []ucv.Key, error) {
	items := []ucv.Item{}
	keys := []ucv.Key{}
	tg := known.changeTimePulse(cm, &items, &keys)

	err := known.changeTimeMode(cm, opts, ver, port, &items, &keys)
	if err != nil {
		return nil, nil, err
	}

	if v, ok := gpsprot.CfgAntennaCableDelay.Get(cm); ok {
		ucv.AddItem(&items, ucv.KTpAntCabledelay, int64(v))
	}
	if v, ok := gpsprot.CfgTimePulsePolarityRising.Get(cm); ok {
		ucv.AddItem(&items, ucv.KTpPolTp1, v)
	}
	if v, ok := gpsprot.CfgPrimaryGNSS.Get(cm); ok {
		tg = gnssToTimegridTp1(v)
		ucv.AddItem(&items, ucv.KTpTimegridTp1, tg)
		ucv.AddItem(&items, ucv.KNavspgUtcstandard, gnssToEnumNavspgUtcstandard(v))
		ucv.AddItem(&items, ucv.KRateTimeref, gnssToRateTimeref(v))
	}
	if v, ok := gpsprot.CfgSolutionPeriod.Get(cm); ok {
		ucv.AddItem(&items, ucv.KRateMeas, uint64(uint16(v.Round(time.Millisecond)/time.Millisecond)))
		ucv.AddItem(&items, ucv.KRateNav, 1)
	}
	if v, ok := gpsprot.CfgStationary.Get(cm); ok {
		dm := ucv.ENavspgDynmodelPort
		if v {
			dm = ucv.ENavspgDynmodelStat
		}
		ucv.AddItem(&items, ucv.KNavspgDynmodel, dm)
	}
	if v, ok := gpsprot.CfgNMEAEnabled.Get(cm); ok {
		k := portOutprotNmeaKey(port)
		if k != 0 {
			ucv.AddItem(&items, k, v)
		}
	}
	if v, ok := gpsprot.CfgBaudRate.Get(cm); ok {
		k := portBaudRateKey(port)
		if k != 0 {
			ucv.AddItem(&items, k, uint64(v))
		}
	}
	enaKeys := map[gpsprot.GNSS]ucv.KeyL{
		gpsprot.GPS:   ucv.KSignalGpsEna,
		gpsprot.GAL:   ucv.KSignalGalEna,
		gpsprot.GLO:   ucv.KSignalGloEna,
		gpsprot.BDS:   ucv.KSignalBdsEna,
		gpsprot.NAVIC: ucv.KSignalNavicEna,
		gpsprot.QZSS:  ucv.KSignalQzssEna,
		gpsprot.SBAS:  ucv.KSignalSbasEna,
	}
	if v, ok := gpsprot.CfgGNSSEnabled.Get(cm); ok {
		supportedGNSS := ver.GNSS
		if v&supportedGNSS&gpsprot.MajorGNSSSet == 0 {
			return nil, nil, errors.New("must enable at least one major GNSS")
		}
		for g, k := range enaKeys {
			if supportedGNSS.Contains(g) {
				ucv.AddItem(&items, k, v.Contains(g))
			}
		}
	}
	if opts.EnableTimeMsg {
		ucv.AddItem(&items, timegridTp1ToMsgRateKey(tg).KeyU(port), 1)
		ucv.AddItem(&items, ucv.KUbxTimTp.KeyU(port), 1)
	}
	if opts.EnableLeapSecondMsg {
		ucv.AddItem(&items, ucv.KUbxNavTimels.KeyU(port), 1)
	}
	return items, keys, nil
}

func (known *CfgVals) configPreSetItems(cm *gpsprot.ConfigMap, opts gpsprot.ConfigOptions) []ucv.Item {
	if tm, ok := cfgValGet(known, ucv.KTmodeMode); !ok || tm != ucv.ETmodeModeSurveyIn {
		return nil
	}
	// we know the receiver is in currenntly in survey mode
	// Do we want to start a survey when we are in survey mode?
	if !opts.Survey.When.Contains(gpsprot.TimeModeSurvey) {
		return nil
	}
	// If so, we need to set the mode to Disabled first
	return []ucv.Item{
		ucv.MakeItem(ucv.KTmodeMode, ucv.ETmodeModeDisabled),
		// on F9P HPG 1.12, it's not enough just to set the mode to Disabled
		// this seems to do the trick
		// actually not: seems only changing the survey parameters from the last survey
		ucv.MakeItem(ucv.KTmodeSvinAccLimit, 0),
		ucv.MakeItem(ucv.KTmodeSvinMinDur, 0),
	}
}

func (known *CfgVals) changeTimeMode(cm *gpsprot.ConfigMap, opts gpsprot.ConfigOptions, ver *Version, port ucv.Port, items *[]ucv.Item, keys *[]ucv.Key) error {
	svMsgKey := ucv.KUbxTimSvin
	switch ver.ProductCategory() {
	case "FTS": // do nothing
	case "TIM": // do nothing
	case "HPG":
		svMsgKey = ucv.KUbxNavSvin
	default:
		return nil
	}
	if v, ok := gpsprot.CfgFixedPosECEF.Get(cm); ok {
		err := addTmodeECEF(items, v)
		if err != nil {
			return err
		}
	}
	tmReq := gpsprot.TimeMode(0)
	tmReq, _ = gpsprot.CfgTimeMode.Get(cm)
	tmKnown := gpsprot.TimeMode(0)
	if tm, ok := cfgValGet(known, ucv.KTmodeMode); ok {
		tmKnown = tmodeModeToTimeMode(tm)
	}
	when := opts.Survey.When
	if tmKnown == 0 {
		// We need to know the current time mode if we might initiate a survey
		needToKnow := false
		if tmReq != 0 {
			needToKnow = when.Contains(tmReq)
		} else {
			// need to know, if we might
			needToKnow = when != 0
		}
		if needToKnow {
			ucv.AddKey(keys, ucv.KTmodeMode)
			return nil
		}
	}
	tm := tmKnown
	if tmReq != 0 {
		tm = tmReq
	}
	if tm == 0 {
		// no time mode known, no time mode requested and no possibility of starting a survey
		return nil
	}
	if when.Contains(tm) {
		// need to do a survey
		ucv.AddItem(items, ucv.KTmodeMode, ucv.ETmodeModeSurveyIn)
		ucv.AddItem(items, ucv.KTmodeSvinMinDur, uint64(opts.Survey.MinDur.Round(time.Second)/time.Second))
		var mm10 int64
		mm10, _ = divModRound(int64(opts.Survey.AccLimit), int64(gpsprot.Millimeter/10))
		ucv.AddItem(items, ucv.KTmodeSvinAccLimit, uint64(mm10))
		ucv.AddItem(items, svMsgKey.KeyU(port), 1)
		return nil
	}

	if tmReq != gpsprot.TimeModeSurvey {
		ucv.AddItem(items, ucv.KTmodeMode, timeModeToTmodeMode(tmReq))
		ucv.AddItem(items, svMsgKey.KeyU(port), 0)
		return nil
	}
	// Remaining possibility:
	// The user requested TimeMode=Survey in the ConfigMap,
	// but in ConfigOptions says not to start a Survey when the TimeMode is Survey.
	// This is actually OK, if the receiver is already in Survey mode.
	return nil
}

func (raw *CfgVals) cookTmodeECEF() (gpsprot.Point3D, bool) {
	pt := gpsprot.Point3D{}
	ty, ok := cfgValGet(raw, ucv.KTmodePosType)
	if !ok || ty != ucv.ETmodePosTypeEcef {
		return pt, false
	}
	kecef := []ucv.KeyI{ucv.KTmodeEcefX, ucv.KTmodeEcefY, ucv.KTmodeEcefZ}
	kecefhp := []ucv.KeyI{ucv.KTmodeEcefXHp, ucv.KTmodeEcefYHp, ucv.KTmodeEcefZHp}
	for i := 0; i < 3; i++ {
		v, ok := cfgValGet(raw, kecef[i])
		if !ok {
			return pt, false
		}
		hp, ok := cfgValGet(raw, kecefhp[i])
		if !ok {
			return pt, false
		}
		pt[i] = lengthHP(int32(v), int8(hp))
	}
	return pt, true
}

func addTmodeECEF(items *[]ucv.Item, p gpsprot.Point3D) error {
	kecef := []ucv.KeyI{ucv.KTmodeEcefX, ucv.KTmodeEcefY, ucv.KTmodeEcefZ}
	kecefhp := []ucv.KeyI{ucv.KTmodeEcefXHp, ucv.KTmodeEcefYHp, ucv.KTmodeEcefZHp}
	for i := 0; i < 3; i++ {
		cm, frac, err := splitLength(p[i])
		if err != nil {
			return err
		}
		ucv.AddItem(items, kecef[i], int64(cm))
		ucv.AddItem(items, kecefhp[i], int64(frac))
	}
	ucv.AddItem(items, ucv.KTmodePosType, ucv.ETmodePosTypeEcef)
	return nil
}

func (raw *CfgVals) getTimePulseOnlyWhenLocked() (v bool, ok bool) {
	if v, ok := cfgValGet(raw, ucv.KTpUseLockedTp1); !ok || !v {
		return false, false
	}
	if v, ok := cfgValGet(raw, ucv.KTpPulseLengthDef); !ok || v != ucv.ETpPulseLengthDefLength {
		return false, false
	}
	if v, ok := cfgValGet(raw, ucv.KTpLenTp1); ok {
		return v == 0, true
	}
	return false, false
}

func (raw *CfgVals) getTimePulseAlignToGNSS() (v bool, ok bool) {
	v1, ok1 := cfgValGet(raw, ucv.KTpAlignToTowTp1)
	v2, ok2 := cfgValGet(raw, ucv.KTpSyncGnssTp1)
	ok = ok1 && ok2
	v = v1 && v2
	return
}

func (raw *CfgVals) getTimePulsePeriod() (time.Duration, bool) {
	if v, ok := cfgValGet(raw, ucv.KTpPulseDef); !ok || v != ucv.ETpPulseDefPeriod {
		return 0, false
	}
	if v, ok := cfgValGet(raw, ucv.KTpUseLockedTp1); !ok || !v {
		return 0, false
	}
	if v, ok := cfgValGet(raw, ucv.KTpPeriodLockTp1); ok {
		return time.Duration(v) * time.Microsecond, true
	}
	return 0, false
}

func (raw *CfgVals) getTimePulseWidth() (time.Duration, bool) {
	if v, ok := cfgValGet(raw, ucv.KTpPulseLengthDef); !ok || v != ucv.ETpPulseLengthDefLength {
		return 0, false
	}
	if v, ok := cfgValGet(raw, ucv.KTpUseLockedTp1); !ok || !v {
		return 0, false
	}
	if v, ok := cfgValGet(raw, ucv.KTpLenLockTp1); ok {
		return time.Duration(v) * time.Microsecond, true
	}
	return 0, false
}

// changeTimePulse compiles the parts of the configuration related to the time pulse.
// If it infers the GNSS to which the pulse is aligned, it returns that.
func (known *CfgVals) changeTimePulse(cm *gpsprot.ConfigMap, items *[]ucv.Item, keys *[]ucv.Key) ucv.EnumTpTimegridTp1 {
	tg := ucv.ETpTimegridTp1Utc
	period, havePeriod := gpsprot.CfgTimePulsePeriod.Get(cm)
	width, haveWidth := gpsprot.CfgTimePulseWidth.Get(cm)
	align, haveAlign := gpsprot.CfgTimePulseAlignToGNSS.Get(cm)
	onlyWhenLocked, haveOnlyWhenLocked := gpsprot.CfgTimePulseOnlyWhenLocked.Get(cm)
	useLock := align
	if haveAlign {
		ucv.AddItem(items, ucv.KTpUseLockedTp1, align)
		ucv.AddItem(items, ucv.KTpAlignToTowTp1, align)
		ucv.AddItem(items, ucv.KTpSyncGnssTp1, align)
		if _, ok := gpsprot.CfgPrimaryGNSS.Get(cm); !ok {
			tg = known.inferTimegridTp1(cm, items, keys)
		}
	} else {
		if havePeriod || haveWidth {
			if v, ok := cfgValGet(known, ucv.KTpUseLockedTp1); ok {
				useLock = v
			} else {
				// need more info
				ucv.AddKey(keys, ucv.KTpUseLockedTp1)
			}
		}
	}
	if havePeriod {
		us := uint64(period.Round(time.Microsecond) / time.Microsecond)
		// always set unlocked period
		ucv.AddItem(items, ucv.KTpPeriodTp1, us)
		if useLock {
			ucv.AddItem(items, ucv.KTpPeriodLockTp1, us)
		}
		ucv.AddItem(items, ucv.KTpPulseDef, ucv.ETpPulseDefPeriod)
	}
	if haveWidth {
		us := uint64(width.Round(time.Microsecond) / time.Microsecond)
		if useLock {
			ucv.AddItem(items, ucv.KTpLenLockTp1, us)
			if haveOnlyWhenLocked {
				usNoLock := us
				if onlyWhenLocked {
					usNoLock = 0
				}
				ucv.AddItem(items, ucv.KTpLenTp1, usNoLock)
			} else {
				known.inferTpLenTp1(us, cm, items, keys)
			}
		} else {
			ucv.AddItem(items, ucv.KTpLenTp1, us)
		}
		if us != 0 {
			ucv.AddItem(items, ucv.KTpTp1Ena, true)
		}
		ucv.AddItem(items, ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength)
	}
	return tg
}

func (known *CfgVals) inferTpLenTp1(lenLock uint64, cm *gpsprot.ConfigMap, items *[]ucv.Item, keys *[]ucv.Key) {
	if def, ok := cfgValGet(known, ucv.KTpPulseLengthDef); ok {
		if def == ucv.ETpPulseLengthDefLength {
			// we can just leave as is
			return
		}
		if duty, ok := cfgValGet(known, ucv.KTpDutyTp1); ok {
			l := lenLock
			if duty == 0 {
				l = 0
			}
			// our model is that the unlocked pulse length is either zero or the same as the locked pulse length
			ucv.AddItem(items, ucv.KTpLenTp1, l)
			return
		}
	} else {
		ucv.AddKey(keys, ucv.KTpPulseLengthDef)
	}
	ucv.AddKey(keys, ucv.KTpDutyTp1)
}

func (known *CfgVals) inferTimegridTp1(cm *gpsprot.ConfigMap, items *[]ucv.Item, keys *[]ucv.Key) ucv.EnumTpTimegridTp1 {
	missing := []ucv.Key(nil)
	if tg, ok := cfgValGetMiss(known, ucv.KTpTimegridTp1, &missing); ok && tg != ucv.ETpTimegridTp1Utc {
		return tg
	}
	if u, ok := cfgValGetMiss(known, ucv.KNavspgUtcstandard, &missing); ok && len(missing) == 0 {
		tg := navspgUtcstandardToTimegridTp1(u)
		if tg != ucv.ETpTimegridTp1Utc {
			ucv.AddItem(items, ucv.KTpTimegridTp1, tg)
			return tg
		}
	}
	if false {
		// This isn't a good idea because the default for this is GPS.
		if r, ok := cfgValGetMiss(known, ucv.KRateTimeref, &missing); ok && len(missing) == 0 {
			tg := rateTimeRefToTimegridTp1(r)
			if tg != ucv.ETpTimegridTp1Utc {
				ucv.AddItem(items, ucv.KTpTimegridTp1, tg)
				return tg
			}
		}
	}

	// Fall back to first of GPS, Galileo, BeiDou, GLONASS that is enabled (in that order)
	sigEna := []ucv.KeyL{ucv.KSignalGpsEna, ucv.KSignalGalEna, ucv.KSignalBdsEna, ucv.KSignalGloEna}
	sigTg := []ucv.EnumTpTimegridTp1{ucv.ETpTimegridTp1Gps, ucv.ETpTimegridTp1Gal, ucv.ETpTimegridTp1Bds, ucv.ETpTimegridTp1Glo}
	for i, sig := range sigEna {
		if ena, ok := cfgValGetMiss(known, sig, &missing); ok && ena && len(missing) == 0 {
			ucv.AddItem(items, ucv.KTpTimegridTp1, sigTg[i])
			return sigTg[i]
		}
	}
	*keys = append(*keys, missing...)
	return ucv.ETpTimegridTp1Utc
}

func cfgValSet[T comparable](vals *CfgVals, k ucv.TypedKey[T], v T) {
	ucv.MapSet(vals.Map, k, v)
}

func cfgValGet[T comparable](vals *CfgVals, k ucv.TypedKey[T]) (t T, ok bool) {
	return ucv.MapGet(vals.Map, k)
}

func cfgValGetMiss[T comparable](vals *CfgVals, k ucv.TypedKey[T], missing *[]ucv.Key) (t T, ok bool) {
	t, ok = cfgValGet(vals, k)
	if !ok {
		ucv.AddKey(missing, k)
	}
	return
}

func navspgUtcstandardToTimegridTp1(u ucv.EnumNavspgUtcstandard) ucv.EnumTpTimegridTp1 {
	switch u {
	case ucv.ENavspgUtcstandardUsno:
		return ucv.ETpTimegridTp1Gps
	case ucv.ENavspgUtcstandardEu:
		return ucv.ETpTimegridTp1Gal
	case ucv.ENavspgUtcstandardSu:
		return ucv.ETpTimegridTp1Glo
	case ucv.ENavspgUtcstandardNtsc:
		return ucv.ETpTimegridTp1Bds
	case ucv.ENavspgUtcstandardNpli:
		return ucv.ETpTimegridTp1Navic
	//	case ucv.ENavspgUtcstandardNict:
	//		return ucv.ETpTimegridTp1Qzss

	default:
		return ucv.ETpTimegridTp1Utc
	}
}

func rateTimeRefToTimegridTp1(r ucv.EnumRateTimeref) ucv.EnumTpTimegridTp1 {
	switch r {
	case ucv.ERateTimerefGps:
		return ucv.ETpTimegridTp1Gps
	case ucv.ERateTimerefGal:
		return ucv.ETpTimegridTp1Gal
	case ucv.ERateTimerefGlo:
		return ucv.ETpTimegridTp1Glo
	case ucv.ERateTimerefBds:
		return ucv.ETpTimegridTp1Bds
	default:
		return ucv.ETpTimegridTp1Utc
	}
}

func gnssToRateTimeref(g gpsprot.GNSS) ucv.EnumRateTimeref {
	switch g {
	case gpsprot.GPS:
		return ucv.ERateTimerefGps
	case gpsprot.GAL:
		return ucv.ERateTimerefGal
	case gpsprot.GLO:
		return ucv.ERateTimerefGlo
	case gpsprot.BDS:
		return ucv.ERateTimerefBds
	default:
		return ucv.ERateTimerefUtc
	}
}

func gnssToEnumNavspgUtcstandard(g gpsprot.GNSS) ucv.EnumNavspgUtcstandard {
	switch g {
	case gpsprot.GPS:
		return ucv.ENavspgUtcstandardUsno
	case gpsprot.GAL:
		return ucv.ENavspgUtcstandardEu
	case gpsprot.GLO:
		return ucv.ENavspgUtcstandardSu
	case gpsprot.BDS:
		return ucv.ENavspgUtcstandardNtsc
	case gpsprot.QZSS:
		return ucv.ENavspgUtcstandardNict
	case gpsprot.NAVIC:
		return ucv.ENavspgUtcstandardNpli
	}
	return ucv.ENavspgUtcstandardAuto
}

func gnssToTimegridTp1(g gpsprot.GNSS) ucv.EnumTpTimegridTp1 {
	switch g {
	case gpsprot.GPS:
		return ucv.ETpTimegridTp1Gps
	case gpsprot.GAL:
		return ucv.ETpTimegridTp1Gal
	case gpsprot.GLO:
		return ucv.ETpTimegridTp1Glo
	case gpsprot.BDS:
		return ucv.ETpTimegridTp1Bds
	case gpsprot.NAVIC:
		return ucv.ETpTimegridTp1Navic
	default:
		return ucv.ETpTimegridTp1Utc
	}
}

func timeModeToTmodeMode(t gpsprot.TimeMode) ucv.EnumTmodeMode {
	switch t {
	case gpsprot.TimeModeSurvey:
		return ucv.ETmodeModeSurveyIn
	case gpsprot.TimeModeFixed:
		return ucv.ETmodeModeFixed
	default:
		return ucv.ETmodeModeDisabled
	}
}

func tmodeModeToTimeMode(t ucv.EnumTmodeMode) gpsprot.TimeMode {
	switch t {
	case ucv.ETmodeModeSurveyIn:
		return gpsprot.TimeModeSurvey
	case ucv.ETmodeModeFixed:
		return gpsprot.TimeModeFixed
	default:
		return gpsprot.TimeModeDisabled
	}
}

func timegridTp1ToMsgRateKey(tg ucv.EnumTpTimegridTp1) ucv.KeyM {
	switch tg {
	case ucv.ETpTimegridTp1Gps:
		return ucv.KUbxNavTimegps
	case ucv.ETpTimegridTp1Gal:
		return ucv.KUbxNavTimegal
	case ucv.ETpTimegridTp1Glo:
		return ucv.KUbxNavTimeglo
	case ucv.ETpTimegridTp1Bds:
		return ucv.KUbxNavTimebds
	default:
		return ucv.KUbxNavTimeutc
	}
}

func portOutprotNmeaKey(port ucv.Port) ucv.KeyL {
	switch port {
	case ucv.UART1:
		return ucv.KUart1outprotNmea
	case ucv.UART2:
		return ucv.KUart2outprotNmea
	case ucv.USB:
		return ucv.KUsboutprotNmea
	}
	return 0
}

func portBaudRateKey(port ucv.Port) ucv.KeyU {
	switch port {
	case ucv.UART1:
		return ucv.KUart1Baudrate
	case ucv.UART2:
		return ucv.KUart2Baudrate
	}
	return 0
}

// splitLength splits a Length into a int32 and int8.
// The int32 is the length in centimeters. The int8 is the remainder in units of 0.1mm.
func splitLength(n gpsprot.Length) (int32, int8, error) {
	q, r := divModRound(int64(n), int64(gpsprot.Centimeter))
	if q < math.MinInt32 || q > math.MaxInt32 {
		return 0, 0, fmt.Errorf("length %v is out of range", n)
	}
	cm := int32(q)
	q, _ = divModRound(r, int64(gpsprot.Millimeter/10))
	return cm, int8(q), nil
}
