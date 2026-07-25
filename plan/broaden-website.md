# Broaden website positioning

## Positioning points

1. **Precision timing and positioning for computer systems with modern GNSS receivers.** This is the scope of the site: GNSS as part of computer systems, including computer clocks, network timing, receiver configuration, correction streams, packet flows, services, logs, metrics, HTTP APIs, and web monitoring. It is not about GNSS or precision timekeeping in the abstract.

2. **The site is a technical resource for using affordable GNSS hardware with computer systems.** It should help people understand how modern GNSS receivers, computer clocks, timestamping hardware, correction streams, operating-system APIs, and observability fit together. SatPulse software is a means to that end, not the whole subject of the site.

3. **`satpulsed` is an integrated GNSS daemon.** `satpulsed` coordinates receiver configuration, timing, RTCM/Ntrip correction streams, packet routing, packet proxying, packet logging, and observability through HTTP/web interfaces and Prometheus metrics from one TOML configuration file. Integrated means these are parts of one daemon and one configuration model, not separate services that the user has to assemble.

4. **`satpulsetool` is a command-line suite for setup, configuration, diagnostics, and experiments.** It is useful both with and without the daemon: configuring receivers, inspecting packet streams, capturing logs, decoding packets, testing hardware, and running simulation or measurement tools.

5. **SatPulse Workbench is a browser-based app for interactive receiver monitoring and configuration.** `satpulsewb` is a single self-contained binary run on the machine to which the receiver is attached -- possibly a headless Raspberry Pi -- and used from a web browser, which may be on a different machine. It is cross-vendor and cross-platform, in contrast to the vendor tools, which are single-vendor and Windows-only. It provides live monitoring (sky view, signal strengths, position, survey), device-independent configuration, message files, a cross-protocol packet inspector, and correction consumption over Ntrip or TCP, with any number of read-only windows and a single write seat. It has a good out-of-the-box experience: run the command and the browser opens on a live view. It is a hands-on commissioning tool, not a daemon: `satpulsed` is for unattended service, `satpulsewb` for interactive work.

6. **SatPulse has deep support for GNSS timing with PTP hardware clocks.** On Linux systems with suitable PHC/PPS hardware, `satpulsed` can discipline the PTP hardware clock from a GNSS receiver, provide clock-quality updates to `ptp4l` for PTP service, and provide refclock samples to NTP servers such as chrony and ntpd-rs. This is the most mature part of SatPulse.

7. **SatPulse works with positioning and authentication features implemented by the receiver.** For hardware RTK, the base receiver generates RTCM correction packets, and SatPulse routes those packets to TCP clients, local Ntrip clients, or upstream Ntrip casters; the rover receiver consumes RTCM packets and computes the RTK position solution. The same principle applies to features such as PPP-HAS and OSNMA: SatPulse focuses on configuring the receiver, moving the needed data, and exposing status and observability, rather than implementing the GNSS algorithms itself.

8. **Receiver configuration is a core part of SatPulse.** SatPulse combines high-level, device-independent configuration with low-level, device-dependent configuration using message files. The high-level model lets users configure receiver behavior in GNSS terms, such as timing mode, fixed position, time pulse, satellite systems and signals, navigation messages, raw observation messages, RTCM output, and reference station ID. Message files provide a structured way to send device-specific messages for receiver details that cannot be expressed in the device-independent model.

9. **SatPulse is designed for modern GNSS hardware and Unix-like computer systems.** Most receiver-facing functionality works on Unix-like systems; PHC/PTP hardware-clock timing is Linux-specific; `satpulsetool` and `satpulsewb` also work on Windows and macOS, and on macOS SatPulse installs from source via a Homebrew tap. On the receiver side, SatPulse supports standard NMEA, RTCM correction streams, and vendor-specific protocols for u-blox and eight other vendors.

## Navigation structure

This is the final navigation structure we eventually want to aim for.

