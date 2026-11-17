package casbin

import "fmt"

const Rxm2MeasxID MsgID = clsRxm2 | (0x00 << 8)

// Rxm2TimeFlags is the receiver time status bitfield in RXM2-MEASX.
type Rxm2TimeFlags uint8

const (
	Rxm2TimeTOWValid Rxm2TimeFlags = 1 << iota
	Rxm2TimeWNValid
	Rxm2TimeLeapValid
	Rxm2TimeReliable
	Rxm2TimeClockJump
)

// Rxm2TrackingFlags is the per-measurement tracking status bitfield in
// RXM2-MEASX.
type Rxm2TrackingFlags uint8

const (
	Rxm2TrackingPRValid Rxm2TrackingFlags = 1 << iota
	Rxm2TrackingCPValid
	Rxm2TrackingHalfCycleValid
	Rxm2TrackingHalfCycleSubtracted
	Rxm2TrackingNoMSAmbiguity
	Rxm2TrackingEphemerisValid
	Rxm2TrackingElevationLow
)

// Rxm2MeasxFixed is the fixed epoch header of RXM2-MEASX (16 bytes).
type Rxm2MeasxFixed struct {
	RawTOW      uint32        `json:"rawTow"`      // ms, raw receiver time (integer part)
	RawTOWSubms int32         `json:"rawTowSubms"` // ms, fractional raw receiver time, scaled 2^-30
	Wn          uint16        `json:"wn"`          // GPS week number
	LeapSec     int8          `json:"leapS"`       // s, GPS-UTC leap seconds
	NumMeas     uint8         `json:"numMeas"`     // number of measurements
	TimeFlags   Rxm2TimeFlags `json:"rvtFlagx"`    // receiver time status
	TSrc        Tim2TSrc      `json:"tSrc"`        // receiver time source
	_           uint8
	_           uint8
}

// Rxm2MeasxMeas is one raw signal measurement in RXM2-MEASX (32 bytes).
type Rxm2MeasxMeas struct {
	PRMes      float64           `json:"prMes"`      // m, pseudorange
	CPMes      float64           `json:"cpMes"`      // m, carrier phase
	CPRateMes  float32           `json:"cpRateMes"`  // m/s, carrier phase rate
	GNSSID     GNSSID            `json:"gnssid"`     // satellite system ID
	SVID       uint8             `json:"svid"`       // satellite ID
	SigID      SigID             `json:"sigid"`      // signal ID
	FreqID     uint8             `json:"freqid"`     // GLONASS frequency ID, [1,14] maps to [-7,+6]
	CPLockTime uint16            `json:"cpLockTime"` // ms, carrier phase lock time
	CNO        uint8             `json:"cn0"`        // dB-Hz, carrier-to-noise ratio
	PRRMS      uint8             `json:"prRms"`      // m, pseudorange tracking error
	DORMS      uint8             `json:"doRms"`      // dm/s, Doppler tracking error
	_          uint8             // reserved
	TrkFlags   Rxm2TrackingFlags `json:"trkind"` // satellite tracking status
	Chn        uint8             `json:"chn"`    // tracking channel number
}

// Rxm2Measx is RXM2-MEASX (0x13 0x00) - multi-GNSS raw measurements.
type Rxm2Measx struct {
	Rxm2MeasxFixed
	Meas []Rxm2MeasxMeas `json:"meas"`
}

func (m *Rxm2Measx) ID() MsgID { return Rxm2MeasxID }

func (m *Rxm2Measx) InitVaryingPart(payloadLen int) error {
	n, err := sliceLen(m, payloadLen, 16, 32)
	if err != nil {
		return err
	}
	if n != int(m.NumMeas) {
		return fmt.Errorf("bad CASIC-%v measurement count (%d, payload contains %d)", m.ID(), m.NumMeas, n)
	}
	m.Meas = make([]Rxm2MeasxMeas, n)
	return nil
}

func (m *Rxm2Measx) FixedPart() any   { return &m.Rxm2MeasxFixed }
func (m *Rxm2Measx) VaryingPart() any { return &m.Meas }

var _ VaryingMsg = (*Rxm2Measx)(nil)

func init() {
	regMsg[Rxm2Measx]("MEASX")
}
