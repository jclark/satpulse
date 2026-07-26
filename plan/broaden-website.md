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
      - title: "Without a PHC"
        url: "/setup/without-phc.html"
      - title: "Setup chrony"
        url: "/setup/chrony.html"
      - title: "Monitoring"
        url: "/setup/monitor.html"
      - title: "Setup PHC synchronization"
        url: "/setup/phc.html"
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
        url: "/gps-config/fixed-position.html"
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

The Introduction, Hardware and Internals lists reflect what is now
deployed; the new pages still to come are GNSS HATs, the workbench
setup page, the GPS configuration section, and
the Internals section's AI usage page. The
other differences from the deployed navigation -- the Setup retitles
and reordering, the removal of the setup GPS configuration entry, and
the `satpulsewb(1)` man-page entry -- follow from stages 2-4; the
retirement of the RTK and precise-position howtos follows 0.3 final. The Setup
list reads baseline first, through Monitoring, then the
precision-timing pages, matching the stage 2 two-track index.
The workbench deliberately gets a Setup entry, not a top-level
section: it is one page, placed after Serial connection so that
device name, permissions, and baud rate are established before it.
Because it has no top-level presence, its discoverability rides on
the home page and `intro/satpulse.md`.

Hardware is a single "Hardware" section, not separate GNSS and timing
sections. An earlier rework split it that way, and the split was
undone because the GNSS receiver and antenna serve timing and
positioning alike, so splitting them off left the non-timing side thin
and put the timing-curated pages in the wrong group.

## How the work is staged

The work is divided into five stages in priority order. Each stage is
publishable on its own and leaves the site better than the stage
before it. Later stages are not prerequisites for publishing earlier
ones.

1. Correct the site against the published pre-release (done). This
   is what makes the site publishable now.
2. Align the tutorial path with the published pre-release, chiefly by
   moving RTK into setup and making setup work without a PHC.
3. Add SatPulse Workbench. Gated on a new pre-release that includes
   `satpulsewb`.
4. Rework that supports product use, chiefly the GPS configuration
   section.
5. Remaining work: content fills and new pages that nothing depends
   on.

Good enough beats best. The published site is updated whenever the
working state is better than what is live, not when a stage is
finished. The bar for publishing is that what is there is correct --
no wrong claims -- not that it is complete: visible TODOs on published
pages are acceptable, and a current site with recorded gaps is much
better than an out-of-date one. Stages order the work; they do not
gate publication.

Moved and superseded pages follow the conventions in
`docs/CLAUDE.md`: a moved page gets a `redirect_from` entry on the
page at its new URL in the same change, while a superseded page
sits out of the navigation with `sitemap: false` until its content
is confirmed covered by the pages that replace it, and gets its
redirect when it is finally deleted. `intro/gnss.md` is in the
superseded holding state now.

The home page is not reworked once. It evolves alongside the rest of
the site, and its claims must never outrun what the other pages can
back up. The first step, now done, put an honest, vision-led
transitional page in place. Each later stage earns one more home-page
claim: the accessible no-PHC timing path in stage 2, the workbench
paragraph in stage 3, the broader technical-resource claim in stage 5
once the hardware content is filled in.

### Which version the site describes

Stages 1 and 2 describe the published pre-release v0.3-pre-20260619,
not master. That tag has native Ntrip (the `[[ntrip.mountpoint]]`
caster, the `[stream.push]` server, and the `[stream.pull]` client),
`satpulsetool convobs` for RINEX conversion, NTP samples without a
PHC, and high-level configuration for u-blox and Unicore. It does not
have `satpulsewb`, which did not exist when the tag was cut, and it
does not have Septentrio or SPARTN support, although NEWS lists both
under 0.3. None of the three is documented before stage 3. Stages 3
and 4 describe the fresh pre-release that stage 3 cuts; stage 5 is
version-independent.

