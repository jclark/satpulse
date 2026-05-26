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

## Outline

Top-level docs sections for the new site, and the leaf pages
under each. Two-level navigation throughout; every top-level is an
expandable section with pages at the leaf.

Home page
  - scope of site
  - key things that SatPulse can help you with (and hardware/software requirements for each,
    and how to navidate the site)
    - transfer of time from GNSS to a computer system (better than Pi with GPIO PPS)
    - precise GNSS positioning (better than just simple connection to L1-receiver)
    - GPS configuration and how satpulse can help

1. **Introduction**
   - SatPulse (overview of what SatPulse does, vision, value proposition, status)
   - GPS (what you need to know when choosing GPS/GNSS hardware and configuring)
   - Timing (how to do better than regular NTP; why PHC/PTM/switches etc are important)
   - Other software (ptp4l, chrony, rtklib, gpsd,...)

2. **GNSS hardware** (general GNSS hardware; everyone uses this
   regardless of whether they want timing or positioning)
   - GNSS modules
   - GNSS boards
   - GNSS receivers
   - GNSSDOs
   - Antennas
   - Vendors

3. **PHC/PTP hardware** (niche hardware needed only for PHC-based
   timing: PTP service, or chrony reading a PHC directly)
   - PHCs with SDPs
   - Precision Time Measurement (PTM)
   - PTP switches
   - Raspberry Pi CM4/CM5 PHC builds
   - Intel NIC PHC builds

4. **Setup** (gets `satpulsed` running on its own, not talking to
   any other program; pages can be skipped when they do not apply,
   e.g. non-Pi users skip the Pi-specific OS pages, users without a
   PHC skip "Synchronizing a PHC")
   - Overview (difference setup tracks)
   - Raspberry Pi OS
   - Fedora on Raspberry Pi
   - Raspberry Pi UARTs
   - Network configuration
   - Installing SatPulse
   - Serial connection to receiver
   - Configuring and running satpulsed
   - Synchronizing a PHC (bottom sections can include sync config and basic tuning with link to blog as example)
   - Monitoring
   - Establishing precise position with PPP

5. **Connections** (each page pairs a SatPulse configuration with
   configuration of an external program)
   - chrony
   - ptp4l
   - Ntrip and corrections (sufficient to setup RTK - both base and rover, including config)
   - Prometheus / Grafana
   - TCP proxy for external apps
   
6. **GPS configuration** (configuring the receiver itself; covers
   both timing and positioning settings; vendor pages added as they
   are written, u-blox first, eventually one per supported vendor)
   - Overview: different kinds of config
   - High-level configuration
   - Low-level configuration (includes AI-assisted message
     libraries)
   - u-blox (can include OSNMA section)
   - ... (more vendors added per release)

7. **Howtos** -- (non core tasks)
   - Measuring sync quality
   - Windows PTP client

8. **Man pages** (kept as the existing section)
   - satpulsetool(1)
   - satpulsetool-gps(1)
   - satpulsetool-sdp(1)
   - satpulsetool-syncsim(1)
   - satpulse.toml(5)
   - satpulsed(8)

9. **Internals**
    - Packages
    - AI use

## Initial automated content reshuffle

This phase makes the site look structurally close to the new outline
while preserving the fact that the site content was manually written.
AI moves existing content into the new structure under strict
provenance rules, but does not rewrite prose.

The main structural tool is `docs/_data/navigation.yml`: it presents
pages from existing directories under the new top-level
sections. The deliberate automated-phase path changes are the
`intro.md` split and the `setup/gps-config.md` move.

### Rules

- Content is moved, but body content must not be modified,
  summarized, paraphrased, corrected, deleted, or added.
- Provenance comments, title rewrite comments, and the audit note are
  allowed as metadata; they are not site prose.
- Every inserted automated-phase comment must start with
  `<! -- AI:`.
- The only prose-like change allowed is a page title change. Every
  rewritten title must have an adjacent comment saying it was
  rewritten, for example:
  `<! -- AI: title rewritten during automated reshuffle -->`. For
  Markdown headings, put the comment immediately before the heading.
  For YAML front matter titles, put the comment immediately after the
  front matter block, because Jekyll front matter must remain first
  in the file.
