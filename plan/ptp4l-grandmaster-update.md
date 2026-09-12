# ptp4l grandmaster update not retried (#456)

Status: focused fixes to the current design are in progress and will be landed independently. A behavior-preserving PTP domain/updater encapsulation refactoring, a small change to what NoSync targets contain, and a larger updater redesign are separate follow-ups as of 2026-09-06. Baseline line numbers below are as of master 9dd87199; the staged phase 1 changes are described separately.

## Phase 1: focused fixes to the current design

The currently staged changes to `controller.go` and `gm.go` deliberately retain the existing architecture and fix three problems:

1. The critical observed late-start failure. Controller ticks collect the result of the first SET and, after an unsuccessful attempt, retry after 30 seconds. The retry goes through `Controller.gmUpdate` so the desired properties are recalculated before being sent. Thus ptp4l receives the current settings when it starts after satpulsed instead of remaining on its defaults indefinitely.
2. A target change while a request is in flight. The last attempted properties and whether that attempt was confirmed are recorded. A target that differs from the last attempt is always sent on the first controller tick after the older request completes, even if it equals an earlier confirmed value. It does not wait for another mode or leap-second change.
3. Delivery of the final NoSync target during shutdown. If an older request is outstanding, `Grandmaster.Close` waits for it before queuing NoSync. Once NoSync is queued, closing the request channel does not cancel it; the worker and daemon wait group give that final request its own 100 ms operation deadline.

Only the first item is required for the specific late-start failure. The other two are adjacent correctness fixes found while reviewing the same request state machine. Together they are a small, focused improvement to the current design, not the literal minimum number of changes for late-start retry alone. They must be retained, completed and landed without waiting for the larger redesign.

Controller-level tests in `phcsync/grandmaster_test.go` replace the direct `Grandmaster` tests. They use a simulated PHC, pulse and tick events, and a time-message buffer to reach tracking through the controller's public API, then inspect the actual PMC worker's SET messages through a fake transport. Coverage includes retry delay and successful convergence without another mode or leap-information change, recalculation across the leap announcement boundary and positive/negative leap transitions, delivery of a changed target after an in-flight request, final NoSync delivery after an older request with its own 100 ms deadline, and avoidance of a duplicate final request when NoSync is already in flight. `testing/synctest` controls time without real sleeps or sockets. Assertions concern controller events and externally sent settings; setup and transport adaptation isolate the current updater wiring for phase 4.

This phase does not add periodic GET reconciliation and therefore does not detect a ptp4l restart after a successful update when the desired properties have not changed. It also does not attempt the broader ownership and goroutine restructuring described below.

The preceding committed `pmc.Client` late-response fix is retained as a prerequisite. It prevents a response received after a deadline from poisoning subsequent retry transactions.

### Problem

When ptp4l is started after satpulsed, ptp4l's grandmaster settings are never updated. satpulsed reports tracking mode and in sync, but `pmc` shows ptp4l still on its defaults: clockClass 248, clockAccuracy 0xfe, timeSource 0xa0 (internal oscillator), currentUtcOffsetValid 0, timeTraceable 0. The user has seen this several times.

The same mechanism means a ptp4l restart while satpulsed is tracking also leaves ptp4l on its defaults indefinitely (see "Other defects" below); that case was not observed this time but follows from the code.

### Evidence (2026-09-03, local time UTC+7)

| Time | Event | Source |
|------|-------|--------|
| 11:34:34 | `satpulse@ttyUBX0.service` started | `systemctl status` |
| 11:34:48 | sync flag went to 1 | `/var/log/satpulse/clock.enp4s0.log` (UTC timestamps, last column is the sync flag) |
| 11:35:03 | `ptp4l.service` started | `ps -o lstart` |

The daemon journal (`sudo journalctl -u satpulse@ttyUBX0 --since 11:34 | grep -i 'ptp4l\|grandmaster'`) contains exactly one line, at 11:34:48: "the ptp4l management socket is not ready" with path `/var/run/ptp4l`, and nothing after it. There is no "has become ready" line and no "successfully updated" line.

### Diagnosis

The update path is: `phcsync.Controller.gmUpdate` -> `ptpgm.Grandmaster.Update` -> buffered request channel -> `ptpgm.PTP4LWorker` goroutine -> `pmc.Client` -> unixgram datagram to ptp4l's `uds_address`.

`Grandmaster.Update` (`time/internal/ptpgm/gm.go:65`) has retry semantics: while nothing has ever been confirmed (`gm.actual == nil`) and the state is InSync, every call re-sends the request. But it only does anything when it is called, and `gmUpdate` (`time/internal/phcsync/controller.go:458`) is called from just three places:

- `changeMode` (line 419), i.e. on a mode transition
- `LeapSecond` (line 305), which the dispatcher only invokes when the leap second state actually changes (`time/internal/gpsevent/dispatcher.go:593`)
- `Close` (line 364)

It is not called per sample. So the single attempt made when the controller entered tracking got ENOENT (no socket yet), the worker closed the response channel without a value, and no further call to `Update` happened because the mode stayed tracking and the leap second state did not change.

History: the pre-phcsync `Monitor` called `gmUpdate` from `updateSyncState`, which ran on every sample, so the retry worked by call frequency. The phcsync rewrite (7afa2495, d22d8c93) moved the call to mode changes and lost the retry without anyone noticing.

### How the phase 1 baseline works

This section describes the master version on which the staged phase 1 changes are based. It intentionally does not describe the additional retry, last-attempt and shutdown state in those staged changes.

`Grandmaster` (`gm.go`) is single-threaded state owned by the controller and must never block. `Update` computes the target props (clock quality from sync state plus leap second state), checks for a pending request, dedups against `gm.actual`, and hands a `GrandmasterUpdateRequest` with a fresh one-shot response channel to the worker via a non-blocking send on a channel of capacity 1. The reply is only picked up by `handleResponse` on the next `Update` call.