docs:
  - title: "Introduction"
    children:
      - title: "Overview"
        url: "/intro/index.html"
      - title: "GNSS basics"
        url: "/intro/gnss-basics.html"
      - title: "Precision timing"
        url: "/intro/timing.html"
      - title: "Precision positioning"
        url: "/intro/positioning.html"
      - title: "SatPulse"
        url: "/intro/satpulse.html"
      - title: "Other software"
        url: "/intro/other-software.html"
  - title: "Hardware"
    children:
      - title: "Overview"
        url: "/hardware/index.html"
      - title: "GNSS modules"
        url: "/hardware/gnss-modules.html"
      - title: "GNSS boards for timing"
        url: "/hardware/gnss-boards.html"
      - title: "Enclosed GNSS receivers"
        url: "/hardware/gnss-enclosed.html"
      - title: "GNSSDOs"
        url: "/hardware/gnssdos.html"
      - title: "GNSS HATs"
        url: "/hardware/gnss-hats.html"
      - title: "Raspberry Pi CM4/CM5 builds"
        url: "/hardware/cm-build.html"
      - title: "Intel NIC builds"
        url: "/hardware/intel-build.html"
      - title: "PTP client hardware"
        url: "/hardware/clients.html"
      - title: "PTM hardware"
        url: "/hardware/ptm.html"
      - title: "PTP switches"
        url: "/hardware/switches.html"
      - title: "Antennas"
        url: "/hardware/antennas.html"
      - title: "Vendors"
        url: "/hardware/vendors.html"
  - title: "Setup"
    children:
      - title: "Overview"
        url: "/setup/index.html"
      - title: "Raspberry Pi OS"
        url: "/setup/rpi-os.html"
      - title: "Fedora on Raspberry Pi"
        url: "/setup/fedora-rpi.html"
      - title: "Raspberry Pi UARTs"
        url: "/setup/rpi-uart.html"
      - title: "Network configuration"
        url: "/setup/network.html"
      - title: "Installing SatPulse"
        url: "/setup/satpulse-install.html"
      - title: "Serial connection"
        url: "/setup/gps-serial.html"
      - title: "Using SatPulse Workbench"
        url: "/setup/workbench.html"
      - title: "Run satpulsed"
        url: "/setup/satpulsed.html"
      - title: "Setup PHC synchronization"
        url: "/setup/phc.html"
      - title: "Without a PHC"
        url: "/setup/without-phc.html"
      - title: "Monitoring"
        url: "/setup/monitor.html"
      - title: "Setup chrony"
        url: "/setup/chrony.html"
      - title: "Setup ptp4l"
        url: "/setup/ptp4l.html"
      - title: "Setup RTK"
        url: "/setup/rtk.html"
  - title: "GPS configuration"
    children:
      - title: "Overview"
        url: "/gps-config/index.html"
      - title: "High-level configuration"
        url: "/gps-config/high-level.html"
      - title: "Precisely determine position"
        url: "/howtos/precise-position.html"
      - title: "Message files"
        url: "/gps-config/msg-files.html"
      - title: "u-blox"
        url: "/gps-config/u-blox.html"
      - title: "Unicore"
        url: "/gps-config/unicore.html"
  - title: "Howtos"
    children:
      - title: "Measuring sync quality"
        url: "/howtos/measure.html"
      - title: "Windows PTP client"
        url: "/howtos/ptp-windows.html"
  - title: "Man pages"
    children:
      - title: "satpulsetool(1)"
        url: "/man/satpulsetool.1.html"
      - title: "satpulsetool-gps(1)"
        url: "/man/satpulsetool-gps.1.html"
      - title: "satpulsetool-pack(1)"
        url: "/man/satpulsetool-pack.1.html"
      - title: "satpulsetool-scan(1)"
        url: "/man/satpulsetool-scan.1.html"
      - title: "satpulsetool-sdp(1)"
        url: "/man/satpulsetool-sdp.1.html"
      - title: "satpulsetool-syncsim(1)"
        url: "/man/satpulsetool-syncsim.1.html"
      - title: "satpulsetool-convobs(1)"
        url: "/man/satpulsetool-convobs.1.html"
      - title: "satpulsewb(1)"
        url: "/man/satpulsewb.1.html"
      - title: "satpulse.toml(5)"
        url: "/man/satpulse.toml.5.html"
      - title: "satpulsed(8)"
        url: "/man/satpulsed.8.html"
  - title: "Internals"
    children:
      - title: "Packages"
        url: "/internals/packages.html"
      - title: "Supporting a new vendor"
        url: "/internals/vendor-support.html"
      - title: "AI usage"
        url: "/internals/ai-usage.html"

