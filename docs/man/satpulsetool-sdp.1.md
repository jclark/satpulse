# NAME

satpulsetool-sdp - manage software-defined pins on PTP hardware clocks

# SYNOPSIS

**satpulsetool** [*global options*] **sdp** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-show**] [**\-i**\|**\-\-extts**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-t**\|**\-\-timeout** *seconds*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-pin** *index*\|*name*] [**\-\-chan** *index*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-show\-stale**] [**\-j**\|**\-\-jsonl**]\
&nbsp;&nbsp;&nbsp;&nbsp;[*interface*]

# DESCRIPTION

The **satpulsetool** **sdp** command manages software-defined pins (SDP) on PTP hardware clocks (PHC). These pins can be configured for various functions including external timestamp input (PPS input) and periodic output (PPS output).

At most one of the **\-\-show** and **\-\-extts** options can be specified.
If neither of these options is specified, the command behaves as if **\-\-show** was specified.

# OPTIONS

**\-h**, **\-\-help**  
: Show usage help for the **sdp** command.

**\-\-show**  
: Show information about SDPs.
If the *interface* argument is specified, shows information about the SDPs of that interface.
Otherwise, shows information about all interfaces with a PHC that has SDPs.

**\-i**, **\-\-extts**  
: Check input on a pin.
This configures a pin of the interface as an input pin and reads timestamp events from the PHC device.
It will show each timestamp as it is received.
It will read timestamp events for the length of time specified by the **\-\-t** option; default is 2 seconds.
if this is 0, it will read until interrupted.
The pin to be used is specified with the **\-\-pin** option; the default is pin 0.
The **\-\-chan** option can also be used to specify the timestamping channel to be used; the default is channel 0.

**\-t**, **\-\-timeout** *seconds*  
: Length of time in seconds for which to read timestamp events. 
Applies only when **\-\-extts** options is specified.
The default is 2.

**\-\-pin** *index*\|*name*  
: Select the pin to be used by **\-\-extts**. The default is 0.

**\-\-chan** *index*  
: Select the channel to be used by **\-\-extts**. The default is 0.

**\-\-show\-stale**  
: Include stale timestamps in the output. By default, timestamps that were buffered before reading timestamp events started are not displayed. Applies only when **\-\-extts** options is specified.

**\-j**, **\-\-jsonl**  
: Output in JSON lines format instead of human-readable text.

# EXAMPLES

List all interfaces with PTP hardware clock and pins:

    satpulsetool sdp

Show pin configuration for interface enp4s0:

    satpulsetool sdp enp4s0

Check input on pin 1 of enp4s0 for 10 seconds:

    satpulsetool sdp -i --pin 1 --timeout 10 enp4s0

Check input continuously with JSON output:

    satpulsetool sdp -i --timeout 0 --jsonl enp4s0

Show all timestamps including buffered ones for 5 seconds:

    satpulsetool sdp -i --show-stale --timeout 5 enp4s0

# NOTES

Software-defined pins are a feature of certain network interface cards with PTP hardware clock support. Common examples include Intel i210, i225, and i226 series controllers which provide 4 SDP pins (SDP0-SDP3).

Pin functions must be configured before use. The **\-\-extts** mode handles this configuration automatically, but manual configuration through sysfs may be required for other use cases.

External timestamp events from all channels are reported, not just the configured channel. The channel number is included in JSON output to distinguish between sources.

# EXIT STATUS

**0**  
: Successful operation.

**1**  
: An error occurred (invalid interface, permission denied, device error, etc.).

**2**  
: No data available (no interfaces found in list mode, or no timestamps received in extts mode). This exit status is reserved for future implementation.

# FILES

**/dev/ptp***N*  
: PTP hardware clock device files used for pin configuration and timestamp reading.

**/sys/class/ptp/ptp***N***/**  
: Sysfs directory containing PHC information including pin configuration.

**/sys/class/net/***interface***/device/ptp/***  
: Symlink to the PTP clock associated with a network interface.

# SEE ALSO

**satpulsetool(1)**, **satpulsed(8)**, **ptp4l(8)**, **phc2sys(8)**