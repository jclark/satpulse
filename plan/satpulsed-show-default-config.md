# Add `--show-default-config` to satpulsed (#169)

## Summary
Add `-C/--show-default-config` flag to satpulsed that outputs a TOML file with default configuration and helpful comments, similar to syncsim's implementation.

## Key Challenges

### 1. NaN Fields (antennaCableDelay, antennaCableLength)
These default to `math.NaN()` meaning "don't modify GPS receiver's setting". TOML can't represent NaN, but we need them in output for discoverability.

**Approach:**
1. Create default config (includes NaNs)
2. Overwrite NaN fields with typical example values
3. Encode to TOML string
4. Use regexp to comment out those overwritten fields
5. Comments for these fields say "omit to keep GPS receiver's setting"

**Result:**
```toml
[gps]
config = true
surveyTime = 2000
# antennaCableDelay = 100.0
# antennaCableLength = 10.0
antennaCableVF = 0.66
```

### 2. Empty Arrays
Empty arrays won't appear in output. Include zero-valued dummy entries (syncsim pattern):
```go
HTTP: []HTTPConfig{{}},
```

### 3. Flag Implementation
`-C` should work without requiring a config file. Handle like `--help`/`--version` - check before config file requirement, output and exit.

## Files to Modify

### internal/daemon/config.go
Add `comment` tags to Config, SerialConfig, PHCConfig, LeapSecondConfig, PTPConfig, PTP4LConfig, NTPConfig, NTPSockConfig, LogConfig.

Add `WriteDefaultConfig(w io.Writer) error`:
```go
func WriteDefaultConfig(w io.Writer) error {
    cfg := defaultConfig()
    // Overwrite NaN fields with example values
    cfg.GPS.AntennaCableDelay = 100.0
    cfg.GPS.AntennaCableLength = 10.0

    var buf bytes.Buffer
    if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
        return err
    }

    // Comment out the NaN-replacement fields
    s := buf.String()
    s = regexp.MustCompile(`(?m)^antennaCableDelay = `).ReplaceAllString(s, "# antennaCableDelay = ")
    s = regexp.MustCompile(`(?m)^antennaCableLength = `).ReplaceAllString(s, "# antennaCableLength = ")

    _, err := io.WriteString(w, s)
    return err
}
```

Modify `defaultConfig()` to include dummy array entries:
```go
cfg.HTTP = []HTTPConfig{{}}
cfg.Proxy.TCP = []TCPService{{}}
cfg.Proxy.Socket = []SocketService{{}}
```

### internal/daemon/gps.go
Add `comment` tags to GPSConfig fields. For NaN fields:
```go
AntennaCableDelay  float64 `toml:"antennaCableDelay" comment:"Antenna cable delay in ns (omit to keep GPS setting)"`
AntennaCableLength float64 `toml:"antennaCableLength" comment:"Antenna cable length in m (omit to keep GPS setting)"`
```

### internal/daemon/http.go
Add `comment` tags to HTTPConfig.

### internal/proxy/proxy.go
Add `comment` tags to Config, Options, TCPService, SocketService.

### internal/daemon/flags.go
Add `showDefaultConfig` to `flagVars` struct:
```go
type flagVars struct {
    // ... existing fields ...
    showDefaultConfig bool
}
```

Add flag and check before config file requirement:
```go
flags.BoolVarP(&vars.showDefaultConfig, "show-default-config", "C", false, "print default config as TOML and exit")

// After parsing, before config file check:
if vars.showDefaultConfig {
    return &vars, "", nil
}
```

### internal/daemon/daemon.go
In `Cmd()`, check before `LoadConfig`:
```go
if vars.showDefaultConfig {
    if err := WriteDefaultConfig(os.Stdout); err != nil {
        cmd.ErrPrintln(progName, err)
        os.Exit(1)
    }
    return
}
```

## Implementation Order

1. Add `comment` tags to all config structs
2. Add corresponding `description` fields to `configs/config-schema.json`
3. Add dummy array entries to `defaultConfig()`
4. Add `WriteDefaultConfig()` with NaN field handling
5. Add `-C` flag handling in `parseFlags`
6. Test: `go run ./cmd/satpulsed -C`
