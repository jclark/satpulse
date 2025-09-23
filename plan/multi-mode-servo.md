# Multi-mode servo

This is a plan for improving the servo in SatPulse. The current servo is very simple.

One motivation for this is to take advantage of the [TimeHAT](https://www.tindie.com/products/timeappliances/timehat-i226-nic-with-pps-inout-for-rpi5/) and [TimeNIC](https://www.tindie.com/products/timeappliances/timenic-i226-pcie-nic-with-pps-inout-and-tcxo/),
in which the PHC uses a high-quality oscillator.
This opens the potential to add a holdover capability to SatPulse.

## Holdover

The essence of holdover is that you have a local oscillator separate from the GNSS module's oscillator,
which you can use to keep time when the GNSS module loses tracking.

There are a couple of different aspects to holdover:
A. how it affects the disciplining of the PHC
B. how it affects reporting of time metadata to the PTP server

From SatPulse's perspective, there are three approaches to doing holdover, depending on where the oscillator lives:
1. the oscillator can be upstream of SatPulse i.e. the pulses that SatPulse sees are using an oscillator to maintain time even when the GNSS isn't tracking; this happens when the input to SatPulse is a GNSSDO; in this case SatPulse doesn't need to do anything special on A, but it does need to do something special for B
2. the oscillator can be downstream of SatPulse i.e. the oscillator is part of the PHC; for this, we need to mainly focus on A
3. the oscillator can be independent: in this approach, there would be a separate PPS input from an oscillator on another SDP, which would not be aligned to UTC seconds, but which would be more stable than the computer's internal clock; this is what chrony does with its `local` directive

With TimeHAT and TimeNIC, approach 2 is being used.

In this document we are focusing on aspect A.
We will also need to address aspect B, but it is relatively straightforward compared to aspect A.

## Current servo design

The current servo has two phases:

### Startup
- Observe a few PPS samples to estimate phase and frequency.  
- If the current phase is off significantly, apply a single step to bring the PHC close to GNSS time.  
- On the next PPS, measure the residual offset, treat it as the set/get delay, and apply a second step to compensate.  
- Adjust the frequency based on the observed offset.  
- Transition into PI control.

### Phase correction
- Use a PI controller with fixed coefficients (currently Kp = 0.7, Ki = 0.3).  
- Continuously adjust the frequency to minimise the residual phase error.  
- There is no separate tracking mode — phase correction continues indefinitely once started.


## Limitations

There are several problems with the current design:

a) It doesn't distinguish between initial phase correction and tracking; but they are very different things. In initial phase correction, we are trying to align the phase of the PHC with the GNSS. This cannot be done by a step alone: that will only get you to a few microseconds accuracy. The PTP server won't be ready to serve clients until after initial phase correction has been done. This means initial phase correction can and should adjust the frequency quite aggressively. In tracking mode, PTP clients are using the server and we should be gently adjusting the frequency just enough to keep things in sync.  This is a separate problem from holdover.

b) When the PPS stops, then no samples are generated and the servo does nothing. But the frequency at any given time is chosen to nudge the phase back into alignment. If that frequency is held too long, it will nudge too far. If the PPS stops, we want instead to switch to the frequency which will best maintain the clock's time. This problem is made worse by (a): our phase correction is more aggressive than appropriate for tracking which means the nudges are stronger. If we fixed (a) the nudges would be more gentle.

c) When the PPS comes back after stopping, the servo continues as normal. This isn't ideal because if it's been a long time, we may want to step. In addition, the level of aggressiveness we want here is somewhat between initial phase correction and tracking.
Depending on how long the PPS has stopped, clients may still be using us.


## Multi-mode servo design

To fix these limitations, the idea is to switch over to a design that recognizes multiple modes, which can be switched between.

### Startup
- **Goal:** Measure initial phase and frequency, then place the PHC close to GNSS time.  
- **Method:**  
  - Observe a few PPS samples without correcting; estimate phase error and frequency offset.  
  - Apply a single time step to remove most of the offset.  
  - On the next PPS, measure the residual; treat it as the set/get delay and immediately apply a second step by that amount to pre-compensate future sets.  
  - Keep the frequency estimate for seeding the PI controller in later modes.

### Initial phase correction
- **Goal:** Reduce the residual phase error after startup.  
- **Method:**  
  - Use a PI controller with large Kp and small Ki.  
  - Initialise the integral state with the frequency estimate from startup so the output is continuous.

### Tracking
- **Goal:** Maintain alignment without over-reacting to GNSS jitter.  
- **Method:**  
  - Use a PI controller with small Kp and small Ki.  
  - Initialise the integral state with the frequency estimate from startup.  
  - Keep short-term and long-term frequency estimates updated in the background.

### Missed PPS sample (micro holdover)
- **Goal:** Avoid overshoot when a single PPS sample is missing or rejected.  
- **Method:**  
  - Keep the PI controller active but do not update its integral for that tick.  
  - Output the short-term frequency estimate for that tick.  
  - Resume normal tracking on the next valid PPS.

### Holdover
- **Goal:** Keep PHC close to true time when PPS is unavailable or unreliable.  
- **Method:**  
  - Do not use a PI controller.  
  - Start with the short-term frequency estimate and blend towards the long-term estimate as holdover lengthens.

### Recovery
- **Goal:** Return from holdover to normal tracking smoothly.  
- **Method:**  
  - Use a PI controller with medium Kp and medium Ki.  
  - Initialise the integral state with the frequency estimate used at the end of holdover (the short/long blend at exit).  
  - If the residual phase error is small, walk back without a step; if it is large due to a long outage, apply a single step and then continue with the controller.

## Testing

The goal is to allow reasonably, realistic testing of the servo without hardware, and running mmuch faster than realtime.
The compoenents for this are as follows.

### 1. PHC oscillator simulator

Simulate a hardware oscillator as a function that maps from a float64 representing simulated true time from start of the simulation to a float64 representing the fractional frequency error at that true time.
Meaning if the return value is +1e-6, the clock runs fast by 1 µs per second of true time; if it’s -1e-6, it runs slow by 1 µs per second.
This function can be built-up out of building blocks of functions of the same type,
modelling specific sources of error like noise or drift.


### 2. GNSS PPS simulator

Simulate the PPS as a function that maps from a float64 representing simulated true time to the fractional *phase* error at that true time.

### 3. Virtual clock  

This uses the PHC oscillator simulator

Implement frequency adjustments and phase steps in software, without modifying the oscillator, similar to how virtual clocks in the Linux PHC subsytem are implemented on top of free running clocks.

### 4. Servo testing harness

This is parameteried by two functions: the PHC oscillator simulator and the GNSS PPS simulator.
The servo acts on the virtual clock.

We can use this to simulate both normal operation of the GNSS and situations where the GNSS loses its lock.