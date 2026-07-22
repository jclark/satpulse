package gpsprot

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
)

// ConfigTarget represents configuration target settings
type ConfigTarget struct {
	Props ConfigProps
	Get   PropIDs // Bitmask of properties to retrieve
	Opts  ConfigOptions
}

// PropIDs represents a set of configuration property names
type PropIDs uint32

// TimePulse represents the time pulse configuration settings
type TimePulse struct {
	Width          time.Duration // width of 0 means disabled
	Period         time.Duration
	AlignToGNSS    bool
	OnlyWhenLocked bool
	PolarityRising bool
}

// ConfigProps represents a collection configuration properties
type ConfigProps struct {
	valid             PropIDs // says which fields are valid
	signalsEnabled    SignalSet
	timeGNSS          GNSS
	timePulse         TimePulse
	mode              Mode
	antennaCableDelay time.Duration
	navMsgAuth        NavMsgAuth
	rtcmBaseID        uint16
	minElevation      Angle
	baudRate          uint32
	port              string
}

const (
	PropIDSignalsEnabled PropIDs = 1 << iota
	PropIDTimeGNSS
	// eventually the individual time pulse properties will be combined into a single property
	PropIDTimePulseWidth
	PropIDTimePulsePeriod
	PropIDTimePulseAlignToGNSS
	PropIDTimePulseOnlyWhenLocked
	PropIDTimePulsePolarityRising
	PropIDMode
	PropIDAntennaCableDelay
	PropIDNavMsgAuth
	PropIDRTCMBaseID
	PropIDMinElevation
	PropIDBaudRate
	PropIDPort
	PropIDTimePulse PropIDs = PropIDTimePulseWidth | PropIDTimePulsePeriod |
		PropIDTimePulseAlignToGNSS | PropIDTimePulseOnlyWhenLocked | PropIDTimePulsePolarityRising
)

// PropIDsReadOnly is the bitmask of properties that can only be
// retrieved, not set. Callers must not include any of these bits in
// target.Props; the contract is enforced at the public entry point in
// gpscfg.Configure (panic on violation). Backends populate these
// values on the result with the normal setter.
const PropIDsReadOnly PropIDs = PropIDPort

// propNames lists the property names in the same order as the bit constants
var propNames = []string{
	"SignalsEnabled",
	"TimeGNSS",
	"TimePulseWidth",
	"TimePulsePeriod",
	"TimePulseAlignToGNSS",
	"TimePulseOnlyWhenLocked",
	"TimePulsePolarityRising",
	"Mode",
	"AntennaCableDelay",
	"NavMsgAuth",
	"RTCMBaseID",
	"MinElevation",
	"BaudRate",
	"Port",
}

// IsEmpty returns true if no properties are set
func (cp *ConfigProps) IsEmpty() bool {
	return cp.valid == 0
}

type SaveType uint8

const (
	SaveNone    SaveType = iota
	SaveMinimal          // save the minimum to save the configuration changes specified in ConfigTarget
	SaveAll              // save the current configuration
)

var saveTypeNames = [...]string{"none", "minimal", "all"}

// MarshalText marshals SaveType as a name.
func (t SaveType) MarshalText() ([]byte, error) {
	if int(t) >= len(saveTypeNames) {
		return nil, fmt.Errorf("unknown save type %d", t)
	}
	return []byte(saveTypeNames[t]), nil
}

// UnmarshalText unmarshals SaveType from a name.
func (t *SaveType) UnmarshalText(text []byte) error {
	for i, name := range saveTypeNames {
		if string(text) == name {
			*t = SaveType(i)
			return nil
		}
	}
	return fmt.Errorf("unknown save type %q", text)
}

type ResetType uint8

const (
	ResetNone    ResetType = iota
	ResetReload            // reload the configuration from non-volatile memory without a reset/start (if possible)
	ResetCold              // restore configuration from non-volatile memory and perform a cold start
	ResetFactory           // restore non-volatile memory to factory defaults and then ResetCold
)

var resetTypeNames = [...]string{"none", "reload", "cold", "factory"}

// MarshalText marshals ResetType as a name.
func (t ResetType) MarshalText() ([]byte, error) {
	if int(t) >= len(resetTypeNames) {
		return nil, fmt.Errorf("unknown reset type %d", t)
	}
	return []byte(resetTypeNames[t]), nil
}

// UnmarshalText unmarshals ResetType from a name.
func (t *ResetType) UnmarshalText(text []byte) error {
	for i, name := range resetTypeNames {
		if string(text) == name {
			*t = ResetType(i)
			return nil
		}
	}
	return fmt.Errorf("unknown reset type %q", text)
}

type jsonFlagType interface {
	~int | ~uint8 | ~uint16
}

type jsonFlag[T jsonFlagType] struct {
	name string
	flag T
}

func marshalJSONFlags[T jsonFlagType](f T, table []jsonFlag[T], kind string) ([]byte, error) {
	names := make([]string, 0)
	var covered T
	for _, e := range table {
		covered |= e.flag
		if f&e.flag != 0 {
			names = append(names, e.name)
		}
	}
	if unknown := f &^ covered; unknown != 0 {
		return nil, fmt.Errorf("unknown %s flags 0x%x", kind, unknown)
	}
	return json.Marshal(names)
}

