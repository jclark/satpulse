package uncmsg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/gps/lib/novmsg"
)

// Message IDs for satellite-related messages
const (
	SatsInfoID MsgID = 2124
	BestSatID  MsgID = 1041
)

// SysID represents the GNSS system identifier in Unicore messages
// Based on Table 7-112 in protocol.md
type SysID byte

// GNSS system ID constants from Unicore protocol
const (
	SysGPS SysID = iota
	SysGLO
	SysSBAS
	SysGAL
	SysBDS
	SysQZSS
	SysNAVIC
)

// FreqID represents the frequency identifier in Unicore messages
// Based on Table 7-113 in protocol.md
type FreqID byte

// Frequency ID constants from Unicore protocol (Table 7-113)
// GPS frequencies
const (
	FreqGPSL1CA FreqID = 0  // L1 C/A
	FreqGPSL2P  FreqID = 9  // L2P(Y)
	FreqGPSL1CP FreqID = 3  // L1C pilot
	FreqGPSL1CD FreqID = 11 // L1C data
	FreqGPSL5D  FreqID = 6  // L5 data
	FreqGPSL5P  FreqID = 14 // L5 pilot
	FreqGPSL2CL FreqID = 17 // L2C(L)
)

// GLONASS frequencies
const (
	FreqGLOL1CA FreqID = 0 // L1 C/A
	FreqGLOL2CA FreqID = 5 // L2 C/A
	FreqGLOG3I  FreqID = 6 // G3I
	FreqGLOG3Q  FreqID = 7 // G3Q
)

// Galileo frequencies
const (
	FreqGALE1B  FreqID = 1  // E1B
	FreqGALE1C  FreqID = 2  // E1C
	FreqGALE5AP FreqID = 12 // E5A pilot
	FreqGALE5BP FreqID = 17 // E5B pilot
	FreqGALE6B  FreqID = 18 // E6B
	FreqGALE6C  FreqID = 22 // E6C
)

// BeiDou frequencies
const (
	FreqBDSB1I  FreqID = 0  // B1I
	FreqBDSB1Q  FreqID = 4  // B1Q
	FreqBDSB1CP FreqID = 8  // B1C(Pilot)
	FreqBDSB1CD FreqID = 23 // B1C(Data)
	FreqBDSB2Q  FreqID = 5  // B2Q
	FreqBDSB2I  FreqID = 17 // B2I
	FreqBDSB2aP FreqID = 12 // B2a(Pilot)
	FreqBDSB2aD FreqID = 28 // B2a(Data)
	FreqBDSB3Q  FreqID = 6  // B3Q
	FreqBDSB3I  FreqID = 21 // B3I
	FreqBDSB2bI FreqID = 13 // B2b(I)
)

// QZSS frequencies
const (
	FreqQZSSL1CA FreqID = 0  // L1 C/A
	FreqQZSSL1CB FreqID = 1  // L1C/B
	FreqQZSSL1CP FreqID = 3  // L1C pilot
	FreqQZSSL1S  FreqID = 4  // L1S
	FreqQZSSL5D  FreqID = 6  // L5 data
	FreqQZSSL1CD FreqID = 11 // L1C data
	FreqQZSSL5P  FreqID = 14 // L5 pilot
	FreqQZSSL2CL FreqID = 17 // L2C(L)
	FreqQZSSL6D  FreqID = 21 // L6D
	FreqQZSSL6E  FreqID = 27 // L6E
)

// SBAS frequencies
const (
	FreqSBASL1CA FreqID = 0 // L1 C/A
	FreqSBASL5I  FreqID = 6 // L5(I)
)

// NavIC frequencies
const (
	FreqNAVICL5D FreqID = 6  // L5 data
	FreqNAVICL5P FreqID = 14 // L5 pilot
)

// SatsInfo represents detailed information for all tracked satellites
type SatsInfo struct {
	SatsInfoInitChunk
	Sats []SatsInfoSat
}

