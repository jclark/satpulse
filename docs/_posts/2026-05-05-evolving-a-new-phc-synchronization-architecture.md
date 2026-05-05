---
title: Evolving a new PHC synchronization architecture for SatPulse 0.2
date: 2026-05-05 11:30:00 +0700
---

One of the major changes in SatPulse 0.2 is a new architecture for the PHC synchronization subsystem.
The PHC synchronization subsystem has two inputs: a stream of timestamps from the PHC and a stream of messages from the GPS receiver.
Its primary function is to synchronize the time of the PHC with the GPS receiver's time.
This involves generating a stream of *samples* from the two streams of timestamps and messages.
A sample says what the offset is between PHC time and GNSS time.
These samples are then used to synchronize the PHC and to update the PTP grandmaster with the synchronization status.

Generating samples includes the following tasks:
* pulse edge filtering: some Intel NICs generate timestamps for both edges of a pulse; in this case, we have to identify which edges are leading edges
* sample completion: the timestamp for a leading edge marks the top of a second; completing the sample means determining which second that is
* sawtooth correction: a GPS receiver can only generate a pulse on an edge of its internal clock, but there can be an offset between the edge of its internal clock and the top of the second; a timing-grade GPS receiver will output a message for each pulse giving the size of this offset; the sample then needs to be adjusted for this offset

## 0.1 PHC synchronization architecture

My initial implementation of PHC synchronization followed the approach of the `ts2phc` program,
included in LinuxPTP.
The approach consists of a 2-stage pipeline.
The initial stage generates samples by combining timestamps from the PHC with time-of-day information from GPS messages.
The samples are fed into a second stage, which uses a PI servo to adjust the phase and frequency of the PHC.

This approach evolved to add a monitoring stage to the pipeline between the sample-generation stage and the servo.
This monitoring stage had a variety of responsibilities.
It determined whether the PHC was in sync with GNSS time, and used this to dynamically update
the PTP grandmaster's clock quality.
It also performed outlier detection using a MAD algorithm.

I found two major problems with this pipeline approach.
The first problem was that each stage in the pipeline ended up
maintaining its own state, but these states were not coordinated.

- the sample-generation stage had an initialization state for analyzing the intervals between edges;
this was used with Intel NICs that timestamp both edges of a pulse to ensure that trailing edges were ignored
- the monitoring stage maintained state of whether the PHC was synchronized to GNSS time
- the servo stage maintained state related to deciding whether to step the PHC

This became particularly problematic for the sample-generation stage.
It's important for PHC synchronization to be as reliable as possible.
I found that GPS messages were not completely reliable for determining time-of-day information.
Perhaps the most common problem is that the GPS emits too many messages for the available serial bandwidth,
which causes messages to be delayed or dropped.
When in a synchronized state, a more reliable way is to use the PHC, since the PHC will be accurate to within a microsecond or so,
but this doesn't work at all when the PHC is not synchronized.
However, the sample-generation stage doesn't have access to the synchronization state of downstream stages.
The sample-generation stage became increasingly complex over time, using ad hoc heuristics to decide whether to prefer
information from the PHC or from messages.

This ties into the second main problem. I had very limited ability to test the pipeline as a whole.
My main approach was to save the inputs and outputs of the sample-generation stage;
I could then replay the inputs to make sure they produced the same outputs.
But if the sample-generation stage was affected by the monitoring stage, this would no longer be possible.

The first problem meant that a rewrite was needed: I hadn't decomposed the problem in the best way.
And if I was going to do a rewrite, then I should solve the second problem once and for all,
and that meant I needed a simulator.

The most significant open source project in the timing simulation space that I know of is Miroslav Lichvar's [clknetsim](https://gitlab.com/chrony/clknetsim).
In fact, this project is what made me realise that serious testing needed a simulator.
However, clknetsim would not work for SatPulse, because it relies on being able to redirect system calls using LD_PRELOAD,
but SatPulse is written in Go and on Linux Go usually produces statically linked executables; system calls are raw kernel syscalls, which do not
go through a dynamic library. So that meant I needed to develop my own simulator.
Also clknetsim does not handle the GPS side of things.

