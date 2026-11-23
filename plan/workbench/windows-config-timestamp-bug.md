# Windows desktop configuration timestamp bug

## Summary

The desktop app can fail a GPS configuration operation on Windows even though the receiver sends the expected response packets. The observed failure is a false `missing config items` error during a sequence of u-blox `CFG-VALGET` requests.

The packet timeline shows that the receiver did not drop the response. Instead, the desktop configuration path advanced to the next request too early, then failed before the real response for that request was processed.

The likely cause is timestamp equality around the request boundary. The UBX configurator uses packet read timestamps and request sent timestamps to decide whether a response belongs to the current request. On Windows, two `time.Now()` samples in this path can be equal across very short operations. When a previous response timestamp equals a later request's sent timestamp, the response is treated as not before the request and can be incorrectly counted as a response to the later request.

This is not caused by multiple input packets sharing the same input timestamp. That can happen normally. The problematic case is when an outbound request timestamp is equal to the timestamp of an already processed inbound packet used as the latest response time for the same message ID.

## Relevant code paths

The desktop app sends configuration requests through:

- `desktop/app.go`
  - `packetWorker` owns packet processing for the connected receiver.
  - `ReadConfig` and `ApplyConfig` transition to `StateConfiguring` and call `sendConfigRequest`.
  - `packetWorker` runs `gpscfg.Configure` inline when it receives a config request.

- `gps/app/gpscfg/gpscfg.go`
  - `configure` drives a `gpsprot.ConfigDirector`.
  - At the top of each action loop it calls `director.AdvanceTimeTo(time.Now())`.
  - For `ConfigActionSendRequest`, it writes the packet and then calls `cfgtor.Request(action.Index).SetSentTime(time.Now())`.
  - For `ConfigActionWaitUntil`, it reads one packet from the packet channel and processes it with `mh.packet(packet)`.

- `gps/internal/ubx/ubxcfg.go`
  - `Configurator.processMsg` records the last read time for each UBX configuration message ID in `c.tRead`.
  - `msgPollRequest.AwaitingResponse` decides whether a poll request still needs a response with:

```go
return r.tRead[r.msg.ID()].Before(tSent)
```

  - `pollRequest.AwaitingResponse` uses the same pattern:

```go
return r.tRead[r.msgID].Before(tSent)
```

  - ACK matching also uses the sent timestamp as a boundary:

```go
ubxbin.PacketMsgId(packet) == msgID && !t.Before(cr.sentTime)
```

All `CFG-VALGET` requests have the same UBX message ID. That means the `c.tRead` map can only say "the last CFG-VALGET response was read at this time"; it cannot distinguish which specific `CFG-VALGET` request that response belongs to.

## Observed packet timeline

The following packet timeline is sufficient to understand the bug. Times are UTC. `out` means the desktop app wrote the packet to the receiver. `in` means the desktop app read the packet from the receiver.

| Time | Direction | Packet | Length | Meaning |
| --- | --- | --- | ---: | --- |
| `00:58:06.871445` | out | `MON-VER` | 8 | Probe request sent. |
| `00:58:06.872698` | in | `MON-VER` | 228 | Probe response received. |
| `00:58:06.873399` | out | `CFG-VALGET` | 20 | Configuration request #0 sent. |
| `00:58:06.875667` | in | `CFG-VALGET` | 102 | Response to request #0 received. |
| `00:58:06.875667` | out | `CFG-VALGET` | 16 | Configuration request #1 sent. |
| `00:58:06.875667` | in | `ACK-ACK` | 10 | ACK for a `CFG-VALGET`. |
| `00:58:06.876189` | out | `CFG-VALGET` | 152 | Configuration request #2 sent. |
| `00:58:06.879326` | in | `CFG-VALGET` | 32 | Response to request #1 received. |
| `00:58:06.879326` | in | `ACK-ACK` | 10 | ACK for a `CFG-VALGET`. |
| `00:58:06.879326` | local system log | configuration failure | | `missing config items` reported. |
| `00:58:06.883235` | in | `CFG-VALGET` | 247 | Response to request #2 received. |
| `00:58:06.883235` | in | `ACK-ACK` | 10 | ACK for a `CFG-VALGET`. |

The key observation is that request #2's response arrived at `00:58:06.883235`, but the configuration operation failed at `00:58:06.879326`, about 3.9 ms earlier.

## Step by step failure

Define these timestamps:

```text
T0 = 00:58:06.875667
T1 = 00:58:06.876189
T2 = 00:58:06.879326
T3 = 00:58:06.883235
```

Request #0 behaves correctly:

```text
00:58:06.873399 out CFG-VALGET #0
00:58:06.875667 in  CFG-VALGET response #0
```

Processing response #0 updates the UBX configurator's last read time for `CFG-VALGET`:

```go
tRead[CFG-VALGET] = T0
```

Then request #1 is sent with the same timestamp:

```text
00:58:06.875667 out CFG-VALGET #1
```

When request #1 calls `SetSentTime(T0)`, `msgPollRequest.AwaitingResponse` checks:

```go
r.tRead[CFG-VALGET].Before(tSent)
```

Substituting the observed values:

```go
T0.Before(T0) == false
```

So request #1 is incorrectly classified as not needing a response. The code believes a `CFG-VALGET` response has already been seen at or after this request's sent time. In reality, the response at `T0` belongs to request #0.

The ACK at the same timestamp can also be treated as eligible for request #1 because ACK matching uses:

```go
!t.Before(cr.sentTime)
```

Substituting the observed values:

```go
!T0.Before(T0) == true
```

At this point request #1 can be considered complete even though its real response has not arrived yet.

The director then sends request #2:

```text
00:58:06.876189 out CFG-VALGET #2
```

