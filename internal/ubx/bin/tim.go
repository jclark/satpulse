package bin

const (
	TimSvinID MsgID = clsTim | (0x04 << 8)
	TimTosID  MsgID = clsTim | (0x12 << 8)
	TimTPID   MsgID = clsTim | (0x01 << 8)
)

type TimTos struct {
	Version           byte
	GNSSID            GNSSID
	_                 [2]byte
	Flags             TimTosFlags
	Year              uint16
	Month             byte
	Day               byte
	Hour              byte
	Minute            byte
	Second            byte
	UTCStandard       UTCStandard
	UTCOffset         int32
	UTCUncertainty    uint32
	Week              uint32
	TOW               uint32
	GNSSOffset        int32
	GNSSUncertainty   uint32
	IntOscOffset      int32
	IntOscUncertainty uint32
	ExtOscOffset      int32
	ExtOscUncertainty uint32
}

func (m *TimTos) ID() MsgID { return TimTosID }

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
	TOWMS    uint32
	TOWSubMS uint32
	QErr     int32
	Week     uint16
	Flags    TimTPFlags
	RefInfo  TimTPRefInfo
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
	Dur    uint32
	MeanX  int32
	MeanY  int32
	MeanZ  int32
	MeanV  uint32
	Obs    uint32
	Valid  byte
	Active byte
	_      [2]byte
}

func (m *TimSvin) ID() MsgID { return TimSvinID }

func init() {
	regMsg[TimSvin]("SVIN")
	regMsg[TimTos]("TOS")
	regMsg[TimTP]("TP")
}
