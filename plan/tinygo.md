# Build the GPS tree with TinyGo (#491)

Standard Go cannot produce a binary for a microcontroller, and on
WebAssembly it produces a large one. TinyGo addresses both. This plan covers
making the GPS tree build and run under TinyGo, for two targets that share
nearly all of the work:

1. **Embedded.** Run GPS processing on a microcontroller, where standard Go
   is not an option at all.
2. **WebAssembly, concretely SatPulse Workbench in the browser.** Standard Go
   already works there, so TinyGo is an optimisation of an existing product
   rather than an enabler: it is worth doing for download size, and nothing
   depends on it.

The two targets differ in what they demand. Embedded needs the whole runtime
story -- a UART, a scheduler that does not starve, no file system. The browser
needs only that the existing module get smaller. They converged on one
blocker, the missing `reflect.Select`, which is now supplied; what remains
shared is the build tags, the runtime gaps and the maintenance of that
adapter, which is why they belong in one plan.

## What builds today

All 65 non-main packages under `gps/` compile for `-target=riscv-qemu`,
checked by blank-importing every one of them from a single `main` package.
That includes `gpsreg` and its dependencies: `gpsprot`, the protocol packages
under `gps/internal/`, and the message and binary-format libraries. They also
pass `tinygo test -target=riscv-qemu`, once the packages that need a facility
bare metal lacks have excluded themselves (below).

That one-probe check is what the `Build (TinyGo)` workflow runs, generating
the probe from `go list` so it cannot go stale. A file whose name already
restricts it to Linux needs `!baremetal` as well, and it is easy to miss:
`gps/app/pps/sleep_linux.go` was exactly that, and nothing caught it until a
reviewer read the rule and looked for files it had not been applied to.

The same workflow then runs the tests under QEMU, split across three runners.
There is no list of packages to leave out: a package that cannot run on bare
metal says so itself, either with a build tag on the test file or with a skip
at the helper that needs the facility. TinyGo is pinned in the workflow, so a
version bump fails CI, which is the point: the `bcast` adapter has to be
rechecked against the new runtime.

The sweep is slow, and the cost is the compiler rather than the tests. TinyGo
builds each test binary whole-program, with no prebuilt standard library to
link against, so a package costs 4 seconds warm and 10 cold before a single
test runs -- `gps/ptime` has one test taking 0.05s and takes 4s to get there.
Across 59 packages that is minutes: locally, at the parallelism a standard
runner has, 2m02 warm and 2m54 cold, and on CI 5m22 in one job, 4m16 in two
shards, 3m58 in three. Each runner also pays a fixed cost of its own, about a
minute to install TinyGo and QEMU plus the cold compile of the standard
library, so the sharding saturates rather than scaling, and the shards are
uneven because they are split by position in the package list and not by cost:
in the three-shard run they took 2m30, 3m55 and 2m41.

Raising `tinygo test -p` above the core count makes it worse, not better: at
`-p 8` the two shards went from 4m16 to 4m39, four cores losing more to
contention than eight build jobs recovered. The remaining lever is caching
`~/.cache/tinygo`, which is what the cold-to-warm difference above would buy,
but it is 296 MB and a cache written by a pull request cannot be read by any
other, so it would pay nothing until it reached `master`.

It passes `-short`, for the reason given under the test volume below.

Note that a link failure, unlike a test failure, aborts the whole `./gps/...`
run at the package where it happens, so a package that fails to link hides
every package after it.

`-target=riscv-qemu` is the compile check to trust. TinyGo sets `GOOS=linux` on
bare-metal targets, so on ARM targets such as `pico` and `esp32c3` the
`_linux.go` files and `golang.org/x/sys/unix` compile against stub syscalls and
a clean build proves nothing. On `riscv-qemu` the GOARCH mismatch makes them
fail loudly. To compile-check a library package, build a `main` package that
blank-imports it.

The browser shell `cmd/satpulsewbwasm` also compiles for `-target=wasm`.

## Build tags

Platform-specific files are selected by capability, not by naming a target:

