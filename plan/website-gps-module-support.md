# GPS module support section (#453)

## Problem

Which GPS modules SatPulse supports, and how well, has no single home on the site. The vendor list for high-level configuration is stated in several places (the protocol-support section of `intro/satpulse.md`, `gps-config/high-level.md`, `setup/satpulsed.md`, `setup/rtk.md`), and the copies already disagree: the intro list has three vendors, the others two. Meanwhile the per-vendor knowledge that exists in the repository and the test fleet (tested models and firmware, working timing configurations, receiver behaviour) is not on the site at all.

The reader's question is "is my receiver supported, and how well". Support attaches to the module inside a product and the protocol family its firmware speaks, not to the finished receiver: an enclosed receiver built on a UM980 is supported because the UM980 is. So the answer is organized per vendor, at module level.

High-level configuration support has three states: supported (u-blox, Unicore), experimental (CASIC, enabled only with an explicit vendor), and not supported (everything else, where message files do all configuration). Support is not one scalar, though; other dimensions include `satpulsetool convobs` raw-observation conversion, which native messages are decoded, and which of those are translated into the device-independent model.

## The section

A new top-level nav section, "GPS module support", after "GPS configuration". The title names the relationship (how SatPulse supports the modules), not the objects: a section called just "GPS receivers" or "GPS modules" would blur into the hardware section's GNSS modules page, since module and receiver are two words for the same thing.

The entry page is "Supported vendors": an index, not an overview. It lists the vendors, with aliases where the vendor is known under more than one name (SinoGNSS/COMNav, Techtotop/Taidou, Zhongke/CASIC), and points to the per-vendor pages. A tick/cross support matrix summarizing the dimensions may be added eventually; initially it is just the list. The uniform shape of the vendor pages does the explaining; the index does not define terms up front.

One page per vendor, covering every supported vendor from the start: u-blox, Unicore, Zhongke/CASIC, Allystar, Techtotop/Taidou, Septentrio, ByNav, SinoGNSS/COMNav, Quectel, Airoha. A page does not have to be long to be useful; a few lines of verified key facts answers the reader's whole question, and having a page for every supported vendor is itself the impressive fact.

The section is built breadth first, in slices: each slice adds a small increment to every vendor page, then the next slice adds another, so the pages grow gradually and stay uniform in depth. The first slice might be just what `intro/satpulse.md` currently says about each vendor. u-blox will eventually be the deepest page, but it is not the template; the simple key-facts shape is.

## Vendor page shape

The standard shape, settled on the Unicore page (the model for the others), filled in slice by slice:

- Opening paragraph: SatPulse supports GPS modules from this vendor, with a link to the vendor's site, and the supported modules qualified. The qualification says what is likely to work, not just what was tested: for u-blox a very wide range, for Unicore the NebulasIV family. It also excludes vendor ranges that use a completely different protocol (e.g. Unicore's standard precision range and the UT986).
- Protocol paragraph: the character of the vendor protocol. Binary or ASCII, whether periodic data and configuration are split, relationships to other protocols (NovAtel-like, UBX-like). Rearranged from existing wording where possible.
- A "For these modules, SatPulse:" bulleted list keyed to the supporting-a-protocol dimensions: decodes the packet formats, converts periodic data into the device-independent data model, supports high-level configuration (or not), supports low-level configuration (message files provided, plus the response pattern or protocol-specific message types that go with them), converts raw observations into RINEX. The tier 1/tier 2 concept is dropped: the dimension bullets state the extent of support directly.
- Modules tested: the specific modules SatPulse is known to work on, from the packet testdata corpus and the systest fleet. No firmware versions.
- A supported-messages table, titled in the vendor's own terminology ("Supported logs" for Unicore, "Supported messages" for u-blox): one row per native message with a real decoder (messages recognized by name only get no row). Columns: name (without any ASCII/binary suffix variant), message number, what SatPulse uses it for, and whether high-level configuration automatically enables it ("Automatically enabled"). Uses are comma-separated and use device-independent vocabulary in the style of the `--pvt-out`/`--sats-out`/`--raw-out` options and the Workbench configuration form: UTC time, TAI time, time pulse, leap second, geodetic position, geodetic velocity, ECEF position, ECEF velocity, satellites, satellite usage, solution quality, raw observations, receiver identification, decode only. "time pulse" means the decoder emits a TimeMsg with Ref PrePulse or PostPulse; that high-level configuration answers a timePulse request by enabling the log does not qualify (Unicore RECTIME is NavSolution, so no Unicore log says "time pulse"). Mined from the vendor protocol package and its message library (e.g. `gps/internal/unc` plus `gps/lib/uncmsg`). For the UBX-like protocols (u-blox UBX, CASIC, Allystar, SDBP), where configuration and data output share one packet format, the table also includes the configuration messages SatPulse uses, with configuration uses in the same style: time pulse configuration, signal configuration, time mode configuration, message configuration, etc. For these protocols the number column is instead the hex class and ID in a single column, in the style of the vendor manuals: 0x01 0x21.

