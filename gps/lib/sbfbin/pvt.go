package sbfbin

type Mode uint8
type ErrCode uint8
type TimeSystem uint8
type Datum uint8
type WACorrInfo uint8
type AlertFlag uint8
type PPPInfo uint16
type Misc uint8

const (
	ModeNoPVT Mode = iota
	ModeStandalone
	ModeDifferential
	ModeFixedLocation
	ModeRTKFixed
	ModeRTKFloat
	ModeSBAS
	ModeMovingBaseRTKFixed
	ModeMovingBaseRTKFloat
	ModeReserved9
	ModePPP
)

// Fix returns the PVT solution type (low nibble of Mode), one of the Mode*
// constants (ModeNoPVT, ModeStandalone, ...).
func (m Mode) Fix() Mode {
	return m & 0x0F
}

// Is2D reports whether the 2D-mode bit (bit 7) is set in Mode.
func (m Mode) Is2D() bool {
	return m&0x80 != 0
}

// DeterminingFixed reports whether the "still determining the fixed position"
// bit (bit 6) is set in Mode.
func (m Mode) DeterminingFixed() bool {
	return m&0x40 != 0
}

// IsSBAS reports whether the fix is SBAS-aided, in which case ReferenceID is
// an SBAS PRN rather than an RTCM base ID.
func (m Mode) IsSBAS() bool {
	return m.Fix() == ModeSBAS
}

// WACorrInfo base-type codes, encoded in bits 5-6 of WACorrInfo.
const (
	WACorrBaseNone     uint8 = 0 // unknown or not in differential positioning mode
	WACorrBasePhysical uint8 = 1 // physical base station (RTK)
	WACorrBaseVirtual  uint8 = 2 // virtual base station (VRS)
	WACorrBaseSSR      uint8 = 3 // state-space (RTK-SSR / PPP-RTK)
)

// BaseType returns the base-type code (bits 5-6) of WACorrInfo.
func (w WACorrInfo) BaseType() uint8 {
	return uint8(w>>5) & 0x3
}

// CovDNU is the do-not-use sentinel of the PosCov*/VelCov* covariance fields
// (f4, exactly representable, safe for == comparison).
const CovDNU = -2e10

const (
	ErrNone ErrCode = iota
	ErrNotEnoughMeasurements
	ErrNotEnoughEphemerides
	ErrDOPTooLarge
	ErrResidualsTooLarge
	ErrNoConvergence
	ErrNotEnoughMeasurementsAfterOutlierRejection
	ErrExportLaw
	ErrNotEnoughCorrections
	ErrBaseStationUnavailable
	ErrAmbiguitiesNotFixed
)

const (
	TimeSystemGPS TimeSystem = iota
	TimeSystemGalileo
	TimeSystemReserved2
	TimeSystemGLONASS
	TimeSystemBeiDou
	TimeSystemQZSS
	TimeSystemFugroAtomiChron TimeSystem = 100
	TimeSystemDNU             TimeSystem = 0xFF
)

const (
	DatumWGS84             Datum  = 0
	DatumBaseStation       Datum  = 19
	DatumETRS89            Datum  = 30
	DatumNAD83             Datum  = 31
	DatumNAD83PACP00       Datum  = 32
	DatumNAD83MARP00       Datum  = 33
	DatumGDA94             Datum  = 34
	DatumGDA2020           Datum  = 35
	DatumJGD2011           Datum  = 36
	DatumUserDefined250    Datum  = 250
	DatumUserDefined251    Datum  = 251
	DatumDNU               Datum  = 0xFF
	PVTAccuracyDNU         uint16 = 0xFFFF
	PVTReferenceIDMulti    uint16 = 0xFFFE
	PVTReferenceIDDNU      uint16 = 0xFFFF
	PVTMeanCorrAgeDNU      uint16 = 0xFFFF
	PVTCoordinateDNU              = -2e10
	PVTClockDNU                   = -2e10
	PVTVelocityDNU                = -2e10
	PVTCourseOverGroundDNU        = -2e10
)

type pvtGeodeticFixed struct {
	Mode        Mode
	Error       ErrCode
	Latitude    float64
	Longitude   float64
	Height      float64
	Undulation  float32
	Vn          float32
	Ve          float32
	Vu          float32
	COG         float32
	RxClkBias   float64
	RxClkDrift  float32
	TimeSystem  TimeSystem
	Datum       Datum
	NrSV        uint8
	WACorrInfo  WACorrInfo
	ReferenceID uint16
	MeanCorrAge uint16
	SignalInfo  uint32
	AlertFlag   AlertFlag
	NrBases     uint8
}

type pvtCartesianFixed struct {
	Mode        Mode
	Error       ErrCode
	X           float64
	Y           float64
	Z           float64
	Undulation  float32
	Vx          float32
	Vy          float32
	Vz          float32
	COG         float32
	RxClkBias   float64
	RxClkDrift  float32
	TimeSystem  TimeSystem
	Datum       Datum
	NrSV        uint8
	WACorrInfo  WACorrInfo
	ReferenceID uint16
	MeanCorrAge uint16
	SignalInfo  uint32
	AlertFlag   AlertFlag
	NrBases     uint8
}

type pvtRev1 struct {
	PPPInfo PPPInfo
	Latency uint16
}

type pvtRev2 struct {
	HAccuracy uint16
	VAccuracy uint16
	Misc      Misc
}