- `unix` means `golang.org/x/sys/unix` is a real syscall interface.
- `unix || windows` means `syscall.SIGHUP` exists.
- The complement gets a fallback: a `term` whose `Open` always fails, a
  `notifyReopen` that does nothing.

Bare metal needs the extra `!baremetal` half of each condition, because TinyGo
reports `GOOS=linux` on a chip where none of it works. So `ptime_unix.go` and
`term_unix.go` are `unix && !baremetal`, and the fallbacks are
`(!unix && !windows) || baremetal`. New platform-specific files follow the same
shape. The capability halves are not TinyGo-specific and are on `master`
already; only the `baremetal` halves belong here.

## Size on WebAssembly

Measured on `cmd/satpulsewbwasm`, TinyGo 0.42.0, sizes in bytes, Brotli at
quality 11 and gzip at level 9:

| Build | Raw | Gzip | Brotli |
| --- | ---: | ---: | ---: |
| Standard Go, `-ldflags '-s -w'` | 13,549,630 | 3,211,337 | 2,311,786 |
| TinyGo, default | 10,327,414 | 2,831,210 | 2,022,733 |
| TinyGo, `-no-debug` | 3,382,053 | 1,175,321 | 862,686 |

`-no-debug` is the whole point: without it TinyGo saves 12% compressed, with it
63%. Debug information consumes nearly the entire advantage, so an unqualified
TinyGo build looks barely worth having and would mislead anyone evaluating it.
`-opt=z` makes no difference, being already the default.

Two incidental findings. `net/http` costs 4,647 bytes of flash in the TinyGo
build and `crypto/tls` is absent entirely, because whole-program dead-code
elimination already drops the unreachable Ntrip caster: a `nohttp` build tag
and TinyGo address the same waste, and their savings do not add up. And
go-toml works under TinyGo, listing and parsing the built-in message-file
catalog on wasm and on `riscv-qemu` alike.

The figures were taken before the `reflect.Select` adapter below, so they
measure a module that compiled but panicked on the first broadcast. The
adapter is small and is not expected to change the conclusion, but the
combination has not been built or measured, and nobody has yet loaded a
TinyGo-built Workbench in a browser.

## Package-level regexps

A package-level `regexp.MustCompile` is an initializer TinyGo's compile-time
init interpreter cannot evaluate, so it stays as runtime init code and roots
the regexp engine and the unicode tables even where the regexp is never used.
On the ESP32-C3 a program with one unused package-level regexp needed 130,488
bytes of flash and 140,464 of RAM, against 5,580 and 8,616 with none. Each is
now wrapped in `sync.OnceValue`, so it compiles on first use and the linker
can drop it when unused; the only difference for a normal Go build is that a
bad pattern panics at first use rather than at init.

It pays on wasm too, by the same dead-code elimination. TinyGo `-no-debug`,
Brotli, with and without the change: a `gpsreg` plus `gpsdecode` probe is
158,979 against 207,030, and a `session` plus `gpsreg` plus `msgfile` probe is
317,845 against 341,077. The saving shrinks as more is linked, as a fixed cost
should.

This is a TinyGo optimisation, not a general one. The same change measured on
the standard Go browser Workbench makes it slightly *larger*, 2,316,524 Brotli
against 2,310,733, because the `sync.OnceValue` closures cost something and Go's
linker cannot drop the regexp engine anyway. So it stays on this branch.

## reflect.Select, and the adapter that supplies it

`gps/app/bcast` selects across a changing set of subscriber sends with
`reflect.Select`, which TinyGo does not implement. It panicked at run time,
not compile time, on wasm as well as on bare metal. Since `Session`
constructs a `bcast.Bcast[scan.Packet]` for every connection, and `bcast` also
backs the Ntrip caster and stream push and pull, nothing that served, relayed
or monitored worked under TinyGo at all.

