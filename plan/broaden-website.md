# Broaden website positioning

## Positioning points

1. **Precision timing and positioning for computer systems with modern GNSS receivers.** This is the scope of the site: GNSS as part of computer systems, including computer clocks, network timing, receiver configuration, correction streams, packet flows, services, logs, metrics, HTTP APIs, and web monitoring. It is not about GNSS or precision timekeeping in the abstract.

2. **The site is a technical resource for using affordable GNSS hardware with computer systems.** It should help people understand how modern GNSS receivers, computer clocks, timestamping hardware, correction streams, operating-system APIs, and observability fit together. SatPulse software is a means to that end, not the whole subject of the site.

3. **`satpulsed` is an integrated GNSS daemon.** `satpulsed` coordinates receiver configuration, timing, RTCM/Ntrip correction streams, packet routing, packet proxying, packet logging, and observability through HTTP/web interfaces and Prometheus metrics from one TOML configuration file. Integrated means these are parts of one daemon and one configuration model, not separate services that the user has to assemble.

4. **`satpulsetool` is a command-line suite for setup, configuration, diagnostics, and experiments.** It is useful both with and without the daemon: configuring receivers, inspecting packet streams, capturing logs, decoding packets, testing hardware, and running simulation or measurement tools.

5. **SatPulse has deep support for GNSS timing with PTP hardware clocks.** On Linux systems with suitable PHC/PPS hardware, `satpulsed` can discipline the PTP hardware clock from a GNSS receiver, provide clock-quality updates to `ptp4l` for PTP service, and provide refclock samples to NTP servers such as chrony and ntpd-rs. This is the most mature part of SatPulse.

6. **SatPulse works with positioning and authentication features implemented by the receiver.** For hardware RTK, the base receiver generates RTCM correction packets, and SatPulse routes those packets to TCP clients, local Ntrip clients, or upstream Ntrip casters; the rover receiver consumes RTCM packets and computes the RTK position solution. The same principle applies to features such as PPP-HAS and OSNMA: SatPulse focuses on configuring the receiver, moving the needed data, and exposing status and observability, rather than implementing the GNSS algorithms itself.

7. **Receiver configuration is a core part of SatPulse.** SatPulse combines high-level, device-independent configuration with low-level, device-dependent configuration using message files. The high-level model lets users configure receiver behavior in GNSS terms, such as timing mode, fixed position, time pulse, satellite systems and signals, navigation messages, raw observation messages, RTCM output, and reference station ID. Message files provide a structured way to send device-specific messages for receiver details that cannot be expressed in the device-independent model.

8. **SatPulse is designed for modern GNSS hardware and Unix-like computer systems.** Most receiver-facing functionality works on Unix-like systems; PHC/PTP hardware-clock timing is Linux-specific; `satpulsetool` also works on Windows. On the receiver side, SatPulse supports standard NMEA, RTCM correction streams, and vendor-specific protocols for u-blox and seven other vendors.

## Navigation structure

This is the final navigation structure we eventually want to aim for.

