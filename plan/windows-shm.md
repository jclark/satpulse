# NTP SHM refclock support on Windows (#339)

Depends on `plan/windows-port.md`: the tree must cross-compile and run on
Windows first. Related to `plan/windows-svc.md` (#338): the SHM writer is
part of the macOS-style feature set the service plan assumes works on
Windows, but it is independent of the service integration itself and can
land separately.

## Goal

Make `time/lib/ntpshm` write samples to the ntpd/NTPsec SHM refclock on
Windows, bringing the daemon up to the macOS feature set. Today the
package builds on Windows via `shm_stub.go`, whose `newShmWriter` returns
`ErrUnsupported`; a configured `[ntp.shm]` segment therefore fails to
attach. After this work, `ntpshm.New` attaches to a Win32 named file
mapping that ntpd (built with `SYS_WINNT`) reads, exactly as on Unix.

The SHM writer is wired through `daemon.NewSHMWriter` /
`gpsevent.dispatcher` and is gated only on `cfg.NTP.SHM`, independent of
PHC/PTP (`clk == nil`), so no daemon changes are needed -- this is
entirely inside `time/lib/ntpshm`.

## Background and current state

- `shm.go` (`//go:build linux || darwin`) attaches via SysV shared
  memory: `unix.SysvShmGet(shmKey(segment), expectedSize, IPC_CREAT|mode)`
  then `SysvShmAttach`, casting the mapped bytes to `*shmTime`. `write`
  publishes timestamps with the `Valid`/`Count` seqlock dance; `close`
  detaches.
- `shm_stub.go` (`//go:build !linux && !darwin`) is the no-op fallback
  that returns `ErrUnsupported`. Windows currently uses it.
- ntpd's `refclock_shm.c` under `SYS_WINNT` uses a paging-file-backed
  named file mapping instead of SysV. This plan reimplements `shmWriter`
  against that mechanism, leaving the rest of the package untouched.

## The struct layout carries over unchanged

On windows/amd64 the C `struct shmTime` is byte-identical to the existing
64-bit Linux/Darwin layout: `time_t` is 64-bit (8-byte aligned) and every
other field is a 4-byte `int`/`unsigned`, giving the same `0x60`-byte
struct as `ztypes_linux.go`. So the generated `shmTime` and `shmSec =
int64` are reused as-is -- no cgo, no new `go:generate`/`ztypes` step.

ntpd's `struct shmTime` (from `refclock_shm.c`), for reference:

```c
struct shmTime {
    int mode;
    volatile int count;
    time_t clockTimeStampSec;
    int clockTimeStampUSec;
    time_t receiveTimeStampSec;
    int receiveTimeStampUSec;
    int leap;
    int precision;
    int nsamples;
    volatile int valid;
    unsigned clockTimeStampNSec;
    unsigned receiveTimeStampNSec;
    int dummy[8];
};
```

This matches the cgo-derived fields field-for-field (`unsigned` nsec maps
to the existing `int32`; harmless, the values are sub-second nanoseconds).

## The Windows attach mechanism

A new `shm_windows.go` (`//go:build windows`) provides the same
unexported surface the stub does -- a `shmWriter` plus `newShmWriter`,
`write`, `close`. `write` and `init` are identical to the Unix versions
(pure `atomic` stores into `w.t`); only attach/detach differ. The struct
gains a Windows handle alongside the mapped pointer:

```go
type shmWriter struct {
    t    *shmTime
    h    windows.Handle // file-mapping object handle
    addr uintptr        // MapViewOfFile base, for UnmapViewOfFile
}
```

`newShmWriter`:

1. Build the object name. ntpd uses `"%s\\NTP%d"` with the namespace
   prefix `Local` for `segment < 2` and `Global` for `segment >= 2`
   (`segment & 0xFF`). This is the exact analog of the Unix `0600` vs
   `0666` mode split in `shmMode`: the `>= 2` "world" tier maps to the
   `Global\` namespace.
2. For `segment >= 2`, build a `*windows.SecurityAttributes` whose
   descriptor grants Everyone full access. Prefer
   `windows.SecurityDescriptorFromString("D:(A;;GA;;;WD)")` over
   hand-rolling a NULL DACL -- same effect (world-accessible), but
   explicit and not flagged by security scanners. For `segment < 2`,
   pass `nil` (process-default DACL).
3. `h, err := windows.CreateFileMapping(windows.InvalidHandle, sa,
   windows.PAGE_READWRITE, 0, expectedSize, name)` -- `InvalidHandle`
   (ntpd's `(HANDLE)0xffffffff`) backs the mapping with the system paging
   file. The call opens the mapping if it already exists, matching
   `IPC_CREAT` open-or-create semantics.
4. `addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_WRITE, 0, 0,
   expectedSize)`; on error `CloseHandle(h)`.
5. `w := shmWriter{t: (*shmTime)(unsafe.Pointer(addr)), h: h, addr: addr};
   w.init()`.

The returned `Attach` keeps `Segment` and the existing numeric `Key`
(`shmKey`) for parity in the daemon's "attached" log line, even though the
Win32 object is addressed by name rather than key.

`close`: `windows.UnmapViewOfFile(w.addr)` then `windows.CloseHandle(w.h)`
(guarding the zero-value writer as the Unix `close` guards `len(data) ==
0`).

All APIs are in `golang.org/x/sys/windows` (already a dependency, v0.43.0,
used by `gps/lib/term/term_windows.go`).

## Build-tag changes

- `shm.go` stays `//go:build linux || darwin`.
- `shm_stub.go` narrows to `//go:build !linux && !darwin && !windows`.
- `shm_sec.go` currently defines `shmSec = int64` under `(linux ||
  darwin) && (amd64 || arm64)`. Widen its constraint to include `windows`
  (Windows builds target amd64), or add a `shm_sec_windows.go`. Widening
  the existing file is simpler.
