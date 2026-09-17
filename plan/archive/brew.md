# Homebrew packaging for macOS (#322)

## Motivation

macOS has no packaged install for `satpulsed`/`satpulsetool`: the only option
today is to clone the repo and build by hand. Provide a Homebrew tap so macOS
users can install and update both with a single command, and run the daemon
under launchd via `brew services`, giving macOS the same convenience the
`.deb`/`.rpm` assets give Linux users.

This plan covers the initial, deliberately minimal increment: a tap that builds
from source, starts/stops the daemon with `brew services`, and uses the macOS
find-serial tool from `plan/macos-find-serial.md` to resolve the
daemon's serial device at service start. It has no bottles and no release
automation. It is a clean step up from "clone and build by hand".

Scope notes:

- macOS is a secondary platform. Nothing here changes Linux behaviour, the main
  module's dependencies, or the daemon. The macOS find-serial logic
  lives in a small separate C tool, not in `satpulsed`/`satpulsetool`.
- The serial-device naming problem on macOS (USB `/dev/cu.*` paths change with
  port/hub) is the most urgent real blocker. This plan depends on
  `plan/macos-find-serial.md` and wires that generic discovery tool into the
  launchd service command.
- Baud-rate discovery (#326) is also out of scope and separate.

## Homebrew background

For readers who do not use Homebrew, the facts this plan relies on:

- **Homebrew is the de facto macOS package manager.** It installs everything
  under a single **prefix**: `/opt/homebrew` on Apple Silicon, `/usr/local` on
  Intel. `$(brew --prefix)` reports it; formulae never hardcode it but use path
  helpers (`bin`, `sbin`, `etc`, `var`, `man1`, `pkgshare`) that resolve under
  it.
- **The prefix is owned by the installing user**, not root. Homebrew `chown`s it
  to you at install time, which is why `brew` (and daemons it starts) run
  without `sudo`. This is the key difference from Linux: paths that need root
  there (`/etc`, `/var/log`) are **user-writable** here because they live under
  the user-owned prefix (`<prefix>/etc`, `<prefix>/var/log`). That is what lets
  a non-root daemon read its config and write its logs with no privilege
  juggling and no code changes -- the macOS analogue of the Linux system paths.
- **A formula** is a Ruby recipe describing how to fetch, build, and install one
  package. A **tap** is a git repo of formulae (named `homebrew-<x>`); once
  tapped, `brew install <user>/<x>/<formula>` works and `brew upgrade` tracks
  new versions.
- **Build from source vs bottle.** By default a source-only formula compiles on
  the user's machine (needs the toolchain). A **bottle** is a prebuilt per-arch
  binary archive Homebrew downloads and "pours" instead of compiling. This plan
  is source-only; bottles are Future work.
- **Files install into the prefix.** A formula stages into its **keg**
  (`<prefix>/Cellar/<formula>/<version>/...`) and Homebrew symlinks that into the
  prefix (`<prefix>/bin`, `<prefix>/share/man`, ...). Config installed to
  `<prefix>/etc` is treated specially: an existing file is **not overwritten** on
  upgrade, so user edits survive.
- **`brew services` runs the daemon under launchd.** It reads the formula's
  `service` block and generates a launchd job. Run as a normal user (the common
  case), it writes a **LaunchAgent** to `~/Library/LaunchAgents/`, which runs as
  that user with no root. macOS requires user agents to live there -- launchd
  will not load one from the prefix -- so the plist is the one piece outside the
  prefix. (`sudo brew services ...` instead writes a system **LaunchDaemon** to
  `/Library/LaunchDaemons` running as root; not what we use.)

The resulting layout for a normal Apple Silicon install:

| File | Location |
|---|---|
| `satpulsed` | `/opt/homebrew/sbin/satpulsed` |
| `satpulsetool` | `/opt/homebrew/bin/satpulsetool` |
| find-serial tool | `/opt/homebrew/bin/find-serial` |
| config | `/opt/homebrew/etc/satpulse.toml` |
| man pages | `/opt/homebrew/share/man/man{1,5,8}/...` |
| gpsmsg tree | `/opt/homebrew/share/satpulse/gpsmsg/...` |
| logs | `/opt/homebrew/var/log/satpulse/...` |
| launchd plist | `~/Library/LaunchAgents/homebrew.mxcl.satpulse.plist` |

Everything but the plist is under the user-owned prefix; on Intel substitute
`/usr/local`. This single-user, user-owned-prefix model is why the Linux
non-root concerns (root-only `/var/log`, privilege drop) simply do not arise.

## Key decisions

These were settled during design; record them so the increment is unambiguous.

- **Separate tap repo**, not a formula in the satpulse repo. The tap is
  `github.com/jclark/homebrew-satpulse`, installed as `jclark/satpulse`. Users
  run `brew install jclark/satpulse/satpulse`. A formula carried in the source
  repo would need a local-tap symlink dance and fights `brew services`
  resolution; a real tap is the with-the-grain model.
- **Two formulae for now**:
  - `satpulse` -- the main formula. **Head-only initially** (a `head` line, no
    `stable` block), so `brew install --HEAD jclark/satpulse/satpulse` builds
    the tip of `master`. It gains a real `stable` block at the v0.3 release.
  - `satpulse@pre` -- the prerelease channel. A normal `stable` formula pinned
    to a chosen commit.
  - These give the three maturity tiers: `--HEAD` (master) < `@pre` < `satpulse`
    stable (once v0.3 ships). Specific past releases, if ever wanted, become
    additional `satpulse@X.Y.rb` files; not built now.
- **Git download strategy on every channel**, never a release tarball.
  `unix-build.sh` derives the embedded version from git (`git log`,
  `git describe`), so the build directory must be a real clone with `.git`. A
  GitHub `archive/.../X.tar.gz` has no `.git` and would break it. Homebrew's git
  strategy clones and checks out a revision with `.git` intact, so
  `unix-build.sh` runs unmodified.
  - Consequence: **no `sha256` to maintain** -- the commit `revision` is the
    integrity check. Per channel:
    - `head "https://github.com/jclark/satpulse.git", branch: "master"`
    - `url "https://github.com/jclark/satpulse.git", revision: "<full sha>"`
- **`@pre` is decoupled from the Linux prerelease tags.** It pins a bare
  `revision` (any commit on master we choose), with an explicit, monotonically
  increasing `version` so `brew upgrade` fires. Reuse the Linux date convention
  for the version string, minus the leading `v`: `version "0.3-pre-20260619"`.
  Homebrew parses `pre` as a prerelease token and orders by the trailing date.
  Re-pointing `@pre` is a two-line edit: new `revision`, bumped `version`.
- **Build from source, no bottles.** Omit the `bottle` block; every install
  compiles (needs the Go toolchain, declared `depends_on "go" => :build`).
  Bottles (prebuilt per-arch binaries that `brew` pours) are Future work.
- **No formula-update automation.** `revision`/`version` bumps are manual.
- **Paths live under the Homebrew prefix**, written by the shipped macOS config,
  not by hardcoded `/var/log` etc. A `brew services` daemon for a normal user is
  a launchd **LaunchAgent** running as that user, so `/var/log`, `/var/run`,
  `/etc` are not writable. The macOS `satpulse.toml` we install sets `log.dir`
  (and any socket paths) under the prefix/`var`. The prefix is `/opt/homebrew`
  on Apple Silicon and `/usr/local` on Intel; the formula's path helpers
  (`bin`, `sbin`, `etc`, `var`, `man1`, `pkgshare`) resolve these, so nothing is
  hardcoded.
- **The service resolves the serial device at launch.** The launchd job runs the
  macOS find-serial tool in `--exec` mode, with `satpulsed` as the
  child command. The service passes `-d {}` to `satpulsed`; the discovery tool
  replaces `{}` with the current `/dev/cu.*` path and `execvp`s the daemon.
  Default matching is any USB serial callout device. Installations with multiple
  USB serial devices can add `--vid`/`--pid` match options to the discovery-tool
  arguments.

## Formula shape

The two formulae share `def install` and a `service` block; the only difference
is the download spec. Sketch (illustrative, not final):

```ruby
class Satpulse < Formula
  desc "Integrated GPS timing daemon and configuration tool"
  homepage "https://github.com/jclark/satpulse"
  head "https://github.com/jclark/satpulse.git", branch: "master"
  # stable block added at the v0.3 release:
  # url "https://github.com/jclark/satpulse.git", tag: "v0.3", revision: "<sha>"

  depends_on "go" => :build

  def install
    system "./unix-build.sh"
    goos = "darwin"
    goarch = Hardware::CPU.arm? ? "arm64" : "amd64"
    out = "out/#{goos}_#{goarch}"
    sbin.install "#{out}/satpulsed"
    bin.install  "#{out}/satpulsetool"
    # find-serial is a standalone Darwin C tool with its own Makefile, built
    # separately from the Go binaries (unix-build.sh does not build it).
    system "make", "-C", "macos"
    bin.install "macos/find-serial"
    # man pages, configs/gpsmsg tree, config-schema.json, macOS satpulse.toml
    etc.install "configs/satpulse-macos.toml" => "satpulse.toml" unless (etc/"satpulse.toml").exist?
  end

  service do
    run [opt_bin/"find-serial", "--exec", "--",
         opt_sbin/"satpulsed", "-f", etc/"satpulse.toml", "-d", "{}"]
    keep_alive true
    log_path     var/"log/satpulse/launchd.out.log"
    error_log_path var/"log/satpulse/launchd.err.log"
  end

  test do
    assert_match "satpulse", shell_output("#{bin}/satpulsetool --version")
  end
end
```

`satpulse@pre.rb` (class `SatpulseATPre`) is the same body with:

```ruby
  version "0.3-pre-20260619"
  url "https://github.com/jclark/satpulse.git", revision: "<full sha>"
```

Man pages are installed, for parity with the deb/rpm. They are generated from
`docs/man/*.md` by `pandoc`; `pandoc` is a Homebrew formula and `=> :build`, so
it is a one-time build-time download (bottled) that never becomes a runtime
dependency. The wrinkle is that **`unix-build.sh` does not generate man pages --
only the Makefile does** (`out/%: docs/man/%.md` -> `pandoc ... -t man`), so the
formula must generate them itself rather than copy from `out/`. `def install`
therefore:

1. `depends_on "pandoc" => :build`.
2. Runs pandoc over the nine pages in `docs/man/` (`satpulsetool.1`, the six
   `satpulsetool-*.1`, `satpulse.toml.5`, `satpulsed.8`), with the same metadata
   flags the Makefile uses.
3. Applies the two path substitutions the Makefile does, but to the brew prefix:
   `satpulsetool-gps.1` (gpsmsg dir `/usr/share/satpulse/gpsmsg` ->
   `<prefix>/share/satpulse/gpsmsg`) and `satpulsed.8` (config path
   `/etc/satpulse.toml` -> `<prefix>/etc/satpulse.toml`).
4. `man1.install` / `man5.install` / `man8.install` the results.

Alternatively, teach `unix-build.sh` to generate the pages when pandoc is
present so the formula installs from `out/`; the prefix substitutions still have
to happen in `def install` either way.

## macOS config

Ship `configs/satpulse-macos.toml` (already exists) as the default
`<prefix>/etc/satpulse.toml`, adjusted so a non-root LaunchAgent works:

- `log.dir` under `<prefix>/var/log/satpulse`.
- `phc.interface = ""` (no PHC hardware on macOS).
- any Unix socket paths under the prefix or `/tmp`, not `/var/run`.
- no default `serial.device`; the Homebrew service supplies the current device
  path with `-d {}` through the find-serial tool. Users running
  `satpulsed` manually can still pass `-d` or set `serial.device` themselves.

Do not overwrite an existing `<prefix>/etc/satpulse.toml` on upgrade.

## CI on the tap repo (Tier 1)

Add a PR workflow to `homebrew-satpulse` that proves the formulae build and
install from source on a clean macOS runner, across both architectures:

```yaml
name: brew test
on: pull_request
jobs:
  test:
    strategy:
      matrix:
        os: [macos-14, macos-13]   # arm64 + intel
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: Homebrew/actions/setup-homebrew@master
      - run: brew tap jclark/satpulse "$PWD"
      - run: brew install --HEAD --build-from-source satpulse
      - run: brew install --build-from-source satpulse@pre
      - run: satpulsetool --version
      - run: find-serial
```

Notes:

- Drive the install explicitly rather than relying on `brew test-bot
  --only-formulae`, which is oriented around stable formulae and bottling and
  does not fit a head-only formula cleanly. `brew test-bot --only-tap-syntax`
  can be added for the lint/style pass.
- The smoke test must stay hardware-free (no serial device on a runner):
  `satpulsetool --version`, `satpulsed --help`, and the find-serial
  tool in listing mode only.

## Path to the v0.3 stable

When v0.3 is cut:

- Add the `stable` block to `satpulse.rb` (`url ...git, tag: "v0.3", revision:
  "<sha>"`), keeping the `head` line. Plain `brew install
  jclark/satpulse/satpulse` then works; `--HEAD` still builds master.
- By then the find-serial tool and the macOS config defaults should
  be in master, so stable and head agree and the shared `def install`/`service`
  needs no `build.head?` conditionals.

## Future work

- **Tier 2 CI: launchd system smoke.** A macOS sibling of `smoketest/system.py`
  (`system_macos.py`) that installs via the formula, starts the service through
  `brew services`/`launchctl`, replays a packet log through a FIFO, runs the
  existing `run_checks` scenario assertions, and stops cleanly. Mostly redundant
  with the `smoketest/run.py` pass already in `build-macos.yml` (which exercises
  the same binaries and checks at the binary level), so this is a nice-to-have,
  not essential. It requires: a launchd lifecycle rewrite (`system.py` is
  systemd-specific -- `systemctl`/`journalctl`/`udevadm`/`systemd-escape` have
  no macOS analogue); the tap CI checking out the satpulse repo for the harness
  and testdata; and verifying `brew services` starts a LaunchAgent in a headless
  GitHub runner session (the one genuine unknown -- fall back to `launchctl
  bootstrap gui/$UID` or a direct binary launch if it is flaky).
- **Bottles.** Prebuilt per-arch binaries poured by `brew`, for parity with the
  `.deb`/`.rpm` downloads and no toolchain on the user's machine. Adds a
  `bottle` block plus build-and-host work (e.g. GitHub Packages) and CI to build
  them on tag.
- **Release automation.** Auto-bump `revision`/`version` (and later bottle
  hashes) in the tap on each release/prerelease, e.g. a release script or
  `brew bump-formula-pr`.
- **`@`-versioned releases.** `satpulse@X.Y.rb` files to keep specific past
  releases installable, once there is more than one release worth pinning.
