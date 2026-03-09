# Time sync rewrite

Inputs are:

* configuration from satpulse.toml (at startup)  
* timestamp events from PHC extts subsystem  
* abstract messages from GNSS subsystem, representing  
  * current time in various ways  
  * leap second information

Outputs are:

* disciplining the PHC: both stepping and adjusting frequency  
* generating samples to send to chrony  
* sending status updates to PTP grandmaster  
* providing observability of the PHC synchronization process

Covers existing combine, mon, servo packages. Currently work as follows:

* combine combines time messages and pulses to generate samples, which it sends to mon  
* mon does  
  * outlier detection  
  * keeps track of missing samples  
  * determines appropriate PTP state  
  * sends good samples to servo  
  * gets back frequency changes from servo  
  * generates samples for chrony when in sync  
* servo  
  * uses samples to drive a PI to adjust the PHC  
  * tells mon how it adjusted

Problems:

* each of combine, mon and servo have their own states, but these states are internal, and not synchronized across packages  
* combine package is far too complex  
* all of three packages are not configurable; not way for user to tune them  
* mon tries to use offsets between PHC and GNSS to judge accuracy of PHC, but I believe this is fundamentally misconceived  
  * if you suddenly see a 100ns offset between PHC and GNSS, it doesn’t mean your PHC is wrong by 100ns; much more likely that the GNSS has a transient spike  
* servo at startup and servo when tracking use same Kp/Ki parameters, but different parameters would be appropriate  
* generally poor testability  
* cannot handle 50% duty cycle when both edges of a pulse are timestamped
* no support for holdover


Rewrite will introduce a new `phcsync` package. Key idea is to be modal. There will be a mode that applies through all stages:

- sample generation
- PTP mode changes
- servo mode
- chrony sample generation

There will also be a helper package `timemsg`.

We will add support holdover in a second stage after getting everything else working.

## Modes

Starting set of modes

* init
* converging
* track
* lost

These are in order, except that lost can go back to converging.

### init mode

* initialization
* no knowledge of PHC time or GNSS time
* before transitioning out of this mode, PHC is accurate to the nearest second
* performs a step at the end unless PHC is already close
* assume no modification in PHC frequency or phase

Generate samples by gathering 3 - 5 seconds worth of pulses

* determine PHC time correspond to 1 GNSS second  
  * with single edge, PHC time between pulses should be constant  
  * with two edges, PHC time between alternating pulses should be constant  
  * within the max phase adjust of 1 second  
* Should also determine speed of system clock relative to PHC  
  * take two SYS_OFFSET readings of PHC  
* For pulses we have  
  * timestamp PHC time  
  * post-reading  
    * wallclock + monotonic system time  
    * PHC time  
* Estimate monotonic time of pulse  
  * Compute delta_phc = PHC time after reading - PHC time of timestamp  
  * Scale to a delta in system clock time to get delta_mono  
  * monotonic pulse time = Monotonic system time after reading - delta_mono  
* Try to figure out pulse width  
  * may fail if 50% duty cycle  
* If NTP is synchronised, estimate wall clock time of pulses  
  * scale delta_phc by 1/1_PHC_second  
  * subtract from wallclock time of post-pulse reading  
  * check that wallclock time is second aligned  
* Generate array of time messages  
  * all nav messages (linked to nav solution)  
  * all same native message type  
  * where multiple possibilities  
    * prefer native TAI  
    * prefer not NMEA  
    * prefer linked to configured time GNSS  
* Attempt to align time message array with pulse array  
  * pulses and time messages should be aligned  
    * if we haven’t resolved edge yet, then every other edge aligned   
  * require reasonably consistent delay (configurable parameter)  
  * use delay to resolve edge ambiguity  
* Result is an array of samples (when successful)  
* Exit when we have sequence of samples  
  * where pulses are consistent  
  * where time messages are consistent

Array of samples is used to

* generate step using median of sample offsets
* estimate frequency to start PI for tracking mode
* preferred nav message for use in future
* when both edges are being timestamped, estimate pulse width or validate configured estimate

### converging mode

