// Package ntrip implements an Ntrip caster that serves RTCM correction
// data from a GPS receiver to connecting Ntrip clients (rovers).
package ntrip

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// SharedStreamConfig holds the global STR-record fields that apply to
// every stream served from this satpulse instance, whether by the
// caster or by stream.push entries.  Embedded in ntrip.Config; also
// used for stream.push.
//
// All fields are operator-overridable.  StreamRecordBuilder resolves
// unset fields from gps state at construction time but does not mutate
// this struct.
type SharedStreamConfig struct {
	Country       string                 `toml:"country" comment:"ISO 3166 country code shown in the source table"`
	Network       string                 `toml:"network" comment:"Network name shown in the source table"`
	Lat           opt.Val[gpsprot.Angle] `toml:"lat" comment:"Latitude override; default derived from receiver fixed position"`
	Lon           opt.Val[gpsprot.Angle] `toml:"lon" comment:"Longitude override; default derived from receiver fixed position"`
	Generator     string                 `toml:"generator" comment:"Generator string; default receiver hardware or satpulse version"`
	FormatDetails string                 `toml:"formatDetails" comment:"RTCM message list override; default synthesised from enabled GNSS"`
	Bitrate       int                    `toml:"bitrate" comment:"Default per-mountpoint bitrate"`
}

// StreamConfig holds per-stream STR-record fields, embedded in both
// MountConfig (caster) and stream.NtripPushConfig (push destination).
type StreamConfig struct {
	Description string `toml:"description" comment:"Mountpoint description shown in the source table (default = name)"`
	Bitrate     int    `toml:"bitrate" comment:"Bitrate shown in the source table for this mountpoint"`
}

// Config is the TOML configuration for the Ntrip caster.
type Config struct {
	Listen             string `toml:"listen" comment:"TCP listen address"`
	SharedStreamConfig        // embedded
	Users      []UserConfig  `toml:"user" comment:"Defined users for basic authentication"`
	Mountpoint []MountConfig `toml:"mountpoint" comment:"Mountpoints served by the caster"`
}

// UserConfig describes one Ntrip user with username/password credentials.
type UserConfig struct {
	Username string `toml:"username" comment:"Username for basic authentication"`
	Password string `toml:"password" comment:"Password for basic authentication"`
}

// MountConfig describes one mountpoint served by the caster.
type MountConfig struct {
	Name         string   `toml:"name" comment:"Mountpoint name (URL path component)"`
	StreamConfig          // embedded
	Users        []string `toml:"users" comment:"Restrict access to these users (default all)"`
}

// DefaultListen is the default listen address used when [ntrip.listen]
// is unset.  2101 is the IANA-assigned Ntrip port.
const DefaultListen = ":2101"

// DefaultCountry is used when [ntrip.country] is unset.  ISO 3166
// user-assigned range, conventional "unknown" placeholder.
const DefaultCountry = "XXX"

// DefaultNetwork is used when [ntrip.network] is unset.
const DefaultNetwork = "Misc"

// mountpointNameRE validates a mountpoint name per the Ntrip v2 spec:
// up to 100 characters of [A-Za-z0-9._-].
var mountpointNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// formatDetailsRE validates an STR-record format-details string:
// comma-separated alphanumeric/underscore tokens, each optionally
// followed by (digits) for an update rate in seconds.
var formatDetailsRE = regexp.MustCompile(`^[A-Za-z0-9_]+(\([0-9]+\))?(,[A-Za-z0-9_]+(\([0-9]+\))?)*$`)

// Validate checks that the [ntrip] configuration is internally
// consistent.  An empty [ntrip] section is valid (it supplies
// STR-record defaults for stream.push entries even when no caster
// runs).  Whether to start the caster is decided by the caller
// based on len(cfg.Mountpoint).
func (cfg *Config) Validate() error {
	if cfg.FormatDetails != "" && !formatDetailsRE.MatchString(cfg.FormatDetails) {
		return fmt.Errorf("ntrip: invalid formatDetails %q", cfg.FormatDetails)
	}
	if cfg.Bitrate < 0 {
		return fmt.Errorf("ntrip: invalid bitrate %d", cfg.Bitrate)
	}
	for _, fld := range []struct{ name, value string }{
		{"country", cfg.Country},
		{"network", cfg.Network},
		{"generator", cfg.Generator},
	} {
		if err := checkSTRField(fld.value); err != nil {
			return fmt.Errorf("ntrip.%s: %w", fld.name, err)
		}
	}
	users := make(map[string]struct{}, len(cfg.Users))
	for i := range cfg.Users {
		u := &cfg.Users[i]
		if u.Username == "" {
			return fmt.Errorf("ntrip.user[%d]: username is required", i)
		}
		if _, dup := users[u.Username]; dup {
			return fmt.Errorf("ntrip.user: duplicate username %q", u.Username)
		}
		users[u.Username] = struct{}{}
	}
	names := make(map[string]struct{}, len(cfg.Mountpoint))
	for i := range cfg.Mountpoint {
		m := &cfg.Mountpoint[i]
		if !mountpointNameRE.MatchString(m.Name) {
			return fmt.Errorf("ntrip.mountpoint[%d]: invalid name %q", i, m.Name)
		}
		if _, dup := names[m.Name]; dup {
			return fmt.Errorf("ntrip.mountpoint: duplicate name %q", m.Name)
		}
		names[m.Name] = struct{}{}
		if m.Bitrate < 0 {
			return fmt.Errorf("ntrip.mountpoint[%q]: invalid bitrate %d", m.Name, m.Bitrate)
		}
		if err := checkSTRField(m.Description); err != nil {
			return fmt.Errorf("ntrip.mountpoint[%q].description: %w", m.Name, err)
		}
		for _, name := range m.Users {
			if _, ok := users[name]; !ok {
				return fmt.Errorf("ntrip.mountpoint[%q]: undefined user %q", m.Name, name)
			}
		}
	}
	return nil
}

// checkSTRField rejects characters that would break the
// semicolon-separated, CRLF-terminated source-table line format if
// embedded in an operator-controlled STR field.
func checkSTRField(s string) error {
	if i := strings.IndexAny(s, ";\r\n"); i >= 0 {
		return fmt.Errorf("disallowed character %q at offset %d", s[i:i+1], i)
	}
	return nil
}

// listen returns the configured listen address, applying the default.
func (cfg *Config) listen() string {
	if cfg.Listen == "" {
		return DefaultListen
	}
	return cfg.Listen
}