The Introduction and Hardware lists reflect what is now deployed; the
additions still to come are GNSS HATs, the workbench setup page, the
setup RTK workflow, the GPS configuration section, and the Internals
section (the current single `internals.md` page renamed to Packages,
with the vendor bring-up guide `vendor-support.md` as a peer).
The workbench deliberately gets a Setup entry, not a top-level
section: it is one page, placed after Serial connection so that
device name, permissions, and baud rate are established before it.
Because it has no top-level presence, its discoverability rides on
the home page and `intro/satpulse.md`.

## Incremental rework steps

Each step is an independent change that improves the site and moves it
toward the final structure. The steps are ordered: earlier steps
unblock later ones, and every step must leave the site usable for both
the published stable binary and a build from master.

Good enough beats best. The published site is updated whenever the
working state is better than what is live, not when a slice is
finished. The bar for publishing is that what is there is correct --
no wrong claims -- not that it is complete: visible TODOs on published
pages are acceptable, and a current site with recorded gaps is much
better than an out-of-date one. Slices order the work; they do not
gate publication. Priority follows from this: fix what is wrong
first, then fill what is missing, and among the missing, first what
prevents people from using the software effectively.

The home page is not reworked once. It evolves in stages alongside the
rest of the site, and its claims must never outrun what the other
pages can back up. The first step, now done, put an honest, vision-led
transitional page in place; later steps each earn an additional
home-page claim (the broader technical-resource claim after the intro
and hardware slice, the workbench paragraph in the workbench slice,
the accessible no-PHC timing path after the setup rework, and so on).

Release handling: the line that matters is what is done, which means
what is on master. A binary 0.3 pre-release is now published
(v0.3-pre-20260619, linked from the home page), and on macOS SatPulse
installs from source via a Homebrew tap. A fresh pre-release,
including `satpulsewb` in the Linux packages, precedes the workbench
slice below, so that its pages can lead with real install commands.
The docs describe everything on master, and version-sensitive pages
(RTK and corrections, configuration, the setup reframe) are written
once against that near-final 0.3 feature set rather than for the
stable binary and rewritten later.

The tutorial docs need a reusable inline label marking content that is
not in the current stable release (0.2) -- in effect, "new in 0.3" --
analogous to the pre-release banner the man pages already carry
(`man_prerelease_notice`). The man-page banner is whole-page; the
tutorial label is per-feature, because a single tutorial page often
mixes stable and newer content. The version-sensitive steps below
assume this label exists. When 0.3 final ships, the labels go away.

### Transitional home page (done)

Done: the 0.1-era home page has been replaced with an honest,
vision-led transitional page along the lines below. The broader
"technical resource for GNSS hardware" claim is still held back, and is
earned by the intro and hardware slice that follows. The notes below
are kept as the record of the intent.

Replace the 0.1-era home page with an honest, vision-led transitional
page now, before the deeper rework. This step does not wait on other
pages, because it routes readers to sources that are already accurate
rather than making claims the site cannot yet back.

Frame the out-of-sync state positively: the software has moved ahead
of the site because development has been fast, not because the site is
unreliable. State the vision as direction, not current state, drawing
on the "Where SatPulse is heading" section of the 2026-04-01
module-tour post and the scope and product positioning points: 0.1
was narrow (transfer time from a receiver to a PHC); a GPS subsystem
good enough for timing is most of a general one; far more people want
precision positioning than precision timing; so SatPulse is heading
toward positioning, with timing still core.

Do a light scope correction so the page no longer implies a PHC is
required: SatPulse is for precision timing and positioning, receiver
configuration, and observability. Route readers using the triad: the
narrative site documents the 0.1 baseline and is accurate for the
mature PTP/PHC timing path; the recent-changes page (NEWS) is the
authoritative record of everything since; the man pages are current
reference; the blog has depth. Offer two doors: a precise PTP/NTP time
server (the existing setup guide applies), and everything newer
(recent changes, man pages, blog), noting that it is easy to build
from source.