This applies to the tutorial pages. The man pages are different: they
are generated from master and track it, so they legitimately describe
things no download has yet, and the banner in `_layouts/man.html` says
so. Do not pull them back to the tag.

The current stable release is 0.2, so the tutorial docs need a
reusable inline label marking content that is newer -- in effect, "new
in 0.3" -- analogous to the pre-release banner the man pages already
carry (the `man_prerelease_notice` flag in `_config.yml`, rendered by
`_layouts/man.html`). The man-page banner is whole-page; the tutorial
label is per-feature, because a single tutorial page often mixes
stable and newer content. Stage 1 needs it. Implement the label as an
include gated on a `_config.yml` flag in the same way, so that when
0.3 final ships the labels go away with a single switch; removing the
markup from the pages can follow as a separate cleanup.

## Stage 1: correct the site against the published pre-release (done)

Done: the label mechanism is in place and the corrections below are
applied. The home page's sentence about the tutorials falling behind
was also softened to say the coverage is not yet complete, since the
wrong claims are now fixed.

Fix the claims that are wrong today, and add nothing else. The site is
publishable the moment this is done. The findings below were verified
against the 0.3 man pages, the Makefile, and the default config, and
re-checked against the v0.3-pre-20260619 tag (audit of 2026-07).

First, add the "new in 0.3" inline label described above: several of
the corrections below replace a false claim with behaviour that is in
the pre-release but not in stable 0.2, and needs marking as such.

Wrong -- word- or line-level fixes:

- `setup/gps-config.md`: "supports only the UBX protocol" -- false
  since 0.2; high-level configuration covers u-blox and Unicore. Also
  the flag bullet "`-s 38400` specifies the new speed": the new-speed
  flag is `--speed` (as the example command correctly uses); `-s` is
  the current host-port speed. Interim fix: the page is removed in
  stage 4, but it is wrong on the live site now.
- `howtos/precise-position.md`: "SatPulse currently only knows how to
  configure GNSS receivers that use u-blox's UBX protocol" -- same
  falsehood.
- `howtos/rtk.md`: "to use NTRIP on the rover with satpulsed, you
  would need a writeable TCP connection" -- false; `[stream.pull]` is
  a native Ntrip client.
