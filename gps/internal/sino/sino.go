package sino

import "github.com/jclark/satpulse/gps/gpsprot"

// NewNMEASVNumbering returns the NMEA satellite numbering scheme for SinoGNSS receivers.
// I have observed GLONASS slot numbers > 24 (for spares),
// but this would overlap with the NavIC range, so we don't include them here.
func NewNMEASVNumbering() []gpsprot.NMEASVNumberingRange {
	return []gpsprot.NMEASVNumberingRange{
		{MinID: 38, MaxID: 61, MinNum: 1, GNSS: gpsprot.GLO, SignalID: ""},
		{MinID: 62, MaxID: 70, MinNum: 1, GNSS: gpsprot.NAVIC, SignalID: ""},
		{MinID: 71, MaxID: 106, MinNum: 1, GNSS: gpsprot.GAL, SignalID: ""},
		{MinID: 131, MaxID: 140, MinNum: 1, GNSS: gpsprot.QZSS, SignalID: ""},
		{MinID: 141, MaxID: 203, MinNum: 1, GNSS: gpsprot.BDS, SignalID: ""},
		{MinID: 220, MaxID: 238, MinNum: 20, GNSS: gpsprot.SBAS, SignalID: ""},
	}
}