Claim anything that is done, which means anything on master: it builds
today as a `0.3-pre` version, so it is usable now even though no binary
pre-release has been published. That includes integrated, native
Ntrip, so present it as available today, marked as needing a build from
source (the published 0.2 binary does not have it yet). Precise
positioning, including RTK, is likewise usable today, either via the
existing howto (receiver RTCM output plus the TCP proxy and external
Ntrip tools) or via native Ntrip in a build from master. Do not claim
anything that is not yet on master. Hold back the broader "technical
resource for GNSS hardware" claim until the hardware split and module
beef-up land; add it then.

Two wording decisions are left to the author: how broad the opening
scope statement should be, and how bluntly to state that the narrative
site currently documents 0.1.

### Intro and hardware (mostly done)

The introduction and hardware sections are reworked together as one
slice, because they share content boundaries that have to be drawn
once: the PTM concept (`intro/timing.md`) versus PTM hardware selection
(`hardware/ptm.md`); positioning concepts (`intro/positioning.md`)
versus module and receiver selection (`hardware/gnss-modules.md`); the
timing mental model (`intro/timing.md`) versus the concrete builds and
boards. Both sections are version-independent -- the content is the
same for the stable release and any pre-release -- so they are written
once now, regardless of the 0.3 release boundary. Completing this slice
earns the broader "technical resource for GNSS hardware" claim on the
home page.

#### What is done

The structural rework is in place, every page has content, and most
of the originally listed remaining work has since been completed.

- Navigation is a single "Hardware" section: overview, GNSS modules,
  boards, enclosed receivers, GNSSDOs, CM4/CM5 build, Intel build,
  PTP client hardware, PTM, switches, antennas, and vendors.
  An earlier rework split this into separate "GNSS hardware" and
  "Precision time hardware" sections; that split was undone because the
  GNSS receiver and antenna serve timing and positioning alike, so
  splitting them off left the non-timing side thin and put the
  timing-curated pages in the wrong group.
- `hardware/index.md` is the reworked combined overview: retitled to
  "Hardware", organized as general-purpose hardware (receiver, antenna)
  plus the extra hardware timing needs, with the three receiver forms
  (board, enclosed, GNSSDO) laid out.
- `hardware/gnss-boards.md` is retitled "GNSS boards and cards for
  timing" (nav label "GNSS boards for timing").
- The enclosed-receiver page is `hardware/gnss-enclosed.md`, titled
  "Enclosed GNSS receivers".
- The vendors page is a single combined page (`hardware/vendors.md`)
  covering GNSS and timing hardware vendors alike.
- The introduction is split into six nav pages: overview, GNSS basics,
  precision timing, precision positioning, SatPulse, and other
  software. (`intro/gnss.md` still exists but is orphaned -- not in the
  nav and not linked from any page; fold it into `gnss-basics.md` or
  remove it.)
- `intro/positioning.md` is written: raw observations, PPP, RTK, and
  carrier phase.
- `intro/other-software.md` is filled: every entry has content,
  including the vendor-tools entry, which notes that the vendor tools
  are all Windows-only (a useful foil for the workbench).
- `intro/satpulse.md` is essentially complete: it covers
  `satpulsed`/`satpulsetool`, timing with and without a PHC, positioning
  (RTK/Ntrip, new in 0.3), receiver configuration, observability,
  packet processing, and the tier 1/tier 2 protocol breakdown. It does
  not yet mention the workbench; that is part of the workbench slice.
- The howto "Precisely determining antenna position"
  (`howtos/precise-position.md`) exists, covering timing mode and
  survey-in.

#### Cross-linking the two sections

The sections currently under-link each other, both from concept to
example and from concept to how-to. The intended pattern:

- Intro concept pages link out to the hardware pages for real examples:
  the GNSS page to modules and receivers, the timing page to the CM4/CM5
  and Intel builds and PTM, the positioning page to RTK-capable modules
  and receivers.
- Hardware pages link back to the intro concept pages for the
  underlying concepts. A few of these exist already (`gnss-modules` ->
  `intro/gnss`, `ptm` -> `intro/timing`); make the back-links
  systematic.
- For "how do I actually do it" links, start by linking
  `intro/satpulse.md` to the relevant man pages, which are the current
  reference. The positioning page can point at the existing RTK howto
  until the RTK setup workflow exists.

