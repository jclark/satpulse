# NAME

satpulsetool-sdp - manage software-defined pins on PTP hardware clocks

# SYNOPSIS

**satpulsetool** [*global options*] **sdp** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-i**\|**\-\-extts**] [**\-o**\|**\-\-perout**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-disable**] [**\-\-show**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-t**\|**\-\-timeout** *seconds*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-w**\|**\-\-width** *seconds*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-period** *seconds*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-p**\|**\-\-pin** *index*\|*name*] [**\-\-chan** *index*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-show\-stale**] [**\-j**\|**\-\-jsonl**]\
&nbsp;&nbsp;&nbsp;&nbsp;[*interface*]

# DESCRIPTION

The **satpulsetool** **sdp** command manages software-defined pins (SDP) on PTP hardware clocks (PHC).
A pin can be configured for input, allowing timestamps of pulses received by the pin to be read,
for example when the pin is connected the the PPS output of a GNSS receiver.
This is called external timestamping (extts).
A pin can alternatively be configured for output, allowing periodic pulses aligned to the PHC
to be generated, for example when the pin is connected to an oscilloscope.
This is called periodic output (perout).

The mode of operation is determined by the **\-\-extts**, **\-\-perout**, **\-\-disable** and **\-\-show** options.
At most one of these options can be specified;
if none of them are specified, the command behaves as if **\-\-show** was specified.
In **\-\-show** mode, the **interface** is optional.
In other modes, it is required.
If the **interface** is specified, then root privileges are required.

# OPTIONS

**\-h**, **\-\-help**  
: Show usage help for the **sdp** command.

**\-i**, **\-\-extts**  
: Check input on a pin.
This configures a pin of the interface as an input pin and reads timestamp events from the PHC device.
It will show each timestamp as it is received.
It will read timestamp events for the length of time specified by the **\-\-t** option; default is 2 seconds.
if this is 0, it will read until interrupted.
The pin to be used is specified with the **\-\-pin** option; the default is pin 0.
The **\-\-chan** option can also be used to specify the timestamping channel to be used; the default is channel 0.

**\-o**, **\-\-perout**  
: Enable periodic output on a pin.
This configures a pin of the interface as an output pin generating periodic pulses.
The period can be specified with **\-\-period**; it defaults to 1 second, resulting in a PPS signal.
Some Ethernet drivers allow the width of the pulses to be specified using the **\-\-width** option.
If **\-\-width** is not specified, the pulse width is chosen by the ethernet driver.
For Intel igb and igc drivers, the width cannnot be specified, and is always half the period,
i.e. a duty cycle of 50%.
The pin to be used is specified with the **\-\-pin** option; the default is pin 0.
The **\-\-chan** option can also be used to specify the periodic output channel to be used; the default is channel 0.

**\-\-disable**  
: Disable the pin function.
This sets the specified pin's function to none, effectively disabling any input or output operations on that pin.
The pin to disable is specified with the **\-\-pin** option; the default is pin 0.

**\-\-show**  
: Show information about SDPs.
If the *interface* argument is specified, shows information about the SDPs of that interface.
Otherwise, shows information about all interfaces with a PHC that has SDPs.

**\-t**, **\-\-timeout** *seconds*  
: Length of time in seconds for which to read timestamp events. 
Applies only when **\-\-extts** options is specified.
The default is 2.

**\-w**, **\-\-width** *seconds*  
: Specifies the pulse width for periodic output in seconds.
Ethernet drivers are typically limited in what pulse widths they support.
Some drivers, such as the Intel igb and igc, do not support specifying the width at all.
Must be greater than 0 and less than the period.
Supports exponential notation (e.g., 1e-1 for 100ms, 1e-6 for 1 microsecond).
Applies only when **\-\-perout** option is specified.

**\-\-period** *seconds*  
: Set the pulse period for periodic output in seconds.
The default is 1.0 second, resulting in a PPS signal.
A period of 0 disables the output signal.
Supports exponential notation (e.g., 1e-1 for 100ms, 1e-3 for 1ms).
Applies only when **\-\-perout** option is specified.

**\-p**, **\-\-pin** *index*\|*name*
: Select the pin to be used by **\-\-extts**, **\-\-perout**, or **\-\-disable**. The default is 0.

**\-\-chan** *index*  
: Select the channel to be used by **\-\-extts** or **\-\-perout**. The default is 0.

**\-\-show\-stale**  
: Include stale timestamps in the output. By default, timestamps that were buffered before reading timestamp events started are not displayed. Applies only when **\-\-extts** options is specified.

**\-j**, **\-\-jsonl**  
: Output in JSON lines format instead of human-readable text.

# EXAMPLES

List all interfaces with PTP hardware clock that has SDPs:

    satpulsetool sdp

Show SDP configuration for interface enp4s0:

    satpulsetool sdp enp4s0

Check input on eth0, showing timestamp events received for 2 seconds, with pin defaulting to pin 0:

    satpulsetool sdp -i eth0

Output a PPS signal on eth0, with pin defaulting to pin 0:

    satpulsetool sdp -o eth0

Check input on pin 1 of eth0, showing timestamp events received for 30 seconds in JSON lines format:

    satpulsetool sdp -i -j --timeout 30 -p 1 eth0

Output a PPS signal on pin 1 of eth0:

    satpulsetool sdp -o -p 1 eth0

Disable periodic output on eth0:

    satpulsetool sdp -o --period 0 eth0

Disable pin 1 on eth0, making it neither an input pin nor an output pin:

    satpulsetool sdp --disable --pin 1 eth0

# EXIT STATUS

**0**  
: Success

**1**  
: Error

**2**  
: No data found: no interfaces found to show, or no timestamps received with **\-\-extts**

# SEE ALSO

**satpulsetool(1)**, **satpulsed(8)**, **ptp4l(8)**, **ts2phc(8)**