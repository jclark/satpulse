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
