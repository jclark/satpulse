package rtcmbin

// Meter is the number of RTCM length units per meter (0.0001 m resolution).
const Meter = 10000

// MT1005 represents an RTCM 1005 Stationary RTK Reference Station ARP message.
// ECEF coordinates are in units of 0.0001 m.
type MT1005 struct {
	MsgHdr
	StationID    uint16 `bits:"12" json:"stationID"`
	ITRFYear     uint8  `bits:"6" json:"itrfYear"`
	GPS          bool   `bits:"1" json:"gps"`
	GLONASS      bool   `bits:"1" json:"glonass"`
	Galileo      bool   `bits:"1" json:"galileo"`
	RefStation   bool   `bits:"1" json:"refStation"`
	ECEFX        int64  `bits:"38" json:"ecefX"`
	SingleOsc    bool   `bits:"1" json:"singleOsc"`
	Reserved     uint8  `bits:"1" json:"-"`
	ECEFY        int64  `bits:"38" json:"ecefY"`
	QuarterCycle uint8  `bits:"2" json:"quarterCycle"`
	ECEFZ        int64  `bits:"38" json:"ecefZ"`
}

// ECEF returns the antenna reference point in meters.
func (m *MT1005) ECEF() [3]float64 {
	return [3]float64{float64(m.ECEFX) / Meter, float64(m.ECEFY) / Meter, float64(m.ECEFZ) / Meter}
}

// MT1006 represents an RTCM 1006 Stationary RTK Reference Station ARP
// with Antenna Height message. ECEF coordinates and antenna height are
// in units of 0.0001 m.
type MT1006 struct {
	MT1005
	AntennaHeight uint16 `bits:"16" json:"antennaHeight"`
}
