# What the owner provides, and what should exist first

A configurator project starts from inputs only the owner can supply
and from lower stages that should already exist. Missing items here
are worth raising at the START, once, rather than discovering
mid-loop.

## Inputs from the owner

- Protocol documentation: where it lives (e.g. ~/gps-protocol-docs/
  <vendor>/, AI-readable Markdown) and which documents cover which
  firmware families. Vendor docs are reference material, not truth -
  every protocol so far has had errata only hardware revealed.
- Attached hardware: for each receiver, the device path, model,
  firmware version/family, and serial speed (the CLAUDE.local.md
  pattern). Include known traps (e.g. a default baud whose default
  output saturates the line) and how receivers identities were
  verified.
- Permissions and limits: which units may be disrupted (NVM writes,
  resets), anything forbidden (e.g. factory reset on a unit holding a
  survey; no sdp/pulse observation without permission), and whether
  power cycling is possible (it may be the only oracle for some
  persistence questions).
- Reference implementations, if any (e.g. ~/casictool for CASIC V5):
  read them BEFORE rediscovering hardware behavior; their errata
  notes are condensed hardware experience.
- Branch expectations: the feature branch name, and that tooling/
  framework changes go on their own independently-landing branches.
- The governing documents: gpshwtest/SEMANTICS.md, the plan, CONTEXT.md (owner
  directives and verified findings), and the definition of done.

## Prerequisite lower stages

The implementation climbs stages; each lower stage de-risks the next:

1. Message files (configs/gpsmsg/<vendor>/*.toml), hardware-verified.
   These should exist BEFORE configurator work starts. They are the
   stage-0 instrument (answering the design-shaping questions needs
   only satpulsetool gps -m <file> -t <tag>, no new Go code), the
   recovery tool during hardware sessions (quiet/restore/reset tags),
   and tested prototypes of the wire structs - a verified payload
   layout transfers almost mechanically into the packet library.
   Their VERIFICATION COMMENTS are the value: an unverified tag can
   encode a payload that never worked. The gps-msg-file, gps-msg-add,
   and gps-msg-test skills build this stage on its own.
2. The packet library (gps/lib/<proto>bin): framing, checksums,
   ACK/NAK, and structs for the messages the configurator will need,
   each with a round-trip test. Often partially exists; extend it
   message by message as message files verify the layouts.
3. The packet processor (gps/internal/<proto>): parsing and
   gpsprot message conversion for the navigation output the
   configurator will enable.
4. The configurator itself - this skill's subject.

Expect feedback downwards: configurator work discovers wrong or
missing tags (fix the message file with a verification note), missing
structs, and processor gaps. Keep each fix at its own stage.
