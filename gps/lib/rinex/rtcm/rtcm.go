// Package rtcm converts RTCM MSM7 observation messages to RINEX records.
package rtcm

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
)

const (
	speedOfLight       = 299792458.0
	rangeMS            = speedOfLight * 0.001
	p2_10              = 9.765625e-4
	p2_29              = 1.862645149230957e-9
	p2_31              = 4.656612873077393e-10
	rinexTicksPerMS    = 10000
	secondMS           = 1000
	hourMS             = 60 * 60 * secondMS
	dayMS              = 24 * hourMS
	weekMS             = 7 * dayMS
	halfWeekMS         = weekMS / 2
	maxInterval        = weekMS * 1000 * 1000
	bdtOffsetMS        = 14 * secondMS
	glonassUTCOffsetMS = 3 * hourMS
	defaultGPSUTCMS    = 18 * secondMS
)

// TimeInterval constrains RTCM epoch resolution to a half-open UTC interval.
// The zero value means no constraint is supplied.
type TimeInterval struct {
	Start    time.Time
	Duration time.Duration
}

// IsZero reports whether week is the zero value, meaning no constraint is
// supplied. Any other value is a constraint and is subject to validation.
func (week TimeInterval) IsZero() bool {
	return week.Start.IsZero() && week.Duration == 0
}

// Options controls RTCM to RINEX conversion.
type Options struct {
	// UseSpecPhaseRangeRateSign interprets MSM PhaseRangeRate with strict
	// RTCM sign. RTCM defines PhaseRange with the same sign as pseudorange,
	// and PhaseRangeRate as d(PhaseRange)/dt, so RINEX Doppler is
	// -PRR/wavelength. The default keeps the receiver-compatible polarity
	// observed in UM980 and ZED-F9P MSM streams, where encoded PRR has the
	// same sign as RINEX Doppler.
	UseSpecPhaseRangeRateSign bool
	// OmitZeroDoppler omits RTCM MSM Doppler observations with numeric value
	// zero, matching RTKLIB Explorer output.
	OmitZeroDoppler bool
}

// Converter converts parsed RTCM messages to RINEX records.
type Converter struct {
	sink   rinex.Sink
	opts   Options
	week   TimeInterval
	leapMS int64
	time   map[rtcmbin.GNSS]rinex.Time
	lock   map[signalKey]uint16
	arc    map[signalKey]uint32
	slip   map[signalKey]bool
}

type signalKey struct {
	sat rinex.SatelliteID
	sig rinex.SignalID
}

// New creates a Converter that writes records to sink.
// It panics if sink is nil.
func New(sink rinex.Sink, opts Options) *Converter {
	if sink == nil {
		panic("nil RINEX sink")
	}
	return &Converter{
		sink:   sink,
		opts:   opts,
		leapMS: defaultGPSUTCMS,
		time:   make(map[rtcmbin.GNSS]rinex.Time),
		lock:   make(map[signalKey]uint16),
		arc:    make(map[signalKey]uint32),
		slip:   make(map[signalKey]bool),
	}
}

// ConvertMsg converts one parsed RTCM message.
func (c *Converter) ConvertMsg(m rtcmbin.Msg, week TimeInterval) (bool, error) {
	if m, ok := m.(*rtcmbin.MSMHiRes); ok {
		if m.MSMLevel() != 7 {
			c.setWeek(week)
			return false, nil
		}
		return c.ConvertMSM7(m, week)
	}
	c.setWeek(week)
	switch m := m.(type) {
	case *rtcmbin.MT1005:
		return c.emitMetadata(metadata1005(m))
	case *rtcmbin.MT1006:
		return c.emitMetadata(metadata1006(m))
	case *rtcmbin.MT1007:
		return c.emitMetadata(metadata1007(m))
	case *rtcmbin.MT1008:
		return c.emitMetadata(metadata1008(m))
	case *rtcmbin.MT1013:
		c.leapMS = int64(m.LeapSeconds) * secondMS
		leap := int16(m.LeapSeconds)
		meta := rinex.Metadata{LeapSeconds: &leap}
		meta.Marker.Number = strconv.FormatUint(uint64(m.StationID), 10)
		return c.emitMetadata(meta)
	case *rtcmbin.MT1033:
		return c.emitMetadata(metadata1033(m))
	case *rtcmbin.MT1230:
		meta := rinex.Metadata{}
		meta.Marker.Number = strconv.FormatUint(uint64(m.StationID), 10)
		return c.emitMetadata(meta)
	default:
		return false, nil
	}
}