## Simulator

The goal of the simulator is not to be perfect, but to be realistic enough to enable closed-loop testing of synchronization algorithms.

The simulator is initialized with a configuration and performs a simulation for some period of time.
The simulator is driven by the progress of simulated time, which represents true time.
As simulated time progresses, the simulator emits timestamps and GPS messages.
It also implements a PHC interface that can be used to adjust the phase and frequency of the simulated PHC.
There is a crucial feedback loop:
each timestamp is measured with respect to the PHC and has to take account of any phase and frequency adjustments made through the PHC interface.
Another complicating factor is that GPS messages can include sawtooth corrections for the PPS signal
and these corrections have to match the timestamps being generated.

When run under a simulator, the code under test produces its normal output,
but the simulator can observe the offsets between the simulated true time and the simulated PHC.
It can produce a log of these offsets and also generate statistics such as the maximum offset and the Allan deviation.
These statistics could only be produced in real-world testing by using a reference clock that tracks UTC with much greater accuracy than a GPS PPS signal. This would require expensive hardware such as a caesium clock or better still, a hydrogen maser; a rubidium clock would not be sufficient.

The configuration includes error models for the PHC oscillator and the GPS PPS signal.
Each error model consists of a number of components that describe different sources of error, which are combined additively.
For example, the PHC error model has components for white, flicker and random walk FM noise.
The GPS PPS error model includes a component for sawtooth error,
which is used in generating both the timestamp for the PPS edge and the sawtooth correction in the corresponding GPS message.

In Go, the error models are represented by `func(t float64) float64`:
the return value gives the instantaneous error at simulated time t.
The return value for the PHC error model is a frequency error,
whereas the return value for the GPS PPS error model is a phase error.
This reflects the underlying physical reality that an oscillator is a continuous process whose state at any instant is a rate, whereas a PPS signal is a discrete process whose state for each pulse is a position in time.

The error models can be derived from physical measurements made of the PHC oscillator and the GPS PPS signal.
(A PHC oscillator can be measured by making the PHC output a PPS signal while free-running.)
In a future post, I will go into more detail about how I made measurements and used them to derive error models.

## 0.2 PHC synchronization architecture

The approach in 0.2 is modal. There are three modes: reset, converging and tracking.
At a high level, these modes work as follows.
Reset is the initial mode: its job is to generate a single, reliable sample; it does this by collecting a batch of timestamps and GPS messages.
After generating the sample, reset mode will perform a step of the PHC so as to guarantee that the PHC is close to the GNSS time.
At that point, it transitions to converging mode. Its job is to aggressively adjust the frequency so as to bring the PHC into as precise as possible alignment with GNSS time.
When the offsets between the PHC and timestamps are no longer decreasing, it transitions to tracking mode.
Its job is to continually tweak the PHC frequency so as to keep the offsets as small as possible.
It remains in tracking mode so long as the offsets indicate that the PHC is still synchronized to GNSS time.
If synchronization is lost, it transitions to reset mode.

Each mode is associated with a clock quality notified to the PTP grandmaster:
tracking mode is associated with a clock quality representing a synchronized state;
reset and converging mode are associated with a clock quality representing an unsynchronized state.

The following table summarizes the operation of the modes.

| Task | Reset | Converging | Tracking |
| --- | --- | --- | --- |
| Pulse edge filtering | analysis of batch of pulse edges | parity from leading edge learned in reset | alignment of edge to top of second |
| Sample completion | alignment of batch of timestamps and messages | round timestamp to nearest second | round timestamp to nearest second |
| Sawtooth correction | not applied | not applied | applied |
| Outlier detection | validation of batch of timestamps and messages | none |  MAD-based  |
| PHC control | step when leaving mode | aggressive PI servo | gentle PI servo |
| Successful exit | valid sample | offsets stabilize | none |
| Failure exit | none | too many missing samples | too many bad samples |

