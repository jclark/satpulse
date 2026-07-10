package sbfbin

import "encoding/binary"

type RiseSet uint8
type SatelliteInfo uint8
type SlotStatus uint16

const (
	SatVisibilityAzimuthDNU   = 0xFFFF
	SatVisibilityElevationDNU = -32768
	SatVisibilityRiseSetDNU   = 0xFF
	SatelliteInfoAlmanac      = 1
	SatelliteInfoEphemeris    = 2
	SatelliteInfoUnknown      = 0xFF
	ChannelElevationDNU       = -128
)

// ChannelStatus 2-bit slot-status values, guide ChannelStatus tables.
const (
	TrackStatusIdle     uint8 = 0
	TrackStatusSearch   uint8 = 1
	TrackStatusSync     uint8 = 2
	TrackStatusTracking uint8 = 3

	PVTStatusNotUsed  uint8 = 0
	PVTStatusWaitEph  uint8 = 1
	PVTStatusUsed     uint8 = 2
	PVTStatusRejected uint8 = 3
)

// Slot returns the 2-bit status field for signal slot i (0-7) of a
// ChannelStatus HealthStatus/TrackingStatus/PVTStatus sequence.
func (s SlotStatus) Slot(i int) uint8 {
	return uint8(s>>(2*i)) & 0x3
}

// SVID range boundaries, guide sec 4.1.9. The ranges are inclusive. Several
// constellations occupy two disjoint ranges.
const (
	SVIDGPSMin     uint16 = 1
	SVIDGPSMax     uint16 = 37
	SVIDGLOMin     uint16 = 38
	SVIDGLOMax     uint16 = 61
	SVIDGLOUnknown uint16 = 62
	SVIDGLOMin2    uint16 = 63
	SVIDGLOMax2    uint16 = 68
	SVIDGALMin     uint16 = 71
	SVIDGALMax     uint16 = 106
	SVIDLBandMin   uint16 = 107
	SVIDLBandMax   uint16 = 119
	SVIDSBASMin    uint16 = 120
	SVIDSBASMax    uint16 = 140
	SVIDBDSMin     uint16 = 141
	SVIDBDSMax     uint16 = 180
	SVIDQZSSMin    uint16 = 181
	SVIDQZSSMax    uint16 = 190
	SVIDNavICMin   uint16 = 191
	SVIDNavICMax   uint16 = 197
	SVIDSBASMin2   uint16 = 198
	SVIDSBASMax2   uint16 = 215
	SVIDNavICMin2  uint16 = 216
	SVIDNavICMax2  uint16 = 222
	SVIDBDSMin2    uint16 = 223
	SVIDBDSMax2    uint16 = 245
	SVIDGPSExtMin  uint16 = 250
	SVIDGPSExtMax  uint16 = 251
)

type satVisibilityHead struct {
	N        uint8
	SBLength uint8
}

type satInfoBase struct {
	SVID          uint8
	FreqNr        uint8
	Azimuth       uint16
	Elevation     int16
	RiseSet       RiseSet
	SatelliteInfo SatelliteInfo
}

type satInfoRev1 struct {
	SVIDFull uint16
}

// SatInfo is a SatVisibility sub-block.
type SatInfo struct {
	satInfoBase
	satInfoRev1
}

// SatVisibility is the SBF SatVisibility block.
type SatVisibility struct {
	satVisibilityHead
	SatInfo []SatInfo
}

// BlockNumber returns the SBF block number.
func (*SatVisibility) BlockNumber() uint16 { return SatVisibilityID }

// Chunks returns the binary chunks for the block parameters.
func (s *SatVisibility) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		if err := setCount(&s.N, len(s.SatInfo), "SatVisibility SatInfo"); err != nil {
			yield(err)
			return
		}
		if s.N > 0 && s.SBLength == 0 {
			s.SBLength = uint8(binary.Size(SatInfo{}))
		}
		if !yield(&s.satVisibilityHead) {
			return
		}
		if len(s.SatInfo) != int(s.N) {
			s.SatInfo = make([]SatInfo, int(s.N))
		}
		baseLen := binary.Size(satInfoBase{})
		rev1Len := binary.Size(satInfoRev1{})
		for i := range s.SatInfo {
			if !yield(&s.SatInfo[i].satInfoBase) {
				return
			}
			used := baseLen
			if int(s.SBLength) >= baseLen+rev1Len {
				if !yield(&s.SatInfo[i].satInfoRev1) {
					return
				}
				used += rev1Len
			}
			if !yield(paddingChunk(int(s.SBLength) - used)) {
				return
			}
		}
	}
}

type channelStatusHead struct {
	N         uint8
	SB1Length uint8
	SB2Length uint8
	Reserved  [3]uint8
}

// ChannelSatInfo is a ChannelStatus satellite sub-block.
type ChannelSatInfo struct {
	SVID           uint8
	FreqNr         uint8
	SVIDFull       uint16
	AzimuthRiseSet uint16
	HealthStatus   SlotStatus
	Elevation      int8
	N2             uint8
	RxChannel      uint8
	Reserved2      uint8
}

// ChannelAzimuthDNU is the do-not-use value of the 9-bit azimuth subfield of
// ChannelSatInfo.AzimuthRiseSet.
const ChannelAzimuthDNU uint16 = 511

// Azimuth returns the whole-degree azimuth (bits 0-8) of a ChannelSatInfo and
// whether it is available (not the do-not-use sentinel).
func (s *ChannelSatInfo) Azimuth() (uint16, bool) {
	az := s.AzimuthRiseSet & 0x1FF
	if az == ChannelAzimuthDNU {
		return 0, false
	}
	return az, true
}

// ChannelStateInfo is a ChannelStatus state sub-block.
type ChannelStateInfo struct {
	Antenna        uint8
	Reserved       uint8
	TrackingStatus SlotStatus
	PVTStatus      SlotStatus
	PVTInfo        uint16
}

// ChannelStatus is the SBF ChannelStatus block.
type ChannelStatus struct {
	channelStatusHead
	SatInfo   []ChannelSatInfo
	StateInfo [][]ChannelStateInfo
}

// BlockNumber returns the SBF block number.
func (*ChannelStatus) BlockNumber() uint16 { return ChannelStatusID }

// Chunks returns the binary chunks for the block parameters.
func (c *ChannelStatus) Chunks() func(yield func(chunk any) bool) {
	return twoLevelChunks(&c.N, &c.SB1Length, &c.SB2Length, &c.channelStatusHead, &c.SatInfo, &c.StateInfo,
		func(s *ChannelSatInfo) uint8 { return s.N2 },
		func(s *ChannelSatInfo, n uint8) { s.N2 = n })
}

func init() {
	regBlock[SatVisibility]("SatVisibility")
	regBlock[ChannelStatus]("ChannelStatus")
}
