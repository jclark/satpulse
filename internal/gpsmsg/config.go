package gpsmsg

import (
	"encoding/json"
	"fmt"
	"time"
)

type Configurator interface {
	Config() *Config
	NextRequest() ConfigRequest
}

type ConfigRequest interface {
	Packet() []byte
	Ackable() bool
	Done()
	ID() string
}

type Config struct {
	m map[string]interface{}
}

type CfgKey interface {
	fmt.Stringer
	cfgKey()
}

func (c *Config) Contains(k CfgKey) bool {
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

func (k TypedCfgKey[T]) Get(cfg *Config) (T, bool) {
	if cfg.m != nil {
		value, exists := cfg.m[k.s]
		if exists {
			return value.(T), true
		}
	}
	var zero T
	return zero, false
}

func (k TypedCfgKey[T]) Set(cfg *Config, v T) {
	if cfg.m == nil {
		cfg.m = make(map[string]interface{})
	}
	cfg.m[k.s] = v
}

func (c *Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.serializableMap())
}

func (c *Config) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}

func (c *Config) String() string {
	return fmt.Sprint(c.serializableMap())
}

// Inconsistent returns a Config with the entries in c2 that are inconsistent with c.
// An entry is inconsistent if it exists in both c and c2 but has different values.
func (c *Config) Inconsistent(c2 *Config) *Config {
	m := make(map[string]interface{})
	for k, v := range c.m {
		if v2, exists := c2.m[k]; exists && v != v2 {
			m[k] = v2
		}
	}
	return &Config{m}
}

// Missing returns a Config with the entries from c2 that are missing from c.
func (c *Config) Missing(c2 *Config) *Config {
	m := make(map[string]interface{})
	for k, v := range c2.m {
		if _, exists := c.m[k]; !exists {
			m[k] = v
		}
	}
	return &Config{m}
}

func (c *Config) IsEmpty() bool {
	return len(c.m) == 0
}

func (c *Config) serializableMap() map[string]interface{} {
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

func (cfg *Config) SetSane() {
	CfgSolutionPeriod.Set(cfg, 1*time.Second)
	CfgTimePulsePeriod.Set(cfg, 1*time.Second)
	CfgTimePulseWidth.Set(cfg, time.Second/10)
	CfgTimePulsePolarityRising.Set(cfg, true)
	CfgTimePulseAlignToGNSS.Set(cfg, true)
	CfgTimePulseOnlyWhenLocked.Set(cfg, true)
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
