# Configuration response sequencing

## Problem

GPS configuration request completion currently depends on timestamp ordering. For poll-style requests, the UBX configurator records the last read timestamp for each configuration message ID and decides whether a request is still awaiting a response by comparing that timestamp with the request's sent timestamp.

For example, `msgPollRequest.AwaitingResponse` uses:

```go
return r.tRead[r.msg.ID()].Before(tSent)
```

This is fragile when two events at the request boundary have equal timestamps. A response to an earlier request can have the same timestamp as a later request's `SetSentTime`. In that case, `Before` returns false, so the later request can be treated as already having a response.

This is especially visible for repeated `CFG-VALGET` requests because all of them share the same UBX message ID while asking for different keys. A response to one `CFG-VALGET` can satisfy the timestamp check for the next `CFG-VALGET` if the timestamps are equal or otherwise not strictly ordered.

The missing concept is response consumption. Once a response has completed one request, the next request must not be able to reuse that same response event.

## Goal

Use a monotonic sequence number to determine response ownership.

Timestamps should continue to be used for deadlines, retry timing, pauses, and speed-change timing. They should not be the only mechanism for deciding whether a response belongs to a request.

The desired invariant is:

> A request is satisfied only by a matching response or ACK/NACK event processed after that request entered the corresponding awaiting state.

This applies to ACK ownership as well as response ownership. An ACK or NACK for an earlier request must not be reusable by a later request with the same acknowledged message ID.

The fix should not special-case `CFG-VALGET` by validating requested keys. `CFG-VALGET` exposed the bug because repeated requests share the same UBX message ID, but the ownership problem is general.

## Design

Add an event sequence counter to the UBX configurator.

Conceptually:

```go
type Configurator struct {
	eventSeq uint64
	tRead    map[ubxbin.MsgID]time.Time
	msgSeq   map[ubxbin.MsgID]uint64
}
```

Every inbound configuration-relevant UBX message processed by the configurator increments `eventSeq`. The configurator records the new sequence number for that message ID:

```go
c.eventSeq++
c.tRead[mid] = t
c.msgSeq[mid] = c.eventSeq
```

Each poll request records the sequence number visible when the request is sent:

```go
sentSeq := c.eventSeq
```

Then response detection changes from timestamp comparison to sequence comparison:

```go
responseReceived := c.msgSeq[mid] > sentSeq
```

This makes equal timestamps harmless. If response #0 and request #1 both have timestamp `T0`, response #0 still has a sequence number that is not greater than request #1's send boundary.

ACK and NACK packets should use the same event sequence boundary. When an `ACK-ACK` or `ACK-NAK` is processed, the configurator should increment `eventSeq` and pass that ACK/NACK event sequence into ACK ownership matching. A request may claim the ACK/NACK only if the event sequence is greater than the request's sent sequence.

## API shape

Use the Option B direction: keep the public `gpsprot.ConfigRequest` interface unchanged and keep sequence ownership inside the UBX implementation.

Extend the UBX-internal `configRequest` with a sent sequence:

```go
type configRequest struct {
	ops       requestOps
	cfg       *Configurator
	state     configRequestState
	sentTime  time.Time
	sentSeq   uint64
}
```

When `SetSentTime` is called, the request records the configurator's current sequence before evaluating whether a response is needed:

```go
cr.sentSeq = cr.cfg.eventSeq
needsResponse := cr.ops.AwaitingResponse(cr.sentSeq)
```

Change the UBX-internal `requestOps.AwaitingResponse` method to take the sent sequence and drop the time argument:

```go
AwaitingResponse(sentSeq uint64) bool
```

`requestOps` is package-private (lowercase) and has no implementors outside `gps/internal/ubx`, so there is no compatibility reason to retain `tSent`. Implementations that previously ignored the time argument (`msgRequest`, `msgRateRequest`) can ignore the sequence argument the same way. The poll implementations should use:

```go
return r.msgSeq[mid] <= sentSeq
```

The problem is currently in the UBX implementation's response ownership model, not in the shared director API. The shared director still needs only actions, deadlines, and request state transitions. Keeping the public interface unchanged reduces risk for other protocols such as Unicore and CASIC.

## Implementation steps

1. Add `msgSeq map[ubxbin.MsgID]uint64` and `eventSeq uint64` to `gps/internal/ubx.Configurator`.

2. Initialize `msgSeq` in `newConfigurator` beside `tRead`.

3. Add helpers to record configuration message observations and generic config events:

```go
func (c *Configurator) nextEventSeq() uint64 {
	c.eventSeq++
	return c.eventSeq
}

func (c *Configurator) noteMsg(mid ubxbin.MsgID, t time.Time) {
	seq := c.nextEventSeq()
	c.tRead[mid] = t
	c.msgSeq[mid] = seq
}
```

4. Replace all direct assignments to `c.tRead[...] = t` in `processMsg` with `c.noteMsg(..., t)`.

   The current sites are:

   - `MonComms`: `c.tRead[mt.ID()] = t`
   - `MonGnss`: `c.tRead[mt.ID()] = t`
   - `MonGnss1`: `c.tRead[mt.ID()] = t`
   - `mid.CfgClass()`: `c.tRead[mid] = t`

