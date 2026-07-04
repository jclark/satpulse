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
	BasePositionDNU          = -2e10
)

type diffCorrInHead struct {
	Mode   DiffCorrMode
	Source DiffCorrSource
}

// DiffCorrIn is the SBF DiffCorrIn block.
type DiffCorrIn struct {
	diffCorrInHead
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
		payloadLen := d.payloadLen()
		if payloadLen > 0 {
			if payloadLen < 2 {
				yield(chunkError("payload shorter than DiffCorrIn fixed fields"))
				return
			}
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
