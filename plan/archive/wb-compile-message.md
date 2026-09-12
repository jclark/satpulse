# Compile the default message-file library into satpulsewb (#386)

satpulsewb ships the default GPS message-file library inside the
binary, so the Workbench works with no installed library: standalone
Windows and macOS binaries, `go install`ed binaries, and builds run
from a checkout all get the full catalog. `SATPULSE_GPSMSG_PATH`
remains the way to bring on-disk files in: its directories are
searched ahead of the built-in library, and no disk lookup happens
at all when it is unset.

## Search-path semantics

- Env var unset: the built-in library only.
- Env var set: the listed directories in order, then the built-in
  library. First match by name wins, so a file in an env directory
  shadows a same-named built-in; built-ins without a shadow stay in
  the catalog.

The user config dir library (`~/.config/satpulse/gpsmsg` etc.) and
the installed system libraries are dropped from satpulsewb's search
path entirely. `msgDirs()` in `cmd/satpulsewb/msgfile.go` collapses
to: env dirs (when set) followed by the built-in entry. Delete
`msgdirs_unix.go`, `msgdirs_darwin.go`, `msgdirs_darwin_amd64.go`,
`msgdirs_darwin_arm64.go`, `msgdirs_windows.go`, and the
`defaultDirs`/`os.UserConfigDir` logic. The installed
`/usr/share/satpulse/gpsmsg` library remains, for satpulsetool's
documented manual use; satpulsewb just no longer reads it.

## The embedded library

`gps/msgfile` itself gains:

- `gpsmsg.zip` -- the default library (`configs/gpsmsg/*/*.toml`,
  about 173K of TOML, about 37K deflated), checked in.
- `//go:embed gpsmsg.zip` and one exported function (e.g.
  `Builtin()`) returning the built-in library as a `Dir` (see
  below), beside the other search-path functions in `name.go`.
  `archive/zip`'s `Reader` implements `fs.FS`, so the accessor is
  `zip.NewReader` over the embedded bytes (lazily,
  `sync.OnceValue`); files inflate per `Open`, with no extraction
  step.
- The generator (below) as a `//go:build ignore` file.

The zip being checked in, the package compiles everywhere with no
generation step, and binaries that never call `Builtin` do not
carry the bytes: the linker's dead-code elimination drops an embed
variable reachable only from an uncalled function (verified with a
test build). So satpulsetool, which depends on `gps/msgfile` via
`internal/gpscmd`, is unaffected, and satpulsed has no
`gps/msgfile` dependency at all. The desktop module (desktop-gui
branch) is a second future consumer: a separate Go module driving
the shared workbench layer through `gps/app/session`, today
loading message files only via a native file dialog; it already
imports `gps/msgfile`, so adopting the catalog needs no new
imports. The same goes for a possible later satpulsetool feature
resolving `-m u-blox/gen9` as a library name instead of a path.

## The zip and its generator

The zip is checked in so that the module is self-contained: plain
`go build`, `go install github.com/jclark/satpulse/cmd/...@latest`,
bare `go test`, and gopls all work from a checkout or module
download with no make step. Consequently the build paths (Makefile,
`unix-build.sh`, `bsd-build.sh`, `win-build.ps1`, and the desktop
Makefile on its branch) are untouched: generation is a developer
action, run only when the library under `configs/gpsmsg` changes.

The generator is a plain Go program with no options:

- Two required positional arguments: source directory and output
  zip file. Anything else exits with a usage message.
- Reads the list of files to pack from stdin, newline-separated,
  relative to the source directory; the listed paths double as the
  zip entry names (`vendor/file.toml`). Trim a trailing `\r` and
  skip blank lines, so PowerShell-piped input works.
- Deterministic output: entries sorted by name, timestamps zeroed,
  deflate at best compression. Regenerating from unchanged sources
  yields a byte-identical file, so staleness checks are a plain
  byte comparison and there is no spurious churn.

The file list always comes from `git ls-files`, never a directory
walk: untracked local TOMLs (test data) must not leak into the
binary. Regeneration is wired as a `//go:generate` line in the
package -- via `sh -c`, since go:generate has no shell:

    //go:generate sh -c "git -C ../../configs/gpsmsg ls-files -- '*/*.toml' | go run gen.go ../../configs/gpsmsg gpsmsg.zip"

so `go generate ./gps/msgfile` is the documented regeneration
step.

## Staleness guards

Checking in a generated artifact risks shipping a stale zip after a
library edit. The check is a small checked-in script that
regenerates to a temp file and byte-compares against the checked-in
zip -- a complete check (edits, additions, deletions, renames) made
possible by the deterministic generator. It runs as a dedicated
GitHub workflow (in the style of `node-tests.yml`): all-branch push
and pull-request triggers, path-filtered to `configs/gpsmsg/**`,
`gps/msgfile/gpsmsg.zip`, and `gps/msgfile/gen.go` -- the only
paths whose changes can create or fix staleness -- so a stale
commit gets a red check naming the problem on any branch. The
script also runs by hand for a local check before pushing.

