package sbfbin

import "encoding/binary"

type OSNMAStatus uint16
type RFStatusFlags uint8
type RFBandInfo uint8
type ExtError uint8
type RxState uint32
type RxError uint32

const (
	QualityIndicatorUnknown = 15
	ReceiverStatusCPUDNU    = 0xFF
	ReceiverStatusCmdDNU    = 0
	ReceiverStatusTempDNU   = 0
	AGCGainDNU              = -128
	AGCSampleVarDNU         = 0
	RFPowerDNU              = 0
)

// QualityInd is the SBF QualityInd block.
type QualityInd struct {
	Reserved   uint8
	Indicators []uint16
}

// BlockNumber returns the SBF block number.
func (*QualityInd) BlockNumber() uint16 { return QualityIndID }

// Chunks returns the binary chunks for the block parameters.
func (q *QualityInd) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		var n uint8
		if err := setCount(&n, len(q.Indicators), "QualityInd indicator"); err != nil {
			yield(err)
			return
		}
		if !yield(&n) || !yield(&q.Reserved) {
			return
		}
		if len(q.Indicators) != int(n) {
			q.Indicators = make([]uint16, int(n))
		}
		yield(q.Indicators)
	}
}

// GALAuthStatus is the SBF GALAuthStatus block.
type GALAuthStatus struct {
	OSNMAStatus      OSNMAStatus
	TrustedTimeDelta float32
	GalActiveMask    uint64
	GalAuthenticMask uint64
	GpsActiveMask    uint64
	GpsAuthenticMask uint64
}

// BlockNumber returns the SBF block number.
func (*GALAuthStatus) BlockNumber() uint16 { return GALAuthStatusID }

type rfStatusHead struct {
	SBLength uint8
	Flags    RFStatusFlags
	Reserved [3]uint8
}

// RFBand is an RFStatus sub-block.
type RFBand struct {
	Frequency uint32
	Bandwidth uint16
	Info      RFBandInfo
	Power     int8
}

// RFStatus is the SBF RFStatus block.
type RFStatus struct {
	rfStatusHead
	RFBand []RFBand
}

// BlockNumber returns the SBF block number.
func (*RFStatus) BlockNumber() uint16 { return RFStatusID }

// Chunks returns the binary chunks for the block parameters.
func (r *RFStatus) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		var n uint8
		if err := setCount(&n, len(r.RFBand), "RFStatus RFBand"); err != nil {
			yield(err)
			return
		}
		if n > 0 && r.SBLength == 0 {
			r.SBLength = uint8(binary.Size(RFBand{}))
		}
		if !yield(&n) || !yield(&r.rfStatusHead) {
			return
		}
		if len(r.RFBand) != int(n) {
			r.RFBand = make([]RFBand, int(n))
		}
		pad := int(r.SBLength) - binary.Size(RFBand{})
		for i := range r.RFBand {
			if !yield(&r.RFBand[i]) {
				return
			}
			if !yield(paddingChunk(pad)) {
				return
			}
		}
	}
}

type receiverStatusHead struct {
	CPULoad     uint8
	ExtError    ExtError
	UpTime      uint32
	RxState     RxState
	RxError     RxError
	SBLength    uint8
	CmdCount    uint8
	Temperature uint8
}

// AGCState is a ReceiverStatus sub-block.
type AGCState struct {
	FrontEndID   uint8
	Gain         int8
	SampleVar    uint8
	BlankingStat uint8
}

// ReceiverStatus is the SBF ReceiverStatus block.
type ReceiverStatus struct {
	receiverStatusHead
	AGCState []AGCState
}

// BlockNumber returns the SBF block number.
func (*ReceiverStatus) BlockNumber() uint16 { return ReceiverStatusID }

// Chunks returns the binary chunks for the block parameters.
func (r *ReceiverStatus) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		var n uint8
		if err := setCount(&n, len(r.AGCState), "ReceiverStatus AGCState"); err != nil {
			yield(err)
			return
		}
		if n > 0 && r.SBLength == 0 {
			r.SBLength = uint8(binary.Size(AGCState{}))
		}
		if !yield(&r.CPULoad) || !yield(&r.ExtError) || !yield(&r.UpTime) ||
			!yield(&r.RxState) || !yield(&r.RxError) || !yield(&n) ||
			!yield(&r.SBLength) || !yield(&r.CmdCount) || !yield(&r.Temperature) {
			return
		}
		if len(r.AGCState) != int(n) {
			r.AGCState = make([]AGCState, int(n))
		}
		pad := int(r.SBLength) - binary.Size(AGCState{})
		for i := range r.AGCState {
			if !yield(&r.AGCState[i]) {
				return
			}
			if !yield(paddingChunk(pad)) {
				return
			}
		}
	}
}

func init() {
	regBlock[QualityInd]("QualityInd")
	regBlock[GALAuthStatus]("GALAuthStatus")
	regBlock[RFStatus]("RFStatus")
	regBlock[ReceiverStatus]("ReceiverStatus")
}
