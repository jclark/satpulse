# Vendor declarations and experimental config protocols (#392)

Four branches add high-level config protocol support for new vendors
(casic, allystar, septentrio, quectel). Config protocol support is
tricky, so newly added config protocols should start as experimental:
present in the build, but not participating in unknown-vendor probing
until proven. This plan defines how experimental config protocols are
controlled, and introduces a `SATPULSE_VENDORS` environment variable
so that enabling them (and asserting a vendor generally) does not
require repeating `--vendor` on every invocation.

## Design shape

### Vendor is an assertion

Specifying a vendor - via `--vendor`, satpulsed's `vendor` config key,
or the `SATPULSE_VENDORS` declaration below - asserts what hardware is
attached. An assertion of vendor X always enables X's config protocol,
whether experimental or not: naming the vendor is the clearest possible
statement of intent, and refusing to configure explicitly named
hardware would be surprising.

### Experimental is a default, not a mode

There is no experimental flag, label, or mode. "Experimental" means
exactly one thing: the vendor's config protocol is excluded from the
default probe set used when no vendor is asserted. Nothing else about
the protocol differs.

Users only ever name vendors, and names do not change meaning over
time. This gives graduation invariance: any setting a user makes to
use vendor X means the same thing before and after X's config protocol
graduates to the default set, and graduation itself changes nothing
for them. (This is why an "enable experimental" flag is the wrong
approach: its referent would drift with every release, silently
changing users' probe sets.)

### The unasserted default is deliberately asymmetric

With no assertion, recognition and probing default differently,
because their risks differ:

- Recognition (passive): all packet formats are recognized. The cost
  of a large default is at worst misdetection.
- Probing (active): only non-experimental config protocols probe. The
  cost of a large default is transmitting packets at hardware that may
  mishandle them.

The default is "maximally capable passively, maximally cautious
actively". An assertion moves both axes to the asserted vendor(s).

### SATPULSE_VENDORS is a declaration

`SATPULSE_VENDORS` declares a fact about the machine: devices attached
here will be from one of these vendors (comma-separated vendor names).
Behavior is derived from the declaration; it serves as the ambient
default for an explicitly specified vendor - the `--vendor` flag or
satpulsed's TOML `vendor` key - which overrides it per invocation.
All the tools consult it, satpulsed included.

- Singleton (the typical user, who has exactly one receiver):
  equivalent in effect to `--vendor X` on every tool that takes a
  vendor, including vendor-specific tuning and dialect selection.
  Set once, everything behaves as asserted.
- Multiple vendors: a candidate set for the detection tools
  (recognition of their formats, probing of their config protocols).
  Tools that need a single vendor (e.g. replay's interpretation
  dialect) treat a multi-vendor declaration as no assertion - they
  cannot honestly do anything else.
- `all`: the vacuous declaration - anything may be attached.
  Recognize everything, probe everything including experimental.
  This is the development and test-harness setting.
- Unset: no declaration made. The asymmetric default above applies.
  The default is deliberately not expressible as a declaration value;
  it is the absence of one.

### What does not change

- decode and annotate: untouched. They identify single, already-split
  packets, where full-universe identification is the only sensible
  behavior; they take no vendor and gain none.
- scan and replay: no new behavior; their existing `--vendor` flags
  gain `SATPULSE_VENDORS` as an ambient default like every other
  vendor parameter.
- gpshwtest: sets `SATPULSE_VENDORS=all` in the environment of the
  tools it spawns. It genuinely does not assert - it discovers - and
  as a test harness, probing everything the build has is its job.
- Desktop GUI: inherits `SATPULSE_VENDORS` from the process
  environment with no code change.


## Implementation

Mostly in gpsreg.

### Vendor type

Get rid of VendorUnknown, but still start vendors at 1 (`iota + 1`),
so Vendor(0) is an invalid vendor value. The zero value has no name:
it means "no vendor specified" and exists only at parse boundaries
(`ParseVendor("")` and an absent TOML `vendor` key return it); it
never reaches the create functions as a vendor.

Consequences:

- `String()` loses its special "Unknown" case; Vendor(0) falls
  through to the `Vendor(%d)` form.
- `vendorNames` indexing keeps its off-by-one (name[i] is
  Vendor(i+1)).
- A package variable `allVendors` lists every valid vendor value.
- Existing `VendorUnknown` references (decodecmd, annotatecmd,
  convobscmd, scancmd/replaycmd defaults, satpulsewb, session and its
  tests, protocolMap) are migrated mechanically per the caller
  section below.

### EnvVendors

```go
func EnvVendors() ([]Vendor, error)
```

Parses `SATPULSE_VENDORS`:

- unset or empty: nil, no error
- `all`: all valid vendors (`all` combined with other names is an
  error)
- otherwise: comma-separated vendor names, each parsed by
  `ParseVendor` (so aliases work); whitespace around names is
  trimmed; an empty element or unrecognized name is an error naming
  the variable; duplicates are dropped, order preserved

For callers a non-nil error is fatal at startup.

### CreateConfigProtocol: the one wiring point

```go
// CreateConfigProtocol returns the config protocol for vendor, or
// nil if it has none.
func CreateConfigProtocol(vendor Vendor) gpsprot.ConfigProtocol
```

One-to-one switch: Ublox -> ubx, Unicore -> unc. This is the one
place a new config protocol has to be wired in; each config branch
adds one case (e.g. Zhongke -> casic).

There is no per-vendor counterpart on the formats side:
`CreatePacketFormats` below consults `allVendorPacketFormatsMap`
directly.

### Two-stage resolution

Resolution is split into two stages so that a single `[]Vendor` is
threaded everywhere instead of a (vendor, envVendors) pair.

Stage 1 combines the explicitly specified vendor with the
declaration. It lives in the application layer (`gps/app/cmd`), the
one impure step, called once per entry point:

```go
// ResolveVendors returns the vendor list in effect: the explicitly
// specified vendor if any, else the SATPULSE_VENDORS declaration,
// else nil.
func ResolveVendors(vendor gpsreg.Vendor) ([]gpsreg.Vendor, error)
```

Stage 2 is the gpsreg create functions, pure functions of the one
list, each defaulting an empty list per its axis:

```go
var defaultConfigProtocolVendors = []Vendor{VendorUblox, VendorUnicore}
```

This is what controls the non-experimental config protocols, and is
the entire representation of "experimental": a vendor whose config
protocol exists but is not listed here is experimental. Graduation
is adding the vendor to this list.

```go
func CreateConfigProtocols(vendors []Vendor) []gpsprot.ConfigProtocol
```

An empty list defaults to `defaultConfigProtocolVendors`; each
vendor maps through `CreateConfigProtocol`, dropping nils, order
preserved, so with nothing set the probe order stays ubx then unc,
unchanged for existing users. A vendor with no config protocol
contributes nothing, so an explicitly specified one yields an empty
result: listen-only detection, as today.

```go
func CreatePacketFormats(vendors []Vendor) []gpsprot.PacketFormat
```

An empty list defaults to `allVendors`. Scan order matters and comes
solely from the flat `allVendorPacketFormats` list, which stays
authoritative: the result is NMEA and RTCM followed by a walk of
that list, keeping each format that belongs to at least one given
vendor per `allVendorPacketFormatsMap` (which loses its
VendorUnknown entry; the order of its entries is irrelevant).
Membership is compared by `Tag()`, not interface equality (some
PacketFormat implementations hold func fields, and comparing those
panics). `CreatePacketFormats(nil)` yields the whole flat list -
today's VendorUnknown format set in its current order. Vendors
sharing formats (Unicore's entry includes the nov formats) need no
dedup; the flat list has none.

```go
func CreatePacketProcessors(vendors []Vendor) map[gpsprot.Tag]gpsprot.PacketProcessor
```

The processor map itself stays complete (all tags) as today; the
vendor-specific tuning (`SetVendor`: NMEA SV numbering, nov dialect)
is applied iff exactly one vendor is given. This is what makes a
singleton declaration equivalent to `--vendor X` everywhere,
including replay's dialect selection.

`protocolMap` initializes from `CreatePacketFormats(nil)`.

### Callers

Every entry point with a vendor parameter calls
`cmd.ResolveVendors` once at startup (error fatal) and threads the
resulting `[]Vendor` down. The call sits in the tool's entry
function, never inside flag parsing or config loading: those
functions stay environment-free, so their tests are hermetic
without any env isolation.

- gpscmd, scancmd, replaycmd: `Cmd` resolves after `parseFlags`
  succeeds and passes the list to `run`. `parseFlags`/`resolveConn`
  do not touch the environment.
- satpulsewb: resolves at startup; the list is stored on the server
  (it replaces the scalar `--vendor` field) and passed to
  `session.Connect` on every connect. A singleton list also drives
  message-file preselection (`sessionVendorName`), so a singleton
  declaration preselects like `--vendor` does.
- satpulsed: resolves `cfg.GPS.Vendor` in the daemon entry right
  after `LoadConfig` (same fatal exit as a bad config file) and
  passes the list to `run`; the three call sites use `gpsreg.Create*`
  directly. `GPSConfig` stays purely the TOML representation, and
  `LoadConfig` stays environment-free.
- decodecmd, annotatecmd, convobscmd: `CreatePacketFormats(nil)`,
  deliberately env-blind - they identify single, already-split
  packets against the full universe.

`gps/app/session` holds no vendor state of its own beyond the
connection: `Connect(op, vendors []gpsreg.Vendor)` takes the
resolved list, immutable while the connection lives, and the
internal `gpsreg.Create*` calls use it. The desktop GUI picks this
up when it next merges master, resolving at app startup like
satpulsewb.

gpshwtest: `tool.py` sets `SATPULSE_VENDORS=all` in the environment
of the satpulsetool processes it spawns.

### Docs and release notes

- Document `SATPULSE_VENDORS` in the satpulsed.8, satpulsetool.1
  (and the gps/scan subcommand pages), and satpulsewb.1 man pages;
  satpulsed.8 and satpulsewb.1 already have environment sections.
  No new TOML key, so satpulse.toml.5 is unchanged apart from noting
  the env var's interaction with `vendor`.
- NEWS.md entry (user-facing feature).

### Tests

In reg_test.go: EnvVendors parsing via `t.Setenv` (unset, list,
aliases, whitespace, `all`, `all` mixed with names, unknown name,
duplicates); CreatePacketFormats filtering and order (empty list,
single vendor, unicore+novatel dedup); CreateConfigProtocols
default vs given-vendor behavior. In gps/app/cmd: ResolveVendors
precedence (explicit vendor wins, declaration otherwise, nil when
neither, error on malformed declaration). Keep proportional - the
creators are table lookups.