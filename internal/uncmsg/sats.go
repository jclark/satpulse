package uncmsg

import "github.com/jclark/satpulse/internal/novmsg"

// Message IDs for satellite-related messages
const (
	SatsInfoID MsgID = 2124
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
	PRN        byte   // Satellite PRN number
	Azimuth    uint16 // Azimuth (degrees)
	Elevation  byte   // Elevation (degrees)
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

func init() {
	// Register known message types
	regMsg[SatsInfo]("SATSINFO")
}
