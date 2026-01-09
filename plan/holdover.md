# Holdover

## Flavours of holdover

The essence of holdover is that you have a local oscillator separate from the GNSS module's oscillator,
which you can use to keep time when the GNSS module lost its lock.

In terms of implementation, there would be two additional modes: holdover and reconverge.

The holdover mode is when the GNSS module has lost its lock and is no longer aligned to GNSS time. This corresponds to the PTP grandmaster state with a clock class of holdover. The absence of PPS is one way to detect this. Another way is by time messages that say the time is invalid. We would stay in holdover mode for a configurable length of time, but typically this would be short enough that the clock is guaranteed to be off by no more than microseconds. If this time is exceeded without GNSS regaining its lock, we would go into lost mode. But if we get the lock back, then we go into reconverge mode. This is similar to converging mode. However, and important difference is that the grandmaster would be in a holdover state, and so still potentially serving clients. When the phase has reconverged, we would enter tracking mode again. If in reconverge, we lost PPS we would go back to holdover.

The details of the implementation depend on where the oscillator lives. I can see three approaches to this.

I can see three approaches to holdover based on where this oscillator lives.

### Holdover using PHC oscillator

With this approach, the oscillator is part of the PHC. Examples of suitable hardware are the [TimeHAT](https://www.tindie.com/products/timeappliances/timehat-i226-nic-with-pps-inout-for-rpi5/) and [TimeNIC](https://www.tindie.com/products/timeappliances/timenic-i226-pcie-nic-with-pps-inout-and-tcxo/),
which include a high-quality oscillator.

During tracking mode, we would extend the tracking of frequency to also estimate frequency over a longer time period. In holdover mode, we would adjust the frequency to be a blend of the short and long-term frequency estimates appropriately blended. More generally, during tracking we build a model of how the frequency changes, and then apply that model during holdover.

During reconverge, we use a servo similar to converge in order to converge the phase again. But the Kp/Ki coefficients would be designed to keep the adjustment of frequency sufficiently gentle that it does not negatively affect clients. This is different from the situation with converging mode, where the goal is to converge to the correct phase as rapidly as possible. 

### Holdover using GNSSDO

With this approach, SatPulse gets pulses from the oscillator instead of from the GNSS directly.
There is a GNSSDO that is using the GNSS to discipline the oscillator upstream of SatPulse. Suitable hardware would be the BG7TBL CM55.

In this case, we would need to identify when the GNSS receiver as lost or regained tracking using time messages rather than pulses. The configurable length of time to stay in holdover would apply as usual. In reconverge mode, we would probably want different Kp/Ki coefficients so we track the GNSSDO's reconvergence closely. (But not the same Kp/Ki as with holdover using PHC.)

The challenge here is that we don't know how long it will take after the signal is restored for it to reconverged.

### Holdover using an independent oscillator

In this approach, a PPS signal from an oscillator would be connected to an SDP on the PHC, which would be in addition to the PPS signal from the GNSS.
This second PPS signal would have a stable frequency but would not be aligned to UTC seconds.
This is what chrony does with its `local` directive.

Suitable hardware would be the Leo Bodnar LBE-1421.
This has two SMA outputs.
One can be configured to produce a PPS signal that is passed through from the GNSS.
The other can be configured to produce a 1Hz (or greater frequency ) 50% duty signal that is disciplined but not phase aligned.
Both these signals would need to be fed into a card with two SDPs (e.g. an Intel card).

During tracking, we would want to have a servo that uses both the GNSS PPS for phase correction and the frequency pulse for frequency correction. During holdover, we would do just frequency correction using the frequency pulse. We can detect mode transition by absence of the GNSS PPS.

## Implementation

### Modes and advertised PTP clock quality

Each PHC sync mode determines what the PTP grandmaster advertises:

- `tracking`: advertise the normal in-sync `ClockQuality` (configured today via `ptp.*`).
- `holdover`: advertise a holdover `ClockQuality` (clock class `ClockClassHoldover`).
- `reconverging`: advertise the same holdover `ClockQuality` as `holdover`.
- `reset`/`converging`: advertise non-synced/degraded `ClockQuality` (current behavior).

(`holdover` and `reconverging` differ only in servo behavior; they intentionally do not differ in advertised PTP quality.)

### State transitions (PHC holdover + GPSDO holdover)

The intended state machine is:

- `tracking → holdover`: triggered by too many missing samples (a missing-signal problem, not an outlier problem).
- `holdover → reconverging`: first present (non-missing) sample after a holdover period.
- `reconverging → holdover`: any missing sample while reconverging (flapping is acceptable because PTP advertised quality does not change).
- `reconverging → tracking`: reconvergence criteria satisfied (like `converging` but tuned to be gentle to clients; still advertising holdover while reconverging).
- `holdover|reconverging → reset`: total time spent in `holdover + reconverging` exceeds the holdover timer.

Outliers do not trigger entry into holdover. Holdover is specifically the response to missing reference, not “reference present but bad”.

### Holdover timer semantics

The holdover timer starts from the last *good* tracking sample:

- `holdoverStart` is the time of the last `tracking` sample that was accepted and fed to the tracking servo (i.e. a “good” sample, not missing/outlier).
- The timer is not reset when moving between `holdover` and `reconverging`; it measures total elapsed time since the last good tracking sample.
- If the timer expires while in either `holdover` or `reconverging`, transition to `reset`.
- When transitioning back to `tracking`, clear the holdover timer state.

### PHC-based holdover vs GPSDO-based holdover

We will support two implementational variants selected by a `gps.disciplined` boolean:

- **PHC-based holdover (`gps.disciplined=false`)**
  - “Missing sample” is the usual PHC/PPS notion: we cannot form a sample because pulses/messages are absent (or otherwise missing).
  - `holdover` servo behavior: run on the PHC holdover model (frequency estimate built during tracking) while missing.
  - `reconverging` servo behavior: run a gentle phase-converging servo (distinct gains from `converging`) while still advertising holdover.

- **GPSDO-based holdover (`gps.disciplined=true`)**
  - The PPS may continue even when GNSS lock is lost, so “missing sample” is defined by *loss of time lock*, not by missing pulses.
  - A `TimeMsg` signals loss of lock by having both `UTCTime == nil` and `TAITime == 0`.
  - `TimeMsgBuffer` will expose a lock query API (separate from `GetPostTimeMessages`), implemented roughly as “does the most recent navigation-solution (`NavSolution`) `TimeMsg` indicate a valid time (and is it not stale)?”
  - `holdover` servo behavior: continue tracking the disciplined PPS using holdover-appropriate `Kp/Ki`.
  - `reconverging` servo behavior: use distinct `Kp/Ki` tuned to follow the GPSDO’s relock behavior while still advertising holdover.

The controller will create the appropriate `sampleGenerator` and `sampleProcessor` implementations for `holdover`/`reconverging` based on `gps.disciplined`.
