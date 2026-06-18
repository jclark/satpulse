# The verification ladder

Verification climbs four rungs. Each rung catches what the one below
cannot; the top rung (gpshwtest) caught the only two real
post-implementation bugs in the CASIC work, both invisible to the
rungs below because they encode the implementor's own assumptions.

## 1. Offline tests through the real path

A fake receiver driven through the real PacketProcessor ->
ConfigProtocol -> ConfigDirector pipeline (see
gps/internal/casic/cascfg_test.go and the ubx/unc equivalents). Rules
learned:

- The fake must mimic DISCOVERED hardware behavior, not assumed
  behavior: every quirk found on hardware (ack-without-apply, mask
  clamping, per-port poll responses, silent receivers, late probe
  NAKs, baud switches) gets encoded in the fake plus a regression
  test. A fake that only reflects the design verifies nothing.
- Write the failing test FIRST when fixing a bug found on hardware;
  a fix whose test never failed proves nothing (an exclusion-window
  bug was reproduced offline this way before the fix).
- Round-trip tests for every new wire struct in the protocol library;
  audit coverage against the registration list, not memory.
- Table-driven, per the repo's go-unit-test skill.

## 2. Replay tests with committed traces

Record real configuration sessions against hardware, commit the traces,
and replay them offline through the real ConfigDirector so the
configurator's wire output is pinned against genuine receiver responses.
This is the committed evidence the definition of done requires; unlike
rung 1's hand-built fake, the receiver side here is captured, not
assumed. Framework lives in internal/gpscmd/testdata/ and
internal/gpscmd/replay_test.go.

Generating traces:
- A receiver stub (e.g. testdata/at632.sh) sets device, speed, vendor,
  and the scenario-start reset (reset_args; default --reload, overridden
  where the receiver cannot reload - V6 CASIC uses --factory-reset).
- Command files (testdata/cmd/<area>.sh) each exercise one option area
  with a sequence of `t` invocations (pvt-out, signal, pps, tmode, ...).
- run.sh resets to a clean baseline, then runs the command file under
  --test-log, writing one committed trace per scenario to
  testdata/<vendor>/<receiver>-<area>.jsonl.

The test (internal/gpscmd/replay_<proto>_test.go):
- Lists the scenario files; TestReplay<PROTO> replays each via
  testReplayFile, feeding the recorded receiver responses through the
  real PacketProcessor -> ConfigProtocol -> ConfigDirector path and
  asserting the configurator's OUTGOING packets match what was recorded.
- The compare func does exact byte comparison of sent packets
  (casicPacketsEqual / uncPacketsEqual).

Rules learned:
- The test pins only what the configurator SENDS. Receiver telemetry
  (timestamps, position, sats) varies on every recapture; it is input,
  not asserted. A regenerated trace must leave the outgoing packets
  byte-identical - if they move, the configurator changed, not the dice.
- Start each scenario from a known baseline, so the reset must actually
  reset. Confirm on hardware that the chosen reset works: a reset that is
  ACKed but silently ignored yields dirty, non-reproducible traces. (V6
  CASIC --reload is exactly this trap - ACKed, no effect.)
- One scenario per option area keeps a failing replay localized.

## 3. gpshwtest characterization

The gpshwtest framework (in the repo, on the main branch) probes the
tool's device-independent guarantees against the real receiver and emits
a characterization.
Contract: FAILURES are tool-guarantee violations only; receiver
limitations are data; runs must be run-to-run identical; receivers are
left as found. Process:

- Run until clean, then commit the baseline
  (gpshwtest/baselines/<identity>.json) and a per-receiver note
  (gpshwtest/HW/<receiver>.md) describing limitations relative to the
  full model. Baselines for a new configurator belong on the
  CONFIGURATOR's branch (owner ruling).
- Rerun for byte-identical output before trusting a baseline.
- When the framework reports a failure, suspect the tool first: both
  CASIC failures it found were real configurator bugs (a misreported
  achieved signal set; the save/speed ordering). It also caught a
  readback rounding bug that masqueraded as a receiver limitation -
  when a "limitation" looks odd, check our own conversion first.
- Disruptive coverage (--disruptive: speed, NVM persistence) is
  required coverage too, gated; it must include recovery (rediscover
  a receiver whose speed changed; restore sane NVM).

## 4. Observation, not enablement

For message output, an ACKed enable is not delivery; only observing
emission is. Event-shaped messages (leap announcements, survey steps)
may legitimately never appear in an observation window - record that
as characterization, not failure. Plan watch durations around the
protocol's own schedules (GPS subframe data: 12.5 min).

## Evidence standards

Before claiming verification, state what observation would differ if
the claim were false; if nothing would, the test discriminates nothing
(see rulings.md, "Evidence must discriminate"). Report results
faithfully: failed runs with output, skipped checks as skipped, weak
oracles as weak. Restore receivers after every hardware session and
verify the restoration (sample the line; check the state read back).
The as-found state is not always reachable through the property
interface - an auto/default mode can alias an explicit set, and writing
a value can couple in others (CASIC V6 NAVBAND); when restore cannot
reproduce the initial readback, record that rather than claiming a clean
restore.
