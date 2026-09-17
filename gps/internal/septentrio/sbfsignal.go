package septentrio

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// sigEntry maps an SBF observed-axis signal number to its constellation and
// frequency band.
type sigEntry struct {
	gnss gpsprot.GNSS
	band gpsprot.Band
}

// sbfSignalTable is the observed-axis master table (guide sec 4.1.10). The
// table is used for PVT SignalInfo summary bits, not MeasEpoch signal
// identity.
var sbfSignalTable = map[uint8]sigEntry{
	sbfbin.SigNumGPSL1CA:         {gpsprot.GPS, gpsprot.BandL1},
	sbfbin.SigNumGPSL1P:          {gpsprot.GPS, gpsprot.BandL1},
	sbfbin.SigNumGPSL2P:          {gpsprot.GPS, gpsprot.BandL2},
	sbfbin.SigNumGPSL2C:          {gpsprot.GPS, gpsprot.BandL2},
	sbfbin.SigNumGPSL5:           {gpsprot.GPS, gpsprot.BandL5},
	sbfbin.SigNumGPSL1C:          {gpsprot.GPS, gpsprot.BandL1},
	sbfbin.SigNumQZSSL1CA:        {gpsprot.QZSS, gpsprot.BandL1},
	sbfbin.SigNumQZSSL2C:         {gpsprot.QZSS, gpsprot.BandL2},
	sbfbin.SigNumGLONASSL1CA:     {gpsprot.GLO, gpsprot.BandL1},
	sbfbin.SigNumGLONASSL1P:      {gpsprot.GLO, gpsprot.BandL1},
	sbfbin.SigNumGLONASSL2P:      {gpsprot.GLO, gpsprot.BandL2},
	sbfbin.SigNumGLONASSL2CA:     {gpsprot.GLO, gpsprot.BandL2},
	sbfbin.SigNumGLONASSL3:       {gpsprot.GLO, gpsprot.BandE5b},
	sbfbin.SigNumBeiDouB1C:       {gpsprot.BDS, gpsprot.BandL1},
	sbfbin.SigNumBeiDouB2a:       {gpsprot.BDS, gpsprot.BandL5},
	sbfbin.SigNumNavICL5:         {gpsprot.NAVIC, gpsprot.BandL5},
	sbfbin.SigNumGalileoE1:       {gpsprot.GAL, gpsprot.BandL1},
	sbfbin.SigNumGalileoE6:       {gpsprot.GAL, gpsprot.BandE6},
	sbfbin.SigNumGalileoE5a:      {gpsprot.GAL, gpsprot.BandL5},
	sbfbin.SigNumGalileoE5b:      {gpsprot.GAL, gpsprot.BandE5b},
	sbfbin.SigNumGalileoE5AltBOC: {gpsprot.GAL, gpsprot.BandL5 | gpsprot.BandE5b},
	sbfbin.SigNumSBASL1CA:        {gpsprot.SBAS, gpsprot.BandL1},
	sbfbin.SigNumSBASL5:          {gpsprot.SBAS, gpsprot.BandL5},
	sbfbin.SigNumQZSSL5:          {gpsprot.QZSS, gpsprot.BandL5},
	sbfbin.SigNumQZSSL6:          {gpsprot.QZSS, gpsprot.BandE6},
	sbfbin.SigNumBeiDouB1I:       {gpsprot.BDS, gpsprot.BandL1},
	sbfbin.SigNumBeiDouB2I:       {gpsprot.BDS, gpsprot.BandE5b},
	sbfbin.SigNumBeiDouB3I:       {gpsprot.BDS, gpsprot.BandE6},
	sbfbin.SigNumQZSSL1C:         {gpsprot.QZSS, gpsprot.BandL1},
	sbfbin.SigNumQZSSL1S:         {gpsprot.QZSS, gpsprot.BandL1},
	sbfbin.SigNumBeiDouB2b:       {gpsprot.BDS, gpsprot.BandE5b},
	sbfbin.SigNumNavICL1:         {gpsprot.NAVIC, gpsprot.BandL1},
	sbfbin.SigNumQZSSL1CB:        {gpsprot.QZSS, gpsprot.BandL1},
	sbfbin.SigNumQZSSL5S:         {gpsprot.QZSS, gpsprot.BandL5},
}

// sbfSignalNumber looks up an observed-axis signal number.
func sbfSignalNumber(n uint8) (sigEntry, bool) {
	e, ok := sbfSignalTable[n]
	return e, ok
}

// sbfSVID maps an SBF SVID (guide sec 4.1.9) to a gpsprot.SVID. It reports
// false for non-satellite SVIDs (L-band beams, unmapped).
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
