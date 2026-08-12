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

	"github.com/jclark/satpulse/gps/app/ntrip"
	"github.com/jclark/satpulse/gps/app/stream"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/internal/proxy"
	"github.com/jclark/satpulse/time/internal/refclock"
	"github.com/jclark/satpulse/time/internal/ts"
	"github.com/jclark/satpulse/time/lib/ntpshm"
	"github.com/jclark/satpulse/time/lib/pmc"
	"github.com/jclark/satpulse/time/sockrefclock"
	"github.com/jclark/satpulse/time/timeprov"
	"github.com/pelletier/go-toml/v2"
)

const configFileEnvVar = "SATPULSE_CONFIG_FILE"

type Config struct {
	Serial     SerialConfig
	GPS        GPSConfig
	PHC        PHCConfig
	Sync       phcsync.Config
	User       []UserConfig
	Proxy      proxy.Config
	Ntrip      ntrip.Config
	Stream     stream.Config
	HTTP       []HTTPConfig
	LeapSecond LeapSecondConfig
	PTP        PTPConfig
	NTP        NTPConfig
	Log        LogConfig
}

// UserConfig describes one user with name/password credentials.
// Used by any feature that needs basic auth (currently the Ntrip
// caster); referenced from per-feature config by user name.
type UserConfig struct {
	Name     string `toml:"name" comment:"User name"`
	Password string `toml:"password" comment:"Password"`
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
	ClockAccuracy           int64        `toml:"clockAccuracy"`
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
	Sock     *NTPSockConfig     `toml:"sock"`
	SHM      *NTPSHMConfig      `toml:"shm"`
	Timeprov *NTPTimeprovConfig `toml:"timeprov"`
}

type NTPSockConfig struct {
	Path string `toml:"path"`
}

type NTPTimeprovConfig struct {
	Pipe       string  `toml:"pipe"`
	Dispersion float64 `toml:"dispersion"`
}

type NTPSHMConfig struct {
	Segment   *uint8 `toml:"segment"`
	Precision *int8  `toml:"precision"`
}

const serialSHMPrecision int8 = -1

type LogConfig struct {
	Interval int    `toml:"interval"`
	Verbose  bool   `toml:"verbose"`
	Dir      string `toml:"dir"`
	Clock    bool   `toml:"clock"`  // whether to generate a clock log
	Event    bool   `toml:"event"`  // whether to generate an event log
	Packet   bool   `toml:"packet"` // whether to generate a packet log
	Track    bool   `toml:"track"`  // whether to generate a track log
}

var leapSecondDefault = LeapSecondConfig{
	Date:   toml.LocalDate{Year: 2016, Month: int(ptime.LeapSecond2016Month), Day: ptime.LeapSecond2016Day},
	Before: ptime.LeapSecond2016UTCOffBefore,
	After:  ptime.LeapSecond2016UTCOffAfter,
}

// LoadConfig loads the configuration from the first path that exists.
// It returns the config and the path of the file that was used.
func LoadConfig(paths ...string) (*Config, string, error) {
	f, err := openConfig(paths)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	cfg, err := readConfig(f)
	if err != nil {
		return nil, "", err
	}
	return cfg, f.Name(), nil
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
	cfg.PTP.ClockAccuracy = 150
	cfg.PTP.OffsetScaledLogVariance = pmc.OffsetScaledLogVarianceUnknown
	cfg.Sync = phcsync.DefaultConfig()
	return cfg
}

func (cfg *Config) anyHTTP(f func(HTTPConfig) bool) bool {
	for _, c := range cfg.HTTP {
		if f(c) {
			return true
		}
	}
	return false
}

func (cfg *Config) httpWantsSatellites() bool {
	return cfg.anyHTTP(HTTPConfig.gui) || cfg.anyHTTP(HTTPConfig.metrics)
}

// hasNtripStream reports whether any STR record will be
// synthesised at runtime -- a caster mountpoint or an RTCM
// stream.push entry -- and so needs signals-enabled and mode read
// back from the receiver to fill in STR-record fields.  Non-RTCM
// pushes omit the STR header entirely and do not need this state.
func (cfg *Config) hasNtripStream() bool {
	return len(cfg.Ntrip.Mountpoint) > 0 || hasRTCMPush(cfg)
}

func (cfg *Config) hasRTCMMSM7To4() bool {
	for i := range cfg.Ntrip.Mountpoint {
		if cfg.Ntrip.Mountpoint[i].MSM7to4 {
			return true
		}
	}
	return hasRTCMPushMSM7To4(cfg)
}