docs:
  - title: "Introduction"
    children:
      - title: "Overview"
        url: "/intro/index.html"
      - title: "GNSS"
        url: "/intro/gnss.html"
      - title: "Precision timing"
        url: "/intro/timing.html"
      - title: "Positioning"
        url: "/intro/positioning.html"
      - title: "SatPulse features"
        url: "/intro/satpulse-features.html"
      - title: "Other software"
        url: "/intro/other-software.html"
  - title: "GNSS hardware"
    children:
      - title: "Overview"
        url: "/hardware/gnss.html"
      - title: "GNSS modules"
        url: "/hardware/gnss-modules.html"
      - title: "GNSS HATs"
        url: "/hardware/gnss-hats.html"
      - title: "GNSS receivers"
        url: "/hardware/gnss-receivers.html"
      - title: "GNSSDOs"
        url: "/hardware/gnssdos.html"
      - title: "Antennas"
        url: "/hardware/antennas.html"
      - title: "Vendors"
        url: "/hardware/vendors.html"
  - title: "Precision time hardware"
    children:
      - title: "Overview"
        url: "/hardware/index.html"
      - title: "Selected GNSS boards"
        url: "/hardware/gnss-boards.html"
      - title: "Raspberry Pi CM4/CM5 builds"
        url: "/hardware/cm-build.html"
      - title: "Intel NIC builds"
        url: "/hardware/intel-build.html"
      - title: "PTP client hardware"
        url: "/hardware/clients.html"
      - title: "Precision Time Measurement"
        url: "/hardware/ptm.html"
      - title: "PTP switches"
        url: "/hardware/switches.html"
      - title: "Vendors"
        url: "/hardware/timing-vendors.html"
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
      - title: "Configuring fixed position"
        url: "/howtos/fixed-pos.html"
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
      - title: "satpulsetool-sdp(1)"
        url: "/man/satpulsetool-sdp.1.html"
      - title: "satpulsetool-syncsim(1)"
        url: "/man/satpulsetool-syncsim.1.html"
      - title: "satpulse.toml(5)"
        url: "/man/satpulse.toml.5.html"
      - title: "satpulsed(8)"
        url: "/man/satpulsed.8.html"
  - title: "Internals"
    children:
      - title: "Packages"
        url: "/internal/packages.html"
      - title: "AI usage"
        url: "/internals/ai-usage.html"

## Incremental rework steps

Each step is an independent change that improves the site and moves it
toward the final structure. The steps are ordered: earlier steps
unblock later ones, and every step must leave the site usable for both
the published stable binary and a build from master.

The home page is not reworked once. It evolves in stages alongside the
rest of the site, and its claims must never outrun what the other
pages can back up. The first step, now done, put an honest, vision-led
transitional page in place; later steps each earn an additional
home-page claim (the broader technical-resource claim after the intro
and hardware slice, the accessible no-PHC timing path after the setup
rework, and so on).

Release handling: the line that matters is what is done, not what has
a published binary. Everything on the master branch is usable today --
it builds as a `0.3-pre` version that anyone can clone and run, even
though no binary pre-release has been published. The docs describe
everything on master, and version-sensitive pages (RTK and
corrections, configuration, the setup reframe) are written once against
that near-final feature set rather than for the stable binary and
rewritten later. Mark inline the features that are on master but not in
the published stable binary (0.2), so binary-only users know they need
to build from source (or wait for a 0.3 binary). Building is easy, so
cutting a binary 0.3 pre-release is a convenience and a labelled
testing target, not what makes these features usable.

The tutorial docs need a reusable inline label marking content that is
not in the current stable release (0.2), analogous to the pre-release
banner the man pages already carry (`man_prerelease_notice`). The
man-page banner is whole-page; the tutorial label is per-feature,
because a single tutorial page often mixes stable and newer content.
The version-sensitive steps below assume this label exists.

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
module-tour post and positioning points 1-6: 0.1 was narrow (transfer
time from a receiver to a PHC); a GPS subsystem good enough for timing
is most of a general one; far more people want precision positioning
than precision timing; so SatPulse is heading toward positioning, with
timing still core.

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

### Intro and hardware (current slice)

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

The structural rework is in place and every page has content.

- Navigation is split into two top-level sections: "GNSS hardware"
  (overview, modules, receivers, GNSSDOs, antennas, vendors) and
  "Precision time hardware" (overview, selected GNSS boards, CM4/CM5
  build, Intel build, client hardware, PTM, switches, timing vendors).
- The old `hardware/index.md` is now the precision-time-hardware
  overview, and a new `hardware/gnss.md` is the GNSS-hardware overview.
- `hardware/gnss-boards.md` has moved to the precision-time-hardware
  section, with the nav label "Selected GNSS boards".
- The vendors page is split: GNSS vendors stay on `hardware/vendors.md`,
  and timing-only vendors are in `hardware/timing-vendors.md`.
- The introduction is split into six pages: overview, GNSS, precision
  timing, precision positioning, SatPulse, and other software.
- `intro/satpulse.md` is essentially complete: it covers
  `satpulsed`/`satpulsetool`, timing with and without a PHC, positioning
  (RTK/Ntrip, new in 0.3), receiver configuration, observability,
  packet processing, and the tier 1/tier 2 protocol breakdown.