Later slices, still to be shaped:

- The library message file tags worth knowing, including explicitly the tags that set the module up for NTP use and for PHC/PTP use with satpulsed. In principle these fall out of the timing pages' device-independent requirements combined with the message mapping, but the page states them explicitly.
- Per-model detail, mined from the high-level configuration characterizations.
- Vendor mechanics that affect what a user does. For u-blox, for example: how ports work, native USB having no speed, save behaviour, the constraints on signal combinations.
- The vendor-specific message types for message files (the TODO in `gps-config/msg-files.md` resolves to these pages).

## Sources

The material is mined, not written from memory:

- The vendor protocol packages under `gps/`: the decoded message set, and the handler code that is the native-to-device-independent mapping.
- `configs/gpsmsg/<vendor>/`: the library files, their header comments, and READMEs, for the files and tags. A fair amount of page-ready prose already exists as comments there.
- `systest/gpscfg/*.sh`: each script records what a vendor without high-level configuration needs for a working timing setup (which native messages to enable, NMEA off, PPS on, plus wrinkles like the mosaic's command escape). The knowledge is the point, not the scripts themselves.
- `gpshwtest/HW/*.md` and `gpshwtest/baselines/*.json`: the exact model and firmware combinations characterized, and receiver behaviour (signal-combination rules, save granularity, quantizations). These need filtering: receiver behaviour is page material; the probe methodology and tool-bug archaeology are internal.
- The archived high-level configuration plans in `plan/archive/` (ubx-config, unicore, casic-config, techtotop, novatel, septentrio) for per-vendor design rationale.
- The systest fleet host files (private repository) for the models in continuous use and how they are wired.
- The local vendor protocol documentation (location in CLAUDE.local.md) as background: the protocol landscape (generations, port model, command style) the author needs to understand to write the page at all, and the source of the vendors' own terminology.

Because the facts are mineable, a first draft of a slice can be automated from its sources across all the vendors, then edited into the site voice.

## Interaction with other pages

- `intro/satpulse.md`: the protocol-support material moves here; the intro section shrinks and points at the index.
- The pages that currently enumerate the high-level vendors (`gps-config/high-level.md`, `setup/satpulsed.md`, `setup/rtk.md`) stop naming vendors and link instead.
- `setup/ntp.md` and `setup/phc.md` (#434) state their receiver requirements in terms of the device-independent model of `gpsprot/msg.go`, plus the routes to meeting them; the vendor page owns the concrete answer (the message mapping and the explicit tags) and links back. Neither restates the other: the setup page owns "what must be true", the vendor page owns "how, for this module".
- `hardware/gnss-modules.md` links each vendor's section to its module-support page. The hardware page says what to buy; the module-support page says what SatPulse does with it.

## Issues

- #453 is this plan's primary issue: short (motivation, the nav change), linking here for the detail.
- #434 (GPS configuration section) is rescoped to not own per-vendor configuration: its index and coverage statements link this section's index rather than the intro protocol-support list, its ntp/phc item is restated in the device-independent terms above without per-vendor tag examples, and it references this plan's issue.
- #445 and #446 (the u-blox and Unicore configuration pages) are closed: with the section built breadth first, their material arrives as later slices of this work rather than as standalone per-vendor pages.

The section should exist before the ntp/phc sections of #434 are written, so that their links have a target.

## Status

The first slice is done:

- The nav section exists, between Setup and Man pages for now; it moves after GPS configuration when the website-gps-config branch is merged.
- The index page lists the vendors with links. It also carries the supporting-a-protocol dimension list (moved out of `intro/satpulse.md`), so it is not the bare index sketched above.
- All nine vendor pages have the settled shape: opening paragraph, protocol paragraph, dimension bullets, tested models, supported-message table.
- There is additionally a NovAtel page, not in the original plan: it documents the OEM6/OEM7 protocol structure once, with the full decoded-log table; the ByNav and SinoGNSS pages link to it and state their vendor-specific differences (own logs, position type values, command syntax), as does the Unicore compatible-logs paragraph. No NovAtel receiver has been tested; the page says so.
- There is no Airoha page; the PAIR material lives on the Quectel page.
- The tier 1/tier 2 vocabulary is gone from the site.
- Interaction items done: `intro/satpulse.md`'s protocol-support section shrinks to the NMEA/RTCM framing plus a pointer to the index; the home page links the section; `setup/satpulsed.md` and `setup/rtk.md` link instead of enumerating the high-level vendors; `hardware/gnss-modules.md` links each of its vendor sections to the vendor's support page.
- `gps-config/high-level.md` is deliberately untouched on this branch; its vendor enumeration is replaced when merging with website-gps-config.

Remaining: the later slices above, the ntp/phc linkage (#434), and possibly the index support matrix.
