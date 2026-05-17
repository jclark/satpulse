package stream

import (
	"fmt"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/ntrip"
	"github.com/jclark/satpulse/gps/gpsreg"
)

// Config matches the [stream] TOML table.
type Config struct {
	Pull *PullConfig  `toml:"pull"`
	Push []PushConfig `toml:"push"`
}

// PullConfig matches the [stream.pull] TOML table.  Exactly one of
// TCP or Ntrip must be set.
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
// by which sub-table is present; currently only ntrip is supported.
// Protocol is the packet format forwarded to the destination; an
// unset value defaults per transport.
type PushConfig struct {
	Protocol gpsreg.Protocol  `toml:"protocol"`
	Ntrip    *NtripPushConfig `toml:"ntrip"`
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

// Validate checks the [stream] configuration.  Absent [stream.pull]
// and absent [[stream.push]] are both valid (stream is optional).
func (cfg *Config) Validate() error {
	if cfg.Pull != nil {
		if err := cfg.Pull.Validate(); err != nil {
			return err
		}
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
	if cfg.Ntrip == nil {
		return fmt.Errorf("must configure ntrip")
	}
	// Ntrip is currently the only transport, so defaulting Protocol
	// to RTCM here is correct.  When a second transport is added,
	// move this defaulting into a per-transport step (TCP, for
	// example, has no obvious default and should require an
	// explicit protocol).
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

// hasNtrip reports whether any [[stream.push]] entry uses ntrip.*.
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

// Prepare builds a PullSetup for cfg, or returns nil when cfg is
// nil.  version feeds the Ntrip User-Agent header.  pw and portLock
// are the correction-output port (the receiver's main serial
// connection in the simplest case).
func (cfg *PullConfig) Prepare(version string,
	pw PacketWriter, portLock gpsio.OutPortLock) *PullSetup {
	if cfg == nil {
		return nil
	}
	var src Source
	var addr string
	switch {
	case cfg.TCP != nil:
		addr = cfg.TCP.Address
		src = &TCPSource{Addr: addr}
	case cfg.Ntrip != nil:
		addr = cfg.Ntrip.Address
		src = &NtripSource{
			Addr:       cfg.Ntrip.Address,
			Mountpoint: cfg.Ntrip.Mountpoint,
			Username:   cfg.Ntrip.Username,
			Password:   cfg.Ntrip.Password,
			UserAgent:  NtripUserAgent{Version: version},
		}
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
// required fields.
func (cfg *PullConfig) Validate() error {
	hasTCP := cfg.TCP != nil
	hasNtrip := cfg.Ntrip != nil
	switch {
	case hasTCP && hasNtrip:
		return fmt.Errorf("stream.pull: tcp and ntrip are mutually exclusive")
	case !hasTCP && !hasNtrip:
		return fmt.Errorf("stream.pull: must configure either tcp or ntrip")
	case hasTCP && cfg.TCP.Address == "":
		return fmt.Errorf("stream.pull.tcp.address is required")
	case hasNtrip:
		if cfg.Ntrip.Address == "" {
			return fmt.Errorf("stream.pull.ntrip.address is required")
		}
		if cfg.Ntrip.Mountpoint == "" {
			return fmt.Errorf("stream.pull.ntrip.mountpoint is required")
		}
	}
	return nil
}