The response channel does two jobs. Its existence (`gm.respCh != nil`) means "request in flight, do not send another". Its content distinguishes success from failure: on success the worker sends back `req.props` (an echo of what it was given) and closes; on failure it only closes. `handleResponse` records the echoed props as `gm.actual` so that `actual` reflects what was sent even if the target moved while the request was in flight. `gm.actual == nil` means "never confirmed" and gates the very first send: a NoSync target is not sent until there has been an InSync one.

`PTP4LWorker` (`time/internal/ptpgm/ptp4l.go:21`) loops over requests, sets a 100 ms deadline (`ptp4lTimeout`), and does one `sendRecv`: a SET of GRANDMASTER_SETTINGS_NP and one read. ENOENT and ECONNREFUSED are logged once ("not ready") until the next success ("has become ready"); a deadline error is logged as a timeout; anything else is a warning. Every success is logged at Info with the props.

The non-blocking design is required because a blocking send or receive on the socket previously stalled the main goroutine when ptp4l was restarted. Whatever replaces this must keep the controller goroutine free of socket I/O.

`pmc.Client` (`time/lib/pmc/client.go`) binds an abstract-namespace unixgram socket (`\x00satpulse-<pid>`, see `transport.go`) and sends with `WriteTo` to the configured path each time, so a recreated socket file is picked up automatically. `Recv` (line 71) reads exactly one datagram and returns an error if its sequence ID, target port number, or action field does not match. Issue #64 (closed) is the SELinux history around ptp4l replying to the local socket; read it before changing the local socket.

`MsgPreparer` sets domainNumber, majorSdoId (transportSpecific) and minorSdoId from `[ptp]` config. This matters: ptp4l with `transportSpecific 0x1` silently ignores management messages with transportSpecific 0. Verified on this machine: `pmc` without `-t 1` gets no reply, with `-t 1` it does.

`Settings()` (`gm.go:128`) fills the whole GRANDMASTER_SETTINGS_NP TLV:

| Field | satpulse sends today | ptp4l.conf key (linuxptp `config.c`) |
|-------|----------------------|--------------------------------------|
| clockClass | 6 in sync, 52 out of sync | clockClass (default 248) |
| clockAccuracy | `ptp.clockAccuracy` | clockAccuracy (default 0xfe) |
| offsetScaledLogVariance | `ptp.offsetScaledLogVariance` or `ptp.allanDeviation`, default 0xffff | offsetScaledLogVariance |
| currentUtcOffset, leap59/61, utcOffsetValid | from GNSS leap second state | utc_offset (offset only) |
| ptpTimescale | always on | not a key; derived (`clock.c:1375`) |
| timeTraceable | on in sync, off out of sync | none |
| frequencyTraceable | on in sync, off out of sync | none |
| timeSource | GNSS (0x20) | timeSource (default internal oscillator) |

## Phase 2: PTP domain and updater encapsulation refactoring

This is a self-contained, behavior-preserving refactoring. It can be implemented and landed independently of the updater redesign in phase 4, and remains useful whether or not that redesign is undertaken.

The daemon currently constructs and stores a `pmc.Client` through `PTPConfig.NewClient()`, even though PMC is specific to ptp4l. At the same time, the three fields that identify the PTP domain are passed around as unrelated configuration fields. Phase 2 fixes those ownership and representation boundaries without changing when or what satpulse sends to ptp4l.

`ptpgm` gains the implementation-independent runtime value:

```go
type Domain struct {
	DomainNumber uint8
	MajorSdoID   uint8
	MinorSdoID   uint8
}
```

`PTPConfig.Domain()` returns this value from `DomainNumber`, `MajorSdoID` and `MinorSdoID`. `PTPConfig.NewClient()` is removed.

The existing ptp4l worker becomes an opaque `ptpgm.PTP4LUpdater`. `NewPTP4LUpdater` accepts a `Domain` and UDS address, constructs the PMC client, applies all three domain fields to it and returns an error synchronously. The daemon stores the updater rather than a PMC client and runs its `Run` method with the existing `GrandmasterUpdateRequest` channel. The updater owns and closes its client.

The following behavior is deliberately unchanged:

- PMC client construction occurs at the same point during daemon startup, and construction failure still prevents startup.
- The controller, `Grandmaster`, request and response channels, goroutine structure and shutdown ordering are unchanged.
- The updater performs the same SET-only operation for each request, with the same 100 ms deadline, response handling and logging.
- There is no periodic GET, new retry policy, latest-value mailbox or change to grandmaster properties.

The focused tests verify that `PTPConfig.Domain()` preserves all three domain fields and that `NewPTP4LUpdater` applies them to its PMC client. This phase needs no NEWS entry.

Implementation sequence:

1. Add `ptpgm.Domain` to `gm.go`.
2. Add and test `PTPConfig.Domain()`.
3. Replace `PTPConfig.NewClient()` and the `PTP4LWorker` function with `NewPTP4LUpdater` and its `Run` method, moving the existing PMC construction and worker logic without changing them.
4. Update the daemon to construct the updater from `Domain()` and the UDS address and run it with the existing request channel.
5. Run the relevant tests and land this refactoring separately.

## Phase 3: NoSync targets carry no leap information

This is a small change to what a NoSync target contains, independent of the updater redesign in phase 4. It removes the phase 1 hazard that retries during a receiver outage re-derive leap state from a frozen sample reference time: in reset mode `Tick` generates no missing samples, so `lastSample.Ref` stops advancing, and a retry that succeeds after a leap transition can push the pre-leap offset and leap flag to ptp4l after ptp4l has applied the leap itself.

A NoSync target clears `CurrentUTCOffsetValid` and the leap flags, keeping only `PTPTimescale`, NoSync clock quality and the time source. Its numeric UTC offset is retained from the last InSync target rather than recomputed; `Update` ignores the leap second state argument when the state is NoSync. The first-InSync gate is unchanged: the initial NoSync target has a zero offset and is never sent, and every later NoSync target carries an offset the controller has already attempted to send. InSync targets are unchanged.