A new `configs/gpsmsg/CLAUDE.md` states that any change to the
message files requires regenerating the embedded zip, giving the
`go generate ./gps/msgfile` command.

Day-to-day development of message files does not depend on
regeneration at all: pointing `SATPULSE_GPSMSG_PATH` at the
checkout's `configs/gpsmsg` puts the working-tree files ahead of
the built-ins.

## msgfile generalization to fs.FS

`gps/msgfile`'s library search currently works on `[]string` of os
directories. Generalize the search-path element to a `Dir`
interface:

    type Dir interface {
        fs.FS
        DisplayPath(name string) string
        Load(name string) (*Parsed, error)
    }

with an os-directory implementation and a built-in implementation
over the embedded zip. `fs.FS` covers cataloging (reading vendor
directories and their files); `DisplayPath` gives the user-visible
path for an entry; `Load` reads a file and walks its `[[include]]`
tree. Then:

- `EnvDirs`, `ListNames`, `FindName` work over `[]Dir`. Cataloging
  is naturally within a directory, so it runs over the `fs.FS`:
  `FindName` resolves to a `Dir` plus a slash-separated name rather
  than an os path.
- Loading differs by storage, because `[[include]]` resolution
  differs:
  - An os directory loads through the existing path-based `Load`:
    it resolves the entry to its native path and hands it to
    `Load`, so includes resolve natively and are **not** confined
    to the search directory -- `..` reaches wherever it points,
    exactly as `satpulsetool -m` does. An on-disk file behaves the
    same whether it is reached through the library search path or
    named directly. There is no library-root sandbox.
  - The embedded zip loads through a `LoadFS` variant that walks
    the `[[include]]` tree inside an `fs.FS` using the `path`
    package. Includes there resolve within the archive -- not as a
    policy but because nothing exists outside it. The shipped
    defaults use includes (`u-blox/gen9.toml` includes `ubx.toml`,
    `unicore/um982.toml` includes `um980.toml`, `zhongke/*` include
    `casic.toml`, ...), and these are all siblings, so they resolve
    the same either way.
  Unify the tree/merge/duplicate-include logic behind a small seam
  (open a file, resolve an include src relative to its base) so it
  exists once for the os and fs.FS cases.
- The path-based `Load` keeps its signature for satpulsetool, which
  takes explicit file paths and stdin and never touches library
  search. The os `Dir` reuses it, so a library file and a `-m` file
  resolve includes identically.
- `Entry.Path` and satpulsewb's `msgFileResult.Path` are
  user-visible, so built-in entries carry a display string from
  `DisplayPath` (e.g. `built-in:u-blox/gen9.toml`) instead of a
  filesystem path.

## Testing

- `fs.FS` variants in `msgfile` tested with `fstest.MapFS`,
  including an include chain and name shadowing across dirs.
- On-disk library loading resolves includes natively: an include
  using `..` to reach outside the search directory works, matching
  `satpulsetool`. The built-in zip resolves its includes within the
  archive.
- `cmd/satpulsewb`: adapt the `msgDirs` test (the `systemDirs` test
  goes away with the files), and a catalog test asserting built-in
  entries appear, plus env-dir shadowing of a built-in name.

## Documentation

- Rewrite the search-path material in `docs/man/satpulsewb.1.md`
  (ENVIRONMENT, currently lines 85-92): the library is built in;
  `SATPULSE_GPSMSG_PATH` directories are searched ahead of it; the
  user-library and installed-library paragraphs go. State that
  includes resolve relative to the file itself, not along the search
  path -- dropping the old "must have its included files alongside
  it" wording, which wrongly implied confinement (on-disk includes
  are unconfined, matching `satpulsetool`).
- `NEWS.md` entry: satpulsewb needs no installed message-file
  library, and no longer reads the user or system libraries unless
  `SATPULSE_GPSMSG_PATH` names them.

## Alternatives considered

- Raw `//go:embed */*.toml` of the sources: no compression, and
  go:embed cannot filter by git-tracked status, so untracked local
  files would leak into the binary.
- Generating the zip at build time, gitignored: eliminates the
  staleness problem and the checked-in blob, but every build path
  (including the desktop module's wails build) must generate first,
  and plain `go build` / `go install @latest` breaks on a clean
  checkout. Rejected to keep the module go-installable.
- Extracting embedded files to a temp directory at startup instead
  of teaching `msgfile` about `fs.FS`: no msgfile changes, but
  writes the library to disk on every run, has cleanup questions,
  and shows temp paths in the UI.
