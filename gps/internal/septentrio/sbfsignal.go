package septentrio

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// sigEntry maps an SBF observed-axis signal number to its constellation, the
// best-fit gpsprot.SignalID for the component the receiver tracks, and its
// frequency band.
type sigEntry struct {
	gnss gpsprot.GNSS
	id   gpsprot.SignalID
	band gpsprot.Band
}

// sbfSignalTable is the observed-axis master table (guide sec 4.1.10). The
// SignalID is the physically-tracked component; band is used for BandsUsed.
// Signal number 19 (Galileo E6) defaults to E6-C; the E6-B alternative is
// resolved dynamically from MeasEpoch.CommonFlags. Signal number 22
// (E5AltBOC) has no gpsprot.SignalID yet (gap), so only its GNSS/band apply.
var sbfSignalTable = map[uint8]sigEntry{
	sbfbin.SigNumGPSL1CA:         {gpsprot.GPS, gpsprot.SigIDGPSL1CA, gpsprot.BandL1},
	sbfbin.SigNumGPSL1P:          {gpsprot.GPS, gpsprot.SigIDGPSL1PY, gpsprot.BandL1},
	sbfbin.SigNumGPSL2P:          {gpsprot.GPS, gpsprot.SigIDGPSL2P, gpsprot.BandL2},
	sbfbin.SigNumGPSL2C:          {gpsprot.GPS, gpsprot.SigIDGPSL2CL, gpsprot.BandL2},
	sbfbin.SigNumGPSL5:           {gpsprot.GPS, gpsprot.SigIDGPSL5Q, gpsprot.BandL5},
	sbfbin.SigNumGPSL1C:          {gpsprot.GPS, gpsprot.SigIDGPSL1CP, gpsprot.BandL1},
	sbfbin.SigNumQZSSL1CA:        {gpsprot.QZSS, gpsprot.SigIDQZSSL1CA, gpsprot.BandL1},
	sbfbin.SigNumQZSSL2C:         {gpsprot.QZSS, gpsprot.SigIDQZSSL2CL, gpsprot.BandL2},
	sbfbin.SigNumGLONASSL1CA:     {gpsprot.GLO, gpsprot.SigIDGLOL1, gpsprot.BandL1},
	sbfbin.SigNumGLONASSL1P:      {gpsprot.GLO, gpsprot.SigIDGLOL1P, gpsprot.BandL1},
	sbfbin.SigNumGLONASSL2P:      {gpsprot.GLO, gpsprot.SigIDGLOL2P, gpsprot.BandL2},
	sbfbin.SigNumGLONASSL2CA:     {gpsprot.GLO, gpsprot.SigIDGLOL2, gpsprot.BandL2},
	sbfbin.SigNumGLONASSL3:       {gpsprot.GLO, gpsprot.SigIDGLOL3Q, gpsprot.BandE5b},
	sbfbin.SigNumBeiDouB1C:       {gpsprot.BDS, gpsprot.SigIDBDSB1CP, gpsprot.BandL1},
	sbfbin.SigNumBeiDouB2a:       {gpsprot.BDS, gpsprot.SigIDBDSB2aP, gpsprot.BandL5},
	sbfbin.SigNumNavICL5:         {gpsprot.NAVIC, gpsprot.SigIDNAVICL5, gpsprot.BandL5},
	sbfbin.SigNumGalileoE1:       {gpsprot.GAL, gpsprot.SigIDGALE1C, gpsprot.BandL1},
	sbfbin.SigNumGalileoE6:       {gpsprot.GAL, gpsprot.SigIDGALE6C, gpsprot.BandE6},
	sbfbin.SigNumGalileoE5a:      {gpsprot.GAL, gpsprot.SigIDGALE5aQ, gpsprot.BandL5},
	sbfbin.SigNumGalileoE5b:      {gpsprot.GAL, gpsprot.SigIDGALE5bQ, gpsprot.BandE5b},
	sbfbin.SigNumGalileoE5AltBOC: {gpsprot.GAL, gpsprot.SigIDInvalid, gpsprot.BandL5 | gpsprot.BandE5b},
	sbfbin.SigNumSBASL1CA:        {gpsprot.SBAS, gpsprot.SigIDGPSL1CA, gpsprot.BandL1},
	sbfbin.SigNumSBASL5:          {gpsprot.SBAS, gpsprot.SigIDGPSL5I, gpsprot.BandL5},
	sbfbin.SigNumQZSSL5:          {gpsprot.QZSS, gpsprot.SigIDQZSSL5Q, gpsprot.BandL5},
	sbfbin.SigNumQZSSL6:          {gpsprot.QZSS, gpsprot.SigIDQZSSL6, gpsprot.BandE6},
	sbfbin.SigNumBeiDouB1I:       {gpsprot.BDS, gpsprot.SigIDBDSB1I, gpsprot.BandL1},
	sbfbin.SigNumBeiDouB2I:       {gpsprot.BDS, gpsprot.SigIDBDSB2I, gpsprot.BandE5b},
	sbfbin.SigNumBeiDouB3I:       {gpsprot.BDS, gpsprot.SigIDBDSB3I, gpsprot.BandE6},
	sbfbin.SigNumQZSSL1C:         {gpsprot.QZSS, gpsprot.SigIDQZSSL1CP, gpsprot.BandL1},
	sbfbin.SigNumQZSSL1S:         {gpsprot.QZSS, gpsprot.SigIDQZSSL1S, gpsprot.BandL1},
	sbfbin.SigNumBeiDouB2b:       {gpsprot.BDS, gpsprot.SigIDBDSB2bI, gpsprot.BandE5b},
	sbfbin.SigNumNavICL1:         {gpsprot.NAVIC, gpsprot.SigIDNAVICL1, gpsprot.BandL1},
	sbfbin.SigNumQZSSL1CB:        {gpsprot.QZSS, gpsprot.SigIDQZSSL1CB, gpsprot.BandL1},
	sbfbin.SigNumQZSSL5S:         {gpsprot.QZSS, gpsprot.SigIDQZSSL5S, gpsprot.BandL5},
}

