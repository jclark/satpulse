package stream

import (
	"fmt"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/ntrip"
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

// NtripConfig matches [stream.pull.ntrip].
type NtripConfig struct {
	Address    string `toml:"address"`
	Mountpoint string `toml:"mountpoint"`
	Username   string `toml:"username"`
	Password   string `toml:"password"`
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

// Prepare builds a PullSetup for cfg, or returns nil when the pull
// is disabled (neither tcp nor ntrip set).  version feeds the Ntrip
// User-Agent header.  pw and portLock are the correction-output port
// (the receiver's main serial connection in the simplest case).
func (cfg *PullConfig) Prepare(version string,
	pw PacketWriter, portLock gpsio.OutPortLock) *PullSetup {
	var src Source
	var addr string
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
		return nil
	}
	return &PullSetup{
		pull:       NewPull(),
		source:     src,
		addr:       addr,
		pktFormats: defaultPullFormats,
		pw:         pw,
		portLock:   portLock,
	}
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
	}
	return nil
}
