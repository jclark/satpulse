package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/pmc"
	"github.com/jclark/satpulse/internal/proxy"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/refclock"
	"github.com/jclark/satpulse/internal/sockrefclock"
	"github.com/jclark/satpulse/internal/ts"
	"github.com/pelletier/go-toml/v2"
)

const configFileEnvVar = "SATPULSE_CONFIG_FILE"

type Config struct {
	Serial     SerialConfig
	GPS        GPSConfig
	PHC        PHCConfig
	Sync       phcsync.Config
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
	ClockAccuracy           int          `toml:"clockAccuracy"`
	OffsetScaledLogVariance uint16       `toml:"offsetScaledLogVariance"`
	AllanDeviation          float64      `toml:"allanDeviation"`
	DomainNumber            uint8        `toml:"domainNumber"`
	MajorSdoID              uint8        `toml:"majorSdoId"`
	MinorSdoID              uint8        `toml:"minorSdoId"`
	PTP4L                   *PTP4LConfig `toml:"ptp4l"`
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
	Clock    bool   `toml:"clock"`  // whether to generate a clock log
	Event    bool   `toml:"event"`  // whether to generate an event log
	Packet   bool   `toml:"packet"` // whether to generate a packet log
}

var leapSecondDefault = LeapSecondConfig{
	Date:   toml.LocalDate{Year: 2016, Month: int(ptime.LeapSecond2016Month), Day: ptime.LeapSecond2016Day},
	Before: ptime.LeapSecond2016UTCOffBefore,
	After:  ptime.LeapSecond2016UTCOffAfter,
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
	err = cfg.Sync.Validate()
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
	cfg.PTP.ClockAccuracy = 150
	cfg.PTP.OffsetScaledLogVariance = pmc.OffsetScaledLogVarianceUnknown
	cfg.Sync = phcsync.DefaultConfig()
	return cfg
}

func (cfg *Config) httpWantsSatellites() bool {
	for _, c := range cfg.HTTP {
		if c.gui() || c.metrics() {
			return true
		}
	}
	return false
}

func (cfg PHCConfig) OpenClock(ctx context.Context, lg *slog.Logger) (*ts.Clock, error) {
	if cfg.Interface == "" {
		return nil, nil
	}
	return ts.OpenClock(ctx, lg, cfg.Interface, cfg.PinDesc(), cfg.Wait)
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

func (cfg *NTPConfig) NewRefClock(lg *slog.Logger) (refclock.RefClock, error) {
	if cfg.Sock == nil || cfg.Sock.Path == "" {
		return nil, nil
	}
	rc, err := sockrefclock.New(cfg.Sock.Path)
	if err != nil {
		return nil, err
	}
	return refclock.NewLoggingSockRefClock(lg, rc), nil
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

// ClockQuality returns the PTP ClockQuality for when the clock is in sync.
func (cfg *PTPConfig) ClockQuality() (pmc.ClockQuality, error) {
	acc := pmc.DurationToClockAccuracy(time.Duration(cfg.ClockAccuracy) * time.Nanosecond)
	if acc == 0 {
		return pmc.ClockQuality{}, fmt.Errorf("ptp.clockAccuracy %d ns: out of range", cfg.ClockAccuracy)
	}
	oslv := cfg.OffsetScaledLogVariance
	if cfg.AllanDeviation != 0 {
		if cfg.OffsetScaledLogVariance != pmc.OffsetScaledLogVarianceUnknown {
			return pmc.ClockQuality{}, errors.New("ptp: cannot specify both offsetScaledLogVariance and allanDeviation")
		}
		oslv = pmc.AdevToOffsetScaledLogVariance(cfg.AllanDeviation, 1.0)
	}
	return pmc.ClockQuality{
		ClockClass:              pmc.ClockClassSyncPrimaryRef,
		ClockAccuracy:           acc,
		OffsetScaledLogVariance: oslv,
	}, nil
}

// ClockPath returns the path for the clock log file.
// phcPath is the path to the PHC device (e.g. /dev/ptp0)
func (cfg *LogConfig) ClockPath(phcPath, ext string) string {
	return cfg.path(cfg.Clock, "clock", phcPath, ext)
}

func (cfg *LogConfig) EventPath(serialPath, ext string) string {
	return cfg.path(cfg.Event, "event", serialPath, ext)
}

func (cfg *LogConfig) PacketPath(serialPath, ext string) string {
	return cfg.path(cfg.Packet, "packet", serialPath, ext)
}

func (cfg *LogConfig) path(enable bool, kind, devPath, ext string) string {
	if !enable {
		return ""
	}
	return filepath.Join(cfg.Dir, kind+"."+filepath.Base(devPath)+ext)
}
