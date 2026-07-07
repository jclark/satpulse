package qtmmsg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/gps/lib/fieldenc"
	"github.com/jclark/satpulse/gps/lib/opt"
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

// HexByte is a uint8 that marshals as two uppercase hex digits,
// matching the receiver's signal-mask fields (e.g. "3F").
type HexByte uint8

// MarshalText implements encoding.TextMarshaler.
func (v HexByte) MarshalText() ([]byte, error) {
	return fmt.Appendf(nil, "%02X", uint8(v)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *HexByte) UnmarshalText(text []byte) error {
	n, err := strconv.ParseUint(string(text), 16, 8)
	*v = HexByte(n)
	return err
}

// Hex32 is a uint32 that marshals as eight uppercase hex digits,
// matching the receiver's protocol-mask fields (e.g. "00000007").
type Hex32 uint32

// MarshalText implements encoding.TextMarshaler.
func (v Hex32) MarshalText() ([]byte, error) {
	return fmt.Appendf(nil, "%08X", uint32(v)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *Hex32) UnmarshalText(text []byte) error {
	n, err := strconv.ParseUint(string(text), 16, 32)
	*v = Hex32(n)
	return err
}

// Fixed4 is a float64 that marshals with exactly four decimal places,
// matching the receiver's ECEF coordinate fields (e.g. "0.0000").
type Fixed4 float64

// MarshalText implements encoding.TextMarshaler.
func (v Fixed4) MarshalText() ([]byte, error) {
	return strconv.AppendFloat(nil, float64(v), 'f', 4, 64), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *Fixed4) UnmarshalText(text []byte) error {
	f, err := strconv.ParseFloat(string(text), 64)
	*v = Fixed4(f)
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

// PPS output modes (the CfgPPS/CfgPPS2 Mode field).
const (
	PPSModeAlways  = 1 // PPS always output
	PPSModeFixOnly = 2 // PPS output only in 2D/3D fix mode
)

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

// CfgSignal represents the PQTMCFGSIGNAL tuple (GNSS signal masks).
// Applies only after the configuration is saved and the module
// restarted. The receiver stores masks it cannot honor (e.g. GPS L1
// cleared) without clamping; readback reflects the stored mask.
type CfgSignal struct {
	GPSSig HexByte // bit 0=L1 C/A, 1=L2C, 2=L5-Q
	GLOSig HexByte // bit 0=G1 C/A, 1=G2 C/A
	GALSig HexByte // bit 0=E1, 1=E5a, 2=E5b, 3=E6
	BDSSig HexByte // bit 0=B1I, 1=B2I, 2=B3I, 3=B1C, 4=B2a, 5=B2b
	QZSSig HexByte // bit 0=L1 C/A, 1=L2C, 2=L5-Q, 3=L6
	NACSig HexByte // bit 0=L5
}

func (*CfgSignal) cfgMsg()          {}
func (*CfgSignal) Sentence() string { return "PQTMCFGSIGNAL" }

// CfgCnst represents the PQTMCFGCNST tuple (constellation enables).
// Applies only after the configuration is saved and the module
// restarted. A disabled constellation gates the CfgSignal and CfgSat
// masks for it.
type CfgCnst struct {
	GPS     uint8 // 0=disable, 1=enable
	GLONASS uint8
	Galileo uint8
	BDS     uint8
	QZSS    uint8
	NavIC   uint8
}

func (*CfgCnst) cfgMsg()          {}
func (*CfgCnst) Sentence() string { return "PQTMCFGCNST" }

// PQTMCFGSVIN mode values.
const (
	SvinModeDisable  = 0
	SvinModeSurveyIn = 1
	SvinModeFixed    = 2 // fixed ECEF position
)

// CfgSvin represents the PQTMCFGSVIN tuple (Survey-in feature).
// Applies only after the configuration is saved and the module
// restarted, and only in base receiver mode. The surveyed result
// never appears in this tuple; PQTMSVINSTATUS carries it.
type CfgSvin struct {
	Mode       uint8  // 0=disable, 1=Survey-in, 2=Fixed (ECEF)
	CfgCnt     uint32 // s, minimum positioning times in Survey-in mode, 0-86400
	AccLimit3D Fixed1 // m, 3D accuracy limit, 0.0 = no limit
	ECEFX      Fixed4 // m, WGS84 ECEF X
	ECEFY      Fixed4 // m, WGS84 ECEF Y
	ECEFZ      Fixed4 // m, WGS84 ECEF Z
	Distance   Fixed1 // m, min distance between successive Survey-in results
}

func (*CfgSvin) cfgMsg()          {}
func (*CfgSvin) Sentence() string { return "PQTMCFGSVIN" }

// writeFields omits the optional Distance field when zero, matching
// the verified 6-field set form.
func (m *CfgSvin) writeFields() ([]string, error) {
	fields, err := fieldenc.Encode(m)
	if err != nil {
		return nil, err
	}
	if m.Distance == 0 {
		fields = fields[:6]
	}
	return fields, nil
}

// PQTMCFGRCVRMODE mode values.
const (
	RcvrModeUnknown = 0
	RcvrModeRover   = 1
	RcvrModeBase    = 2 // base station
)

// CfgRcvrMode represents the PQTMCFGRCVRMODE tuple (receiver working
// mode). Applies only after the configuration is saved and the module
// restarted. Each mode keeps its own message-rate table.
type CfgRcvrMode struct {
	Mode uint8 // 0=unknown, 1=Rover, 2=Base Station
}

func (*CfgRcvrMode) cfgMsg()          {}
func (*CfgRcvrMode) Sentence() string { return "PQTMCFGRCVRMODE" }

// CfgMsgRate represents the PQTMCFGMSGRATE tuple in its current-
// interface form (message name, rate, and version for PQTM messages
// or time offset for RTCM3 MSM messages). Rate changes apply live.
type CfgMsgRate struct {
	MsgName string         // e.g. "GGA", "PQTMPVT", "RTCM3-107X"
	Rate    uint32         // 0 = not output, N = once every N fixes
	MsgVer  opt.Val[uint8] // PQTM message version / MSM time offset; absent otherwise
}

func (*CfgMsgRate) cfgMsg()          {}
func (*CfgMsgRate) Sentence() string { return "PQTMCFGMSGRATE" }

// writeFields omits the trailing version field for messages that do
// not carry one.
func (m *CfgMsgRate) writeFields() ([]string, error) {
	fields, err := fieldenc.Encode(m)
	if err != nil {
		return nil, err
	}
	if !m.MsgVer.IsSet() {
		fields = fields[:2]
	}
	return fields, nil
}

// CfgProt represents the PQTMCFGPROT tuple (input/output protocol
// enables for a port). Applies live. Clearing the NMEA output bit on
// the active port silences periodic output but not command responses.
type CfgProt struct {
	PortType   uint8 // 1=UART
	PortID     uint8 // 1=UART1, 2=UART2, 3=UART3
	InputProt  Hex32 // bit 0=NMEA, 1=QGC, 2=RTCM3
	OutputProt Hex32 // bit 0=NMEA, 1=QGC, 2=RTCM3
}

func (*CfgProt) cfgMsg()          {}
func (*CfgProt) Sentence() string { return "PQTMCFGPROT" }

// CfgRtcm represents the PQTMCFGRTCM tuple (RTCM output
// configuration). MSMElevThd is RTCM-only, distinct from CfgEleThd's
// position-engine threshold.
type CfgRtcm struct {
	MSMType     uint8  // 3-7 = MSM3-MSM7
	MSMMode     uint8  // always 0 (no output when no satellite searched)
	MSMElevThd  Fixed1 // degrees, [-90.0, 90.0]; -90.0 = no limit
	Reserved0   string // always "07"
	Reserved1   string // always "06"
	EPHMode     uint8  // 0=disable, 1=on update, 2=update+interval, 3=each epoch
	EPHInterval uint32 // s, 0-7200, used when EPHMode=2
}

func (*CfgRtcm) cfgMsg()          {}
func (*CfgRtcm) Sentence() string { return "PQTMCFGRTCM" }

// CfgRSID represents the PQTMCFGRSID tuple (RTCM reference station ID).
type CfgRSID struct {
	ID uint16 // 0-4095
}

func (*CfgRSID) cfgMsg()          {}
func (*CfgRSID) Sentence() string { return "PQTMCFGRSID" }

// Verno holds a PQTMVERNO response, the probe reply carrying the
// firmware identity (e.g. "LG290P03AANR02A01S,2025/12/12,11:21:01").
type Verno struct {
	VerStr    string
	BuildDate string // yyyy/mm/dd
	BuildTime string // hh:mm:ss
}

// ParseVerno parses a PQTMVERNO response. It returns (nil, nil) if
// the payload is not a PQTMVERNO data response.
func ParseVerno(payload string) (*Verno, error) {
	name, rest, hasRest := strings.Cut(payload, ",")
	if name != "PQTMVERNO" || !hasRest {
		return nil, nil
	}
	v := &Verno{}
	if err := fieldenc.Decode(strings.Split(rest, ","), v); err != nil {
		return nil, fmt.Errorf("qtmmsg: PQTMVERNO: %w", err)
	}
	return v, nil
}

// UniqID holds a PQTMUNIQID response, the module's unique chip ID.
type UniqID struct {
	Length uint8  // ID length in bytes
	ID     string // hex digits (e.g. "0000183B31B3C252")
}

// ParseUniqID parses a PQTMUNIQID response. It returns (nil, nil) if
// the payload is not a PQTMUNIQID OK response.
func ParseUniqID(payload string) (*UniqID, error) {
	rc := ClassifyResponse(payload)
	if rc.Sentence != "PQTMUNIQID" || rc.Kind != ResponseOKData {
		return nil, nil
	}
	v := &UniqID{}
	if err := fieldenc.Decode(strings.Split(payload, ",")[2:], v); err != nil {
		return nil, fmt.Errorf("qtmmsg: PQTMUNIQID: %w", err)
	}
	return v, nil
}

// LstMsgLine is one entry of a PQTMLSTMSG dump: an enabled message on
// the queried port. Disabled messages do not appear in the dump, and
// each receiver mode keeps its own table. The dump ends with an
// "End" line; a dump line can rarely be aborted mid-sentence by the
// receiver (the scanner drops the fragment), so a reader that needs
// certainty re-issues the command.
type LstMsgLine struct {
	PortType uint8          // 1=UART
	PortID   uint8          // 1=UART1, 2=UART2, 3=UART3
	MsgName  string         // e.g. "GGA", "PQTMTXT", "RTCM3-107X"
	Rate     uint32         // output once every Rate fixes
	MsgVer   opt.Val[uint8] // PQTM message version / MSM offset; absent otherwise
}

// ParseLstMsg parses one PQTMLSTMSG response line. It returns
// (nil, true, nil) for the terminating End line, (line, false, nil)
// for a dump entry, and (nil, false, nil) if the payload is not a
// PQTMLSTMSG OK response.
func ParseLstMsg(payload string) (line *LstMsgLine, end bool, err error) {
	rc := ClassifyResponse(payload)
	if rc.Sentence != "PQTMLSTMSG" || rc.Kind != ResponseOKData {
		return nil, false, nil
	}
	fields := strings.Split(payload, ",")[2:]
	if fields[0] == "End" {
		return nil, true, nil
	}
	line = &LstMsgLine{}
	if err := fieldenc.Decode(fields, line); err != nil {
		return nil, false, fmt.Errorf("qtmmsg: PQTMLSTMSG: %w", err)
	}
	return line, false, nil
}

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
	regCfg[CfgSignal]()
	regCfg[CfgCnst]()
	regCfg[CfgSvin]()
	regCfg[CfgRcvrMode]()
	regCfg[CfgMsgRate]()
	regCfg[CfgProt]()
	regCfg[CfgRtcm]()
	regCfg[CfgRSID]()
}