func unmarshalJSONFlags[T jsonFlagType](data []byte, f *T, table []jsonFlag[T], kind string) error {
	if string(data) == "null" {
		return fmt.Errorf("null not allowed for %s flags", kind)
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	var flags T
	for _, name := range names {
		found := false
		for _, e := range table {
			if name == e.name {
				flags |= e.flag
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown %s flag %q", kind, name)
		}
	}
	*f = flags
	return nil
}

// PVTMsgFlags says what messages relating to Position, Velocity, and Time are wanted.
// The PVTMsgOff option says to turn off PVT messages that are not enabled.
// This makes PVTMsgFlags different from the other message flags,
// where the equivalent of PVTMsgOff semantics is always applied.
// A zero value means no specific PVT message configuration is requested.
type PVTMsgFlags uint16

const (
	PVTMsgPos            PVTMsgFlags = 1 << iota // position
	PVTMsgVel                                    // velocity
	PVTMsgTime                                   // time of navigation solution
	PVTMsgTimePulse                              // time of time pulse
	PVTMsgLeapSecond                             // date of most recently announced leap second
	PVTMsgSurvey                                 // survey-in progress
	PVTMsgTAI                                    // want time in TAI not UTC (option)
	PVTMsgECEF                                   // want position in ECEF coordinates (option)
	PVTMsgTimePulseAfter                         // ensure there is a time message following the time pulse (option)
	PVTMsgQuality                                // solution quality messages (fix level, corrections, DOPs)
	PVTMsgEpoch                                  // end-of-epoch messages (NavEpochMsg)
	PVTMsgOff                                    // turn off any unneeded PVT messages (option)
)

const PVTMsgAny PVTMsgFlags = PVTMsgPos | PVTMsgVel | PVTMsgTime | PVTMsgTimePulse | PVTMsgLeapSecond | PVTMsgSurvey | PVTMsgQuality | PVTMsgEpoch // any message (not option)

var pvtMsgJSON = []jsonFlag[PVTMsgFlags]{
	{"pos", PVTMsgPos},
	{"vel", PVTMsgVel},
	{"time", PVTMsgTime},
	{"timePulse", PVTMsgTimePulse},
	{"leapSecond", PVTMsgLeapSecond},
	{"survey", PVTMsgSurvey},
	{"tai", PVTMsgTAI},
	{"ecef", PVTMsgECEF},
	{"timePulseAfter", PVTMsgTimePulseAfter},
	{"quality", PVTMsgQuality},
	{"epoch", PVTMsgEpoch},
	{"off", PVTMsgOff},
}

// MarshalJSON marshals PVTMsgFlags as an array of names.
func (f PVTMsgFlags) MarshalJSON() ([]byte, error) {
	return marshalJSONFlags(f, pvtMsgJSON, "PVT message")
}

// UnmarshalJSON unmarshals PVTMsgFlags from an array of names.
func (f *PVTMsgFlags) UnmarshalJSON(data []byte) error {
	return unmarshalJSONFlags(data, f, pvtMsgJSON, "PVT message")
}

// PVTMsgTimingPTP is the PVT message set required to drive a PTP
// grandmaster from a receiver's hardware time pulse.
const PVTMsgTimingPTP PVTMsgFlags = PVTMsgTimePulse | PVTMsgTimePulseAfter | PVTMsgTAI | PVTMsgLeapSecond | PVTMsgSurvey | PVTMsgQuality | PVTMsgEpoch

// PVTMsgTimingSerialUTC is the PVT message set required when time is
// delivered in UTC over the serial stream only, without a hardware
// time pulse.
const PVTMsgTimingSerialUTC PVTMsgFlags = PVTMsgTime | PVTMsgLeapSecond | PVTMsgSurvey | PVTMsgQuality | PVTMsgEpoch

// These methods are to make PVTMsgFlags more consistent with Option[*Flags] for the other flags.

// IsZero returns true if no PVT message flags are set
func (f *PVTMsgFlags) IsZero() bool {
	return *f == 0
}

// IsSet returns true if any PVT message flags are set
func (f *PVTMsgFlags) IsSet() bool {
	return *f != 0
}

// Get returns the PVT message flags value
func (f *PVTMsgFlags) Get() PVTMsgFlags {
	return *f
}

// Set sets the PVT message flags value
func (f *PVTMsgFlags) Set(v PVTMsgFlags) {
	*f = v
}

type SatsMsgFlags uint8

const (
	SatsMsgSat    SatsMsgFlags = 1 << iota // position of SVs
	SatsMsgSignal                          // signal strength of each signal from each SV
	SatsMsgNone   SatsMsgFlags = 0
	SatsMsgAny    SatsMsgFlags = SatsMsgSat | SatsMsgSignal // any message (not flag)
)

var satsMsgJSON = []jsonFlag[SatsMsgFlags]{
	{"sat", SatsMsgSat},
	{"signal", SatsMsgSignal},
}

// MarshalJSON marshals SatsMsgFlags as an array of names.
func (f SatsMsgFlags) MarshalJSON() ([]byte, error) {
	return marshalJSONFlags(f, satsMsgJSON, "satellite message")
}

// UnmarshalJSON unmarshals SatsMsgFlags from an array of names.
func (f *SatsMsgFlags) UnmarshalJSON(data []byte) error {
	return unmarshalJSONFlags(data, f, satsMsgJSON, "satellite message")
}

type NMEAMsgFlags uint16

const (
	NMEAMsgRMC NMEAMsgFlags = 1 << iota
	NMEAMsgGGA
	NMEAMsgGSA
	NMEAMsgGSV
	NMEAMsgZDA
	NMEAMsgVTG
	NMEAMsgGLL
	NMEAMsgOther NMEAMsgFlags = 1 << 15 // other unspecified NMEA messages
	// may have flags like NMEA version or rate in the future
	NMEAMsgNone NMEAMsgFlags = 0
	NMEAMsgAny  NMEAMsgFlags = NMEAMsgRMC | NMEAMsgGGA | NMEAMsgGSA | NMEAMsgGSV | NMEAMsgZDA | NMEAMsgVTG | NMEAMsgGLL | NMEAMsgOther // any message (not flag)
)

var nmeaMsgJSON = []jsonFlag[NMEAMsgFlags]{
	{"RMC", NMEAMsgRMC},
	{"GGA", NMEAMsgGGA},
	{"GSA", NMEAMsgGSA},
	{"GSV", NMEAMsgGSV},
	{"ZDA", NMEAMsgZDA},
	{"VTG", NMEAMsgVTG},
	{"GLL", NMEAMsgGLL},
	{"other", NMEAMsgOther},
}

// MarshalJSON marshals NMEAMsgFlags as an array of names.
func (f NMEAMsgFlags) MarshalJSON() ([]byte, error) {
	return marshalJSONFlags(f, nmeaMsgJSON, "NMEA message")
}

// UnmarshalJSON unmarshals NMEAMsgFlags from an array of names.
func (f *NMEAMsgFlags) UnmarshalJSON(data []byte) error {
	return unmarshalJSONFlags(data, f, nmeaMsgJSON, "NMEA message")
}

type RTCMMsgFlags uint16

const (
	RTCMMsgMSM4  RTCMMsgFlags = 1 << iota // MSM4 for all enabled GNSS
	_                                     // 5
	_                                     // 6
	RTCMMsgMSM7                           // MSM7 for all enabled GNSS
	RTCMMsgARP                            // RTCM message 1005
	RTCMMsgLax                            // Do the best we can on enabling RTCM messages
	RTCMMsgOther RTCMMsgFlags = 1 << 15   // other unspecified RTCM messages
	// may have flags for rate
	RTCMMsgNone     RTCMMsgFlags = 0
	RTCMMsgAuto     RTCMMsgFlags = RTCMMsgMSM4 | RTCMMsgARP | RTCMMsgLax                 // enable intelligently
	RTCMMsgAutoMSM7 RTCMMsgFlags = RTCMMsgMSM7 | RTCMMsgARP | RTCMMsgLax                 // enable intelligently, preferring MSM7
	RTCMMsgAny      RTCMMsgFlags = RTCMMsgMSM4 | RTCMMsgMSM7 | RTCMMsgARP | RTCMMsgOther // any message (not flag)
)

var rtcmMsgJSON = []jsonFlag[RTCMMsgFlags]{
	{"MSM4", RTCMMsgMSM4},
	{"MSM7", RTCMMsgMSM7},
	{"ARP", RTCMMsgARP},
	{"lax", RTCMMsgLax},
	{"other", RTCMMsgOther},
}

// MarshalJSON marshals RTCMMsgFlags as an array of names.
func (f RTCMMsgFlags) MarshalJSON() ([]byte, error) {
	return marshalJSONFlags(f, rtcmMsgJSON, "RTCM message")
}

// UnmarshalJSON unmarshals RTCMMsgFlags from an array of names.
func (f *RTCMMsgFlags) UnmarshalJSON(data []byte) error {
	return unmarshalJSONFlags(data, f, rtcmMsgJSON, "RTCM message")
}

type RawMsgFlags uint8

const (
	RawMsgObs     RawMsgFlags = 1 << iota // Raw observation messages (for RINEX)
	RawMsgNavData                         // Raw navigation date e.g. subframes for GPS
	RawMsgNone    RawMsgFlags = 0
	RawMsgAny     RawMsgFlags = RawMsgObs | RawMsgNavData // any message (not flag)
)

var rawMsgJSON = []jsonFlag[RawMsgFlags]{
	{"obs", RawMsgObs},
	{"navData", RawMsgNavData},
}

// MarshalJSON marshals RawMsgFlags as an array of names.
func (f RawMsgFlags) MarshalJSON() ([]byte, error) {
	return marshalJSONFlags(f, rawMsgJSON, "raw message")
}

// UnmarshalJSON unmarshals RawMsgFlags from an array of names.
func (f *RawMsgFlags) UnmarshalJSON(data []byte) error {
	return unmarshalJSONFlags(data, f, rawMsgJSON, "raw message")
}

type ConfigOptions struct {
	Socket     bool                  // connected via socket, skip serial detection
	ForceProbe bool                  // force probe even when no config changes needed
	Save       SaveType              // what to save to non-volatile memory
	Reset      ResetType             // what kind of reset to perform
	PVTMsg     PVTMsgFlags           // messages relating to Position, Velocity, and Time
	NMEAMsg    opt.Val[NMEAMsgFlags] `json:",omitzero"`
	RTCMMsg    opt.Val[RTCMMsgFlags] `json:",omitzero"` // RTCM 3.x messages
	SatsMsg    opt.Val[SatsMsgFlags] `json:",omitzero"`
	RawMsg     opt.Val[RawMsgFlags]  `json:",omitzero"`
	Survey     Survey
	SetStatic  bool         // ensure receiver is in static mode without changing existing fixed position
	TimeAssist TimeEstimate // provide time assistance to the receiver
	OSNMA      OSNMAOptions // options for OSNMA authentication
}

type OSNMAOptions struct {
	MerkleTreeRoot [32]byte // all zeros means not set
}

func NewConfigTarget() *ConfigTarget {
	return &ConfigTarget{}
}

// NoOp says whether the target is a no-op, except possibly for detecting the receiver
func (ct *ConfigTarget) NoOp() bool {
	return ct.Props.IsEmpty() && ct.Get == 0 && ct.Opts.NoOp()
}

func (ct *ConfigTarget) UsesAny(props ...PropIDs) bool {
	uses := ct.Props.valid | ct.Get
	for _, p := range props {
		if uses&p != 0 {
			return true
		}
	}
	return false
}

func (o ConfigOptions) NoOp() bool {
	var zero ConfigOptions
	return o == zero
}

func (o *ConfigOptions) SetsMsgs() bool {
	return o.PVTMsg.IsSet() || o.SatsMsg.IsSet() || o.NMEAMsg.IsSet() || o.RTCMMsg.IsSet() || o.RawMsg.IsSet()
}

func (o *ConfigOptions) EnablesMsgs() bool {
	return o.PVTMsg&PVTMsgAny != 0 || o.SatsMsg.Get()&SatsMsgAny != 0 || o.NMEAMsg.Get()&NMEAMsgAny != 0 || o.RTCMMsg.Get()&RTCMMsgAny != 0 || o.RawMsg.Get()&RawMsgAny != 0
}

// String returns a human-readable representation of the PropIDs flags
func (p PropIDs) String() string {
	var names []string
	for i := 0; i < len(propNames); i++ {
		if p&(1<<i) != 0 {
			names = append(names, propNames[i])
		}
	}
	return "PropIDs(" + strings.Join(names, "|") + ")"
}

// propIDJSON maps JSON property names to PropIDs bitmasks.
// These match the keys used in serializableMap.
var propIDJSON = map[string]PropIDs{
	"signalsEnabled":           PropIDSignalsEnabled,
	"timeGNSS":                 PropIDTimeGNSS,
	"timePulse":                PropIDTimePulse,
	"timePulse.width":          PropIDTimePulseWidth,
	"timePulse.period":         PropIDTimePulsePeriod,
	"timePulse.alignToGNSS":    PropIDTimePulseAlignToGNSS,
	"timePulse.onlyWhenLocked": PropIDTimePulseOnlyWhenLocked,
	"timePulse.polarityRising": PropIDTimePulsePolarityRising,
	"mode":                     PropIDMode,
	"antennaCableDelay":        PropIDAntennaCableDelay,
	"navMsgAuth":               PropIDNavMsgAuth,
	"rtcmBaseID":               PropIDRTCMBaseID,
	"minElevation":             PropIDMinElevation,
	"baudRate":                 PropIDBaudRate,
	"port":                     PropIDPort,
}

// propIDJSONNames lists the JSON names in a stable order for marshaling.
// "timePulse" must come before the sub-properties so it takes priority.
var propIDJSONNames = []string{
	"signalsEnabled", "timeGNSS",
	"timePulse", "timePulse.width", "timePulse.period",
	"timePulse.alignToGNSS", "timePulse.onlyWhenLocked", "timePulse.polarityRising",
	"mode", "antennaCableDelay", "navMsgAuth", "rtcmBaseID", "minElevation", "baudRate", "port",
}

// MarshalJSON marshals PropIDs as a JSON array of property name strings.
func (p PropIDs) MarshalJSON() ([]byte, error) {
	names := make([]string, 0)
	var emitted PropIDs
	for _, name := range propIDJSONNames {
		mask := propIDJSON[name]
		if p&mask == mask && emitted&mask == 0 {
			names = append(names, name)
			emitted |= mask
		}
	}
	return json.Marshal(names)
}

// UnmarshalJSON unmarshals PropIDs from a JSON array of property name strings.
func (p *PropIDs) UnmarshalJSON(data []byte) error {
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	var bits PropIDs
	for _, name := range names {
		b, ok := propIDJSON[name]
		if !ok {
			return fmt.Errorf("unknown property name %q", name)
		}
		bits |= b
	}
	*p = bits
	return nil
}

// GetSignalsEnabled returns the signalsEnabled value and whether it's set
func (cp *ConfigProps) GetSignalsEnabled() (SignalSet, bool) {
	if cp.valid&PropIDSignalsEnabled != 0 {
		return cp.signalsEnabled, true
	}
	return 0, false
}

// SetSignalsEnabled sets the signalsEnabled value
func (cp *ConfigProps) SetSignalsEnabled(val SignalSet) {
	cp.signalsEnabled = val
	cp.valid |= PropIDSignalsEnabled
}

// GetTimeGNSS returns the timeGNSS value and whether it's set
func (cp *ConfigProps) GetTimeGNSS() (GNSS, bool) {
	if cp.valid&PropIDTimeGNSS != 0 {
		return cp.timeGNSS, true
	}
	return 0, false
}

// SetTimeGNSS sets the timeGNSS value
func (cp *ConfigProps) SetTimeGNSS(val GNSS) {
	cp.timeGNSS = val
	cp.valid |= PropIDTimeGNSS
}

// GetTimePulseWidth returns the timePulseWidth value and whether it's set
func (cp *ConfigProps) GetTimePulseWidth() (time.Duration, bool) {
	if cp.valid&PropIDTimePulseWidth != 0 {
		return cp.timePulse.Width, true
	}
	return 0, false
}

// SetTimePulseWidth sets the timePulseWidth value
func (cp *ConfigProps) SetTimePulseWidth(val time.Duration) {
	cp.timePulse.Width = val
	cp.valid |= PropIDTimePulseWidth
}

// GetTimePulsePeriod returns the timePulsePeriod value and whether it's set
func (cp *ConfigProps) GetTimePulsePeriod() (time.Duration, bool) {
	if cp.valid&PropIDTimePulsePeriod != 0 {
		return cp.timePulse.Period, true
	}
	return 0, false
}

// SetTimePulsePeriod sets the timePulsePeriod value
func (cp *ConfigProps) SetTimePulsePeriod(val time.Duration) {
	cp.timePulse.Period = val
	cp.valid |= PropIDTimePulsePeriod
}

// GetTimePulseAlignToGNSS returns the timePulseAlignToGNSS value and whether it's set
func (cp *ConfigProps) GetTimePulseAlignToGNSS() (bool, bool) {
	if cp.valid&PropIDTimePulseAlignToGNSS != 0 {
		return cp.timePulse.AlignToGNSS, true
	}
	return false, false
}

// SetTimePulseAlignToGNSS sets the timePulseAlignToGNSS value
func (cp *ConfigProps) SetTimePulseAlignToGNSS(val bool) {
	cp.timePulse.AlignToGNSS = val
	cp.valid |= PropIDTimePulseAlignToGNSS
}

// GetTimePulseOnlyWhenLocked returns the timePulseOnlyWhenLocked value and whether it's set
func (cp *ConfigProps) GetTimePulseOnlyWhenLocked() (bool, bool) {
	if cp.valid&PropIDTimePulseOnlyWhenLocked != 0 {
		return cp.timePulse.OnlyWhenLocked, true
	}
	return false, false
}

// SetTimePulseOnlyWhenLocked sets the timePulseOnlyWhenLocked value
func (cp *ConfigProps) SetTimePulseOnlyWhenLocked(val bool) {
	cp.timePulse.OnlyWhenLocked = val
	cp.valid |= PropIDTimePulseOnlyWhenLocked
}

// GetTimePulsePolarityRising returns the timePulsePolarityRising value and whether it's set
func (cp *ConfigProps) GetTimePulsePolarityRising() (bool, bool) {
	if cp.valid&PropIDTimePulsePolarityRising != 0 {
		return cp.timePulse.PolarityRising, true
	}
	return false, false
}

// SetTimePulsePolarityRising sets the timePulsePolarityRising value
func (cp *ConfigProps) SetTimePulsePolarityRising(val bool) {
	cp.timePulse.PolarityRising = val
	cp.valid |= PropIDTimePulsePolarityRising
}

// timePulseProps combines all time pulse related property IDs
const timePulseProps = PropIDTimePulseWidth | PropIDTimePulsePeriod |
	PropIDTimePulseAlignToGNSS | PropIDTimePulseOnlyWhenLocked | PropIDTimePulsePolarityRising

// GetTimePulse returns the entire TimePulse struct and whether all TimePulse properties are set
func (cp *ConfigProps) GetTimePulse() (TimePulse, bool) {
	// Check if all TimePulse properties are valid
	if (cp.valid & timePulseProps) == timePulseProps {
		return cp.timePulse, true
	}
	return TimePulse{}, false
}

// SetTimePulse sets all timePulse properties at once
func (cp *ConfigProps) SetTimePulse(tp TimePulse) {
	cp.timePulse = tp
	cp.valid |= timePulseProps
}

// GetMode returns the mode value and whether it's set
func (cp *ConfigProps) GetMode() (Mode, bool) {
	if cp.valid&PropIDMode != 0 {
		return cp.mode, true
	}
	return Mode{}, false
}

// SetMode sets the mode value
func (cp *ConfigProps) SetMode(val Mode) {
	cp.mode = val
	cp.valid |= PropIDMode
}

// GetAntennaCableDelay returns the antennaCableDelay value and whether it's set
func (cp *ConfigProps) GetAntennaCableDelay() (time.Duration, bool) {
	if cp.valid&PropIDAntennaCableDelay != 0 {
		return cp.antennaCableDelay, true
	}
	return 0, false
}

// SetAntennaCableDelay sets the antennaCableDelay value
func (cp *ConfigProps) SetAntennaCableDelay(val time.Duration) {
	cp.antennaCableDelay = val
	cp.valid |= PropIDAntennaCableDelay
}

// GetNavMsgAuth returns the navMsgAuth value and whether it's set
func (cp *ConfigProps) GetNavMsgAuth() (NavMsgAuth, bool) {
	if cp.valid&PropIDNavMsgAuth != 0 {
		return cp.navMsgAuth, true
	}
	return NavMsgAuthNone, false
}

// SetNavMsgAuth sets the navMsgAuth value
func (cp *ConfigProps) SetNavMsgAuth(val NavMsgAuth) {
	cp.navMsgAuth = val
	cp.valid |= PropIDNavMsgAuth
}

// GetRTCMBaseID returns the rtcmBaseID value and whether it's set
func (cp *ConfigProps) GetRTCMBaseID() (uint16, bool) {
	if cp.valid&PropIDRTCMBaseID != 0 {
		return cp.rtcmBaseID, true
	}
	return 0, false
}

// SetRTCMBaseID sets the rtcmBaseID value
func (cp *ConfigProps) SetRTCMBaseID(val uint16) {
	cp.rtcmBaseID = val
	cp.valid |= PropIDRTCMBaseID
}

// GetMinElevation returns the minElevation value and whether it's set
func (cp *ConfigProps) GetMinElevation() (Angle, bool) {
	if cp.valid&PropIDMinElevation != 0 {
		return cp.minElevation, true
	}
	return 0, false
}

// SetMinElevation sets the minElevation value
func (cp *ConfigProps) SetMinElevation(val Angle) {
	cp.minElevation = val
	cp.valid |= PropIDMinElevation
}

// GetBaudRate returns the baud rate and whether it's set.
// In a result, value 0 means the port type has no baud rate (USB / I2C / SPI).
func (cp *ConfigProps) GetBaudRate() (uint32, bool) {
	if cp.valid&PropIDBaudRate != 0 {
		return cp.baudRate, true
	}
	return 0, false
}

// SetBaudRate sets the baud rate. In a target, the value is the new
// speed to configure.
func (cp *ConfigProps) SetBaudRate(val uint32) {
	cp.baudRate = val
	cp.valid |= PropIDBaudRate
}

// GetPort returns the active receiver port name and whether it is
// set. Port is a read-only property: it can appear on a result, but
// must not appear in target.Props.
func (cp *ConfigProps) GetPort() (string, bool) {
	if cp.valid&PropIDPort != 0 {
		return cp.port, true
	}
	return "", false
}

// SetPort records the active receiver port name on a result.
// Public so backends can populate it; callers building a target must
// not use this. See PropIDsReadOnly.
func (cp *ConfigProps) SetPort(val string) {
	cp.port = val
	cp.valid |= PropIDPort
}

// ReadOnlyProps returns the subset of PropIDsReadOnly that are set on
// cp. Used by gpscfg.Configure to detect API misuse (read-only
// properties in target.Props).
func (cp *ConfigProps) ReadOnlyProps() PropIDs {
	return cp.valid & PropIDsReadOnly
}

// ClearReadOnlyProps removes all read-only properties from cp so it
// can safely be reused as target.Props (e.g. after deserializing a
// prior result).
func (cp *ConfigProps) ClearReadOnlyProps() {
	cp.valid &^= PropIDsReadOnly
}

// SetsAny returns true if any of the specified properties are set in the ConfigProps
func (cp *ConfigProps) SetsAny(props ...PropIDs) bool {
	for _, p := range props {
		if cp.valid&p != 0 {
			return true
		}
	}
	return false
}

// MarshalJSON marshals the config properties to JSON
func (cp *ConfigProps) MarshalJSON() ([]byte, error) {
	m := cp.serializableMap()
	return json.Marshal(m)
}

// UnmarshalJSON deserializes a JSON map into ConfigProps.
// Each key present in the map calls the corresponding setter.
// Keys absent from the map leave properties unchanged.
func (cp *ConfigProps) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var tmp ConfigProps
	for key, val := range raw {
		switch key {
		case "signalsEnabled":
			var m map[string][]string
			if err := json.Unmarshal(val, &m); err != nil {
				return fmt.Errorf("signalsEnabled: %w", err)
			}
			ss, err := ParseSignalMap(m)
			if err != nil {
				return fmt.Errorf("signalsEnabled: %w", err)
			}
			tmp.SetSignalsEnabled(ss)
		case "timeGNSS":
			var s string
			if err := json.Unmarshal(val, &s); err != nil {
				return fmt.Errorf("timeGNSS: %w", err)
			}
			g, err := ParseGNSS(s)
			if err != nil {
				return fmt.Errorf("timeGNSS: %w", err)
			}
			tmp.SetTimeGNSS(g)
		case "timePulse":
			var tpRaw map[string]json.RawMessage
			if err := json.Unmarshal(val, &tpRaw); err != nil {
				return fmt.Errorf("timePulse: %w", err)
			}
			if err := tmp.unmarshalTimePulse(tpRaw); err != nil {
				return fmt.Errorf("timePulse: %w", err)
			}
		case "mode":
			var mRaw map[string]json.RawMessage
			if err := json.Unmarshal(val, &mRaw); err != nil {
				return fmt.Errorf("mode: %w", err)
			}
			if err := tmp.unmarshalMode(mRaw); err != nil {
				return fmt.Errorf("mode: %w", err)
			}
		case "antennaCableDelay":
			var nanos int64
			if err := json.Unmarshal(val, &nanos); err != nil {
				return fmt.Errorf("antennaCableDelay: %w", err)
			}
			tmp.SetAntennaCableDelay(time.Duration(nanos))
		case "navMsgAuth":
			var s string
			if err := json.Unmarshal(val, &s); err != nil {
				return fmt.Errorf("navMsgAuth: %w", err)
			}
			switch s {
			case "none":
				tmp.SetNavMsgAuth(NavMsgAuthNone)
			case "OSNMA":
				tmp.SetNavMsgAuth(NavMsgAuthOSNMA)
			default:
				return fmt.Errorf("navMsgAuth: unknown value %q", s)
			}
		case "rtcmBaseID":
			var id uint16
			if err := json.Unmarshal(val, &id); err != nil {
				return fmt.Errorf("rtcmBaseID: %w", err)
			}
			tmp.SetRTCMBaseID(id)
		case "minElevation":
			var deg float64
			if err := json.Unmarshal(val, &deg); err != nil {
				return fmt.Errorf("minElevation: %w", err)
			}
			tmp.SetMinElevation(DegreesFromFloat(deg))
		case "baudRate":
			var v uint32
			if err := json.Unmarshal(val, &v); err != nil {
				return fmt.Errorf("baudRate: %w", err)
			}
			tmp.SetBaudRate(v)
		case "port":
			var s string
			if err := json.Unmarshal(val, &s); err != nil {
				return fmt.Errorf("port: %w", err)
			}
			tmp.SetPort(s)
		default:
			return fmt.Errorf("unknown property %q", key)
		}
	}
	cp.CopyFrom(&tmp)
	return nil
}

func (cp *ConfigProps) unmarshalTimePulse(raw map[string]json.RawMessage) error {
	for key, val := range raw {
		switch key {
		case "width":
			var secs float64
			if err := json.Unmarshal(val, &secs); err != nil {
				return fmt.Errorf("width: %w", err)
			}
			cp.SetTimePulseWidth(time.Duration(secs * float64(time.Second)))
		case "period":
			var secs float64
			if err := json.Unmarshal(val, &secs); err != nil {
				return fmt.Errorf("period: %w", err)
			}
			cp.SetTimePulsePeriod(time.Duration(secs * float64(time.Second)))
		case "alignToGNSS":
			var b bool
			if err := json.Unmarshal(val, &b); err != nil {
				return fmt.Errorf("alignToGNSS: %w", err)
			}
			cp.SetTimePulseAlignToGNSS(b)
		case "onlyWhenLocked":
			var b bool
			if err := json.Unmarshal(val, &b); err != nil {
				return fmt.Errorf("onlyWhenLocked: %w", err)
			}
			cp.SetTimePulseOnlyWhenLocked(b)
		case "polarityRising":
			var b bool
			if err := json.Unmarshal(val, &b); err != nil {
				return fmt.Errorf("polarityRising: %w", err)
			}
			cp.SetTimePulsePolarityRising(b)
		default:
			return fmt.Errorf("unknown field %q", key)
		}
	}
	return nil
}

func (cp *ConfigProps) unmarshalMode(raw map[string]json.RawMessage) error {
	var m Mode
	hasECEF := false
	hasLLH := false
	for key, val := range raw {
		switch key {
		case "static":
			if err := json.Unmarshal(val, &m.Static); err != nil {
				return fmt.Errorf("static: %w", err)
			}
		case "fixedPosECEF":
			var coords [3]float64
			if err := json.Unmarshal(val, &coords); err != nil {
				return fmt.Errorf("fixedPosECEF: %w", err)
			}
			for i := range 3 {
				m.FixedPosECEF[i] = Meters(coords[i])
			}
			hasECEF = true
		case "fixedPosLLH":
			var coords [2]float64
			if err := json.Unmarshal(val, &coords); err != nil {
				return fmt.Errorf("fixedPosLLH: %w", err)
			}
			m.FixedPosLLH[0] = DegreesFromFloat(coords[0])
			m.FixedPosLLH[1] = DegreesFromFloat(coords[1])
			hasLLH = true
		case "height":
			var meters float64
			if err := json.Unmarshal(val, &meters); err != nil {
				return fmt.Errorf("height: %w", err)
			}
			m.Height = Meters(meters)
		case "fixedPosAcc":
			var meters float64
			if err := json.Unmarshal(val, &meters); err != nil {
				return fmt.Errorf("fixedPosAcc: %w", err)
			}
			m.FixedPosAcc = Meters(meters)
		default:
			return fmt.Errorf("unknown field %q", key)
		}
	}
	switch {
	case hasECEF:
		m.PosType = PosTypeECEF
	case hasLLH:
		m.PosType = PosTypeLLH
	}
	cp.SetMode(m)
	return nil
}

// MarshalText marshals the config properties to text
func (cp *ConfigProps) MarshalText() ([]byte, error) {
	return []byte(cp.String()), nil
}

// String returns a string representation of the configuration
func (cp *ConfigProps) String() string {
	m := cp.serializableMap()
	return fmt.Sprint(m)
}

// CopyFrom copies the properties in other to this ConfigProps.
// Properties that are not set in other are unchanged in cp.
func (cp *ConfigProps) CopyFrom(other *ConfigProps) {
	if cp == other || other == nil {
		return
	}
	if other.valid&PropIDSignalsEnabled != 0 {
		cp.signalsEnabled = other.signalsEnabled
	}
	if other.valid&PropIDTimeGNSS != 0 {
		cp.timeGNSS = other.timeGNSS
	}
	if other.valid&PropIDTimePulseWidth != 0 {
		cp.timePulse.Width = other.timePulse.Width
	}
	if other.valid&PropIDTimePulsePeriod != 0 {
		cp.timePulse.Period = other.timePulse.Period
	}
	if other.valid&PropIDTimePulseAlignToGNSS != 0 {
		cp.timePulse.AlignToGNSS = other.timePulse.AlignToGNSS
	}
	if other.valid&PropIDTimePulseOnlyWhenLocked != 0 {
		cp.timePulse.OnlyWhenLocked = other.timePulse.OnlyWhenLocked
	}
	if other.valid&PropIDTimePulsePolarityRising != 0 {
		cp.timePulse.PolarityRising = other.timePulse.PolarityRising
	}
	if other.valid&PropIDMode != 0 {
		cp.mode = other.mode
	}
	if other.valid&PropIDAntennaCableDelay != 0 {
		cp.antennaCableDelay = other.antennaCableDelay
	}
	if other.valid&PropIDNavMsgAuth != 0 {
		cp.navMsgAuth = other.navMsgAuth
	}
	if other.valid&PropIDRTCMBaseID != 0 {
		cp.rtcmBaseID = other.rtcmBaseID
	}
	if other.valid&PropIDMinElevation != 0 {
		cp.minElevation = other.minElevation
	}
	if other.valid&PropIDBaudRate != 0 {
		cp.baudRate = other.baudRate
	}
	if other.valid&PropIDPort != 0 {
		cp.port = other.port
	}
	cp.valid |= other.valid
}

// Inconsistent returns a ConfigProps with entries in other that are inconsistent with this one.
// An entry is inconsistent if it exists in both ConfigProps but has different values.
func (cp *ConfigProps) Inconsistent(other *ConfigProps) *ConfigProps {
	result := new(ConfigProps)

	// Only check fields that are valid in both configs
	both := cp.valid & other.valid

	// Check each property that's valid in both
	if both&PropIDSignalsEnabled != 0 && cp.signalsEnabled != other.signalsEnabled {
		result.SetSignalsEnabled(other.signalsEnabled)
	}
	if both&PropIDTimeGNSS != 0 && cp.timeGNSS != other.timeGNSS {
		result.SetTimeGNSS(other.timeGNSS)
	}
	if both&PropIDTimePulseWidth != 0 && cp.timePulse.Width != other.timePulse.Width {
		result.SetTimePulseWidth(other.timePulse.Width)
	}
	if both&PropIDTimePulsePeriod != 0 && cp.timePulse.Period != other.timePulse.Period {
		result.SetTimePulsePeriod(other.timePulse.Period)
	}
	if both&PropIDTimePulseAlignToGNSS != 0 && cp.timePulse.AlignToGNSS != other.timePulse.AlignToGNSS {
		result.SetTimePulseAlignToGNSS(other.timePulse.AlignToGNSS)
	}
	if both&PropIDTimePulseOnlyWhenLocked != 0 && cp.timePulse.OnlyWhenLocked != other.timePulse.OnlyWhenLocked {
		result.SetTimePulseOnlyWhenLocked(other.timePulse.OnlyWhenLocked)
	}
	if both&PropIDTimePulsePolarityRising != 0 && cp.timePulse.PolarityRising != other.timePulse.PolarityRising {
		result.SetTimePulsePolarityRising(other.timePulse.PolarityRising)
	}
	if both&PropIDMode != 0 && cp.mode != other.mode {
		result.SetMode(other.mode)
	}
	if both&PropIDAntennaCableDelay != 0 && cp.antennaCableDelay != other.antennaCableDelay {
		result.SetAntennaCableDelay(other.antennaCableDelay)
	}
	if both&PropIDNavMsgAuth != 0 && cp.navMsgAuth != other.navMsgAuth {
		result.SetNavMsgAuth(other.navMsgAuth)
	}
	if both&PropIDRTCMBaseID != 0 && cp.rtcmBaseID != other.rtcmBaseID {
		result.SetRTCMBaseID(other.rtcmBaseID)
	}
	if both&PropIDMinElevation != 0 && cp.minElevation != other.minElevation {
		result.SetMinElevation(other.minElevation)
	}
	if both&PropIDBaudRate != 0 && cp.baudRate != other.baudRate {
		result.SetBaudRate(other.baudRate)
	}
	if both&PropIDPort != 0 && cp.port != other.port {
		result.SetPort(other.port)
	}
	return result
}

// Missing returns a ConfigProps with entries from other that are missing from this one.
// An entry is missing if it exists in other but not in this ConfigProps.
func (cp *ConfigProps) Missing(other *ConfigProps) *ConfigProps {
	result := new(ConfigProps)

	// Copy all values from other
	*result = *other

	// Keep only the properties that are in other but not in this one
	result.valid = other.valid &^ cp.valid

	return result
}

// serializableMap converts the valid properties to a map suitable for serialization
func (cp *ConfigProps) serializableMap() map[string]interface{} {
	m := make(map[string]interface{})

	if cp.valid&PropIDSignalsEnabled != 0 {
		m["signalsEnabled"] = cp.signalsEnabled.GNSSSignalMap()
	}
	if cp.valid&PropIDTimeGNSS != 0 {
		m["timeGNSS"] = cp.timeGNSS
	}
	if cp.valid&timePulseProps != 0 {
		tpm := make(map[string]interface{})
		if cp.valid&PropIDTimePulseWidth != 0 {
			tpm["width"] = float64(cp.timePulse.Width) / float64(time.Second)
		}
		if cp.valid&PropIDTimePulsePeriod != 0 {
			tpm["period"] = float64(cp.timePulse.Period) / float64(time.Second)
		}
		if cp.valid&PropIDTimePulseAlignToGNSS != 0 {
			tpm["alignToGNSS"] = cp.timePulse.AlignToGNSS
		}
		if cp.valid&PropIDTimePulseOnlyWhenLocked != 0 {
			tpm["onlyWhenLocked"] = cp.timePulse.OnlyWhenLocked
		}
		if cp.valid&PropIDTimePulsePolarityRising != 0 {
			tpm["polarityRising"] = cp.timePulse.PolarityRising
		}
		m["timePulse"] = tpm
	}
	if cp.valid&PropIDMode != 0 {
		mm := make(map[string]interface{})
		mm["static"] = cp.mode.Static
		if cp.mode.PosType == PosTypeECEF {
			mm["fixedPosECEF"] = []float64{
				cp.mode.FixedPosECEF[0].Meters(),
				cp.mode.FixedPosECEF[1].Meters(),
				cp.mode.FixedPosECEF[2].Meters(),
			}
		}
		if cp.mode.PosType == PosTypeLLH {
			mm["fixedPosLLH"] = []float64{
				cp.mode.FixedPosLLH[0].Degrees(),
				cp.mode.FixedPosLLH[1].Degrees(),
			}
			mm["height"] = cp.mode.Height.Meters()
		}
		if cp.mode.PosType != PosTypeNone && cp.mode.FixedPosAcc != 0 {
			mm["fixedPosAcc"] = cp.mode.FixedPosAcc.Meters()
		}
		m["mode"] = mm
	}
	if cp.valid&PropIDAntennaCableDelay != 0 {
		m["antennaCableDelay"] = cp.antennaCableDelay.Nanoseconds()
	}
	if cp.valid&PropIDNavMsgAuth != 0 {
		switch cp.navMsgAuth {
		case NavMsgAuthNone:
			m["navMsgAuth"] = "none"
		case NavMsgAuthOSNMA:
			m["navMsgAuth"] = "OSNMA"
		}
	}
	if cp.valid&PropIDRTCMBaseID != 0 {
		m["rtcmBaseID"] = cp.rtcmBaseID
	}
	if cp.valid&PropIDMinElevation != 0 {
		m["minElevation"] = cp.minElevation.Degrees()
	}
	if cp.valid&PropIDBaudRate != 0 {
		m["baudRate"] = cp.baudRate
	}
	if cp.valid&PropIDPort != 0 {
		m["port"] = cp.port
	}
	return m
}

// SetPPS configures the properties for a pulse-per-second output
func (cp *ConfigProps) SetPPS(width time.Duration) {
	cp.SetTimePulse(TimePulse{
		Period:         1 * time.Second,
		Width:          width,
		PolarityRising: true,
		AlignToGNSS:    true,
		OnlyWhenLocked: true,
	})
}

type SurveyFlags int

const (
	SurveyAgain SurveyFlags = 1 << iota // do a survey even if we have done one already
)

var surveyJSON = []jsonFlag[SurveyFlags]{
	{"again", SurveyAgain},
}

// MarshalJSON marshals SurveyFlags as an array of names.
func (f SurveyFlags) MarshalJSON() ([]byte, error) {
	return marshalJSONFlags(f, surveyJSON, "survey")
}

// UnmarshalJSON unmarshals SurveyFlags from an array of names.
func (f *SurveyFlags) UnmarshalJSON(data []byte) error {
	return unmarshalJSONFlags(data, f, surveyJSON, "survey")
}

// Survey specifies the parameters for performing a survey-in.
type Survey struct {
	Flags    SurveyFlags   // control survey behavior
	MinDur   time.Duration // survey should run at least this long
	AccLimit Length        // survey should run until this accuracy is achieved
}

type TimeMode byte

const (
	TimeModeDisabled TimeMode = iota + 1
	TimeModeSurvey
	TimeModeFixed
)

type TimeModeSet byte

const (
	TimeModeNone TimeModeSet = 0
	TimeModeAny  TimeModeSet = 1<<TimeModeDisabled | 1<<TimeModeSurvey | 1<<TimeModeFixed
)

func (m TimeModeSet) Contains(t TimeMode) bool {
	return m&(1<<t) != 0
}

func TimeModeFlags(ms ...TimeMode) TimeModeSet {
	flags := TimeModeSet(0)
	for _, m := range ms {
		flags |= 1 << m
	}
	return flags
}

type PosType byte

const (
	PosTypeNone PosType = iota
	PosTypeECEF
	PosTypeLLH
)

type Mode struct {
	Static       bool     // true if receiver should assume antenna position does not move
	PosType      PosType  // which coordinate system to use for fixed position
	FixedPosECEF Point3D  // ECEF coordinates (when PosType == PosTypeECEF)
	FixedPosLLH  [2]Angle // Latitude and Longitude (when PosType == PosTypeLLH)
	Height       Length   // Height (when PosType == PosTypeLLH)
	FixedPosAcc  Length   // accuracy of fixed position; 0 = no accuracy stated (a receiver that stores none reads back 0; the JSON form omits the key instead)
}

func (m Mode) IsZero() bool {
	return m == Mode{}
}

type NavMsgAuth byte

const (
	NavMsgAuthNone NavMsgAuth = iota // no authentication
	NavMsgAuthOSNMA
)

// TimeEstimate represents an estimate of the current UTC time that can be provided to the GPS receiver.
// An EstimatedTime of zero means that no estimate is available.
// If TimeOfEstimate is non-zero, the EstimatedTime to be sent to the GPS receiver will be adjusted by the
// elapsed time between the time at which the estimate is sent and TimeOfEstimate.
// If TimeOfEstimate is zero, then the EstimatedTime is sent as-is, which is useful for testing purposes.
// For a system time generated by time.Now(), EstimatedTime and TimeOfEstimate will be the same.
// If EstimatedTime is non-zero, then the Accuracy must also be non-zero.
type TimeEstimate struct {
	EstimatedTime  time.Time             // the estimated wall-clock time
	TimeOfEstimate time.Time             // the monotonic time at which the estimate was made
	Accuracy       time.Duration         // the accuracy of the estimate
	LeapSecond     ptime.LeapSecondState // leap second information, if known
	Trusted        bool                  // whether the estimate is trusted
}
