package ubx

import (
	"errors"
	"slices"
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
	ucv.KUbxNavSat,
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

var cfgValKeysByProp = map[gpsprot.PropIDs][]ucv.AnyTypedKey{
	gpsprot.PropIDTimePulseWidth: {
		ucv.KTpPulseLengthDef,
		ucv.KTpUseLockedTp1,
		ucv.KTpLenLockTp1,
		ucv.KTpLenTp1,
	},
}

func MakeCfgVals() CfgVals {
	return CfgVals{make(ucv.Map)}
}

func (vals *CfgVals) valsPtr() *CfgVals {
	if vals.isNil() {
		*vals = MakeCfgVals()
	}
	return vals
}

func (vals *CfgVals) isNil() bool {
	return vals.Map == nil
}

// AddData adds cfgData containing items to the CfgVals map.
// It returns a map of the groups that were added.
func (raw *CfgVals) AddData(cfgData []byte) (map[uint8]struct{}, error) {
	items, err := ucv.UnmarshalItems(cfgData)
	if err != nil {
		return nil, err
	}
	groups := make(map[uint8]struct{})
	for _, item := range items {
		groups[item.Key.Group()] = struct{}{}
	}
	raw.AddItems(items)
	return groups, nil
}

func (raw *CfgVals) Cook(ver *Version, port ucv.Port, cp *gpsprot.ConfigProps) {
	if v, ok := raw.getSignalsEnabled(); ok {
		cp.SetSignalsEnabled(v)
	}
	if v, ok := cfgValGet(raw, ucv.KTmodeMode); ok {
		cp.SetTimeMode(tmodeModeToTimeMode(v))
	}
	if v, ok := raw.cookTmodeECEF(); ok {
		cp.SetFixedPosECEF(v)
	}
	if v, ok := raw.getMode(); ok {
		cp.SetMode(v)
	}
	if v, ok := raw.getTimePulseAlignToGNSS(); ok {
		cp.SetTimePulseAlignToGNSS(v)
	}
	if v, ok := raw.getTimePulsePeriod(); ok {
		cp.SetTimePulsePeriod(v)
	}
	if v, ok := raw.getTimePulseWidth(); ok {
		cp.SetTimePulseWidth(v)
	}
	if v, ok := raw.getTimePulseOnlyWhenLocked(); ok {
		cp.SetTimePulseOnlyWhenLocked(v)
	}
	if v, ok := cfgValGet(raw, ucv.KTpAntCabledelay); ok {
		cp.SetAntennaCableDelay(time.Duration(v))
	}
	if v, ok := cfgValGet(raw, ucv.KTpPolTp1); ok {
		cp.SetTimePulsePolarityRising(v)
	}
	if v, ok := cfgValGet(raw, ucv.KNavspgDynmodel); ok {
		cp.SetStationary(v == ucv.ENavspgDynmodelStat)
	}
}

// Transaction determines the transaction to achieve the specified target.
// The known argument gives what is known about the current configuration.
// The returned items specifies configuration database changes that are needed.
// If the specified changes cannot be determined because the value of some keys is not known, those keys are returned in the ucv.Key slice.
// Typically this function will get called twice.
// The first time, known will be empty, and some more keys will be needed.
// The caller will then fetch the additional keys, add them to known and call again.
func (known *CfgVals) Transaction(target *gpsprot.ConfigTarget, ver *Version, port ucv.Port, monEnabledGNSS gpsprot.GNSSSet) ([]ucv.Item, []ucv.Key, error) {
	tb := newTxnBuilder(known, target, ver, port, monEnabledGNSS)
	err := tb.build()
	if err != nil {
		return nil, nil, err
	}
	return tb.items, tb.keys, nil
}

// txnBuilder builds a configuration transaction by accumulating the configuration database
// changes needed to achieve the target configuration. The builder tracks both required
// config items and any additional keys that need to be queried.
type txnBuilder struct {
	known          *CfgVals
	monEnabledGNSS gpsprot.GNSSSet // enabled GNSS from UBX-MON-GNSS, zero value means no MON-GNSS info available
	target         *gpsprot.ConfigTarget
	ver            *Version
	port           ucv.Port
	items          []ucv.Item
	keys           []ucv.Key
	survey         bool
}

func newTxnBuilder(known *CfgVals, target *gpsprot.ConfigTarget, ver *Version, port ucv.Port, monEnabledGNSS gpsprot.GNSSSet) *txnBuilder {
	return &txnBuilder{
		known:          known,
		monEnabledGNSS: monEnabledGNSS,
		target:         target,
		ver:            ver,
		port:           port,
		items:          []ucv.Item{},
		keys:           []ucv.Key{},
	}
}

// build determines the transaction to achieve the specified target configuration.
// The known field contains what is known about the current configuration.
// If the transaction cannot be determined because some keys are missing, those keys
// are accumulated in the keys field. Typically this will be called twice:
// first with partial knowledge, then again after fetching the needed keys.
func (tb *txnBuilder) build() error {
	if tb.target.Get&^(gpsprot.PropIDTimePulseWidth|gpsprot.PropIDSignalsEnabled) != 0 {
		return errors.New("getting configuration properties with UBX-CFG-VALGET implemented only for time pulse width and signals enabled")
	}

	cp := &tb.target.Props
	tg := tb.timePulseBuild()

	err := tb.timeModeBuild()
	if err != nil {
		return err
	}

	if v, ok := cp.GetAntennaCableDelay(); ok {
		txnAddItem(tb, ucv.KTpAntCabledelay, int64(v))
	}
	if v, ok := cp.GetTimePulsePolarityRising(); ok {
		txnAddItem(tb, ucv.KTpPolTp1, v)
	}
	if v, ok := cp.GetTimeGNSS(); ok {
		tg = gnssToTimegridTp1(v)
		txnAddItem(tb, ucv.KTpTimegridTp1, tg)
		txnAddItem(tb, ucv.KNavspgUtcstandard, gnssToEnumNavspgUtcstandard(v))
		txnAddItem(tb, ucv.KRateTimeref, gnssToRateTimeref(v))
	}
	if v, ok := cp.GetStationary(); ok {
		dm := ucv.ENavspgDynmodelPort
		if v {
			dm = ucv.ENavspgDynmodelStat
		}
		txnAddItem(tb, ucv.KNavspgDynmodel, dm)
	}
	if true {
		// new code, under development
		err = tb.messagesBuild()
	} else {
		// old code
		err = tb.messagesBuildOld(tg)
	}
	if err != nil {
		return err
	}
	return nil
}

func (tb *txnBuilder) messagesBuildOld(tg ucv.EnumTpTimegridTp1) error {
	opts := &tb.target.Opts
	if opts.EnablesMsgs() {
		txnAddItem(tb, ucv.KRateMeas, 1000)
		txnAddItem(tb, ucv.KRateNav, 1)
	}
	if opts.NMEAMsg.IsSet() {
		v := opts.NMEAMsg.Get() != 0
		k := portOutprotNmeaKey(tb.port)
		if k != 0 {
			txnAddItem(tb, k, v)
		}
	}
	if opts.PVTMsg.Get()&gpsprot.PVTMsgTimePulse != 0 {
		// XXX this is not consistent with the legacy case
		// simpler just to enable NAV-TIMEGPS
		// the fractional TOW will not be the same but we don't use that
		txnAddItem(tb, timegridTp1ToMsgRateKey(tg).KeyU(tb.port), 1)
		txnAddItem(tb, ucv.KUbxTimTp.KeyU(tb.port), 1)
	}
	if opts.PVTMsg.Get()&gpsprot.PVTMsgLeapSecond != 0 {
		txnAddItem(tb, ucv.KUbxNavTimels.KeyU(tb.port), 1)
	}
	if opts.PVTMsg.Get()&gpsprot.PVTMsgSurvey != 0 {
		// XXX this isn't turning it off as it did before
		switch tb.ver.ProductCategory() {
		case "TIM":
			txnAddItem(tb, ucv.KUbxTimSvin.KeyU(tb.port), 1)
		case "HPG":
			txnAddItem(tb, ucv.KUbxNavSvin.KeyU(tb.port), 1)
		}
	}
	if opts.SatsMsg.IsSet() {
		rate := uint64(0)
		if opts.SatsMsg.Get()&gpsprot.SatsMsgSV != 0 {
			rate = 1
		}
		txnAddItem(tb, ucv.KUbxNavSat.KeyU(tb.port), rate)
	}
	return nil
}

func (tb *txnBuilder) messagesBuild() error {
	msgChanges := newMsgChanges()
	enabledGNSS := tb.monEnabledGNSS
	if enabledGNSS == 0 {
		if enabledSignals, ok := tb.known.getSignalsEnabled(); ok {
			enabledGNSS = enabledSignals.GNSSSet()
		}
	}
	err := msgChanges.options(&tb.target.Opts, tb.ver, enabledGNSS, tb.survey)
	if err != nil {
		return err
	}
	if msgChanges.usesRate() {
		txnAddItem(tb, ucv.KRateMeas, 1000)
		txnAddItem(tb, ucv.KRateNav, 1)
	}
	tb.items = append(tb.items, msgChanges.items(tb.port)...)
	return nil
}

func (known *CfgVals) BaudRate(target *gpsprot.ConfigTarget, port ucv.Port) []ucv.Item {
	items := []ucv.Item{}
	baudRate := target.Opts.BaudRate
	if baudRate != 0 {
		k := portBaudRateKey(port)
		if k != 0 {
			ucv.AddItem(&items, k, uint64(baudRate))
		}
	}
	return items
}

func (known *CfgVals) NavMsgAuth(props *gpsprot.ConfigProps) []ucv.Item {
	items := []ucv.Item{}
	nma, ok := props.GetNavMsgAuth()
	if ok {
		osnmaEnable := false
		if nma&gpsprot.NavMsgAuthOSNMA != 0 {
			osnmaEnable = true
		}
		ucv.AddItem(&items, ucv.KGalUseOsnma, osnmaEnable)
	}
	return items
}

func (tb *txnBuilder) timeModeBuild() error {
	switch tb.ver.ProductCategory() {
	case "FTS", "TIM", "HPG":
		// these products support time mode
	default:
		return nil
	}
	cp := &tb.target.Props
	if v, ok := cp.GetFixedPosECEF(); ok {
		err := addTmodeECEF(&tb.items, v)
		if err != nil {
			return err
		}
	}
	tmReq := gpsprot.TimeMode(0)
	tmReq, _ = cp.GetTimeMode()
	tmKnown := gpsprot.TimeMode(0)
	if tm, ok := cfgValGet(tb.known, ucv.KTmodeMode); ok {
		tmKnown = tmodeModeToTimeMode(tm)
	}
	opts := &tb.target.Opts
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
			txnAddKey(tb, ucv.KTmodeMode)
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
		tb.survey = true
		addSurveyItems(&tb.items, opts.Survey)
		return nil
	}

	if tmReq != gpsprot.TimeModeSurvey {
		txnAddItem(tb, ucv.KTmodeMode, timeModeToTmodeMode(tmReq))
		return nil
	}
	// Remaining possibility:
	// The user requested TimeMode=Survey in the ConfigProps,
	// but in ConfigOptions says not to start a Survey when the TimeMode is Survey.
	// This is actually OK, if the receiver is already in Survey mode.
	return nil
}

