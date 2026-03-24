package casbin

const (
	Tim2TpxID     MsgID = clsTim2 | (0x00 << 8)
	Tim2TimeGPSID MsgID = clsTim2 | (0x01 << 8)
	Tim2TimeBDSID MsgID = clsTim2 | (0x02 << 8)
	Tim2TimeGLNID MsgID = clsTim2 | (0x03 << 8)
	Tim2TimeGALID MsgID = clsTim2 | (0x04 << 8)
	Tim2TimeIRNID MsgID = clsTim2 | (0x05 << 8)
	Tim2TimePosID MsgID = clsTim2 | (0x06 << 8)
	Tim2LsID      MsgID = clsTim2 | (0x07 << 8)
	Tim2LyID      MsgID = clsTim2 | (0x08 << 8)
	Tim2TcxoID    MsgID = clsTim2 | (0x09 << 8)
)

// Tim2TSrc identifies the GNSS time source for timing messages (section 3.7.4).
// This uses different numbering from the general GNSSID.
type Tim2TSrc uint8

const (
	Tim2TSrcGPS Tim2TSrc = iota
	Tim2TSrcBDS
	Tim2TSrcGLN
	Tim2TSrcGAL
	Tim2TSrcIRN
)

// Tim2TimeBase selects the reference time base for PPS output.
type Tim2TimeBase uint8

const (
	Tim2TimeBaseGNSS Tim2TimeBase = iota
	Tim2TimeBaseUTC
)

// Tim2PPSFlag is a bitfield indicating PPS pulse reliability.
type Tim2PPSFlag uint8

const (
	Tim2PPSTOWValid  Tim2PPSFlag = 1 << iota // B0: TOW available
	Tim2PPSWnValid                           // B1: week number valid
	Tim2PPSLsValid                           // B2: leap second info valid
	Tim2PPSTimeValid                         // B3: time valid
	_
	_
	Tim2PPSPulseValid // B6: PPS pulse valid
)

// Tim2Tpx is TIM2-TPX (0x12 0x00) - timing pulse information (24 bytes)
type Tim2Tpx struct {
	TOW      uint32       // ms, GNSS system time integer ms
	TOWSubms int32        // scaled 2^-30 ms, fractional ms
	Wn       uint16       // week number
	PPSFlag  Tim2PPSFlag  // PPS reliability flags
	TBase    Tim2TimeBase // 0=GNSS, 1=UTC
	TSrc     Tim2TSrc     // timing source
	_        uint8
	_        uint16
	_        uint16
	LeapSec  int8  // s, current leap seconds
	QuanErr  int8  // 0.1 ns, PPS quantization error
	TAcc     int16 // 0.1 ns, time error estimate
	Dt2Utc   int16 // 0.1 ns, satellite time offset to UTC
}

func (m *Tim2Tpx) ID() MsgID { return Tim2TpxID }

// Tim2WnValid indicates week number validity (section 3.7.5).
type Tim2WnValid uint8

const (
	Tim2WnInvalid      Tim2WnValid = iota
	Tim2WnExternal
	Tim2WnFromGNSS
	Tim2WnFromGNSSGood
)

// Tim2TimeFlag is a bitfield indicating time validity (section 3.7.3).
type Tim2TimeFlag uint8

const (
	Tim2TimeFlagTOWValid  Tim2TimeFlag = 1 << iota // B0: TOW valid
	Tim2TimeFlagWnValid                            // B1: week number valid
	Tim2TimeFlagLsValid                            // B2: leap second info valid
	Tim2TimeFlagReliable                           // B3: time reliable
)

// Tim2UtcFlag indicates UTC parameter validity.
type Tim2UtcFlag uint8

const (
	Tim2UtcNone      Tim2UtcFlag = iota
	Tim2UtcUpdated
	Tim2UtcExpired
	Tim2UtcAuxiliary
)

// Tim2LsEventFlag indicates leap second event validity.
type Tim2LsEventFlag uint8

const (
	Tim2LsEventNone     Tim2LsEventFlag = iota
	Tim2LsEventNoEvent
	Tim2LsEventNormal
	Tim2LsEventAbnormal
)

// Tim2TimeGNSS is the shared structure for TIM2-TIMEGPS/BDS/GLN/GAL/IRN
// (0x12 0x01-0x05) - GNSS system time information (36 bytes)
// Tim2TimeGNSS is the shared structure for TIM2-TIMEGPS/BDS/GLN/GAL/IRN
// (0x12 0x01-0x05) - GNSS system time information (36 bytes)
type Tim2TimeGNSS struct {
	Nav2TOW                         // ms, integer TOW
	TOWSubms int32           // scaled 2^-30 ms, fractional TOW
	Wn       uint16          // week number
	TOWFlag  PVTValid    // TOW valid flag
	WnFlag   Tim2WnValid     // week number valid flag
	TotSec   uint32          // s, total seconds
	TAcc     float32         // ns, time error estimate
	Dt2Utc   float32         // ns, offset to UTC
	DtLsfUtc int32           // s, time until leap second event
	Ls       int8            // s, current leap seconds
	Lsf      int8            // s, forecast leap seconds
	TFlag    Tim2TimeFlag    // time status flags
	TSrc     Tim2TSrc        // timing source
	UtcFlag  Tim2UtcFlag     // UTC parameter valid flag
	LsFlag   Tim2LsEventFlag // leap second event valid flag
	LsYear   uint16          // BIT[15:1]=year, BIT0=0(Jun30)/1(Dec31)
}