#### Remaining work

The key things still to do, roughly in priority order.

- Finish the precision timing page. The marked TODO section remains:
  bring the NTP/chrony role into the "synchronizing the system clock"
  discussion (how time reaches chrony/ntpd-rs both with a PHC and
  without one), and fold in or place the draft material at the end of
  the page (TAI/UTC/leap seconds and NMEA versus vendor protocols for
  PTP; GLONASS as the worse fit). Keep the "synchronizing the system
  clock"/PTM concept section solid, because `hardware/ptm.md` links
  back to it for the concept while keeping concrete
  NIC/chipset/command selection on the hardware side.
- Finish the GNSS modules beef-up. Sections exist for u-blox (with
  per-platform detail), Unicore, Allystar, and Quectel; the remaining
  vendors and the selection content (form factor, bands, timing
  features, price, where to buy) from the module-tour material are
  still to fold in. Route protocol internals (CASIC NAV/NAV2 classes,
  NovAtel-style packet formats, the ComNav configuration weaknesses)
  to the vendor configuration pages or internals, not the buyer-facing
  page.
- Resolve the constellation-comparison TODO in `intro/gnss-basics.md`
  and the orphaned `intro/gnss.md`.
- Fill the "Platform support" TODO in `intro/satpulse.md`, and add the
  man-page how-links described above. (The platform-support fill is
  folded into the workbench slice, since the workbench and the
  Homebrew tap change what it should say.)

Out of scope for this slice: the all-new GNSS HATs page (boards for the
Raspberry Pi 40-pin header) stays a later step, and the setup,
configuration, and RTK reworks below come after.

### Corrections pass (do first)

Fix the claims that are wrong today. This pass is independent of the
pre-release and is publishable on its own the moment it is done. The
findings below were verified against the 0.3 man pages, the Makefile,
and the default config on master (audit of 2026-07).

Wrong -- word- or line-level fixes:

- `setup/gps-config.md`: "supports only the UBX protocol" -- false
  since 0.2; high-level configuration covers u-blox and Unicore. Also
  the flag bullet "`-s 38400` specifies the new speed": the new-speed
  flag is `--speed` (as the example command correctly uses); `-s` is
  the current host-port speed.
- `howtos/precise-position.md`: "SatPulse currently only knows how to
  configure GNSS receivers that use u-blox's UBX protocol" -- same
  falsehood.
- `howtos/rtk.md`: "to use NTRIP on the rover with satpulsed, you
  would need a writeable TCP connection" -- false on master;
  `[stream.pull]` is a native Ntrip client.
- `setup/without-phc.md`: `ntp` is listed among sections with no
  effect without a PHC; on 0.3, `[ntp]` without `[phc]` sends samples
  timed from the serial messages, so it moves to the works list with
  the accuracy caveat. Also `proxy.sock` should be `proxy.socket`.
- `setup/satpulse-install.md`: satpulsetool installs to
  `/usr/local/bin`, not `/usr/local/sbin`.
- `setup/monitor.md`: the packet-log filename example
  `packet.ttyAMA.jsonl` should be `packet.ttyAMA0.jsonl`.

Misleading -- a few sentences each, same pass:

- `setup/index.md`: "use of SatPulse for a PTP/NTP time server
  requires a PHC" -- true only for PTP. (The full two-track rewrite
  happens in the workbench slice; this is the one-line interim fix.)
- `setup/satpulsed.md`: auto-configuration described as u-blox-only
  (omits Unicore, and u-blox support extends beyond generation 10);
  the minimal example config presents `[phc]` as part of the minimum.
- `setup/gps-serial.md`: the probe is described as UBX-only; it
  detects receivers across vendors and probes both high-level
  configuration protocols.
- `howtos/precise-position.md`: add `satpulsetool convobs` as the
  native RINEX conversion path, keeping the rtklib `convbin` recipe as
  an alternative.
- `howtos/rtk.md`: rewrite the "Using NTRIP" section around the
  native capabilities (`[[ntrip.mountpoint]]` caster, `[[stream.push]]`
  server, `[stream.pull]` client), keeping the rtklib recipe as an
  alternative. This is the interim fix; the full RTK rework below
  still comes later.

