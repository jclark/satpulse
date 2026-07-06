# Semantics of device-independent GPS configuration

This document defines the semantics of high-level (device-independent) GPS receiver configuration, as exposed by **satpulsetool gps** and implemented by the `gps/gpsprot` configuration model (`ConfigTarget` in `configtarget.go`). It describes what a request *means* and what is guaranteed, independent of any particular receiver. How a particular receiver realizes these semantics, and bugs in the implementation, are deliberately out of scope.

The underlying logic is uniform even where it is not obvious: **satpulse adjusts a request only using knowledge it actually has** - knowledge deduced from the receiver's identity or reported by the receiver's protocol - **and everything that happened is visible in the responses**. It never guesses at restrictions the receiver has not declared; those surface as refusals from the receiver itself. A requester can therefore always discover exactly what was achieved without knowing anything about the receiver.

## The model

A configuration request is built from three kinds of things, and one invocation may combine all three:

- **Properties**: named pieces of receiver configuration state. A request can set properties to desired values and read their current values back.
- **Message output requests**: what the receiver should emit, expressed per group of related messages.
- **Operations**: actions the receiver performs once - survey-in, saving to non-volatile memory, resets.

The distinction matters because the guarantees differ. Properties have readback; operations and message requests do not - their effects are observed differently (a survey completes, saved configuration survives a reset, requested messages flow).

**Showing changes nothing.** `--show-receiver` and `--show-config` never change the receiver's configuration, not even its running state, and are not delayed: they answer promptly rather than waiting on the receiver's own reporting schedules.

## Properties

The properties are: the enabled signals, timing GNSS, time pulse (width, period, alignment to GNSS, only-when-locked behavior, polarity), positioning mode (mobile or static, with the fixed position and its accuracy), antenna cable delay, minimum elevation, RTCM base station ID, and serial speed.

**Setting is best-effort.** A set request expresses intent in device-independent vocabulary; the realization is the closest configuration the receiver supports:

- Values are quantized to the receiver's resolution (a pulse width requested as 123.456 us may be achieved as 123 us).
- Values are limited to the receiver's range (a value beyond a bound may be achieved as the bound).
- Settings the receiver couples together move together.

What was *achieved* is reported in the response, not what was requested. The achieved values are the values the receiver **accepted**: they come from the set exchange itself, not from a separate readback. The requested and achieved values may legitimately differ; the difference characterizes the receiver, not a fault.

**Readback tells the truth.** Reading a property returns its current value as the receiver stores it. The stored value is normally identical to the accepted one, but they are two different observations and a receiver may re-express what it accepted when storing it (a fixed position accepted in geodetic coordinates may be stored, and therefore read back, in ECEF coordinates). Both reports are truthful at their own stage; neither is derived from the other.

**Nonexistence is shown, not announced.** A backend that does not have a property does not fail requests that mention it. Setting it reports nothing achieved for it, and readback does not include it. The absence in the responses *is* the statement that the property does not exist there.

**Refusal changes nothing.** When the receiver refuses a request, the request fails with an error and the receiver's configuration is left unchanged - a failed request is never a partial one. (The error text comes from the receiver and may not be illuminating; the semantics are in the refusal itself.)

### Enabled signals

The enabled-signals property looks like it has two dimensions - constellations and bands - but it does not. Its value is a single *set of signals*, where each signal is a concrete broadcast belonging to one constellation and one frequency band: GPS L1 C/A, GPS L2C, Galileo E1, Galileo E5b, BeiDou B1I, GLONASS L1, QZSS L1S, and so on (`signal.go` defines the full universe).

**Constellations and bands are request syntax, not settings.** A request names constellations, and optionally bands, and that denotes a signal set: the named constellations' signals in the named bands. Naming no bands means all bands. So `GPS,GAL` denotes every GPS and Galileo signal; `GPS,GAL` with band `L1` denotes their L1-band signals only. Individual signal syntax can add concrete signals to that set or, without `--gnss`, name the complete enabled set directly; `--except-signal` subtracts concrete signals from a `--gnss`/`--band` expansion. There is no notion of enabling a constellation apart from enabling its signals.

**Realization is a pipeline with a precise meaning of best effort:**

1. satpulse deduces the **supported signal set** of this receiver from its identity (model and firmware version, and what the protocol reports about its signal plans).
2. The requested set is **intersected** with the supported set. This intersection is what best effort means: signals the receiver does not have are dropped silently, because their absence is knowable in advance and reflected in the achieved set.
3. Restrictions that the receiver's protocol itself **reports** are applied next. For example, the protocol may report how many major constellations can be enabled simultaneously; if the intersection exceeds that, satpulse fixes up the request to fit, with a deliberate preference for what to sacrifice (GLONASS is dropped first). This is still knowledge the receiver declared, so satpulse may act on it.
4. The resulting set is requested from the receiver.
5. Restrictions the protocol does **not** report are not modeled and not guessed at. If the receiver rejects the requested set - some receivers enforce per-constellation signal pairings, or other validity rules satpulse has no way to know - the rejection surfaces as a refusal: error, configuration unchanged.

The achieved set is reported and reads back, so every silent intersection, every fixup, and every refusal is visible to the requester. A receiver's signal limitations need no documentation to be discovered: request and observe.