- Change navigation in `docs/_data/navigation.yml`.
- Create files and directories only for moved-content destinations,
  the required `docs/homeless.md` page, and the automated-phase audit
  note.
- No content is deleted. Move every source chunk not assigned
  below to `docs/homeless.md`.
- Every moved chunk must have a provenance comment before and after
  the chunk in its destination. The comments must identify the source
  file and the original section heading. For unheaded chunks, identify
  the original line range. For example:
  `<! -- AI: moved-from: docs/intro.md, section "Features" -->`
  and
  `<! -- AI: /moved-from: docs/intro.md, section "Features" -->`.
- At every removal site, leave a comment identifying the destination,
  for example:
  `<! -- AI: moved-to: docs/intro/satpulse.md, section "Features" -->`.
- When moving content out of a source page, preserve the relative
  order of chunks.
- Existing Markdown, Liquid tags, image references, links, tables,
  and code blocks must be preserved byte-for-byte inside moved
  chunks. Adjust surrounding blank lines only to keep Markdown valid.
- Produce a short audit note listing each source file touched, each
  destination file created or changed, each title rewrite, and any
  content placed in `docs/homeless.md`.
- After the automated phase, another AI must be able to review
  compliance mechanically by checking provenance comments, title
  rewrite comments, the audit note, and diffs showing that body text
  was moved unchanged.

### Content moves

- Keep `index.md` as the home page. Include it in the navigation
  audit and top navigation, but do not rewrite it during the
  automated phase.
- Remove `intro.md` as a top-level page and replace it with a
  top-level `intro/` directory.
- Split existing `intro.md` by section:
  - move the first two paragraphs and "Features" into
    `intro/satpulse.md` and change the title to "SatPulse"
  - move "Basics of how it works" into `intro/timing.md` and change
    the title to "Timing"; also move the full body of
    `_posts/2025-05-21-time-server-architecture.md`, the opening
    conceptual chunk of
    `_posts/2026-04-01-using-satpulse-for-timing-without-a-phc.md`
    through the paragraph ending "Prometheus metrics", and the
    `_posts/2026-04-06-building-an-ntp-server-on-a-raspberry-pi-with-chrony-or-ntpd-rs.md`
    section `## Hardware`
  - move "Relationship to other software" into
    `intro/other-software.md` and change the title to "Other
    software"; also move the gpsd-comparison post opening chunk from
    "In version 0.1 SatPulse focused" through the paragraph ending
    "`satpulsetool` does not need `satpulsed` to be running"
- Build `intro/gps.md` from the third paragraph of `intro.md` and
  the conceptual material in `hardware/gnss-modules.md` up to
  `## u-blox`, and change the title to "GPS".
- Make `hardware/gnss-modules.md` start with the
  vendor-specific module content at `## u-blox`.
- Keep all GNSS hardware pages under `hardware/`. In navigation, show
  only the GNSS-facing pages under "GNSS hardware".
- Retitle `hardware/index.md` to "GNSS and PTP hardware overview".
- Move the 2026-04-01 module-tour post sections `## Unicore` through
  `## Techtotop/Taidou` into `hardware/gnss-modules.md` unchanged,
  after the existing vendor sections.
- Move the 2026-04-01 module-tour post section
  `## Where SatPulse is heading` into `intro/satpulse.md`
  unchanged, after the old "Features" section.
- Move the tinyGTC "providing time" material into
  `hardware/gnssdos.md` unchanged.
- Keep PHC/PTP hardware pages under `hardware/`, but expose them in a
  separate "PHC/PTP hardware" nav section.
- Add `hardware/phc-sdps.md` from these unchanged chunks:
  `hardware/index.md` from the paragraph beginning "The key hardware
  requirement" through the two build bullets, `setup/phc.md` from the
  opening paragraph through the paragraph ending "there are four pins
  called SDP0, SDP1, SDP2 and SDP3", `hardware/clients.md` from the
  opening paragraph through the paragraph before `## Intel`,
  `hardware/intel-build.md` from the opening paragraph through the
  paragraph before `## PC system`, the tinyGTC post from the opening
  paragraph through the paragraph ending "In both cases, this works
  only with Linux", and the 2026-05-03 hardware test matrix post from
  the opening paragraph through the end of the hardware table.
