# Optional features via build tags (#552)

`satpulsed` and `satpulsetool` link every feature they support. A build
for a small target -- an OpenWrt router, or eventually a microcontroller
-- pays for Prometheus metrics, an HTTP server and thirteen GPS protocol
families whether or not it uses them. This plan adds compile-time
exclusion of those features, with no change to the API callers use and no
new runtime machinery.

## Goal

- Build tags select the feature set. The default build is exactly what it
  is today, so a plain `make` is unaffected.
- Top-level features are subtractive: `noprom`, `nohttp`, `nopprof`. Each
  build names what it leaves out.
- Vendors are gated by `novendor`, which drops all vendor-specific
  protocol code and leaves NMEA and RTCM, with positive tags re-enabling
  individual vendors on top: `-tags novendor,ublox`.
- Which combinations we ship is policy and lives in the `Makefile` as
  named targets. Build tags stay one-per-feature and nothing names a
  deployment.
- A configuration that asks for an excluded feature fails through the
  error path that already exists. Where it does not, the seam is in the
  wrong place and moves; no availability registry is added.

## What the size actually is

Measured on amd64 with the current `make` flags, by cutting each
feature's packages from the import graph, recomputing reachability from
`main`, and attributing `go tool nm -size` bytes to the packages that
become unreachable. `satpulsed` is 20,036,385 bytes.

| part | % of binary |
|---|---|
| debug info and symbol table | 31.4 |
| `net/http` and the crypto tree it roots | 14.3 |
| Prometheus client, protobuf, expfmt, procfs | 9.3 |
| pprof endpoints | 1.6 |
| all nine vendors (u-blox 1.5, the rest 3.9) | 5.4 |
| PHC sync | 0.4 |
| Ntrip caster and client, own code | 0.2 |
| web UI: SSE and embedded assets | 0.1 |
| core: Go runtime, type tables, stdlib, satpulse's own code | 38.2 |

Two figures need a caveat. The Prometheus row is an extrapolation from
symbol bytes and is probably low: a direct build comparison of
`client_golang` plus `promhttp` against a `net/http` base measured
4.27 MB, where the extrapolation predicts about 2.7 MB, because the
uniform text-to-file scale factor understates protobuf's table-heavy
code. Call Prometheus 9-13%. Everything else held to within about 10% of
a direct measurement where one was possible.

Stripping is not a feature and not part of this plan, but it belongs in
the same conversation: `-ldflags "-s -w"` takes `satpulsed` from
20,036,385 to 13,754,530 bytes and `satpulsetool` from 16,957,704 to
11,620,514. Reflection, struct tags, TOML and JSON decoding, panic
tracebacks with file and line, and `runtime/pprof` symbolisation all
keep working, because `.gopclntab` and the runtime type metadata are
untouched. What is lost is Delve and gdb, `go tool nm`/`objdump`, and
`perf` symbol resolution.

## Why HTTP is one lump

Cutting `net/http` frees 14.3% because it is charged the whole crypto
tree: `tls`, `x509`, `ecdsa`, `ed25519`, `elliptic`, `math/big` and the
FIPS-140 modules. That tree is reachable from a plain cleartext server.
The linker's own `-dumpdep` output gives the path:

```
net/http.(*Server).Serve
  -> net/http.(*conn).serve
    -> net/http.(*conn).maybeServeUnencryptedHTTP2
      -> crypto/tls.(*Conn).clientHandshake-fm
        -> crypto/tls.(*Conn).clientHandshake
```

Every connection goes through `conn.serve`, which sniffs for the h2c
preface. To hand a cleartext socket to the HTTP/2 code, which wants a
`*tls.Conn`, that path calls `tls.Client`, and a method value roots the
whole handshake. The guard is a runtime map lookup on
`Server.TLSNextProto`, so the linker cannot see that it always returns
false for us.

Measured, all stripped: hello world 1.59 MB; importing `net/http` but
never serving 3.80 MB with 21 `crypto/tls` symbols, all type
descriptors; calling `Server.Serve` 5.78 MB with 572; adding an
`http.Get` 6.29 MB. So serving plain HTTP costs about 2 MB of crypto and
the client adds only 0.5 MB on top.

Neither obvious lever removes it. `-tags nethttpomithttp2` drops the h2
bundle but keeps `maybeServeUnencryptedHTTP2` and its `tls.Client` call:
on `satpulsed` it saves 0.50 MB and leaves the `crypto/tls` symbol count
at 572. `Server.Protocols` is read at run time inside
`onceSetNextProtoDefaults`, so it cannot help the linker either. This is
why the axis is all-or-nothing: with `net/http` you get the crypto tree,
and the only alternatives are no HTTP server or an HTTP/1.1 server that
is not `net/http`. The latter is out of scope here.