Consequences: while satpulse is unsynchronized, ptp4l announces the last known offset flagged as not valid and no pending leap, so ptp4l does not apply a leap on its own during an outage, and a late-succeeding retry cannot conflict with its state. No five-second fallback to `UTCOffBefore` is needed here because nothing suppresses reconciliation around the transition; that fallback belongs to phase 4.

Changes: `SetClockSync` and `Update` in `gm.go`; the NoSync expectation in the `grandmaster_test.go` Close subtests.

## Phase 4: updater redesign

Phase 4 is the separate channel-based controller/updater redesign. It replaces the request/response state machine and adds periodic GET/conditional-SET reconciliation, including recovery after ptp4l restarts following successful convergence.

### Other defects in the same path

1. ptp4l restart. After one success `gm.target == *gm.actual`, so `Update` never resends even when called. A restarted ptp4l comes up on its config defaults and stays there until satpulsed changes mode.
2. Late reply poisons the socket. If ptp4l answers after the 100 ms deadline, the datagram stays in the receive buffer. The next `Recv` reads that stale datagram, fails with "sequence ID mismatch", and leaves the fresh reply behind, so every later exchange fails the same way. Meanwhile ptp4l has applied every SET, so satpulse logs errors while ptp4l is actually correct.

### Design problems with the current approach

The late-start failure is one symptom of a broader ownership problem. Desired-state calculation, request scheduling, transport outcomes and external-state detection are split across three layers, but no layer has enough information to ensure that ptp4l converges to the current target.

1. There is no single owner for convergence. `Controller` owns the inputs needed to calculate the current properties, including the reference time used for leap-second state. `Grandmaster` owns the target, deduplication and retry state. `PTP4LWorker` owns the operation and is the only layer that knows why it failed. Adding retry policy therefore requires either leaking transport outcomes back into `Grandmaster` or putting policy in the worker without giving it ownership of the current target.

2. `gm.actual` is not the actual ptp4l state. It is only the properties from the last SET for which a response was received. A SET can take effect even when its response is lost, ptp4l can restart and restore its configuration defaults, and another actor can change the settings. None of those changes are reflected in `gm.actual`, so equality with it is not proof that ptp4l currently has the target settings.

3. Successful convergence is assumed to be permanent. Once `gm.target == *gm.actual`, no further message is sent. This is why a ptp4l restart is not repaired: the local cache still says that the old process confirmed the target. Detecting this requires either periodically reasserting the target or reading the settings back from ptp4l.

4. The target contains time-dependent derived state. `LeapSecondState` is calculated by `Controller.gmUpdate` from `lastSample.Ref`, then stored as a snapshot in `gm.target`. Retrying the snapshot can send stale leap flags or UTC offset, while a confirmed target is never recomputed merely because reference time crossed the 12-hour announcement boundary or the leap itself. The work in progress routes a retry back through `Controller.gmUpdate`, but doing so exposes the awkward split between scheduling and target calculation.

5. One in-flight request is treated as a reason to stop reconciling. The single-request rule is needed by the existing PMC request/response bookkeeping, but `Update` simply returns when `respCh` is non-nil. A newer target is retained in memory without arranging an immediate follow-up when the request completes. Before the current work this could leave the newer target unsent indefinitely; the work in progress adds the last attempted properties and polls for completion so a changed target can bypass the retry delay.

6. Request completion is polled as a side effect of another operation. Originally, a response was collected only on the next `Update`; the retry work adds collection from a controller tick. The worker cannot directly trigger reconciliation when a request finishes, so correctness depends on some unrelated future call occurring and on the ordering between that call, target changes and retry-timer maintenance.

7. The response protocol discards the reason for failure. Success echoes the requested properties, while every failure is represented by closing the response channel without a value. This is enough to retry blindly, but not to let the component responsible for convergence distinguish a cheap immediate ENOENT/ECONNREFUSED failure from a request that occupied the worker until its 100 ms deadline or from a management-protocol error. The worker has the information, while `Grandmaster` does not.

8. Retry timing is entangled with request state. In the first retry implementation, a zero `nextRetry` meant "arm on the next tick", `Update` reset it even when no request was enqueued, and responses were not drained until the timer fired. Target, last attempt, confirmed result, in-flight state and next permitted retry are distinct states, but the original model represented only some of them. The work in progress adds `lastAttempt` and resets the timer only after an enqueue, which fixes immediate problems but makes the missing state model more apparent.

9. Shutdown needs a special transaction protocol. The controller must send a final NoSync target after cancellation, but a previous request may still be outstanding and the producer is required to wait for its response before sending another request. Simply updating the target and closing the channel can lose NoSync. The work in progress waits for an older transaction, enqueues NoSync as the sole outstanding request, then relies on the worker and daemon wait group to give it its full 100 ms allowance before process exit.

10. Error logging does not model persistent failure states. ENOENT and ECONNREFUSED are latched by `sockOK`, but timeouts and other errors are logged on every attempt. A timeout after `sockOK` has been cleared also falls through to the generic warning branch. Any periodic retry makes persistent configuration or protocol failures produce an unbounded stream of repeated messages unless logging records transitions between outcome classes.

11. The transport originally treated a late response as a new failure. A reply arriving after one deadline remained queued and was consumed by the next request, producing a sequence mismatch and leaving the new reply behind. The preceding late-response patch teaches `pmc.Client` to discard known late replies, but the defect illustrates that retry behavior cannot be designed independently of response correlation and socket queue state.

12. The baseline had no package tests for the asynchronous state. Phase 1 adds controller-level coverage in `phcsync` of target changes during an in-flight request, retry timing and property recalculation, and shutdown ordering through the PMC worker with a fake transport. Actual socket appearance, non-responsive sockets and a restarted ptp4l still require the phase 4 updater and socket integration tests described below.

Taken together, these problems require a different ownership boundary rather than more conditions in the current `Grandmaster` state machine. The controller should own calculation and publication of desired state, while one implementation-specific updater should own convergence of its external PTP server to that state.