Audited clean (no wrong claims): the home page, chrony, ptp4l, phc,
network, RPi pages, measure, ptp-windows, and the intro pages.

### Workbench and 0.3 alignment (next slice)

The shortest path to a site aligned with 0.3, whose headline addition
is SatPulse Workbench. The workbench is currently invisible on the
site: only NEWS and the `satpulsewb(1)` man page mention it. This
slice is almost entirely additive; the deeper setup, configuration,
and RTK reworks below stay later steps. A launch blog post is
promotion, not alignment, and follows the slice rather than being part
of it.

The steps, in order:

1. Cut a fresh 0.3 pre-release. `satpulsewb` is now included in the
   Linux packages (this landed after v0.3-pre-20260619 was cut), and
   the Homebrew tap covers macOS. The pre-release comes first so the
   workbench page can lead with real install commands rather than
   "build from source".

2. Rewrite `setup/index.md` as two tracks. Baseline (works for
   everyone): OS setup as applicable, install SatPulse, serial
   connection and satpulsetool contact check, try the workbench,
   configure and run `satpulsed`, feed chrony without a PHC, monitor.
   Precision timing (PHC hardware): PHC/PPS identification, PHC
   synchronization, chrony on the PHC, ptp4l. The child pages stay
   untouched in this slice; the index re-sequences and reframes them.
   This pulls the "make setup no-PHC first" step of the setup
   sequencing section forward; the rest of that section remains later
   follow-through.

