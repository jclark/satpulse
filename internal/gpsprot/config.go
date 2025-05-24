package gpsprot

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

// PacketExchanger manages the processing and generation of packets for a GPS receiver.
// An implementation of protocol captures the semantics of a particular GPS receiver protocol,
// such as UBX or NMEA.
// Methods on PacketExchanger do not themselves send or receive.
// A PacketExchanger is attached to a PacketProcessor when it is created
// by PacketProcessor.CreatePacketExchanger and installs a NativeMsgHandler.
// Splitting sequences of bytes in packets is handled by the scan package.
// Packets to be processed are passed to ProcessPacket method on the PacketProcessor.
// There are two stages: probing and configuration.
// In probing, ProbePacket returns a packet to be sent to the GPS receiver;
// ProbeOK as soon as a packet has been processed that indicates the GPS receiver is responding.
// Configuration is managed by a Configurator created by Configure;
// repeated calls to NextRequest return the next request to be sent to the GPS receiver.
// After a Configurator has been created, calls to ProcessPacket will pass configuration-related packets
// to the Configurator via the NativeMsgHandler.
// The packets processed by ProcessPacket determine the requests returned by NextRequest.
type PacketExchanger interface {
	ProbePacket() []byte
	ProbeOK() bool
	Configure(*ConfigTarget) (Configurator, error)
}

type Ack struct {
	// OK is true if the acknowledgement was an ACK rather than a NACK.
	OK bool
	// TRead is the time at which the acknowledgement was received.
	TRead time.Time
}

// Configurator manages the generation and interpretation of configuration-related packets.
// It is created by the Configure method of a Protocol.
// It will receive packets to interpret via calls to ProcessPacket on the Protocol that created it.
// If the Ackable method of a ConfigRequest returns true, then FindAck should be used
// to find whether the acknowledgement has been received.
type Configurator interface {
	// ConfigProps returns the current configuration of the GPS receiver.
	// It should be called after NextRequest returns nil.
	ConfigProps() *ConfigProps
	// NextRequest returns the next request that should be sent to the GPS receiver.
	// If there are no more requests, it returns nil, nil.
	NextRequest() (ConfigRequest, error)
	FindAck(packet []byte, tSent time.Time) *Ack
}

// ConfigRequest represents a request to be sent to the GPS receiver to configure it.
// The request may expect an acknowledgement.
// The request may be getting aspects of the current configuration,
// or setting aspects of the configuration.
// A ConfigRequest is associated with the Configurator that created it.
type ConfigRequest interface {
	// Packet returns the packet to be sent to the GPS receiver.
	Packet() []byte
	// ChangeSpeed returns the serial port speed to change to after the request is sent, or 0 if no change is needed
	ChangeSpeed() int
	// Ackable returns true if the packet is one that needs an acknowledgement.
	// An acknowledgement can either be an ACK or a NACK.
	Ackable() bool
	// Done should be called when the request has been sent and the acknowledgement received,
	// and the acknowledge was an ACK.
	// This causes the Configurator to update its state to reflect that the configuration has been applied.
	Done()
	// ID returns a human-readable identifier for the request type.
	ID() string
	AwaitingResponse(tSent time.Time) bool
}

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
	Width          time.Duration
	Period         time.Duration
	AlignToGNSS    bool
	OnlyWhenLocked bool
	PolarityRising bool
}

// ConfigProps represents a collection configuration properties
type ConfigProps struct {
	valid             PropIDs // says which fields are valid
	signalsEnabled    SignalSet
	primaryGNSS       GNSS
	solutionPeriod    time.Duration
	timePulse         TimePulse
	timeMode          TimeMode
	antennaCableDelay time.Duration
	fixedPosECEF      Point3D
	fixedPosAcc       Length
	stationary        bool
	nmeaEnabled       bool
	baudRate          uint32
}

const (
	PropIDSignalsEnabled PropIDs = 1 << iota
	PropIDPrimaryGNSS
	PropIDSolutionPeriod
	// eventually the individual time pulse properties will be combined into a single property
	PropIDTimePulseWidth
	PropIDTimePulsePeriod
	PropIDTimePulseAlignToGNSS
	PropIDTimePulseOnlyWhenLocked
	PropIDTimePulsePolarityRising
	PropIDTimeMode
	PropIDAntennaCableDelay
	PropIDFixedPosECEF
	PropIDFixedPosAcc
	PropIDStationary
	PropIDNMEAEnabled
	PropIDBaudRate
)