### Design goals

1. Keep the controller independent of ptp4l and the PTP management protocol. It should calculate the desired grandmaster properties but should not know how they are applied, whether an operation succeeded, or when it should be retried.
2. Allow a different PTP server implementation to be supported by replacing one updater. The boundary is a channel of `GrandmasterProps`; no additional Go interface is needed. Each updater maps those common desired properties to its own update mechanism and owns its own retry policy.
3. Give one goroutine complete ownership of convergence for each configured PTP server. Protocol I/O, operation serialization, observed external state, retry timing and outcome logging must not be split between the controller and another worker.
4. Keep the controller goroutine non-blocking. In particular, it must never wait for a PTP server operation or for the updater to receive a new desired state.
5. Converge to the newest desired state. Intermediate states may be discarded if the controller publishes changes faster than the updater can apply them.
6. Do not periodically SET unchanged ptp4l settings. A SET causes ptp4l to run its state-decision processing, so periodic reconciliation should use GET and SET only when the returned settings differ from the target.
7. Detect loss of convergence after a successful update. The design must repair a late ptp4l start, a ptp4l restart and an external change to the settings without relying on a controller state transition.
8. Make shutdown bounded while giving the final NoSync update its own full 100 ms ptp4l operation deadline.
9. Make persistent failures quiet. Logs should describe changes in availability or operation outcome rather than repeat the same error on every reconciliation attempt.
10. Treat configuration of the ptp4l updater as requesting satpulse to own GRANDMASTER_SETTINGS_NP immediately. Management must not be gated on reaching InSync or receiving the first sample.
11. Let the controller decide when updates must pause around a leap transition. Updaters receive that policy in the desired properties and need neither reference timestamps nor their own leap-window calculations.

### Proposed design

#### Goroutine structure

There are two owners separated by a channel:

```
controller goroutine
    calculate GrandmasterProps
    publish latest value
             |
             v
    chan GrandmasterProps (capacity 1)
             |
             v
PTP4LUpdater goroutine
    own pmc.Client
    serialize GET and SET operations
    reconcile ptp4l with the latest value
```

The controller is the channel's only sender and closer; it may also receive solely to discard an obsolete queued value. Exactly one selected updater consumes values for application. The updater is started and joined by the daemon like the current worker. It does not stop directly on context cancellation because the controller publishes NoSync after cancellation as part of orderly shutdown; it stops after the update channel is closed and the final value has been processed.

The channel is the replaceable updater boundary. An updater for another PTP implementation takes the same `<-chan GrandmasterProps` but otherwise has its own dependencies, protocol and retry behavior. No updater code runs on the controller goroutine, and no protocol-specific result is sent back to the controller.

The diagram applies when a PTP updater is configured. Without one, the daemon creates no update channel or updater goroutine and passes a nil channel to the controller. Grandmaster publication and closure are no-ops for a nil channel, matching the current nil-`Grandmaster` behavior and preventing the latest-value loop from blocking on a nil channel.

#### Updater configuration

The daemon converts TOML configuration into inputs for the two owners:

- `ClockQuality()` derives the InSync clock quality from `ClockAccuracy`, `OffsetScaledLogVariance` and `AllanDeviation`. The daemon passes this to the controller for construction of `GrandmasterProps`.
- `Domain()` supplies the common PTP domain to the selected updater. The ptp4l updater also receives its implementation-specific UDS address.

The daemon creates no protocol client. A future updater can consume the same `GrandmasterProps` channel and `Domain` while using different implementation-specific configuration and update machinery.

#### Desired state

`GrandmasterProps` is the complete desired state sent across the channel. It contains the clock quality, leap-second state, traceability flags and a `CloseToLeap` policy field. Construction of a value is pure: given the configured in-sync clock quality, synchronization state, retained UTC offset, sample reference time and leap-second information, it produces one `GrandmasterProps` value. The reference time is an input to construction only; neither it nor a monotonic timestamp is passed in `GrandmasterProps`.

NoSync uses the current best-known numeric UTC offset because `currentUtcOffset` is present in the management TLV even when `CurrentUTCOffsetValid` is clear; zero is not used as an unknown sentinel. `CurrentUTCOffsetValid`, the leap flags, `TimeTraceable` and `FrequencyTraceable` are clear, clock quality is the normal NoSync quality, `PTPTimescale` is set, and the time source is GNSS. When the controller is initialized into Reset mode, it immediately publishes this value.

The daemon initializes the best-known state from the configured or default leap-second information and the build date returned as a string by `cmd.Version()`. It parses the build date with `time.Parse`, reduces it to a UTC calendar date, and evaluates the leap-second information at that date: a transition before the build date uses `UTCOffAfter`, while a transition after it uses `UTCOffBefore`. This uses the build date only as a trustworthy lower bound and never depends on the server's wall clock. If the build date was not compiled in or cannot be parsed, the calculation uses a fixed `time.Date` known to be after the default 2016 transition instead; absence of build metadata does not prevent startup and does not change the build date reported by `cmd.Version()`.

The controller retains the resulting numeric offset. InSync properties use `leapSecond.StateAt(lastSample.Ref)`, update the retained offset and set `CurrentUTCOffsetValid`. While InSync, the controller recomputes the value on the regular controller tick as well as after a leap-second update. It retains the last value it published and does nothing when the newly calculated value is equal, including `CloseToLeap` in the comparison. This catches time-dependent leap-state and quiet-window transitions without sending a value every 250 ms. Reset and Converging normally use the retained offset but clear the validity and leap flags, as phase 3 already does; the pre-leap offset fallback below is the exception near a transition and is new here.

There is no first-InSync gate. A configured updater means that satpulse owns the complete GRANDMASTER_SETTINGS_NP TLV from controller initialization, including while the clock never reaches InSync. The updater therefore restores the current NoSync target after a ptp4l restart just as it restores an InSync target.