var _ novmsg.Chunked = (*SatsInfo)(nil)

type SatsInfoInitChunk struct {
	SatNumber     byte // Number of tracked satellites to follow
	VersionNumber byte // Version number, default = 2
	Reserved1     byte // Reserved
	Reserved2     byte // Reserved
	Reserved3     byte // Reserved
	FreqFlag      byte // Frequency flag bitmask for this satellite
}

type SatsInfoSat struct {
	SatsInfoSatChunk
	Freqs []SatsInfoFreq // length guaranteed > 0
}

// SatsInfoSatChunk represents information for a single satellite in SATSINFO
type SatsInfoSatChunk struct {
	PRN       byte   // Satellite PRN number
	Azimuth   uint16 // Azimuth (degrees)
	Elevation byte   // Elevation (degrees)
}

// SatsInfoFreq represents information for a single frequency in a satellite
type SatsInfoFreq struct {
	SysStatus  SysID
	SNR        byte
	FreqStatus FreqID
	FreqNo     byte // Number of frequencies for this PRN
}

// ID returns the message ID for SATSINFO
func (s *SatsInfo) ID() (MsgID, string) {
	return SatsInfoID, "SATSINFOA"
}

// Chunks implements the novmsg.Chunked interface for SatsInfo
func (s *SatsInfo) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		// First yield the header chunk to read satellite count
		if !yield(&s.SatsInfoInitChunk) {
			return
		}

		// Allocate the Sats slice based on SatNumber if not already allocated
		if len(s.Sats) == 0 {
			s.Sats = make([]SatsInfoSat, s.SatNumber)
		}

		// Read each satellite
		for i := range s.Sats {
			// Yield satellite chunk
			if !yield(&s.Sats[i].SatsInfoSatChunk) {
				return
			}

			// This is weird. The number of frequencies is in the per-frequency chunk.
			// And the manual says it is updated according to real-time calculation.
			// So I think we have to check each block until we have a complete set of frequencies,
			// according to FreqNo in the last per-frequency chunk.
			j := 0
			for {
				if j >= len(s.Sats[i].Freqs) {
					s.Sats[i].Freqs = append(s.Sats[i].Freqs, SatsInfoFreq{})
				}
				if !yield(&s.Sats[i].Freqs[j]) {
					return
				}
				freqNo := int(s.Sats[i].Freqs[j].FreqNo)
				j++
				if j >= freqNo {
					break
				}
			}
		}
	}
}

// BestSatInitChunk represents the initial chunk of BESTSAT message
type BestSatInitChunk struct {
	NumEntries uint32
}

// BestSat represents information about satellites used in the position solution
type BestSat struct {
	BestSatInitChunk
	Sats []BestSatEntry
}

var _ novmsg.Chunked = (*BestSat)(nil)

// BestSatEntry represents a single satellite entry in BESTSAT message
type BestSatEntry struct {
	System  SatSys    // GNSS satellite system (Table 7-116)
	SatID   SatID     // Satellite ID (PRN + frequency channel for GLONASS)
	Status  SatStatus // Satellite status (0 in binary, "GOOD" in ASCII)
	SigMask SigUsed   // Signal usage bitmask
}

// SatID represents satellite ID in BESTSAT message
// In binary: high 16 bits = PRN, low 16 bits = frequency channel (GLONASS only)
// In ASCII: "PRN" or "PRN+channel" for GLONASS
type SatID uint32

// PRN returns the satellite PRN number (high 16 bits)
func (id SatID) PRN() uint16 {
	return uint16(id >> 16)
}

// FreqChannel returns the frequency channel (low 16 bits, GLONASS only)
func (id SatID) FreqChannel() uint16 {
	return uint16(id & 0xFFFF)
}

