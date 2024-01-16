package daemon

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/cmd/daemon/proxy"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/sockrefclock"
	"github.com/pelletier/go-toml/v2"

	"github.com/jclark/satpulse/internal/pmc"
)

const programName = "satpulse"

const defaultConfigFileConst = "/usr/local/etc/" + programName + ".toml"

// This is a var so that it can be overridden at build time or test time.
var defaultConfigFile = defaultConfigFileConst

type Config struct {
	Serial     SerialConfig
	GPS        GPSConfig
	PHC        PHCConfig
	Proxy      proxy.Config
	HTTP       []HTTPConfig
	LeapSecond LeapSecondConfig
	PTP        PTPConfig
	NTP        NTPConfig
}

type SerialConfig struct {
	Device string
	Speed  *int
}

type LeapSecondConfig struct {
	Date          toml.LocalDate
	Before, After uint8
}

type PTPConfig struct {
	DomainNumber uint8        `toml:"domainNumber"`
	MajorSdoID   uint8        `toml:"majorSdoId"`
	MinorSdoID   uint8        `toml:"minorSdoId"`
	PTP4L        *PTP4LConfig `toml:"ptp4l"`
}

type PTP4LConfig struct {
	UDSAddress string `toml:"udsAddress"`
}

type NTPConfig struct {
	Sock *NTPSockConfig `toml:"sock"`
}

type NTPSockConfig struct {
	Path string `toml:"path"`
}

var leapSecondDefault = LeapSecondConfig{
	Date:   toml.LocalDate{Year: 2016, Month: int(time.December), Day: 31},
	Before: 36,
	After:  37,
}

func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
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
	cfg.GPS = gpsDefault
	cfg.LeapSecond = leapSecondDefault
	return cfg
}

func (cfg LeapSecondConfig) leapSecond() ptime.LeapSecond {
	return ptime.LeapSecondOnDate(cfg.Date.AsTime((time.UTC)), int16(cfg.Before), int16(cfg.After))
}

func (cfg *NTPConfig) NewRefClock(lg *slog.Logger) (mon.RefClock, error) {
	if cfg.Sock == nil || cfg.Sock.Path == "" {
		return nil, nil
	}
	rc, err := sockrefclock.New("", cfg.Sock.Path)
	if err != nil {
		return nil, err
	}
	return mon.NewLoggingSockRefClock(lg, rc), nil
}

func (cfg *PTPConfig) NewClient() (*pmc.Client, error) {
	ptp4l := cfg.PTP4L
	if ptp4l == nil {
		return nil, nil
	}
	cc := pmc.NewClientConfig()
	cc.RemoteSocketPath = ptp4l.UDSAddress
	cl, err := pmc.NewClient(cc)
	if err != nil {
		return nil, err
	}
	cl.DomainNumber = cfg.DomainNumber
	cl.MajorSdoID = cfg.MajorSdoID
	cl.MinorSdoID = cfg.MinorSdoID
	return cl, nil
}
