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

## What does an ACK actually guarantee?

Does an acknowledged set always take effect? Receivers exist that ACK
and ignore (CASIC V5 NavSystem=0) or ACK and clamp (CASIC V6 reception
mask). Where that happens, the on-ACK readback rule needs its narrow
exception (one post-set poll). Test deliberately: set a value the
receiver might refuse and read it back independently.

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

## NVM model

What does save cover - sections (CASIC V5 honors section bits) or
everything (CASIC V6 documents the mask as reserved but mask 0 saves
NOTHING with a vacuous ACK)? Does reload acknowledge or restart the
receiver (V5 ACKs in place, V6 restarts silently)? What do resets
clear, and does the receiver answer anything afterwards?

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

## Line budget

At the default baud and default message load, does the transmit line
saturate? CASIC V5 at 9600 runs a ~6 s queue under default NMEA:
responses lag or vanish and packets get spliced mid-stream. This
shapes probing strategy, response windows, message-enable ordering
(quietest first), and operational guidance (persistently raise the
baud; free the line first by disconnecting the antenna or quieting
output if it is already saturated).

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

## What is genuinely not in the protocol

Some requested information will have no carrier (CASIC: epoch markers,
RTCM base ID, V5 per-signal info, active port). Audit the protocol
documents for each before concluding absence - "I could not find it"
is weaker than a documented audit - and record the audit result.
