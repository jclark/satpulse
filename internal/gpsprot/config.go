package gpsprot

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
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
// When configuration errors occur, NextRequest may return an error but the caller should
// continue calling NextRequest to allow recovery operations; ConfigProps should be called
// even after errors to see what configuration was achieved.
type Configurator interface {
	// ConfigProps returns the current configuration of the GPS receiver.
	// It should be called after NextRequest returns nil.
	ConfigProps() *ConfigProps
	// ReceiverInfo returns static information about the GPS receiver.
	ReceiverInfo() *ReceiverInfo
	// NextRequest returns the next request that should be sent to the GPS receiver.
	// If there are no more requests, it returns nil, nil.
	// If the error is non-nil, then ConfigRequest will be nil;
	// this indicates an error generating the request; the caller should report
	// the error, but continue calling NextRequest to allow recovery operations.
	NextRequest() (ConfigRequest, error)
	// FindAck will search for acknowledgement response for the request.
	// It must be called after the request has been sent.
	// Finding a negative acknowledgement (ie. Ack.OK is false) usually causes the
	// Configurator to initiate recovery (which may involve sending more requests).
	// It is the caller's responsibility to report the negative acknowledgement.
	FindAck(packet []byte, tSent time.Time) *Ack
	// Abort informs the Configurator that configuration should be aborted.
	// This typically triggers recovery operations on the next NextRequest call.
	// This should be called if a request does not get a response or an ACK within a reasonable time.
	// It doesn't need to be called if it got a NACK.
	Abort()
}

// ReceiverInfo provides static information about the GPS receiver.
type ReceiverInfo struct {
	Vendor          string      `json:"vendor"`          // receiver vendor (e.g., "u-blox")
	Firmware        string      `json:"firmware"`        // information about firmware; for u-blox, format would be e.g. "TIM 2.20 PROTVER 18.00"
	Hardware        string      `json:"hardware"`        // information about hardware; for u-blox, this is the model (e.g., "ZED-F9T")
	SupportedGNSS   GNSSSet     `json:"supportedGNSS"`   // supported GNSS constellations
	VendorSpecific  interface{} `json:"-"`               // vendor-specific information, excluded from JSON
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
	// Pause returns the time to wait after receiving the packet ACK before sending the next packet.
	Pause() time.Duration
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
	timeGNSS          GNSS
	timePulse         TimePulse
	mode              Mode
	antennaCableDelay time.Duration
	navMsgAuth        NavMsgAuth
	rtcmBaseID        uint16
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
	PropIDTimePulse PropIDs = PropIDTimePulseWidth | PropIDTimePulsePeriod |
		PropIDTimePulseAlignToGNSS | PropIDTimePulseOnlyWhenLocked | PropIDTimePulsePolarityRising
)

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

type ResetType uint8

const (
	ResetNone    ResetType = iota
	ResetReload            // reload the configuration from non-volatile memory without a reset/start (if possible)
	ResetCold              // restore configuration from non-volatile memory and perform a cold start
	ResetFactory           // restore non-volatile memory to factory defaults and then ResetCold
)

type Option[T any] struct {
	val T
	set bool
}

func (o *Option[T]) Set(v T)      { o.set, o.val = true, v }
func (o *Option[T]) Get() T       { return o.val }
func (o *Option[T]) Clear()       { var zero T; o.set, o.val = false, zero }
func (o *Option[T]) IsSet() bool  { return o.set }
func (o *Option[T]) IsZero() bool { return !o.set }

// MakeOption creates an Option with the given value set
func MakeOption[T any](v T) Option[T] {
	var opt Option[T]
	opt.Set(v)
	return opt
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
	PVTMsgOff                                    // turn off any unneeded PVT messages (option)
)

const PVTMsgAny PVTMsgFlags = PVTMsgPos | PVTMsgVel | PVTMsgTime | PVTMsgTimePulse | PVTMsgLeapSecond | PVTMsgSurvey // any message (not option)

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

type NMEAMsgFlags uint16

