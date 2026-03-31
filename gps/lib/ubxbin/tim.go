package ubxbin

const (
	TimSvinID MsgID = clsTim | (0x04 << 8)
	TimTosID  MsgID = clsTim | (0x12 << 8)
	TimTPID   MsgID = clsTim | (0x01 << 8)
)

type TimTos struct {
	Version           byte        `json:"version"`
	GNSSID            GNSSID      `json:"gnssId"`
	_                 [2]byte
	Flags             TimTosFlags `json:"flags"`
	Year              uint16      `json:"year"`
	Month             byte        `json:"month"`
	Day               byte        `json:"day"`
	Hour              byte        `json:"hour"`
	Minute            byte        `json:"minute"`
	Second            byte        `json:"second"`
	UTCStandard       UTCStandard `json:"utcStandard"`
	UTCOffset         int32       `json:"utcOffset"`
	UTCUncertainty    uint32      `json:"utcUncertainty"`
	Week              uint32      `json:"week"`
	TOW               uint32      `json:"TOW"`
	GNSSOffset        int32       `json:"gnssOffset"`
	GNSSUncertainty   uint32      `json:"gnssUncertainty"`
	IntOscOffset      int32       `json:"intOscOffset"`
	IntOscUncertainty uint32      `json:"intOscUncertainty"`
	ExtOscOffset      int32       `json:"extOscOffset"`
	ExtOscUncertainty uint32      `json:"extOscUncertainty"`
}

var _ PartiallyHandledMsg = (*TimTos)(nil)

func (m *TimTos) ID() MsgID { return TimTosID }

func (m *TimTos) IsHandled() bool {
	return m.Version == 0
}

type TimTosFlags uint32

const (
	TimTosLeapNow TimTosFlags = 1 << iota
	TimTosLeapSoon
	TimTosLeapPositive
	TimTosTimeInLimit
	TimTosIntOscInLimit
	TimTosExtOscInLimit
	TimTosGNSSTimeValid
	TimTosUTCTimeValid
	TimTosDiscSrc0
	TimTosDiscSrc1
	TimTosDiscSrc2
	TimTosRAIM
	TimTosCohPulse
	TimTosLockedPulse
	TimTosDiscSrc TimTosFlags = TimTosDiscSrc0 | TimTosDiscSrc1 | TimTosDiscSrc2
)

// values for (Flags & TimTosDiscSrc)
const (
	TimTosDiscSrcIntOsc TimTosFlags = iota << 8
	TimTosDiscSrcGNSS
	TimTosDiscSrcEXTINT0
	TimTosDiscSrcEXTINT1
	TimTosDiscSrcIntOscHost
	TimTosDiscSrcExtOscHost
)

type TimTP struct {
	TOWMS    uint32       `json:"towMS"`
	TOWSubMS uint32       `json:"towSubMS"`
	QErr     int32        `json:"qErr"`
	Week     uint16       `json:"week"`
	Flags    TimTPFlags   `json:"flags"`
	RefInfo  TimTPRefInfo `json:"refInfo"`
}

func (m *TimTP) ID() MsgID { return TimTPID }

type TimTPFlags byte

const (
	TimTPTimeBase TimTPFlags = 1 << iota
	TimTPUTCAvailable
	timTPRAIM1
	timTPRAIM2
	TimTPQErrInvalid
	TimTPRAIM TimTPFlags = timTPRAIM1 | timTPRAIM2
)

const (
	TimTPTimeBaseGNSS TimTPFlags = 0
	TimTPTimeBaseUTC  TimTPFlags = TimTPTimeBase
)

// Values for TimTPFlags & TimTPRAIM
const (
	TimTPRAIMNotActive TimTPFlags = timTPRAIM1
	TimTPRAIMActive    TimTPFlags = timTPRAIM2
)

type TimTPRefInfo byte

const (
	TimTPTimeRefGNSS TimTPRefInfo = 0x0F
)

func (ri TimTPRefInfo) UTCStandard() UTCStandard {
	return UTCStandard(ri >> 4)
}

// Values for TimTPRefInfo & TimTPTimeRefGNSS
const (
	TimTPTimeRefGPS TimTPRefInfo = iota
	TimTPTimeRefGLONASS
	TimTPTimeRefBeiDou
	TimTPTimeRefGalileo
	TimTPTimeRefNavIC
	TimTPTimeRefGNSSUnknown TimTPRefInfo = 15
)

type TimSvin struct {
	Dur    uint32 `json:"dur"`
	MeanX  int32  `json:"meanX"`
	MeanY  int32  `json:"meanY"`
	MeanZ  int32  `json:"meanZ"`
	MeanV  uint32 `json:"meanV"`
	Obs    uint32 `json:"obs"`
	Valid  byte   `json:"valid"`
	Active byte   `json:"active"`
	_      [2]byte
}

func (m *TimSvin) ID() MsgID { return TimSvinID }

func init() {
	regMsg[TimSvin]("SVIN")
	regMsg[TimTos]("TOS")
	regMsg[TimTP]("TP")
}
