---
title: High-level configuration
sitemap: false
---

SatPulse has a high-level, device-independent model for configuring GPS receivers.
You express what you want in GNSS terms,
such as which signals to enable, how the receiver should determine time and position, and what it should output.
SatPulse translates that into the receiver's own protocol.
Currently the model is implemented for u-blox receivers (from the u-blox 6 platform through to the X20 platform)
and Unicore Nebulas IV receivers (UM980, UM981, UM982, UM960).

TODO: link to the full semantics specification when it is published in the Internals section.

## Three kinds of things

A configuration request is built from three kinds of things:

* properties: named pieces of receiver configuration state,
  such as the enabled signals, the time pulse shape, the timing mode with its fixed position,
  the antenna cable delay, and the serial speed;
* message output: what the receiver should emit;
* actions: things the receiver does once,
  such as surveying its position, saving configuration to non-volatile memory, and resetting.

The distinction matters because what you can expect differs.
Properties can be read back, to see what the receiver stores.
Message output and actions have no readback; you observe their effects:
requested messages flow, a survey completes, a saved configuration survives a power cycle.

## One big request

You can express your intent for the receiver as one big request, combining all three kinds:
enable these signals, use this fixed position, output these messages, change the serial speed, and save.
Commissioning a receiver is naturally one big request,
and the Workbench Configuration tab and the `satpulsetool gps` command line are both shaped around expressing one.
You do not have to work this way; a request can be as small as reading back one property.
But the guarantees are stated at the level of the whole request.
In particular, when one part of a request fails, the rest still runs,
but a requested save does not happen:
the receiver's saved configuration only ever advances from a request in which everything succeeded.

## Configuration groups

Your request does not have to express everything.
The configuration divides into independent groups of related properties.
For example, the time pulse is one group, which includes the pulse width and the pulse period;
the set of enabled signals is another;
each kind of message output is a group of its own.
A group that a request says nothing about is left alone.
You can change the enabled signals without affecting message output, and the other way round.

Within a group, however, a request is complete: it says everything about that group.
The enabled signals are requested as the complete set,
and constellations and signals not named end up disabled.
A message output request names everything wanted from its group,
and the rest of the group is turned off.
This is what keeps requests self-contained:
what you get in a group is determined by your request alone,
not by what the receiver happened to be doing before.

The groups are drawn so that things that typically interact belong to the same group.
PVT message output is one group because protocols combine PVT information into messages in different ways;
to choose the right messages for a receiver, SatPulse needs to know all the wanted PVT information at once.
Satellite message output does not enter into that choice, so it is a group of its own.

## Requests are best effort

You ask in device-independent terms, and the receiver does the closest thing it supports.
Values are quantized to the receiver's resolution and limited to its ranges.
The response reports what was achieved, which is what the receiver accepted.
A difference between what you requested and what was achieved is not an error;
it tells you something about your receiver.

You also do not need to know in advance what your receiver supports.
Asking for something the receiver does not have is not a failure:
it is simply absent from the response.
The Workbench shows the same thing up front, by greying out what the connected receiver cannot do.

## Two kinds of message output

Output carried by the receiver's own protocol is requested as information:
position, velocity, and time; the time of the time pulse; leap second information;
per-satellite and per-signal status; raw observations and navigation data.
Which native messages deliver the information is SatPulse's business;
the guarantee is that what you asked for is delivered.
A native message often carries several kinds of information,
so more may arrive than you asked for; that is normal and harmless.

NMEA and RTCM output is different: there you request the standardized messages themselves,
such as RMC, GSV, and MSM4.
Downstream consumers of NMEA and RTCM parse exactly these message formats,
so controlling exactly which ones appear is the point.

## Nothing persists unless you save

Configuration requests change the receiver's running configuration.
The changes are volatile: power cycling the receiver restores its saved configuration.
Making changes survive a power cycle is a separate, explicit action:
save, either minimally (at least what this request changed, and as little else as possible)
or everything.
In the other direction there is a ladder of resets.
A reload restores the saved configuration.
A reset additionally discards the receiver's knowledge of time, position, and satellite orbits.
A factory reset first restores the saved configuration itself to factory defaults.

## satpulsed, satpulsetool, and the Workbench

The model is directly exposed in two places:

* the `satpulsetool gps` command, where one command line expresses one request
  (see [satpulsetool-gps(1)]({% link man/satpulsetool-gps.1.md %}));
* the Configuration tab in SatPulse Workbench, which is a graphical view of the model:
  you edit the configuration as a form and apply it as one request
  (see [satpulsewb(1)]({% link man/satpulsewb.1.md %})).

Both expose the full model, including saves and resets.
They are for commissioning: one-time setup done with a human present, watching the responses.
You set the speed, choose the constellations and signals, survey or set the fixed position, and save,
so that the receiver's saved configuration is the baseline that production starts from.

satpulsed is for production: it runs unattended,
and it uses the model differently, by assembling a request itself.
With `config = true`, it derives the receiver configuration that the rest of `satpulse.toml` needs,
and applies the request each time it starts.
Most importantly, it enables the message output whose information satpulsed itself is going to use.
The `[gps]` table supplies what cannot be derived, such as a fixed position
(see [satpulse.toml(5)]({% link man/satpulse.toml.5.md %})).
The request satpulsed assembles is deliberately a non-disruptive subset of the model:
changes that can be made on the fly without disrupting the ongoing operation of the receiver.
It never saves, never resets, does not change the serial speed,
and does not change the enabled constellations and signals
(which typically need a reset to take effect).
This is also what makes it safe to try:
whatever satpulsed changes, a power cycle restores your receiver exactly as it was.
