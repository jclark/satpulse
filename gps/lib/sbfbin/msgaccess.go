package sbfbin

// This file exports the wire-level constants, masks, and accessors that the
// gpsprot conversion layer (gps/internal/septentrio) needs, so that layer
// never inlines a raw SBF wire value of its own.

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

// WACorrInfo base-type codes, encoded in bits 5-6 of WACorrInfo.
const (
	WACorrBaseNone     uint8 = 0 // no differential/wide-area info used
	WACorrBasePhysical uint8 = 1 // physical base station (RTK)
	WACorrBaseVirtual  uint8 = 2 // virtual base station (VRS)
	WACorrBaseSSR      uint8 = 3 // state-space (RTK-SSR / PPP-RTK)
)

// BaseType returns the base-type code (bits 5-6) of WACorrInfo.
func (w WACorrInfo) BaseType() uint8 {
	return uint8(w>>5) & 0x3
}

// IsSBAS reports whether the fix is SBAS-aided, in which case ReferenceID is
// an SBAS PRN rather than an RTCM base ID.
func (m Mode) IsSBAS() bool {
	return m.Fix() == ModeSBAS
}

// CommonFlagsE6BUsed is the MeasEpoch CommonFlags bit indicating that the
// Galileo E6 measurement (signal number 19) is the E6-B component; when clear
// it is E6-C.
const CommonFlagsE6BUsed CommonFlags = 1 << 6

// E6BUsed reports whether the E6-B-used bit is set.
func (f CommonFlags) E6BUsed() bool {
	return f&CommonFlagsE6BUsed != 0
}

// Observed-axis signal numbers, guide sec 4.1.10. These index both
// MeasEpoch's SigIdxLo field and the PVT SignalInfo bitmask.
const (
	SigNumGPSL1CA         uint8 = 0
	SigNumGPSL1P          uint8 = 1
	SigNumGPSL2P          uint8 = 2
	SigNumGPSL2C          uint8 = 3
	SigNumGPSL5           uint8 = 4
	SigNumGPSL1C          uint8 = 5
	SigNumQZSSL1CA        uint8 = 6
	SigNumQZSSL2C         uint8 = 7
	SigNumGLONASSL1CA     uint8 = 8
	SigNumGLONASSL1P      uint8 = 9
	SigNumGLONASSL2P      uint8 = 10
	SigNumGLONASSL2CA     uint8 = 11
	SigNumGLONASSL3       uint8 = 12
	SigNumBeiDouB1C       uint8 = 13
	SigNumBeiDouB2a       uint8 = 14
	SigNumNavICL5         uint8 = 15
	SigNumGalileoE1       uint8 = 17
	SigNumGalileoE6       uint8 = 19
	SigNumGalileoE5a      uint8 = 20
	SigNumGalileoE5b      uint8 = 21
	SigNumGalileoE5AltBOC uint8 = 22
	SigNumLBand           uint8 = 23
	SigNumSBASL1CA        uint8 = 24
	SigNumSBASL5          uint8 = 25
	SigNumQZSSL5          uint8 = 26
	SigNumQZSSL6          uint8 = 27
	SigNumBeiDouB1I       uint8 = 28
	SigNumBeiDouB2I       uint8 = 29
	SigNumBeiDouB3I       uint8 = 30
	SigNumQZSSL1C         uint8 = 32
	SigNumQZSSL1S         uint8 = 33
	SigNumBeiDouB2b       uint8 = 34
	SigNumNavICL1         uint8 = 37
	SigNumQZSSL1CB        uint8 = 38
	SigNumQZSSL5S         uint8 = 39
)

// SignalNumber returns the observed signal number for a MeasEpoch master
// channel, resolving the SigIdxLo==31 extension via ObsInfo bits 3-7.
func (t *MeasEpochChannelType1) SignalNumber() uint8 {
	return measSignalNumber(t.Type, t.ObsInfo)
}

// SignalNumber returns the observed signal number for a MeasEpoch slave
// channel, resolving the SigIdxLo==31 extension via ObsInfo bits 3-7.
func (t *MeasEpochChannelType2) SignalNumber() uint8 {
	return measSignalNumber(t.Type, t.ObsInfo)
}

// AntennaID returns the AntennaID field of a MeasEpoch master channel.
func (t *MeasEpochChannelType1) AntennaID() uint8 {
	return measAntennaID(t.Type)
}

// AntennaID returns the AntennaID field of a MeasEpoch slave channel.
func (t *MeasEpochChannelType2) AntennaID() uint8 {
	return measAntennaID(t.Type)
}

func measSignalNumber(typ MeasType, obs ObsInfo) uint8 {
	n := uint8(typ) & 0x1F
	if n == MeasSigIdxExtension {
		return 32 + (uint8(obs)>>3)&0x1F
	}
	return n
}

func measAntennaID(typ MeasType) uint8 {
	return uint8(typ) >> 5
}

// CN0dBHz returns the C/N0 in dB-Hz for a MeasEpoch master channel, applying
// the +10 dB offset that all signal numbers except GPS L1P/L2P use. It reports
// false when the CN0 field is at its do-not-use sentinel.
func (t *MeasEpochChannelType1) CN0dBHz() (float64, bool) {
	return measCN0dBHz(t.CN0, t.SignalNumber())
}

// CN0dBHz returns the C/N0 in dB-Hz for a MeasEpoch slave channel.
func (t *MeasEpochChannelType2) CN0dBHz() (float64, bool) {
	return measCN0dBHz(t.CN0, t.SignalNumber())
}

func measCN0dBHz(cn0, sig uint8) (float64, bool) {
	if cn0 == MeasType1CN0DNU {
		return 0, false
	}
	v := float64(cn0) * 0.25
	if sig != SigNumGPSL1P && sig != SigNumGPSL2P {
		v += 10
	}
	return v, true
}

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

// TOWDNU and WNcDNU are the do-not-use sentinels of the block-header
// timestamp.
const (
	TOWDNU uint32 = 0xFFFFFFFF
	WNcDNU uint16 = 0xFFFF
)

// BaseSourceRTCM is the BaseStation.Source value for coordinates decoded from
// an RTCM3 1005/1006 message.
const BaseSourceRTCM BaseSource = 8

// CovDNU is the do-not-use sentinel of the PosCov*/VelCov* covariance fields
// (f4, exactly representable, safe for == comparison).
const CovDNU = -2e10

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
