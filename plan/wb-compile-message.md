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

New subpackage `gps/msgfile/gpsmsg` containing:

- `gpsmsg.zip` -- the default library (`configs/gpsmsg/*/*.toml`,
  about 173K of TOML, about 37K deflated), checked in.
- An embed file with `//go:embed gpsmsg.zip` and one exported
  function (e.g. `Builtin()`) returning the built-in library as a
  `msgfile.Dir` (see below). `archive/zip`'s `Reader` implements
  `fs.FS`, so the accessor is `zip.NewReader` over the embedded
  bytes (lazily, `sync.OnceValue`); files inflate per `Open`, with
  no extraction step.
- The generator (below) as a `//go:build ignore` file.
- A staleness test (below).

Why a subpackage rather than `gps/msgfile` itself: only importers
of the subpackage need the zip to compile, and only binaries that
call `Builtin` carry the bytes. satpulsetool depends on
`gps/msgfile` (via `internal/gpscmd`) but not on the subpackage, so
it is unaffected. (Verified separately: the linker's dead-code
elimination does drop an embed variable reachable only from an
uncalled function, but the subpackage makes that moot.) satpulsed
has no `gps/msgfile` dependency at all. The desktop module
(desktop-gui branch) is a second future consumer: it is a separate
Go module driving the shared workbench layer through
`gps/app/session`, today loading message files only via a native
file dialog; when it adopts the catalog it imports this subpackage
like satpulsewb does. A possible later satpulsetool feature --
`-m u-blox/gen9` resolving a library name instead of a path --
would import the subpackage from gpscmd at that point.

Add the subpackage to `docs/internals.md`.

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
subpackage -- via `sh -c`, since go:generate has no shell:

    //go:generate sh -c "git -C ../../../configs/gpsmsg ls-files -- '*/*.toml' | go run gen.go ../../../configs/gpsmsg gpsmsg.zip"

so `go generate ./gps/msgfile/gpsmsg` is the documented
regeneration step.

## Staleness guards

Checking in a generated artifact risks shipping a stale zip after a
library edit. Two guards, both byte comparisons thanks to the
deterministic generator:

- A unit test in the subpackage that re-derives the expected
  contents (enumerating tracked TOMLs with `git ls-files`, reading
  them from the working tree) and compares against the embedded zip
  entry-by-entry. A forgotten regeneration fails `make test` /
  `go test ./...`. The test skips when git is unavailable (module
  download, source tarball), where staleness is impossible anyway.
- A pre-commit hook for faster feedback: a small checked-in script
  that regenerates to a temp file and diffs against the checked-in
  zip. Hooks themselves cannot be checked in, so the script ships
  in the repo with a one-line setup note (`git config
  core.hooksPath` or a symlink into `.git/hooks`).

Day-to-day development of message files does not depend on
regeneration at all: pointing `SATPULSE_GPSMSG_PATH` at the
checkout's `configs/gpsmsg` puts the working-tree files ahead of
the built-ins.

## msgfile generalization to fs.FS

`gps/msgfile`'s library search currently works on `[]string` of os
directories. Generalize the search-path element to something like

    type Dir struct {
        FS      fs.FS
        Display string
    }

with a constructor wrapping an os directory via `os.DirFS(path)`
(Display = the path). Then:

- `EnvDirs`, `ListNames`, `FindName` work over `[]Dir`; `FindName`
  resolves to a dir plus slash-separated name rather than an os
  path.
- A `LoadFS` variant runs the `[[include]]` walk inside an `fs.FS`
  using the `path` package. This is required: the shipped defaults
  use includes (`u-blox/gen9.toml` includes `ubx.toml`,
  `unicore/um982.toml` includes `um980.toml`, `zhongke/*` include
  `casic.toml`, ...) and they must resolve within the embedded zip.
  Internally, unify the walk behind a small seam (open a file,
  resolve an include src relative to its base) so the
  tree/merge/duplicate-include logic in `msgfile.go` exists once
  for the os and fs.FS cases.
- The path-based `Load` keeps its signature for satpulsetool,
  which takes explicit file paths and stdin and never touches
  library search.
- `Entry.Path` and satpulsewb's `msgFileResult.Path` are
  user-visible, so built-in entries carry a display string derived
  from `Dir.Display` (e.g. `built-in:u-blox/gen9.toml`) instead of
  a filesystem path.

## Testing

- `fs.FS` variants in `msgfile` tested with `fstest.MapFS`,
  including an include chain and name shadowing across dirs.
- The staleness test above.
- `cmd/satpulsewb`: adapt the `msgDirs` test (the `systemDirs` test
  goes away with the files), and a catalog test asserting built-in
  entries appear, plus env-dir shadowing of a built-in name.

## Documentation

- Rewrite the search-path material in `docs/man/satpulsewb.1.md`
  (ENVIRONMENT, currently lines 85-92): the library is built in;
  `SATPULSE_GPSMSG_PATH` directories are searched ahead of it; the
  user-library and installed-library paragraphs go. Keep the note
  that includes resolve relative to the file itself, so a shadowing
  file needs its included files alongside it.
- `NEWS.md` entry: satpulsewb needs no installed message-file
  library, and no longer reads the user or system libraries unless
  `SATPULSE_GPSMSG_PATH` names them.
- `docs/internals.md` entry for the new subpackage.

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
- Embedding directly in `gps/msgfile` (no subpackage): one function
  in the right package, but makes the zip a compile prerequisite of
  most of the repo's build/test graph instead of just the GUI
  binaries.
