package stream

import (
	"fmt"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

// Config matches the [stream] TOML table.
type Config struct {
	Pull *PullConfig `toml:"pull"`
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

// Validate checks the [stream] configuration.  An absent
// [stream.pull] table is valid (stream pull is optional).
func (cfg *Config) Validate() error {
	if cfg.Pull == nil {
		return nil
	}
	return cfg.Pull.Validate()
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
