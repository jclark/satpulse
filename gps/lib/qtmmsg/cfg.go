package qtmmsg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/gps/lib/fieldenc"
)

// CfgMsg is implemented by all known PQTM configuration command tuples.
// One struct per CFG command: the get response (NAME,OK,<tuple>) parses
// into it and the set command (NAME,W,<tuple>) serializes from it, the
// tuples being field-for-field identical on the wire. The set of types
// implementing this interface is closed.
type CfgMsg interface {
	cfgMsg()
	Sentence() string // full sentence name (e.g. "PQTMCFGPPS")
}

// EncodeWrite returns the NMEA payload (between $ and *) of the set
// command for a CFG tuple, e.g. "PQTMCFGPPS,W,1,1,100,1,1,0".
func EncodeWrite(m CfgMsg) (string, error) {
	fields, err := writeFieldsOf(m)
	if err != nil {
		return "", fmt.Errorf("qtmmsg: %s: %w", m.Sentence(), err)
	}
	return m.Sentence() + ",W," + strings.Join(fields, ","), nil
}

// writeFielder is implemented by CFG types whose W form is not always
// the full tuple (the spec makes trailing fields conditional).
type writeFielder interface {
	writeFields() ([]string, error)
}

func writeFieldsOf(m CfgMsg) ([]string, error) {
	if wf, ok := m.(writeFielder); ok {
		return wf.writeFields()
	}
	return fieldenc.Encode(m)
}

// ParseCfgResponse parses a CFG get response (NAME,OK,<tuple>) into the
// tuple struct registered for that sentence.
//
// It returns (nil, nil) if the payload is not an OK-with-data response
// for a registered CFG sentence; the caller should fall through to
// other handlers. It returns a non-nil error if the response is for a
// registered sentence but the tuple cannot be parsed.
func ParseCfgResponse(payload string) (CfgMsg, error) {
	rc := ClassifyResponse(payload)
	if rc.Kind != ResponseOKData {
		return nil, nil
	}
	ctor := cfgMap[rc.Sentence]
	if ctor == nil {
		return nil, nil
	}
	msg := ctor()
	fields := strings.Split(payload, ",")[2:]
	if err := fieldenc.Decode(fields, msg); err != nil {
		return nil, fmt.Errorf("qtmmsg: %s: %w", rc.Sentence, err)
	}
	return msg, nil
}

// Fixed1 is a float64 that marshals with exactly one decimal place,
// matching the receiver's fixed-point text fields (e.g. "5.0").
type Fixed1 float64

// MarshalText implements encoding.TextMarshaler.
func (v Fixed1) MarshalText() ([]byte, error) {
	return strconv.AppendFloat(nil, float64(v), 'f', 1, 64), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *Fixed1) UnmarshalText(text []byte) error {
	f, err := strconv.ParseFloat(string(text), 64)
	*v = Fixed1(f)
	return err
}

// CfgUART represents the PQTMCFGUART tuple (UART interface).
type CfgUART struct {
	Index    uint8  // 1=UART1, 2=UART2, 3=UART3; 0=current UART (W form only)
	BaudRate uint32 // bps
	DataBit  uint8  // 8; 0 means unset (W form sends BaudRate only)
	Parity   uint8  // 0=none, 1=odd, 2=even, 3=mark, 4=space
	StopBit  uint8  // 1 or 2
	FlowCtrl uint8  // 0=none
}

func (*CfgUART) cfgMsg()          {}
func (*CfgUART) Sentence() string { return "PQTMCFGUART" }

// writeFields implements the spec's optional W fields: a zero Index
// selects the current-UART form (no index field), and a zero DataBit
// sends the baud rate alone.
func (m *CfgUART) writeFields() ([]string, error) {
	fields, err := fieldenc.Encode(m)
	if err != nil {
		return nil, err
	}
	if m.DataBit == 0 {
		fields = fields[:2]
	}
	if m.Index == 0 {
		fields = fields[1:]
	}
	return fields, nil
}

// CfgPPS represents the PQTMCFGPPS tuple (PPS feature).
type CfgPPS struct {
	Index    uint8  // 1=PPS1
	Enable   uint8  // 0=disable, 1=enable
	Duration uint16 // ms, 0-900
	Mode     uint8  // 1=always output, 2=only in 2D/3D fix mode
	Polarity uint8  // 0=low, 1=high
	Reserved uint8  // always 0
}

func (*CfgPPS) cfgMsg()          {}
func (*CfgPPS) Sentence() string { return "PQTMCFGPPS" }

// writeFields omits the fields after Enable when disabling, as the
// spec requires.
func (m *CfgPPS) writeFields() ([]string, error) {
	if m.Enable == 0 {
		return []string{strconv.Itoa(int(m.Index)), "0"}, nil
	}
	return fieldenc.Encode(m)
}

// CfgPPS2 represents the PQTMCFGPPS2 tuple (PPS extend feature,
// R02A01S+). It addresses the same underlying state as CfgPPS plus
// Period and Userdelay, which a CfgPPS write leaves unchanged.
type CfgPPS2 struct {
	Index     uint8  // 1=PPS1
	Enable    uint8  // 0=disable, 1=enable
	Duration  uint16 // ms, (0, Period]
	Mode      uint8  // 1=always output, 2=only in 2D/3D fix mode
	Polarity  uint8  // 0=low, 1=high
	Reserved1 uint8  // always 0
	Period    uint16 // ms, 10-60000
	Userdelay int32  // ns
	Reserved2 uint8  // always 1
	Reserved3 uint8  // always 0
	Reserved4 uint8  // always 0
	Reserved5 uint8  // always 0
}

func (*CfgPPS2) cfgMsg()          {}
func (*CfgPPS2) Sentence() string { return "PQTMCFGPPS2" }

// writeFields omits the fields after Enable when disabling, as the
// spec requires.
func (m *CfgPPS2) writeFields() ([]string, error) {
	if m.Enable == 0 {
		return []string{strconv.Itoa(int(m.Index)), "0"}, nil
	}
	return fieldenc.Encode(m)
}

// CfgFixRate represents the PQTMCFGFIXRATE tuple (fix interval).
// Applies only after the configuration is saved and the module
// restarted.
type CfgFixRate struct {
	FixInterval uint32 // ms
}

func (*CfgFixRate) cfgMsg()          {}
func (*CfgFixRate) Sentence() string { return "PQTMCFGFIXRATE" }

// CfgEleThd represents the PQTMCFGELETHD tuple (elevation threshold
// for the position engine).
type CfgEleThd struct {
	Ele Fixed1 // degrees, [-90.0, 90.0]; -90.0 = no limitation
}

func (*CfgEleThd) cfgMsg()          {}
func (*CfgEleThd) Sentence() string { return "PQTMCFGELETHD" }

var cfgMap = make(map[string]func() CfgMsg)

func regCfg[T any, PT interface {
	*T
	CfgMsg
}]() {
	m := PT(new(T))
	cfgMap[m.Sentence()] = func() CfgMsg { return PT(new(T)) }
}

func init() {
	regCfg[CfgUART]()
	regCfg[CfgPPS]()
	regCfg[CfgPPS2]()
	regCfg[CfgFixRate]()
	regCfg[CfgEleThd]()
}
