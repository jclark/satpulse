package as

import "github.com/jclark/satpulse/internal/gpsprot"

// NewNMEASVNumbering returns a slice of NMEASVNumberingRange describing the NMEA satellite numbering scheme used by Allystar.
func NewNMEASVNumbering() []gpsprot.NMEASVNumberingRange {
	return []gpsprot.NMEASVNumberingRange{
		{MinID: 33, MaxID: 64, MinNum: 120, GNSS: gpsprot.SBAS, SignalID: ""},
		{MinID: 65, MaxID: 96, MinNum: 1, GNSS: gpsprot.GLO, SignalID: ""},
		{MinID: 193, MaxID: 199, MinNum: 1, GNSS: gpsprot.QZSS, SignalID: ""},
		{MinID: 201, MaxID: 263, MinNum: 1, GNSS: gpsprot.BDS, SignalID: ""},
		{MinID: 301, MaxID: 336, MinNum: 1, GNSS: gpsprot.GAL, SignalID: ""},
		{MinID: 401, MaxID: 432, MinNum: 1, GNSS: gpsprot.GPS, SignalID: gpsprot.SigIDGPSL1C},
		{MinID: 501, MaxID: 532, MinNum: 1, GNSS: gpsprot.GPS, SignalID: gpsprot.SigIDGPSL2CM},
		{MinID: 565, MaxID: 596, MinNum: 1, GNSS: gpsprot.GLO, SignalID: gpsprot.SigIDGLOL2},
		{MinID: 601, MaxID: 663, MinNum: 1, GNSS: gpsprot.BDS, SignalID: gpsprot.SigIDBDSB1C},
		{MinID: 651, MaxID: 682, MinNum: 1, GNSS: gpsprot.GPS, SignalID: gpsprot.SigIDGPSL5I},
		{MinID: 701, MaxID: 763, MinNum: 1, GNSS: gpsprot.BDS, SignalID: gpsprot.SigIDBDSB2I},
		{MinID: 801, MaxID: 863, MinNum: 1, GNSS: gpsprot.BDS, SignalID: gpsprot.SigIDBDSB3I},
		{MinID: 843, MaxID: 849, MinNum: 1, GNSS: gpsprot.QZSS, SignalID: gpsprot.SigIDQZSSL5I},
		{MinID: 851, MaxID: 913, MinNum: 1, GNSS: gpsprot.BDS, SignalID: gpsprot.SigIDBDSB2a},
		{MinID: 901, MaxID: 917, MinNum: 1, GNSS: gpsprot.NAVIC, SignalID: gpsprot.SigIDNAVICL5},
		{MinID: 951, MaxID: 986, MinNum: 1, GNSS: gpsprot.GAL, SignalID: gpsprot.SigIDGALE5a},
	}
}
