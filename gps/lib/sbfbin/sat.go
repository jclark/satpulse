package sbfbin

import "encoding/binary"

type RiseSet uint8
type SatelliteInfo uint8
type SlotStatus uint16

const (
	SatVisibilityAzimuthDNU   = 65535
	SatVisibilityElevationDNU = -32768
	SatVisibilityRiseSetDNU   = 255
	SatelliteInfoAlmanac      = 1
	SatelliteInfoEphemeris    = 2
	SatelliteInfoUnknown      = 255
	ChannelElevationDNU       = -128
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
