# Design-shaping questions to answer before designing

The shape of a configurator falls out of the protocol's answers to
these questions. Resolve them on real hardware (plan stage 0) before
committing to a design; vendor documentation gets several of them wrong
for every protocol so far. The CASIC answers are given as a worked
example of how much each answer constrains the design.

## Response correlation: what does an acknowledgement name?

What identifies which request a response answers? This single property
dictates how many requests may be outstanding.

CASIC: ACK/NAK echoes only class+id, so at most one request per
class+id may be live (requests with the same id serialize; distinct ids
pipeline), and responses arrive strictly in request order. ubx ACKs
similarly name the message; unc matches command echoes in order.

Allystar: sets get ACK/NAK naming class+id, but polls get NO ACK - the
data message itself (id echo) is the answer, so a poll COMPLETES on
its data message. Response order across distinct ids is NOT request
order; correlate per class+id only. Same-id queues can be finite: one
unit answers exactly 12 of 16 queued same-id polls and silently drops
the rest - measure the depth, do not assume it.

## What does an ACK actually guarantee?

Does an acknowledged set always take effect, and what does the ACK
MEAN? Receivers exist that ACK and clamp (CASIC V6 reception mask,
Allystar CFG-NAVSAT): there the ACK's own semantics are "enabled the
intersection of the request with my capability" without naming the
result, so the achieved value comes from a post-set readback - that
is the ACK's meaning, not a workaround. Receivers also exist that ACK
and simply do not store the value (CASIC V5 NavSystem=0): that is a
DEFECT - report the accepted values, record the defect, and do NOT
add a verify roundtrip (owner ruling; see rulings.md). Test
deliberately: set a value the receiver might refuse and read it back
independently, to learn which kind you have.

The converse fails too: a receiver can NAK a set yet APPLY it (the
Allystar TAU951M NAKs the raw-output disable and applies it), so a NAK
is not proof of no-effect - the configurator, the fake, and restore
logic must all encode this where observed. Outcomes can even be
ERRATIC: after byte-identical duty-0 CFG-PPS writes the Allystar
TAU1302's readback sometimes shows no effect and sometimes a fully
cleared block. Erratic hardware is a defect to record in the HW
note, not to verify around: report the accepted values per the
readback ruling; the discrepancy is visible on a later readback.

## What does a NAK tell you?

CASIC NAKs carry no reason: unsupported-id and value-refusal are
wire-identical, which forces poll-first discovery (an id whose poll
answers exists; its set being NAKed is then a value refusal). A
protocol with reason codes can skip discovery polls.

## Poll semantics

How is a value read back? Does one poll produce one response, several
(CASIC CFG-PRT answers once per port - which also makes the active
port unidentifiable), or a dump (CASIC CFG-MSG with empty payload
dumps the whole rate table)? Can the active port be identified at all?

Is there an auto/default mode whose readback is indistinguishable from
an equivalent explicit setting, and can writing an explicit value couple
in others? CASIC V6 NAVBAND auto reads back the same enabled set as an
explicit mask, and writing the mask pulls in coupled signals (B1I drags
in B1C). When this is so, the as-found state may be unrepresentable as a
property value, which breaks faithful restore - see verification.md.

Coupling can also poison write-and-clamp: on the Allystar TAU1302,
writing an unsupported GAL E5 bit SWITCHES ON GAL E1, so
syntax-equivalent requests achieve different sets. The tempting fix -
intersect requests BEFORE the wire with a signal plan deduced from the
hardware identity string - was tried and REVERSED by the owner: the
chip number does not determine the signal plan (see rulings.md,
"Identity strings do not enumerate capability"). Write the requested
mask, let the silicon clamp, and report the achieved set from the
post-set readback the ACK semantics already earn; coupling then
surfaces honestly in the achieved set and is recorded in the HW note.

## Pipelining tolerance

How many requests survive in flight, across message kinds (sets,
polls, text)? Verify with mixed back-to-back requests on hardware.
The answer with the correlation granularity yields the
outstanding-request policy.

## Probe packet

What single packet identifies the protocol family without changing
receiver state, such that EVERY firmware answers something? CASIC: a
CFG-MSG poll of MON-VER - V6 answers MON-VER then ACK, V5 (no MON-VER)
answers NAK; either response proves CASIC. The NAK-as-detection trick
generalizes: a definite negative answer is still an answer. Account
for late probe responses arriving after configuration starts.

## Baud-change handshake

When does the receiver switch - immediately, after ACK, after delay?
At which rate does the ACK arrive (CASIC: at the NEW rate, so the
switch can garble it)? What solicited traffic can confirm the change?
Does a saved rate get re-applied by reload or soft reset (CASIC V5:
NO for either - the live port keeps its rate; a true persistence
oracle may require power cycling)?

Which port does a baud set even affect? On Allystar a set switches
the ARRIVING port's live rate immediately, whatever its portID field
says - portID selects only the stored readback record - so "which
UART am I on" is unanswerable AND irrelevant, and the transition
destroys the ACK at both rates (no-op sets ACK normally). Confirm
with a solicited poll at the new rate on a DISTINCT message id: where
same-id requests serialize, a confirm poll on the set's own id would
deadlock behind the never-retried speed change.

## NVM model

What does save cover - sections (CASIC V5 honors section bits) or
everything (CASIC V6 documents the mask as reserved but mask 0 saves
NOTHING with a vacuous ACK)? Does reload acknowledge or restart the
receiver (V5 ACKs in place, V6 restarts silently)? What do resets
clear, and does the receiver answer anything afterwards?