The key point to notice in the table is that each mode performs its tasks very differently.
I want to focus particularly on sample completion, which is the most fundamental part of sample generation.
In production, SatPulse should be spending 99.9999% of its time in tracking mode,
and in tracking mode sample completion is utterly trivial:
the PHC clock will be accurate to a microsecond, so you can just round the timestamp to know which second the timestamp is for.

In contrast, sample completion in reset mode is quite elaborate.
If reset mode makes the wrong choice of second, then that will persist throughout the operation of the daemon,
so the implementation takes a lot of care to ensure that it is right.
It collects time messages and timestamps for several seconds,
and then performs multiple consistency and quality checks.

Reset mode has to correlate time messages with timestamps.
For time messages, we record the monotonic time at which we read the first character of the message.
But these monotonic times cannot be compared directly with the timestamps, which are in the PHC time domain.
The natural way to handle this is to record the monotonic time immediately after the timestamp is read.
But the Raspberry Pi CM4/CM5 ethernet PHY driver has a quirk which makes this insufficient by itself:
the driver can deliver the timestamp to user space up to 0.25s after the pulse occurred.
To handle this, we also record the PHC time immediately after reading the timestamp,
and then adjust the post-read monotonic time by the difference between the post-read PHC time and the timestamp.
We also have to account for the possibility that the PHC is fast or slow.
The average interval in PHC time between successive pulses tells us how much PHC time corresponds to one second,
and we use this to scale the PHC difference before using it to adjust the monotonic time.

The decomposition of responsibilities is as follows.
The main implementation package is `phcsync`.
It has a controller, which is responsible for orchestrating the modes.
For each mode, there is a sample-generator and a sample-processor.
The sample-generator is responsible for pulse edge filtering, sample completion and sawtooth correction;
in tracking mode it uses the pulse width discovered in reset mode.
The sample-processor is responsible for outlier detection,
and for determining when and how to adjust the PHC and change mode;
these PHC adjustments and mode changes are then performed by the controller.
The sample-processors for converging and tracking mode share a PI servo implementation;
in tracking mode, the servo is initialized using the PHC frequency error discovered in reset mode.
The controller feeds samples from the sample-generator to the sample-processor.
The controller also synthesizes missing samples and feeds them to the sample-processor.
The controller notifies the PTP grandmaster for mode changes that imply a change in clock quality.
Thus, as in 0.1, there is a 3-stage pipeline: sample-generator then sample-processor then controller.
But the pipeline is driven by the controller, and the sample-generator and sample-processor
are mode-specific.

The other implementation package is `timemsg`,
which serves as a bridge between `phcsync` and the GPS subsystem.
`timemsg` maintains a buffer of recent time-related messages from the GPS receiver.
`phcsync` defines an interface which captures what it needs to know about time messages,
and `timemsg` implements this.
Reset mode obviously depends on this interface,
but tracking mode also uses it for sawtooth corrections.
This separation between `phcsync` and `timemsg` was also designed to enable `timemsg`
to be reused for a new feature in 0.2: samples can be provided to an NTP server
based on serial timing, without needing a PHC.

The benefits in terms of user-visible features of the 0.2 implementation are relatively modest.
Reset mode can disambiguate leading and trailing edges even with a 50% duty cycle.
PHC synchronization parameters are now fully configurable using a new `[sync]` section of the config file.
The simulator has a CLI that can be used to tune these parameters, in particular the Kp/Ki constants for the tracking servo.

But the major wins from the new architecture are in terms of improving reliability and providing a foundation for future development.
The most important missing feature at the moment is holdover:
the modal architecture can accommodate this in a natural way.
Sample generation has a clean and principled architecture that solves the problems this had in 0.1;
in tracking mode, it does not depend on time messages and so should be more reliable.
The most important aspect of the architecture is, I believe, the simulator.
This solves the testability problem we had in 0.1 and improves the reliability of 0.2.
But it is also crucial for future development: without a simulator,
it would be very difficult to develop a reliable implementation of complex features like holdover.