- `setup/without-phc.md`: `ntp` is listed among sections with no
  effect without a PHC; since 0.2 (#77), `[ntp]` without `[phc]`
  sends samples timed from the serial messages, so it moves to the
  works list with the accuracy caveat and no version label. Also
  `proxy.sock` should be `proxy.socket`.
- `setup/satpulse-install.md`: satpulsetool installs to
  `/usr/local/bin`, not `/usr/local/sbin`.
- `setup/monitor.md`: the packet-log filename example
  `packet.ttyAMA.jsonl` should be `packet.ttyAMA0.jsonl`.

Misleading -- a few sentences each, same pass:

- `setup/index.md`: "use of SatPulse for a PTP/NTP time server
  requires a PHC" -- true only for PTP. One-line interim fix; the
  two-track rewrite is stage 2.
- `setup/satpulsed.md`: auto-configuration described as u-blox-only
  (omits Unicore, and u-blox support extends beyond generation 10);
  the minimal example config presents `[phc]` as part of the minimum.
- `setup/gps-serial.md`: the probe is described as UBX-only; it
  detects receivers across vendors and probes both high-level
  configuration protocols.
- `howtos/precise-position.md`: add `satpulsetool convobs` as the
  native RINEX conversion path, keeping the rtklib `convbin` recipe as
  an alternative. Interim: stage 4 makes convobs part of the
  fixed-position workflow under GPS configuration.
- `howtos/rtk.md`: keep the str2str recipes; reword the false rover
  sentence so it states str2str's needs; add a plain page-level
  notice that correction delivery is now native (caster, server,
  client, plain TCP) with a pointer to satpulse.toml(5), the version
  mentioned in prose without the inline label. The native
  capabilities get their real treatment in the stage 2 setup page.

Audited clean (no wrong claims): the home page, chrony, ptp4l, phc,
network, RPi pages, measure, ptp-windows, and the intro pages.

A second audit (2026-07) covering the hardware pages and the man
pages, which the first pass did not reach, found two more wrong
claims, now also fixed:

- `man/satpulse.toml.5.md` stated the automatic satellites-output
  rule backwards -- "only if the serial speed is less than 38400",
  where `time/app/daemon/gps.go` enables it only at 38400 or above,
  as the following sentence and both tutorial pages say.
- `hardware/gnss-modules.md` still described Unicore support as "in
  development"; high-level configuration for the UM980 series shipped
  in 0.2 (#139).

The rest of the hardware section makes only one SatPulse claim, the
holdover caveat in `hardware/gnssdos.md`, which is accurate while #152
is open.

The man-page banner in `_layouts/man.html` was reworded in the same
pass. It read "This man page is for the pre-release version of
SatPulse 0.3", which a reader takes as the dated pre-release linked
from the home page rather than current master; every build off master
reports `0.3-pre` (`Makefile`), so the man pages legitimately run
ahead of any download, and the banner now says so and points at the
installed man pages.

Two tier 2 model names looked like conflicts and were checked; both
are settled, and neither was a wrong claim. `intro/satpulse.md` said
SinoGNSS support was "validated on the K901" while every verification
comment in `configs/gpsmsg/sinognss/sinognss.toml` names a K902; both
modules were used, so the page now says "the K901 and K902", matching
NEWS. The ByNav M2 versus M20 naming also came up, and is genuinely
confusing, but the confusion is in the vendor's naming and in the
message file, not on the website: the site says M10 and M20
throughout, and the only occurrence of M2 is a shipped 0.2 NEWS
entry. Out of scope here.

## Stage 2: align the tutorial path with the published pre-release

The tutorial path still describes a narrower product than the
published pre-release is. Two gaps matter: RTK sits in Howtos as a
side topic rather than in the setup path, and setup implies that
niche PHC/PPS hardware is required. Nothing here depends on a new
pre-release.

### Add RTK to Setup (done)

Done: `setup/rtk.html` is written, device-independent and verified
against the pre-release, and sits at the end of the Setup nav until
the reordering below places it properly. The old howto stays in the
Howtos nav, its opening notice framing it as the 0.2 path and
pointing pre-release users at the new page. The two pieces of
follow-up work are recorded elsewhere: VRS support in stage 3, and
the howto's retirement, which follows 0.3 final, in the navigation
section above and in stage 3's note.

### Make setup no-PHC first

The current site makes SatPulse look as if it is only for users who
already have niche PHC/PPS hardware. That is a major adoption barrier.
Make no-PHC operation the baseline setup path, and make PHC
synchronization an optional precision-timing step.

Nothing blocks this: the default `satpulse.toml` ships with
`phc.interface` commented out, and its header says the only entry that
must be edited to get satpulsed started is the serial speed. The
no-PHC baseline works out of the box, and `[ntp]` without `[phc]` is
stable 0.2 behaviour, so the baseline track needs no version labels.

Rewrite `setup/index.md` as two tracks. Baseline (works for everyone):
OS setup as applicable, install SatPulse, serial connection and
satpulsetool contact check, configure and run `satpulsed`, feed chrony
without a PHC, monitor. Precision timing (PHC hardware): PHC/PPS
identification, PHC synchronization, chrony on the PHC, ptp4l. Stage 3
inserts the workbench into the baseline track.

The follow-through in the child pages:

- Make the without-PHC page read like a normal setup path rather than
  a fallback for users missing the "real" hardware. Source notes: the
  timing-without-a-PHC post and any existing no-PHC setup material.
  The page should state the accuracy class honestly and link to PHC
  synchronization as the upgrade path.
- Keep `setup/phc.md` focused on configuring SatPulse to discipline a
  PHC. Hardware requirements and buying guidance should live in the
  hardware section. chrony and ptp4l integration should remain in
  their own task pages.
- chrony should cover both the no-PHC baseline and the PHC-backed
  case. ptp4l should be clearly PHC-dependent. This keeps external
  software setup consistent with the new baseline instead of implying
  that all useful SatPulse setups require PTP hardware.

### Other stage 2 items

- `setup/satpulse-install.md`: add a macOS section covering the
  Homebrew tap (`jclark/homebrew-satpulse`), and update the trailing
  BSD sentence now that brew covers macOS. This is true of the
  published pre-release and does not wait on the workbench.
- `setup/satpulse-install.md`, same pass: the package table offers
  only `_amd64.deb` and `_arm64.deb`, but the release also ships an
  `armhf` .deb built for ARMv6 (#305), so a Pi Zero user has no way
  to find the right file. The installed-files list also omits the
  message files under `share/satpulse/gpsmsg` (#233).
- Apply the "new in 0.3" label where stage 1 did not reach:
  `intro/other-software.md` states the NTPsec SHM path (#300), which
  is in the pre-release but not in stable 0.2, without a label. The
  same is true of `--speed` on Unicore (#167) as used in
  `setup/gps-config.md`, but that page goes away in stage 4, so
  labelling it there is worth little.
- Home page: strengthen the timing door to offer the accessible
  no-PHC path up front, now that setup has a coherent one, and point
  the positioning material at the RTK setup workflow instead of the
  howto.

## Stage 3: SatPulse Workbench

Gated on cutting a fresh pre-release that includes `satpulsewb` in the
Linux packages (this landed after v0.3-pre-20260619 was cut; the
Homebrew tap covers macOS). The pre-release comes first so the
workbench page can lead with real install commands rather than "build
from source".

The workbench is currently invisible on the site: only NEWS and the
`satpulsewb(1)` man page mention it. This stage is almost entirely
additive. A launch blog post is promotion, not alignment, and follows
the stage rather than being part of it.

The fresh pre-release also unblocks one RTK item: Virtual Reference
Station support (`ntrip.nmeaSend` and `ntrip.nmeaSendInterval` on
`[stream.pull]`, and `--nmea-send-pos` on `satpulsetool ntrip`, #325)
landed after v0.3-pre-20260619, so the stage 2 RTK setup page
deliberately omits it. Add it to that page as soon as the pre-release
is cut. The other outstanding RTK item, retiring the howto with a
redirect to the setup page, is gated on 0.3 final, not on this
pre-release.

The steps, in order:

1. Cut the pre-release.

2. Write the workbench page (`setup/workbench.md`, nav "Using
   SatPulse Workbench", after "Serial connection"). One page: what it
   is, a screenshot tour, a quickstart (device and speed are known
   from the serial page -- the workbench discovers serial devices but
   does not autobaud, which is why establishing contact with
   satpulsetool comes first), and remote/headless use in brief (SSH
   tunnel, `--listen`, the access token), deferring detail to
   `satpulsewb(1)`. State vendor coverage honestly as a growing
   matrix: monitoring and packet decoding across all supported
   protocols; high-level configuration for u-blox and Unicore today,
   with CASIC, Septentrio, Allystar, and Quectel on pending branches;
   the UI greys out what a receiver does not support. Include one
   crisp sentence distinguishing the workbench from the satpulsed
   dashboard: the workbench is the interactive commissioning tool you
   run; the dashboard is observability for the deployed daemon.

3. Screenshots. A handful: sky view with signals, the config tab, the
   packet inspector, map/scatter. Manual captures from a live session
   are fine; repeatable capture tooling is out of scope. This is the
   site's first non-prose asset type; the home page thumbnail comes
   from the same set.

4. Add the workbench to the stage 2 baseline setup track, between the
   serial connection and running `satpulsed`.

5. Home page: one paragraph high up -- new in 0.3, SatPulse Workbench,
   browser-based cross-vendor receiver monitoring and configuration --
   with a thumbnail linking to the workbench page. The screenshot does
   more positioning work than any wording change.

6. `intro/satpulse.md`: add `satpulsewb` to the two-item component
   list, add a short paragraph in the receiver-configuration section
   (the same high-level model, interactively), and fill the "Platform
   support" TODO, which covers the workbench platforms and the
   Homebrew tap.

7. `setup/satpulse-install.md`: add `satpulsewb` to the
   installed-binaries lists.

8. `_data/navigation.yml`: add `satpulsewb(1)` to the man pages
   section (the page already renders; it is just unlisted), and the
   "Using SatPulse Workbench" entry from step 2.

## Stage 4: rework that supports product use

### Build the GPS configuration section

Receiver configuration is a core SatPulse capability, but it is
currently buried in the setup path. Remove the existing setup GPS
configuration page and build a top-level GPS configuration section as
in the outline (overview, high-level configuration, message files,
u-blox first, more vendors later).

By this stage there are three frontends to one configuration model
(the `satpulsed` config file, `satpulsetool gps`, and the workbench
config and messages tabs), and that is itself a selling point. Open
decision: write the section concepts-first, with each concept page
showing the workbench UI and the CLI as parallel means (workbench
screenshots as the default illustration, CLI kept for scripting and
headless use), versus keeping the section CLI-oriented with the
workbench page self-contained. Whichever way, state high-level vendor
coverage as a growing matrix rather than prose that hard-codes today's
vendor list.

The first coherent version should explain high-level configuration,
message files, volatile versus persistent changes, `satpulsed` versus
`satpulsetool gps`, and the current u-blox support. Source notes: the
old setup GPS configuration page, the GPS message files post, and the
u-blox material.

Make the message-file page depend on the high-level configuration
page, so users understand why low-level messages are sometimes needed
before seeing the syntax. Source notes: the GPS message files post and
implementation plans for message bundles, includes, response matching,
packet logs, and message-library workflows. Man pages should be linked
as reference, not copied into the guide.

Add vendor configuration pages only when they carry current tasks. Use
u-blox as the first vendor page because there is enough user-facing
material: supported generations, timing products, high-precision
products, L5, OSNMA notes if applicable to current support, persistent
versus volatile config, and links back to high-level tasks. Unicore
high-level configuration is in the pre-release, so a Unicore page
follows when there is enough tested, user-facing material; CASIC,
Septentrio, Allystar, and Quectel get pages as they land and
accumulate material. Protocol internals belong in implementation notes
unless they affect user choices.

### Make fixed position the organizing task for PPP

Place post-processed PPP under GPS configuration as part of the
fixed-position workflow. The user task is "configure the receiver with
a good fixed position"; post-processed PPP via `satpulsetool convobs`
is one way to obtain that position. The workflow is a new page,
`gps-config/fixed-position.md` (nav "Precisely determine position").
The older howto `howtos/precise-position.md` stays in the Howtos nav
alongside it until 0.3 final ships, then is retired under the
superseded-page convention.

### Cross-link the intro and hardware sections

The two sections under-link each other, both from concept to example
and from concept to how-to. The intended pattern:

- Intro concept pages link out to the hardware pages for real
  examples: the GNSS page to modules and receivers, the timing page to
  the CM4/CM5 and Intel builds and PTM, the positioning page to
  RTK-capable modules and receivers. No intro page links into the
  hardware section today.
- Hardware pages link back to the intro concept pages for the
  underlying concepts. A few of these exist already (`gnss-modules` ->
  `intro/gnss-basics`, `hardware/index` and `ptm` -> `intro/timing`);
  make the back-links systematic.
- For "how do I actually do it" links, link `intro/satpulse.md` to the
  relevant man pages, and the positioning page to the RTK setup
  workflow.

## Stage 5: remaining work

Content fills and new pages that nothing else depends on. Roughly in
priority order.

The intro and hardware sections share content boundaries that were
drawn once and should be respected by these fills: the PTM concept
(`intro/timing.md`) versus PTM hardware selection (`hardware/ptm.md`);
positioning concepts (`intro/positioning.md`) versus module and
receiver selection (`hardware/gnss-modules.md`); the timing mental
model versus the concrete builds and boards. Both sections are
version-independent, so nothing here is tied to a release.

- The precision timing page is done; what follows is constraints on
  editing it, not outstanding work. Keep the "synchronizing the system
  clock"/PTM concept section solid, because `hardware/ptm.md` links
  back to it for the concept while keeping concrete
  NIC/chipset/command selection on the hardware side. The page names
  no software as the thing that does a job, saying "the PTP daemon"
  and "an NTP daemon" throughout, so do not introduce chrony, ntpd-rs
  or ptp4l into it. Its TODO asked for exactly that and was dropped:
  both configurations are already described in role terms, the no-PHC
  one in the opening and the PHC one in "Synchronizing the system
  clock".
- Finish the GNSS modules beef-up. Sections exist for u-blox (with
  per-platform detail), Unicore, Allystar, and Quectel; the remaining
  vendors and the selection content (form factor, bands, timing
  features, price, where to buy) from the module-tour material are
  still to fold in. Route protocol internals (CASIC NAV/NAV2 classes,
  NovAtel-style packet formats, the ComNav configuration weaknesses)
  to the vendor configuration pages or internals, not the buyer-facing
  page.
- Resolve the constellation-comparison TODO in `intro/gnss-basics.md`.
- Delete the superseded `intro/gnss.md`. The coverage check has been
  done, and four things need placing first. Three of them are the
  TODOs above: the constellation comparison and the NMA/OSNMA/QZNMA
  material are what `gnss-basics.md`'s TODO asks for, and the
  vendor-protocol-versus-NMEA TAI argument is already sitting as
  unplaced draft in `intro/timing.md`. The fourth has no home yet:
  the band-combination list (L1, L1/L2, L1/L5, L1/L2/L5, L1/L2/L5/L6,
  and cheap Chinese dual-band modules tending to L1/L5), which
  `gnss-basics.md` explains as a concept without giving the set and
  `hardware/gnss-modules.md` uses without defining. Its other
  sections are covered: timing mode and quantization error in
  `intro/timing.md`, RTK and raw data and satellite-broadcast PPP in
  `intro/positioning.md`, protocol basics in `gnss-basics.md`.
  `intro.md` is already deleted: everything in it was superseded and
  much of it had become wrong, and its one unique sentence, the
  convention that the docs use "GPS" informally for any GNSS, is now
  in the terminology paragraph of `gnss-basics.md`.
- `setup/monitor.md` presents `clock` and `packet` as "the available
  log types"; `satpulse.toml(5)` also documents `event` and `track`.
  `log.track` (0.2) has no tutorial mention anywhere on the site.
- Add the GNSS HATs page (boards for the Raspberry Pi 40-pin header).
- Build the Internals section (done, apart from the AI usage page,
  which does not exist yet, so the section starts with two). The
  single `internals.md` page became `internals/packages.md` and the
  vendor bring-up guide `internals/vendor-support.md` is its peer.
  The rename was done on master, not on the website branch: it
  touches root `CLAUDE.md` and about nine files in `plan/`, and
  `internals.md` is edited by nearly every feature branch, so a
  rename parked on a long-lived branch collides with all of them.
  The page's trailing "Test harnesses" section was recast as a
  Python packages section, parallel to the Go and npm ones, and
  `smoketest/` documented there for the first time.
- Home page: add the broader "technical resource for GNSS hardware"
  claim once the hardware and module content backs it up.
