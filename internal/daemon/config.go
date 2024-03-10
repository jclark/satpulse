package daemon

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/proxy"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/sockrefclock"
	"github.com/jclark/satpulse/internal/ts"
	"github.com/pelletier/go-toml/v2"

	"github.com/jclark/satpulse/internal/pmc"
)

const configFileEnvVar = "SATPULSE_CONFIG_FILE"

type Config struct {
	Serial     SerialConfig
	GPS        GPSConfig
	PHC        PHCConfig
	Proxy      proxy.Config
	HTTP       []HTTPConfig
	LeapSecond LeapSecondConfig
	PTP        PTPConfig
	NTP        NTPConfig
	Log        LogConfig
}

type SerialConfig struct {
	Device string
	Speed  *int
}

type PHCConfig struct {
	Interface string `toml:"interface"`
	Pin       uint8  `toml:"pin"`
	Channel   uint8  `toml:"channel"`
	Wait      bool   `toml:"wait"`
}

type LeapSecondConfig struct {
	Date   toml.LocalDate `toml:"date"`
	Before uint8          `toml:"before"`
	After  uint8          `toml:"after"`
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

type LogConfig struct {
	Interval int    `toml:"interval"`
	Verbose  bool   `toml:"verbose"`
	Dir      string `toml:"dir"`
	Clock    bool   `toml:"clock"` // whether to generate a clock log
	Event    bool   `toml:"event"` // whether to generate an event log
}

var leapSecondDefault = LeapSecondConfig{
	Date:   toml.LocalDate{Year: 2016, Month: int(time.December), Day: 31},
	Before: 36,
	After:  37,
}

func LoadConfig(paths ...string) (*Config, error) {
	f, err := openConfig(paths)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readConfig(f)
}

func openConfig(paths []string) (*os.File, error) {
	for _, path := range paths {
		f, err := os.Open(path)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, os.ErrNotExist) || len(paths) == 1 {
			return nil, err
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no configuration files specified")
	}
	return nil, fmt.Errorf("cannot open any of the configuration files: %s", strings.Join(paths, ", "))
}

func configErrorDetail(err error) string {
	if s, ok := err.(fmt.Stringer); ok {
		return s.String()
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
	cfg.Log.Interval = 30
	cfg.Log.Dir = "/var/log/satpulse"
	return cfg
}

func (cfg PHCConfig) OpenClock(lg *slog.Logger) (*ts.Clock, phc.DriverFlags, error) {
	return ts.OpenClock(cfg.Interface, cfg.PinDesc(), cfg.Wait, lg)
}

func (cff PHCConfig) PinDesc() ts.PinDesc {
	return ts.PinDesc{
		PinIndex:  uint32(cff.Pin),
		ChanIndex: uint32(cff.Channel),
	}
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

// ClockPath returns the path for the clock log file.
// phcPath is the path to the PHC device (e.g. /dev/ptp0)
func (cfg *LogConfig) ClockPath(phcPath, ext string) string {
	return cfg.path(cfg.Clock, "clock", phcPath, ext)
}

func (cfg *LogConfig) EventPath(serialPath, ext string) string {
	return cfg.path(cfg.Event, "event", serialPath, ext)
}

func (cfg *LogConfig) path(enable bool, kind, devPath, ext string) string {
	if !enable {
		return ""
	}
	return filepath.Join(cfg.Dir, kind+"."+filepath.Base(devPath)+ext)
}
