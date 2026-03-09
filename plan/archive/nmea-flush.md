# NMEA Satellite Flushing Logic

This document explains the original flushing logic for combining GSV (satellite view) and GSA (satellite active) messages before the changes for GitHub issue #88.

## Overview

The NMEA satellite buffer needs to solve two key problems:
1. **Combining multi-part GSV sequences** - GSV messages come as "message X of Y" sequences that need to be reassembled
2. **Combining GSV with GSA data** - GSA messages contain which satellites are actively used for positioning, but arrive separately from GSV messages

The original logic used **GNSS constellation tracking** to determine when all expected GSV sequences were complete, combined with a **learning mechanism** for GSA timing.

## GNSS Constellation Tracking

The system tracked which GNSS constellations (GPS, GLONASS, Galileo, etc.) were sending GSV messages:

```go
type satellitesBuffer struct {
    // ... other fields ...
    gnssComplete gpsprot.GNSSSet // which GNSS have completed their GSV sequences  
    gnssExpected gpsprot.GNSSSet // which GNSS we expect to see
    gnssKnown    bool            // whether we've learned the expected pattern
}
```

### Learning Phase vs Operating Phase

**First few bursts (Learning):**
- `gnssExpected` accumulates all GNSS types that send GSV messages
- `gnssKnown` remains false until we see a repeated pattern

**After learning (`gnssKnown = true`):**
- System knows which GNSS constellations to expect
- Only flushes when ALL expected constellations complete their GSV sequences

### Flush Trigger Logic

When a GSV sequence completed (final message where `gsv.numMsg == gsv.msgNum`):

```go
flag := gpsprot.GNSSSetOf(gsv.gnss)
if sb.gnssExpected&flag != 0 {
    sb.gnssKnown = true  // We've seen this GNSS before - learning complete
} else {
    sb.gnssExpected |= flag  // Add this GNSS to expected set
}
sb.gnssComplete |= flag  // Mark this GNSS as complete

// Only flush when we know the pattern AND all expected GNSS are complete
if sb.gnssKnown && sb.gnssComplete == sb.gnssExpected {
    sb.maybeFlush(h)
}
```

## GSA Learning Mechanism

The system handles GSA (satellite active) messages that arrive at different times relative to GSV messages.

### The Problem
GPS receivers vary in their message ordering:
- Some send: GSV → GSA → GSV → GSA (interleaved)  
- Others send: GSV GSV GSV → GSA GSA GSA (batched)
- Message order can be: GSA first, or GSV first

### The Solution: Adaptive Learning

The `gsaWait` flag creates a learning mechanism that adapts to each receiver's pattern:

**State tracking:**
- `gsas []gsaSentence` - buffer of received GSA messages
- `gsaWait bool` - flag indicating we're waiting for GSV after seeing GSA

**Learning flow for "GSV first, then GSA" pattern:**

1. **First burst:**
   - GSV completes → GNSS tracking triggers `maybeFlush()`
   - `gsaWait` is false, so flush immediately (Used flags will be wrong)
   - GSA arrives later in same burst
   - `idle()` called at burst end: sees no GSV but has GSA
   - Sets `gsaWait = true` and discards GSA (learning complete)

2. **Subsequent bursts:**
   - GSV completes → GNSS tracking triggers `maybeFlush()`  
   - `gsaWait` is true, so clear flag but don't flush (wait for GSA)
   - GSA arrives later in burst
   - `idle()` called: flushes combined GSV+GSA with correct Used flags

### The Trade-off

- **First burst**: Sacrifice accuracy to learn the timing pattern
- **All subsequent bursts**: Perfect GSV+GSA combination with correct Used flags
- Since the system runs continuously (every second), getting the first second wrong is inconsequential

### Dynamic Adaptation: "Wait Only Once"

The GSA learning mechanism includes a crucial feature to handle changing patterns:

**The `maybeFlush()` "wait only once" logic:**
```go
func (sb *satellitesBuffer) maybeFlush(h gpsprot.MsgHandler) {
    if len(sb.gsvs) == 0 {
        return
    }
    if sb.gsaWait {
        sb.gsaWait = false // wait only once
    } else {
        sb.flush(h)
    }
}
```