const (
	NMEAMsgRMC NMEAMsgFlags = 1 << iota
	NMEAMsgGGA
	NMEAMsgGSA
	NMEAMsgGSV
	NMEAMsgZDA
	NMEAMsgVTG
	NMEAMsgOther NMEAMsgFlags = 1 << 15 // other unspecified NMEA messages
	// may have flags like NMEA version or rate in the future
	NMEAMsgNone NMEAMsgFlags = 0
	NMEAMsgAny  NMEAMsgFlags = NMEAMsgRMC | NMEAMsgGGA | NMEAMsgGSA | NMEAMsgGSV | NMEAMsgZDA | NMEAMsgVTG | NMEAMsgOther // any message (not flag)
)

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
	RTCMMsgNone RTCMMsgFlags = 0
	RTCMMsgAuto RTCMMsgFlags = RTCMMsgMSM4 | RTCMMsgARP | RTCMMsgLax                 // enable intelligently
	RTCMMsgAny  RTCMMsgFlags = RTCMMsgMSM4 | RTCMMsgMSM7 | RTCMMsgARP | RTCMMsgOther // any message (not flag)
)

type RawMsgFlags uint8

const (
	RawMsgObs     RawMsgFlags = 1 << iota // Raw observation messages (for RINEX)
	RawMsgNavData                         // Raw navigation date e.g. subframes for GPS
	RawMsgNone    RawMsgFlags = 0
	RawMsgAny     RawMsgFlags = RawMsgObs | RawMsgNavData // any message (not flag)
)

type ForceProbe uint8

const (
	ForceProbeWhenNoOutput ForceProbe = 1 << iota // force probe even if no input has been detected
	ForceProbeWhenNoConfig                        // force probe even when no config changes needed
)

type ConfigOptions struct {
	Detected   bool        // has already been detected, no need to detect it again
	ForceProbe ForceProbe  // control when to force probing
	Save       SaveType    // what to save to non-volatile memory
	Reset      ResetType   // what kind of reset to perform
	PVTMsg     PVTMsgFlags // messages relating to Position, Velocity, and Time
	NMEAMsg    Option[NMEAMsgFlags]
	RTCMMsg    Option[RTCMMsgFlags] // RTCM 3.x messages
	SatsMsg    Option[SatsMsgFlags]
	RawMsg     Option[RawMsgFlags]
	Survey     Survey
	SetStatic  bool         // ensure receiver is in static mode without changing existing fixed position
	BaudRate   uint32       // serial port baud rate, 0 means do not change
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
		if cp.mode.PosType != PosTypeNone {
			mm["fixedPosAcc"] = cp.mode.FixedPosAcc.Meters()
		}
		m["mode"] = mm
	}
	if cp.valid&PropIDAntennaCableDelay != 0 {
		m["antennaCableDelay"] = float64(cp.antennaCableDelay) / float64(time.Second)
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

type Angle int64

const (
	Nanodegrees  Angle = 1
	Microdegrees Angle = 1000 * Nanodegrees
	Millidegrees Angle = 1000 * Microdegrees
	Degrees      Angle = 1000 * Millidegrees
)

func DegreesFromFloat(f float64) Angle {
	return Angle(math.Round(f * float64(Degrees)))
}

func (a Angle) Degrees() float64 {
	return float64(a) / float64(Degrees)
}

func (a Angle) String() string {
	return fmt.Sprintf("%v", a.Degrees())
}

func ParseAngle(s string) (Angle, error) {
	var f float64
	var trailing string
	if n, err := fmt.Sscanf(s, "%f%s", &f, &trailing); n != 1 || err != io.EOF {
		return 0, fmt.Errorf("invalid angle: %q", s)
	}
	n, err := float64ToInt64(math.Round(f * float64(Degrees)))
	if err != nil {
		return 0, fmt.Errorf("invalid angle %f: %w", f, err)
	}
	return Angle(n), nil
}

type Point3D [3]Length

func (p Point3D) String() string {
	var s [3]string
	for i := 0; i < 3; i++ {
		s[i] = strconv.FormatFloat(p[i].Meters(), 'f', -1, 64)
	}
	return fmt.Sprintf("%s,%s,%s", s[0], s[1], s[2])
}

func (p Point3D) IsZero() bool {
	return p[0] == 0 && p[1] == 0 && p[2] == 0
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

type SurveyFlags int

const (
	SurveyAgain SurveyFlags = 1 << iota // do a survey even if we have done one already
)

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
	FixedPosAcc  Length   // accuracy of fixed position
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
