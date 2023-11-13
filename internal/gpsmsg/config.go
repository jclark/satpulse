package gpsmsg

import (
	"encoding/json"
	"fmt"
	"time"
)

type Configurator interface {
	ConfigMap() *ConfigMap
	NextRequest() (ConfigRequest, error)
}

type ConfigRequest interface {
	Packet() []byte
	Ackable() bool
	Done()
	ID() string
}

type ConfigMap struct {
	m map[string]interface{}
}

type CfgKey interface {
	fmt.Stringer
	cfgKey()
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
			j[k] = map[string]float64{
				"x": float64(t.X) / float64(Meter),
				"y": float64(t.Y) / float64(Meter),
				"z": float64(t.Z) / float64(Meter),
			}
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
		case MajorGNSSSet:
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

var CfgGNSSEnabled = makeCfgKey[MajorGNSSSet]("gnssEnabled")
var CfgPrimaryGNSS = makeCfgKey[MajorGNSS]("primaryGNSS")
var CfgSolutionPeriod = makeCfgKey[time.Duration]("solutionPeriod")
var CfgTimePulseWidth = makeCfgKey[time.Duration]("timePulseWidth")
var CfgTimePulsePeriod = makeCfgKey[time.Duration]("timePulsePeriod")
var CfgTimePulseAlignToGNSS = makeCfgKey[bool]("timePulseAlignToGNSS")
var CfgTimePulseOnlyWhenLocked = makeCfgKey[bool]("timePulseOnlyWhenLocked")
var CfgTimePulsePolarityRising = makeCfgKey[bool]("timePulsePolarityRising")
var CfgTimeMode = makeCfgKey[TimeMode]("timeMode")
var CfgAntennaCableDelay = makeCfgKey[time.Duration]("antennaCableDelay")
var CfgSurveyMinDur = makeCfgKey[time.Duration]("surveyMinDur")
var CfgSurveyAccLimit = makeCfgKey[Length]("surveyAccLimit")
var CfgFixedPosECEF = makeCfgKey[Point3D]("fixedPosECEF")
var CfgFixedPosAcc = makeCfgKey[Length]("fixedPosAcc")
var CfgStationary = makeCfgKey[bool]("stationary")
var CfgNMEAEnabled = makeCfgKey[bool]("nmeaEnabled")
var CfgBaudRate = makeCfgKey[uint32]("baudRate")

func (cm *ConfigMap) SetSane() {
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

type Point3D struct {
	X, Y, Z Length
}

type TimeMode byte

const (
	TimeModeDisabled TimeMode = iota
	TimeModeSurvey
	TimeModeFixed
)
