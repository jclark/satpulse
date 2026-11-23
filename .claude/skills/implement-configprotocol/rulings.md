# Owner rulings, with the incidents that forced them

These are decisions the project owner handed down during the CASIC
configurator work (#229), each stated with the mistake that provoked
it. They are case law: when a similar situation arises, the ruling
applies, and arguing yourself out of it is repeating a mistake that has
already been corrected once.

## Readback semantics: an ACK is the readback

When a set is acknowledged, the value reported back is what the
receiver accepted - record the ACKed values as the assumed
configuration. A refusal (NAK) leaves the assumed configuration
unchanged. Do NOT re-poll changed values in a verify phase.

Incident: the CASIC configurator originally re-polled everything it
set. Ruling: "completely bogus ... that is not the semantic of
readback. When you set something, the value you report back is what
the receiver accepted. See SEMANTICS.md" (now gpshwtest/SEMANTICS.md).

A post-set readback is legitimate ONLY where the acknowledgement's
own semantics leave the achieved value unnamed. The CASIC CFG-NAVBAND
and Allystar CFG-NAVSAT ACK means "enabled the intersection of the
request with my capability" - reasonable receiver semantics, not a
bug - so the achieved signal set is read back to report the value the
receiver says it enabled, which the semantics require to be visible.
That is reading what the receiver is telling us, not checking up on
it.

A receiver that ACKs a value and does not store it is a DEFECT, and
the engine never adds a roundtrip to work around defects: report the
accepted values and record the defect in the HW note - a later
readback shows the discrepancy to anyone who asks. Incident: the
Allystar configurator verified CFG-PPS sets by readback because the
TAU1302 sometimes ACKs a zero duty cycle without storing it. Ruling:
"we are not doing an extra readback to work around bugs. What the
receiver tells us is what we report." The CASIC V5 NavSystem=0
ACK-and-ignore readback is the same wrong pattern - fix it when that
branch is next touched.

## Identity strings do not enumerate capability

Do not key capability on a hardware-identity table where the identity
does not determine the capability. Incident: to tame the TAU1302's
signal coupling, the Allystar configurator intersected signal requests
with a per-chip signal plan keyed on the MON-VER chip number. Owner
reversal: the chip number does not determine the signal plan - an
HD9310 ships in both L1/L2 and L1/L5 variants - so the table would
silently strip L5 from a variant it had not met (an active bug, not
mere fragility). Write the requested mask, let the silicon clamp, and
report the achieved set from the readback the ACK semantics already
earn. Identity-keyed capability is legitimate only where the identity
truly determines it: Allystar RTCM presence follows the chip FAMILY
(HD8* none, all others present).

## Probing is state-neutral

A probe must not change receiver state. Reliability comes from
repeating the probe, not from preparing the receiver or from long
timeouts. Late responses to earlier probes must be counted and
consumed so they are never misattributed to configuration requests.

Incidents: the CASIC probe initially quieted NMEA output first
("that really isn't OK. That is not the semantics"), and the response
window was stretched to 8 s for a saturated line ("far too long ...
Not an acceptable UI. ... probe multiple times for example").

## Nothing implementable is out of scope

If the protocol offers it, implement it. Do not defer features by
judgment call. If unsure whether something is implementable, audit the
protocol documentation (an agent against ~/gps-protocol-docs is fine);
do not ask the owner whether to implement it.

Incident: --raw-out and --rtcm-out were deferred as out of scope.
Ruling: "We should certainly implement --rtcm-out if it is possible
(of course) ... NOthing implementable is out of scope. NOTHING!"

This is about CONFIGURATION scope, not message processing - see
"Configuration only" below.

## Configuration only - do not implement message processing

The configurator's job is to ENABLE messages, not to teach the stack to
understand them. "Nothing implementable is out of scope" covers the
configuration knobs (flags, options, message-enable); it does NOT extend
to message PROCESSING - the lib-layer decode and the domain-layer
conversion to gpsprot.Msg.

Enabling any message is configuration and in scope. PREFER messages
whose processing already exists (a lib struct plus the gpsprot.Msg
conversion that consumes it), and choose a processed message when a
configuration option offers a choice. You MAY still enable a message
whose processing is not yet implemented - but do NOT implement that
processing here. Call it out as separate work on its own branch; this
change only enables the message.

Incident (#229 aftermath): the CASIC configurator bundled message decode
- TIM2-LS leap seconds, TIM2-TIMEPOS survey, V5 MSG-GPSUTC - into the
configuration commits. The processing later landed independently on
master; the duplicate, divergent decode then collided on merge and cost
a session to disentangle (revert the decode off the config branch,
re-land it on master, merge back). Configuration enables a message; a
separate, independently-landing change makes the stack process it.

## Receiver output beyond the model gets an explicit Other member

When the receiver's output of a message kind is wider than the modeled
group, neither blanket answer survives contact: turning the extra
targets off on every request made the TAU1302's shipped
broadcast-ephemeris RTCM output undeliverable through the option;
leaving them alone (the first fix) made a complete request unable to
silence them - "none" left ephemeris flowing. Owner resolution: an
explicit Other member in the group model. A request WITHOUT Other is
complete over the receiver's entire output of that kind and turns the
extra targets off (NAK-tolerant); a request WITH Other is restricted
to the modeled group and leaves them as found. Auto must NOT include
Other. CLI exposure of "other" is its own follow-up issue; until it
lands, a characterization restore that needs it fails honestly (see
verification.md).

## Semantics are never device-dependent

The man page and gpshwtest/SEMANTICS.md define behavior for every receiver. A
backend that satisfies them differently is fine; a backend where the
OBSERVABLE semantics differ is a bug, and so is an unclear man page.

Incident: --speed combined with --save persisted the OLD baud rate on
CASIC and this was reported to the owner as a footnote. Ruling: "NOOO
it is a bug. ... I don't know where you got the idea that a detail
like this can acceptably be device dependent." The fix: the speed
change runs before the NVM phase, so a save persists the new rate -
which is also the ubx ordering (setBaudRate before saveMinimal).

## Speed changes must work reliably

A baud change can garble its own ACK, so confirmation must come from
SOLICITED traffic at the new rate: unc repeats the identical command
(a no-op whose ACK confirms the original by request-order matching);
casic follows the change with a poll whose answer confirms the change
directly. Heuristic windows (valid-packet-after-delay) are only for
unsolicited traffic and must not be the only path: a solicited answer
arriving "too early" once made a successful change report failure.
Speed-change requests are never retried (director contract); requests
sequenced after one run on the confirmed new link.

## Vendor side channels only where the protocol leaves no choice

Prefer the binary protocol for everything. A proprietary text command
is acceptable only where the protocol offers no alternative.

Incident: PCAS NMEA commands crept into probing and rate restating.
Ruling: "We should not generally be using PCAS commands to do
anything. Only exception is for V5 to get the version info."

## Answer your own questions

Hardware, protocol documents, and existing code answer most questions.
Asking the owner something answerable is the failure mode. ("For all
your questions, go answer them yourself. ... stop using questions as
an excuse not to work.") Reserve the owner for genuine policy
decisions, and present those with analysis and a recommendation.

## The elegance bar

"A beautiful elegant implementation much better than UBX. Resilient in
the face of NAKs. If it is not yet, refactor until it is." NAK-driven
fallback is first-class behavior, not an error path. An independent
style review against CLAUDE.md plus the existing configurators, late in
the work, found real bugs as well as polish - worth repeating.

## Branch separation

Feature work and tooling/framework work live on separate branches that
land independently. Never merge one into the other; never mix the two
concerns in one commit. Characterization baselines for a new
configurator belong with the CONFIGURATOR branch (they are meaningless
before it lands), even though they live under the framework's
directory.

Incident: gpshwtest was merged into casic-config and two commits mixed
both concerns; disentangling cost a session. ("I really wanted these
to be two completely separate branches that can land independently.")

## Evidence must discriminate

A hardware "proof" only proves something if the test could have failed.
Incident: speed persistence was "proven" by a cold reset while the
running rate equaled the saved rate - the test could not distinguish
persistence from inertia, and on the V5 a soft reset in fact does NOT
re-apply the saved port rate. Before claiming verification, ask what
observation would differ if the claim were false. Relatedly: check
test output BEFORE committing, and never report a skipped or weak
verification as a strong one.

A reset can preserve config BY DESIGN, which makes it a non-oracle: a
cold start that keeps configuration (CASIC V6, documented) proves
nothing about persistence, and a reload or soft reset may be
ACKed-but-ignored. Read the protocol docs for what each reset actually
preserves before using one as a persistence oracle; where every
available reset preserves config, only a power cycle discriminates (USB
unbind/rebind is the practical mechanism).
