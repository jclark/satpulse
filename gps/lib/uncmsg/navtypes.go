package uncmsg

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/lib/novmsg"
)

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

// PPPService identifies the satellite-based PPP correction service indicated
// by a Unicore station ID. Unicore encodes these as 99xx (99 concatenated
// with satellite number) in the stn ID field of BESTNAV/BESTNAVXYZ/PPPNAV.
type PPPService uint8

const (
	PPPNone PPPService = iota
	PPPB2b             // BeiDou B2b PPP (stn ID 9959, 9960, 9961)
	PPPHAS             // Galileo E6 HAS (stn ID 9901)
	PPPMDC             // QZSS L6 MDC PPP (stn ID 9934, 9935, 9936, 9939)
)

// StnIDPPPService returns the PPP correction service for a Unicore station ID,
// or PPPNone if the station ID does not identify a PPP service.
func StnIDPPPService(s novmsg.StationID) PPPService {
	switch strings.TrimRight(string(s[:]), "\x00") {
	case "9959", "9960", "9961":
		return PPPB2b
	case "9901":
		return PPPHAS
	case "9934", "9935", "9936", "9939":
		return PPPMDC
	default:
		return PPPNone
	}
}

// Shared types aliased from novmsg (used in both NovAtel and Unicore protocols).
type DatumID = novmsg.DatumID
type HexByte = novmsg.HexByte
type StationID = novmsg.StationID

const DatumWGS84 = novmsg.DatumWGS84
