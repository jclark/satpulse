package uncmsg

import "fmt"

// SolStatus represents the solution status in Unicore BESTNAV/BESTNAVXYZ messages
type SolStatus uint32

const (
	SolComputed     SolStatus = 0
	InsufficientObs SolStatus = 1
	NoConvergence   SolStatus = 2
	Singularity     SolStatus = 3
	CovTrace        SolStatus = 4
)

// String returns the ASCII representation of SolStatus
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
	default:
		return fmt.Sprintf("%d", s)
	}
}

// ParseSolStatus converts an ASCII solution status string to SolStatus enum
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
	default:
		return 0, fmt.Errorf("unknown solution status: %s", s)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (s *SolStatus) UnmarshalText(text []byte) error {
	val, err := ParseSolStatus(string(text))
	if err != nil {
		return err
	}
	*s = val
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (s SolStatus) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// PosVelType represents the position or velocity type in Unicore BESTNAV/BESTNAVXYZ messages
type PosVelType uint32

const (
	PosVelNone            PosVelType = 0
	PosVelFixedPos        PosVelType = 1
	PosVelFixedHeight     PosVelType = 2
	PosVelDopplerVelocity PosVelType = 8
	PosVelSingle          PosVelType = 16
	PosVelPSRDiff         PosVelType = 17
	PosVelSBAS            PosVelType = 18
	PosVelL1Float         PosVelType = 32
	PosVelIonoFreeFloat   PosVelType = 33
	PosVelNarrowFloat     PosVelType = 34
	PosVelL1Int           PosVelType = 48
	PosVelWideInt         PosVelType = 49
	PosVelNarrowInt       PosVelType = 50
	PosVelINS             PosVelType = 52
	PosVelINSPSRSP        PosVelType = 53
	PosVelINSPSRDiff      PosVelType = 54
	PosVelINSRTKFloat     PosVelType = 55
	PosVelINSRTKFixed     PosVelType = 56
	PosVelPPPConverging   PosVelType = 68
	PosVelPPP             PosVelType = 69
	PosVelPPPAR           PosVelType = 70
	PosVelPPPRTK          PosVelType = 71
)

// String returns the ASCII representation of PosVelType
func (p PosVelType) String() string {
	switch p {
	case PosVelNone:
		return "NONE"
	case PosVelFixedPos:
		return "FIXEDPOS"
	case PosVelFixedHeight:
		return "FIXEDHEIGHT"
	case PosVelDopplerVelocity:
		return "DOPPLER_VELOCITY"
	case PosVelSingle:
		return "SINGLE"
	case PosVelPSRDiff:
		return "PSRDIFF"
	case PosVelSBAS:
		return "SBAS"
	case PosVelL1Float:
		return "L1_FLOAT"
	case PosVelIonoFreeFloat:
		return "IONOFREE_FLOAT"
	case PosVelNarrowFloat:
		return "NARROW_FLOAT"
	case PosVelL1Int:
		return "L1_INT"
	case PosVelWideInt:
		return "WIDE_INT"
	case PosVelNarrowInt:
		return "NARROW_INT"
	case PosVelINS:
		return "INS"
	case PosVelINSPSRSP:
		return "INS_PSRSP"
	case PosVelINSPSRDiff:
		return "INS_PSRDIFF"
	case PosVelINSRTKFloat:
		return "INS_RTKFLOAT"
	case PosVelINSRTKFixed:
		return "INS_RTKFIXED"
	case PosVelPPPConverging:
		return "PPP_CONVERGING"
	case PosVelPPP:
		return "PPP"
	case PosVelPPPAR:
		return "PPP_AR"
	case PosVelPPPRTK:
		return "PPP_RTK"
	default:
		return fmt.Sprintf("%d", p)
	}
}

// ParsePosVelType converts an ASCII position/velocity type string to PosVelType enum
func ParsePosVelType(s string) (PosVelType, error) {
	switch s {
	case "NONE":
		return PosVelNone, nil
	case "FIXEDPOS":
		return PosVelFixedPos, nil
	case "FIXEDHEIGHT":
		return PosVelFixedHeight, nil
	case "DOPPLER_VELOCITY":
		return PosVelDopplerVelocity, nil
	case "SINGLE":
		return PosVelSingle, nil
	case "PSRDIFF":
		return PosVelPSRDiff, nil
	case "SBAS":
		return PosVelSBAS, nil
	case "L1_FLOAT":
		return PosVelL1Float, nil
	case "IONOFREE_FLOAT":
		return PosVelIonoFreeFloat, nil
	case "NARROW_FLOAT":
		return PosVelNarrowFloat, nil
	case "L1_INT":
		return PosVelL1Int, nil
	case "WIDE_INT":
		return PosVelWideInt, nil
	case "NARROW_INT":
		return PosVelNarrowInt, nil
	case "INS":
		return PosVelINS, nil
	case "INS_PSRSP":
		return PosVelINSPSRSP, nil
	case "INS_PSRDIFF":
		return PosVelINSPSRDiff, nil
	case "INS_RTKFLOAT":
		return PosVelINSRTKFloat, nil
	case "INS_RTKFIXED":
		return PosVelINSRTKFixed, nil
	case "PPP_CONVERGING":
		return PosVelPPPConverging, nil
	case "PPP", "ppp":
		return PosVelPPP, nil
	case "PPP_AR":
		return PosVelPPPAR, nil
	case "PPP_RTK":
		return PosVelPPPRTK, nil
	default:
		return 0, fmt.Errorf("unknown position/velocity type: %s", s)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (p *PosVelType) UnmarshalText(text []byte) error {
	val, err := ParsePosVelType(string(text))
	if err != nil {
		return err
	}
	*p = val
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (p PosVelType) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

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
	return []byte(fmt.Sprintf("%02x", uint8(h))), nil
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
	// Find the length (exclude trailing zero bytes)
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
