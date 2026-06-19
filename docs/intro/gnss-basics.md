---
title: GNSS basics
---

<!--
Outline only - to be written up into prose.

Framing question: what is GNSS, and what can a computer do with it?
Audience: a competent computer person who is new to GNSS.

Spine (the arc in one breath): out of the box a computer gets a rough position and
time, and the point - SatPulse, timing, positioning - is doing far better; this page
is the groundwork: what GNSS is (constellations) -> the receiver you plug in -> how it
computes position and time -> how you read that (messages, then the precise pulse) ->
and the ionosphere, the dominant error, and why multi-band beats it.

Deliberate forward references that pay off on the page:
- "four in view" (What GNSS is) is answered in "What it computes"
- the ionosphere flagged in "What it computes" is answered in "The ionosphere, and why bands help"

Deferred to other pages - keep out of basics:
- constellation differences: system time scales -> timing; satellite-broadcast PPP
  (HAS / B2b / MADOCA) and message authentication (OSNMA / QZNMA) -> positioning / integrity
- timing mode, sawtooth / quantization error -> timing
- RTK, PPP, carrier phase, raw-observation output -> positioning
- precise-time depth (hardware timestamping, the PHC) -> timing
- which specific module / board / receiver to buy -> hardware section
-->

## Intro and scope

- The question: what is GNSS, and what can a computer do with it?
- Survey answer: plug in a receiver and a computer gets a rough position (meters) and a rough time.
- The point - and what SatPulse is for - is doing far better: centimeters, nanoseconds.
- This page is the groundwork; the doing-better is the timing and positioning pages.

## What GNSS is: constellations

- Terminology: GPS is the US system, one of several; GNSS is the umbrella term; "GPS" is used loosely for any of them.
- A constellation is a collection of satellites orbiting Earth in a coordinated way.
- For GNSS that means medium Earth orbit (MEO), arranged so there are always at least four in view (why four: see "What it computes"), which works out to at least about 24 satellites per constellation for global coverage.
- The global four: GPS (US), Galileo (EU), BeiDou (China), GLONASS (Russia). Regional ones - QZSS (Japan), NavIC (India) - augment coverage in their area.
- Modern receivers track several constellations at once: more satellites, better coverage.

## The bridge: a receiver you plug in

- What it is physically: a chip (the silicon) packaged into a module, mounted on a board, sometimes in its own enclosure. "Receiver" names the role, not any one of those forms.
- It plugs into the computer over USB or serial; antenna in, power.
- Which specific one to buy is the hardware section's job.

A GNSS board is built around a *module*.
Most of its capabilities come from the module it uses.
A module is a small, thin, rectangular, metal‑shielded component with solder pads underneath, typically around 1–3 cm across and a few millimeters thick;
it is built around a GNSS chip, which is the silicon that does the actual GNSS processing,
and integrates other components such as flash memory and an oscillator.
Designing a GNSS board around a module is relatively straightforward.
Designing a module around a bare chip is significantly harder.
Designing a GNSS chip is orders of magnitude more difficult and expensive; only a few companies do it.
Some vendors make both modules and chips; others make modules only or chips only.

A GNSS module cannot be connected to computer directly. It needs first to be integrated into a board.
The board provides connections for antenna input, PPS output, serial IO and power.
The board may be designed to go inside a computer's case, or it may have its own separate enclosure.


## What it computes: position and time, together

- Each satellite broadcasts where it is and when it transmitted (the navigation message).
- The receiver measures the signal's travel time, giving a range to each satellite (a pseudorange).
- From four or more satellites it solves at once for four unknowns: its 3D position and its own clock offset. (This is why you need four in view.)
- Position and time fall out of the same measurement - why one cheap device is both a position sensor and a clock, and why this site covers both.
- This uses the code (pseudoranges) and gets you to meters; carrier phase, the precise technique, is the positioning page's story.
- The measurement is not perfect; the biggest error is the ionosphere (see "The ionosphere, and why bands help").

## How the computer reads it: messages

- The receiver and computer talk in messages over the serial link. At this level everything is a message: position, velocity, time, corrections, raw data, configuration.
- NMEA is the standard protocol: it gives you the solution, and is enough for basic use.
- Vendor-specific protocols are richer, and are the only way to configure the receiver or reach its advanced features (there is no standard configuration protocol).
- Time comes this way too, but only roughly.

## The PPS pulse

- The time in a message is coarse: the receiver emits a burst of messages each second, and at serial speeds that burst takes a good fraction of a second (0.2-0.3 s) to arrive, and not by a fixed amount - so the time can be a third of a second late.
- So the receiver also provides one thing that is not a message: a precise electrical pulse, once per second, on its own pin.
- Division of labor: the pulse marks the instant precisely, the message labels which second it is - together, precise time.
- Making that precision real - hardware timestamping, the PHC - is the timing page.

## The ionosphere, and why bands help

- The pseudorange is corrupted by signal delays; the dominant and least predictable one is the ionosphere.
- Signals sit in bands (L1, L2, L5, L6), and the ionosphere delays different bands by different amounts.
- A receiver using two or more bands measures that difference and cancels most of the ionospheric error - the single biggest lever on basic accuracy.
- Single-band, dual-band, triple-band, all-band.