// ConvertMSM7 converts one parsed RTCM MSM7 message.
func (c *Converter) ConvertMSM7(m *rtcmbin.MSMHiRes, week TimeInterval) (bool, error) {
	if m.MSMLevel() != 7 {
		return false, nil
	}
	c.setWeek(week)
	t, err := c.resolveTime(&m.MSMHeader, week)
	if err != nil {
		return false, err
	}
	gnss := m.GNSS()
	ncell := m.Nsat() * m.Nsig()
	if ncell > 64 {
		return false, fmt.Errorf("RTCM MSM7 cell mask has %d bits, max 64", ncell)
	}
	c.time[gnss] = t
	sats := m.Satellites()
	sigs := m.Signals()
	seen := false
	cell := 0
	for i, satID := range sats {
		for j, sigID := range sigs {
			k := i*len(sigs) + j
			if !cellSet(m.CellMask, ncell, k) {
				continue
			}
			ok, err := c.convertCell(t, gnss, m, i, cell, satID, sigID)
			if err != nil {
				return seen, err
			}
			if ok {
				seen = true
			}
			cell++
		}
	}
	return seen, nil
}

func (c *Converter) setWeek(week TimeInterval) {
	if week.IsZero() {
		return
	}
	validateWeek(week)
	c.week = week
}

func (c *Converter) emitMetadata(meta rinex.Metadata) (bool, error) {
	return true, c.sink.Metadata(meta)
}

func (c *Converter) convertCell(t rinex.Time, gnss rtcmbin.GNSS, m *rtcmbin.MSMHiRes, satIndex, cellIndex int, satID, sigID uint8) (bool, error) {
	sys := rtcmbin.RINEXSys(gnss)
	satNum := rtcmbin.RINEXSatNum(gnss, satID)
	sig := rtcmbin.RINEXSig(gnss, sigID)
	if sys == "" || satNum == 0 || sig == "" {
		return false, nil
	}
	obs := rinex.SignalObservation{
		T:   t,
		Sat: rinex.SatelliteID(fmt.Sprintf("%s%02d", sys, satNum)),
		Sig: rinex.SignalID(sig),
	}
	var frq *int8
	if v, ok := glonassFrequencyChannel(gnss, m, satIndex); ok {
		obs.Frq = opt.Make(v)
		frq = &v
	}
	if pr, ok := pseudorange(m, satIndex, cellIndex); ok {
		obs.PR = opt.Make(pr)
	}
	freq, hasFrequency := frequencyHz(sys, sig, frq)
	if cp, ok := carrierPhase(m, satIndex, cellIndex, freq, hasFrequency); ok {
		obs.CP = opt.Make(cp)
	}
	if dop, ok := doppler(m, satIndex, cellIndex, freq, hasFrequency, c.opts.UseSpecPhaseRangeRateSign); ok && (!c.opts.OmitZeroDoppler || dop != 0) {
		obs.Do = opt.Make(dop)
	}
	if v, ok := cn0(m, cellIndex); ok {
		obs.CN0 = opt.Make(v)
	}
	obs.Arc, obs.HC = c.arcHC(obs.Sat, obs.Sig, m, cellIndex, obs.CP.IsSet())
	if len(obs.ObservationCodes()) == 0 {
		return false, nil
	}
	return true, c.sink.Observation(obs)
}

func metadata1005(m *rtcmbin.MT1005) rinex.Metadata {
	pos := m.ECEF()
	meta := rinex.Metadata{ApproxPosition: &pos}
	meta.Marker.Number = strconv.FormatUint(uint64(m.StationID), 10)
	return meta
}

