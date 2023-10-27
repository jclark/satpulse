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

type TypedCfgKey[T any] struct {
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
	return []byte(fmt.Sprint(c.serializableMap())), nil
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
		default:
			j[k] = v
		}
	}
	return j
}

func makeCfgKey[T any](s string) TypedCfgKey[T] {
	return TypedCfgKey[T]{anyCfgKey{s}}
}

var CfgEnabledGNSS = makeCfgKey[[]MajorGNSS]("enabledGNSS")
var CfgSolutionPeriod = makeCfgKey[time.Duration]("solutionPeriod")
var CfgTimePulseWidth = makeCfgKey[time.Duration]("timePulseWidth")
var CfgTimePulsePeriod = makeCfgKey[time.Duration]("timePulsePeriod")
var CfgTimePulseGNSS = makeCfgKey[MajorGNSS]("timePulseGNSS")
var CfgTimePulseOnlyWhenLocked = makeCfgKey[bool]("timePulseOnlyWhenLocked")
var CfgTimePulsePolarityRising = makeCfgKey[bool]("timePulsePolarityRising")
var CfgTimeMode = makeCfgKey[TimeMode]("timeMode")
var CfgAntennaCableDelay = makeCfgKey[time.Duration]("antennaCableDelay")
var CfgSurveyMinDur = makeCfgKey[time.Duration]("surveyMinDur")
var CfgSurveyAccLimit = makeCfgKey[Length]("surveyAccLimit")
var CfgFixedPosECEF = makeCfgKey[Point3D]("fixedPosECEF")
var CfgFixedPosAcc = makeCfgKey[Length]("fixedPosAcc")
var CfgUtcStandard = makeCfgKey[MajorGNSS]("utcStandard") // nil value to use auto
var CfgStationary = makeCfgKey[bool]("stationary")

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