type pvtTrailer struct {
	pvtRev1
	pvtRev2
	payloadSize
}

func (p *pvtTrailer) setDNUDefaults() {
	p.Latency = PVTAccuracyDNU
	p.HAccuracy = PVTAccuracyDNU
	p.VAccuracy = PVTAccuracyDNU
}

func (p *pvtTrailer) chunks(name string, fixed any) func(yield func(chunk any) bool) {
	return revisionChunks(p.payloadLen(), name, fixed, &p.pvtRev1, &p.pvtRev2)
}

func (p *pvtTrailer) latestRev() uint8 { return 3 }

// PVTGeodetic is the SBF PVTGeodetic block.
type PVTGeodetic struct {
	pvtGeodeticFixed
	pvtTrailer
}

// PVTCartesian is the SBF PVTCartesian block.
type PVTCartesian struct {
	pvtCartesianFixed
	pvtTrailer
}

// BlockNumber returns the SBF block number.
func (*PVTGeodetic) BlockNumber() uint16 { return PVTGeodeticID }

// BlockNumber returns the SBF block number.
func (*PVTCartesian) BlockNumber() uint16 { return PVTCartesianID }

// Chunks returns the binary chunks for the block parameters.
func (p *PVTGeodetic) Chunks() func(yield func(chunk any) bool) {
	return p.pvtTrailer.chunks("PVTGeodetic", &p.pvtGeodeticFixed)
}

// Chunks returns the binary chunks for the block parameters.
func (p *PVTCartesian) Chunks() func(yield func(chunk any) bool) {
	return p.pvtTrailer.chunks("PVTCartesian", &p.pvtCartesianFixed)
}

// EndOfPVT is the SBF EndOfPVT block.
type EndOfPVT struct{}

// BlockNumber returns the SBF block number.
func (*EndOfPVT) BlockNumber() uint16 { return EndOfPVTID }

type posCovCartesian struct {
	Mode   Mode
	Error  ErrCode
	Cov_xx float32
	Cov_yy float32
	Cov_zz float32
	Cov_bb float32
	Cov_xy float32
	Cov_xz float32
	Cov_xb float32
	Cov_yz float32
	Cov_yb float32
	Cov_zb float32
}

// PosCovCartesian is the SBF PosCovCartesian block.
type PosCovCartesian struct {
	posCovCartesian
}

// BlockNumber returns the SBF block number.
func (*PosCovCartesian) BlockNumber() uint16 { return PosCovCartesianID }

type posCovGeodetic struct {
	Mode       Mode
	Error      ErrCode
	Cov_latlat float32
	Cov_lonlon float32
	Cov_hgthgt float32
	Cov_bb     float32
	Cov_latlon float32
	Cov_lathgt float32
	Cov_latb   float32
	Cov_lonhgt float32
	Cov_lonb   float32
	Cov_hb     float32
}

// PosCovGeodetic is the SBF PosCovGeodetic block.
type PosCovGeodetic struct {
	posCovGeodetic
}

// BlockNumber returns the SBF block number.
func (*PosCovGeodetic) BlockNumber() uint16 { return PosCovGeodeticID }

type velCovCartesian struct {
	Mode     Mode
	Error    ErrCode
	Cov_VxVx float32
	Cov_VyVy float32
	Cov_VzVz float32
	Cov_DtDt float32
	Cov_VxVy float32
	Cov_VxVz float32
	Cov_VxDt float32
	Cov_VyVz float32
	Cov_VyDt float32
	Cov_VzDt float32
}

// VelCovCartesian is the SBF VelCovCartesian block.
type VelCovCartesian struct {
	velCovCartesian
}

// BlockNumber returns the SBF block number.
func (*VelCovCartesian) BlockNumber() uint16 { return VelCovCartesianID }

type velCovGeodetic struct {
	Mode     Mode
	Error    ErrCode
	Cov_VnVn float32
	Cov_VeVe float32
	Cov_VuVu float32
	Cov_DtDt float32
	Cov_VnVe float32
	Cov_VnVu float32
	Cov_VnDt float32
	Cov_VeVu float32
	Cov_VeDt float32
	Cov_VuDt float32
}

// VelCovGeodetic is the SBF VelCovGeodetic block.
type VelCovGeodetic struct {
	velCovGeodetic
}

// BlockNumber returns the SBF block number.
func (*VelCovGeodetic) BlockNumber() uint16 { return VelCovGeodeticID }

// DOP is the SBF DOP block.
type DOP struct {
	NrSV     uint8
	Reserved uint8
	PDOP     uint16
	TDOP     uint16
	HDOP     uint16
	VDOP     uint16
	HPL      float32
	VPL      float32
}

// BlockNumber returns the SBF block number.
func (*DOP) BlockNumber() uint16 { return DOPID }

func init() {
	regBlock[PVTGeodetic]("PVTGeodetic")
	regBlock[PVTCartesian]("PVTCartesian")
	regBlock[EndOfPVT]("EndOfPVT")
	regBlock[PosCovCartesian]("PosCovCartesian")
	regBlock[PosCovGeodetic]("PosCovGeodetic")
	regBlock[VelCovCartesian]("VelCovCartesian")
	regBlock[VelCovGeodetic]("VelCovGeodetic")
	regBlock[DOP]("DOP")
}