There is no local `actual` state. A value returned by a successful SET says what that transaction applied; it is not evidence that the external server remains in that state indefinitely.

#### Controller leap-window policy

A private constant in `gm.go` defines a ten-second quiet interval around a leap transition: five seconds before and five seconds after, aligning with the agreed chrony policy. The constant is the five-second half-window. The common property construction computes `delta := sample.Ref.Sub(leapSecond.OffChangeTime)` using the controller's existing `ptime.Time` values. For an actual offset-changing leap, InSync properties carry `CloseToLeap = -1` when `-window <= delta < 0`, `CloseToLeap = +1` when `0 <= delta < window`, and zero otherwise. At the transition itself the value is `+1`. With no sample reference time or no offset-changing leap, the value is zero. This applies to both positive and negative leaps and to the configured transition, not every UTC midnight.

`CloseToLeap` is controller policy shared by all updater implementations. It is not a PTP time flag and is not encoded in GRANDMASTER_SETTINGS_NP. Updaters suppress GET and SET while it is nonzero, continue consuming and coalescing desired values, and resume reconciliation when it becomes zero. They do not extrapolate reference time, inspect the system wall clock, or calculate a separate deadline for leaving the window. A transition from `-1` to `+1` remains suppressed even though the desired UTC offset and leap flags change.

NoSync and shutdown must not wait for another sample to end suppression. If synchronization is lost or shutdown begins while close to the transition, the controller publishes `CloseToLeap = 0`, clears the leap flags and `CurrentUTCOffsetValid`, uses `leapSecond.UTCOffBefore` as the numeric UTC offset, and applies normal NoSync clock quality with both traceability flags clear. This fallback applies on either side of the transition, even if the last InSync value already carried the post-leap offset. `PTPTimescale` remains set and the time source remains GNSS. Outside this edge case, NoSync retains the best-known numeric offset as usual. NoSync always has `CloseToLeap = 0`.

#### Latest-value mailbox

The update channel has capacity one and represents the latest value not yet consumed by the updater. Publishing never waits. If the channel already contains a value, `gmUpdate` removes that obsolete value and then sends the new one:

```go
for {
	select {
	case updateCh <- props:
		return
	case <-updateCh:
	}
}
```

This is safe because the controller is the sole sender and closer, its receives only discard the value it is replacing, and the updater is the only receiver that applies values. If the updater concurrently receives the old value, the following send puts the new value in the now-empty channel. If it has already begun applying the old value, that operation cannot be cancelled, but the new value remains queued and is applied next. Channel capacity must remain one; the publishing operation and its assumptions should be kept in one helper and tested directly.

The updater also drains any immediately available value before starting an operation, so a value that has already been superseded is not needlessly applied. It checks the channel again after every blocking PMC operation. A new target takes precedence over the completed operation. Normally it is immediately SET; the leap-window policy below can instead suppress operations or require a fresh GET when leaving the window. A completed GET result is never reused to reconcile a superseding target.

#### PTP4L updater

`PTP4LUpdater` owns the current target, creation and lifetime of `pmc.Client`, the reconciliation timer and all operation outcomes. The daemon does not construct or access the client. The updater creates it when processing its first target, using its `Domain` and socket address, retains it across operations, and closes it before returning. Failure to create the local PMC client is an updater error with the normal 30-second error retry; absence of the remote ptp4l socket is instead detected by GET or SET and gets the 1-second missing-socket retry.

The updater performs only one PMC transaction at a time, which naturally preserves the requirement that a new request is not sent until the response or deadline for the preceding request has been handled. Before every GET or SET it checks the result of setting the existing 100 ms deadline. If setting the deadline fails, it does not start the operation, closes and discards that client, classifies the result as another transport error, and creates a new client on the next attempt. During shutdown, failure to create the client or set the deadline is failure of the one final NoSync attempt; it is logged and the updater exits without retrying.

On receipt of a new target, the updater drains the mailbox to get the newest target. If `CloseToLeap` is nonzero, it starts no GET or SET and suspends the reconciliation timer, continuing to receive targets. An operation already outstanding is allowed to finish within its existing 100 ms deadline; its result cannot trigger another operation while suppression is active. The updater checks for a new target and suppression before every transaction, including a conditional SET following GET.

When `CloseToLeap` returns to zero while still InSync, the updater immediately performs a fresh GET and conditionally SETs the newest target using exact settings equality. This also applies if several suppressed targets were coalesced. A NoSync target, including the final shutdown target, instead follows the immediate-SET path, so it needs no GET and never waits for another sample. Other new targets outside the quiet window immediately cause SET GRANDMASTER_SETTINGS_NP. A later target received while an operation is outstanding is processed immediately after that transaction finishes, subject to these same rules.

One timer drives GET GRANDMASTER_SETTINGS_NP whenever there is a target and leap-window suppression is inactive. The delay is selected from the outcome of the last operation:

- After a successful GET or SET, reconcile again after 5 seconds.
- After ENOENT or ECONNREFUSED, retry after 1 second. A missing or stale socket fails immediately, so this remains cheap even when ptp4l is not run for long periods.
- After a deadline or any other transport or protocol error, retry after 30 seconds.

There is no exponential backoff, retry count or accumulated retry state. Outside leap-window suppression and the fresh GET on resumption, a new target bypasses the timer and causes an immediate SET. After that SET, the timer is reset according to its outcome. When a GET finds a mismatch and is followed by SET, the SET outcome selects the next delay. The GET/SET status bits described under Logging do not affect this scheduling policy.

GET returns ptp4l's current settings without causing state-decision processing:

- If GET returns settings equal to the latest target, the updater does nothing.
- If GET returns different settings, the updater sends SET with the latest target.
- If GET or SET fails, no separate retry state is created. The next timer event is the retry and reconciliation attempt.
- A successful GET is used for conditional reconciliation only when no new target arrived while it was outstanding. If a new target arrived during GET or SET, the updater consumes the newest target and follows the new-target rules above, including suppression. Outside the quiet-window rules, it immediately SETs the new target even if the completed GET result already equals it.

