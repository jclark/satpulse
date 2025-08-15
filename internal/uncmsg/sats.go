package uncmsg

// Message IDs for satellite-related messages
const (
	SatsInfoID MsgID = 2124
)

// SatsInfo represents detailed information for all tracked satellites
type SatsInfo struct {
	SatsInfoInitChunk
	Sats []SatsInfoSat
}

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
	Freqs []SatsInfoFreq
}

// SatsInfoSatChunk represents information for a single satellite in SATSINFO
type SatsInfoSatChunk struct {
	PRN        byte   // Satellite PRN number
	Azimuth    uint16 // Azimuth (degrees)
	Elevation  byte   // Elevation (degrees)
}

// SatsInfoFreq represents information for a single frequency in a satellite
type SatsInfoFreq struct {
	SysStatus  byte
	SNR        byte
	FreqStatus byte
	FreqNo     byte // Number of frequencies for this PRN
}

// ID returns the message ID for SATSINFO
func (s *SatsInfo) ID() (MsgID, string) {
	return SatsInfoID, "SATSINFOA"
}

// Chunks implements the ChunkedMsg interface for SatsInfo
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