The real response for request #1 arrives later:

```text
00:58:06.879326 in CFG-VALGET response #1
00:58:06.879326 in ACK-ACK
```

But request #1 has already been advanced past. Because request #2 is now active, response #1 can update the shared last-read timestamp for `CFG-VALGET` while request #2 is waiting:

```go
tRead[CFG-VALGET] = T2
```

For request #2, the sent time is `T1`. The same response check becomes:

```go
T2.Before(T1) == false
```

That makes request #2 look as if it has received its response. The configurator advances and evaluates the requested configuration data. The data expected from request #2 is missing, because request #2's actual response has not arrived yet.

The operation fails:

```text
00:58:06.879326 configuration failed: missing config items
```

Then the actual response for request #2 arrives:

```text
00:58:06.883235 in CFG-VALGET response #2
00:58:06.883235 in ACK-ACK
```

This response contains the items that were reported missing, but it arrives after the configuration operation has already returned failure.

## Why the code appeared to serialize requests

The code intends to wait for the response to one request before sending the next request. Structurally, the request loop does serialize through request states.

The failure is not that the director intentionally sends multiple `CFG-VALGET` requests without waiting. The failure is that request #1 was incorrectly marked complete immediately after being sent because the shared `tRead[CFG-VALGET]` timestamp from request #0 was equal to request #1's sent timestamp.

Once request #1 is mistakenly marked complete, the director is allowed to send request #2. The observed early send is a consequence of the timestamp comparison, not an explicit batching decision.

## Why this is Windows and desktop shaped

The issue has been observed in the desktop app on Windows. It has not been observed with the CLI on Windows, and it has not been observed with the desktop app on macOS.

That points to the interaction between the desktop packet plumbing, scheduling, and Windows clock behavior rather than a receiver protocol failure.

The repo already contains a recent related change:

```text
17cdbcb3 gpsprot: fix spurious ConfigDirector panic on Windows
```

That change replaced a strict time-advance guard in `ConfigDirector` with an `AdvanceTimeTo` call-count guard. The commit message describes the underlying condition: on Windows, two `time.Now()` samples across a short-running iteration can land in the same tick. This bug is another case where equal timestamps matter.

The earlier change fixed a spurious panic in `ConfigDirector`. The current failure is a different leak of the same assumption: the UBX configurator still uses strict timestamp ordering to distinguish old responses from current responses.

## Why shared input timestamps are not enough to explain the bug

Input packet timestamps can legitimately be equal. A scanner can read multiple packets in one read operation or otherwise assign the same `TRead` to more than one packet. That is acceptable.

The bad condition is narrower:

1. A response to an earlier request is processed and recorded in `tRead[msgID]`.
2. A later request with the same `msgID` is sent.
3. The later request's `SetSentTime` is equal to the earlier response's `TRead`.
4. `tRead[msgID].Before(tSent)` returns false.
5. The later request is treated as already having a response.

Repeated `CFG-VALGET` polls are vulnerable because all of them share one UBX message ID, while each poll can ask for different keys.

## Why increasing the packet log channel did not help

The packet log contains the missing response packet. The failure happened before that response was consumed by the configuration operation. This is not evidence of packet log backpressure or packet loss.

The large response for request #2 is logged at `00:58:06.883235`, after the configuration failure at `00:58:06.879326`. Increasing the packet log channel size cannot fix a state machine that has already decided the request completed before its response arrived.

## Likely fixes

A robust fix should avoid using wall-clock equality as the only request boundary for configuration responses.

Possible approaches:

1. Make each request boundary strictly later than any packet timestamp already processed by the configurator.

   For example, the configuration driver could maintain the last processed packet `TRead` and ensure the `SetSentTime` value is after it. This is a narrow fix but still relies on timestamps.

2. Track per-request response progress instead of only `tRead[msgID]`.

   For repeated poll messages such as `CFG-VALGET`, this is more robust. A request should only be satisfied by response data processed while that request is actually awaiting a response, not by the global last-read timestamp for the same message ID.

3. For `CFG-VALGET`, validate that the response contains at least one requested key for the active request before considering it satisfied.

   This would avoid counting a response to a different `CFG-VALGET` poll as success for the current poll. It is protocol-specific but matches the actual ambiguity.

4. Introduce a monotonic sequence counter in the configuration driver.

   Each processed packet can increment a counter. A request can record the counter at send time and require a response observed after that counter. This avoids depending on wall-clock granularity.

The safest direction is either per-request response tracking or a packet sequence counter. Both model the real ordering property needed by the configurator: responses must be observed after the request becomes active, not merely have a timestamp that is not before the request's wall-clock sent time.

## Regression test shape

A useful regression test should simulate equal timestamps around a repeated `CFG-VALGET` sequence.

The test should model:

```text
send #0 at T0 - 2 ms
receive response #0 at T0
send #1 at T0
receive ACK for #0 at T0
send #2 must not happen yet
receive response #1 at T0 + 4 ms
then send #2
receive response #2 at T0 + 8 ms
configuration succeeds
```

The important assertion is that request #2 is not sent before response #1 is processed, even when response #0 and request #1 share the same timestamp.

## Current conclusion

The failure is best explained as a timestamp-boundary race in the desktop configuration path on Windows.

The receiver responded correctly. The packet log captured the response that contained the missing data. The configuration operation failed because the state machine advanced one `CFG-VALGET` request too early after a previous response timestamp equaled the next request's sent timestamp.

The root cause is not packet loss, packet log channel capacity, or a receiver protocol problem. The root cause is that response ownership is inferred from `time.Time` comparisons and shared message IDs, and Windows can expose equality at exactly the boundary where the code needs strict ordering.