// sbfSignalNumber looks up an observed-axis signal number.
func sbfSignalNumber(n uint8) (sigEntry, bool) {
	e, ok := sbfSignalTable[n]
	return e, ok
}

// measEpochSignalID returns the precise SignalID for a MeasEpoch signal
// number, resolving the Galileo E6 component from CommonFlags.
func measEpochSignalID(n uint8, common sbfbin.CommonFlags) (gpsprot.SignalID, bool) {
	e, ok := sbfSignalTable[n]
	if !ok || e.id == gpsprot.SigIDInvalid {
		return "", false
	}
	if n == sbfbin.SigNumGalileoE6 && common.E6BUsed() {
		return gpsprot.SigIDGALE6B, true
	}
	return e.id, true
}

// famSlot is one ChannelStatus family bit-slot: the coarse family-level
// SignalID and the MeasEpoch signal number it joins to for CN0/precise-ID
// overlay. A zero id marks a reserved slot.
type famSlot struct {
	id     gpsprot.SignalID
	sigNum uint8
}

// noJoin marks a family slot with no MeasEpoch signal-number counterpart.
const noJoin uint8 = 255

// chanFamilies gives, per GNSS, the ChannelStatus bit-slot layout (slot index
// = position of the 2-bit field). It is both the family-level SignalID source
// and the join table to MeasEpoch signal numbers (guide sec 9.1/9.3).
var chanFamilies = map[gpsprot.GNSS][]famSlot{
	gpsprot.GPS: {
		{gpsprot.SigIDGPSL1CA, sbfbin.SigNumGPSL1CA},
		{gpsprot.SigIDGPSL1PY, sbfbin.SigNumGPSL1P},
		{gpsprot.SigIDGPSL2P, sbfbin.SigNumGPSL2P},
		{gpsprot.SigIDGPSL2CL, sbfbin.SigNumGPSL2C},
		{gpsprot.SigIDGPSL5Q, sbfbin.SigNumGPSL5},
		{gpsprot.SigIDGPSL1CP, sbfbin.SigNumGPSL1C},
	},
	gpsprot.GLO: {
		{gpsprot.SigIDGLOL1, sbfbin.SigNumGLONASSL1CA},
		{gpsprot.SigIDGLOL1P, sbfbin.SigNumGLONASSL1P},
		{gpsprot.SigIDGLOL2P, sbfbin.SigNumGLONASSL2P},
		{gpsprot.SigIDGLOL2, sbfbin.SigNumGLONASSL2CA},
		{gpsprot.SigIDGLOL3Q, sbfbin.SigNumGLONASSL3},
	},
	gpsprot.GAL: {
		{gpsprot.SigIDInvalid, noJoin},
		{gpsprot.SigIDGALE1, sbfbin.SigNumGalileoE1},
		{gpsprot.SigIDInvalid, noJoin},
		{gpsprot.SigIDGALE6, sbfbin.SigNumGalileoE6},
		{gpsprot.SigIDGALE5a, sbfbin.SigNumGalileoE5a},
		{gpsprot.SigIDGALE5b, sbfbin.SigNumGalileoE5b},
		{gpsprot.SigIDGALE5, sbfbin.SigNumGalileoE5AltBOC},
	},
	gpsprot.SBAS: {
		{gpsprot.SigIDGPSL1CA, sbfbin.SigNumSBASL1CA},
		{gpsprot.SigIDGPSL5I, sbfbin.SigNumSBASL5},
	},
	gpsprot.BDS: {
		{gpsprot.SigIDBDSB1I, sbfbin.SigNumBeiDouB1I},
		{gpsprot.SigIDBDSB2I, sbfbin.SigNumBeiDouB2I},
		{gpsprot.SigIDBDSB3I, sbfbin.SigNumBeiDouB3I},
		{gpsprot.SigIDBDSB1C, sbfbin.SigNumBeiDouB1C},
		{gpsprot.SigIDBDSB2a, sbfbin.SigNumBeiDouB2a},
		{gpsprot.SigIDBDSB2b, sbfbin.SigNumBeiDouB2b},
	},
	gpsprot.QZSS: {
		{gpsprot.SigIDQZSSL1CA, sbfbin.SigNumQZSSL1CA},
		{gpsprot.SigIDQZSSL2CL, sbfbin.SigNumQZSSL2C},
		{gpsprot.SigIDQZSSL5Q, sbfbin.SigNumQZSSL5},
		{gpsprot.SigIDQZSSL6, sbfbin.SigNumQZSSL6},
		{gpsprot.SigIDQZSSL1CP, sbfbin.SigNumQZSSL1C},
		{gpsprot.SigIDQZSSL1S, sbfbin.SigNumQZSSL1S},
		{gpsprot.SigIDQZSSL1CB, sbfbin.SigNumQZSSL1CB},
		{gpsprot.SigIDQZSSL5S, sbfbin.SigNumQZSSL5S},
	},
	gpsprot.NAVIC: {
		{gpsprot.SigIDNAVICL5, sbfbin.SigNumNavICL5},
		{gpsprot.SigIDNAVICL1, sbfbin.SigNumNavICL1},
	},
}

