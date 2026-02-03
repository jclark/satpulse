package ubx

import "github.com/jclark/satpulse/gps/gpsprot"

// NewNMEASVNumbering returns a slice of NMEASVNumberingRange describing the NMEA satellite numbering scheme used by u-blox.
func NewNMEASVNumbering() []gpsprot.NMEASVNumberingRange {
	return []gpsprot.NMEASVNumberingRange{
		{MinID: 33, MaxID: 64, MinNum: 120, GNSS: gpsprot.SBAS, SignalID: ""},
		{MinID: 65, MaxID: 96, MinNum: 1, GNSS: gpsprot.GLO, SignalID: ""},
		{MinID: 152, MaxID: 158, MinNum: 152, GNSS: gpsprot.SBAS, SignalID: ""},
		{MinID: 193, MaxID: 202, MinNum: 1, GNSS: gpsprot.QZSS, SignalID: ""},
		{MinID: 247, MaxID: 253, MinNum: 1, GNSS: gpsprot.NAVIC, SignalID: ""},
		{MinID: 301, MaxID: 336, MinNum: 1, GNSS: gpsprot.GAL, SignalID: ""},
		{MinID: 401, MaxID: 463, MinNum: 1, GNSS: gpsprot.BDS, SignalID: ""},
	}
}