// Tim2TimeGPS is TIM2-TIMEGPS (0x12 0x01)
type Tim2TimeGPS struct{ Tim2TimeGNSS }

func (m *Tim2TimeGPS) ID() MsgID { return Tim2TimeGPSID }

// Tim2TimeBDS is TIM2-TIMEBDS (0x12 0x02)
type Tim2TimeBDS struct{ Tim2TimeGNSS }

func (m *Tim2TimeBDS) ID() MsgID { return Tim2TimeBDSID }

// Tim2TimeGLN is TIM2-TIMEGLN (0x12 0x03)
type Tim2TimeGLN struct{ Tim2TimeGNSS }

func (m *Tim2TimeGLN) ID() MsgID { return Tim2TimeGLNID }

// Tim2TimeGAL is TIM2-TIMEGAL (0x12 0x04)
type Tim2TimeGAL struct{ Tim2TimeGNSS }

func (m *Tim2TimeGAL) ID() MsgID { return Tim2TimeGALID }

// Tim2TimeIRN is TIM2-TIMEIRN (0x12 0x05)
type Tim2TimeIRN struct{ Tim2TimeGNSS }

func (m *Tim2TimeIRN) ID() MsgID { return Tim2TimeIRNID }

// Tim2FixMode indicates how the timing position was determined.
type Tim2FixMode uint8

const (
	Tim2FixModeAutonomous Tim2FixMode = iota + 1
	Tim2FixModeExternal
	Tim2FixModeRealtime
)

// Tim2TimePos is TIM2-TIMEPOS (0x12 0x06) - timing engine position status (64 bytes)
type Tim2TimePos struct {
	XTim       float64      // m, timing engine fixed position X
	YTim       float64      // m, timing engine fixed position Y
	ZTim       float64      // m, timing engine fixed position Z
	XFix       float64      // m, positioning engine real-time position X
	YFix       float64      // m, positioning engine real-time position Y
	ZFix       float64      // m, positioning engine real-time position Z
	SurTimer   uint32       // s, survey-in running time
	SurPacc    float32      // m, survey-in position accuracy
	FixFlag    PVTValid // position validity flag
	TimFixMode Tim2FixMode  // how timing position was determined
	PRResiRms  uint16       // m, pseudorange residual RMS
	PosBias    uint16       // m, deviation between timing and real-time position
	PDOP       uint8        // scaled 0.1
	_          uint8
}

func (m *Tim2TimePos) ID() MsgID { return Tim2TimePosID }

// Tim2RaimType is the RAIM alarm type (section 3.7.6).
type Tim2RaimType uint8

const (
	Tim2RaimNone       Tim2RaimType = iota
	Tim2RaimNoLsInfo
	Tim2RaimLsNormal
	Tim2RaimLsAbnormal
	Tim2RaimWnNormal
	Tim2RaimWnAbnormal
)

// Tim2Ls is TIM2-LS (0x12 0x07) - leap second exception alarm (16 bytes)
type Tim2Ls struct {
	UtcExpPrnMask uint64       // PRN mask (BIT0=PRN1, etc.)
	GNSSID        GNSSID       // system ID (section 1.4)
	SigID         uint8        // signal ID (section 1.4)
	SVID          uint8        // satellite ID
	RaimType      Tim2RaimType // alarm type
	Wnlsf         uint8        // week number of leap second event
	Dn            uint8        // day number of leap second event
	Dtls          int8         // leap second value before event
	Dtlsf         int8         // leap second value after event
}

func (m *Tim2Ls) ID() MsgID { return Tim2LsID }

// Tim2Ly is TIM2-LY (0x12 0x08) - week number exception alarm (16 bytes)
type Tim2Ly struct {
	WntExpPrnMask uint64       // PRN mask (BIT0=PRN1, etc.)
	GNSSID        GNSSID       // system ID (section 1.4)
	SigID         uint8        // signal ID (section 1.4)
	SVID          uint8        // satellite ID
	RaimType      Tim2RaimType // alarm type
	WnBias        int16        // bias between wrong and correct week number
	_             uint16
}

func (m *Tim2Ly) ID() MsgID { return Tim2LyID }

// Tim2Tcxo is TIM2-TCXO (0x12 0x09) - TCXO frequency offset info (20 bytes)
type Tim2Tcxo struct {
	DfuRatio  float32      // relative frequency offset of sampling clock
	Dfu2Tcxo  float32      // difference between sampling clock and TCXO offset
	DfxRatio  float32      // TCXO frequency offset (DfuRatio + Dfu2Tcxo)
	VcBias    int32        // scaled 2^-28, VCTCXO voltage control offset
	FrqFlag   PVTValid // frequency valid flag
	TcxoDoCnt uint8        // VCTCXO taming convergence count
	_         uint16
}

func (m *Tim2Tcxo) ID() MsgID { return Tim2TcxoID }

func init() {
	regMsg[Tim2Tpx]("TPX")
	regMsg[Tim2TimeGPS]("TIMEGPS")
	regMsg[Tim2TimeBDS]("TIMEBDS")
	regMsg[Tim2TimeGLN]("TIMEGLN")
	regMsg[Tim2TimeGAL]("TIMEGAL")
	regMsg[Tim2TimeIRN]("TIMEIRN")
	regMsg[Tim2TimePos]("TIMEPOS")
	regMsg[Tim2Ls]("LS")
	regMsg[Tim2Ly]("LY")
	regMsg[Tim2Tcxo]("TCXO")
}