func metadata1006(m *rtcmbin.MT1006) rinex.Metadata {
	meta := metadata1005(&m.MT1005)
	meta.AntennaDelta = &[3]float64{float64(m.AntennaHeight) / rtcmbin.Meter, 0, 0}
	return meta
}

func metadata1007(m *rtcmbin.MT1007) rinex.Metadata {
	meta := rinex.Metadata{Antenna: rinex.Antenna{Type: cleanASCII(m.AntennaDescriptor)}}
	meta.Marker.Number = strconv.FormatUint(uint64(m.StationID), 10)
	return meta
}

func metadata1008(m *rtcmbin.MT1008) rinex.Metadata {
	meta := metadata1007(&m.MT1007)
	meta.Antenna.Number = cleanASCII(m.AntennaSerialNumber)
	return meta
}

func metadata1033(m *rtcmbin.MT1033) rinex.Metadata {
	meta := metadata1008(&m.MT1008)
	meta.Receiver = rinex.Receiver{
		Number:  cleanASCII(m.ReceiverSerialNumber),
		Type:    cleanASCII(m.ReceiverTypeDescriptor),
		Version: cleanASCII(m.ReceiverFirmwareVersion),
	}
	return meta
}

func cleanASCII(s rtcmbin.ASCIIString) string {
	return strings.TrimRight(string(s), "\x00 ")
}

func (c *Converter) resolveTime(h *rtcmbin.MSMHeader, week TimeInterval) (rinex.Time, error) {
	offsets, err := c.epochWeekOffsets(h)
	if err != nil {
		return 0, err
	}
	if !week.IsZero() {
		return resolveWeek(offsets, week)
	}
	gnss := h.GNSS()
	prev, seen := c.time[gnss]
	if !seen && !c.week.IsZero() {
		return resolveWeek(offsets, c.week)
	}
	if seen {
		return resolveContinuity(offsets, prev), nil
	}
	return 0, fmt.Errorf("RTCM MSM7 epoch needs a week constraint")
}

// epochWeekOffsets returns possible GPS-week-relative epoch offsets in milliseconds.
// It normally returns one offset; GLONASS day 7 means the day is unknown, so
// the same time-of-day is possible on each day of the week.
func (c *Converter) epochWeekOffsets(h *rtcmbin.MSMHeader) ([]int64, error) {
	switch h.GNSS() {
	case rtcmbin.GLONASS:
		return c.glonassEpochWeekOffsets(h.EpochTime)
	case rtcmbin.BEIDOU:
		if h.EpochTime >= weekMS {
			return nil, fmt.Errorf("invalid RTCM MSM7 epoch time %d", h.EpochTime)
		}
		return []int64{int64(h.EpochTime) + bdtOffsetMS}, nil
	case "":
		return nil, fmt.Errorf("unknown RTCM MSM7 GNSS for message %d", h.MsgNum)
	default:
		if h.EpochTime >= weekMS {
			return nil, fmt.Errorf("invalid RTCM MSM7 epoch time %d", h.EpochTime)
		}
		return []int64{int64(h.EpochTime)}, nil
	}
}

func (c *Converter) glonassEpochWeekOffsets(epoch uint32) ([]int64, error) {
	day := int64(epoch >> 27)
	tod := int64(epoch & (1<<27 - 1))
	if tod >= dayMS {
		return nil, fmt.Errorf("invalid GLONASS time of day %d", tod)
	}
	if day != 7 {
		return []int64{day*dayMS + tod - glonassUTCOffsetMS + c.leapMS}, nil
	}
	offsets := make([]int64, 0, 7)
	for d := range int64(7) {
		offsets = append(offsets, d*dayMS+tod-glonassUTCOffsetMS+c.leapMS)
	}
	return offsets, nil
}

