package sbfbin

type DiffCorrMode uint8
type DiffCorrSource uint8
type BaseType uint8
type BaseSource uint8

const (
	DiffCorrModeRTCMv2 DiffCorrMode = iota
	DiffCorrModeCMR
	DiffCorrModeRTCMv3
	DiffCorrModeRTCMV
	DiffCorrModeSPARTN
	DiffCorrSourceDNU DiffCorrSource = 0xFF

	BaseTypeFixed   BaseType = 0
	BaseTypeMoving  BaseType = 1
	BaseTypeUnknown BaseType = 0xFF
)

// BaseSourceRTCM is the BaseStation.Source value for coordinates decoded from
// an RTCM3 1005/1006 message.
const BaseSourceRTCM BaseSource = 8

type diffCorrInHead struct {
	Mode   DiffCorrMode
	Source DiffCorrSource
}

// DiffCorrIn is the SBF DiffCorrIn block.
type DiffCorrIn struct {
	diffCorrInHead
	// Correction includes the block's 0-3 padding bytes: SBF gives no explicit
	// correction length, so consumers must find the true end from the
	// correction format's own framing.
	Correction []byte
	payloadSize
}

// BlockNumber returns the SBF block number.
func (*DiffCorrIn) BlockNumber() uint16 { return DiffCorrInID }

// Chunks returns the binary chunks for the block parameters.
func (d *DiffCorrIn) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		if !yield(&d.diffCorrInHead) {
			return
		}
		n := len(d.Correction)
		if payloadLen := d.payloadLen(); payloadLen > 0 {
			n = payloadLen - 2
		}
		if len(d.Correction) != n {
			d.Correction = make([]byte, n)
		}
		yield(d.Correction)
	}
}

// BaseStation is the SBF BaseStation block.
type BaseStation struct {
	BaseStationID uint16
	BaseType      BaseType
	Source        BaseSource
	Datum         Datum
	Reserved      uint8
	X             float64
	Y             float64
	Z             float64
}

// BlockNumber returns the SBF block number.
func (*BaseStation) BlockNumber() uint16 { return BaseStationID }

func init() {
	regBlock[DiffCorrIn]("DiffCorrIn")
	regBlock[BaseStation]("BaseStation")
}
