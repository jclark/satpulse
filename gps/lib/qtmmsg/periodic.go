// Package qtmmsg parses Quectel PQTM proprietary NMEA messages.
package qtmmsg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/gps/lib/fieldenc"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// PeriodicMsg is implemented by all known PQTM periodic output messages.
// These are messages whose output rate is controlled by PQTMCFGMSGRATE.
// The set of types implementing this interface is closed.
type PeriodicMsg interface {
	periodicMsg()        // private marker
	ID() (string, uint8) // name (e.g. "PVT") and message version
}

// decUint8 is a uint8 that parses as base-10, avoiding octal interpretation
// of NMEA zero-padded fields like "09".
type decUint8 uint8

func (v *decUint8) UnmarshalText(text []byte) error {
	n, err := strconv.ParseUint(string(text), 10, 8)
	if err != nil {
		return err
	}
	*v = decUint8(n)
	return nil
}

// skip discards a field value during parsing. Used for reserved PQTM fields.
type skip struct{}

func (*skip) UnmarshalText([]byte) error { return nil }

// PVT represents a PQTMPVT position/velocity/time message.
type PVT struct {
	TOW     uint32           // ms, GPS time of week
	Date    string           // yyyymmdd
	Time    string           // hhmmss.sss, UTC
	Res     opt.Val[string]  // reserved
	FixType uint8            // 0=no fix, 2=2D, 3=3D
	NumSV   decUint8         // satellites used
	LeapS   opt.Val[uint8]   // leap seconds
	Lat     opt.Val[float64] // deg
	Lon     opt.Val[float64] // deg
	Alt     opt.Val[float64] // m, above MSL
	Sep     opt.Val[float64] // m, geoidal separation
	VelN    opt.Val[float64] // m/s
	VelE    opt.Val[float64] // m/s
	VelD    opt.Val[float64] // m/s
	Spd     opt.Val[float64] // m/s
	Heading opt.Val[float64] // deg
	HDOP    opt.Val[float64]
	PDOP    opt.Val[float64]
}

func (*PVT) periodicMsg()        {}
func (*PVT) ID() (string, uint8) { return "PVT", 1 }

// VEL represents a PQTMVEL velocity message.
type VEL struct {
	Time       string           // hhmmss.sss, UTC
	VelN       opt.Val[float64] // m/s
	VelE       opt.Val[float64] // m/s
	VelD       opt.Val[float64] // m/s
	GrdSpd     opt.Val[float64] // m/s, 2D speed
	Spd        opt.Val[float64] // m/s, 3D speed
	COG        opt.Val[float64] // deg, course over ground
	GrdSpdAcc  opt.Val[float64] // m/s
	SpdAcc     opt.Val[float64] // m/s
	HeadingAcc opt.Val[float64] // deg
}

func (*VEL) periodicMsg()        {}
func (*VEL) ID() (string, uint8) { return "VEL", 1 }

// EPE represents a PQTMEPE estimated position error message.
type EPE struct {
	EPENorth opt.Val[float64] // m
	EPEEast  opt.Val[float64] // m
	EPEDown  opt.Val[float64] // m
	EPE2D    opt.Val[float64] // m
	EPE3D    opt.Val[float64] // m
}

func (*EPE) periodicMsg()        {}
func (*EPE) ID() (string, uint8) { return "EPE", 2 }

// DOP represents a PQTMDOP dilution of precision message.
type DOP struct {
	TOW  uint32 // ms, GPS time of week
	GDOP opt.Val[float64]
	PDOP opt.Val[float64]
	TDOP opt.Val[float64]
	VDOP opt.Val[float64]
	HDOP opt.Val[float64]
	NDOP opt.Val[float64]
	EDOP opt.Val[float64]
}

func (*DOP) periodicMsg()        {}
func (*DOP) ID() (string, uint8) { return "DOP", 1 }

// SVINStatus represents a PQTMSVINSTATUS survey-in status message.
type SVINStatus struct {
	TOW     uint32           // ms, GPS time of week
	Valid   uint8            // 0=invalid, 1=in-progress, 2=valid
	Res0    string           // reserved (hex)
	Obs     uint32           // position observations used
	CfgDur  uint32           // configured observation count
	MeanX   opt.Val[float64] // m, ECEF
	MeanY   opt.Val[float64] // m, ECEF
	MeanZ   opt.Val[float64] // m, ECEF
	MeanAcc opt.Val[float64] // m
}

func (*SVINStatus) periodicMsg()        {}
func (*SVINStatus) ID() (string, uint8) { return "SVINSTATUS", 1 }

