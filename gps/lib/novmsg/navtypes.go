package novmsg

import "fmt"

// DatumID represents a geodetic datum identifier.
// Binary: uint32 (61 = WGS84). ASCII: symbolic name (e.g. "WGS84").
type DatumID uint32

const DatumWGS84 DatumID = 61

// String returns the ASCII representation of DatumID
func (d DatumID) String() string {
	if d == DatumWGS84 {
		return "WGS84"
	}
	return fmt.Sprintf("%d", d)
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (d *DatumID) UnmarshalText(text []byte) error {
	if string(text) == "WGS84" {
		*d = DatumWGS84
		return nil
	}
	return fmt.Errorf("unknown datum: %s", text)
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (d DatumID) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// HexByte is a uint8 that serializes as 2-digit lowercase hex in ASCII.
// Used for bitmask fields like ExtSolStat, GalBDS3Sig, GPSGLOBDS2Sig.
type HexByte uint8

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (h *HexByte) UnmarshalText(text []byte) error {
	var v uint64
	_, err := fmt.Sscanf(string(text), "%x", &v)
	if err != nil {
		return fmt.Errorf("invalid hex byte: %s", text)
	}
	*h = HexByte(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (h HexByte) MarshalText() ([]byte, error) {
	return fmt.Appendf(nil, "%02x", uint8(h)), nil
}

// StationID represents a base station ID, stored as [4]byte in binary
// and as a quoted string (e.g. "") in ASCII.
type StationID [4]byte

// UnmarshalText implements encoding.TextUnmarshaler.
// Strips surrounding quotes: "" -> empty, "0" -> "0".
func (s *StationID) UnmarshalText(text []byte) error {
	t := string(text)
	if len(t) >= 2 && t[0] == '"' && t[len(t)-1] == '"' {
		t = t[1 : len(t)-1]
	}
	*s = StationID{}
	copy(s[:], t)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
// Returns the station ID as a quoted string.
func (s StationID) MarshalText() ([]byte, error) {
	n := len(s)
	for n > 0 && s[n-1] == 0 {
		n--
	}
	out := make([]byte, 0, n+2)
	out = append(out, '"')
	out = append(out, s[:n]...)
	out = append(out, '"')
	return out, nil
}

// Pos contains the geodetic position fields shared between NovAtel BESTPOS
// and Unicore BESTNAV. The binary layout is identical across vendors;
// the SolStatus (S) and PosType (P) enum types vary by vendor.
type Pos[S, P ~uint32] struct {
	PSolStatus    S         // position solution status
	PosType       P         // position type
	Lat           float64   // latitude (degrees)
	Lon           float64   // longitude (degrees)
	Hgt           float64   // height above MSL (meters)
	Undulation    float32   // geoid undulation (meters)
	DatumID       DatumID   // datum ID (61 = WGS84)
	LatSigma      float32   // latitude std dev (meters)
	LonSigma      float32   // longitude std dev (meters)
	HgtSigma      float32   // height std dev (meters)
	StnID         StationID // base station ID
	DiffAge       float32   // differential data age (seconds)
	SolAge        float32   // solution age (seconds)
	NumSVs        uint8     // satellites tracked
	NumSolnSVs    uint8     // satellites in solution
	NumSolnL1SVs  uint8     // satellites with L1/E1/B1 signals in solution
	NumSolnMulti  uint8     // satellites with multi-frequency in solution
	Reserved      uint8     // reserved
	ExtSolStat    HexByte   // extended solution status
	GalBDS3Sig    HexByte   // Galileo/BDS3 signal mask
	GPSGLOBDS2Sig HexByte   // GPS/GLONASS/BDS2 signal mask
}

// XYZ contains the ECEF position+velocity fields shared between NovAtel
// BESTXYZ (ID 241) and Unicore BESTNAVXYZ (ID 240). The binary layout is
// identical across vendors; the SolStatus (S) and PosType/VelType (P) enum
// types vary by vendor.
type XYZ[S, P ~uint32] struct {
	PSolStatus    S         // position solution status
	PosType       P         // position type
	PX            float64   // ECEF X (meters)
	PY            float64   // ECEF Y (meters)
	PZ            float64   // ECEF Z (meters)
	PXSigma       float32   // X std dev (meters)
	PYSigma       float32   // Y std dev (meters)
	PZSigma       float32   // Z std dev (meters)
	VSolStatus    S         // velocity solution status
	VelType       P         // velocity type
	VX            float64   // ECEF VX (m/s)
	VY            float64   // ECEF VY (m/s)
	VZ            float64   // ECEF VZ (m/s)
	VXSigma       float32   // VX std dev (m/s)
	VYSigma       float32   // VY std dev (m/s)
	VZSigma       float32   // VZ std dev (m/s)
	StnID         StationID // base station ID
	VLatency      float32   // velocity latency (seconds)
	DiffAge       float32   // differential age (seconds)
	SolAge        float32   // solution age (seconds)
	NumSVs        uint8     // satellites tracked
	NumSolnSVs    uint8     // satellites in solution
	NumSolnL1SVs  uint8     // satellites with L1/E1/B1 signals in solution
	NumSolnMulti  uint8     // satellites with multi-frequency in solution
	Reserved      uint8     // reserved
	ExtSolStat    HexByte   // extended solution status
	GalBDS3Sig    HexByte   // Galileo/BDS3 signal mask
	GPSGLOBDS2Sig HexByte   // GPS/GLONASS/BDS2 signal mask
}