**PPP is never enabled implicitly.** Enabling a signal that carries PPP corrections - Galileo E6 for HAS - does not enable PPP itself; an explicit PPP property is planned. This is a rule about PPP, not about corrections generally: SBAS is deliberately the opposite, and enabling an SBAS signal enables SBAS.

## Message output

There are two completely different kinds of message output request, and they must not be conflated.

**Wire-format output: NMEA and RTCM.** These requests name standardized wire messages themselves - NMEA sentence types (RMC, GGA, GSA, GSV, ZDA, VTG, GLL) and RTCM message families (MSM4 and MSM7 per enabled constellation, ARP). For these groups the messages *are* the interface: downstream consumers parse exactly these standardized formats, so the request must control exactly which appear. The meaning is that the receiver emits the named message types. Every request is complete: message types in the group that are not named are turned off.

**Semantic output: PVT, satellite information, raw data.** These requests name information in the device-independent model:

- PVT: position, velocity, time of the navigation solution, time of the time pulse, leap second information, survey-in progress, solution quality, end-of-epoch markers.
- Satellite information: per-satellite status (position in the sky, use in the solution), and per-signal status (each signal received from each satellite).
- Raw data: raw observations (for RINEX), raw navigation data (subframes).

The meaning is: *the receiver is configured so that the named information is delivered*. Which receiver messages deliver it is the realization's business. Message granularity belongs to the receiver - one native message often carries several kinds of information - so a message enabled because it delivers requested information may deliver other information along with it. **Delivering more than was asked for is normal and meaningless; the guarantee is that what was asked for is delivered.** Asking what messages realized a request is asking the wrong question; the right question is whether the requested information arrived.

Raw navigation data is event output, not solution-rate output: the request means each item of navigation data is delivered as the receiver obtains it (a subframe as decoded, an ephemeris when new or changed), not re-delivered at a fixed rate. A receiver may additionally deliver the navigation data it already holds when output is enabled; data it has not yet decoded appears only as broadcast. Re-delivery of unchanged navigation data is not requested and is not significant.

Content preferences select *within* the delivered information rather than adding kinds of it: TAI time rather than UTC, ECEF coordinates rather than geodetic. The `after` preference concerns the time pulse: a pulse-time message that precedes its pulse is not sufficient on its own; there must be time information following the pulse, which a post-pulse message satisfies by itself and a pre-pulse message satisfies only in combination with time messages after the pulse.

Satellite information is an independent stream, not part of the navigation epoch: the end-of-epoch marker asserts that the epoch's solution information has been delivered, and says nothing about satellite information, which may be delivered after it.

**Turning off.** A request also says what happens to the rest of its group: messages other than those carrying requested information are turned off. For the wire-format groups and the satellite and raw groups this is implicit in every request. For the PVT group it is explicit (`off`), and a PVT request without `off` is incremental: it enables what it names and leaves the rest of the group alone. The asymmetry is deliberate, for efficiency - PVT messages are the high-rate operational core, and incremental requests let one message be added without reconfiguring and disrupting the rest. The convenience values `ptp` and `ntp` are exact abbreviations of flag sets (see **satpulsetool-gps(1)**) and mean nothing beyond their expansion.

## Operations

**Survey-in** starts a position survey, parameterized by a minimum duration and an accuracy limit; the survey runs until both are satisfied, and the receiver then assumes the surveyed antenna position and runs in static mode. The parameters belong to the operation, not to the configuration state: they cannot be read back, any more than any other argument of a past action. The *result* - the positioning mode with its fixed position - is a property.

**Save** persists running configuration to non-volatile memory.

- Saving everything persists the entire running configuration.
- Saving minimally persists at least the changes made by the same invocation, plus as little else as possible. How little is the receiver's *save granularity*: receivers persist configuration in groups, so saving one property may persist everything in its group. The granularity is a partition of the properties, ranging from one group per property (perfectly selective) to a single group (saving anything saves everything).

**Resets** restore state from non-volatile memory, with strictly increasing scope: *reload* restores configuration only; *reset* additionally discards the receiver's knowledge of position, time, and satellite orbits; *factory reset* first restores the non-volatile memory itself to factory defaults and then resets.

Changes made after the last save do not survive a reload or reset. That is not a hazard but the point: reload is how unsaved changes are deliberately discarded.

## Capabilities

A backend declares which optional capabilities it offers as a set of flags (`supports` in the JSON output): signal selection, speed configuration, survey and survey accuracy, survey progress messages, fixed position and its accuracy, raw output, RTCM MSM4/MSM7 output, RTCM base ID. The flags bound what can be expected of requests in advance; they do not gate them. Properties show their own nonexistence in the responses, as above.

An RTCM output request on a backend without the RTCM capabilities currently fails with an error. That behavior predates the capability flags and exists because a message request has no property through which the failure could show; the intended direction is for the flags to carry this information instead of an error.

## Summary of guarantees

Whatever the receiver:

- An invocation responds; it does not hang or silently do nothing.
- `--show-receiver` and `--show-config` change nothing on the receiver and wait for nothing.
- PPP is never enabled implicitly by a signal request.
- Reported values are the truth: a set response reports what the receiver accepted, and readback reports what it stores.
- A reported error is the truth: a failed request leaves the configuration unchanged.
- Requested information is delivered when the receiver can deliver it; more may come with it.
- Persistence works as stated: what a save was asked to persist survives reload and reset.

See **satpulsetool-gps(1)** for the command-line syntax that expresses these requests.
