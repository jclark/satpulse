package gpsprot

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"
)

// Protocol manages the processing and generation of packets for a GPS receiver.
// An implementation of protocol captures the semantics of a particular GPS receiver protocol,
// such as UBX or NMEA.
// Methods on Protocol do not themselves send or receive
// Splitting sequences of bytes in packets is handled by the scan package.
// Packets to be processed are passed to ProcessPacket.
// There are three stages: probing, configuration, and normal operation.
// In normal operation, calls to ProcessPacket result in calls to the MsgHandler,
// which is set by SetHandler.
// In probing, ProbePacket returns a packet to be sent to the GPS receiver;
// ProbeOK as soon as a packet has been processed that indicates the GPS receiver is responding.
// Configuration is managed by a Configurator created by Configure;
// repeated calls to NextRequest return the next request to be sent to the GPS receiver.
// After a Configurator has been created, calls to ProcessPacket will pass configuration-related packets
// to the Configurator.
// The packets processed by ProcessPacket determine the requests returned by NextRequest.
// If the Ackable method of a ConfigRequest returns true, then FindAck should be used
// to find whether the acknowledgement has been received.
type Protocol interface {
	ProbePacket() []byte
	ProbeOK() bool
	SetHandler(h MsgHandler)
	ProcessPacket(data string, tRead time.Time) error
	Configure(target *ConfigMap, opts ConfigOptions) (Configurator, error)
	FindAck(packet []byte, tSent time.Time) *Ack
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
type Configurator interface {
	// ConfigMap returns the current configuration of the GPS receiver.
	// It should be called after NextRequest returns nil.
	ConfigMap() *ConfigMap
	// NextRequest returns the next request that should be sent to the GPS receiver.
	// If there are no more requests, it returns nil, nil.
	NextRequest() (ConfigRequest, error)
}

// ConfigRequest represents a request to be sent to the GPS receiver to configure it.
// The request may expect an acknowledgement.
// The request may be getting aspects of the current configuration,
// or setting aspects of the configuration.
// A ConfigRequest is associated with the Configurator that created it.
type ConfigRequest interface {
	// Packet returns the packet to be sent to the GPS receiver.
	Packet() []byte
	// Ackable returns true if the packet is one that needs an acknowledgement.
	// An acknowledgement can either be an ACK or a NACK.
	Ackable() bool
	// Done should be called when the request has been sent and the acknowledgement received,
	// and the acknowledge was an ACK.
	// This causes the Configurator to update its state to reflect that the configuration has been applied.
	Done()
	// ID returns a human-readable identifier for the request type.
	ID() string
}

type ConfigOptions struct {
	Flash               bool
	Reset               bool
	Detect              bool
	EnableLeapSecondMsg bool
	EnableTimeMsg       bool
	Survey              Survey
}

// Survey specifies whether a survey should be performed, and if so, its parameters
// The survey is performed when the time mode is one of the modes in When.
// If When is non-zero, then AccLimit must also be non-zero.
type Survey struct {
	When     TimeModeSet   // perform a survey when the time mode is one of these
	MinDur   time.Duration // survey should run at least this long
	AccLimit Length        // survey should run until this accuracy is achieved
}

type ConfigMap struct {
	m map[string]interface{}
}

type CfgKey interface {
	fmt.Stringer
	cfgKey()
}

type CfgKeySet map[CfgKey]struct{}

func (s CfgKeySet) Contains(k CfgKey) bool {
	_, exists := s[k]
	return exists
}

func (s CfgKeySet) Add(k CfgKey) {
	s[k] = struct{}{}
}

func (c *ConfigMap) Contains(k CfgKey) bool {
	_, exists := c.m[k.String()]
	return exists
}

type anyCfgKey struct {
	s string
}

func (k anyCfgKey) cfgKey() {}

func (k anyCfgKey) String() string {
	return k.s
}

type TypedCfgKey[T comparable] struct {
	anyCfgKey
}

func (k TypedCfgKey[T]) Get(cm *ConfigMap) (T, bool) {
	if cm.m != nil {
		value, exists := cm.m[k.s]
		if exists {
			return value.(T), true
		}
	}
	var zero T
	return zero, false
}

func (k TypedCfgKey[T]) Set(cm *ConfigMap, v T) {
	if cm.m == nil {
		cm.m = make(map[string]interface{})
	}
	cm.m[k.s] = v
}

func (c *ConfigMap) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.serializableMap())
}

