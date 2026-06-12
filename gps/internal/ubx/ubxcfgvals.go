package ubx

import (
	"errors"
	"slices"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

type CfgVals struct {
	ucv.Map
}

var AllKeys = []ucv.AnyTypedKey{
	ucv.KNavspgDynmodel,
	ucv.KNavspgInfilMinelev,
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
	ucv.KTpDutyLockTp1,
	ucv.KTpFreqTp1,
	ucv.KTpFreqLockTp1,
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
	gpsprot.PropIDMinElevation: {
		ucv.KNavspgInfilMinelev,
	},
	gpsprot.PropIDTimeGNSS: {
		ucv.KTpTimegridTp1,
	},
	gpsprot.PropIDAntennaCableDelay: {
		ucv.KTpAntCabledelay,
	},
	gpsprot.PropIDRTCMBaseID: {
		ucv.KRtcmDf003Out,
	},
	gpsprot.PropIDTimePulsePolarityRising: {
		ucv.KTpPolTp1,
	},
	gpsprot.PropIDTimePulse: {
		ucv.KTpAlignToTowTp1,
		ucv.KTpAntCabledelay,
		ucv.KTpDutyLockTp1,
		ucv.KTpDutyTp1,
		ucv.KTpFreqLockTp1,
		ucv.KTpFreqTp1,
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

func (raw *CfgVals) Cook(ver *Version, cp *gpsprot.ConfigProps, port *ubxbin.PortID) {
	if ver.tpIndex() == 1 {
		raw = &CfgVals{ucv.RemapMap(raw.Map, ucv.KeyRemap(ucv.TPKeyPairs, 1, 0))}
	}
	if v, ok := raw.getSignalsEnabled(); ok {
		cp.SetSignalsEnabled(v)
	}
	if v, ok := raw.getMode(); ok {
		cp.SetMode(v)
	}
	if v, ok := raw.getTimeGNSS(); ok {
		cp.SetTimeGNSS(v)
	}
	if v, ok := raw.getTimePulse(); ok {
		cp.SetTimePulse(v)
	}
	if v, ok := cfgValGet(raw, ucv.KTpAntCabledelay); ok {
		cp.SetAntennaCableDelay(time.Duration(v))
	}
	if v, ok := cfgValGet(raw, ucv.KNavspgInfilMinelev); ok {
		cp.SetMinElevation(gpsprot.Angle(v) * gpsprot.Degrees)
	}
	if v, ok := cfgValGet(raw, ucv.KRtcmDf003Out); ok {
		cp.SetRTCMBaseID(uint16(v))
	}
	if v, ok := raw.getBaudRate(port); ok {
		cp.SetBaudRate(v)
	}
}

// Transaction determines the transaction to achieve the specified target.
// The known argument gives what is known about the current configuration.
// The returned items specifies configuration database changes that are needed.
// If the specified changes cannot be determined because the value of some keys is not known, those keys are returned in the ucv.Key slice.
// Typically this function will get called twice.
// The first time, known will be empty, and some more keys will be needed.
// The caller will then fetch the additional keys, add them to known and call again.
//
// portOK is false when the active receiver port could not be
// discovered. In that case Transaction returns an error if the
// transaction would have included any port-specific item (per-port
// message rates or protocol enables); other items are emitted
// normally. The read path (addGetKeys) silently omits the UART
// baud-rate key when portOK is false.
func (known *CfgVals) Transaction(target *gpsprot.ConfigTarget, ver *Version, port ucv.Port, portOK bool, monEnabledGNSS gpsprot.GNSSSet) ([]ucv.Item, []ucv.Key, error) {
	var tp1to2 map[ucv.Key]ucv.Key
	if ver.tpIndex() == 1 {
		tp1to2 = ucv.KeyRemap(ucv.TPKeyPairs, 0, 1)
		known = &CfgVals{ucv.RemapMap(known.Map, ucv.KeyRemap(ucv.TPKeyPairs, 1, 0))}
	}
	tb := newTxnBuilder(known, target, ver, port, portOK, monEnabledGNSS)
	err := tb.build()
	if err != nil {
		return nil, nil, err
	}
	keys := known.addGetKeys(target.Get, ver, port, portOK, tb.keys)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	if tp1to2 != nil {
		return ucv.RemapItems(tb.items, tp1to2), ucv.RemapKeys(keys, tp1to2), nil
	}
	return tb.items, keys, nil
}

// addGetKeys adds the keys that are needed to get the specified properties.
// This doesn't handle the SignalsEnabled or NMAMsgAuth properties, which are handled separately.
//
// For PropIDBaudRate: if the active port is a UART, add the matching
// KUart{N}Baudrate key; for USB / I2C / SPI no key is needed because
// CfgVals.getBaudRate reports those as zero. When portOK is false,
// no baud-rate key is added; the result then omits baudRate rather
// than reporting a guessed speed.
func (known *CfgVals) addGetKeys(ids gpsprot.PropIDs, ver *Version, port ucv.Port, portOK bool, keys []ucv.Key) []ucv.Key {
	tks := []ucv.AnyTypedKey{}
	if ids&gpsprot.PropIDBaudRate != 0 && portOK {
		if k := portBaudRateKey(port, portOK); k != 0 {
			if !known.Contains(k.Key()) {
				keys = append(keys, k.Key())
			}
		}
	}
	switch ids & gpsprot.PropIDTimePulse {
	// we handle a few of these properties individually
	case gpsprot.PropIDAntennaCableDelay, 0:
		// we will handle these below uniformly
	default:
		// handle these as a group
		tks = append(tks, cfgValKeysByProp[gpsprot.PropIDTimePulse]...)
		ids &^= gpsprot.PropIDTimePulse
	}
	for id, tk := range cfgValKeysByProp {
		if id&ids == id { // be careful not to match PropIDTimePulse
			if id == gpsprot.PropIDRTCMBaseID && !ver.rtcmSupport().df003Out {
				continue
			}
			tks = append(tks, tk...)
		}
	}
	if ids&gpsprot.PropIDMode != 0 {
		if ver.tmodeLevel() == 0 {
			tks = append(tks, ucv.KNavspgDynmodel)
		} else {
			tks = append(tks, tmodeRequiredKeys(tmodeInfoAll)...)
		}
		ids &^= gpsprot.PropIDMode
	}
	for _, tk := range tks {
		k := tk.Key()
		if !known.Contains(k) {
			keys = append(keys, k)
		}
	}
	return keys
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
	portOK         bool
	items          []ucv.Item
	keys           []ucv.Key
	survey         bool
}

func newTxnBuilder(known *CfgVals, target *gpsprot.ConfigTarget, ver *Version, port ucv.Port, portOK bool, monEnabledGNSS gpsprot.GNSSSet) *txnBuilder {
	return &txnBuilder{
		known:          known,
		monEnabledGNSS: monEnabledGNSS,
		target:         target,
		ver:            ver,
		port:           port,
		portOK:         portOK,
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
	cp := &tb.target.Props
	tg := tb.timePulseBuild()

	err := tb.modeBuild()
	if err != nil {
		return err
	}

	if v, ok := cp.GetAntennaCableDelay(); ok {
		n, err := antCableDelayNanos(v)
		if err != nil {
			return err
		}
		txnAddItem(tb, ucv.KTpAntCabledelay, n)
	}
	if v, ok := cp.GetMinElevation(); ok {
		if deg, ok := angleToInt8Degrees(v); ok {
			txnAddItem(tb, ucv.KNavspgInfilMinelev, int64(deg))
		}
	}
	if v, ok := cp.GetRTCMBaseID(); ok && tb.ver.rtcmSupport().df003Out {
		txnAddItem(tb, ucv.KRtcmDf003Out, uint64(v))
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
		if opts.SatsMsg.Get()&gpsprot.SatsMsgSat != 0 {
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
	if enabledGNSS == 0 {
		enabledGNSS = tb.ver.GNSS
	}
	err := msgChanges.options(&tb.target.Opts, tb.ver, enabledGNSS, tb.survey)
	if err != nil {
		return err
	}
	if !tb.portOK && (msgChanges.usesRate() || msgChanges.hasPortItems()) {
		return errors.New("message configuration requested but receiver port unknown")
	}
	if msgChanges.usesRate() {
		txnAddItem(tb, ucv.KRateMeas, 1000)
		txnAddItem(tb, ucv.KRateNav, 1)
	}
	tb.items = append(tb.items, msgChanges.items(tb.port, tb.portOK)...)
	return nil
}

// BaudRate returns the items required to set the target baud rate on
// port. Callers must only invoke this with a known port (see
// valBaudRate); for non-UART ports the result is empty.
func (known *CfgVals) BaudRate(target *gpsprot.ConfigTarget, port ucv.Port) []ucv.Item {
	items := []ucv.Item{}
	baudRate, ok := target.Props.GetBaudRate()
	if ok && baudRate != 0 {
		k := portBaudRateKey(port, true)
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

func (tb *txnBuilder) modeBuild() error {
	switch tb.ver.ProductCategory() {
	case "FTS", "TIM", "HPG":
		return tb.timeModeBuild()
	default:
		tb.dynModelBuild()
		return nil
	}
}

func (tb *txnBuilder) dynModelBuild() {
	static := dynModelStatic(tb.target)
	if static == nil {
		return
	}
	dm := ucv.ENavspgDynmodelPort
	if *static {
		dm = ucv.ENavspgDynmodelStat
	}
	txnAddItem(tb, ucv.KNavspgDynmodel, dm)
}

func (tb *txnBuilder) timeModeBuild() error {
	// Determine whether we have the needed keys
	info := tmodeRequiredInfo(tb.target, resurveyChange)
	requiredKeys := tmodeRequiredKeys(info)
	for _, key := range requiredKeys {
		if !tb.known.Contains(key.Key()) {
			tb.keys = append(tb.keys, key.Key())
		}
	}

	// If we're missing keys, return early; will be called again after fetching
	if len(tb.keys) > 0 {
		return nil
	}

	// Convert from keys to intermediate form (tmodeConfig)
	var cur tmodeConfig
	if !cur.fromCfgVals(tb.known, info) {
		return errors.New("internal error: polling tmode config did not work correctly")
	}

	// Determine the time mode in intermediate form
	// Second timeModeConfig will always be nil, because we are using resurveyChange
	tmc, _, err := createTmodeConfigs(tb.target, &cur, resurveyChange)
	if err != nil {
		return err
	}

	// Convert from intermediate form to items
	if tmc != nil {
		tmc.addItems(&tb.items, false)
		// Need to set survey flag so that survey messages get enabled
		if tmc.mode == tmodeSurveyIn {
			tb.survey = true
		}
	}

	return nil
}

func (raw *CfgVals) getMode() (gpsprot.Mode, bool) {
	tmc := tmodeConfig{}
	if tmc.fromCfgVals(raw, tmodeInfoRelevant) {
		return tmc.getMode(), true
	}
	mode := gpsprot.Mode{}
	if v, ok := cfgValGet(raw, ucv.KNavspgDynmodel); ok {
		mode.Static = v == ucv.ENavspgDynmodelStat
		return mode, true
	}
	return mode, false
}

// getTimePulse reads raw u-blox CFG-TP keys and returns the abstract TimePulse.
// Returns false if any required key is missing. This is the inverse of timePulseBuild.
func (raw *CfgVals) getTimePulse() (gpsprot.TimePulse, bool) {
	var tpZero gpsprot.TimePulse
	locked, ok := cfgValGet(raw, ucv.KTpUseLockedTp1)
	if !ok {
		return tpZero, false
	}
	var tp gpsprot.TimePulse
	// AlignToGNSS: only meaningful when USE_LOCKED is set
	if locked {
		v1, ok1 := cfgValGet(raw, ucv.KTpAlignToTowTp1)
		v2, ok2 := cfgValGet(raw, ucv.KTpSyncGnssTp1)
		if !ok1 || !ok2 {
			return tpZero, false
		}
		tp.AlignToGNSS = v1 && v2
	}
	// OnlyWhenLocked
	if !locked {
		tp.OnlyWhenLocked = false
	} else {
		def, ok := cfgValGet(raw, ucv.KTpPulseLengthDef)
		if !ok {
			return tpZero, false
		}
		switch def {
		case ucv.ETpPulseLengthDefLength:
			v, ok := cfgValGet(raw, ucv.KTpLenTp1)
			if !ok {
				return tpZero, false
			}
			tp.OnlyWhenLocked = v == 0
		case ucv.ETpPulseLengthDefRatio:
			v, ok := cfgValGet(raw, ucv.KTpDutyTp1)
			if !ok {
				return tpZero, false
			}
			tp.OnlyWhenLocked = v == 0
		}
	}
	// Period
	def, ok := cfgValGet(raw, ucv.KTpPulseDef)
	if !ok {
		return tpZero, false
	}
	switch def {
	case ucv.ETpPulseDefPeriod:
		var v uint64
		if locked {
			v, ok = cfgValGet(raw, ucv.KTpPeriodLockTp1)
		} else {
			v, ok = cfgValGet(raw, ucv.KTpPeriodTp1)
		}
		if !ok {
			return tpZero, false
		}
		tp.Period = time.Duration(v) * time.Microsecond
	case ucv.ETpPulseDefFreq:
		var v uint64
		if locked {
			v, ok = cfgValGet(raw, ucv.KTpFreqLockTp1)
		} else {
			v, ok = cfgValGet(raw, ucv.KTpFreqTp1)
		}
		if !ok || v == 0 {
			return tpZero, false
		}
		tp.Period = time.Duration(1e6/v) * time.Microsecond
	}
	// Width
	ena, ok := cfgValGet(raw, ucv.KTpTp1Ena)
	if !ok {
		return tpZero, false
	}
	if !ena {
		tp.Width = 0
	} else {
		ldef, ok := cfgValGet(raw, ucv.KTpPulseLengthDef)
		if !ok {
			return tpZero, false
		}
		switch ldef {
		case ucv.ETpPulseLengthDefLength:
			var v uint64
			if locked {
				v, ok = cfgValGet(raw, ucv.KTpLenLockTp1)
			} else {
				v, ok = cfgValGet(raw, ucv.KTpLenTp1)
			}
			if !ok {
				return tpZero, false
			}
			tp.Width = time.Duration(v) * time.Microsecond
		case ucv.ETpPulseLengthDefRatio:
			var duty float64
			if locked {
				duty, ok = cfgValGet(raw, ucv.KTpDutyLockTp1)
			} else {
				duty, ok = cfgValGet(raw, ucv.KTpDutyTp1)
			}
			if !ok {
				return tpZero, false
			}
			tp.Width = time.Duration(float64(tp.Period) * duty / 100)
		}
	}
	// Polarity
	pol, ok := cfgValGet(raw, ucv.KTpPolTp1)
	if !ok {
		return tpZero, false
	}
	tp.PolarityRising = pol
	return tp, true
}

func (raw *CfgVals) getTimeGNSS() (gpsprot.GNSS, bool) {
	if tg, ok := cfgValGet(raw, ucv.KTpTimegridTp1); ok {
		if g := timegridTp1ToGNSS(tg); g != 0 {
			return g, true
		}
	}
	return 0, false
}

// getBaudRate returns the baud rate for the active port. USB / I2C / SPI
// return (0, true) ("not applicable"). UART returns the value if it is
// in the val map (post-write); otherwise the result is unset since we
// don't poll KUart{N}Baudrate.
func (raw *CfgVals) getBaudRate(port *ubxbin.PortID) (uint32, bool) {
	if port == nil {
		return 0, false
	}
	k := portBaudRateKey(ucv.Port(*port), true)
	if k == 0 {
		return 0, true
	}
	if v, ok := cfgValGet(raw, k); ok {
		return uint32(v), true
	}
	return 0, false
}

// timePulseBuild compiles the parts of the configuration related to the time pulse.
// If it infers the GNSS to which the pulse is aligned, it returns that.
func (tb *txnBuilder) timePulseBuild() ucv.EnumTpTimegridTp1 {
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
	case ucv.I2C:
		return ucv.KI2coutprotNmea
	case ucv.UART1:
		return ucv.KUart1outprotNmea
	case ucv.UART2:
		return ucv.KUart2outprotNmea
	case ucv.USB:
		return ucv.KUsboutprotNmea
	case ucv.SPI:
		return ucv.KSpioutprotNmea
	}
	return 0
}

func portOutprotRtcm3xKey(port ucv.Port) ucv.KeyL {
	switch port {
	case ucv.I2C:
		return ucv.KI2coutprotRtcm3x
	case ucv.UART1:
		return ucv.KUart1outprotRtcm3x
	case ucv.UART2:
		return ucv.KUart2outprotRtcm3x
	case ucv.USB:
		return ucv.KUsboutprotRtcm3x
	case ucv.SPI:
		return ucv.KSpioutprotRtcm3x
	}
	return 0
}

// portBaudRateKey returns the val key for the UART baud rate of port,
// or 0 if port is not a UART or portOK is false. !portOK and
// non-UART collapse to the same "no key" result; callers that need
// to distinguish them must check portOK separately.
func portBaudRateKey(port ucv.Port, portOK bool) ucv.KeyU {
	if !portOK {
		return 0
	}
	switch port {
	case ucv.UART1:
		return ucv.KUart1Baudrate
	case ucv.UART2:
		return ucv.KUart2Baudrate
	}
	return 0
}

func resolveSignalConstraints(ver *Version, enabled, supported gpsprot.SignalSet) gpsprot.SignalSet {
	if sig, ok := ver.singleBDSL1Signal(); ok {
		b1 := gpsprot.SignalSetOf(gpsprot.SigBDSB1I, gpsprot.SigBDSB1C)
		if enabled&supported&b1 != 0 {
			enabled &^= b1
			enabled |= gpsprot.SignalSetOf(sig)
		}
	}
	return enabled
}

// EnableSignals returns the items needed to enable the given signals,
// constrained by the supported signal set.
func (known *CfgVals) EnableSignals(enabled, supported gpsprot.SignalSet) (gpsprot.SignalSet, []ucv.Item) {
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
	if ver.isModel("NEO-F10T") {
		// The NEO-F10T does not support BDS B1I,
		// despite the fact that it has a key for it. Argh!
		// This must be a bug in the firmware.
		supported &^= gpsprot.SignalSetOf(gpsprot.SigBDSB1I)
	}
	return supported
}

// monGnss1Signals returns the supported signals for the active signal plan.
func monGnss1Signals(m *ubxbin.MonGnss1) gpsprot.SignalSet {
	id := m.ActivePlanID()
	for i := range m.Plans {
		if m.Plans[i].ID == id {
			return monGnssPlanSignals(&m.Plans[i])
		}
	}
	return 0
}

// monGnssPlanSignals converts a MON-GNSS v1 signal plan to a gpsprot.SignalSet.
func monGnssPlanSignals(p *ubxbin.MonGnssPlan) gpsprot.SignalSet {
	var ss gpsprot.SignalSet
	addIf := func(sup bool, sig gpsprot.Signal) {
		if sup {
			ss |= gpsprot.SignalSetOf(sig)
		}
	}
	addIf(p.GpsSup&ubxbin.MonGnssGpsL1CA != 0, gpsprot.SigGPSL1CA)
	addIf(p.GpsSup&ubxbin.MonGnssGpsL1C != 0, gpsprot.SigGPSL1C)
	addIf(p.GpsSup&ubxbin.MonGnssGpsL2C != 0, gpsprot.SigGPSL2C)
	addIf(p.GpsSup&ubxbin.MonGnssGpsL5 != 0, gpsprot.SigGPSL5)
	addIf(p.GalSup&ubxbin.MonGnssGalE1 != 0, gpsprot.SigGALE1)
	addIf(p.GalSup&ubxbin.MonGnssGalE5a != 0, gpsprot.SigGALE5a)
	addIf(p.GalSup&ubxbin.MonGnssGalE5b != 0, gpsprot.SigGALE5b)
	addIf(p.GalSup&ubxbin.MonGnssGalE6 != 0, gpsprot.SigGALE6)
	addIf(p.BdsSup&ubxbin.MonGnssBdsB1I != 0, gpsprot.SigBDSB1I)
	addIf(p.BdsSup&ubxbin.MonGnssBdsB1C != 0, gpsprot.SigBDSB1C)
	addIf(p.BdsSup&ubxbin.MonGnssBdsB2I != 0, gpsprot.SigBDSB2I)
	addIf(p.BdsSup&ubxbin.MonGnssBdsB2a != 0, gpsprot.SigBDSB2a)
	addIf(p.BdsSup&ubxbin.MonGnssBdsB3I != 0, gpsprot.SigBDSB3I)
	addIf(p.GloSup&ubxbin.MonGnssGloL1OF != 0, gpsprot.SigGLOL1)
	addIf(p.GloSup&ubxbin.MonGnssGloL2OF != 0, gpsprot.SigGLOL2)
	addIf(p.SbasSup&ubxbin.MonGnssSbasL1CA != 0, gpsprot.SigSBASL1CA)
	// L1C/B is an updated L1C/A; CFG-SIGNAL doesn't distinguish them.
	addIf(p.QzssSup&(ubxbin.MonGnssQzssL1CA|ubxbin.MonGnssQzssL1CB) != 0, gpsprot.SigQZSSL1CA)
	addIf(p.QzssSup&ubxbin.MonGnssQzssL1C != 0, gpsprot.SigQZSSL1C)
	addIf(p.QzssSup&ubxbin.MonGnssQzssL1S != 0, gpsprot.SigQZSSL1S)
	addIf(p.QzssSup&ubxbin.MonGnssQzssL2C != 0, gpsprot.SigQZSSL2C)
	addIf(p.QzssSup&ubxbin.MonGnssQzssL5 != 0, gpsprot.SigQZSSL5)
	addIf(p.NavicSup&ubxbin.MonGnssNavicL5 != 0, gpsprot.SigNAVICL5)
	return ss
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