- Keep `hardware/cm-build.md`, `hardware/intel-build.md`,
  `hardware/ptm.md`, `hardware/switches.md`, and
  `hardware/clients.md` under PHC/PTP hardware. Keep
  `hardware/clients.md` titled "PTP client hardware".
- Keep OS and platform setup pages where they are:
  `setup/rpi-os.md`, `setup/fedora-rpi.md`,
  `setup/rpi-uart.md`, `setup/network.md`,
  `setup/satpulse-install.md`, and `setup/gps-serial.md`.
- Retitle `setup/phc.md` to "Synchronizing a PHC".
- Retitle `setup/without-phc.md` to "Running satpulsed without a
  PHC". Move the basic configuration from the 2026-04-01 no-PHC post
  unchanged.
- Move `howtos/precise-position.md` into the Setup nav as
  "Establishing precise position with PPP". Leave the file path in
  `howtos/` for now.
- Keep `setup/monitor.md` as the setup-level monitoring page. Move
  the Prometheus/Grafana and TCP proxy sections unchanged into
  Connections pages.
- Keep `setup/chrony.md` as the "chrony" page. Move the
  NTP-without-PHC chrony setup from the 2026-04-06 post unchanged
  into it.
- Keep `setup/ptp4l.md` as the "ptp4l" page.
- Retitle `howtos/rtk.md` as "Ntrip and corrections" and expose it
  in the Connections nav. Leave the file path in `howtos/` until the
  manual cleanup phase.
- Add `setup/prometheus.md` from the `### Prometheus and Grafana`
  section of `setup/monitor.md` and the hardware test matrix
  dashboard paragraph and image at the end of the 2026-05-03 hardware
  test matrix post, using unchanged chunks.
- Add `setup/proxy.md` from the `## TCP proxy for Windows
  applications` section of `setup/monitor.md`, the
  `### Using manufacturer's Windows application` section of
  `gps-config/index.md`, and the packet-stream rationale beginning
  "However, SatPulse emphasizes a different approach" through the
  three numbered examples in the gpsd-comparison post, using
  unchanged chunks.
- Move `setup/gps-config.md` to `gps-config/index.md` so the section
  has a top-level directory and renders at `/gps-config/`.
- Retitle `gps-config/index.md` to "GPS configuration".
- Add `gps-config/high-level.md` from these unchanged chunks:
  `gps-config/index.md` from the opening paragraph through the
  paragraph ending "By default, it will enable all bands",
  `setup/satpulsed.md` section `## GPS configuration`, and
  `satpulsetool-gps(1)` section `## High-level configuration`
  through the paragraph before `## Low-level configuration`.
- Add `gps-config/message-files.md` from these unchanged chunks: the
  2026-01-29 configuration post section `## GPS message files`
  through the end of that post, and `satpulsetool-gps(1)` section
  `## Low-level configuration` through the paragraph before
  `## Packet capture`.
- Add `gps-config/ublox.md` as the first vendor page from the
  2026-04-01 module-tour post section `## u-blox` and the u-blox L5
  configuration paragraph from `gps-config/index.md`, using unchanged
  chunks.
- Keep `howtos/measure.md` as "Measuring sync quality", but move the
  tinyGTC measurement material unchanged into it.
- Keep `howtos/ptp-windows.md` as-is.
- Keep `internals.md` as "Packages".
- Move all remaining content that does not fit the new structure to
  `docs/homeless.md`.

### Navigation shape

Set `docs/_data/navigation.yml` to this structure:

