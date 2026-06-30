package stream

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/ntrip"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
)

// Config matches the [stream] TOML table.
type Config struct {
	Pull PullConfig   `toml:"pull"`
	Push []PushConfig `toml:"push"`
}

// PullConfig matches the [stream.pull] TOML table.  At most one of
// TCP or Ntrip is set; with neither set the pull is disabled, so an
// empty table is valid.
type PullConfig struct {
	TCP   *TCPConfig   `toml:"tcp"`
	Ntrip *NtripConfig `toml:"ntrip"`
}

// TCPConfig matches [stream.pull.tcp].
type TCPConfig struct {
	Address string `toml:"address"`
}

// NtripConfig matches [stream.pull.ntrip].  NMEASendInterval is the
// GGA upload interval in seconds; an unset value defaults to
// DefaultNMEASendInterval and a configured 0 means upload once per
// connection.
type NtripConfig struct {
	Address          string   `toml:"address"`
	Mountpoint       string   `toml:"mountpoint"`
	Username         string   `toml:"username"`
	Password         string   `toml:"password"`
	NMEASend         bool     `toml:"nmeaSend"`
	NMEASendInterval *float64 `toml:"nmeaSendInterval"`
}

const (
	// DefaultNMEASendInterval is the GGA upload interval used when nmeaSend
	// is enabled but nmeaSendInterval is unset.  The satpulsetool ntrip
	// diagnostic uses the same default.
	DefaultNMEASendInterval = 5 * time.Second

	// MinNMEASendInterval is the smallest periodic GGA upload interval.  A
	// configured 0 is still allowed and means upload once per connection.
	MinNMEASendInterval = time.Second

	// MaxNMEASendInterval is the largest periodic GGA upload interval.  It
	// is a sanity bound that also keeps the seconds-to-Duration conversion
	// well clear of int64 overflow.
	MaxNMEASendInterval = 366 * 24 * time.Hour
)

// ValidateNMEASendInterval checks a GGA upload interval, in seconds: it must be
// finite, and either 0 (upload once per connection) or between
// MinNMEASendInterval and MaxNMEASendInterval.  The returned error has no
// prefix so callers can wrap it with the relevant config key or flag name.
func ValidateNMEASendInterval(secs float64) error {
	if math.IsNaN(secs) || math.IsInf(secs, 0) {
		return errors.New("must be a finite number")
	}
	if secs < 0 {
		return errors.New("must be non-negative")
	}
	if secs > 0 && secs < MinNMEASendInterval.Seconds() {
		return fmt.Errorf("must be 0 or at least %d", MinNMEASendInterval/time.Second)
	}
	if secs > MaxNMEASendInterval.Seconds() {
		return fmt.Errorf("must be at most %d", MaxNMEASendInterval/time.Second)
	}
	return nil
}

// PushConfig is one [[stream.push]] entry.  Transport is selected
// by which sub-table is present.  Protocol is the packet format
// forwarded to the destination; an unset value defaults per
// transport.
type PushConfig struct {
	Protocol gpsreg.Protocol  `toml:"protocol"`
	Ntrip    *NtripPushConfig `toml:"ntrip"`
	UDP      *UDPPushConfig   `toml:"udp"`
}

// NtripPushConfig is the operator-facing TOML for one Ntrip push
// destination.  It is a pure TOML decoder target.
type NtripPushConfig struct {
	Address            string `toml:"address"`
	Mountpoint         string `toml:"mountpoint"`
	Password           string `toml:"password"`
	ntrip.StreamConfig        // embedded: Description, Bitrate
	MSM7to4            bool   `toml:"msm7to4"`
}

// UDPPushConfig is the operator-facing TOML for one UDP push
// destination.
type UDPPushConfig struct {
	Address string `toml:"address"`
}