This one mechanism covers all convergence cases. If the socket is absent when the initial SET is attempted, a later GET notices when ptp4l appears and SETs the target because ptp4l's defaults differ. If ptp4l restarts or another actor changes its settings, a GET finds the mismatch and repairs it. When ptp4l remains correct, reconciliation costs one GET every 5 seconds and causes no BMCA work.

The comparison is exact equality between the complete `pmc.GrandmasterSettings` returned by ptp4l and the settings derived from `GrandmasterProps`, since satpulse deliberately owns the whole GRANDMASTER_SETTINGS_NP TLV. There is no post-leap-successor exception: the controller's quiet-window policy avoids reconciling transient leap-transition disagreements. Outside suppression, even a premature post-leap successor is a mismatch and is corrected.

#### Shutdown

Controller shutdown forcibly publishes NoSync `GrandmasterProps`, replacing any older queued value, and then closes the channel. It uses the retained UTC offset normally, or `leapSecond.UTCOffBefore` when close to the transition as specified above. `CloseToLeap` is zero, the leap and traceability flags are clear, and `CurrentUTCOffsetValid` is false, so a paused updater immediately attempts the final SET without waiting for the quiet window to end or for another sample. This publication bypasses normal equality suppression even if NoSync was the last value published. The forced NoSync is an ordinary update; channel closure means only that there will be no more updates and never causes an operation by itself.

The updater applies the final NoSync target through the normal immediate-SET path. If draining the channel yields both a newest target and channel closure, it applies that target once and then exits. If it observes closure without receiving a new target, it exits without another SET. This covers both possible orderings: the updater may receive and start applying NoSync before the controller closes the channel, or it may receive the buffered NoSync after closure. Neither ordering duplicates the operation.

Once closure has been observed, the updater does not perform a GET or schedule another retry. The NoSync SET receives a newly established 100 ms deadline through the normal operation path, regardless of any preceding operation. If that SET fails or times out, the updater logs the outcome, closes the PMC client and exits.

If another operation is already in progress when shutdown begins, shutdown can take the remainder of that operation's 100 ms deadline plus the final SET's 100 ms deadline. It cannot wait indefinitely. The daemon wait group already waits for the updater goroutine, so the controller itself only needs to publish and close the channel.

#### Logging

The updater retains one status value with independent GET-OK and SET-OK bits, used only for logging. Each completed attempt updates only its own bit: success sets it and failure clears it. Initialize both bits as OK so the first failure of either operation is logged; this initial value means no failure has been recorded, not that an operation has been confirmed. A failure to create the client or set its deadline counts as failure of the operation being attempted.

Log a failure when an operation's bit changes from OK to failed, including the operation and its error. Log recovery when that same bit changes from failed to OK, naming the operation that recovered. GET success never clears a SET failure, and SET success never clears a GET failure. Repeated failures of an operation remain quiet even if the error class changes; error classification still determines the retry delay and the detail and severity of the first failure log. The bits are not reset by client recreation or by entering or leaving leap-window suppression.

In particular, a successful GET returning mismatched settings followed by SET permission failure clears only SET-OK. Further successful GETs and failed SETs leave the status unchanged and produce no repeated failure/recovery messages. A later successful SET restores SET-OK and logs its recovery. Successful periodic GETs are otherwise quiet. A successful SET caused by a desired-state change or observed mismatch can retain the existing update log with the applied properties. The status bits are neither proof of convergence nor a gate on future operations.

#### Code responsibilities

- `time/internal/ptpgm/gm.go` retains the common `Domain`, `GrandmasterProps`, synchronization/configuration types and pure property construction, including `CloseToLeap`, the private quiet-window constant and the NoSync pre-leap offset fallback. `Grandmaster`, `GrandmasterUpdateRequest`, response channels, `actual`, `lastAttempt`, and retry timing are removed. Conversion to `pmc.GrandmasterSettings` moves out of the common state path and into the ptp4l updater; it excludes `CloseToLeap`.
- `time/internal/phcsync/controller.go` stores the optional update channel, derived InSync clock quality, retained UTC offset and last published properties. `gmUpdate` calculates desired state from the sample reference time and leap-second information, deduplicates including `CloseToLeap`, then uses the latest-value publishing helper. Initialization publishes NoSync, `Tick` recomputes InSync leap state, proximity and the retained offset, and `Close` forcibly publishes NoSync and closes the channel. NoSync clears suppression and uses the pre-leap offset near the transition. Publication and closure return immediately when the channel is nil.
- `time/internal/ptpgm/ptp4l.go` makes `PTP4LUpdater` own GET/conditional-SET reconciliation, honoring `CloseToLeap`, outcome-dependent timer scheduling, deadlines, failure classification, GET/SET status-bit logging, client creation and client closure. It receives no reference or monotonic timestamps and computes no leap window. The production entry point binds the PMC operations; an unexported runner can accept fake client operations so the scheduling logic can be tested without network I/O.
- `time/app/daemon/config.go` retains `PTPConfig.Domain()` and `PTPConfig.ClockQuality()`.
- `time/app/daemon/daemon.go` creates the capacity-one channel only when an updater is configured, parses the build-date string from `cmd.Version()`, derives the initial UTC offset from the build date and configured leap-second information, and passes the offset, optional channel and derived InSync clock quality to the controller. It passes a receive-only view, `ptpgm.Domain` and the UDS address to the selected updater, and runs the updater in the daemon wait group. It does not own a PMC client.

The resulting concurrency invariants are:

1. Only the controller sends to, removes queued values from, or closes the update channel.
2. Only the updater accesses its protocol client, timer, current target and operation outcome.
3. At most one PTP management transaction is outstanding.
4. A queued value is always the newest value published by the controller.
5. The forced NoSync publication is attempted exactly once before the updater exits; channel closure itself never causes a SET.

### What ptp4l does (verified in ~/linuxptp, commit bed01a4, 2025-07-29; installed linuxptp 4.2-1)

