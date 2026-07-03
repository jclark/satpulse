package sbfbin

type SyncLevel uint8
type PPSTimescale uint8

const (
	UTCComponentDNU = -128

	PPSTimescaleGPS PPSTimescale = iota + 1
	PPSTimescaleUTC
	PPSTimescaleReceiver
	PPSTimescaleGLONASS
	PPSTimescaleGalileo
	PPSTimescaleBeiDou
	PPSTimescaleFugroAtomiChron PPSTimescale = 100
)

// ReceiverTime is the SBF ReceiverTime block.
type ReceiverTime struct {
	UTCYear   int8
	UTCMonth  int8
	UTCDay    int8
	UTCHour   int8
	UTCMin    int8
	UTCSec    int8
	DeltaLS   int8
	SyncLevel SyncLevel
}

// BlockNumber returns the SBF block number.
func (*ReceiverTime) BlockNumber() uint16 { return ReceiverTimeID }

// XPPSOffset is the SBF xPPSOffset block.
type XPPSOffset struct {
	SyncAge   uint8
	Timescale PPSTimescale
	Offset    float32
}

// BlockNumber returns the SBF block number.
func (*XPPSOffset) BlockNumber() uint16 { return XPPSOffsetID }

func init() {
	regBlock[ReceiverTime]("ReceiverTime")
	regBlock[XPPSOffset]("xPPSOffset")
}