Expect per-unit divergence on the SAME firmware version: Allystar
CFG-CFG load is a genuine in-place load on one unit and an ACKed
NO-OP on two others (all firmware 3.018), so a uniform --reload must
use the soft reset. Mask bits can cover surprising sets (the "NMEA
message rate" bit also persists the RTCM rate table, yet nothing
persists binary message rates), and NVM can hold ghost values that
saves never overwrite and only a factory clear removes. The
persistence oracle must discriminate (rulings.md): a running==saved
check once produced a WRONG "verified" comment in a message file that
stood until a later session re-tested it with a real oracle.

## Firmware family divergence

Do families share the wire format but diverge in message classes and
enum encodings? CASIC V5/V6 share framing but: NAV vs NAV2 classes,
CFG-TP TBase INVERTED between families, different onlyWhenLocked
encodings, different NMEA ZDA ids, different CFG-RST layouts. Family
divergence this deep forces a family abstraction from the start;
probe and verify per family, never extrapolate. When one family's spec
reads like a degraded copy of a sibling's - fields relabeled "reserved"
- suspect the silicon still parses them: CASIC V6 documents the save
mask as reserved yet honors it (mask 0 saves nothing). Probe "reserved"
fields; do not trust the relabel.

## Factory calibration in writable fields

A writable field can hold per-unit factory calibration rather than
user configuration. Test: does the value survive a factory wipe? The
Allystar CFG-PPS Offset (530 ns on two units, 0 on a third) survives
a full factory clear - evidently per-unit calibration, so no user
property maps onto it and every set preserves it by
read-modify-write. Mapping a user property onto such a field would
silently destroy the calibration.

## Time-mode representation

Is there a mode register, or is the state the registers themselves?
Allystar has no mode register: FIXEDECEF nonzero IS position-hold,
both registers zero IS mobile, and survey completion AUTO-WRITES the
surveyed mean into FIXEDECEF - that transfer is the completion
signal. The SVIN-style message is continuous averaging telemetry,
NOT the survey state machine (its counters never reset). Establish
the model by experiment before mapping static/survey/fixed-position
semantics onto the registers.

## Line budget

At the default baud and default message load, does the transmit line
saturate? CASIC V5 at 9600 runs a ~6 s queue under default NMEA:
responses lag or vanish and packets get spliced mid-stream. This
shapes probing strategy, response windows, message-enable ordering
(quietest first), and operational guidance (persistently raise the
baud; free the line first by disconnecting the antenna or quieting
output if it is already saturated).

## Emission schedules

Some messages are output only when something changes, and the change
can be on a scale of minutes: the Septentrio ReceiverSetup block emits
every 60 seconds, and the UTC-parameter blocks that carry leap-second
data emit at the navigation-data renewal rate. A short watch after
enabling such a message observes nothing and invites a false "never
emits". Read the documentation and infer the scale first - finding the
scale may itself be an experiment - and size the observation window to
cover at least one period, or record the negative as bounded ("not
within N seconds"), never as absolute. (Incident: a 4-second stage-0
watch concluded ReceiverSetup was never delivered; the wrong "fact"
shaped the receiver-identity design until a re-test found the
60-second schedule.)

## Acknowledge-but-never-emit

Some firmware acknowledges enabling messages it never emits (CASIC F8N:
TIM2-TPX/LS/TIMEPOS, RXM2, RTCM; V5: the whole MSG class). Enabling is
therefore not evidence of delivery - only observation is. Per the
semantics this is shown as absence, not worked around and not an
error. Budget observation time for messages that are event-shaped or
on slow schedules (subframe data has a 12.5-minute period).

Capability is a property of the hardware, not the firmware version: two
different receivers on the same firmware family (F8N and AT632, both V6)
differ in which messages they actually emit. A support flag like
ConfigSupportRaw must be set from observation of the receiver in hand,
never extrapolated from the firmware version.

Declare the support set subtractively: start from
`gpsprot.ConfigSupportFull` and clear the flags the family lacks, rather
than OR-ing together the flags it has (unc and ubx both do this; see the
`ConfigSupportFull` doc comment). An additive list silently omits any
universally supported flag added to `ConfigSupportFull` later - the
backend keeps compiling and quietly under-declares a capability it
actually has - while a subtractive declaration inherits the new flag
automatically. The obligation flips: enumerate exactly what the receiver
lacks, so nothing it truly implements is dropped.

## What is genuinely not in the protocol

Some requested information will have no carrier (CASIC: epoch markers,
RTCM base ID, V5 per-signal info, active port). Audit the protocol
documents for each before concluding absence - "I could not find it"
is weaker than a documented audit - and record the audit result.

The doc can also be BEHIND the firmware: Allystar 3.018 answers ids
the doc lacks (GST/DTM/JAM sentences, the whole MSM4 family marked
"TBD") and emits messages the doc never mentions. Where discovery is
NAK-driven (every unknown target answers NAK, never silence), a
poll-only id sweep is cheap, state-neutral, and definitive - sweep
the id space on hardware rather than trusting the doc's message list.

Distinguish absence from FIXED behavior: absence means the receiver
cannot do or report the thing, not merely that there is no knob. A
behavior that is fixed and known (the Allystar pulse is always
GNSS-aligned; no CFG field exists) is reported as a constant property
value, not as absent - the report describes the receiver, not the
protocol's knob inventory. Leaving it absent had a real cost: a
shared property-completeness gate blanked the whole time-pulse
section of the --show-config text output (see verification.md and
director-contract.md).
