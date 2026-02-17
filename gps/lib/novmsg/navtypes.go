package novmsg

import "fmt"

// DatumID represents a geodetic datum identifier.
// Binary: uint32 (61 = WGS84). ASCII: symbolic name (e.g. "WGS84").
type DatumID uint32

const DatumWGS84 DatumID = 61

// String returns the ASCII representation of DatumID
func (d DatumID) String() string {
	if d == DatumWGS84 {
		return "WGS84"
	}
	return fmt.Sprintf("%d", d)
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (d *DatumID) UnmarshalText(text []byte) error {
	if string(text) == "WGS84" {
		*d = DatumWGS84
		return nil
	}
	return fmt.Errorf("unknown datum: %s", text)
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (d DatumID) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// HexByte is a uint8 that serializes as 2-digit lowercase hex in ASCII.
// Used for bitmask fields like ExtSolStat, GalBDS3Sig, GPSGLOBDS2Sig.
type HexByte uint8

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (h *HexByte) UnmarshalText(text []byte) error {
	var v uint64
	_, err := fmt.Sscanf(string(text), "%x", &v)
	if err != nil {
		return fmt.Errorf("invalid hex byte: %s", text)
	}
	*h = HexByte(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (h HexByte) MarshalText() ([]byte, error) {
	return fmt.Appendf(nil, "%02x", uint8(h)), nil
}

// StationID represents a base station ID, stored as [4]byte in binary
// and as a quoted string (e.g. "") in ASCII.
type StationID [4]byte

// UnmarshalText implements encoding.TextUnmarshaler.
// Strips surrounding quotes: "" -> empty, "0" -> "0".
func (s *StationID) UnmarshalText(text []byte) error {
	t := string(text)
	if len(t) >= 2 && t[0] == '"' && t[len(t)-1] == '"' {
		t = t[1 : len(t)-1]
	}
	*s = StationID{}
	copy(s[:], t)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
// Returns the station ID as a quoted string.
func (s StationID) MarshalText() ([]byte, error) {
	n := len(s)
	for n > 0 && s[n-1] == 0 {
		n--
	}
	out := make([]byte, 0, n+2)
	out = append(out, '"')
	out = append(out, s[:n]...)
	out = append(out, '"')
	return out, nil
}

// SolStatus represents the solution status in NovAtel OEM7/ByNav messages.
type SolStatus uint32

const (
	SolComputed      SolStatus = 0
	InsufficientObs  SolStatus = 1
	NoConvergence    SolStatus = 2
	Singularity      SolStatus = 3
	CovTrace         SolStatus = 4
	TestDist         SolStatus = 5
	ColdStart        SolStatus = 6
	VHLimit          SolStatus = 7
	Variance         SolStatus = 8
	Residuals        SolStatus = 9
	IntegrityWarning SolStatus = 13
	Pending          SolStatus = 18
	InvalidFix       SolStatus = 19
	Unauthorized     SolStatus = 20
	InvalidRate      SolStatus = 22
)

// String returns the ASCII representation of SolStatus.
func (s SolStatus) String() string {
	switch s {
	case SolComputed:
		return "SOL_COMPUTED"
	case InsufficientObs:
		return "INSUFFICIENT_OBS"
	case NoConvergence:
		return "NO_CONVERGENCE"
	case Singularity:
		return "SINGULARITY"
	case CovTrace:
		return "COV_TRACE"
	case TestDist:
		return "TEST_DIST"
	case ColdStart:
		return "COLD_START"
	case VHLimit:
		return "V_H_LIMIT"
	case Variance:
		return "VARIANCE"
	case Residuals:
		return "RESIDUALS"
	case IntegrityWarning:
		return "INTEGRITY_WARNING"
	case Pending:
		return "PENDING"
	case InvalidFix:
		return "INVALID_FIX"
	case Unauthorized:
		return "UNAUTHORIZED"
	case InvalidRate:
		return "INVALID_RATE"
	default:
		return fmt.Sprintf("%d", s)
	}
}

// ParseSolStatus converts an ASCII solution status string to SolStatus.
func ParseSolStatus(s string) (SolStatus, error) {
	switch s {
	case "SOL_COMPUTED":
		return SolComputed, nil
	case "INSUFFICIENT_OBS":
		return InsufficientObs, nil
	case "NO_CONVERGENCE":
		return NoConvergence, nil
	case "SINGULARITY":
		return Singularity, nil
	case "COV_TRACE":
		return CovTrace, nil
	case "TEST_DIST":
		return TestDist, nil
	case "COLD_START":
		return ColdStart, nil
	case "V_H_LIMIT":
		return VHLimit, nil
	case "VARIANCE":
		return Variance, nil
	case "RESIDUALS":
		return Residuals, nil
	case "INTEGRITY_WARNING":
		return IntegrityWarning, nil
	case "PENDING":
		return Pending, nil
	case "INVALID_FIX":
		return InvalidFix, nil
	case "UNAUTHORIZED":
		return Unauthorized, nil
	case "INVALID_RATE":
		return InvalidRate, nil
	default:
		return 0, fmt.Errorf("unknown solution status: %s", s)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support.
func (s *SolStatus) UnmarshalText(text []byte) error {
	val, err := ParseSolStatus(string(text))
	if err != nil {
		return err
	}
	*s = val
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support.
func (s SolStatus) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// PosType represents the position or velocity type in NovAtel OEM7/ByNav messages.
type PosType uint32

const (
	PosNone                  PosType = 0
	PosFixedPos              PosType = 1
	PosFixedHeight           PosType = 2
	PosFloatConv             PosType = 4
	PosWideLane              PosType = 5
	PosNarrowLane            PosType = 6
	PosDopplerVelocity       PosType = 8
	PosSingle                PosType = 16
	PosPSRDiff               PosType = 17
	PosWAAS                  PosType = 18
	PosPropagated            PosType = 19
	PosL1Float               PosType = 32
	PosIonoFreeFloat         PosType = 33
	PosNarrowFloat           PosType = 34
	PosL1Int                 PosType = 48
	PosWideInt               PosType = 49
	PosNarrowInt             PosType = 50
	PosRTKDirectINS          PosType = 51
	PosINSSBAS               PosType = 52
	PosINSPSRSP              PosType = 53
	PosINSPSRDiff            PosType = 54
	PosINSRTKFloat           PosType = 55
	PosINSRTKFixed           PosType = 56
	PosExtConstrained        PosType = 67
	PosPPPConverging         PosType = 68
	PosPPP                   PosType = 69
	PosOperational           PosType = 70
	PosWarning               PosType = 71
	PosOutOfBounds           PosType = 72
	PosINSPPPConverging      PosType = 73
	PosINSPPP                PosType = 74
	PosPPPBasicConverging    PosType = 77
	PosPPPBasic              PosType = 78
	PosINSPPPBasicConverging PosType = 79
	PosINSPPPBasic           PosType = 80
)

// String returns the ASCII representation of PosType.
func (p PosType) String() string {
	switch p {
	case PosNone:
		return "NONE"
	case PosFixedPos:
		return "FIXEDPOS"
	case PosFixedHeight:
		return "FIXEDHEIGHT"
	case PosFloatConv:
		return "FLOATCONV"
	case PosWideLane:
		return "WIDELANE"
	case PosNarrowLane:
		return "NARROWLANE"
	case PosDopplerVelocity:
		return "DOPPLER_VELOCITY"
	case PosSingle:
		return "SINGLE"
	case PosPSRDiff:
		return "PSRDIFF"
	case PosWAAS:
		return "WAAS"
	case PosPropagated:
		return "PROPAGATED"
	case PosL1Float:
		return "L1_FLOAT"
	case PosIonoFreeFloat:
		return "IONOFREE_FLOAT"
	case PosNarrowFloat:
		return "NARROW_FLOAT"
	case PosL1Int:
		return "L1_INT"
	case PosWideInt:
		return "WIDE_INT"
	case PosNarrowInt:
		return "NARROW_INT"
	case PosRTKDirectINS:
		return "RTK_DIRECT_INS"
	case PosINSSBAS:
		return "INS_SBAS"
	case PosINSPSRSP:
		return "INS_PSRSP"
	case PosINSPSRDiff:
		return "INS_PSRDIFF"
	case PosINSRTKFloat:
		return "INS_RTKFLOAT"
	case PosINSRTKFixed:
		return "INS_RTKFIXED"
	case PosExtConstrained:
		return "EXT_CONSTRAINED"
	case PosPPPConverging:
		return "PPP_CONVERGING"
	case PosPPP:
		return "PPP"
	case PosOperational:
		return "OPERATIONAL"
	case PosWarning:
		return "WARNING"
	case PosOutOfBounds:
		return "OUT_OF_BOUNDS"
	case PosINSPPPConverging:
		return "INS_PPP_CONVERGING"
	case PosINSPPP:
		return "INS_PPP"
	case PosPPPBasicConverging:
		return "PPP_BASIC_CONVERGING"
	case PosPPPBasic:
		return "PPP_BASIC"
	case PosINSPPPBasicConverging:
		return "INS_PPP_BASIC_CONVERGING"
	case PosINSPPPBasic:
		return "INS_PPP_BASIC"
	default:
		return fmt.Sprintf("%d", p)
	}
}

// ParsePosType converts an ASCII position/velocity type string to PosType.
func ParsePosType(s string) (PosType, error) {
	switch s {
	case "NONE":
		return PosNone, nil
	case "FIXEDPOS":
		return PosFixedPos, nil
	case "FIXEDHEIGHT":
		return PosFixedHeight, nil
	case "FLOATCONV":
		return PosFloatConv, nil
	case "WIDELANE":
		return PosWideLane, nil
	case "NARROWLANE":
		return PosNarrowLane, nil
	case "DOPPLER_VELOCITY":
		return PosDopplerVelocity, nil
	case "SINGLE":
		return PosSingle, nil
	case "PSRDIFF":
		return PosPSRDiff, nil
	case "WAAS":
		return PosWAAS, nil
	case "PROPAGATED":
		return PosPropagated, nil
	case "L1_FLOAT":
		return PosL1Float, nil
	case "IONOFREE_FLOAT":
		return PosIonoFreeFloat, nil
	case "NARROW_FLOAT":
		return PosNarrowFloat, nil
	case "L1_INT":
		return PosL1Int, nil
	case "WIDE_INT":
		return PosWideInt, nil
	case "NARROW_INT":
		return PosNarrowInt, nil
	case "RTK_DIRECT_INS":
		return PosRTKDirectINS, nil
	case "INS_SBAS":
		return PosINSSBAS, nil
	case "INS_PSRSP":
		return PosINSPSRSP, nil
	case "INS_PSRDIFF":
		return PosINSPSRDiff, nil
	case "INS_RTKFLOAT":
		return PosINSRTKFloat, nil
	case "INS_RTKFIXED":
		return PosINSRTKFixed, nil
	case "EXT_CONSTRAINED":
		return PosExtConstrained, nil
	case "PPP_CONVERGING":
		return PosPPPConverging, nil
	case "PPP":
		return PosPPP, nil
	case "OPERATIONAL":
		return PosOperational, nil
	case "WARNING":
		return PosWarning, nil
	case "OUT_OF_BOUNDS":
		return PosOutOfBounds, nil
	case "INS_PPP_CONVERGING":
		return PosINSPPPConverging, nil
	case "INS_PPP":
		return PosINSPPP, nil
	case "PPP_BASIC_CONVERGING":
		return PosPPPBasicConverging, nil
	case "PPP_BASIC":
		return PosPPPBasic, nil
	case "INS_PPP_BASIC_CONVERGING":
		return PosINSPPPBasicConverging, nil
	case "INS_PPP_BASIC":
		return PosINSPPPBasic, nil
	default:
		return 0, fmt.Errorf("unknown position/velocity type: %s", s)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support.
func (p *PosType) UnmarshalText(text []byte) error {
	val, err := ParsePosType(string(text))
	if err != nil {
		return err
	}
	*p = val
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support.
func (p PosType) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// Pos contains the geodetic position fields shared between NovAtel BESTPOS
// and Unicore BESTNAV. The binary layout is identical across vendors;
// the SolStatus (S) and PosType (P) enum types vary by vendor.
type Pos[S, P ~uint32] struct {
	PSolStatus    S         // position solution status
	PosType       P         // position type
	Lat           float64   // latitude (degrees)
	Lon           float64   // longitude (degrees)
	Hgt           float64   // height above MSL (meters)
	Undulation    float32   // geoid undulation (meters)
	DatumID       DatumID   // datum ID (61 = WGS84)
	LatSigma      float32   // latitude std dev (meters)
	LonSigma      float32   // longitude std dev (meters)
	HgtSigma      float32   // height std dev (meters)
	StnID         StationID // base station ID
	DiffAge       float32   // differential data age (seconds)
	SolAge        float32   // solution age (seconds)
	NumSVs        uint8     // satellites tracked
	NumSolnSVs    uint8     // satellites in solution
	NumSolnL1SVs  uint8     // satellites with L1/E1/B1 signals in solution
	NumSolnMulti  uint8     // satellites with multi-frequency in solution
	Reserved      uint8     // reserved
	ExtSolStat    HexByte   // extended solution status
	GalBDS3Sig    HexByte   // Galileo/BDS3 signal mask
	GPSGLOBDS2Sig HexByte   // GPS/GLONASS/BDS2 signal mask
}

// XYZ contains the ECEF position+velocity fields shared between NovAtel
// BESTXYZ (ID 241) and Unicore BESTNAVXYZ (ID 240). The binary layout is
// identical across vendors; the SolStatus (S) and PosType/VelType (P) enum
// types vary by vendor.
type XYZ[S, P ~uint32] struct {
	PSolStatus    S         // position solution status
	PosType       P         // position type
	PX            float64   // ECEF X (meters)
	PY            float64   // ECEF Y (meters)
	PZ            float64   // ECEF Z (meters)
	PXSigma       float32   // X std dev (meters)
	PYSigma       float32   // Y std dev (meters)
	PZSigma       float32   // Z std dev (meters)
	VSolStatus    S         // velocity solution status
	VelType       P         // velocity type
	VX            float64   // ECEF VX (m/s)
	VY            float64   // ECEF VY (m/s)
	VZ            float64   // ECEF VZ (m/s)
	VXSigma       float32   // VX std dev (m/s)
	VYSigma       float32   // VY std dev (m/s)
	VZSigma       float32   // VZ std dev (m/s)
	StnID         StationID // base station ID
	VLatency      float32   // velocity latency (seconds)
	DiffAge       float32   // differential age (seconds)
	SolAge        float32   // solution age (seconds)
	NumSVs        uint8     // satellites tracked
	NumSolnSVs    uint8     // satellites in solution
	NumSolnL1SVs  uint8     // satellites with L1/E1/B1 signals in solution
	NumSolnMulti  uint8     // satellites with multi-frequency in solution
	Reserved      uint8     // reserved
	ExtSolStat    HexByte   // extended solution status
	GalBDS3Sig    HexByte   // Galileo/BDS3 signal mask
	GPSGLOBDS2Sig HexByte   // GPS/GLONASS/BDS2 signal mask
}