// NAV represents a PQTMNAV navigation information message.
type NAV struct {
	TimeStatus uint8           // 0=invalid, 1=valid
	TimeRef    uint8           // 1=GPS
	UTC        string          // hhmmss.sss
	Date       string          // yyyymmdd
	TOW        opt.Val[uint32] // ms, GPS time of week
	WN         opt.Val[uint16] // GPS week number
	LeapSec    opt.Val[uint8]  // seconds
	Res0       skip
	Res1       skip
	SolType    opt.Val[uint8] // 0=none, 1=single, 2=SBAS, 5=DGPS, 8=RTK float, 12=RTK fixed
	Res2       skip
	Lat        opt.Val[float64] // deg
	Lon        opt.Val[float64] // deg
	Alt        opt.Val[float64] // m, above MSL
	Sep        opt.Val[float64] // m, geoidal separation
	Res3       skip
	Res4       skip
	LatStd     opt.Val[float64] // m
	LonStd     opt.Val[float64] // m
	AltStd     opt.Val[float64] // m
	Res5       skip
	Res6       skip
	DiffID     opt.Val[uint16]  // 0-4095
	DiffAge    opt.Val[float64] // seconds
	Res7       skip
	// SatView/SatUsed: docs say non-optional, but receiver sends empty before lock.
	SatView opt.Val[decUint8] // satellites in view
	SatUsed opt.Val[decUint8] // satellites in use
	Res8       skip
	Res9       skip
	Res10      skip
	Res11      skip
	Res12      skip
	Res13      skip
	HVel       opt.Val[float64] // m/s
	VVel       opt.Val[float64] // m/s, upward positive
	HVelStd    opt.Val[float64] // m/s
	VVelStd    opt.Val[float64] // m/s
	Res14      skip
	Res15      skip
	COG        opt.Val[float64] // deg, 0-360
	Res16      skip
	Res17      skip
}

func (*NAV) periodicMsg()        {}
func (*NAV) ID() (string, uint8) { return "NAV", 1 }

// PPPNAV represents a PQTMPPPNAV PPP navigation information message.
// Same layout as NAV except field 8 is Datumid instead of reserved.
// LG290P firmware R02A01S appends one extra trailing reserved field
// beyond the v1.0 spec; Res18 absorbs it.
type PPPNAV struct {
	TimeStatus uint8           // 0=invalid, 1=valid
	TimeRef    uint8           // 1=GPS
	UTC        string          // hhmmss.sss
	Date       string          // yyyymmdd
	TOW        opt.Val[uint32] // ms, GPS time of week
	WN         opt.Val[uint16] // GPS week number
	LeapSec    opt.Val[uint8]  // seconds
	Datumid    opt.Val[uint8]  // 1=WGS84, 2=PPP original, 3=CGCS2000
	Res1       skip
	SolType    opt.Val[uint8] // 0=none, 1=single, 2=SBAS, 5=DGPS, 6=PPP converging, 7=PPP convergenced, 8=RTK float, 12=RTK fixed
	Res2       skip
	Lat        opt.Val[float64] // deg
	Lon        opt.Val[float64] // deg
	Alt        opt.Val[float64] // m, above MSL
	Sep        opt.Val[float64] // m, geoidal separation
	Res3       skip
	Res4       skip
	LatStd     opt.Val[float64] // m
	LonStd     opt.Val[float64] // m
	AltStd     opt.Val[float64] // m
	Res5       skip
	Res6       skip
	DiffID     opt.Val[uint16]  // 9001=B2b PPP, 9002=E6 HAS
	DiffAge    opt.Val[float64] // seconds
	Res7       skip
	SatView    opt.Val[decUint8] // satellites in view
	SatUsed    opt.Val[decUint8] // satellites in use
	Res8       skip
	Res9       skip
	Res10      skip
	Res11      skip
	Res12      skip
	Res13      skip
	HVel       opt.Val[float64] // m/s
	VVel       opt.Val[float64] // m/s, upward positive
	HVelStd    opt.Val[float64] // m/s
	VVelStd    opt.Val[float64] // m/s
	Res14      skip
	Res15      skip
	COG        opt.Val[float64] // deg, 0-360
	Res16      skip
	Res17      skip
	Res18      skip
}

func (*PPPNAV) periodicMsg()        {}
func (*PPPNAV) ID() (string, uint8) { return "PPPNAV", 1 }

// GeofenceStatus represents a PQTMGEOFENCESTATUS geofence status message.
type GeofenceStatus struct {
	Time   string // hhmmss.sss, UTC
	State0 uint8  // 0=unknown, 1=inside, 2=outside
	State1 uint8
	State2 uint8
	State3 uint8
}

func (*GeofenceStatus) periodicMsg()        {}
func (*GeofenceStatus) ID() (string, uint8) { return "GEOFENCESTATUS", 1 }

// TXT represents a PQTMTXT text message.
type TXT struct {
	TotalNum decUint8 // total number of sentences
	Num      decUint8 // sentence number
	TextID   decUint8 // 01=notice, 02=warning, 03=error
	Text     string   // text content
}

func (*TXT) periodicMsg()        {}
func (*TXT) ID() (string, uint8) { return "TXT", 1 }