// propNames lists the property names in the same order as the bit constants
var propNames = []string{
	"SignalsEnabled",
	"PrimaryGNSS",
	"SolutionPeriod",
	"TimePulseWidth",
	"TimePulsePeriod",
	"TimePulseAlignToGNSS",
	"TimePulseOnlyWhenLocked",
	"TimePulsePolarityRising",
	"TimeMode",
	"AntennaCableDelay",
	"FixedPosECEF",
	"FixedPosAcc",
	"Stationary",
	"NMEAEnabled",
	"BaudRate",
}

// IsEmpty returns true if no properties are set
func (cp *ConfigProps) IsEmpty() bool {
	return cp.valid == 0
}

type MsgStatus uint8

const (
	MsgStatusUnchanged MsgStatus = iota
	MsgStatusEnabled
	MsgStatusDisabled
)

func (status MsgStatus) IsZero() bool {
	return status == MsgStatusUnchanged
}

func (status MsgStatus) IsSet() bool {
	return status != MsgStatusUnchanged
}

func (status MsgStatus) IsEnabled() bool {
	return status == MsgStatusEnabled
}

type SaveType uint8

const (
	SaveNone    SaveType = iota
	SaveMinimal          // save the minimum to save the configuration changes specified in ConfigTarget
	SaveAll              // save the current configuration
)

type ResetType uint8

const (
	ResetNone    ResetType = iota
	ResetCold              // restore configuration from non-volatile memory and perform a cold start
	ResetFactory           // restore non-volatile memory to factory defaults and then ResetCold
)

type ConfigOptions struct {
	Detected            bool      // has already been detected, no need to detect it again
	ForceProbe          bool      // force probe even if no input has been detected
	Save                SaveType  // what to save to non-volatile memory
	Reset               ResetType // what kind of reset to perform
	EnableLeapSecondMsg bool
	EnableTimeMsg       bool
	SatellitesMsg       MsgStatus
	Survey              Survey
}

