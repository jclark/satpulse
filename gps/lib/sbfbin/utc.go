package sbfbin

type UtcSource uint8

const (
	GALUtcSourceINAV UtcSource = 2
	GALUtcSourceFNAV UtcSource = 16
	UTCDeltaLSDNU              = -128
	GALUtcDNU                  = -2e10
)

// GPSUtc is the SBF GPSUtc block.
type GPSUtc struct {
	PRN       uint8
	Reserved  uint8
	A_1       float32
	A_0       float64
	T_ot      uint32
	WN_t      uint8
	DEL_t_LS  int8
	WN_LSF    uint8
	DN        uint8
	DEL_t_LSF int8
}

// BlockNumber returns the SBF block number.
func (*GPSUtc) BlockNumber() uint16 { return GPSUtcID }

// GALUtc is the SBF GALUtc block.
type GALUtc struct {
	SVID      uint8
	Source    UtcSource
	A_1       float32
	A_0       float64
	T_ot      uint32
	WN_ot     uint8
	DEL_t_LS  int8
	WN_LSF    uint8
	DN        uint8
	DEL_t_LSF int8
}

// BlockNumber returns the SBF block number.
func (*GALUtc) BlockNumber() uint16 { return GALUtcID }

// BDSUtc is the SBF BDSUtc block.
type BDSUtc struct {
	PRN       uint8
	Reserved  uint8
	A_1       float32
	A_0       float64
	DEL_t_LS  int8
	WN_LSF    uint8
	DN        uint8
	DEL_t_LSF int8
}

// BlockNumber returns the SBF block number.
func (*BDSUtc) BlockNumber() uint16 { return BDSUtcID }

func init() {
	regBlock[GPSUtc]("GPSUtc")
	regBlock[GALUtc]("GALUtc")
	regBlock[BDSUtc]("BDSUtc")
}