- `intro/gnss.md`, `intro/timing.md`, and `hardware/gnss-modules.md`
  carry substantial content.

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

- Write `intro/positioning.md` (currently a stub). Cover base versus
  rover, the need for a known base-station position, fixed versus float,
  ambiguity resolution, RTK versus PPP, local OSR versus global SSR
  corrections, and hardware RTK versus software positioning. Harvest the
  conceptual parts of the current RTK howto (especially the base/rover
  model and fixed-position extraction); leave command sequences and
  SatPulse-specific setup in the howto/setup workflow, and point the
  "how" at the existing RTK howto for now.
- Fill `intro/other-software.md` (many stub entries). Complete the empty
  entries -- statime, chrony, ntpd-rs, NTP/NTPSec, RTKLib, BKG Ntrip
  Caster, BNC, SNIP, PyGPSClient, Lady Heather, Geoclue -- describing
  each tool and how it works with or is an alternative to SatPulse. This
  is stub completion, not a later reframing: it does not depend on the
  SatPulse features page, which is already written.
- Finish the precision timing page. Make the PHC mental model central
  and complete: system clock versus PHC, hardware packet timestamping,
  external PPS timestamping, PHC-capable NICs, PTP switches, PTM,
  chrony/NTP, and the precision classes users can realistically expect.
  SatPulse appears as one way to take advantage of the hardware, not the
  reason it works. Keep the "synchronizing the system clock"/PTM concept
  section solid, because `hardware/ptm.md` links back to it for the
  concept while keeping concrete NIC/chipset/command selection on the
  hardware side.
- Finish the GNSS modules beef-up. Fold the per-vendor selection content
  (form factor, bands, timing features, price, where to buy) from the
  module-tour material into the page, keeping the site's voice but
  dropping phrasing tied to that specific post. Route protocol internals
  (CASIC NAV/NAV2 classes, NovAtel-style packet formats, the ComNav
  configuration weaknesses) to the vendor configuration pages or
  internals, not the buyer-facing page.
- Retitle `hardware/gnss-boards.md`. The title and intro should signal
  that these are boards curated for an in-case PPS timing build (CM4/CM5
  and Intel-NIC machines), not a general catalogue. It is still titled
  "GNSS boards and cards".
- Clean up the overviews. Flesh out `hardware/timing-vendors.md` and
  fix the "THis" typo. Leave the first-person and time-relative voice in
  the overviews as is -- it is the site's own voice, not blog cruft to
  strip.
- Fill the "Platform support" TODO in `intro/satpulse.md`, and add the
  man-page how-links described above.

Out of scope for this slice: the all-new GNSS HATs page (boards for the
Raspberry Pi 40-pin header) stays a later step, and the setup,
configuration, and RTK reworks below come after.

### Setup sequencing

The current site makes SatPulse look as if it is only for users who
already have niche PHC/PPS hardware. That is a major adoption barrier.
Make no-PHC operation the baseline setup path, and make PHC
synchronization an optional precision-timing step. Once setup has a
coherent no-PHC path, the home page's timing door can be strengthened
to offer the accessible no-PHC path up front (a later home-page
stage).

Release-sequencing problem: the current default `satpulse.toml` sets a
non-empty `phc.interface`, so a user without PHC hardware has to edit
that out before the no-PHC path works. That makes a no-PHC-first setup
flow awkward before the 0.3 release boundary, because the docs would be
presenting a baseline path that starts with undoing part of the
installed default configuration. This problem is noted here without
choosing the exact staging or documentation resolution yet.

#### Make setup no-PHC first

Change the setup overview so the baseline path is install SatPulse,
connect the receiver, run `satpulsed`, monitor it, and optionally use
chrony without a PHC. Present PHC synchronization as an additional
step for users with suitable hardware, not as the entry requirement.

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
high-level tasks. Add Unicore later when there is enough tested,
user-facing configuration material. Protocol internals belong in
implementation notes unless they affect user choices.

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