// MakeSatID constructs a SatID from PRN and optional frequency channel
func MakeSatID(prn uint16, freqChan ...uint16) SatID {
	var freq uint16
	if len(freqChan) > 0 {
		freq = freqChan[0]
	}
	return SatID((uint32(prn) << 16) | uint32(freq))
}

// String returns the ASCII representation of SatID
func (id SatID) String() string {
	prn := id.PRN()
	freqChan := id.FreqChannel()

	if freqChan == 0 {
		return fmt.Sprintf("%d", prn)
	}
	return fmt.Sprintf("%d+%d", prn, freqChan)
}

// ParseSatID converts an ASCII satellite ID string to SatID
// Supports "PRN" and "PRN+channel" formats
func ParseSatID(s string) (SatID, error) {
	before, after, found := strings.Cut(s, "+")
	prn, err := strconv.ParseUint(before, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid satellite ID: %s", s)
	}
	var freqChan uint64
	if found {
		freqChan, err = strconv.ParseUint(after, 10, 16)
		if err != nil {
			return 0, fmt.Errorf("invalid frequency channel in satellite ID: %s", s)
		}
	}
	return MakeSatID(uint16(prn), uint16(freqChan)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (id *SatID) UnmarshalText(text []byte) error {
	val, err := ParseSatID(string(text))
	if err != nil {
		return err
	}
	*id = val
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (id SatID) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

// SatSys represents the GNSS satellite system in BESTSAT message
// Based on Table 7-116 in protocol.md
type SatSys uint32

// GNSS satellite system constants from Table 7-116
const (
	SatSysGPS     SatSys = 0 // GPS
	SatSysGLONASS SatSys = 1 // GLONASS
	SatSysSBAS    SatSys = 2 // SBAS
	SatSysGALILEO SatSys = 5 // GALILEO
	SatSysBEIDOU  SatSys = 6 // BEIDOU
	SatSysQZSS    SatSys = 7 // QZSS
	SatSysNAVIC   SatSys = 9 // NAVIC
)

// String returns the ASCII representation of SatSys
func (sys SatSys) String() string {
	switch sys {
	case SatSysGPS:
		return "GPS"
	case SatSysGLONASS:
		return "GLONASS"
	case SatSysSBAS:
		return "SBAS"
	case SatSysGALILEO:
		return "GALILEO"
	case SatSysBEIDOU:
		return "BEIDOU"
	case SatSysQZSS:
		return "QZSS"
	case SatSysNAVIC:
		return "NAVIC"
	default:
		return fmt.Sprintf("%d", uint32(sys))
	}
}

// ParseSatSys converts an ASCII satellite system string to SatSys enum
func ParseSatSys(s string) (SatSys, error) {
	switch s {
	case "GPS":
		return SatSysGPS, nil
	case "GLONASS":
		return SatSysGLONASS, nil
	case "SBAS":
		return SatSysSBAS, nil
	case "GALILEO":
		return SatSysGALILEO, nil
	case "BEIDOU":
		return SatSysBEIDOU, nil
	case "QZSS":
		return SatSysQZSS, nil
	case "NAVIC":
		return SatSysNAVIC, nil
	default:
		// Try to parse as number
		if n, err := strconv.ParseUint(s, 10, 32); err == nil {
			return SatSys(n), nil
		}
		return 0, fmt.Errorf("unknown satellite system: %s", s)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (sys *SatSys) UnmarshalText(text []byte) error {
	val, err := ParseSatSys(string(text))
	if err != nil {
		return err
	}
	*sys = val
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (sys SatSys) MarshalText() ([]byte, error) {
	return []byte(sys.String()), nil
}

// SatStatus represents the satellite status in BESTSAT message
type SatStatus uint32

const (
	SatStatusGood SatStatus = 0 // Satellite is good (0 in binary, "GOOD" in ASCII)
)

// SigUsed represents signal usage bitmasks in BESTSAT messages
type SigUsed uint32

// Common signal mask bit (bit 4) used across all constellations
const (
	SigUsedCommonView SigUsed = 1 << 4 // Satellite shared with base station
)

// GPS signal usage masks (Table 7-94)
const (
	SigUsedGPSL1 SigUsed = 1 << 0 // GPS L1 used in Solution
	SigUsedGPSL2 SigUsed = 1 << 1 // GPS L2 used in Solution
	SigUsedGPSL5 SigUsed = 1 << 2 // GPS L5 used in Solution
)

// GLONASS signal usage masks (Table 7-95)
const (
	SigUsedGLOL1 SigUsed = 1 << 0 // GLONASS L1 used in Solution
	SigUsedGLOL2 SigUsed = 1 << 1 // GLONASS L2 used in Solution
	SigUsedGLOL3 SigUsed = 1 << 2 // GLONASS L3 used in Solution
)

// BeiDou signal usage masks (Table 7-96)
const (
	SigUsedBDSB1 SigUsed = 1 << 0 // BeiDou B1 used in Solution
	SigUsedBDSB2 SigUsed = 1 << 1 // BeiDou B2 used in Solution
	SigUsedBDSB3 SigUsed = 1 << 2 // BeiDou B3 used in Solution
)

// Galileo signal usage masks (Table 7-97)
const (
	SigUsedGALE1     SigUsed = 1 << 0 // Galileo E1 used in Solution
	SigUsedGALE5A    SigUsed = 1 << 1 // Galileo E5A used in Solution
	SigUsedGALE5B    SigUsed = 1 << 2 // Galileo E5B used in Solution
	SigUsedGALALTBOC SigUsed = 1 << 3 // Galileo ALTBOC used in Solution
)

// String returns the hex representation of SigUsed (8 digits)
func (su SigUsed) String() string {
	return fmt.Sprintf("%08x", uint32(su))
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (su *SigUsed) UnmarshalText(text []byte) error {
	val, err := strconv.ParseUint(string(text), 16, 32)
	if err != nil {
		return err
	}
	*su = SigUsed(val)
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (su SigUsed) MarshalText() ([]byte, error) {
	return []byte(su.String()), nil
}

// String returns the ASCII representation of SatStatus
func (ss SatStatus) String() string {
	switch ss {
	case SatStatusGood:
		return "GOOD"
	default:
		return fmt.Sprintf("%d", uint32(ss))
	}
}

// ParseSatStatus converts an ASCII status string to SatStatus enum
func ParseSatStatus(s string) (SatStatus, error) {
	switch s {
	case "GOOD":
		return SatStatusGood, nil
	default:
		// Try to parse as number
		if n, err := strconv.ParseUint(s, 10, 32); err == nil {
			return SatStatus(n), nil
		}
		return 0, fmt.Errorf("unknown satellite status: %s", s)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (ss *SatStatus) UnmarshalText(text []byte) error {
	val, err := ParseSatStatus(string(text))
	if err != nil {
		return err
	}
	*ss = val
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (ss SatStatus) MarshalText() ([]byte, error) {
	return []byte(ss.String()), nil
}

// Chunks implements the novmsg.Chunked interface for BestSat
func (b *BestSat) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		// First yield the initial chunk to read satellite count
		if !yield(&b.BestSatInitChunk) {
			return
		}

		// Allocate the Sats slice based on NumEntries if not already allocated
		if len(b.Sats) == 0 && b.NumEntries > 0 {
			b.Sats = make([]BestSatEntry, b.NumEntries)
		}

		// Read each satellite entry (16 bytes each)
		for i := range b.Sats {
			if !yield(&b.Sats[i]) {
				return
			}
		}
	}
}

// ID returns the message ID for BESTSAT
func (b *BestSat) ID() (MsgID, string) {
	return BestSatID, "BESTSATA"
}

func init() {
	// Register known message types
	regMsg[SatsInfo]("SATSINFO")
	regMsg[BestSat]("BESTSAT")
}