// Validate validates the configuration and logs warnings for deprecated options.
// It is separate from LoadConfig because the config contains logging settings.
func (cfg *Config) Validate(lg *slog.Logger) error {
	cfg.GPS.validate(lg)
	if err := cfg.Sync.Validate(); err != nil {
		return &configError{err: err}
	}
	users, err := cfg.userSet()
	if err != nil {
		return &configError{err: err}
	}
	if err := cfg.Ntrip.Validate(users); err != nil {
		return &configError{err: err}
	}
	if err := cfg.Stream.Validate(); err != nil {
		return &configError{err: err}
	}
	return nil
}

// userSet checks the top-level [[user]] table for empty or duplicate
// names and returns the set of defined user names, suitable for
// passing into sub-config validators that resolve user references.
func (cfg *Config) userSet() (map[string]struct{}, error) {
	users := make(map[string]struct{}, len(cfg.User))
	for i := range cfg.User {
		u := &cfg.User[i]
		if u.Name == "" {
			return nil, fmt.Errorf("user[%d]: name is required", i)
		}
		if _, dup := users[u.Name]; dup {
			return nil, fmt.Errorf("user: duplicate name %q", u.Name)
		}
		users[u.Name] = struct{}{}
	}
	return users, nil
}

func (cfg PHCConfig) OpenClock(ctx context.Context, lg *slog.Logger) (*ts.Clock, error) {
	if cfg.Interface == "" {
		return nil, nil
	}
	clk, err := ts.OpenClock(ctx, lg, cfg.Interface, cfg.PinDesc(), cfg.Wait)
	if err != nil {
		var cfgErr *ts.ConfigError
		if errors.As(err, &cfgErr) {
			return nil, &configError{err: err}
		}
		return nil, err
	}
	return clk, nil
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

// NewRefClock creates the configured SOCK-style refclock sink: the
// chrony SOCK refclock for [ntp.sock], or the w32time pipe-timeprov
// refclock for [ntp.timeprov].
func (cfg *NTPConfig) NewRefClock(lg *slog.Logger) (refclock.RefClock, error) {
	hasSock := cfg.Sock != nil && cfg.Sock.Path != ""
	if cfg.Timeprov != nil {
		if hasSock {
			return nil, errors.New("[ntp] cannot configure both sock and timeprov")
		}
		rc, err := timeprov.New(cfg.Timeprov.Pipe, cfg.Timeprov.dispersion())
		if err != nil {
			return nil, err
		}
		return refclock.NewLoggingSockRefClock(lg, rc), nil
	}
	if !hasSock {
		return nil, nil
	}
	rc, err := sockrefclock.New(cfg.Sock.Path)
	if err != nil {
		return nil, err
	}
	return refclock.NewLoggingSockRefClock(lg, rc), nil
}

// dispersion returns the per-sample measurement error to report,
// defaulting to 1 ms, a conservative bound for a serial PPS source.
func (cfg *NTPTimeprovConfig) dispersion() time.Duration {
	if cfg.Dispersion <= 0 {
		return time.Millisecond
	}
	return time.Duration(cfg.Dispersion * float64(time.Second))
}

// NewSHMWriter attaches to the configured NTP SHM segment.
func (cfg *NTPConfig) NewSHMWriter(lg *slog.Logger) (*ntpshm.Writer, error) {
	if cfg.SHM == nil || cfg.SHM.Segment == nil {
		return nil, nil
	}
	segment := *cfg.SHM.Segment
	w, a, err := ntpshm.New(segment)
	if err != nil {
		return nil, fmt.Errorf("attach NTP SHM segment %d: %w", segment, err)
	}
	lg.Info("attached to NTP SHM segment", "segment", a.Segment, "key", fmt.Sprintf("0x%x", a.Key))
	return w, nil
}

func (cfg *Config) shmFixedPrecision() *int8 {
	if cfg.NTP.SHM != nil && cfg.NTP.SHM.Precision != nil {
		return cfg.NTP.SHM.Precision
	}
	if cfg.PHC.Interface == "" {
		p := serialSHMPrecision
		return &p
	}
	return nil
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
		return pmc.ClockQuality{}, configErrorf("ptp.clockAccuracy %d ns: out of range", cfg.ClockAccuracy)
	}
	oslv := cfg.OffsetScaledLogVariance
	if cfg.AllanDeviation != 0 {
		if cfg.OffsetScaledLogVariance != pmc.OffsetScaledLogVarianceUnknown {
			return pmc.ClockQuality{}, configErrorf("ptp: cannot specify both offsetScaledLogVariance and allanDeviation")
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

func (cfg *LogConfig) TrackPath(serialPath, ext string) string {
	return cfg.path(cfg.Track, "track", serialPath, ext)
}

func (cfg *LogConfig) path(enable bool, kind, devPath, ext string) string {
	if !enable {
		return ""
	}
	return filepath.Join(cfg.Dir, kind+"."+filepath.Base(devPath)+ext)
}