// PL represents a PQTMPL protection level message.
type PL struct {
	TOW  opt.Val[uint32] // ms, GPS time of week
	PUL  float64         // probability of uncertainty level (%)
	Res0 skip
	Res1 skip
	PosN opt.Val[uint32] // mm, north
	PosE opt.Val[uint32] // mm, east
	PosD opt.Val[uint32] // mm, down
	VelN opt.Val[uint32] // mm/s, north
	VelE opt.Val[uint32] // mm/s, east
	VelD opt.Val[uint32] // mm/s, down
	Res2 skip
	Res3 skip
	Time opt.Val[uint32] // ns
}

func (*PL) periodicMsg()        {}
func (*PL) ID() (string, uint8) { return "PL", 1 }

// ODO represents a PQTMODO odometer message.
type ODO struct {
	Time  string           // hhmmss.sss, UTC
	State uint8            // 0=disabled, 1=enabled
	Dist  opt.Val[float64] // m, distance since last reset
}

func (*ODO) periodicMsg()        {}
func (*ODO) ID() (string, uint8) { return "ODO", 1 }

// AntennaStatus represents a PQTMANTENNASTATUS antenna status message.
type AntennaStatus struct {
	Status   uint8 // 0=unknown, 1=normal, 2=open, 3=short
	PowerInd uint8 // 0=off, 1=on, 2=unknown
}

func (*AntennaStatus) periodicMsg()        {}
func (*AntennaStatus) ID() (string, uint8) { return "ANTENNASTATUS", 1 }

// EOE represents a PQTMEOE end-of-epoch message.
type EOE struct {
	UTC  string // hhmmss.sss
	Date string // yyyymmdd
	WN   uint16 // GPS week number
	TOW  uint32 // ms, GPS time of week
}

func (*EOE) periodicMsg()        {}
func (*EOE) ID() (string, uint8) { return "EOE", 1 }

// checkVersion parses and validates the version field at the start of a PQTM
// message, returning the remaining fields.
func checkVersion(fields []string, expected uint8) ([]string, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("missing version field")
	}
	v, err := strconv.ParseUint(fields[0], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("invalid version: %w", err)
	}
	if uint8(v) != expected {
		return nil, fmt.Errorf("unsupported version %d (expected %d)", v, expected)
	}
	return fields[1:], nil
}

// ParsePeriodicMsg parses a PQTM periodic output message from a sentence
// payload (the content between $ and *).
//
// It returns (nil, nil) if the payload is not a recognized periodic
// message. This covers both non-PQTM payloads and PQTM messages that
// are not periodic output (e.g. configuration responses, commands).
// The caller should fall through to other handlers in this case.
//
// It returns a non-nil error if the message is a recognized periodic
// type but cannot be parsed (unsupported version or malformed fields).
func ParsePeriodicMsg(payload string) (PeriodicMsg, error) {
	if !strings.HasPrefix(payload, "PQTM") {
		return nil, nil
	}
	comma := strings.IndexByte(payload, ',')
	if comma < 0 {
		return nil, nil
	}
	msgType := payload[4:comma]
	ctor := periodicMap[msgType]
	if ctor == nil {
		return nil, nil
	}
	msg := ctor()
	_, ver := msg.ID()
	fields := strings.Split(payload[comma+1:], ",")
	fields, err := checkVersion(fields, ver)
	if err != nil {
		return nil, fmt.Errorf("qtmmsg: %s: %w", msgType, err)
	}
	if err := fieldenc.Decode(fields, msg); err != nil {
		return nil, fmt.Errorf("qtmmsg: %s: %w", msgType, err)
	}
	return msg, nil
}

// PeriodicMsgVer returns the expected message version for a known periodic
// message name. For example, PeriodicMsgVer("PVT") returns (1, true).
// This is the version parameter used with PQTMCFGMSGRATE.
// Returns (0, false) if the name is not a known periodic message.
func PeriodicMsgVer(name string) (uint8, bool) {
	ctor := periodicMap[name]
	if ctor == nil {
		return 0, false
	}
	_, ver := ctor().ID()
	return ver, true
}

var periodicMap = make(map[string]func() PeriodicMsg)

func regPeriodic[T any, PT interface {
	*T
	PeriodicMsg
}]() {
	m := PT(new(T))
	name, _ := m.ID()
	periodicMap[name] = func() PeriodicMsg { return PT(new(T)) }
}

func init() {
	regPeriodic[PVT]()
	regPeriodic[VEL]()
	regPeriodic[EPE]()
	regPeriodic[DOP]()
	regPeriodic[SVINStatus]()
	regPeriodic[NAV]()
	regPeriodic[PPPNAV]()
	regPeriodic[EOE]()
	regPeriodic[GeofenceStatus]()
	regPeriodic[TXT]()
	regPeriodic[PL]()
	regPeriodic[ODO]()
	regPeriodic[AntennaStatus]()
}