5. Increment `eventSeq` for `ACK-ACK` and `ACK-NAK`, and pass the ACK/NACK event sequence into `processAckNak`.

6. Extend `configRequest` with `cfg *Configurator` and `sentSeq uint64`.

7. Update `addRequest` so every request stores `cfg: c`.

8. In `configRequest.SetSentTime`, record `cr.sentSeq = cr.cfg.eventSeq` before calling `AwaitingResponse`.

9. Change the UBX-internal `requestOps.AwaitingResponse` signature from `(tSent time.Time) bool` to `(sentSeq uint64) bool`.

   This is a mechanical fan-out through all `requestOps` implementors. Grep for `AwaitingResponse` in `gps/internal/ubx/ubxcfg.go` and update `msgRequest`, `msgPollRequest`, `pollRequest`, `msgRateRequest`, and embedded request types that inherit those implementations. Also update the call sites in `configRequest.SetSentTime` and `configRequest.checkComplete`, which currently pass `cr.sentTime` and should pass `cr.sentSeq` instead.

10. Change `msgPollRequest.AwaitingResponse` and `pollRequest.AwaitingResponse` to use sequence comparison for response ownership.

   `msgPollRequest` should check `r.msgSeq[r.msg.ID()] > sentSeq`.

   `pollRequest` should check `r.msgSeq[r.msgID] > sentSeq`.

   The method should return true while the response is still awaited, so the implementation will likely be the inverse:

```go
return r.msgSeq[mid] <= sentSeq
```

   `pollTp5Request` embeds `pollRequest`, so it should inherit the behavior.

11. Change ACK/NACK matching in `processAckNak` so the matching predicate requires the ACK/NACK event sequence to be greater than the request's `sentSeq`.

   The timestamp check `!t.Before(cr.sentTime)` should no longer be the ownership boundary. Timestamps may still be logged or used for pause start times after a valid ACK/NACK has been matched.

12. Keep the old timestamp map for code that needs `tRead` as an observation time, but do not use timestamp comparison as the primary ownership test.

13. Leave timeout and deadline behavior unchanged.

## Test plan

Add a regression test that simulates equal timestamps around repeated `CFG-VALGET` requests.

The test should model this ordering:

```text
send CFG-VALGET #0 at T0 - 2 ms
receive response #0 at T0
send CFG-VALGET #1 at T0
receive ACK for #0 at T0
assert CFG-VALGET #2 is not sent yet
receive response #1 at T0 + 4 ms
receive ACK for #1 at T0 + 4 ms
then CFG-VALGET #2 may be sent
receive response #2 at T0 + 8 ms
receive ACK for #2 at T0 + 8 ms
configuration succeeds
```

Assertions:

- A response with sequence equal to the request's send boundary does not satisfy the request.
- A response with sequence greater than the request's send boundary satisfies the request.
- An ACK/NACK with sequence equal to the request's send boundary does not satisfy or fail the request.
- An ACK/NACK with sequence greater than the request's send boundary can satisfy or fail the request.
- A stale NACK for an earlier request does not fail a later request waiting for an ACK/NACK with the same acknowledged message ID.
- Repeated `CFG-VALGET` requests are not advanced by earlier `CFG-VALGET` responses with equal timestamps.
- The path where a response arrives before the matching ACK still leaves the request awaiting ACK rather than marking it complete.
- Existing timeout and retry tests still pass.

The regression should live near the UBX configuration tests because it is testing UBX response ownership, not desktop UI behavior.

## Risks

The main risk is accidentally changing timeout semantics. The sequence number should only decide whether a response has been observed after the request started waiting. Deadlines should continue to use time.

Another risk is incrementing the sequence for messages that should not satisfy a request. The sequence should be recorded for configuration-relevant messages in the same places where `tRead` is currently recorded. That keeps behavior aligned with the existing request matching model while changing the ownership boundary from timestamp to event order.

There is a separate latent risk in `checkComplete` for ACK states. When the response is observed first, the current `stateAwaitingAckAndResponse` branch transitions to `stateAwaitingAck` and recursively calls `checkComplete` with the response time. That recursive call can mark the request succeeded even though no ACK has been received. Also, `checkPollResponses` currently calls `checkComplete(t)` on every request, including requests already in `stateAwaitingAck`; the `stateAwaitingAck` branch unconditionally succeeds the request when `Pause() == 0`. This is not the root cause of the timestamp equality failure, but it is adjacent to ACK ownership and should be addressed or explicitly verified while adding sequence-based ACK matching.

Implementation should handle both sides of that risk:

- Do not recursively call `checkComplete` after a response transitions `stateAwaitingAckAndResponse` to `stateAwaitingAck`.
- Ensure response-driven completion cannot complete a request that is waiting only for ACK. This can be done by making `checkPollResponses` skip `stateAwaitingAck` requests, or by separating response completion from ACK completion so the `stateAwaitingAck` branch only runs for `processAckNak`.

Do not add `CFG-VALGET` key matching as part of this change. It would be a protocol-specific defense for one message type, while sequence ownership fixes the general same-message-ID ownership problem for poll responses and ACK/NACK packets.
