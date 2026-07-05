---
name: implement-configprotocol
description: Implement gpsprot high-level (device-independent) configuration - the ConfigProtocol/Configurator - for a new GPS receiver protocol family
---

# Implementing high-level configuration for a GPS protocol

This skill captures what was learned implementing the CASIC configurator
(#229, the third configurator after ubx and unc) and the Allystar
configurator (#344, the fourth): the semantics rulings the project owner
handed down, the protocol questions that dictate the design, the
verification ladder, and the working process. Read it before designing
anything; most of it was learned by getting it wrong first.

The owner's eventual goal (not yet reached): plug in a receiver with
the Markdown documentation for its protocol, and an agent brings up
the ENTIRE stack - message files, packet library, packet processor,
configurator, characterization - to full support. Every owner
intervention a session still needs marks a gap in this skill or in
the governing contracts; capture it so the next bring-up needs one
intervention fewer.

## The one rule that governs everything

`gpshwtest/SEMANTICS.md` defines what high-level configuration means
independently of any receiver. Every guarantee in it is binding on every backend:
truthful achieved values and readback, refusal leaves configuration
unchanged, nonexistence shown not announced, requested information
delivered. There is no such thing as an acceptably device-dependent
semantic detail. When tempted to deviate "because this receiver works
differently", re-read [rulings.md](rulings.md) first - the temptation
has come up before and the owner has ruled on it.

## Scope: configuration only, not message processing

This skill implements the CONFIGURATOR - the high-level configuration
knobs (message enable/disable, output modes, rates, survey, speed, NVM
save/reload). Enabling any message the protocol offers is in scope. What
is OUT of scope is message PROCESSING: the lib-layer decode and the
domain-layer conversion to `gpsprot.Msg`.

- PREFER enabling messages the stack already processes (a lib struct
  plus the `gpsprot.Msg` conversion that consumes it). When a
  configuration option can be satisfied by more than one message, choose
  one that is already processed.
- You MAY still enable a message whose processing is not yet implemented
  - that is configuration. But do NOT implement its processing here.
  Call the processing out as separate work on its own branch; this
  change only enables the message. "Nothing implementable is out of
  scope" governs configuration, not processing - see
  [rulings.md](rulings.md).

## What transfers between protocols and what does not

The DESIGN of an existing configurator does not transfer. casic's
single request type with orthogonal capabilities, ubx's step roster,
unc's command repetition - each shape was dictated by its protocol's
properties (response correlation, ACK guarantees, NAK information
content, line budget). Study ubx and unc to learn the ConfigDirector
CONTRACT, then derive the new design from the new protocol's answers to
[protocol-questions.md](protocol-questions.md). The owner's bar is "a
beautiful elegant implementation", judged against the existing
configurators; elegance comes from fitting the protocol, not from
copying a previous shape.

What does transfer:

- The inputs the owner must provide and the lower stages that should
  exist first (verified message files, packet library, processor):
  [inputs.md](inputs.md)
- The semantics contract and the owner's rulings: [rulings.md](rulings.md)
- The design-shaping questions to answer on hardware before committing
  to a design: [protocol-questions.md](protocol-questions.md)
- The ConfigDirector contract knowledge that is not (yet) in the
  gpsprot doc comments: [director-contract.md](director-contract.md)
- The verification ladder and evidence standards:
  [verification.md](verification.md)
- The working process, loop artifacts, and operational gotchas:
  [process.md](process.md)

## Process in one paragraph

Work on a dedicated feature branch; tooling/framework changes
(gpshwtest, shared libraries) go on their own branches that land
independently - never merge one into the other and never mix concerns
in one commit. Start from a staged plan whose stage 0 resolves the
design-shaping unknowns on real hardware; do not trust vendor
documentation. Run as a loop of bounded, committed, verified
increments with PROGRESS.md as compaction-proof memory. Verify up the
ladder: offline fake-receiver tests through the real director path,
replay tests with committed hardware traces, then gpshwtest
characterization to green committed baselines. Implement every
CONFIGURATION the protocol offers - nothing configurable is out of
scope - preferring messages the stack already processes, and defer
message DECODE (not the enabling) to a separate change (see Scope
above). Show what the receiver cannot do as absence, never as an error.
