---
title: SatPulse 0.2 PHC synchronization architecture
---

One of the major changes in SatPulse 0.2 is a new architecture for the PHC synchronization subsystem.
The PHC synchronization subsystem has two inputs: a stream of timestamps from the PHC and a stream of messages from the GPS receiver.
Its primary function is to synchronize the time of the PHC with the GPS receiver's time.
It also needs to update the PTP daemon with current clock quality.

## 0.1 PHC synchronization architecture

My initial implementation of PHC synchronization followed the approach of the `ts2phc` program,
included in LinuxPTP.
The approach consists of a 2-stage pipeline.
The initial stage generates samples, which give the offset between the PHC and GNSS time, by combining timestamps from the PHC with time-of-day information from GPS messages.
The samples are fed into a second stage, which uses a PI servo to adjust the phase and frequency of the PHC.

This approach evolved to add a monitoring stage to the pipeline between the sample-generation stage and the servo.
This monitoring stage had a variety of responsibilities.
It determined whether the PHC was in sync with GNSS time, and used this to dynamically update
the PTP daemon's clock quality.
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
and that meant developing a simulator.

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

Points to cover:
* modal architecture
* say it was developed using the simulator