- SET GRANDMASTER_SETTINGS_NP (`clock.c:693-701`) copies clockQuality, utc_offset, time_flags and time_source into the clock and marks the datasets changed, and ptp4l responds with the new settings (the SET response is effectively a GET result).
- Any management message that reports a change raises EV_STATE_DECISION_EVENT (`port.c:3326-3327`), which re-runs the state decision on all ports. When the best clock identity is unchanged, the reset branch in `handle_state_decision_event` is skipped and `port_state_update` makes no transition, but the state-decision processing still runs. This is why the design avoids unchanged SETs. Checked that far and no further.
- A GET of GRANDMASTER_SETTINGS_NP returns what was last SET (or the config defaults after a restart), not ptp4l.conf.
- When ptp4l is the grandmaster with `LEAP_61` or `LEAP_59` set, it applies the leap itself after the leap has passed: it clears the leap flag and applies the change to `utc_offset` (`clock.c:2200-2232`, via `util.c:488-514`). Satpulse independently makes the same change to its InSync target at `OffChangeTime`. The two transitions need not be simultaneous. The controller's `CloseToLeap` policy suppresses reconciliation around the transition, then the updater resumes with a fresh GET and exact comparison against the latest target. The controller's post-leap target is an expected desired-state update, not evidence of a restart or external modification.
- ptp4l closes and unlinks `uds_address` on exit, so satpulse sees ENOENT while ptp4l is down and ECONNREFUSED only if the file was left behind.

### Other decisions

- Keep overwriting the whole TLV as now. No read-merge of fields satpulse has no config for; the user considers the fixed values (clockClass 6/52, timeSource GNSS, ptpTimescale on) correct.
- Preserve the existing traceability behavior from commit `cdde170e`: timeTraceable and frequencyTraceable are both on when in sync and off when not. This is not new work in these phases and does not need another NEWS entry.
- Use a latest-value channel rather than an atomic pointer plus wake channel or a request/response interface.
- Configuring the updater gives satpulse ownership of GRANDMASTER_SETTINGS_NP immediately. NoSync carries the best-known numeric UTC offset while keeping `CurrentUTCOffsetValid` and the leap and traceability flags clear. The initial offset is derived from the configured leap-second information and build date, and a later NoSync value normally retains the offset established from samples. Near a leap transition, NoSync instead uses the pre-leap offset and clears `CloseToLeap`, allowing an immediate update even if no further samples will arrive. This replaces the historical first-InSync gate and must be included in the NEWS entry.
- The controller decides leap proximity using a private window constant in `gm.go` and publishes only `CloseToLeap`, with values -1, 0 and +1. No reference or monotonic timestamps cross the updater boundary. Suppress GET/SET while nonzero; resume with fresh GET/conditional-SET, except that NoSync and shutdown use an immediate SET. Remove the post-leap-successor equality exception.
- Reconcile ptp4l with periodic GET and conditional SET. Blind periodic SET is rejected because each SET causes ptp4l state-decision processing.
- Reconcile 5 seconds after success, retry a missing/refused socket after 1 second, and retry a deadline or other error after 30 seconds. Use fixed delays selected by outcome, with no backoff state.
- Keep one logging status value with separate GET-OK and SET-OK bits. Only the corresponding operation changes its bit, so a working GET cannot repeatedly mask a persistent SET permission failure.
- Keep the preceding `pmc.Client` late-response fix. It discards responses known to belong to requests that timed out, preventing a late datagram from poisoning later GET or SET transactions.

### Tests

The mailbox helper needs focused tests showing that publishing to an empty channel succeeds, publishing to a full channel replaces the old value, and a concurrent receive cannot cause the new value to be lost.

The updater scheduling and concurrency tests should run inside `testing/synctest.Test`. The updater goroutine, its channel and fake GET and SET operations are created inside the bubble. The fakes block on bubble-local channels, allowing the test to control completion and errors precisely. The bubble's fake clock can exercise the production 1-, 5- and 30-second intervals and 100 ms operation-duration cases without shortened test-only intervals or real sleeps; `synctest.Wait` establishes that all work triggered so far is complete.

Real unixgram sockets must not be used inside the bubble because network I/O is not durably blocking to `testing/synctest` and prevents automatic clock advancement. A small set of ordinary integration tests with a fake ptp4l unixgram endpoint should separately cover PMC message encoding, response handling and socket behavior. The bubbled tests cover updater policy:

1. Controller initialization causes an immediate SET of NoSync with the startup-derived numeric UTC offset marked invalid; transition to InSync outside the quiet window causes a SET with the sample-derived UTC offset marked valid.
2. Several targets published while an operation is blocked are coalesced, and the newest is applied next.
3. A periodic GET that matches the target causes no SET.
4. A periodic GET mismatch causes a SET of the latest target.
5. Outside leap-window suppression, a target arriving during GET takes precedence over the GET result and causes an immediate SET, including when the GET result already equals the new target.
6. A fake operation returning the missing-socket error and later returning settings converges on the next reconciliation.
7. A fake GET returning default settings after earlier convergence models a server restart and causes repair.
8. Success, missing/refused socket, and deadline/other error outcomes schedule the next operation after 5, 1, and 30 seconds respectively, with no progressive backoff.
9. GET-OK and SET-OK change independently within one status value. First failures and subsequent recoveries log once per operation; repeated failures remain quiet. Repeated successful GETs returning mismatched settings followed by SET permission failures must produce no repeated recovery/failure messages. Cover the reverse case too, where a successful SET does not clear an earlier GET failure, and verify that client recreation preserves the bits.
10. Closing with an older operation outstanding applies the forced NoSync with a separate deadline, then closes the client and exits. Tests must cover the updater receiving NoSync both before and after the controller closes the channel and verify that neither ordering causes a duplicate SET.
11. PMC client creation failure is retried as an updater error, while `SetDeadline` failure prevents the operation and causes the client to be recreated on the next attempt. Shutdown retries neither failure.
12. Nonzero `CloseToLeap` suppresses new GET and SET operations and suspends timer-driven reconciliation while newer targets continue to be consumed. An operation already outstanding finishes, but its result cannot cause a conditional SET after a suppressed target has arrived. Both -1 and +1 remain suppressed; a return to zero while InSync causes a fresh GET and conditional SET of the latest target. A NoSync value instead immediately SETs, including when the channel then closes.