* start with phase nearly accurate (far better than nearest second)
* Use alternation rising/falling to ignore falling edges using pulse width from init mode
* Don't bother with pulse correction
* Find right second using rounding
* Exit mode when median absolute offset over last N (e.g. 5) samples hasn't decreased for some number of pulses (i.e., the offset reduction has plateaued)
* Use PI to converge (start with current Kp/Ki)
* PTP mode is still not in sync
* Aggressive Kp/Ki with high Kp
* Need to think about how we handle missing samples in converging mode
  * one missed sample can be ignored
  * if PPS stops for multiple seconds (unlikely), go back to init mode

#### Additional features

**ADJ_SETOFFSET delay compensation**

When stepping the PHC time using ADJ_SETOFFSET (done in init mode), the kernel implementation in ethernet drivers performs a read-modify-write operation: it reads the current PHC time, adds the requested offset, and writes the new value back. This sequence takes time (typically a few microseconds with an i210 NIC), but the drivers don't compensate for this delay. As a result, the clock step is slightly inaccurate - the PHC ends up a few microseconds behind the target time.

Note: The assumption that the offset is primarily due to ADJ_SETOFFSET delay is not entirely accurate, since the observed offset could also include a small component due to frequency error. Although we correct the frequency just before stepping the clock, there is a brief interval between the PPS timestamp and the frequency correction during which the PHC was running at the wrong frequency. However, this secondary effect is small compared to the ADJ_SETOFFSET delay.


The converging mode can be initialized with a flag indicating whether it should compensate for a clock step that was just performed.
If the flag is set:
- if the first pulse is received: measure the offset between the PHC and GPS time and perform a second clock step to correct for this delay;
- if the first pulse is missed: skip the compensation entirely and proceed with normal PI control.

### tracking mode

* PTP mode is in sync
* Initialize PI controller integral term with frequency estimate from init phase
* Distinguish rising/falling using pulse width from init mode
  * primarily by position relative to top of second
  * plus possibly distance from previous edge
* Different Kp/Ki, lower, less aggressive
* Do outlier detection
  * MAD with configurable sigma
  * also maybe min absolute value to be considered outlier
* sample rejected is not sent to servo
* keep track of missed pulses
* Do pulse correction
  * for M8F will need to wait for pulse correction following pulse
* Keep count samples that are abnormal either because missing or outlier (abnormal means not sent to servo) Exit when:
  * number of consecutive abnormal samples greater than configurable value, or
  * proportion of abnormal samples in a configurable window is greater than configurable value
* Don't try to estimate accuracy of PHC using PHC/GNSS offset

#### Additional features

* Adjust frequency for missing sample  
  at any given time, frequency is normally designed to nudge the PHC back into phase  
  * keep track of average frequency over last N seconds  
  * when we miss a sample, set frequency back to average frequency  
  * but don’t change PI constants

### lost mode

* PTP mode is out of sync
* very similar to init mode, maybe the same
* try to generate a sample sequence using same method as init mode
* should not need to reestimate pulse width
* when we have a set of samples,
  * step only if the offset is too much (different parameter here from init mode)
  * enter converging mode

## Implementation components

### phcsync.Controller

General points:

* PHC-specific  
* Should not depend on GPS interface (in gpsprot)  
* Does do logging

Constructed with

* Clock - interface for accessing PHC clock  
  * can be mocked for testing  
  * alias for servo.Clock for now  
  * implemented by ts.Clock  
* TimeMsgBuffer - interface for accessing time messages  
  * does not expose gpsprot types  
  * tuned to what phcsync needs  
* Sampler - for observability, reports samples  
  * alias for mon.Sampler  
* Grandmaster
  * alias for mon.Grandmaster for now
  * for reporting changes in mode
* ProxyRecClock
  * for reporting samples to chrony
* Config  
  * with tunable values  
* LeapSecond  
* slog.Logger

Called with by event dispatcher:

* edge timestamp event  
* notification that time message occurred  
* leap second info  
* regular tick (0.25s)

Delegate work as follows:

* sampleGenerator  
  * called with pulse edge event  
  * called with notification that time message occurred (but not with message)  
    * maybe classify by pulseCorrection  
  * has reference to timemsg.Buffer  
  * generates samples from pulse edges  
  * when both edges timestamped responsible for ignoring falling edge  
  * does not filter samples (e.g. for outliers)  
  * in tracking, converging, at most one sample per edge  
  *  handles pulse correction (sawtooth)  