func resolveWeek(offsets []int64, week TimeInterval) (rinex.Time, error) {
	validateWeek(week)
	end := week.Start.Add(week.Duration)
	var match rinex.Time
	nmatch := 0
	seen := make(map[rinex.Time]bool)
	for _, offset := range offsets {
		w := floorDiv(week.Start.Sub(rinex.Epoch).Milliseconds()-offset, weekMS)
		for i := int64(-1); i <= 2; i++ {
			t := timeFromWeekOffset(w+i, offset)
			if seen[t] {
				continue
			}
			seen[t] = true
			tm := t.CivilTime()
			if !tm.Before(week.Start) && tm.Before(end) {
				match = t
				nmatch++
			}
		}
	}
	switch nmatch {
	case 0:
		return 0, fmt.Errorf("no MSM7 epoch matches the week constraint")
	case 1:
		return match, nil
	default:
		return 0, fmt.Errorf("MSM7 epoch is ambiguous in the week constraint")
	}
}

func validateWeek(week TimeInterval) {
	if week.Start.IsZero() {
		panic("week constraint start is zero")
	}
	if week.Duration <= 0 {
		panic("week constraint must be non-empty")
	}
	if week.Duration > maxInterval {
		panic("week constraint must not exceed one week")
	}
}

func resolveContinuity(offsets []int64, prev rinex.Time) rinex.Time {
	prevMS := int64(prev) / rinexTicksPerMS
	prevWeek := floorDiv(prevMS, weekMS)
	best := continuityCandidate(prevWeek, offsets[0], prevMS)
	bestDiff := absMS(int64(best)/rinexTicksPerMS - prevMS)
	for _, offset := range offsets[1:] {
		t := continuityCandidate(prevWeek, offset, prevMS)
		diff := absMS(int64(t)/rinexTicksPerMS - prevMS)
		if diff < bestDiff {
			best = t
			bestDiff = diff
		}
	}
	return best
}

func continuityCandidate(prevWeek, offset, prevMS int64) rinex.Time {
	candMS := prevWeek*weekMS + offset
	diff := candMS - prevMS
	if diff < -halfWeekMS {
		candMS += weekMS
	} else if diff > halfWeekMS {
		candMS -= weekMS
	}
	return rinex.Time(candMS * rinexTicksPerMS)
}

func timeFromWeekOffset(week, offset int64) rinex.Time {
	return rinex.Time((week*weekMS + offset) * rinexTicksPerMS)
}

func pseudorange(m *rtcmbin.MSMHiRes, satIndex, cellIndex int) (float64, bool) {
	r, ok1 := roughRange(m, satIndex)
	fine, ok3 := at(m.Sig.Pseudorange, cellIndex)
	if !ok1 || !ok3 || fine == -524288 {
		return 0, false
	}
	return r + float64(fine)*p2_29*rangeMS, true
}

func carrierPhase(m *rtcmbin.MSMHiRes, satIndex, cellIndex int, freq float64, hasFrequency bool) (float64, bool) {
	r, ok1 := roughRange(m, satIndex)
	fine, ok3 := at(m.Sig.PhaseRange, cellIndex)
	if !hasFrequency || !ok1 || !ok3 || fine == -8388608 {
		return 0, false
	}
	return (r + float64(fine)*p2_31*rangeMS) * freq / speedOfLight, true
}

// doppler converts MSM PhaseRangeRate to RINEX Doppler. With specSign, it
// follows RTCM's derivative sign and returns -PRR/wavelength; otherwise it
// leaves the observed receiver PRR polarity unchanged.
func doppler(m *rtcmbin.MSMHiRes, satIndex, cellIndex int, freq float64, hasFrequency, specSign bool) (float64, bool) {
	rough, ok1 := at(m.Sat.PhaseRate, satIndex)
	fine, ok2 := at(m.Sig.PhaseRate, cellIndex)
	if !hasFrequency || !ok1 || !ok2 || rough == -8192 || fine == -16384 {
		return 0, false
	}
	prr := float64(rough) + float64(fine)*0.0001
	if specSign {
		prr = -prr
	}
	d := float64(float32(prr * freq / speedOfLight))
	return d, true
}