// sbfSVID maps an SBF SVID (guide sec 4.1.9) to a gpsprot.SVID. freqNr (the
// GLONASS FDMA channel number, offset +8) only disambiguates the unknown-slot
// case. It reports false for non-satellite SVIDs (L-band beams, unmapped).
func sbfSVID(svid uint16) (gpsprot.SVID, bool) {
	switch {
	case svid >= sbfbin.SVIDGPSMin && svid <= sbfbin.SVIDGPSMax:
		return gpsprot.SVID{GNSS: gpsprot.GPS, Num: uint8(svid)}, true
	case svid >= sbfbin.SVIDGLOMin && svid <= sbfbin.SVIDGLOMax:
		return gpsprot.SVID{GNSS: gpsprot.GLO, Num: uint8(svid - 37)}, true
	case svid == sbfbin.SVIDGLOUnknown:
		return gpsprot.SVID{GNSS: gpsprot.GLO, Num: gpsprot.GLOUnknown}, true
	case svid >= sbfbin.SVIDGLOMin2 && svid <= sbfbin.SVIDGLOMax2:
		return gpsprot.SVID{GNSS: gpsprot.GLO, Num: uint8(svid - 38)}, true
	case svid >= sbfbin.SVIDGALMin && svid <= sbfbin.SVIDGALMax:
		return gpsprot.SVID{GNSS: gpsprot.GAL, Num: uint8(svid - 70)}, true
	case svid >= sbfbin.SVIDLBandMin && svid <= sbfbin.SVIDLBandMax:
		return gpsprot.SVID{}, false // L-band beam, not a GNSS SV
	case svid >= sbfbin.SVIDSBASMin && svid <= sbfbin.SVIDSBASMax:
		return gpsprot.SVID{GNSS: gpsprot.SBAS, Num: uint8(svid - 100)}, true
	case svid >= sbfbin.SVIDBDSMin && svid <= sbfbin.SVIDBDSMax:
		return gpsprot.SVID{GNSS: gpsprot.BDS, Num: uint8(svid - 140)}, true
	case svid >= sbfbin.SVIDQZSSMin && svid <= sbfbin.SVIDQZSSMax:
		return gpsprot.SVID{GNSS: gpsprot.QZSS, Num: uint8(svid - 180)}, true
	case svid >= sbfbin.SVIDNavICMin && svid <= sbfbin.SVIDNavICMax:
		return gpsprot.SVID{GNSS: gpsprot.NAVIC, Num: uint8(svid - 190)}, true
	case svid >= sbfbin.SVIDSBASMin2 && svid <= sbfbin.SVIDSBASMax2:
		return gpsprot.SVID{GNSS: gpsprot.SBAS, Num: uint8(svid - 157)}, true
	case svid >= sbfbin.SVIDNavICMin2 && svid <= sbfbin.SVIDNavICMax2:
		return gpsprot.SVID{GNSS: gpsprot.NAVIC, Num: uint8(svid - 208)}, true
	case svid >= sbfbin.SVIDBDSMin2 && svid <= sbfbin.SVIDBDSMax2:
		return gpsprot.SVID{GNSS: gpsprot.BDS, Num: uint8(svid - 182)}, true
	case svid >= sbfbin.SVIDGPSExtMin && svid <= sbfbin.SVIDGPSExtMax:
		return gpsprot.SVID{GNSS: gpsprot.GPS, Num: uint8(svid - 212)}, true
	}
	return gpsprot.SVID{}, false
}