func addSurveyItems(items *[]ucv.Item, opts gpsprot.Survey) {
	ucv.AddItem(items, ucv.KTmodeMode, ucv.ETmodeModeSurveyIn)
	ucv.AddItem(items, ucv.KTmodeSvinMinDur, uint64(opts.MinDur.Round(time.Second)/time.Second))
	var mm10 int64
	mm10, _ = divModRound(int64(opts.AccLimit), int64(gpsprot.Millimeter/10))
	ucv.AddItem(items, ucv.KTmodeSvinAccLimit, uint64(mm10))
}

func (raw *CfgVals) getMode() (gpsprot.Mode, bool) {
	tmc := tmodeConfig{}
	if tmc.fromCfgVals(raw, tmodeInfoRelevant) {
		return tmc.getMode(), true
	}
	return gpsprot.Mode{}, false
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

// timePulseBuild compiles the parts of the configuration related to the time pulse.
// If it infers the GNSS to which the pulse is aligned, it returns that.
func (tb *txnBuilder) timePulseBuild() ucv.EnumTpTimegridTp1 {
	tg := tb.timePulseWrite()
	tb.timePulseRead()
	return tg
}

func (tb *txnBuilder) timePulseRead() {
	// XXX implement for other keys
	if tb.target.Get&gpsprot.PropIDTimePulseWidth == 0 {
		return
	}
	for _, item := range tb.items {
		if item.Key == ucv.KTpLenLockTp1.Key() || (item.Key == ucv.KTpLenTp1.Key() && item.Value != 0) {
			return
		}
	}
	for _, k := range cfgValKeysByProp[gpsprot.PropIDTimePulseWidth] {
		if !tb.known.Contains(k.Key()) {
			tb.keys = append(tb.keys, k.Key())
		}
	}
	slices.Sort(tb.keys)
	tb.keys = slices.Compact(tb.keys)
}

func (tb *txnBuilder) timePulseWrite() ucv.EnumTpTimegridTp1 {
	tg := ucv.ETpTimegridTp1Utc
	cp := &tb.target.Props
	period, havePeriod := cp.GetTimePulsePeriod()
	width, haveWidth := cp.GetTimePulseWidth()
	align, haveAlign := cp.GetTimePulseAlignToGNSS()
	onlyWhenLocked, haveOnlyWhenLocked := cp.GetTimePulseOnlyWhenLocked()
	useLock := align
	if haveAlign {
		txnAddItem(tb, ucv.KTpUseLockedTp1, align)
		txnAddItem(tb, ucv.KTpAlignToTowTp1, align)
		txnAddItem(tb, ucv.KTpSyncGnssTp1, align)
		if _, ok := cp.GetTimeGNSS(); !ok {
			tg = tb.inferTimegridTp1()
		}
	} else {
		if havePeriod || haveWidth {
			if v, ok := cfgValGet(tb.known, ucv.KTpUseLockedTp1); ok {
				useLock = v
			} else {
				// need more info
				txnAddKey(tb, ucv.KTpUseLockedTp1)
			}
		}
	}
	if havePeriod {
		us := uint64(period.Round(time.Microsecond) / time.Microsecond)
		// always set unlocked period
		txnAddItem(tb, ucv.KTpPeriodTp1, us)
		if useLock {
			txnAddItem(tb, ucv.KTpPeriodLockTp1, us)
		}
		txnAddItem(tb, ucv.KTpPulseDef, ucv.ETpPulseDefPeriod)
	}
	if haveWidth {
		us := uint64(width.Round(time.Microsecond) / time.Microsecond)
		if useLock {
			txnAddItem(tb, ucv.KTpLenLockTp1, us)
			if haveOnlyWhenLocked {
				usNoLock := us
				if onlyWhenLocked {
					usNoLock = 0
				}
				txnAddItem(tb, ucv.KTpLenTp1, usNoLock)
			} else {
				tb.inferTpLenTp1(us)
			}
		} else {
			txnAddItem(tb, ucv.KTpLenTp1, us)
		}
		txnAddItem(tb, ucv.KTpTp1Ena, us != 0)
		txnAddItem(tb, ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength)
	}
	return tg
}

func (tb *txnBuilder) inferTpLenTp1(lenLock uint64) {
	if def, ok := cfgValGet(tb.known, ucv.KTpPulseLengthDef); ok {
		if def == ucv.ETpPulseLengthDefLength {
			// we can just leave as is
			return
		}
		if duty, ok := cfgValGet(tb.known, ucv.KTpDutyTp1); ok {
			l := lenLock
			if duty == 0 {
				l = 0
			}
			// our model is that the unlocked pulse length is either zero or the same as the locked pulse length
			txnAddItem(tb, ucv.KTpLenTp1, l)
			return
		}
	} else {
		txnAddKey(tb, ucv.KTpPulseLengthDef)
	}
	txnAddKey(tb, ucv.KTpDutyTp1)
}

func (tb *txnBuilder) inferTimegridTp1() ucv.EnumTpTimegridTp1 {
	if tg, ok := txnGetOrAdd(tb, ucv.KTpTimegridTp1); ok && tg != ucv.ETpTimegridTp1Utc {
		return tg
	}
	if u, ok := txnGetOrAdd(tb, ucv.KNavspgUtcstandard); ok {
		tg := navspgUtcstandardToTimegridTp1(u)
		if tg != ucv.ETpTimegridTp1Utc {
			txnAddItem(tb, ucv.KTpTimegridTp1, tg)
			return tg
		}
	}
	if false {
		// This isn't a good idea because the default for this is GPS.
		if r, ok := txnGetOrAdd(tb, ucv.KRateTimeref); ok {
			tg := rateTimeRefToTimegridTp1(r)
			if tg != ucv.ETpTimegridTp1Utc {
				txnAddItem(tb, ucv.KTpTimegridTp1, tg)
				return tg
			}
		}
	}

	// Fall back to first of GPS, Galileo, BeiDou, GLONASS that is enabled (in that order)
	sigEna := []ucv.KeyL{ucv.KSignalGpsEna, ucv.KSignalGalEna, ucv.KSignalBdsEna, ucv.KSignalGloEna}
	sigTg := []ucv.EnumTpTimegridTp1{ucv.ETpTimegridTp1Gps, ucv.ETpTimegridTp1Gal, ucv.ETpTimegridTp1Bds, ucv.ETpTimegridTp1Glo}
	for i, sig := range sigEna {
		tg := sigTg[i]
		// We need to check that the signal is supported by the receiver, before we use valget to see if it is enabled.
		if gnss := timegridTp1ToGNSS(tg); gnss != 0 && tb.ver.GNSS.Contains(gnss) {
			if ena, ok := txnGetOrAdd(tb, sig); ok && ena {
				txnAddItem(tb, ucv.KTpTimegridTp1, sigTg[i])
				return sigTg[i]
			}
		}
	}
	return ucv.ETpTimegridTp1Utc
}

func cfgValGet[T comparable](vals *CfgVals, k ucv.TypedKey[T]) (t T, ok bool) {
	return ucv.MapGet(vals.Map, k)
}

func txnGetOrAdd[T comparable](tb *txnBuilder, k ucv.TypedKey[T]) (t T, ok bool) {
	t, ok = cfgValGet(tb.known, k)
	if !ok {
		txnAddKey(tb, k)
	}
	return
}

func txnAddItem[T comparable](tb *txnBuilder, key ucv.TypedKey[T], value T) {
	ucv.AddItem(&tb.items, key, value)
}

func txnAddKey[T comparable](tb *txnBuilder, key ucv.TypedKey[T]) {
	ucv.AddKey(&tb.keys, key)
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

func timegridTp1ToGNSS(tg ucv.EnumTpTimegridTp1) gpsprot.GNSS {
	switch tg {
	case ucv.ETpTimegridTp1Gps:
		return gpsprot.GPS
	case ucv.ETpTimegridTp1Gal:
		return gpsprot.GAL
	case ucv.ETpTimegridTp1Glo:
		return gpsprot.GLO
	case ucv.ETpTimegridTp1Bds:
		return gpsprot.BDS
	case ucv.ETpTimegridTp1Navic:
		return gpsprot.NAVIC
	}
	return 0
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

func portOutprotRtcm3xKey(port ucv.Port) ucv.KeyL {
	switch port {
	case ucv.UART1:
		return ucv.KUart1outprotRtcm3x
	case ucv.UART2:
		return ucv.KUart2outprotRtcm3x
	case ucv.USB:
		return ucv.KUsboutprotRtcm3x
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

func (known *CfgVals) EnableSignals(enabled gpsprot.SignalSet, ver *Version) (gpsprot.SignalSet, []ucv.Item) {
	supported := known.signalsSupported(ver)
	items := []ucv.Item{}
	for g := gpsprot.GNSS(1); g <= gpsprot.GNSSLast; g++ {
		gss := gpsprot.BandAll.SignalSet(g) & supported
		if gss == 0 {
			// this GNSS is not supported by the receiver
			continue
		}
		gk := gnssKeyMap[g]
		gEnabled := gss & enabled
		if gEnabled == 0 {
			// there are no signal enabled for this GNSS
			// and this is a GNSS supported by the receiver
			// so just disable this GNSS
			ucv.AddItem(&items, gk, false)

		} else {
			// enable the GNSS
			ucv.AddItem(&items, gk, true)
			// set the supported signals in that GNSS to enabled or disabled
			for sig := range gss.Signals() {
				ucv.AddItem(&items, signalKeyMap[sig], enabled.Contains(sig))
			}
		}
	}
	// If we are enabling GPS L5, and the special key to override the health of GPS L5 is known, then enable it.
	// Otherwise, L5 wil be ignored if GPS L5 is not yet fully operational.
	if _, ok := ucv.MapGet(known.Map, ucv.KGpsL5HealthOverride); ok && (supported & enabled).Contains(gpsprot.SigGPSL5) {
		ucv.AddItem(&items, ucv.KGpsL5HealthOverride, true)
	}
	return supported & enabled, items
}

func (raw *CfgVals) signalsSupported(ver *Version) gpsprot.SignalSet {
	supported := gpsprot.SignalSet(0)
	for k, sig := range keySignalMap {
		_, ok := ucv.MapGet(raw.Map, k)
		if ok {
			supported |= gpsprot.SignalSetOf(sig)
		}
	}
	if ver.Mod == "NEO-F10T" {
		// The NEO-F10T does not support BDS B1I,
		// despite the fact that it has a key for it. Argh!
		// This must be a bug in the firmware.
		supported &^= gpsprot.SignalSetOf(gpsprot.SigBDSB1I)
	}
	return supported
}

func (raw *CfgVals) getSignalsEnabled() (gpsprot.SignalSet, bool) {
	found := false
	enabled1 := gpsprot.SignalSet(0)
	for k, sig := range keySignalMap {
		ena, ok := ucv.MapGet(raw.Map, k)
		if ok {
			found = true
			if ena {
				enabled1 |= gpsprot.SignalSetOf(sig)

			}
		}
	}
	enabled2 := gpsprot.SignalSet(0)
	for k, g := range keyGNSSMap {
		ena, ok := ucv.MapGet(raw.Map, k)
		if ok {
			found = true
			if ena {
				enabled2 |= gpsprot.BandAll.SignalSet(g)
			}
		}
	}
	return enabled1 & enabled2, found
}

var keySignalMap = map[ucv.KeyL]gpsprot.Signal{
	ucv.KSignalGpsL1caEna:  gpsprot.SigGPSL1CA,  // GPS L1 C/A
	ucv.KSignalGpsL2cEna:   gpsprot.SigGPSL2C,   // GPS L2C
	ucv.KSignalGpsL5Ena:    gpsprot.SigGPSL5,    // GPS L5
	ucv.KSignalGloL1Ena:    gpsprot.SigGLOL1,    // GLONASS L1
	ucv.KSignalGloL2Ena:    gpsprot.SigGLOL2,    // GLONASS L2
	ucv.KSignalGalE1Ena:    gpsprot.SigGALE1,    // Galileo E1
	ucv.KSignalGalE5aEna:   gpsprot.SigGALE5a,   // Galileo E5a
	ucv.KSignalGalE5bEna:   gpsprot.SigGALE5b,   // Galileo E5b
	ucv.KSignalGalE6Ena:    gpsprot.SigGALE6,    // Galileo E6
	ucv.KSignalBdsB1Ena:    gpsprot.SigBDSB1I,   // BeiDou B1I
	ucv.KSignalBdsB1cEna:   gpsprot.SigBDSB1C,   // BeiDou B1C
	ucv.KSignalBdsB2Ena:    gpsprot.SigBDSB2I,   // BeiDou B2I
	ucv.KSignalBdsB2aEna:   gpsprot.SigBDSB2a,   // BeiDou B2a
	ucv.KSignalBdsB3Ena:    gpsprot.SigBDSB3I,   // BeiDou B3I
	ucv.KSignalQzssL1caEna: gpsprot.SigQZSSL1CA, // QZSS L1 C/A
	ucv.KSignalQzssL1sEna:  gpsprot.SigQZSSL1S,  // QZSS L1S
	ucv.KSignalQzssL2cEna:  gpsprot.SigQZSSL2C,  // QZSS L2C
	ucv.KSignalQzssL5Ena:   gpsprot.SigQZSSL5,   // QZSS L5
	ucv.KSignalNavicL5Ena:  gpsprot.SigNAVICL5,  // NavIC L5
	ucv.KSignalSbasL1caEna: gpsprot.SigSBASL1CA, // SBAS L1 C/A
}

var signalKeyMap map[gpsprot.Signal]ucv.KeyL

var keyGNSSMap = map[ucv.KeyL]gpsprot.GNSS{
	ucv.KSignalGpsEna:   gpsprot.GPS,   // GPS
	ucv.KSignalGalEna:   gpsprot.GAL,   // Galileo
	ucv.KSignalBdsEna:   gpsprot.BDS,   // BeiDou
	ucv.KSignalGloEna:   gpsprot.GLO,   // GLONASS
	ucv.KSignalNavicEna: gpsprot.NAVIC, // NavIC
	ucv.KSignalQzssEna:  gpsprot.QZSS,  // QZSS
	ucv.KSignalSbasEna:  gpsprot.SBAS,  // SBAS
}

var gnssKeyMap map[gpsprot.GNSS]ucv.KeyL

func init() {
	signalKeyMap = make(map[gpsprot.Signal]ucv.KeyL, len(keySignalMap))
	for k, sig := range keySignalMap {
		signalKeyMap[sig] = k
	}
	gnssKeyMap = make(map[gpsprot.GNSS]ucv.KeyL, len(keyGNSSMap))
	for k, gnss := range keyGNSSMap {
		gnssKeyMap[gnss] = k
	}
}
