package ubx

import (
	"errors"
	"time"

	"github.com/jclark/satpulse/internal/gpsmsg"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

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
	ucv.KUbxTimTp,
}

// configItems turns a Config into a list of UBX configuration items.
// The known argument gives what is known about the current configuration.
// If additional keys are needed to compile the configuration correctly,
// they are returned in the ucv.Key slice.
// Typically this function will get called twice.
// The first time, known will be empty, and some more keys will be needed.
// The caller will then fetch the additional keys, add them to known and call again.
func configItems(cm *gpsmsg.ConfigMap, opts gpsmsg.ConfigOptions, supportedGNSS gpsmsg.GNSSSet, known ucv.Map, port ucv.Port) ([]ucv.Item, []ucv.Key, error) {
	items := []ucv.Item{}
	keys := []ucv.Key{}
	tg := compileTimePulse(known, cm, &items, &keys)
	if v, ok := gpsmsg.CfgAntennaCableDelay.Get(cm); ok {
		ucv.AddItem(&items, ucv.KTpAntCabledelay, int64(v))
	}
	if v, ok := gpsmsg.CfgTimePulsePolarityRising.Get(cm); ok {
		ucv.AddItem(&items, ucv.KTpPolTp1, v)
	}
	if v, ok := gpsmsg.CfgPrimaryGNSS.Get(cm); ok {
		tg = gnssToTimegridTp1(v)
		ucv.AddItem(&items, ucv.KTpTimegridTp1, tg)
		ucv.AddItem(&items, ucv.KNavspgUtcstandard, gnssToEnumNavspgUtcstandard(v))
		ucv.AddItem(&items, ucv.KRateTimeref, gnssToRateTimeref(v))
	}
	if v, ok := gpsmsg.CfgSolutionPeriod.Get(cm); ok {
		ucv.AddItem(&items, ucv.KRateMeas, uint64(uint16(v.Round(time.Millisecond)/time.Millisecond)))
		ucv.AddItem(&items, ucv.KRateNav, 1)
	}
	if v, ok := gpsmsg.CfgStationary.Get(cm); ok {
		dm := ucv.ENavspgDynmodelPort
		if v {
			dm = ucv.ENavspgDynmodelStat
		}
		ucv.AddItem(&items, ucv.KNavspgDynmodel, dm)
	}
	if v, ok := gpsmsg.CfgTimeMode.Get(cm); ok {
		ucv.AddItem(&items, ucv.KTmodeMode, timeModeToTmodeMode(v))
	}
	if v, ok := gpsmsg.CfgFixedPosECEF.Get(cm); ok {
		err := addTmodeECEF(&items, v)
		if err != nil {
			return nil, nil, err
		}
	}
	if v, ok := gpsmsg.CfgNMEAEnabled.Get(cm); ok {
		k := portOutprotNmeaKey(port)
		if k != 0 {
			ucv.AddItem(&items, k, v)
		}
	}
	if v, ok := gpsmsg.CfgBaudRate.Get(cm); ok {
		k := portBaudRateKey(port)
		if k != 0 {
			ucv.AddItem(&items, k, uint64(v))
		}
	}
	enaKeys := map[gpsmsg.GNSS]ucv.KeyL{
		gpsmsg.GPS:   ucv.KSignalGpsEna,
		gpsmsg.GAL:   ucv.KSignalGalEna,
		gpsmsg.GLO:   ucv.KSignalGloEna,
		gpsmsg.BDS:   ucv.KSignalBdsEna,
		gpsmsg.NAVIC: ucv.KSignalNavicEna,
		gpsmsg.QZSS:  ucv.KSignalQzssEna,
		gpsmsg.SBAS:  ucv.KSignalSbasEna,
	}
	if v, ok := gpsmsg.CfgGNSSEnabled.Get(cm); ok {
		if v&supportedGNSS&gpsmsg.MajorGNSSSet == 0 {
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

func addTmodeECEF(items *[]ucv.Item, p gpsmsg.Point3D) error {
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
	return nil
}

// compileTimePulse compiles the parts of the configuration related to the time pulse.
// If it infers the GNSS to which the pulse is aligned, it returns that.
func compileTimePulse(known ucv.Map, cm *gpsmsg.ConfigMap, items *[]ucv.Item, keys *[]ucv.Key) ucv.EnumTpTimegridTp1 {
	tg := ucv.ETpTimegridTp1Utc
	period, havePeriod := gpsmsg.CfgTimePulsePeriod.Get(cm)
	width, haveWidth := gpsmsg.CfgTimePulseWidth.Get(cm)
	align, haveAlign := gpsmsg.CfgTimePulseAlignToGNSS.Get(cm)
	onlyWhenLocked, haveOnlyWhenLocked := gpsmsg.CfgTimePulseOnlyWhenLocked.Get(cm)
	useLock := align
	if haveAlign {
		ucv.AddItem(items, ucv.KTpUseLockedTp1, align)
		ucv.AddItem(items, ucv.KTpAlignToTowTp1, align)
		ucv.AddItem(items, ucv.KTpSyncGnssTp1, align)
		if _, ok := gpsmsg.CfgPrimaryGNSS.Get(cm); !ok {
			tg = inferTimegridTp1(known, cm, items, keys)
		}
	} else {
		if havePeriod || haveWidth {
			if v, ok := ucv.MapGet(known, ucv.KTpUseLockedTp1); ok {
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
				inferTpLenTp1(us, known, cm, items, keys)
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

func inferTpLenTp1(lenLock uint64, known ucv.Map, cm *gpsmsg.ConfigMap, items *[]ucv.Item, keys *[]ucv.Key) {
	if def, ok := ucv.MapGet(known, ucv.KTpPulseLengthDef); ok {
		if def == ucv.ETpPulseLengthDefLength {
			// we can just leave as is
			return
		}
		if duty, ok := ucv.MapGet(known, ucv.KTpDutyTp1); ok {
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

func inferTimegridTp1(known ucv.Map, cm *gpsmsg.ConfigMap, items *[]ucv.Item, keys *[]ucv.Key) ucv.EnumTpTimegridTp1 {
	missing := []ucv.Key(nil)
	if tg, ok := mapGetMiss(known, ucv.KTpTimegridTp1, &missing); ok && tg != ucv.ETpTimegridTp1Utc {
		return tg
	}
	if u, ok := mapGetMiss(known, ucv.KNavspgUtcstandard, &missing); ok && len(missing) == 0 {
		tg := navspgUtcstandardToTimegridTp1(u)
		if tg != ucv.ETpTimegridTp1Utc {
			ucv.AddItem(items, ucv.KTpTimegridTp1, tg)
			return tg
		}
	}
	if false {
		// This isn't a good idea because the default for this is GPS.
		if r, ok := mapGetMiss(known, ucv.KRateTimeref, &missing); ok && len(missing) == 0 {
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
		if ena, ok := mapGetMiss(known, sig, &missing); ok && ena && len(missing) == 0 {
			ucv.AddItem(items, ucv.KTpTimegridTp1, sigTg[i])
			return sigTg[i]
		}
	}
	*keys = append(*keys, missing...)
	return ucv.ETpTimegridTp1Utc
}

func mapGetMiss[T comparable](m ucv.Map, k ucv.TypedKey[T], missing *[]ucv.Key) (t T, ok bool) {
	t, ok = ucv.MapGet(m, k)
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

func gnssToRateTimeref(g gpsmsg.GNSS) ucv.EnumRateTimeref {
	switch g {
	case gpsmsg.GPS:
		return ucv.ERateTimerefGps
	case gpsmsg.GAL:
		return ucv.ERateTimerefGal
	case gpsmsg.GLO:
		return ucv.ERateTimerefGlo
	case gpsmsg.BDS:
		return ucv.ERateTimerefBds
	default:
		return ucv.ERateTimerefUtc
	}
}

func gnssToEnumNavspgUtcstandard(g gpsmsg.GNSS) ucv.EnumNavspgUtcstandard {
	switch g {
	case gpsmsg.GPS:
		return ucv.ENavspgUtcstandardUsno
	case gpsmsg.GAL:
		return ucv.ENavspgUtcstandardEu
	case gpsmsg.GLO:
		return ucv.ENavspgUtcstandardSu
	case gpsmsg.BDS:
		return ucv.ENavspgUtcstandardNtsc
	case gpsmsg.QZSS:
		return ucv.ENavspgUtcstandardNict
	case gpsmsg.NAVIC:
		return ucv.ENavspgUtcstandardNpli
	}
	return ucv.ENavspgUtcstandardAuto
}

func gnssToTimegridTp1(g gpsmsg.GNSS) ucv.EnumTpTimegridTp1 {
	switch g {
	case gpsmsg.GPS:
		return ucv.ETpTimegridTp1Gps
	case gpsmsg.GAL:
		return ucv.ETpTimegridTp1Gal
	case gpsmsg.GLO:
		return ucv.ETpTimegridTp1Glo
	case gpsmsg.BDS:
		return ucv.ETpTimegridTp1Bds
	case gpsmsg.NAVIC:
		return ucv.ETpTimegridTp1Navic
	default:
		return ucv.ETpTimegridTp1Utc
	}
}

func timeModeToTmodeMode(t gpsmsg.TimeMode) ucv.EnumTmodeMode {
	switch t {
	case gpsmsg.TimeModeSurvey:
		return ucv.ETmodeModeSurveyIn
	case gpsmsg.TimeModeFixed:
		return ucv.ETmodeModeFixed
	default:
		return ucv.ETmodeModeDisabled
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