PR #490 makes `Bcast.Run` call `selectCases`, which is `reflect.Select`
everywhere else and, under the `tinygo` tag, an adapter over the runtime's own select
primitive reached by `go:linkname`. The runtime keeps doing the blocking,
locking, choosing and waiter cleanup; the adapter translates `SelectCase`
values into its state and op slices, supplies the shared receive buffer, and
copies the received value back out with its real type. It pins the TinyGo
version, because it mirrors unexported runtime layouts, and it shuffles the
cases because the runtime scans them in order and a continuously ready input
would otherwise starve subscriber sends.

The package's tests pass on the host, on `riscv-qemu` and on `wasm`, and
`gps/app/ntrip` no longer panics on `reflect.Select`.

Two things follow from this being a mirror of runtime internals rather than a
rewrite of `bcast`:

- The version pin can only fail at run time: TinyGo exposes no version build
  tag, so a build-time guard is not available. It runs in an `init`, so an
  upgrade kills the program at startup rather than on the first packet.
- The adapter allocates per call, on the path that runs once per broadcast
  message: 4 allocations and 160 bytes with no subscribers, 10 and 640 bytes
  with four, measured on the host. That is affordable in a browser and worth
  measuring against a device's GC. The `order` slice and the shuffle are the
  removable part, and the better fix is upstream: `chanSelect` carries its own
  `TODO: start from a random index`, which would also make ordinary `select`
  statements fair.

The adapter is written as a prototype of an implementation to push upstream,
so it follows reflect's semantics and panic wording where the checks are the
same, and its TODOs record what a version inside `reflect` would still need:
default cases, a non-addressable received value, and reflect's own panics for
an unexported or non-channel `Chan`. Most of its machinery exists only because
it sits outside `reflect`; upstream would have the channel's element type and
`runtime.alloc` to hand and need neither the mirrored structs nor the version
check. Upstreaming it is the durable fix, and until then every TinyGo release
has to be checked against `src/runtime/chan.go`.

## Server.Protocols, done on master

TinyGo's `net/http` is a port of Go 1.21.4, which has no `Protocols` field, so
`gps/app/ntrip`'s caster did not compile at all. The caster hijacks the
connection and must therefore speak HTTP/1 only.

The caster now says that with a non-nil empty `TLSNextProto`, the older idiom,
with a comment giving the reason. That costs nothing: `net/http` imports
`crypto/tls` regardless, so naming `*tls.Conn` adds no dependency, and the
alternative, a build-tag split with a helper that sets `Protocols` only where
the field exists, was more machinery for the same result. `Protocols` arrived
in Go 1.24, so when TinyGo's `net/http` catches up, restore the direct form
and delete the comment.

## What bare metal cannot do, and how the tests say so

Four facilities are missing, and each is expressed in the code rather than in
a list of packages to skip:

- **No file system.** `gps/msgfile`'s tests write a temporary file and load it
  back, and `gps/internal/septentrio`'s read captures from `testdata`. In both
  the affected tests are spread across most of the package's test files, so
  tagging files out would discard the ones that work. Each instead routes its
  file-system use through a single helper that skips on a `hasFS` constant:
  `tempDir` and `loadTestFile` in `msgfile`, `readCapture` in septentrio,
  where eighteen tests skip and the other fifty-two still run. `go:embed`
  cannot reach outside a package directory, so embedding the packet logs
  instead would need more thought.
- **No network.** `net.Listen` returns "Netdev not set", so `gps/app/ntrip`'s
  caster fixture cannot serve. `newFixture` skips on a `hasNet` constant
  before it starts anything.
- **No `testing/synctest`.** TinyGo has no implementation, and this one fails
  to *link*, so it takes every test in the package with it. The files using it
  carry `!tinygo`.
- **Host-only by nature.** `gps/ts` generates TypeScript and `gps/lib/wakeup`
  measures Linux wake-up latency. Their test files carry `!baremetal`.

Emulation is also slow enough to change what a reasonable test costs:
`gps/scan`'s `TestGoodUBX` ran 50,000 random packets through a 64-byte buffer,
which is 0.14s on a host and was 85.7s on `riscv-qemu` -- on its own, most of
the whole sweep. It is 10,000 now, and 1,000 under `-short`, which CI passes.
That is not a TinyGo concern alone: a low-end OpenWrt target would want the
same.