None of this applies to a TinyGo build. TinyGo's `crypto/tls` is a
shim of under 900 lines with no handshake at all -- `Handshake` is a
no-op and the comments say the network device performs it -- so there is
no crypto tree to root, and its `net/http` is a port of Go 1.21.4, which
predates `maybeServeUnencryptedHTTP2` entirely. The practical effect is
that the axes rank differently per target: on the host the big levers
are stripping and HTTP-plus-metrics, while on bare metal the big lever
is the vendor set, which is worth only about 5% on the host.

## Tag vocabulary and guard forms

```go
//go:build !noprom              // promobs and its wiring
//go:build !nohttp              // the HTTP wiring
//go:build !nopprof             // the pprof route registration
//go:build !novendor || ublox   // a per-vendor file in gpsreg
```

`novendor` with no positive tag leaves NMEA and RTCM, which is a
configuration `gpsreg` already expresses: `CreatePacketFormats` always
includes both and treats vendors as additive, and `VendorOther` already
means a vendor we do not support. So the exported API does not change.

A positive vendor tag on its own is inert, because `!novendor` is already
true, which is the desired property: `-tags ublox` cannot silently mean
something different from what it looks like.

Tag names must be Go build-tag identifiers, so the vendor tags are
`allystar`, `bynav`, `novatel`, `quectel`, `septentrio`, `sinognss`,
`techtotop`, `ublox`, `unicore` and `zhongke`. `ublox` is already an
accepted alias in `vendorMap`. MediaTek, Furuno, SkyTraq, Trimble and
`other` have no constructors or packet formats, so they need no tag.

The axes are nested, not orthogonal: without `net/http` there is no
`/metrics`, no `/sse`, no `/position` and no Ntrip caster. Express that
once, by having the `nohttp` build not compile the HTTP wiring at all,
rather than writing `!noprom && !nohttp` on each affected file.

## Step 1: pprof

The cheapest axis and the one that proves the mechanism: 1.6%, gated by
a single call to `registerPprofHandlers`, no API surface and no config
semantics to decide. Split that registration into a `!nopprof` file and
a stub that registers nothing.

## Step 2: the HTTP wiring

`startHTTP` already returns an `error` and `daemon.go` already propagates
it, so the `nohttp` stub is a function that returns an error when it is
given a non-empty endpoint list and nil otherwise. An HTTP endpoint in
the config then fails startup with a clear message, and a config with no
endpoints runs unchanged.

The Ntrip caster is the other server. `gps/app/ntrip/caster.go` is that
package's only use of `net/http`, and #553 builds it only without
`nohttp`, for the browser Workbench, which links the package for stream
configuration and source-table records but never runs the caster. Those
stay in both builds, and the raw-TCP Ntrip client and upload do not use
`net/http`. A native `nohttp` daemon additionally needs a stub
`ntrip.Start` with the same signature that returns an error, so a
configured caster fails startup the same way a configured HTTP endpoint
does.

`satpulsetool` also reaches `net/http` through `github.com/jaypipes/pcidb`,
used by `internal/sdpcmd` for PCI device names: the library imports it
for an optional database download that is off by default. A `nohttp`
build leaves the library out, keeping the sysfs inspection and showing
numeric PCI IDs.

`satpulsewb` is an HTTP server and has no `nohttp` build.

## Step 3: observers own their endpoints, then Prometheus

`noprom` does not fall out of the current wiring. `promobs.New` returns
only an observer, with no error, and `startHTTP` registers
`promObs.Handler()` under `cfg[i].metrics()`. A stub returning nil turns
a config request into a panic, not an error: `ServeMux.register` rejects
a nil handler with `http: nil handler`. That is a sign the seam is wrong
rather than a reason for new machinery -- the metrics observer is
currently reached from two places, constructed and added to the observer
chain in `daemon.go` and registered as a route in `http.go`.

Move the route to the observer. Add an optional interface for an observer
that also contributes HTTP routes, and have `startHTTP` ask the observer
chain what to register. Then `promobs` is one entry in that chain,
`daemon.go` is the only place naming it, and a `noprom` build simply has
one fewer observer with nothing referring to it. A `metrics = true`
config with no provider fails through the same lookup that would reject
any other unavailable endpoint.