```yaml
main:
  - title: "Home"
    url: "/"
  - title: "Blog"
    url: "/blog.html"
  - title: "GitHub"
    url: https://github.com/jclark/satpulse

docs:
  - title: "Introduction"
    children:
      - title: "SatPulse"
        url: "/intro/satpulse.html"
      - title: "GPS"
        url: "/intro/gps.html"
      - title: "Timing"
        url: "/intro/timing.html"
      - title: "Other software"
        url: "/intro/other-software.html"
  - title: "GNSS hardware"
    children:
      - title: "Overview"
        url: "/hardware/index.html"
      - title: "GNSS modules"
        url: "/hardware/gnss-modules.html"
      - title: "GNSS boards"
        url: "/hardware/gnss-boards.html"
      - title: "GNSS receivers"
        url: "/hardware/gnss-receivers.html"
      - title: "GNSSDOs"
        url: "/hardware/gnssdos.html"
      - title: "Antennas"
        url: "/hardware/antennas.html"
      - title: "Vendors"
        url: "/hardware/vendors.html"
  - title: "PHC/PTP hardware"
    children:
      - title: "PHCs with SDPs"
        url: "/hardware/phc-sdps.html"
      - title: "Precision Time Measurement"
        url: "/hardware/ptm.html"
      - title: "PTP switches"
        url: "/hardware/switches.html"
      - title: "Raspberry Pi CM4/CM5 builds"
        url: "/hardware/cm-build.html"
      - title: "Intel NIC builds"
        url: "/hardware/intel-build.html"
      - title: "PTP client hardware"
        url: "/hardware/clients.html"
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
      - title: "Synchronizing a PHC"
        url: "/setup/phc.html"
      - title: "Without a PHC"
        url: "/setup/without-phc.html"
      - title: "Monitoring"
        url: "/setup/monitor.html"
      - title: "Precise position with PPP"
        url: "/howtos/precise-position.html"
  - title: "Connections"
    children:
      - title: "chrony"
        url: "/setup/chrony.html"
      - title: "ptp4l"
        url: "/setup/ptp4l.html"
      - title: "Ntrip and corrections"
        url: "/howtos/rtk.html"
      - title: "Prometheus and Grafana"
        url: "/setup/prometheus.html"
      - title: "TCP proxy"
        url: "/setup/proxy.html"
  - title: "GPS configuration"
    children:
      - title: "Overview"
        url: "/gps-config/"
      - title: "High-level configuration"
        url: "/gps-config/high-level.html"
      - title: "Message files"
        url: "/gps-config/message-files.html"
      - title: "u-blox"
        url: "/gps-config/ublox.html"
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
        url: "/internals.html"
```

### Automated success criteria

- The automated rules have been followed and are reviewable by
  another AI.
- The new top-level navigation structure is in place.
- For the documentation tree covered by the sidebar,
  `docs/_data/navigation.yml` points to every ordinary non-post
  documentation page under `docs/` after the reshuffle, with the only
  allowed omitted page being `docs/homeless.md`. It does not point to
  missing pages. The home page is represented in the main navigation.
  The blog index, blog posts, assets, includes, layouts, config
  files, and README are outside this navigation audit.
- `docs/homeless.md` exists and contains every moved chunk that was
  not assigned to another destination.
- Every moved chunk has matching `<! -- AI:` provenance comments in
  its destination.
- Every removal site has a `<! -- AI:` destination comment.
- Every rewritten title has a `<! -- AI:` title rewrite comment.
- The audit note exists and lists source files, destination files,
  title rewrites, and homeless content.
- The site builds without missing navigation pages.

## Manual content cleanup

After the automated reshuffle, I will rewrite, smooth, shorten,
expand, and integrate the reshuffled content by hand. Cleanup should
be done in this priority order.

1. `index.md`: rewrite the home page around the broadened site
   scope. Priorities: explain the three main tracks--GNSS time
   transfer to computer systems, precise GNSS positioning, and GPS
   configuration/observability--and give clear entry points into the
   new navigation.
2. `intro/satpulse.md`: turn the first two old intro paragraphs and
   the old "Features" section into a coherent SatPulse overview.
   Priorities: current project scope, what `satpulsed` does, what
   `satpulsetool` does, current status, and the integrated-daemon
   idea. Preserve the old 0.1 feature list as the mature PHC/PTP
   timing subset of SatPulse, but reframe it so it no longer reads as
   the whole product. Integrate it with the broader 0.3 scope:
   receiver configuration, packet routing, RTCM/Ntrip correction
   streams, observability, and positioning support.
