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
	Ack(bool)
	ID() string
}

type Config struct {
	m map[string]interface{}
}

type CfgKey[T any] struct {
	s string
}

func (k CfgKey[T]) Get(cfg *Config) (T, bool) {
	if cfg.m != nil {
		value, exists := cfg.m[k.s]
		if exists {
			return value.(T), true
		}
	}
	var zero T
	return zero, false
}

func (k CfgKey[T]) String() string {
	return k.s
}

func (k CfgKey[T]) Set(cfg *Config, v T) {
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

var CfgEnabledGNSS = CfgKey[[]MajorGNSS]{"enabledGNSS"}
var CfgSolutionPeriod = CfgKey[time.Duration]{"solutionPeriod"}
var CfgTimePulseWidth = CfgKey[time.Duration]{"timePulseWidth"}
var CfgTimePulsePeriod = CfgKey[time.Duration]{"timePulsePeriod"}
var CfgTimePulseGNSS = CfgKey[MajorGNSS]{"timePulseGNSS"}
var CfgTimePulseOnlyWhenLocked = CfgKey[bool]{"timePulseOnlyWhenLocked"}
var CfgTimePulsePolarityRising = CfgKey[bool]{"timePulsePolarityRising"}
var CfgTimeMode = CfgKey[TimeMode]{"timeMode"}
var CfgAntennaCableDelay = CfgKey[time.Duration]{"antennaCableDelay"}
var CfgSurveyMinDur = CfgKey[time.Duration]{"surveyMinDur"}
var CfgSurveyAccLimit = CfgKey[Length]{"surveyAccLimit"}
var CfgFixedPosECEF = CfgKey[Point3D]{"fixedPosECEF"}
var CfgFixedPosAcc = CfgKey[Length]{"fixedPosAcc"}
var CfgUtcStandard = CfgKey[MajorGNSS]{"utcStandard"} // nil value to use auto
var CfgStationary = CfgKey[bool]{"stationary"}

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