* sampleProcessor
  * Gets missing samples and actual samples
  * Decides whether to send sample to servo
  * Determines whether to change mode
  * Tells controller what adjustment to make to the clock

On time pulse event:

* call sampleGenerator with pulse
* get sample/samples from sampleGenerator
* send to sampleProcessor
* Get mode change and phc adjustment
* Apply PHC adjustment
* Send to sampler for observability
* Apply mode change

On tick event:
* when not in init phase, generate missing sample if more than a second since last sample

On mode change, notify grandmaster of change of mode.

In tracking mode, use ProxyRefClock to generate samples for chrony. Do this before sampleProcessor filters out samples. Chrony does it's own filtering.

Files:

* [controller](http://controller.go).go for Controller and related interfaces  
* [tracking.go](http://tracking.go) for implementations related

### timemsg.Buffer

Called with:

* time message event  
* leap second message event  
* implements phcsync.TimeMsgBuffer

This has nothing PHC-related in it.
Can potentially be used to generate serial timing samples for chrony.
Keeps last n seconds of messages (N supplied to constructor)

### logobs package

Provides Observer implementations for logging and monitoring, implementing the obs.Observer interface:

* **ClockLogObserver** - writes structured clock log file
  * Records: timestamp, offset, frequency, outlier flag, era, sync state
  * Same format as current monitor clock log
  * Configurable file path and rotation

* **StatsLogObserver** - logs summary statistics to slog
  * Periodically computes and logs: mean/RMS phase offset, frequency stddev
  * Replaces current monitor's stats logging
  * Configurable logging interval

Both observers are registered with the MultiObserver and receive samples from the Controller via its Sampler interface. This keeps logging concerns separated from control logic and consistent with the existing observability framework (like promobs and sseobs).

## Testing

Key idea is to have a clock simulator. 

The components for this are as follows.

### 1. PHC oscillator simulator

Simulate a hardware oscillator as a function that maps from a float64 representing simulated true time from start of the simulation to a float64 representing the fractional *frequency* error at that true time.
Meaning if the return value is +1e-6, the clock runs fast by 1 µs per second of true time; if it’s -1e-6, it runs slow by 1 µs per second.
This function can be built-up out of building blocks of functions of the same type,
modelling specific sources of error like noise or drift.

### 2. GNSS PPS simulator

Simulate the PPS as a function that maps from a float64 representing simulated true time to the fractional *phase* error at that true time.

### 3. Raw clock

Uses PHC oscillator simulator to represent a free-running PHC clock (i.e. a clock that cannot be adjusted)

### 4. Virtual clock

This is layered on top of a raw clock. It simulates a PHC.

Implement frequency adjustments and phase steps in software, without modifying the oscillator, similar to how virtual clocks in the Linux PHC subsytem are implemented on top of free running clocks.

It also generates timestamps using the GNSS PPS simulator (which are timestamped w.r.t the virtual clock).

This needs to be enhanced to be able optionally to generate timestamps for both edges of a pulse to simulate Intel PHCs.

### 5. TestClock

This is layered on Virtual Clock and implements the servo.Clock interface.

It implements the userspace era concept used by servo.Clock. The era deals with the problem that when you step the PHC, you don't know whether a timestamp is before or after the step.

### 6. Testing harness

This uses the TestClock to test the current servo implementation.

This would be different for phcsync, but the servo implementation can serve as a basis.


## Generate chrony samples without a PHC

It would be useful to be able to work with chrony when there is no PHC.

Chrony has the ability to read Linux PPS events itself but these say only when a pulse edge occurred and not what second it is, so the main thing that is needed is to generate an chrony samples with an approximate time based on time messages. This is issue [#77](https://github.com/jclark/satpulse/issues/77). This would make satpulse viable as an alternative to gpsd.

This could be handled by the timemsg package, which is explicitly designed to be PHC-independent.

## Staging


### Phase A - preparatory (done)

1. Implement clock simulator. This in `internal/clocksim` package.
2. Factor out logging from mon into logobs package.

### Phase B - minimal end-to-end (done)

Eventually combine, mon, phc will go away. But initially we will make new code run in parallel with old code.
To handle this interfaces that will eventually be in phcsync will remain in their existing packages, with aliases in phcsync.

Make this work end to end, but do not implement all features yet. Specifically, do **not** implement:
* Grandmaster settings
* chrony samples
* MAD-based outlier detection (for now, just have single value above which considered an outlier)
* ignoring falling edges (when both edges are timestamped)
* setting tuneable parameters via TOML file
* lost mode (make lost mode do nothing - i.e. it stays unsynchronized)
* sawtooth correction (pulse correction)

Steps to implement.

1. Implement timemsg
2. Start on phcsync.Controller
   * Design public method signatures
   * Empty bodies for now
   * Call from dispatcher.go
   * Don't do anything with Grandmaster, ProxyRefClock
3. Design internal interfaces for sampleGenerator, sampleProcessor, including how phcsync.Controller calls these
4. Factor out PI servo from existing servo.go to use as basis for servo implementation for converging and tracking mode; use same Kp/Ki for now
5. Implement these interfaces for each mode
6. Implement a test harness using clocksim to test this

### Phase C - essential MVP features (done)

Implementing each of these will involved enhancements to simulator to test properly.

1. handle ignoring falling edges, when both edges timestamped (requires enhancements to clocksim), but not 50% duty cycle (or close to it)
2. enter lost mode when pulses stop (transition from tracking to lost based on consecutive missing samples)
3. implement recovery in lost mode (transition from lost back to converging when pulses return)

### Phase D - integrate into daemon (done)

#### Phase D.1 - essentials
* Integrate this into the daemon, so that it is used instead of combine/mon/servo. (done)
* Factor out mon/gm.go into its own package and fix all references; remove aliases in phcsync
* Factor out mon/refclock.go into its own package and fix all references; remove aliases in phcsync
* Replace aliases in phcsync by definitions
* Implement grandmaster settings in phcsync.Controller
* Implement chrony refclock in phcsync.Controller

#### Phase D.2 - decouple from old packages
* Update SampleData.SyncState to use our new mode
* Post-read PHC/system time has wallclock time; compute separate monotonic time
* Port event log replay architecture (internal/gpsevent/replay.go) to use phcsync.Controller instead of combine.Combiner
* Check for no dependencies on combine/mon/servo packages

### Phase E - refinements

Done:
* compensation step at beginning of converging phase 
* MAD-based outlier detection
  * simulate ionospheric disturbances 
  * simulate outliers
* First pass on improving naming and design of tuneable parameters
* Implement setting of tuneable parameters via new section in satpulse.toml
* Validation pf phcsync.Config
* more logging during initialization of what we discovered
* sawtooth correction from PrePulse events (e.g. UBX-TIM-TP)
  * PrePulse messages
  * Needs difficult changes to clocksim and syncsim
  * PostPulse events (may require waiting)
* support 50% duty cycle with both edges

Also done:
* exit tracking when proportion of abnormal samples in a configurable window is greater than configurable value (BadSampleRatioLimit + BadSampleWindow)
* pulseWidth is now auto-detected; config field deprecated

Remaining issues:
* Add phcsync Config parameters to JSON schema (#224)
* Estimate error in system clock when converting between PHC and system clock time domains (#184)

Nits/polish (no issues):
* Validation of limits in Config parameters relative to each other
* PulseWidthTolerance won't work well with very short pulses
* better error messages when timemsg buffered messages are too old

### Phase F - gather better data

In ../clock-model repo. Use Rubidium oscillator with TAPR TICC to gather better data for clock model.

### Phase G - finish up

* G.2 Monitor time messages during tracking (#182), basis for serial-only chrony samples (#77)

### Phase H - holdover

#199 (PHC-based), #152 (GPSDO-based)

### Phase I - Upgrade observability to expose full sync mode

#177
   - Or add mode as a label to existing metrics
   - Keep the existing `satpulse_phc_sync_status` gauge for backward compatibility
   - This allows monitoring systems to distinguish between converging and tracking states