func roughRange(m *rtcmbin.MSMHiRes, satIndex int) (float64, bool) {
	rint, ok1 := at(m.Sat.RangeInt, satIndex)
	rmod, ok2 := at(m.Sat.RangeMod, satIndex)
	if !ok1 || !ok2 || rint == 255 {
		return 0, false
	}
	return float64(rint)*rangeMS + float64(rmod)*p2_10*rangeMS, true
}

func cn0(m *rtcmbin.MSMHiRes, cellIndex int) (float32, bool) {
	v, ok := at(m.Sig.CNR, cellIndex)
	if !ok || v == 0 {
		return 0, false
	}
	return float32(v) * 0.0625, true
}

func glonassFrequencyChannel(gnss rtcmbin.GNSS, m *rtcmbin.MSMHiRes, satIndex int) (int8, bool) {
	if gnss != rtcmbin.GLONASS {
		return 0, false
	}
	v, ok := at(m.Sat.ExtInfo, satIndex)
	if !ok || v > 13 {
		return 0, false
	}
	return int8(v) - 7, true
}

func frequencyHz(sys, sig string, frq *int8) (float64, bool) {
	f, ok := frequencyMHz(sys, sig, frq)
	if !ok {
		return 0, false
	}
	return f * 1e6, true
}

func frequencyMHz(sys, sig string, frq *int8) (float64, bool) {
	if len(sig) != 2 {
		return 0, false
	}
	band := sig[0]
	switch sys {
	case "G":
		switch band {
		case '1':
			return 1575.420, true
		case '2':
			return 1227.600, true
		case '5':
			return 1176.450, true
		}
	case "R":
		if frq == nil {
			return 0, false
		}
		k := float64(*frq)
		switch band {
		case '1':
			return 1602.000 + k*0.5625, true
		case '2':
			return 1246.000 + k*0.4375, true
		}
	case "E":
		switch band {
		case '1':
			return 1575.420, true
		case '5':
			return 1176.450, true
		case '6':
			return 1278.750, true
		case '7':
			return 1207.140, true
		case '8':
			return 1191.795, true
		}
	case "S":
		switch band {
		case '1':
			return 1575.420, true
		case '5':
			return 1176.450, true
		}
	case "J":
		switch band {
		case '1':
			return 1575.420, true
		case '2':
			return 1227.600, true
		case '5':
			return 1176.450, true
		case '6':
			return 1278.750, true
		}
	case "C":
		switch band {
		case '1':
			return 1575.420, true
		case '2':
			return 1561.098, true
		case '5':
			return 1176.450, true
		case '6':
			return 1268.520, true
		case '7':
			return 1207.140, true
		}
	case "I":
		switch band {
		case '5':
			return 1176.450, true
		case '9':
			return 2492.028, true
		}
	}
	return 0, false
}

func (c *Converter) arcHC(sat rinex.SatelliteID, sig rinex.SignalID, m *rtcmbin.MSMHiRes, cellIndex int, hasPhase bool) (arc uint32, hc bool) {
	k := signalKey{sat: sat, sig: sig}
	prev := c.lock[k]
	ll := false
	if cur, ok := at(m.Sig.LockTime, cellIndex); ok {
		if cur < prev || cur == 0 && prev == 0 {
			ll = true
		}
		c.lock[k] = cur
	}
	if c.slip[k] {
		ll = true
		if hasPhase {
			delete(c.slip, k)
		}
	}
	if !hasPhase && ll {
		c.slip[k] = true
	}
	if ll {
		c.arc[k]++
	}
	if half, ok := at(m.Sig.HalfCycle, cellIndex); ok && half {
		hc = true
	}
	return c.arc[k], hc
}

func cellSet(mask uint64, ncell, cell int) bool {
	if ncell <= 0 || cell < 0 || cell >= ncell {
		return false
	}
	return mask>>uint(ncell-1-cell)&1 != 0
}

func at[S ~[]E, E any](s S, i int) (E, bool) {
	if i < 0 || i >= len(s) {
		var zero E
		return zero, false
	}
	return s[i], true
}

func floorDiv(n, d int64) int64 {
	q := n / d
	r := n % d
	if r < 0 {
		q--
	}
	return q
}

func absMS(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
