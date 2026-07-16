package sbfbin

import "github.com/jclark/satpulse/gps/lib/latin1z"

const ReceiverSetupPositionDNU = -2e10

type receiverSetupFixed struct {
	Reserved       [2]uint8
	MarkerName     latin1z.StringZ60
	MarkerNumber   latin1z.StringZ20
	Observer       latin1z.StringZ20
	Agency         latin1z.StringZ40
	RxSerialNumber latin1z.StringZ20
	RxName         latin1z.StringZ20
	RxVersion      latin1z.StringZ20
	AntSerialNbr   latin1z.StringZ20
	AntType        latin1z.StringZ20
	DeltaH         float32
	DeltaE         float32
	DeltaN         float32
}

type receiverSetupRev1 struct {
	MarkerType latin1z.StringZ20
}

type receiverSetupRev2 struct {
	GNSSFWVersion latin1z.StringZ40
}

type receiverSetupRev3 struct {
	ProductName latin1z.StringZ40
	Latitude    float64
	Longitude   float64
	Height      float32
	StationCode latin1z.StringZ10
}

type receiverSetupRev4 struct {
	MonumentIdx uint8
	ReceiverIdx uint8
	CountryCode latin1z.StringZ3
	Reserved1   latin1z.StringZ21
}

// ReceiverSetup is the SBF ReceiverSetup block.
type ReceiverSetup struct {
	receiverSetupFixed
	receiverSetupRev1
	receiverSetupRev2
	receiverSetupRev3
	receiverSetupRev4
	payloadSize
}

// BlockNumber returns the SBF block number.
func (*ReceiverSetup) BlockNumber() uint16 { return ReceiverSetupID }

func (r *ReceiverSetup) setDNUDefaults() {
	r.Latitude = ReceiverSetupPositionDNU
	r.Longitude = ReceiverSetupPositionDNU
	r.Height = ReceiverSetupPositionDNU
}

// Chunks returns the binary chunks for the block parameters.
func (r *ReceiverSetup) Chunks() func(yield func(chunk any) bool) {
	return revisionChunks(r.payloadLen(), "ReceiverSetup", &r.receiverSetupFixed,
		&r.receiverSetupRev1, &r.receiverSetupRev2, &r.receiverSetupRev3, &r.receiverSetupRev4)
}

// encodedRev returns 4: Chunks always writes every trailer group.
func (r *ReceiverSetup) encodedRev() (uint8, bool) { return 4, true }

func init() {
	regBlock[ReceiverSetup]("ReceiverSetup")
}