func (c *ConfigMap) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}

func (c *ConfigMap) String() string {
	return fmt.Sprint(c.serializableMap())
}

// Inconsistent returns a Config with the entries in c2 that are inconsistent with c.
// An entry is inconsistent if it exists in both c and c2 but has different values.
func (c *ConfigMap) Inconsistent(c2 *ConfigMap) *ConfigMap {
	m := make(map[string]interface{})
	for k, v := range c.m {
		if v2, exists := c2.m[k]; exists && v != v2 {
			m[k] = v2
		}
	}
	return &ConfigMap{m}
}

// Missing returns a Config with the entries from c2 that are missing from c.
func (c *ConfigMap) Missing(c2 *ConfigMap) *ConfigMap {
	m := make(map[string]interface{})
	for k, v := range c2.m {
		if _, exists := c.m[k]; !exists {
			m[k] = v
		}
	}
	return &ConfigMap{m}
}

func (c *ConfigMap) IsEmpty() bool {
	return len(c.m) == 0
}

func (c *ConfigMap) serializableMap() map[string]interface{} {
	j := make(map[string]interface{})
	for k, v := range c.m {
		switch t := v.(type) {
		case time.Duration:
			j[k] = float64(t) / float64(time.Second)
		case Length:
			j[k] = float64(t) / float64(Meter)
		case Point3D:
			j[k] = []float64{t[0].Meters(), t[1].Meters(), t[2].Meters()}
		case TimeMode:
			switch t {
			case TimeModeDisabled:
				j[k] = "disabled"
			case TimeModeSurvey:
				j[k] = "survey"
			case TimeModeFixed:
				j[k] = "fixed"
			default:
				j[k] = t
			}
		case GNSSSet:
			j[k] = t.Items()
		default:
			j[k] = v
		}
	}
	return j
}

func makeCfgKey[T comparable](s string) TypedCfgKey[T] {
	return TypedCfgKey[T]{anyCfgKey{s}}
}

var CfgGNSSEnabled = makeCfgKey[GNSSSet]("gnssEnabled")
var CfgPrimaryGNSS = makeCfgKey[GNSS]("primaryGNSS")
var CfgSolutionPeriod = makeCfgKey[time.Duration]("solutionPeriod")
var CfgTimePulseWidth = makeCfgKey[time.Duration]("timePulseWidth")
var CfgTimePulsePeriod = makeCfgKey[time.Duration]("timePulsePeriod")
var CfgTimePulseAlignToGNSS = makeCfgKey[bool]("timePulseAlignToGNSS")
var CfgTimePulseOnlyWhenLocked = makeCfgKey[bool]("timePulseOnlyWhenLocked")
var CfgTimePulsePolarityRising = makeCfgKey[bool]("timePulsePolarityRising")
var CfgTimeMode = makeCfgKey[TimeMode]("timeMode")
var CfgAntennaCableDelay = makeCfgKey[time.Duration]("antennaCableDelay")
var CfgFixedPosECEF = makeCfgKey[Point3D]("fixedPosECEF")
var CfgFixedPosAcc = makeCfgKey[Length]("fixedPosAcc")
var CfgStationary = makeCfgKey[bool]("stationary")
var CfgNMEAEnabled = makeCfgKey[bool]("nmeaEnabled")
var CfgBaudRate = makeCfgKey[uint32]("baudRate")

func (cm *ConfigMap) SetPPS() {
	CfgSolutionPeriod.Set(cm, 1*time.Second)
	CfgTimePulsePeriod.Set(cm, 1*time.Second)
	CfgTimePulseWidth.Set(cm, time.Second/10)
	CfgTimePulsePolarityRising.Set(cm, true)
	CfgTimePulseAlignToGNSS.Set(cm, true)
	CfgTimePulseOnlyWhenLocked.Set(cm, true)
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
	return fmt.Sprintf("%v,%v,%v", p[0].Meters(), p[1].Meters(), p[2].Meters())
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
