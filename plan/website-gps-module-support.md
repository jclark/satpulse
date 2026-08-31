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

The standard shape, filled in slice by slice, each part beyond the first optional:

- Support status, by dimension: high-level configuration (supported / experimental / message files only) and `satpulsetool convobs` support, each stated on the vendor page itself, not on the index.
- Models tested, with firmware.
- Message coverage: which native messages are decoded, and which contribute to which device-independent message of `gpsprot/msg.go` (time, posGeo, posECEF, velGeo, velECEF, leapSecond, survey, satellites, navEpoch, corReport). These device-independent messages are user-visible in the event log, so the mapping explains what the user actually sees.
- The library message file(s) and the tags worth knowing, including explicitly the tags that set the module up for NTP use and for PHC/PTP use with satpulsed. In principle these fall out of the timing pages' device-independent requirements combined with the message mapping, but the page states them explicitly.
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
- The systest fleet host files (`~/satpulse-systest/hosts/*.md`, `gnss:` blocks) for the models in continuous use and how they are wired.
- `~/gps-protocol-docs/` as background: the protocol landscape (generations, port model, command style) the author needs to understand to write the page at all.

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
