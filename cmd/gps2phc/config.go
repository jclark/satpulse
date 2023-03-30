package main

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/jclark/gps2phc/internal/ptime"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Serial     SerialConfig
	Pulse      TimePulseConfig
	TCP        TCPConfig
	LeapSecond LeapSecondConfig
}

type SerialConfig struct {
	Device string
	Speed  *int
}

type TCPConfig struct {
	Port uint16
}

type LeapSecondConfig struct {
	Date          toml.LocalDate
	Before, After uint8
}

var leapSecondDefault = LeapSecondConfig{
	Date:   toml.LocalDate{Year: 2016, Month: int(time.December), Day: 31},
	Before: 36,
	After:  37,
}

func loadConfig(configFile string) (*Config, error) {
	f, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readConfig(f)
}

func configErrorDetail(err error) string {
	var derr *toml.DecodeError
	if errors.As(err, &derr) {
		return derr.String()
	}
	return ""
}

func readConfig(r io.Reader) (*Config, error) {
	cfg := defaultConfig()
	err := toml.NewDecoder(r).DisallowUnknownFields().Decode(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultConfig() *Config {
	cfg := new(Config)
	cfg.LeapSecond = leapSecondDefault
	return cfg
}

func leapSecondFromConfig(cfg LeapSecondConfig) ptime.LeapSecond {
	return ptime.LeapSecondOnDate(cfg.Date.AsTime((time.UTC)), int16(cfg.Before), int16(cfg.After))
}