Serving time on a device is out of scope whatever the facilities. SatPulse
feeds a server that already exists, chrony or a PHC for linuxptp, and a
microcontroller has neither.

## An unexplained memory fault

Two test files crash the `riscv-qemu` target outright, with a RISC-V load
access fault (`mcause=5`) or a misaligned load (`mcause=4`), rather than
failing: `gps/app/ntrip`'s `TestConfigValidate` and `gps/app/stream`'s
`TestConfigPullNtrip`. Both are configuration tests, and both files carry
`!baremetal` with a comment pointing here, so the sweep is green; they are not
excluded because bare metal lacks a facility, which is why they are recorded
separately from the list above.

What has been ruled out:

- Not go-toml. Decoding `ntrip.Config` from a TOML string works on
  `riscv-qemu`, and `TestConfigValidate` does not parse TOML at all: it builds
  `Config` literals and calls `Validate`.
- Not the lazy package-level regexps. Reverting them to eager
  `regexp.MustCompile` leaves the fault in place, and a `sync.OnceValue`
  regexp works in isolation on the same target.
- Not `bcast` or the `reflect.Select` adapter. The fault reproduces without
  either, and the earlier guess that it followed from an abandoned `bcast`
  goroutine is wrong: a minimal reproduction that starts `Run`, cancels and
  calls `t.Fatalf` does not fault.

That leaves something in the validation path itself. Two independent
configuration validators failing the same way suggests one bug rather than
two. It is the thing to settle before a device runs any of this, and it would
most likely be a TinyGo report rather than a change here.

## Hardware

No board has been chosen. `riscv-qemu` matches nothing real: four cores with
atomics, 100 MB of RAM, code in RAM. The leading candidate is the ESP32-C3,
because it is the chip QEMU stands in for, being the same rv32 family, and
because TinyGo's WiFi package `tinygo.org/x/espradio` supports it. From the
TinyGo target files and linker scripts: the C3 is one rv32imc core with no
atomics and no FPU, about 314 KB of DRAM and 8 MB of flash space; the S3 has
atomics, an FPU and 416 KB of DRAM but no QEMU target. espradio covers
`esp32`, `esp32c3` and `esp32s3`, not the C6, and its own QEMU target is
`riscv-qemu` plus a build tag for unit tests, emulating neither the chip nor
the radio.

How much RAM espradio's WiFi and TCP/IP stack leave on a C3 is unmeasured.
Size against a real chip needs no hardware: `tinygo build -size short -target=
xiao-esp32c3`.

## Open questions

1. **The memory fault in the two configuration tests** above. One bug, most
   likely, and probably a TinyGo one.
2. **Upstreaming the `reflect.Select` adapter**, with the runtime's random
   start, so neither target depends on a mirror of unexported layouts that
   each TinyGo release has to be rechecked against.
3. **A first device program.** `scan.New` takes an `io.Reader` and
   `machine.UART` has `Read`, so a device can skip `gpsio` and `term`
   entirely. It needs a reader wrapper that yields: `machine.UART.Read`
   returns `0, nil` when its buffer is empty and `scan` loops immediately on
   that, which would starve every other goroutine under TinyGo's cooperative
   scheduler.
4. **A real `Term` on `machine.UART`**, needed only to reuse `gpsio`, `stream`
   and `session`. Open: what `path` names mean, where TX/RX pins come from,
   and read timeouts by polling. It cannot be tested on `riscv-qemu`, whose
   `machine` package provides no UART.
5. **Per-protocol constructors in `gpsreg`.** Linking the registry pulls in
   every protocol family; a device wants one. Worth measuring against a real
   target once a device program exists.
6. **Compiling message files to JSON at build time**, if a device is to be
   configured from message files.
7. **Whether the browser target is worth a second toolchain** at all, given
   that it buys size alone and costs a permanently separate build. The
   measurement above says the saving is real; the decision is a product one.