// Validate checks the [stream] configuration.  Absent [stream.pull]
// and absent [[stream.push]] are both valid (stream is optional).
func (cfg *Config) Validate() error {
	if err := cfg.Pull.Validate(); err != nil {
		return err
	}
	for i := range cfg.Push {
		if err := cfg.Push[i].Validate(); err != nil {
			return fmt.Errorf("stream.push[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate checks one [[stream.push]] entry and fills in the
// transport-specific default for an unset protocol.
func (cfg *PushConfig) Validate() error {
	hasNtrip := cfg.Ntrip != nil
	hasUDP := cfg.UDP != nil
	switch {
	case hasNtrip && hasUDP:
		return fmt.Errorf("ntrip and udp are mutually exclusive")
	case !hasNtrip && !hasUDP:
		return fmt.Errorf("must configure either ntrip or udp")
	case hasUDP:
		if cfg.UDP.Address == "" {
			return fmt.Errorf("udp.address is required")
		}
		return nil
	}
	if cfg.Protocol == "" {
		cfg.Protocol = gpsreg.Protocol(gpsreg.TagRTCM)
	}
	if cfg.Ntrip.Address == "" {
		return fmt.Errorf("ntrip.address is required")
	}
	if cfg.Ntrip.Mountpoint == "" {
		return fmt.Errorf("ntrip.mountpoint is required")
	}
	if err := ntrip.CheckMountpointName(cfg.Ntrip.Mountpoint); err != nil {
		return fmt.Errorf("ntrip.mountpoint: %w", err)
	}
	if err := checkSourcePassword(cfg.Ntrip.Password); err != nil {
		return fmt.Errorf("ntrip.password: %w", err)
	}
	if err := ntrip.CheckSTRField(cfg.Ntrip.Description); err != nil {
		return fmt.Errorf("ntrip.description: %w", err)
	}
	if cfg.Ntrip.Bitrate < 0 {
		return fmt.Errorf("ntrip.bitrate %d: must be non-negative", cfg.Ntrip.Bitrate)
	}
	if cfg.Ntrip.MSM7to4 && cfg.Protocol.Tag() != gpsreg.TagRTCM {
		return fmt.Errorf("ntrip.msm7to4 requires protocol = \"RTCM\"")
	}
	return nil
}

// checkSourcePassword enforces the Ntrip v1 SOURCE password rule:
// a non-empty "plain ASCII string" (v1 spec, sec 2.2.1), restricted
// further to printable ASCII so that no byte can split the
// whitespace-delimited SOURCE request line or terminate it via
// CR/LF.
func checkSourcePassword(s string) error {
	if s == "" {
		return fmt.Errorf("must not be empty")
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x21 || c > 0x7E {
			return fmt.Errorf("disallowed byte 0x%02x at offset %d", c, i)
		}
	}
	return nil
}

// HasNtripPush reports whether any [[stream.push]] entry uses ntrip.*.
// Used by the daemon to decide whether to wire push and (via
// Config.hasNtripStream) whether STR-record GPS state is needed.
func (cfg *Config) HasNtripPush() bool {
	for i := range cfg.Push {
		if cfg.Push[i].Ntrip != nil {
			return true
		}
	}
	return false
}

// NMEASend reports whether [stream.pull.ntrip] should upload NMEA GGA.
func (cfg *PullConfig) NMEASend() bool {
	return cfg.Ntrip != nil && cfg.Ntrip.NMEASend
}

// nmeaSendInterval returns the resolved GGA upload interval.  An unset
// nmeaSendInterval defaults to DefaultNMEASendInterval; a configured 0
// means upload once per connection.  Consulted only on the NMEASend
// path, so it has no effect when nmeaSend is false.
func (cfg *PullConfig) nmeaSendInterval() time.Duration {
	if cfg.Ntrip != nil && cfg.Ntrip.NMEASendInterval != nil {
		return time.Duration(*cfg.Ntrip.NMEASendInterval * float64(time.Second))
	}
	return DefaultNMEASendInterval
}

// Source builds the correction source for cfg. It returns ok=false when the
// pull is disabled. version feeds the Ntrip User-Agent header.
func (cfg *PullConfig) Source(version string) (src Source, addr string, ok bool) {
	switch {
	case cfg.TCP != nil:
		addr = cfg.TCP.Address
		src = &TCPSource{Addr: addr}
	case cfg.Ntrip != nil:
		addr = NtripNormalizeAddress(cfg.Ntrip.Address)
		src = &NtripSource{
			Addr:       addr,
			Mountpoint: cfg.Ntrip.Mountpoint,
			Username:   cfg.Ntrip.Username,
			Password:   cfg.Ntrip.Password,
			UserAgent:  NtripUserAgent{Version: version},
		}
	default:
		return nil, "", false
	}
	return src, addr, true
}

// NewPull builds a Pull for cfg, or returns nil when the pull is disabled.
// pktFormats are the correction formats to scan. pw and portLock are the
// correction-output port.
func (cfg *PullConfig) NewPull(version string, lg *slog.Logger,
	pktFormats []gpsprot.PacketFormat, pw PacketWriter, portLock gpsio.OutPortLock) (*Pull, string) {
	src, addr, ok := cfg.Source(version)
	if !ok {
		return nil, ""
	}
	nmeaSendInterval := time.Duration(0)
	if cfg.NMEASend() {
		nmeaSendInterval = cfg.nmeaSendInterval()
	}
	if lg != nil && addr != "" {
		lg = lg.With("addr", addr)
	}
	return NewPull(src, lg, pw, portLock, pktFormats, nmeaSendInterval), addr
}

// Validate checks [stream.pull] for transport selection and
// required fields.  An empty table (neither tcp nor ntrip) leaves
// the pull disabled and is valid.
func (cfg *PullConfig) Validate() error {
	hasTCP := cfg.TCP != nil
	hasNtrip := cfg.Ntrip != nil
	switch {
	case !hasTCP && !hasNtrip:
		return nil
	case hasTCP && hasNtrip:
		return fmt.Errorf("stream.pull: tcp and ntrip are mutually exclusive")
	case hasTCP && cfg.TCP.Address == "":
		return fmt.Errorf("stream.pull.tcp.address is required")
	case hasNtrip:
		if cfg.Ntrip.Address == "" {
			return fmt.Errorf("stream.pull.ntrip.address is required")
		}
		if cfg.Ntrip.Mountpoint == "" {
			return fmt.Errorf("stream.pull.ntrip.mountpoint is required")
		}
		if err := ntrip.CheckMountpointName(cfg.Ntrip.Mountpoint); err != nil {
			return fmt.Errorf("stream.pull.ntrip.mountpoint: %w", err)
		}
		if iv := cfg.Ntrip.NMEASendInterval; iv != nil {
			if err := ValidateNMEASendInterval(*iv); err != nil {
				return fmt.Errorf("stream.pull.ntrip.nmeaSendInterval: %w", err)
			}
		}
	}
	return nil
}
