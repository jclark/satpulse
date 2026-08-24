module github.com/jclark/satpulse

go 1.25.0

require golang.org/x/sys v0.47.0

require (
	github.com/jaypipes/pcidb v1.1.1
	github.com/jclark/crc24q v0.0.0-20240116045836-0ddb95a0c2ab
	github.com/mdlayher/netlink v1.11.2
	github.com/prometheus/client_golang v1.24.1
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.70.1
	github.com/spf13/pflag v1.0.10
	go.bug.st/serial v1.8.0
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

require (
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/mdlayher/socket v0.6.1 // indirect
	github.com/pelletier/go-toml/v2 v2.3.1
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
)

// go-toml v2.4.0 through v2.4.3 silently skip encoding.TextUnmarshaler on
// named types with a scalar underlying kind (pelletier/go-toml#1109),
// breaking gpsreg.Protocol parsing in proxy configs. Fixed upstream after
// v2.4.3; remove these when v2.4.4 is released.
exclude (
	github.com/pelletier/go-toml/v2 v2.4.0
	github.com/pelletier/go-toml/v2 v2.4.1
	github.com/pelletier/go-toml/v2 v2.4.2
	github.com/pelletier/go-toml/v2 v2.4.3
)
