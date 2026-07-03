# ConfigDirector contract: working knowledge

The authoritative contract is the doc comments in
`gps/gpsprot/configprotocol.go` (substantially improved on the
unicore-speed branch). This file records the working knowledge that
mattered during the CASIC implementation and is not all stated there
yet; candidates for migrating into the doc comments.

## The shape of a configurator

- The director drives everything through `Actions()` (send request /
  wait until / error). The configurator is time-agnostic: it learns
  time only via `AdvanceTimeTo` and `SetSentTime`.
- Requests may be generated lazily, in waves: `GenerateRequests` is
  called repeatedly, and new requests may be added at any point -
  including fallback requests created when an earlier request is
  NAKed. Phase-gating on "all earlier requests final" is a clean way
  to sequence read-modify-write (poll before set) and to keep a save
  after everything it must persist.
- The configurator decides which requests are sendable
  (ReadyToSend vs NotReady); the policy should be derived from the
  protocol's response-correlation granularity (see
  protocol-questions.md). Promotion on every GenerateRequests call
  works well.

## Retries and failure

- The director retries a request when its deadline passes
  (MayResend); `SetWontResend` is the give-up notification. A request
  may treat give-up as success when silence is an acceptable outcome
  (optional text queries).
- Keep per-attempt response windows short (seconds) and let patience
  come from the director's retry budget; a late ACK to attempt N
  matches the retried attempt N+1 of the same request. Responses
  outside the window must be dropped, or a stale ACK from a previous
  send confirms the wrong attempt.
- A NAK is data, not necessarily failure: the request decides
  (nakOK / fallback / hard failure). Report refusals of genuine value
  requests as errors; show nonexistent features as absence.

## Speed changes

- `GetSpeedChangeAfter` makes the director switch the host rate right
  after sending that request; everything sent later goes at the new
  rate. Speed-change requests are NEVER retried - an unconfirmed one
  fails at its deadline.
- `MaybeSpeedChangeSucceeded(t)` is called for every valid packet; the
  request may confirm itself from packets after an exclusion delay
  (pre-switch packets can still be buffered). This heuristic path must
  not be the only one: follow the change with solicited traffic at the
  new rate (a poll or a repeated command) whose answer confirms the
  change directly and immediately. unc's staged-deadline repeat
  pattern (re-send the identical command as a no-op; its ACK confirms
  the original by request order) is the most robust known form.
- Requests that expect no response of their own but may absorb one
  (the repeat) use the MaybeComplete state entered at send time.

## No-response requests

Resets and some reloads restart the receiver without acknowledging
(CASIC V6 reload, CFG-RST). Such requests succeed when sent
(`SetSentTime` -> succeeded). Expect boot banners and a quiet period
afterwards.

## Probing interplay

Probe packets are sent before Configure; their responses (including
NAKs that ARE the detection signal) can arrive after configuration
begins, especially on saturated lines. The ConfigProtocol must count
outstanding probes and consume their acknowledgements so the
configurator never attributes them to its own requests (the
correlation key is shared).

Check whether that bookkeeping is needed at all before building it:
it exists only where the probe's correlation key collides with
configurator requests. The Allystar probe is a poll that gets no ACK
and answers with a message the configurator never requests, so late
probe responses can collide with nothing and the pending-probe
accounting disappears entirely.