**Scenario: GSA pattern changes**

1. **Burst 1:** GSV → GSA (sets `gsaWait = true`)

2. **Burst 2:** GSV completes, but GSA doesn't come this time
   - `maybeFlush()` called: `gsaWait` is true, so clear flag but don't flush
   - No GSA arrives in burst
   - `idle()` called: `gsaWait` is now false, so flush GSV data (Used flags wrong)

3. **Burst 3+:** GSV completes, no GSA
   - `maybeFlush()` called: `gsaWait` is false, so flush immediately
   - Back to "immediate flush" behavior

**Key benefits:**
- **Prevents indefinite waiting**: If GSA stops coming, system recovers after one missed flush
- **Adapts to pattern changes**: GPS receiver configuration changes, intermittent GSA messages
- **Self-correcting**: Continuously adapts to whatever pattern receiver is currently using
- **Robust**: Handles receivers that change behavior during operation

## Message Flow Examples

### Example 1: GPS + GLONASS receiver

**Burst 1 (Learning):**
```
GPGSV,2,1,... → GPGSV,2,2,... (GPS complete, flush immediately)
GLGSV,1,1,... (GLONASS complete, flush immediately)  
GNGSA,... GNGSA,... (GSA messages)
idle() → sees GSA without GSV, sets gsaWait=true
```

**Burst 2+ (Operating):**
```
GPGSV,2,1,... → GPGSV,2,2,... (GPS complete, but gsaWait=true so don't flush)
GLGSV,1,1,... (GLONASS complete, all expected GNSS done, but still don't flush)
GNGSA,... GNGSA,... (GSA messages arrive)
idle() → flush combined GSV+GSA with correct Used flags
```

### Example 2: Multi-constellation burst

The system waits for ALL expected constellations before flushing:
```
GPGSV complete → don't flush yet, waiting for GLONASS
GLGSV complete → now all expected GNSS complete → maybeFlush()
```

## Why This Approach Worked

1. **Handled receiver variations**: Adapted to different GSA timing patterns
2. **Efficient**: After learning phase, minimal overhead  
3. **Robust**: Worked with any combination of GNSS constellations
4. **Pragmatic**: Traded first-burst accuracy for long-term correctness

## What Changed (Issue #88)

With NMEA 4.10 receivers send **separate GSV sequences per signal ID** within each GNSS:
- GPS L1: `GPGSV,3,1,...,1` → `GPGSV,3,2,...,1` → `GPGSV,3,3,...,1`
- GPS L2: `GPGSV,2,1,...,6` → `GPGSV,2,2,...,6`

This breaks the GNSS constellation tracking because you can't predict when "all GPS sequences" are complete - there might be more signal IDs coming.

So we need to come up with a new design for handling flushing.

## New design

### Assumptions

We will make one significant assumption, which is not guaranteed by the NMEA spec: we will ssume that the receiver will output the sequence of sentences describing a navigation solution in a single burst, with no calls to Idle() in the middle of the burst. (An Idle() call will happen if there is 0.1s without data.)

But
- we will NOT assume that there is always a call to Idle() between bursts (the bandwidth may be completely used up)
- we will NOT assume at most one call to Idle() between bursts (may have multiple idle periods)
- we will NOT make assumptions about the relative order about different kinds of sentence in the burst

### When to flush

Two strategies
1. Flush on Idle() always: if there are GSVs emit them; clear accumulated GSVs/GSAs
2. Flush when we get a repeated GNSS/signal ID combo; this is protection against not getting any Idle() calls

For strategy 2, we want to choose a reasonable boundary between cycles rather than a completely random point based on where we started listening:
1. Introduce haveBoundary boolean in satellitesBuffer
2. We have a possibleBoundary function which tests whether haveBoundary is true; if not, sets haveBoundary and clear accumulated GSV/GSA
3. Call possibleBoundary() on Idle() and when the type of the current sentence is different from the type of the previous sentence