func NewConfigTarget(config bool) *ConfigTarget {
	t := &ConfigTarget{}
	if !config {
		return t
	}
	t.Props.SetPPS()
	t.Props.SetNMEAEnabled(false) // Config is very slow on 8-th gen if NMEA is enabled
	t.Opts.EnableLeapSecondMsg = true
	t.Opts.EnableTimeMsg = true
	return t
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
	return o.Save == 0 && o.Reset == 0 && !o.EnableLeapSecondMsg && !o.EnableTimeMsg && o.SatellitesMsg.IsZero() && o.Survey.When == TimeModeNone
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

// GetPrimaryGNSS returns the primaryGNSS value and whether it's set
func (cp *ConfigProps) GetPrimaryGNSS() (GNSS, bool) {
	if cp.valid&PropIDPrimaryGNSS != 0 {
		return cp.primaryGNSS, true
	}
	return 0, false
}

// SetPrimaryGNSS sets the primaryGNSS value
func (cp *ConfigProps) SetPrimaryGNSS(val GNSS) {
	cp.primaryGNSS = val
	cp.valid |= PropIDPrimaryGNSS
}

// GetSolutionPeriod returns the solutionPeriod value and whether it's set
func (cp *ConfigProps) GetSolutionPeriod() (time.Duration, bool) {
	if cp.valid&PropIDSolutionPeriod != 0 {
		return cp.solutionPeriod, true
	}
	return 0, false
}

// SetSolutionPeriod sets the solutionPeriod value
func (cp *ConfigProps) SetSolutionPeriod(val time.Duration) {
	cp.solutionPeriod = val
	cp.valid |= PropIDSolutionPeriod
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

// GetTimeMode returns the timeMode value and whether it's set
func (cp *ConfigProps) GetTimeMode() (TimeMode, bool) {
	if cp.valid&PropIDTimeMode != 0 {
		return cp.timeMode, true
	}
	return 0, false
}

// SetTimeMode sets the timeMode value
func (cp *ConfigProps) SetTimeMode(val TimeMode) {
	cp.timeMode = val
	cp.valid |= PropIDTimeMode
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

// GetFixedPosECEF returns the fixedPosECEF value and whether it's set
func (cp *ConfigProps) GetFixedPosECEF() (Point3D, bool) {
	if cp.valid&PropIDFixedPosECEF != 0 {
		return cp.fixedPosECEF, true
	}
	return Point3D{}, false
}

// SetFixedPosECEF sets the fixedPosECEF value
func (cp *ConfigProps) SetFixedPosECEF(val Point3D) {
	cp.fixedPosECEF = val
	cp.valid |= PropIDFixedPosECEF
}

// GetFixedPosAcc returns the fixedPosAcc value and whether it's set
func (cp *ConfigProps) GetFixedPosAcc() (Length, bool) {
	if cp.valid&PropIDFixedPosAcc != 0 {
		return cp.fixedPosAcc, true
	}
	return 0, false
}

// SetFixedPosAcc sets the fixedPosAcc value
func (cp *ConfigProps) SetFixedPosAcc(val Length) {
	cp.fixedPosAcc = val
	cp.valid |= PropIDFixedPosAcc
}

// GetStationary returns the stationary value and whether it's set
func (cp *ConfigProps) GetStationary() (bool, bool) {
	if cp.valid&PropIDStationary != 0 {
		return cp.stationary, true
	}
	return false, false
}

// SetStationary sets the stationary value
func (cp *ConfigProps) SetStationary(val bool) {
	cp.stationary = val
	cp.valid |= PropIDStationary
}

// GetNMEAEnabled returns the nmeaEnabled value and whether it's set
func (cp *ConfigProps) GetNMEAEnabled() (bool, bool) {
	if cp.valid&PropIDNMEAEnabled != 0 {
		return cp.nmeaEnabled, true
	}
	return false, false
}

// SetNMEAEnabled sets the nmeaEnabled value
func (cp *ConfigProps) SetNMEAEnabled(val bool) {
	cp.nmeaEnabled = val
	cp.valid |= PropIDNMEAEnabled
}

// GetBaudRate returns the baudRate value and whether it's set
func (cp *ConfigProps) GetBaudRate() (uint32, bool) {
	if cp.valid&PropIDBaudRate != 0 {
		return cp.baudRate, true
	}
	return 0, false
}

// SetBaudRate sets the baudRate value
func (cp *ConfigProps) SetBaudRate(val uint32) {
	cp.baudRate = val
	cp.valid |= PropIDBaudRate
}

// MarshalJSON marshals the config properties to JSON
func (cp *ConfigProps) MarshalJSON() ([]byte, error) {
	m := cp.serializableMap()
	return json.Marshal(m)
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
	if both&PropIDPrimaryGNSS != 0 && cp.primaryGNSS != other.primaryGNSS {
		result.SetPrimaryGNSS(other.primaryGNSS)
	}
	if both&PropIDSolutionPeriod != 0 && cp.solutionPeriod != other.solutionPeriod {
		result.SetSolutionPeriod(other.solutionPeriod)
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
	if both&PropIDTimeMode != 0 && cp.timeMode != other.timeMode {
		result.SetTimeMode(other.timeMode)
	}
	if both&PropIDAntennaCableDelay != 0 && cp.antennaCableDelay != other.antennaCableDelay {
		result.SetAntennaCableDelay(other.antennaCableDelay)
	}
	if both&PropIDFixedPosECEF != 0 && cp.fixedPosECEF != other.fixedPosECEF {
		result.SetFixedPosECEF(other.fixedPosECEF)
	}
	if both&PropIDFixedPosAcc != 0 && cp.fixedPosAcc != other.fixedPosAcc {
		result.SetFixedPosAcc(other.fixedPosAcc)
	}
	if both&PropIDStationary != 0 && cp.stationary != other.stationary {
		result.SetStationary(other.stationary)
	}
	if both&PropIDNMEAEnabled != 0 && cp.nmeaEnabled != other.nmeaEnabled {
		result.SetNMEAEnabled(other.nmeaEnabled)
	}
	if both&PropIDBaudRate != 0 && cp.baudRate != other.baudRate {
		result.SetBaudRate(other.baudRate)
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
		m["signalsEnabled"] = cp.signalsEnabled.GNSSStringGroups()
	}
	if cp.valid&PropIDPrimaryGNSS != 0 {
		m["primaryGNSS"] = cp.primaryGNSS
	}
	if cp.valid&PropIDSolutionPeriod != 0 {
		m["solutionPeriod"] = float64(cp.solutionPeriod) / float64(time.Second)
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
	if cp.valid&PropIDTimeMode != 0 {
		switch cp.timeMode {
		case TimeModeDisabled:
			m["timeMode"] = "disabled"
		case TimeModeSurvey:
			m["timeMode"] = "survey"
		case TimeModeFixed:
			m["timeMode"] = "fixed"
		default:
			m["timeMode"] = cp.timeMode
		}
	}
	if cp.valid&PropIDAntennaCableDelay != 0 {
		m["antennaCableDelay"] = float64(cp.antennaCableDelay) / float64(time.Second)
	}
	if cp.valid&PropIDFixedPosECEF != 0 {
		m["fixedPosECEF"] = []float64{
			cp.fixedPosECEF[0].Meters(),
			cp.fixedPosECEF[1].Meters(),
			cp.fixedPosECEF[2].Meters(),
		}
	}
	if cp.valid&PropIDFixedPosAcc != 0 {
		m["fixedPosAcc"] = float64(cp.fixedPosAcc) / float64(Meter)
	}
	if cp.valid&PropIDStationary != 0 {
		m["stationary"] = cp.stationary
	}
	if cp.valid&PropIDNMEAEnabled != 0 {
		m["nmeaEnabled"] = cp.nmeaEnabled
	}
	if cp.valid&PropIDBaudRate != 0 {
		m["baudRate"] = cp.baudRate
	}
	return m
}

// SetPPS configures the properties for a pulse-per-second output
func (cp *ConfigProps) SetPPS() {
	cp.SetSolutionPeriod(1 * time.Second)
	cp.SetTimePulse(TimePulse{
		Period:         1 * time.Second,
		Width:          time.Second / 10,
		PolarityRising: true,
		AlignToGNSS:    true,
		OnlyWhenLocked: true,
	})
}

type Length int64

const (
	Micrometer Length = 1
	Millimeter Length = 1000 * Micrometer
	Centimeter Length = 10 * Millimeter
	Meter      Length = 100 * Centimeter
)

func Meters(f float64) Length {
	return Length(math.Round(f * float64(Meter)))
}

func (l Length) Meters() float64 {
	return float64(l) / float64(Meter)
}

func (l Length) String() string {
	return fmt.Sprintf("%v", l.Meters())
}

func ParseLength(s string) (Length, error) {
	var f float64
	var trailing string
	if n, err := fmt.Sscanf(s, "%f%s", &f, &trailing); n != 1 || err != io.EOF {
		return 0, fmt.Errorf("invalid length: %q", s)
	}
	n, err := float64ToInt64(math.Round(f * float64(Meter)))
	if err != nil {
		return 0, fmt.Errorf("invalid length %f: %w", f, err)
	}
	return Length(n), nil
}

type Point3D [3]Length

func (p Point3D) String() string {
	var s [3]string
	for i := 0; i < 3; i++ {
		s[i] = strconv.FormatFloat(p[i].Meters(), 'f', -1, 64)
	}
	return fmt.Sprintf("%s,%s,%s", s[0], s[1], s[2])
}

func ParsePoint3D(s string) (Point3D, error) {
	var p Point3D
	var trailing string
	var f [3]float64
	if n, err := fmt.Sscanf(s, "%f,%f,%f%s", &f[0], &f[1], &f[2], &trailing); n != 3 || err != io.EOF {
		return p, fmt.Errorf("invalid 3D coordinates: %q", s)
	}
	for i := 0; i < 3; i++ {
		n, err := float64ToInt64(math.Round(f[i] * float64(Meter)))
		if err != nil {
			return Point3D{}, fmt.Errorf("invalid coordinate %f: %w", f[i], err)
		}
		p[i] = Length(n)
	}
	return p, nil
}

// Survey specifies whether a survey should be performed, and if so, its parameters
// The survey is performed when the time mode is one of the modes in When.
// If When is non-zero, then AccLimit must also be non-zero.
type Survey struct {
	When     TimeModeSet   // perform a survey when the time mode is one of these
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