The position endpoint and the SSE observer follow the same two-place
pattern today and can move to the same interface, which is what makes
this a seam change rather than a special case for Prometheus.

## Step 4: the vendor split

`reg.go` keeps its exported API and its tables become per-vendor
contributions. Five vendor-keyed things have to move, not one:

- `allVendorPacketFormatsMap`
- the processor map in `CreatePacketProcessors`, including the Quectel
  NMEA extension handler
- `CreateConfigProtocol`
- `FindNMEASVNumbering`, which imports `as`, `sino` and `ubx` for NMEA
  satellite numbering
- `novVariantFor`

The last two are the easy ones to miss: they are vendor-specific
behaviour hanging off NMEA, so a `novendor` build that keeps NMEA must
still drop them. A split that covers only packet formats leaves the
vendor packages linked and saves nothing.

Two properties fall out of putting each vendor's imports in its own file.
The package set is derived by the linker from the import graph, so there
is no vendor-to-package map to maintain: `nov` disappears exactly when
the Bynav, NovAtel, SinoGNSS and Unicore files are all absent, and the
per-vendor format libraries (`sbfbin`, `casbin`, `uncmsg`, `novmsg`,
`sdbpbin`, `asbin`, `qtmmsg`) go with their protocol packages. What
stays is genuinely shared: `rtcmbin`, `nmeamsg`, `ubxbin`, `ubxcfgval`,
`term`, `opt`, `check`.

Scan order needs care. `allVendorPacketFormats` is authoritative for
autodetect order, and registration from `init` would make order depend on
file names. Keep an untagged ordered list of `gpsprot.Tag` values --
strings only, no imports -- and have `CreatePacketFormats` walk it and
skip tags nobody registered.

The config side then works with no extra code. Put each vendor's
`vendorMap` entry, with its aliases (`casic`, `comnav`, `taidou`,
`ublox`), in the per-vendor file instead of deriving the map from an
untagged `vendorNames`. `ParseVendor` already returns
`unknown vendor: %q` for an unregistered name and `Vendor.UnmarshalText`
already forwards that into the TOML decode, so `vendor = "u-blox"` in a
`novendor` build fails config parsing with a good message.
`EnvVendors`'s `"all"` then means all available, which is also right.

Leave the `const` block untagged so `iota` does not shift and a `Vendor`
value keeps its meaning across builds. Only the name resolution goes.

## Profiles and testing

With several independent axes the product of combinations is untestable,
so the shipped combinations are named `make` targets that expand to tag
lists, and CI builds those. Nothing in a `//go:build` line names a
deployment: a tag guard that reads `openwrt || mcu` means adding a
profile edits every guarded file.

`make test` only exercises the default build, so the excluded
combinations are covered by compiling them, not by running the suite
under tags. Two builds are enough to catch the common breakage: all
features off, and one vendor only. For `nohttp`, also check that `go list
-deps` of each command built with the tag no longer contains `net/http`,
since a new import anywhere can bring it back without breaking the build.

## Open decisions

- Which profiles to name, and whether they strip. Stripping is per-build
  and the trade is field debuggability, so an OpenWrt or device build can
  strip while the normal Linux packages do not.

## Out of scope

- Replacing `client_golang` with a hand-written text exposition. It would
  remove about 3.8 MB from every build rather than only small ones, and
  `promobs` registers 26 metric families at construction plus about 14
  more lazily, uses `WithLabelValues` and `DeleteLabelValues` to create
  and destroy label children at run time, and has a histogram with
  computed buckets, so it means reimplementing the data model and its
  concurrency, not just the format. The existing replay test already
  parses the endpoint with `expfmt`, so a hand-written version could be
  held to the reference parser with the dependency becoming test-only.
  Worth doing on its own terms, not as a size fix smuggled in here.
- Eliminating `regexp`, worth 1.0%. The win only arrives when the last
  of the 22 call sites goes, and three of them in `gps/internal/unc` are
  validators with nested optional alternations where the pattern is the
  clearer statement of a vendor grammar.
- `math/big`, worth 0.8%, reached only from `gps/lib/decconv` for exact
  decimal scaling.
- An HTTP/1.1 server that is not `net/http`, which is the only way to
  serve HTTP without the crypto tree.
- Anything TinyGo-specific. This work is a size lever for any small
  build and belongs on a branch off `master`.

## Release note

Build-tag selection of optional features is user-facing, so implementing
it needs an entry in `docs/_includes/NEWS.md` in the same change.