3. Write the workbench page (`setup/workbench.md`, nav "Using
   SatPulse Workbench", after "Serial connection"). One page: what it is, a
   screenshot tour, a quickstart (device and speed are known from the
   serial page -- the workbench discovers serial devices but does not
   autobaud, which is why establishing contact with satpulsetool comes
   first), and remote/headless use in brief (SSH tunnel, `--listen`,
   the access token), deferring detail to `satpulsewb(1)`. State
   vendor coverage honestly as a growing matrix: monitoring and packet
   decoding across all supported protocols; high-level configuration
   for u-blox and Unicore today, with CASIC, Septentrio, Allystar, and
   Quectel on pending branches; the UI greys out what a receiver does
   not support. Include one crisp sentence distinguishing the
   workbench from the satpulsed dashboard: the workbench is the
   interactive commissioning tool you run; the dashboard is
   observability for the deployed daemon.

4. Screenshots. A handful: sky view with signals, the config tab, the
   packet inspector, map/scatter. Manual captures from a live session
   are fine; repeatable capture tooling is out of scope for this
   slice. This is the site's first non-prose asset type; the home page
   thumbnail comes from the same set.

5. Home page: one paragraph high up -- new in 0.3, SatPulse Workbench,
   browser-based cross-vendor receiver monitoring and configuration --
   with a thumbnail linking to the workbench page. The screenshot does
   more positioning work than any wording change.

6. `intro/satpulse.md`: add `satpulsewb` to the two-item component
   list, add a short paragraph in the receiver-configuration section
   (the same high-level model, interactively), and fill the "Platform
   support" TODO, which now covers the workbench platforms and the
   Homebrew tap.

7. `setup/satpulse-install.md`: add a macOS section (Homebrew tap),
   add `satpulsewb` to the installed-binaries lists, and update the
   trailing BSD sentence now that brew covers macOS.

8. `_data/navigation.yml`: add `satpulsewb(1)` to the man pages
   section (the page already renders; it is just unlisted), and the
   "Using SatPulse Workbench" entry from step 3.

### Setup sequencing

The current site makes SatPulse look as if it is only for users who
already have niche PHC/PPS hardware. That is a major adoption barrier.
Make no-PHC operation the baseline setup path, and make PHC
synchronization an optional precision-timing step. Once setup has a
coherent no-PHC path, the home page's timing door can be strengthened
to offer the accessible no-PHC path up front (a later home-page
stage).

The release-sequencing problem previously noted here is resolved: the
default `satpulse.toml` on master ships with `phc.interface` commented
out, and its header says the only entry that must be edited to get
satpulsed started is the serial speed. The no-PHC baseline works out
of the box, so nothing blocks making it the documented default path.

#### Make setup no-PHC first

Done as part of the workbench slice above, which rewrites
`setup/index.md` into a baseline track and a precision-timing track.
The subsections below are the follow-through in the child pages.

#### Make without-PHC a normal setup step

Make the without-PHC page read like a normal setup path rather than a
fallback for users missing the "real" hardware. Source notes: the
timing-without-a-PHC post and any existing no-PHC setup material.
The page should state the accuracy class honestly and link to PHC
synchronization as the upgrade path.

#### Keep PHC synchronization optional and explicit

Keep `setup/phc.md` focused on configuring SatPulse to discipline a
PHC. Hardware requirements and buying guidance should live in the
precision time hardware section. chrony and ptp4l integration should
remain in their own task pages.

#### Align chrony and ptp4l with the split setup model

chrony should cover both the no-PHC baseline and the PHC-backed
case. ptp4l should be clearly PHC-dependent. This keeps external
software setup consistent with the new baseline instead of implying
that all useful SatPulse setups require PTP hardware.

### Configuration

Receiver configuration is a core SatPulse capability, but it is
currently buried in the setup path. Remove the existing setup GPS
configuration page and build a top-level GPS configuration section as
in the outline (overview, high-level configuration, message files,
u-blox first, more vendors later). Build it around current-release
functionality first, and mark next-release features inline.

The workbench changes the shape of this section: there are now three
frontends to one configuration model (the `satpulsed` config file,
`satpulsetool gps`, and the workbench config and messages tabs), and
that is itself a selling point. Open decision: write the section
concepts-first, with each concept page showing the workbench UI and
the CLI as parallel means (workbench screenshots as the default
illustration, CLI kept for scripting and headless use), versus keeping
the section CLI-oriented with the workbench page self-contained.
Whichever way, state high-level vendor coverage as a growing matrix
rather than prose that hard-codes today's vendor list.

#### Make current GPS configuration discoverable

The first coherent version should explain high-level configuration,
message files, volatile versus persistent changes, `satpulsed` versus
`satpulsetool gps`, and the current u-blox support. Source notes:
the old setup GPS configuration page, the GPS message files post, and
the u-blox material.

#### Add message files after the high-level model is clear

Make the message-file page depend on the high-level configuration
page, so users understand why low-level messages are sometimes needed
before seeing the syntax. Source notes: the GPS message files post
and implementation plans for message bundles, includes, response
matching, packet logs, and message-library workflows. Man pages
should be linked as reference, not copied into the guide.

#### Add vendor configuration pages only when they carry current tasks

Use u-blox as the first vendor page because there is enough
user-facing material: supported generations, timing products,
high-precision products, L5, OSNMA notes if applicable to current
support, persistent versus volatile config, and links back to
high-level tasks. Unicore high-level configuration is on master, so a
Unicore page follows when there is enough tested, user-facing
material; CASIC, Septentrio, Allystar, and Quectel are on pending
branches and get pages as they land and accumulate material. Protocol
internals belong in implementation notes unless they affect user
choices.

#### Make fixed position the organizing task for PPP

Place post-processed PPP under GPS configuration as part of the
fixed-position workflow. The user task is "configure the receiver with
a good fixed position"; post-processed PPP via `satpulsetool convobs`
is one way to obtain that position. It is a next-release feature, so
mark it inline and keep the older fixed-position howto available
alongside it until the next release, after which the older one goes
away.

### Recast RTK as a setup workflow

Move and update the RTK material. It should cover receiver
RTCM output, SatPulse as a local Ntrip caster, upstream Ntrip use,
rover correction input, mountpoints, source tables, authentication,
and verification. Source notes: `howtos/rtk.md`,
`plan/archive/ntrip-caster.md`, `plan/archive/ntrip-client.md`, and
`plan/archive/stream-pull-daemon.md`.

The rewritten setup workflow should distinguish base and rover
requirements. A base needs a known antenna position and the ability to
emit appropriate RTCM correction messages; a rover needs to accept
corrections and compute an RTK position solution. Do not imply that
RTCM output alone makes a receiver fully RTK-capable: typically a
rover-capable receiver can also act as a base, but the reverse is not
guaranteed.