- New `shm_windows.go` (`//go:build windows`).

## A real semantic difference to document

A SysV segment outlives every process; a Win32 named mapping is
refcounted by open handles and is destroyed when the last handle closes.
In normal operation this is invisible -- satpulsed holds its handle for
its whole run, ntpd holds its -- but if satpulsed exits while ntpd is
between reads/restarts, the segment can disappear and ntpd recreates it on
its next `CreateFileMapping`. The open-or-create call self-heals, but the
difference from Linux (where the segment and its last sample persist
across a writer restart) warrants a comment in `shm_windows.go`.

## Testing

- `shm_test.go` is `//go:build linux || darwin` and uses `unix.SysvShm*`
  for the existing-segment round trip, so it stays Unix-only.
- `TestWriteRoundTrip` constructs `shmWriter{t: &s}` over a local struct
  and exercises only the portable `write` format. Move it to a
  build-tag-free file (e.g. `ntpshm_test.go`, or a new
  `write_test.go`) so it also runs on Windows and guards the on-wire
  layout there. This needs the `shmWriter` literal to be constructible
  with just `t` set, which the proposed struct allows (`h`/`addr` zero).
- An attach round trip on Windows (create two mappings on the same name,
  write through one, read through the other) is a useful `*_windows_test.go`
  addition but optional; the format test plus `build-windows.yml` going
  green is the baseline.
- `make test` / `go test ./...` on Linux and macOS are unchanged.

## Release notes

User-facing (Windows gains NTP SHM output), so completing this adds a
`docs/_includes/NEWS.md` entry under the current unreleased version,
referencing the issue number.

## Files changed or added

- `time/lib/ntpshm/shm_windows.go` -- new `shmWriter` over
  `CreateFileMapping`/`MapViewOfFile`.
- `time/lib/ntpshm/shm_stub.go` -- narrow build tag to exclude windows.
- `time/lib/ntpshm/shm_sec.go` -- widen to include windows (or add
  `shm_sec_windows.go`).
- `time/lib/ntpshm/*_test.go` -- move the portable write-format test out
  of the Unix-tagged file.
- `docs/_includes/NEWS.md` -- feature entry.
