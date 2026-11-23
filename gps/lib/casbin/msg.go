package casbin

const (
	MsgBDSUTCID MsgID = clsMsg | (0x00 << 8)
	MsgGPSUTCID MsgID = clsMsg | (0x05 << 8)
)

// MsgBDSUTC is MSG-BDSUTC (0x08 0x00) - BDS UTC parameters (20 bytes)
type MsgBDSUTC struct {
	_     uint32      // reserved
	A0UTC int32       `json:"a0UTC"` // BDT clock bias, scale 2^-30 s
	A1UTC int32       `json:"a1UTC"` // BDT clock rate, scale 2^-50 s/s
	Dtls  int8        `json:"dtls"`  // Delta UTC before leap second (current leap seconds)
	Dtlsf int8        `json:"dtlsf"` // Delta UTC after leap second (future leap seconds)
	_     uint8       // reserved
	_     uint8       // reserved
	Wnlsf uint8       `json:"wnlsf"` // Week number of new leap second
	Dn    uint8       `json:"dn"`    // Day number of new leap second
	Valid MsgUTCValid `json:"valid"`
	_     uint8       // reserved
}

func (m *MsgBDSUTC) ID() MsgID { return MsgBDSUTCID }

// MsgGPSUTC is MSG-GPSUTC (0x08 0x05) - GPS UTC parameters (20 bytes)
type MsgGPSUTC struct {
	Tot   uint32      `json:"tot"`   // reference time for UTC parameters, scale 2^12 s
	A0UTC int32       `json:"a0UTC"` // GPS clock bias, scale 2^-30 s
	A1UTC int32       `json:"a1UTC"` // GPS clock rate, scale 2^-50 s/s
	Dtls  int8        `json:"dtls"`  // Delta UTC before leap second (current leap seconds)
	Dtlsf int8        `json:"dtlsf"` // Delta UTC after leap second (future leap seconds)
	Wnt   uint8       `json:"wnt"`   // reference week number
	_     uint8       // reserved
	Wnlsf uint8       `json:"wnlsf"` // Week number of new leap second
	Dn    uint8       `json:"dn"`    // Day number of new leap second
	Valid MsgUTCValid `json:"valid"`
	_     uint8       // reserved
}

func (m *MsgGPSUTC) ID() MsgID { return MsgGPSUTCID }

// MsgUTCValid indicates validity of UTC parameters
type MsgUTCValid int8

const (
	MsgUTCInvalid   MsgUTCValid = 0
	MsgUTCUnhealthy MsgUTCValid = 1
	MsgUTCExpired   MsgUTCValid = 2
	MsgUTCValidOK   MsgUTCValid = 3
)

func init() {
	regMsg[MsgBDSUTC]("BDSUTC")
	regMsg[MsgGPSUTC]("GPSUTC")
}
