package ubx

import (
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	ucv "github.com/jclark/gps4ptp/internal/ubxcfgval"
)

// Compile turns a Config into a list of UBX configuration items.
// The known argument gives what is known about the current configuration.
// If additional keys are needed to compile the configuration correctly,
// they are returned in the ucv.Key slice.
// Typically this function will get called twice.
// The first time, known will be empty, and some more keys will be needed.
// The caller will then fetch the additional keys, add them to known and call again.
func Compile(config *gpsmsg.Config, known ucv.Map, port ucv.Port) ([]ucv.Item, []ucv.Key) {
	items := []ucv.Item{}
	keys := []ucv.Key{}
	tg := compileTimePulse(known, config, &items, &keys)
	if v, ok := gpsmsg.CfgAntennaCableDelay.Get(config); ok {
		ucv.AddItem(&items, ucv.KTpAntCabledelay, int64(v))
	}
	if v, ok := gpsmsg.CfgTimePulsePolarityRising.Get(config); ok {
		ucv.AddItem(&items, ucv.KTpPolTp1, v)
	}
	if v, ok := gpsmsg.CfgPrimaryGNSS.Get(config); ok {
		tg = majorGNSSToTimegridTp1(v)
		ucv.AddItem(&items, ucv.KTpTimegridTp1, tg)
		ucv.AddItem(&items, ucv.KNavspgUtcstandard, majorGNSSToEnumNavspgUtcstandard(v))
		ucv.AddItem(&items, ucv.KRateTimeref, majorGNSSToRateTimeref(v))
	}
	if v, ok := gpsmsg.CfgSolutionPeriod.Get(config); ok {
		ucv.AddItem(&items, ucv.KRateMeas, uint64(uint16(v.Round(time.Millisecond)/time.Millisecond)))
		ucv.AddItem(&items, ucv.KRateNav, 1)
	}
	if v, ok := gpsmsg.CfgStationary.Get(config); ok {
		dm := ucv.ENavspgDynmodelPort
		if v {
			dm = ucv.ENavspgDynmodelStat
		}
		ucv.AddItem(&items, ucv.KNavspgDynmodel, dm)
	}
	if v, ok := gpsmsg.CfgNMEAEnabled.Get(config); ok {
		k := portOutprotNmeaKey(port)
		if k != 0 {
			ucv.AddItem(&items, k, v)
		}
	}
	ucv.AddItem(&items, timegridTp1ToMsgRateKey(tg).KeyU(port), 1)
	ucv.AddItem(&items, ucv.KUbxTimTp.KeyU(port), 1)
	return items, keys
}

// compileTimePulse compiles the parts of the configuration related to the time pulse.
// If it infers the GNSS to which the pulse is aligned, it returns that.
func compileTimePulse(known ucv.Map, config *gpsmsg.Config, items *[]ucv.Item, keys *[]ucv.Key) ucv.EnumTpTimegridTp1 {
	tg := ucv.ETpTimegridTp1Utc
	period, havePeriod := gpsmsg.CfgTimePulsePeriod.Get(config)
	width, haveWidth := gpsmsg.CfgTimePulseWidth.Get(config)
	align, haveAlign := gpsmsg.CfgTimePulseAlignToGNSS.Get(config)
	onlyWhenLocked, haveOnlyWhenLocked := gpsmsg.CfgTimePulseOnlyWhenLocked.Get(config)
	useLock := align
	if haveAlign {
		ucv.AddItem(items, ucv.KTpUseLockedTp1, align)
		ucv.AddItem(items, ucv.KTpAlignToTowTp1, align)
		ucv.AddItem(items, ucv.KTpSyncGnssTp1, align)
		if _, ok := gpsmsg.CfgPrimaryGNSS.Get(config); !ok {
			tg = inferTimegridTp1(known, config, items, keys)
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
				inferTpLenTp1(us, known, config, items, keys)
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

func inferTpLenTp1(lenLock uint64, known ucv.Map, config *gpsmsg.Config, items *[]ucv.Item, keys *[]ucv.Key) {
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

func inferTimegridTp1(known ucv.Map, config *gpsmsg.Config, items *[]ucv.Item, keys *[]ucv.Key) ucv.EnumTpTimegridTp1 {
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
	if r, ok := mapGetMiss(known, ucv.KRateTimeref, &missing); ok && len(missing) == 0 {
		tg := rateTimeRefToTimegridTp1(r)
		if tg != ucv.ETpTimegridTp1Utc {
			ucv.AddItem(items, ucv.KTpTimegridTp1, tg)
			return tg
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

func majorGNSSToRateTimeref(g gpsmsg.MajorGNSS) ucv.EnumRateTimeref {
	switch g {
	case gpsmsg.GPS:
		return ucv.ERateTimerefGps
	case gpsmsg.Galileo:
		return ucv.ERateTimerefGal
	case gpsmsg.GLONASS:
		return ucv.ERateTimerefGlo
	case gpsmsg.BeiDou:
		return ucv.ERateTimerefBds
	default:
		return ucv.ERateTimerefUtc
	}
}

func majorGNSSToEnumNavspgUtcstandard(g gpsmsg.MajorGNSS) ucv.EnumNavspgUtcstandard {
	switch g {
	case gpsmsg.GPS:
		return ucv.ENavspgUtcstandardUsno
	case gpsmsg.Galileo:
		return ucv.ENavspgUtcstandardEu
	case gpsmsg.GLONASS:
		return ucv.ENavspgUtcstandardSu
	case gpsmsg.BeiDou:
		return ucv.ENavspgUtcstandardNtsc
	}
	return ucv.ENavspgUtcstandardAuto
}

func majorGNSSToTimegridTp1(g gpsmsg.MajorGNSS) ucv.EnumTpTimegridTp1 {
	switch g {
	case gpsmsg.GPS:
		return ucv.ETpTimegridTp1Gps
	case gpsmsg.Galileo:
		return ucv.ETpTimegridTp1Gal
	case gpsmsg.GLONASS:
		return ucv.ETpTimegridTp1Glo
	case gpsmsg.BeiDou:
		return ucv.ETpTimegridTp1Bds
	default:
		return ucv.ETpTimegridTp1Utc
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
	}
	return 0
}