Controller tests should verify initial NoSync publication with the startup-derived offset, equality suppression including `CloseToLeap`, recomputation of time-dependent leap state and proximity while InSync, replacement of a queued obsolete value, and forced NoSync publication followed by channel closure. A change in `CloseToLeap` alone must cause publication. Outside the leap window, NoSync must retain the best-known numeric offset while keeping `CurrentUTCOffsetValid` and the leap and traceability flags clear, including after an earlier InSync target and during shutdown. Construction, updates and shutdown with a nil update channel must complete without blocking or panicking.

Leap-transition tests should cover both positive and negative leaps, the exact quiet-window boundaries and transition instant, unavailable reference time, and leap information with no offset change. Verify `CloseToLeap` follows 0, -1, +1, 0 as sample reference time crosses the window, without relying on the system wall clock. Model ptp4l applying its leap independently: no new GET or SET occurs while suppressed, and resumption uses a fresh GET, leaves matching post-leap settings alone, and repairs any mismatch using the latest target. An exact post-leap successor observed before the quiet window must be treated as a mismatch, with no special acceptance rule.

For sample loss leading to NoSync and for shutdown, test both sides of the transition while suppression is active. The published fallback must have `CloseToLeap = 0`, `UTCOffset = leapSecond.UTCOffBefore`, `CurrentUTCOffsetValid` and leap/traceability flags clear, and normal NoSync clock quality, even if the preceding InSync value already used the post-leap offset. Verify immediate SET without a preceding GET, without another sample, and without waiting for the quiet window to end. Shutdown still allows only the outstanding operation's remaining deadline plus the final SET's own 100 ms deadline.

Daemon tests should cover build dates before and after a configured leap transition and the fixed fallback date used when build metadata is absent or invalid. The ordinary ptp4l integration tests should verify that `PTP4LUpdater` applies its domain values to management-message headers.

### Implementation sequence

1. Reduce `gm.go` to the common desired-state types and pure property construction, adding `CloseToLeap`, the private quiet-window constant and the NoSync pre-leap offset fallback.
2. Replace the controller's `Grandmaster` pointer with the capacity-one update channel and last-published value; implement latest-value publication including leap-proximity changes and final NoSync closure.
3. Pass `PTPConfig.Domain()` and the derived clock quality to their respective consumers.
4. Change `PTP4LUpdater` to consume the latest-target channel and add GET/conditional-SET reconciliation with exact settings equality, leap-window suppression and resumption, checked deadlines, outcome-dependent scheduling and independent GET/SET status-bit logging. Move client creation into the retrying run loop.
5. Update daemon construction and goroutine wiring, including derivation of the initial UTC offset from the build date.
6. Add the mailbox, controller and fake-ptp4l updater tests.
7. Add the NEWS entry for immediate GRANDMASTER_SETTINGS_NP ownership, then run the full test suite.

Idea from the user, separable from the fix: since a periodic GET is on the table anyway, read other datasets and report them (log transitions, `/metrics`, the SSE stream for the web UI). The library already decodes DEFAULT_DATA_SET, CURRENT_DATA_SET, PARENT_DATA_SET, TIME_PROPERTIES_DATA_SET and GRANDMASTER_SETTINGS_NP (`time/lib/pmc/mid.go`), which is enough to report whether this clock is the grandmaster (PARENT_DATA_SET grandmasterIdentity equals DEFAULT_DATA_SET clockIdentity) and what timePropertiesDS the network is being told. Port state and announce/sync counters would need PORT_DATA_SET and PORT_STATS_NP decoders.

## Related

- `plan/phc-holdover.md` and `plan/gap-mode.md` introduce ClockClassHoldover (7) for holdover mode. The current traceability behavior covers the existing two states; holdover will need its own answer.
- `plan/backlog.md`, "Grandmaster management": read current settings at startup.
- `time/internal/phcsync/grandmaster_test.go` covers phase 1 through the controller and PMC worker using a fake transport. `time/lib/pmc` has marshal and late-response tests. A fake ptp4l over a unixgram socket in the test's temp directory is feasible for testing real socket behavior, including late replies and a socket that appears after the first attempt.

## Testing on this machine

- Units: `satpulse@ttyUBX0.service` (config `/etc/satpulse.toml`, `[ptp] ptp4l.udsAddress = "/var/run/ptp4l"`, `majorSdoId = 1`, `clockAccuracy = 150`) and `ptp4l.service` (`/etc/linuxptp/ptp4l.conf`, gPTP profile, `transportSpecific 0x1`, `uds_address /var/run/ptp4l`). The other instance, `satpulse@ttySEP0.service`, has ptp4l commented out and does not touch ptp4l.
- Read ptp4l state without root via the read-only socket, using a scratch path for pmc's own socket:

```
pmc -u -b 0 -i /tmp/pmc.sock -s /var/run/ptp4lro -t 1 'GET GRANDMASTER_SETTINGS_NP' 'GET TIME_PROPERTIES_DATA_SET' 'GET PARENT_DATA_SET'
```

- Daemon log: `sudo journalctl -u satpulse@ttyUBX0`. The journal is not readable without sudo, so ask the user to run it.
- Clock log: `/var/log/satpulse/clock.enp4s0.log`, columns `date time offset freq outlier era sync` in UTC.
- Reproduce the observed case: restart `satpulse@ttyUBX0` while ptp4l is stopped, wait for tracking, then start ptp4l. Reproduce defect 1: restart ptp4l while satpulsed is tracking. Both need sudo.
- The full build is `make`; run `make test` before committing.