3. `intro/timing.md`: merge the old "Basics of how it works" section
   with the time-server architecture post, the no-PHC timing post,
   and the chrony/ntpd-rs hardware framing. Priorities: explain
   system clock vs PHC, PPS timestamping, ptp4l, chrony, PTP
   metadata, PHC-based PPS timestamping for tens-of-nanoseconds
   timing, GPIO/kernel PPS timestamping for microsecond-class NTP,
   and serial timing as time-of-day labeling rather than precise edge
   timing.
4. `intro/gps.md`: turn the third old intro paragraph and the
   conceptual front of `hardware/gnss-modules.md` into a GPS/GNSS
   concepts page. Priorities: terminology, constellations, bands,
   timing mode, sawtooth error, RTK, raw data, PPP, and navigation
   message authentication. Keep vendor-specific module detail out.
5. `intro/other-software.md`: turn the old relationship section into
   a broader related-software page. Priorities: explain ptp4l,
   chrony, ntpd-rs, gpsd, rtklib, and where SatPulse fits with each.
6. Move `howtos/rtk.md` to `setup/rtk.md` while reworking it as the
   main Ntrip and correction-stream page. Priorities: cover receiver
   RTCM output, SatPulse as a local Ntrip caster, mountpoint and
   source-table configuration, authentication, MSM7-to-MSM4
   conversion, SatPulse as an Ntrip/TCP correction client for a rover
   through `[stream.pull]`, rover verification, and when the TCP proxy
   or external RTKLIB tools still fit. Use
   `plan/archive/ntrip-caster.md`, `plan/archive/ntrip-client.md`,
   and `plan/archive/stream-pull-daemon.md` as source notes for the
   new functionality.
7. `gps-config/index.md`: make the section landing page coherent.
   Priorities: explain why receiver configuration matters, the
   difference between high-level configuration and message files, and
   where `satpulsed` configuration stops and `satpulsetool gps`
   starts.
8. `gps-config/high-level.md`: turn moved chunks into a task-oriented
   guide. Priorities: receiver detection, constellations, bands, PPS,
   time mode, fixed position, raw output, RTCM output, persistence,
   and safe use of non-volatile configuration.
9. `gps-config/message-files.md`: turn blog/man-page chunks into a
   durable low-level configuration guide. Priorities: message-file
   tags, response matching, binary/NMEA/line message types, includes,
   packet logs, and the AI-assisted message-library workflow.
10. `hardware/phc-sdps.md`: turn collected chunks into a concept and
   buying-guide page. Priorities: server PHC requirements, client PHC
   requirements, SDPs, external timestamping, periodic output, Intel
   vs Raspberry Pi CM4/CM5, and what hardware is not sufficient.
11. `setup/monitor.md`: keep this as the basic, low-effort
   monitoring page. Priorities: systemd logs, SatPulse log files, the
   built-in HTTP interface, and the web GUI. Link forward to
   `setup/prometheus.md` for higher-effort Prometheus/Grafana
   monitoring.
12. `setup/prometheus.md`: make the Prometheus/Grafana material a
   standalone connection page. Priorities: required SatPulse HTTP
   config, Prometheus scrape config, Grafana dashboard import, and
   what metrics are useful first. Link back to `setup/monitor.md` as
   the basic monitoring page for enabling the built-in HTTP interface
   and web GUI before adding Prometheus/Grafana.
13. `setup/proxy.md`: make the TCP/Unix socket proxy material a
    standalone connection page. Priorities: read-only vs read-write
    access, security warning, vendor Windows applications, Lady
    Heather, NMEA service use, and packet-stream rationale.
14. `gps-config/ublox.md`: make it the first vendor page. Priorities:
    supported generations, timing vs high-precision products,
    persistent vs volatile changes, L5/OSNMA notes, and links to
    high-level configuration tasks.
15. `docs/homeless.md`: triage all content placed here. Move each
    chunk into a real page during manual cleanup or decide explicitly
    that it should remain outside navigation for now.
