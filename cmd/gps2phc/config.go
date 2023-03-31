package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jclark/gps2phc/internal/ptime"
	"github.com/pelletier/go-toml/v2"

	"github.com/jclark/gps2phc/internal/pmc"
)

const defaultConfigFile = "gps2phc.toml"

type Config struct {
	Serial     SerialConfig
	Pulse      TimePulseConfig
	TCP        TCPConfig
	LeapSecond LeapSecondConfig
	PTP        PTPConfig
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

type PTPConfig struct {
	UDSAddress   string  `toml:"udsAddress"`
	DomainNumber uint8   `toml:"domainNumber"`
	MajorSdoID   uint8   `toml:"majorSdoId"`
	MinorSdoID   uint8   `toml:"minorSdoId"`
	Impl         PTPImpl `toml:"impl"`
}

type PTPImpl uint8

const (
	PTPImplNone PTPImpl = iota
	PTPImplPTP4L
	PTPImplPTP4U
)

var leapSecondDefault = LeapSecondConfig{
	Date:   toml.LocalDate{Year: 2016, Month: int(time.December), Day: 31},
	Before: 36,
	After:  37,
}

var ptpConfigDefault = PTPConfig{
	UDSAddress: pmc.PTP4LSocketPath,
	Impl:       PTPImplPTP4L,
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
	cfg.PTP = ptpConfigDefault
	return cfg
}

func (cfg LeapSecondConfig) leapSecond() ptime.LeapSecond {
	return ptime.LeapSecondOnDate(cfg.Date.AsTime((time.UTC)), int16(cfg.Before), int16(cfg.After))
}

func (cfg *PTPConfig) NewClient() (*pmc.Client, error) {
	cc := pmc.NewClientConfig()
	cc.RemoteSocketPath = cfg.UDSAddress
	cl, err := pmc.NewClient(cc)
	if err != nil {
		return nil, err
	}
	cl.DomainNumber = cfg.DomainNumber
	cl.MajorSdoID = cfg.MajorSdoID
	cl.MinorSdoID = cfg.MinorSdoID
	return cl, nil
}

func (pi PTPImpl) String() string {
	switch pi {
	case PTPImplPTP4L:
		return "ptp4l"
	case PTPImplPTP4U:
		return "ptp4u"
	case PTPImplNone:
		return "none"
	default:
		return fmt.Sprintf("PTPImpl(%d)", pi)
	}
}

func (pi *PTPImpl) UnmarshalText(text []byte) error {
	switch strings.ToLower(string(text)) {
	case "ptp4l":
		*pi = PTPImplPTP4L
	// Not implemented yet
	//case "ptp4u":
	//	*pi = PTPImplPTP4U
	case "none":
		*pi = PTPImplNone
	default:
		return fmt.Errorf("invalid PTP implementation: %q", string(text))
	}
	return nil
}
