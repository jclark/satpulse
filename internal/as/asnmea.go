package as

import "github.com/jclark/satpulse/internal/gpsprot"

// NewNMEASVNumbering returns a slice of NMEASVNumberingRange describing the NMEA satellite numbering scheme used by Allystar.
func NewNMEASVNumbering() []gpsprot.NMEASVNumberingRange {
	return []gpsprot.NMEASVNumberingRange{
		{MinID: 33, MaxID: 64, MinPRN: 120, GNSS: gpsprot.SBAS, SignalID: ""},
		{MinID: 65, MaxID: 96, MinPRN: 1, GNSS: gpsprot.GLO, SignalID: ""},
		{MinID: 193, MaxID: 199, MinPRN: 1, GNSS: gpsprot.QZSS, SignalID: ""},
		{MinID: 201, MaxID: 263, MinPRN: 1, GNSS: gpsprot.BDS, SignalID: ""},
		{MinID: 301, MaxID: 336, MinPRN: 1, GNSS: gpsprot.GAL, SignalID: ""},
		{MinID: 401, MaxID: 432, MinPRN: 1, GNSS: gpsprot.GPS, SignalID: "L1C"},
		{MinID: 501, MaxID: 532, MinPRN: 1, GNSS: gpsprot.GPS, SignalID: "L2CM"},
		{MinID: 565, MaxID: 596, MinPRN: 1, GNSS: gpsprot.GLO, SignalID: "G2"},
		{MinID: 601, MaxID: 663, MinPRN: 1, GNSS: gpsprot.BDS, SignalID: "B1C"},
		{MinID: 651, MaxID: 682, MinPRN: 1, GNSS: gpsprot.GPS, SignalID: "L5"},
		{MinID: 701, MaxID: 763, MinPRN: 1, GNSS: gpsprot.BDS, SignalID: "B2I"},
		{MinID: 801, MaxID: 863, MinPRN: 1, GNSS: gpsprot.BDS, SignalID: "B3I"},
		{MinID: 843, MaxID: 849, MinPRN: 1, GNSS: gpsprot.QZSS, SignalID: "L5"},
		{MinID: 851, MaxID: 913, MinPRN: 1, GNSS: gpsprot.BDS, SignalID: "B2A"},
		{MinID: 901, MaxID: 917, MinPRN: 1, GNSS: gpsprot.NAVIC, SignalID: "L5"},
		{MinID: 951, MaxID: 986, MinPRN: 1, GNSS: gpsprot.GAL, SignalID: "E5A"},
	}
}
